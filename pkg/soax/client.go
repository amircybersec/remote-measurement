package soax

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"math/rand"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"connectivity-tester/pkg/database"
	"connectivity-tester/pkg/fetch"
	"connectivity-tester/pkg/ipinfo"
	"connectivity-tester/pkg/models"

	"github.com/spf13/viper"
	"golang.org/x/exp/slog"
)

type ClientType string

const (
	Residential ClientType = "residential"
	Mobile      ClientType = "mobile"
	MaxRetries             = 10 // Maximum retries per ISP
)

type FetchStats struct {
	UniqueClients   int
	TotalAttempts   int
	DuplicateIPs    int
	RequestedCount  int
	SkippedISPs     []string
	IsPartialResult bool
}

// getISPList fetches the list of ISPs/operators for a given country and client type
func GetISPList(countryISO string, clientType ClientType) ([]string, error) {
	apiKey := viper.GetString("soax.api_key")
	logger := slog.Default()
	var packageKey string
	var endpoint string

	if clientType == Residential {
		packageKey = viper.GetString("soax.residential_package_key")
		endpoint = "https://api.soax.com/api/get-country-isp"
	} else {
		packageKey = viper.GetString("soax.mobile_package_key")
		endpoint = "https://api.soax.com/api/get-country-operators"
	}

	url := fmt.Sprintf("%s?api_key=%s&package_key=%s&country_iso=%s",
		endpoint, apiKey, packageKey, countryISO)

	logger.Debug("Fetching ISP list",
		"country", countryISO,
		"clientType", clientType,
		"endpoint", endpoint,
		"url", url)

	fmt.Printf("url:%s\n", url)

	resp, err := http.Get(url)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch ISP list: %v", err)
	}
	defer resp.Body.Close()

	var isps []string
	if err := json.NewDecoder(resp.Body).Decode(&isps); err != nil {
		return nil, fmt.Errorf("failed to decode ISP list: %v", err)
	}

	return isps, nil
}

// tryGetClientForISP attempts to get a client for a specific ISP
// Update the tryGetClientForISP function to include client type
// File: pkg/soax/client.go

func GetClientForISP(isp string, clientType ClientType, country string, maxRetries int, db *database.DB) (*models.SoaxClient, error) {
	logger := slog.Default()
	var packageID, packageKey string
	if clientType == Residential {
		packageID = viper.GetString("soax.residential_package_id")
		packageKey = viper.GetString("soax.residential_package_key")
	} else {
		packageID = viper.GetString("soax.mobile_package_id")
		packageKey = viper.GetString("soax.mobile_package_key")
	}

	endpoint := viper.GetString("soax.endpoint")
	sessionLength := 6000 // 100 minutes

	logger.Debug("Attempting to get client",
		"isp", isp,
		"clientType", clientType,
		"country", country)

	for retry := 0; retry < maxRetries; retry++ {
		sessionID := rand.Intn(1000000)

		// Properly encode ISP name
		encodedISP := strings.ReplaceAll(url.QueryEscape(isp), "+", "%20")

		transport := fmt.Sprintf("socks5://package-%s-country-%s-sessionid-%d-sessionlength-%d-isp-%s-opt-uniqip:%s@%s",
			packageID, country, sessionID, sessionLength, encodedISP, packageKey, endpoint)

		logger.Debug("Trying to get client",
			"isp", isp,
			"retry", retry,
			"sessionID", sessionID)

		client, err := getClientInfo(transport, sessionID, sessionLength)
		if err != nil {
			if strings.Contains(err.Error(), "general SOCKS server failure") {
				logger.Debug("No available nodes for ISP",
					"isp", isp,
					"error", err)
				return nil, fmt.Errorf("no available nodes for ISP %s", isp)
			}
			logger.Debug("Failed to get client",
				"isp", isp,
				"retry", retry,
				"error", err)
			continue
		}

		// Check if this IP is already in use by a non-expired client
		existingClient, err := db.GetActiveClientByIP(context.Background(), client.IP)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			logger.Error("Error checking for existing client",
				"ip", client.IP,
				"error", err)
			continue
		}

		// If we found an active client with this IP, skip it
		if existingClient != nil && existingClient.ExpirationTime.After(time.Now()) {
			logger.Debug("Skipping duplicate IP with active session",
				"ip", client.IP,
				"expiresAt", existingClient.ExpirationTime)
			continue
		}

		// Set additional fields
		client.ISP = isp
		client.ClientType = string(clientType)
		client.SessionLength = sessionLength

		logger.Debug("Successfully got unique client",
			"ip", client.IP,
			"isp", isp,
			"retry", retry)

		return &client, nil
	}

	logger.Debug("Failed to get unique IP after max retries",
		"isp", isp,
		"maxRetries", maxRetries)

	return nil, fmt.Errorf("failed to get unique IP for ISP %s after %d attempts", isp, maxRetries)
}

func getClientInfo(transport string, sessionID int, sessionLength int) (models.SoaxClient, error) {
	opts := fetch.Options{
		Transport:  transport,
		Method:     "GET",
		Headers:    []string{"User-Agent: MyApp/1.0"},
		TimeoutSec: 10,
		Verbose:    true,
	}

	result, err := fetch.Fetch("https://checker.soax.com/api/ipinfo", opts)
	if err != nil {
		return models.SoaxClient{}, err
	}

	var ipInfo models.SoaxIPInfo
	err = json.Unmarshal(result.Body, &ipInfo)
	if err != nil {
		return models.SoaxClient{}, err
	}

	// Get ASN information from ipinfo.io
	asnInfo, err := ipinfo.GetIPInfo(ipInfo.Data.IP)
	if err != nil {
		return models.SoaxClient{}, err
	}

	orgParts := strings.SplitN(asnInfo.Org, " ", 2)
	var ASNumber, ASOrg string
	if len(orgParts) == 2 {
		ASNumber = strings.TrimPrefix(orgParts[0], "AS")
		ASOrg = orgParts[1]
	} else {
		// If we can't parse it properly, store the whole string in ASOrg
		ASOrg = asnInfo.Org
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

	now := time.Now()
	client := models.SoaxClient{
		SessionID:      sessionID,
		SessionLength:  sessionLength,
		Time:           now,
		ExpirationTime: now.Add(time.Duration(sessionLength) * time.Second),
		IP:             ipInfo.Data.IP,
		IPVersion:      ipVersion,
		Carrier:        ipInfo.Data.Carrier,
		City:           ipInfo.Data.City,
		CountryCode:    ipInfo.Data.CountryCode,
		CountryName:    ipInfo.Data.CountryName,
		ASNumber:       ASNumber,
		ASOrg:          ASOrg,
	}

	return client, nil
}
