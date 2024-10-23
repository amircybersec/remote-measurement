package soax

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"net"
	"strings"
	"time"

	"connectivity-tester/pkg/fetch"
	"connectivity-tester/pkg/ipinfo"
	"connectivity-tester/pkg/models"

	"github.com/google/uuid"
	"github.com/spf13/viper"
)

type ClientType string

const (
	Residential ClientType = "residential"
	Mobile      ClientType = "mobile"
	MaxAttempts            = 50 // Maximum number of attempts to get unique IPs
)

type FetchStats struct {
	UniqueClients   int
	TotalAttempts   int
	DuplicateIPs    int
	RequestedCount  int
	IsPartialResult bool
}

func GetUniqueClients(clientType ClientType, country string, count int) ([]models.SoaxClient, FetchStats, error) {
	var clients []models.SoaxClient
	stats := FetchStats{
		RequestedCount: count,
	}
	seenIPs := make(map[string]bool)

	// Get package credentials based on client type
	var packageID, packageKey string
	if clientType == Residential {
		packageID = viper.GetString("soax.residential_package_id")
		packageKey = viper.GetString("soax.residential_package_key")
	} else {
		packageID = viper.GetString("soax.mobile_package_id")
		packageKey = viper.GetString("soax.mobile_package_key")
	}

	endpoint := viper.GetString("soax.endpoint")
	sessionLength := 6 // 10 minutes

	for stats.TotalAttempts < MaxAttempts && len(clients) < count {
		sessionID := rand.Int63()
		transport := fmt.Sprintf("socks5://package-%s-country-%s-sessionid-%d-sessionlength-%d:%s@%s",
			packageID, country, sessionID, sessionLength, packageKey, endpoint)

		client, err := getClientInfo(transport, sessionID, sessionLength)
		stats.TotalAttempts++

		if err != nil {
			fmt.Printf("Error getting client info for session %d: %v\n", sessionID, err)
			continue
		}

		if seenIPs[client.IP] {
			stats.DuplicateIPs++
			fmt.Printf("Duplicate IP found: %s (Attempt: %d)\n", client.IP, stats.TotalAttempts)
			continue
		}

		seenIPs[client.IP] = true
		stats.UniqueClients++
		clients = append(clients, client)

		fmt.Printf("New unique IP found: %s (Progress: %d/%d, Attempt: %d)\n",
			client.IP, len(clients), count, stats.TotalAttempts)
	}

	stats.IsPartialResult = len(clients) < count

	var err error
	if stats.IsPartialResult {
		err = fmt.Errorf("could only get %d unique IPs out of %d requested after %d attempts",
			len(clients), count, stats.TotalAttempts)
	}

	// Always return the clients we found, along with stats and any error
	return clients, stats, err
}
func getClientInfo(transport string, sessionID int64, sessionLength int) (models.SoaxClient, error) {
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
		UUID:           uuid.New().String(),
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
