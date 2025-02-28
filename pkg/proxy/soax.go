package proxy

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"math/rand"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"connectivity-tester/pkg/fetch"
	"connectivity-tester/pkg/ipinfo"
	"connectivity-tester/pkg/models"
)

type SoaxProvider struct {
	config Config
	logger *slog.Logger
}

func newSoaxProvider(config Config, logger *slog.Logger) *SoaxProvider {
	// Validate required SOAX configuration
	if config.System != SystemSOAX {
		panic("invalid system type for SOAX provider")
	}
	if config.APIKey == "" {
		panic("SOAX API key is required")
	}
	if config.PackageID == "" {
		panic("SOAX package ID is required")
	}
	if config.PackageKey == "" {
		panic("SOAX package key is required")
	}
	if config.Endpoint == "" {
		panic("SOAX endpoint is required")
	}
	if config.SessionLength == 0 {
		config.SessionLength = 360 // default to 6 minutes if not specified
	}
	if config.MaxWorkers == 0 {
		config.MaxWorkers = 1 // default to 1 worker if not specified
	}

	return &SoaxProvider{
		config: config,
		logger: logger,
	}
}

func (p *SoaxProvider) GetProviderName() string {
	return "soax"
}

func shuffleStrings(slice []string) {
	r := rand.New(rand.NewSource(time.Now().UnixNano()))
	for i := len(slice) - 1; i > 0; i-- {
		j := r.Intn(i + 1)
		slice[i], slice[j] = slice[j], slice[i]
	}
}

// GetSessionLength returns the session length in seconds
func (p *SoaxProvider) GetSessionLength() int {
	return p.config.SessionLength
}

func (p *SoaxProvider) GetISPList(countryISO string, clientType models.ClientType) ([]string, error) {
	var packageKey string
	var endpoint string
	var url string

	if clientType == models.ResidentialType {
		packageKey = p.config.PackageKey
		endpoint = "https://api.soax.com/api/get-country-isp"
		url = fmt.Sprintf("%s?api_key=%s&package_key=%s&country_iso=%s&conn_type=wifi",
			endpoint, p.config.APIKey, packageKey, countryISO)
	} else {
		packageKey = p.config.PackageKey
		endpoint = "https://api.soax.com/api/get-country-operators"
		url = fmt.Sprintf("%s?api_key=%s&package_key=%s&country_iso=%s",
			endpoint, p.config.APIKey, packageKey, countryISO)
	}

	resp, err := http.Get(url)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch ISP list: %w", err)
	}
	defer resp.Body.Close()

	var isps []string
	if err := json.NewDecoder(resp.Body).Decode(&isps); err != nil {
		// log body of the response
		body, _ := io.ReadAll(resp.Body)
		fmt.Println("body:", string(body))
		p.logger.Error("failed to decode ISP list", "body", string(body))
		return nil, fmt.Errorf("failed to decode ISP list: %w", err)
	}

	// Shuffle the ISP list before returning
	shuffleStrings(isps)

	return isps, nil
}

func (p *SoaxProvider) GetClientForISP(isp string, clientType models.ClientType, country string, maxRetries int) (*models.Client, error) {
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
			Proxy:         string(SystemSOAX),
		}

		transport := p.BuildTransportURL(tempClient)

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
		asnInfo, err := ipinfo.GetIPInfo(ipInfo.Data.IP)
		if err != nil {
			continue
		}

		// Parse ASN and org name
		orgParts := strings.SplitN(asnInfo.Org, " ", 2)
		var asNumber, asOrg string
		if len(orgParts) == 2 {
			asNumber = strings.TrimPrefix(orgParts[0], "AS")
			asOrg = orgParts[1]
		} else {
			asOrg = asnInfo.Org
		}

		// Use ipinfo.io city as fallback if SOAX city is empty
		city := ipInfo.Data.City
		if city == "" {
			city = asnInfo.City
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
			Proxy:          string(SystemSOAX),
		}

		return client, nil
	}

	return nil, fmt.Errorf("failed to get client for ISP %s after %d attempts", isp, maxRetries)
}

func (p *SoaxProvider) BuildTransportURL(client *models.Client) string {
	var packageID, packageKey string

	// Get the appropriate package ID and key based on client type
	switch client.ClientType {
	case string(models.ResidentialType):
		packageID = p.config.PackageID
		packageKey = p.config.PackageKey
	case string(models.MobileType):
		packageID = p.config.PackageID
		packageKey = p.config.PackageKey
	}

	// Encode ISP name properly
	encodedISP := strings.ReplaceAll(url.QueryEscape(client.ISP), "+", "%20")

	// Generate transport URL
	return fmt.Sprintf("socks5://package-%s-country-%s-sessionid-%d-sessionlength-%d-isp-%s-opt-uniqip:%s@%s",
		packageID,
		client.CountryCode,
		client.SessionID,
		client.SessionLength,
		encodedISP,
		packageKey,
		p.config.Endpoint)
}

// IsValid checks if the client's IP hasn't changed and is still valid
func (p *SoaxProvider) IsValidClient(client *models.Client) (bool, error) {
	transport := p.BuildTransportURL(client)

	opts := fetch.Options{
		Transport:  transport,
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

func (p *SoaxProvider) GetMaxWorkers() int {
	return p.config.MaxWorkers
}
