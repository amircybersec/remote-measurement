package connectivity

import (
	"connectivity-tester/pkg/models"
	"context"
	"encoding/json"
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

type ConnectivityReport struct {
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
	Duration int64     `json:"duration_ms"`
}

type errorJSON struct {
	// TODO: add Shadowsocks/Transport error
	Op string `json:"op,omitempty"`
	// Posix error, when available
	PosixError string `json:"posix_error,omitempty"`
	// TODO: remove IP addresses
	Msg        string `json:"msg,omitempty"`
	MsgVerbose string `json:"msg_verbose,omitempty"`
}

func makeErrorRecord(result *connectivity.ConnectivityError) *errorJSON {
	if result == nil {
		return nil
	}
	var record = new(errorJSON)
	record.Op = result.Op
	record.PosixError = result.PosixError
	record.Msg = findBaseError(result.Err).Error()
	record.MsgVerbose = result.Err.Error()

	return record
}

// findBaseError unwraps an error chain to find the most basic underlying error
func findBaseError(err error) error {
	for err != nil {
		// Try to unwrap as joined errors first
		if unwrapInterface, ok := err.(interface{ Unwrap() []error }); ok {
			errs := unwrapInterface.Unwrap()
			if len(errs) > 0 {
				// Take the last error in the joined slice as it's likely
				// to be the most specific one
				err = errs[len(errs)-1]
				continue
			}
		}

		// Try to unwrap as single error
		unwrapped := errors.Unwrap(err)
		if unwrapped == nil {
			// We've reached the base error
			return err
		}
		err = unwrapped
	}
	return err
}
func (r ConnectivityReport) IsSuccess() bool {
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
	onDialStart func(ctx context.Context, network, addr string),
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
			ConnectStart: func(network, addr string) {
				onDialStart(ctx, network, addr)
			},
			ConnectDone: func(network, addr string, connErr error) {
				onDial(ctx, network, addr, connErr)
			},
		})
		return dialer.DialPacket(ctx, addr)
	})
}

// TestConnectivity performs the connectivity test with the given parameters
func TestConnectivity(transportConfig, proto, resolver, domain string) (ConnectivityReport, error) {
	var report ConnectivityReport

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

	configToDialer.BaseStreamDialer = transport.FuncStreamDialer(func(ctx context.Context, addr string) (transport.StreamConn, error) {
		hostname, _, err := net.SplitHostPort(addr)
		if err != nil {
			return nil, err
		}
		onDial := func(ctx context.Context, network, addr string, connErr error) {
			ip, port, err := net.SplitHostPort(addr)
			if err != nil {
				return
			}
			report := tcpReport{
				Hostname: hostname,
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
		}
		onDialStart := func(ctx context.Context, network, addr string) {
			mu.Lock()
			connectStart[network+"|"+addr] = time.Now()
			mu.Unlock()
		}

		return newTCPTraceDialer(onDNS, onDial, onDialStart).DialStream(ctx, addr)
	})

	configToDialer.BasePacketDialer = transport.FuncPacketDialer(func(ctx context.Context, addr string) (net.Conn, error) {
		hostname, _, err := net.SplitHostPort(addr)
		if err != nil {
			return nil, err
		}
		onDialStart := func(ctx context.Context, network, addr string) {
			mu.Lock()
			connectStart[network+"|"+addr] = time.Now()
			mu.Unlock()
		}
		onDial := func(ctx context.Context, network, addr string, connErr error) {
			ip, port, err := net.SplitHostPort(addr)
			if err != nil {
				return
			}
			report := udpReport{
				Hostname: hostname,
				IP:       ip,
				Port:     port,
				Time:     connectStart[network+"|"+addr].UTC().Truncate(time.Second),
				Duration: time.Since(connectStart[network+"|"+addr]).Milliseconds(),
			}
			if connErr != nil {
				report.Error = connErr.Error()
			}
			mu.Lock()
			udpReports = append(udpReports, report)
			mu.Unlock()
		}

		return newUDPTraceDialer(onDNS, onDial, onDialStart).DialPacket(ctx, addr)
	})

	var dnsResolver dns.Resolver
	switch proto {
	case "tcp":
		streamDialer, err := configToDialer.NewStreamDialer(endToEndTransport)
		if err != nil {
			return ConnectivityReport{}, err
		}
		dnsResolver = dns.NewTCPResolver(streamDialer, resolverAddress)
	case "udp":
		packetDialer, err := configToDialer.NewPacketDialer(endToEndTransport)
		if err != nil {
			return ConnectivityReport{}, err
		}
		dnsResolver = dns.NewUDPResolver(packetDialer, resolverAddress)
	default:
		return ConnectivityReport{}, errors.New("invalid protocol")
	}

	startTime := time.Now()
	result, err := connectivity.TestConnectivityWithResolver(context.Background(), dnsResolver, domain)
	if err != nil {
		return ConnectivityReport{}, err
	}
	testDuration := time.Since(startTime)

	report = ConnectivityReport{
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

	reportJSON, err := json.Marshal(report)
	if err != nil {
		return ConnectivityReport{}, err
	}
	fmt.Printf("report: %v\n", string(reportJSON))

	return report, nil
}

func UpdateResultFromReport(result *models.Server, report ConnectivityReport, proto string) {
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
