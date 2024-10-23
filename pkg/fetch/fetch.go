// Package fetch provides functionality to make HTTP requests through various transports
package fetch

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/textproto"
	"strings"
	"time"

	"github.com/Jigsaw-Code/outline-sdk/x/configurl"
)

// Options contains all the configuration options for making a fetch request
type Options struct {
	// Transport config string
	Transport string
	// Override address to connect to. If empty, use the URL authority
	Address string
	// HTTP method to use (default: "GET")
	Method string
	// Raw HTTP headers to add (without \r\n)
	Headers []string
	// Timeout in seconds (default: 5)
	TimeoutSec int
	// Enable verbose debug output
	Verbose bool
}

// Result contains the response from a fetch request
type Result struct {
	// HTTP response
	Response *http.Response
	// Response body as bytes
	Body []byte
}

// Fetch makes an HTTP request with the given options
func Fetch(url string, opts Options) (*Result, error) {
	if opts.Method == "" {
		opts.Method = "GET"
	}
	if opts.TimeoutSec == 0 {
		opts.TimeoutSec = 5
	}

	var overrideHost, overridePort string
	if opts.Address != "" {
		var err error
		overrideHost, overridePort, err = net.SplitHostPort(opts.Address)
		if err != nil {
			// Fail to parse. Assume the address is host only.
			overrideHost = opts.Address
			overridePort = ""
		}
	}

	dialer, err := configurl.NewDefaultConfigToDialer().NewStreamDialer(opts.Transport)
	if err != nil {
		return nil, fmt.Errorf("could not create dialer: %w", err)
	}

	dialContext := func(ctx context.Context, network, addr string) (net.Conn, error) {
		host, port, err := net.SplitHostPort(addr)
		if err != nil {
			return nil, fmt.Errorf("invalid address: %w", err)
		}
		if overrideHost != "" {
			host = overrideHost
		}
		if overridePort != "" {
			port = overridePort
		}
		if !strings.HasPrefix(network, "tcp") {
			return nil, fmt.Errorf("protocol not supported: %v", network)
		}
		return dialer.DialStream(ctx, net.JoinHostPort(host, port))
	}

	httpClient := &http.Client{
		Transport: &http.Transport{DialContext: dialContext},
		Timeout:   time.Duration(opts.TimeoutSec) * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	req, err := http.NewRequest(opts.Method, url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Process headers
	if len(opts.Headers) > 0 {
		headerText := strings.Join(opts.Headers, "\r\n") + "\r\n\r\n"
		h, err := textproto.NewReader(bufio.NewReader(strings.NewReader(headerText))).ReadMIMEHeader()
		if err != nil {
			return nil, fmt.Errorf("invalid header line: %w", err)
		}
		for name, values := range h {
			for _, value := range values {
				req.Header.Add(name, value)
			}
		}
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("HTTP request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read of page body failed: %w", err)
	}

	return &Result{
		Response: resp,
		Body:     body,
	}, nil
}
