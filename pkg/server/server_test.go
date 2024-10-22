package server

import (
	"fmt"
	"net/url"
	"reflect"
	"testing"
)

func TestResolveURL(t *testing.T) {
	tests := []struct {
		name        string
		transport   string
		wantHost    string
		wantResolvedCount int
		wantErr     bool
	}{
		{
			name:      "Valid IP address",
			transport: "ss://user:pass@192.168.1.1:8388",
			wantHost:  "192.168.1.1",
			wantResolvedCount: 1,
			wantErr:   false,
		},
		{
			name:      "Valid domain name",
			transport: "ss://user:pass@example.com:8388",
			wantHost:  "example.com",
			wantResolvedCount: 2,
			wantErr:   false,
		},
		{
			name:      "Invalid URL",
			transport: "invalid-url",
			wantHost:  "",
			wantResolvedCount: 0,
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := resolveURL(tt.transport)
			fmt.Printf("got: %v\n", got)
			if (err != nil) != tt.wantErr {
				t.Errorf("resolveURL() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if err == nil {
				if got.Host != tt.wantHost {
					t.Errorf("resolveURL() got Host = %v, want %v", got.Host, tt.wantHost)
				}
				if len(got.Resolved) != tt.wantResolvedCount {
					t.Errorf("resolveURL() got %d resolved URLs, want %d", len(got.Resolved), tt.wantResolvedCount)
				}
			}
		})
	}
}

func TestAddTransportInfo(t *testing.T) {
	tests := []struct {
		name    string
		input   *resolvedURLs
		want    []transportJSON
		wantErr bool
	}{
		{
			name: "Valid IP address",
			input: &resolvedURLs{
				Host: "192.168.1.1",
				Resolved: []*url.URL{
					mustParseURL("ss://user:pass@192.168.1.1:8388?key1=value1&key2=value2"),
				},
			},
			want: []transportJSON{
				{
					Scheme:    "ss",
					UserInfo:  "user:pass",
					Host:      "",
					IP:        "192.168.1.1",
					IPVersion: "v4",
					Port:      "8388",
					Params:    map[string]string{"key1": "value1", "key2": "value2"},
				},
			},
			wantErr: false,
		},
		{
			name: "Valid domain name",
			input: &resolvedURLs{
				Host: "example.com",
				Resolved: []*url.URL{
					mustParseURL("ss://user:pass@93.184.216.34:8388"),
					mustParseURL("ss://user:pass@[2001:db8::1]:8388"),

				},
			},
			want: []transportJSON{
				{
					Scheme:    "ss",
					UserInfo:  "user:pass",
					Host:      "example.com",
					IP:        "93.184.216.34",
					IPVersion: "v4",
					Port:      "8388",
					Params:    map[string]string{},
				},
				{	
					Scheme:    "ss",
					UserInfo:  "user:pass",
					Host:      "example.com",
					IP:        "2001:db8::1",
					IPVersion: "v6",
					Port:      "8388",
					Params:    map[string]string{},
				},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := addTransportInfo(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("addTransportInfo() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(tt.input.TransportJSON, tt.want) {
				t.Errorf("addTransportInfo() got = %v, want %v", tt.input.TransportJSON, tt.want)
			}
		})
	}
}

// Helper function to parse URL without error checking
func mustParseURL(s string) *url.URL {
	u, _ := url.Parse(s)
	return u
}
