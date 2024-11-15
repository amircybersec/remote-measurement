package server

import (
	"log/slog"
	"net"
	"net/netip"
	"net/url"
	"strings"
)

// resolvedURLPart represents a resolved URL part.
// each part can have multiple resolved URLs resulting from resolution
// of the hostname to different IP addresses.
type resolvedURLs struct {
	Host          string
	URLs          []*url.URL
	TransportJSON []transportJSON `json:"transport_json"`
}

type transportJSON struct {
	Scheme             string            `json:"scheme"`
	UserInfo           string            `json:"user_info,omitempty"`
	Host               string            `json:"host,omitempty"`
	IP                 string            `json:"ip,omitempty"`
	IPVersion          string            `json:"ip_version,omitempty"`
	Port               string            `json:"port,omitempty"`
	Params             map[string]string `json:"params,omitempty"`
	ResolvedAccessLink string            `json:"resolved_access_link,omitempty"`
}

// resolveParts resolves the hostname in each part of the transport config
// to IP addresses and returns a list of resolved URL parts.
func resolveURL(transport string) (*resolvedURLs, error) {
	u, err := url.Parse(transport)
	if err != nil {
		slog.Error("Failed to parse transport config", "error", err)
		return nil, err
	}

	slog.Debug("Parsing transport config", "url", transport)
	ip := net.ParseIP(u.Hostname())
	if ip != nil {
		// hostname is an IP address
		return &resolvedURLs{Host: u.Hostname(), URLs: []*url.URL{u}}, nil
	} else {
		// hostname is a domain name, try to resolve it
		var accessLinks []*url.URL
		ips, err := net.LookupIP(u.Hostname())
		if err != nil {
			slog.Error("Failed to resolve hostname", "hostname", u.Hostname(), "error", err)
			return nil, err
		}
		for _, ip := range ips {
			tempURL := *u
			// Overwrite the hostname with the resolved IP address
			if ip.To4() != nil {
				tempURL.Host = ip.String() + ":" + u.Port()
			} else if ip.To16() != nil {
				tempURL.Host = "[" + ip.String() + "]" + ":" + u.Port()
			}
			accessLinks = append(accessLinks, &tempURL)
		}
		return &resolvedURLs{Host: u.Hostname(), URLs: accessLinks}, nil
	}
}

func addTransportInfo(r *resolvedURLs) error {
	for _, u := range r.URLs {
		params := make(map[string]string)

		// Use the RawQuery field to get the original encoded query string
		rawQuery := u.RawQuery
		pairs := strings.Split(rawQuery, "&")

		for _, pair := range pairs {
			kv := strings.SplitN(pair, "=", 2) // Split only on the first '='
			if len(kv) == 2 {
				params[kv[0]] = kv[1]
			}
		}

		var domain string
		ip := net.ParseIP(r.Host)
		if ip != nil {
			domain = ""
		} else {
			domain = r.Host
		}

		var ipVersion string
		var ipAddress string
		ipAddr, err := netip.ParseAddr(u.Hostname())
		if err != nil {
			ipAddress = ""
			ipVersion = ""
		} else {
			ipAddress = ipAddr.String()
			// Check IP version
			if ipAddr.Is4() {
				ipVersion = "v4"
			} else if ipAddr.Is6() {
				ipVersion = "v6"
			}

		}

		r.TransportJSON = append(r.TransportJSON, transportJSON{
			Scheme:             u.Scheme,
			Host:               domain,
			UserInfo:           u.User.String(),
			IP:                 ipAddress,
			IPVersion:          ipVersion,
			Port:               u.Port(),
			Params:             params,
			ResolvedAccessLink: u.String(),
		})
	}
	return nil
}
