package connectivity

import (
	"connectivity-tester/pkg/models"
	"context"
	"errors"
	"flag"
	"fmt"
	"net"
	"net/http/httptrace"
	"os"
	"path"
	"sync"
	"time"

	"github.com/Jigsaw-Code/outline-sdk/dns"
	"github.com/Jigsaw-Code/outline-sdk/transport"
	"github.com/Jigsaw-Code/outline-sdk/x/configurl"
	"github.com/Jigsaw-Code/outline-sdk/x/connectivity"
)

type connectivityReport struct {
	Test           testReport  `json:"test"`
	DNSQueries     []dnsReport `json:"dns_queries,omitempty"`
	TCPConnections []tcpReport `json:"tcp_connections,omitempty"`
	UDPConnections []udpReport `json:"udp_connections,omitempty"`
}

type testReport struct {
	// Inputs
	Resolver string `json:"resolver"`
	Proto    string `json:"proto"`

	// Observations
	Time       time.Time  `json:"time"`
	DurationMs int64      `json:"duration_ms"`
	Error      *errorJSON `json:"error"`
}

type dnsReport struct {
	QueryName  string    `json:"query_name"`
	Time       time.Time `json:"time"`
	DurationMs int64     `json:"duration_ms"`
	AnswerIPs  []string  `json:"answer_ips"`
	Error      string    `json:"error"`
}

type tcpReport struct {
	Hostname string    `json:"hostname"`
	IP       string    `json:"ip"`
	Port     string    `json:"port"`
	Error    string    `json:"error"`
	Time     time.Time `json:"time"`
	Duration int64     `json:"duration_ms"`
}

type udpReport struct {
	Hostname string    `json:"hostname"`
	IP       string    `json:"ip"`
	Port     string    `json:"port"`
	Error    string    `json:"error"`
	Time     time.Time `json:"time"`
}

type errorJSON struct {
	// TODO: add Shadowsocks/Transport error
	Op string `json:"op,omitempty"`
	// Posix error, when available
	PosixError string `json:"posix_error,omitempty"`
	// TODO: remove IP addresses
	Msg string `json:"msg,omitempty"`
}

func makeErrorRecord(result *connectivity.ConnectivityError) *errorJSON {
	if result == nil {
		return nil
	}
	var record = new(errorJSON)
	record.Op = result.Op
	record.PosixError = result.PosixError
	record.Msg = unwrapAll(result.Err).Error()
	return record
}

func unwrapAll(err error) error {
	for {
		unwrapped := errors.Unwrap(err)
		if unwrapped == nil {
			return err
		}
		err = unwrapped
	}
}

func (r connectivityReport) IsSuccess() bool {
	if r.Test.Error == nil {
		return true
	} else {
		return false
	}
}

func init() {
	flag.Usage = func() {
		fmt.Fprintf(flag.CommandLine.Output(), "Usage: %s [flags...]\n", path.Base(os.Args[0]))
		flag.PrintDefaults()
	}
}
func newTCPTraceDialer(
	onDNS func(ctx context.Context, domain string) func(di httptrace.DNSDoneInfo),
	onDial func(ctx context.Context, network, addr string, connErr error),
	onDialStart func(ctx context.Context, network, addr string),
) transport.StreamDialer {
	dialer := &transport.TCPDialer{}
	var onDNSDone func(di httptrace.DNSDoneInfo)
	return transport.FuncStreamDialer(func(ctx context.Context, addr string) (transport.StreamConn, error) {
		ctx = httptrace.WithClientTrace(ctx, &httptrace.ClientTrace{
			DNSStart: func(di httptrace.DNSStartInfo) {
				onDNSDone = onDNS(ctx, di.Host)
			},
			DNSDone: func(di httptrace.DNSDoneInfo) {
				if onDNSDone != nil {
					onDNSDone(di)
					onDNSDone = nil
				}
			},
			ConnectStart: func(network, addr string) {
				onDialStart(ctx, network, addr)
			},
			ConnectDone: func(network, addr string, connErr error) {
				onDial(ctx, network, addr, connErr)
			},
		})
		return dialer.DialStream(ctx, addr)
	})
}

func newUDPTraceDialer(
	onDNS func(ctx context.Context, domain string) func(di httptrace.DNSDoneInfo),
	onDial func(ctx context.Context, network, addr string, connErr error),
) transport.PacketDialer {
	dialer := &transport.UDPDialer{}
	var onDNSDone func(di httptrace.DNSDoneInfo)
	return transport.FuncPacketDialer(func(ctx context.Context, addr string) (net.Conn, error) {
		ctx = httptrace.WithClientTrace(ctx, &httptrace.ClientTrace{
			DNSStart: func(di httptrace.DNSStartInfo) {
				onDNSDone = onDNS(ctx, di.Host)
			},
			DNSDone: func(di httptrace.DNSDoneInfo) {
				if onDNSDone != nil {
					onDNSDone(di)
					onDNSDone = nil
				}
			},
			ConnectDone: func(network, addr string, connErr error) {
				onDial(ctx, network, addr, connErr)
			},
		})
		return dialer.DialPacket(ctx, addr)
	})
}

