package proxy

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"math/rand"
	"net"
	"net/url"
	"strings"
	"time"

	"connectivity-tester/pkg/fetch"
	"connectivity-tester/pkg/ipinfo"
	"connectivity-tester/pkg/models"
)

type ProxyRackProvider struct {
	config Config
	logger *slog.Logger
}

func newProxyRackProvider(config Config, logger *slog.Logger) *ProxyRackProvider {
	// Validate required ProxyRack configuration
	if config.System != SystemProxyRack {
		panic("invalid system type for ProxyRack provider")
	}
	if config.Username == "" {
		panic("ProxyRack username is required")
	}
	if config.APIKey == "" {
		panic("ProxyRack API key is required")
	}
	if config.Endpoint == "" {
		panic("ProxyRack endpoint is required")
	}
	if config.SessionLength == 0 {
		config.SessionLength = 360 // default to 6 minutes if not specified
	}
	if config.MaxWorkers == 0 {
		config.MaxWorkers = 1 // default to 1 worker if not specified
	}

	return &ProxyRackProvider{
		config: config,
		logger: logger,
	}
}

// GetSessionLength returns the session length in seconds
func (p *ProxyRackProvider) GetSessionLength() int {
	return p.config.SessionLength
}

func (p *ProxyRackProvider) GetProviderName() string {
	return "proxyrack"
}

func (p *ProxyRackProvider) GetISPList(countryISO string, clientType models.ClientType) ([]string, error) {
	// Build a basic transport URL to access the API
	transport := fmt.Sprintf("socks5://%s-country-%s:%s@%s",
		p.config.Username,
		strings.ToUpper(countryISO),
		p.config.APIKey,
		p.config.Endpoint)

	opts := fetch.Options{
		Transport:  transport,
		Method:     "GET",
		TimeoutSec: 10,
	}

	apiURL := fmt.Sprintf("http://api.proxyrack.net/countries/%s/isps", countryISO)
	result, err := fetch.Fetch(apiURL, opts)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch ISP list: %w", err)
	}

	var isps []string
	if err := json.Unmarshal(result.Body, &isps); err != nil {
		return nil, fmt.Errorf("failed to decode ISP list: %w", err)
	}

	// Shuffle the ISP list before returning
	shuffleStrings(isps)

	return isps, nil
}

func (p *ProxyRackProvider) GetClientForISP(isp string, clientType models.ClientType, country string, maxRetries int) (*models.Client, error) {
	sessionLength := p.config.SessionLength

	for retry := 0; retry < maxRetries; retry++ {
		sessionID := rand.Intn(1000000)

		// Build initial client to get transport URL
		tempClient := &models.Client{
			SessionID:     sessionID,
			SessionLength: sessionLength,
			CountryCode:   country,
			ISP:           isp,
			ClientType:    string(clientType),
			Proxy:         string(SystemProxyRack),
		}

		transport := p.BuildTransportURL(tempClient)

		p.logger.Debug("fetching IP info",
			"transport", transport,
		)

		opts := fetch.Options{
			Transport:  transport,
			Method:     "GET",
			Headers:    []string{"User-Agent: MyApp/1.0"},
			TimeoutSec: 10,
		}

		result, err := fetch.Fetch("https://checker.soax.com/api/ipinfo", opts)
		if err != nil {
			if strings.Contains(err.Error(), "general SOCKS server failure") {
				return nil, fmt.Errorf("no available nodes for ISP %s", isp)
			}
			continue
		}

		var ipInfo models.SoaxIPInfo
		if err := json.Unmarshal(result.Body, &ipInfo); err != nil {
			continue
		}

		// Get ASN information
		ipInfoIO, err := ipinfo.GetIPInfo(ipInfo.Data.IP)
		if err != nil {
			continue
		}

		// Parse ASN and org name
		orgParts := strings.SplitN(ipInfoIO.Org, " ", 2)
		var asNumber, asOrg string
		if len(orgParts) == 2 {
			asNumber = strings.TrimPrefix(orgParts[0], "AS")
			asOrg = orgParts[1]
		} else {
			asOrg = ipInfoIO.Org
		}

		// Use ipinfo.io city as fallback if SOAX city is empty
		city := ipInfo.Data.City
		if city == "" {
			city = ipInfoIO.City
		}

		// Determine IP version
		ip := net.ParseIP(ipInfo.Data.IP)
		var ipVersion string
		if ip.To4() != nil {
			ipVersion = "v4"
		} else if ip.To16() != nil {
			ipVersion = "v6"
		} else {
			ipVersion = "unknown"
		}

		// ensure that the IP is from the correct country
		// sometimes clients have their VPN on which can cause the IP
		// to be from a different country
		if !strings.EqualFold(country, ipInfo.Data.CountryCode) {
			p.logger.Debug("IP is from a different country",
				"ip", ipInfo.Data.IP,
				"expected", country,
				"actual", ipInfo.Data.CountryCode)
			continue
		}

		now := time.Now()
		client := &models.Client{
			IP:             ipInfo.Data.IP,
			ClientType:     string(clientType),
			SessionID:      sessionID,
			SessionLength:  sessionLength,
			Time:           now,
			ExpirationTime: now.Add(time.Duration(sessionLength) * time.Second),
			IPVersion:      ipVersion,
			Carrier:        ipInfo.Data.Carrier,
			City:           city,
			CountryCode:    ipInfo.Data.CountryCode,
			CountryName:    ipInfo.Data.CountryName,
			ASNumber:       asNumber,
			ASOrg:          asOrg,
			LastSeen:       now,
			ISP:            isp,
			Proxy:          string(SystemProxyRack),
		}

		return client, nil
	}

	return nil, fmt.Errorf("failed to get client for ISP %s after %d attempts", isp, maxRetries)
}

// BuildTransportURL returns a transport URL for the ProxyRack provider
func (p *ProxyRackProvider) BuildTransportURL(client *models.Client) string {
	encodedISP := strings.ReplaceAll(url.QueryEscape(client.ISP), "+", "%20")

	return fmt.Sprintf("socks5://%s-country-%s-session-%d-refreshMinutes-%d-isp-%s-autoReplace-strict:%s@%s",
		p.config.Username,
		strings.ToUpper(client.CountryCode),
		client.SessionID,
		client.SessionLength/60,
		encodedISP,
		p.config.APIKey,
		p.config.Endpoint)
}

// IsValid checks if the client's IP hasn't changed and is still valid
func (p *ProxyRackProvider) IsValidClient(client *models.Client) (bool, error) {
	//transport := p.BuildTransportURL(client)

	opts := fetch.Options{
		Transport:  client.ProxyURL,
		Method:     "GET",
		Headers:    []string{"User-Agent: MyApp/1.0"},
		TimeoutSec: 10,
	}

	result, err := fetch.Fetch("https://checker.soax.com/api/ipinfo", opts)
	if err != nil {
		return false, fmt.Errorf("failed to fetch IP info: %w", err)
	}

	var ipInfo models.SoaxIPInfo
	if err := json.Unmarshal(result.Body, &ipInfo); err != nil {
		return false, fmt.Errorf("failed to decode IP info: %w", err)
	}

	// Check if the IP has changed
	if ipInfo.Data.IP != client.IP {
		p.logger.Info("client IP has changed",
			"old_ip", client.IP,
			"new_ip", ipInfo.Data.IP,
			"session_id", client.SessionID)

		// Mark the client as expired by setting expiration time to now
		client.ExpirationTime = time.Now()
		return false, nil
	}

	return true, nil
}

func (p *ProxyRackProvider) GetMaxWorkers() int {
	return p.config.MaxWorkers
}
