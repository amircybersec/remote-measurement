package proxy

import (
	"fmt"
	"log/slog"
	"strings"
	"time"

	"connectivity-tester/pkg/ipinfo"
	"connectivity-tester/pkg/models"
)

type NoneProvider struct {
	config Config
	logger *slog.Logger
}

func newNoneProvider(config Config, logger *slog.Logger) *NoneProvider {
	return &NoneProvider{
		config: config,
		logger: logger,
	}
}

func (p *NoneProvider) GetProviderName() string {
	return "none"
}

// GetISPList for NoneProvider always returns a single empty string
// as we only have one local client
func (p *NoneProvider) GetISPList(countryISO string, clientType models.ClientType) ([]string, error) {
	return []string{"Default"}, nil
}

// GetClientForISP creates a local client representation. clientType, country, maxRetries are ignored.
func (p *NoneProvider) GetClientForISP(isp string, clientType models.ClientType, country string, maxRetries int) (*models.Client, error) {

	// Get local IP information
	ipInfoIO, err := ipinfo.GetIPInfo("")
	if err != nil {
		return nil, fmt.Errorf("failed to get local IP info: %w", err)
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

	client := &models.Client{
		IP:             ipInfoIO.IP,
		ClientType:     "residential",
		SessionID:      1,                    // Fixed session ID for local client
		SessionLength:  p.GetSessionLength(), // 24 hours - local client doesn't expire
		Time:           time.Now(),
		ExpirationTime: time.Now().Add(24 * time.Hour),
		IPVersion:      "v4", // You might want to detect this
		City:           ipInfoIO.City,
		CountryCode:    ipInfoIO.Country,
		ASNumber:       asNumber,
		ASOrg:          asOrg,
		ISP:            asOrg,
		Proxy:          "none",
		ProxyURL:       "", // Empty as we're connecting directly
	}

	return client, nil
}

// BuildTransportURL returns an empty string as we don't need a proxy transport
func (p *NoneProvider) BuildTransportURL(client *models.Client) string {
	return ""
}

// GetSessionLength returns 24 hours in seconds as local client doesn't expire
func (p *NoneProvider) GetSessionLength() int {
	return 86400
}

// IsValidClient always returns true for local client
func (p *NoneProvider) IsValidClient(client *models.Client) (bool, error) {
	return true, nil
}

func (p *NoneProvider) GetMaxWorkers() int {
	return p.config.MaxWorkers
}