// TestConnectivity performs the connectivity test with the given parameters
func TestConnectivity(transportConfig, proto, resolver, domain string) (connectivityReport, error) {
	var report connectivityReport

	endToEndTransport := transportConfig

	resolverAddress := net.JoinHostPort(resolver, "53")
	var connectStart = make(map[string]time.Time)
	var mu sync.Mutex
	dnsReports := make([]dnsReport, 0)
	tcpReports := make([]tcpReport, 0)
	udpReports := make([]udpReport, 0)
	configToDialer := configurl.NewDefaultConfigToDialer()
	
	onDNS := func(ctx context.Context, domain string) func(di httptrace.DNSDoneInfo) {
		dnsStart := time.Now()
		return func(di httptrace.DNSDoneInfo) {
			report := dnsReport{
				QueryName:  domain,
				Time:       dnsStart.UTC().Truncate(time.Second),
				DurationMs: time.Since(dnsStart).Milliseconds(),
			}
			if di.Err != nil {
				report.Error = di.Err.Error()
			}
			for _, ip := range di.Addrs {
				report.AnswerIPs = append(report.AnswerIPs, ip.IP.String())
			}
			mu.Lock()
			dnsReports = append(dnsReports, report)
			mu.Unlock()
		}
	}

	configToDialer.BaseStreamDialer = newTCPTraceDialer(onDNS,
		func(ctx context.Context, network, addr string, connErr error) {
			ip, port, _ := net.SplitHostPort(addr)
			report := tcpReport{
				IP:       ip,
				Port:     port,
				Time:     connectStart[network+"|"+addr].UTC().Truncate(time.Second),
				Duration: time.Since(connectStart[network+"|"+addr]).Milliseconds(),
			}
			if connErr != nil {
				report.Error = connErr.Error()
			}
			mu.Lock()
			tcpReports = append(tcpReports, report)
			mu.Unlock()
		},
		func(ctx context.Context, network, addr string) {
			mu.Lock()
			connectStart[network+"|"+addr] = time.Now()
			mu.Unlock()
		},
	)

	configToDialer.BasePacketDialer = newUDPTraceDialer(onDNS,
		func(ctx context.Context, network, addr string, connErr error) {
			ip, port, _ := net.SplitHostPort(addr)
			report := udpReport{
				IP:   ip,
				Port: port,
				Time: time.Now().UTC().Truncate(time.Second),
			}
			if connErr != nil {
				report.Error = connErr.Error()
			}
			mu.Lock()
			udpReports = append(udpReports, report)
			mu.Unlock()
		},
	)

	var dnsResolver dns.Resolver
	switch proto {
	case "tcp":
		streamDialer, err := configToDialer.NewStreamDialer(endToEndTransport)
		if err != nil {
			return connectivityReport{}, err
		}
		dnsResolver = dns.NewTCPResolver(streamDialer, resolverAddress)
	case "udp":
		packetDialer, err := configToDialer.NewPacketDialer(endToEndTransport)
		if err != nil {
			return connectivityReport{}, err
		}
		dnsResolver = dns.NewUDPResolver(packetDialer, resolverAddress)
	default:
		return connectivityReport{}, errors.New("invalid protocol")
	}

	startTime := time.Now()
	result, err := connectivity.TestConnectivityWithResolver(context.Background(), dnsResolver, domain)
	if err != nil {
		return connectivityReport{}, err
	}
	testDuration := time.Since(startTime)

	report = connectivityReport{
		Test: testReport{
			Resolver:   resolverAddress,
			Proto:      proto,
			Time:       startTime.UTC().Truncate(time.Second),
			DurationMs: testDuration.Milliseconds(),
			Error:      makeErrorRecord(result),
		},
		DNSQueries:     dnsReports,
		TCPConnections: tcpReports,
		UDPConnections: udpReports,
	}

	return report, nil
}


func UpdateResultFromReport(result *models.Server, report connectivityReport, proto string) {
	if report.Test.Error != nil {
		errorMsg := report.Test.Error.Msg
		errorOp := report.Test.Error.Op
		switch proto {
		case "tcp":
			result.TCPErrorMsg = errorMsg
			result.TCPErrorOp = errorOp
		case "udp":
			result.UDPErrorMsg = errorMsg
			result.UDPErrorOp = errorOp
		}
	}
	result.LastTestTime = report.Test.Time
}

