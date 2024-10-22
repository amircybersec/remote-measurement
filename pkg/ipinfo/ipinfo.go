package ipinfo

import (
	"connectivity-tester/pkg/models"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/spf13/viper"
)

type IPInfoResponse struct {
	IP        string `json:"ip"`
	Hostname  string `json:"hostname"`
	Anycast   bool   `json:"anycast"`
	City      string `json:"city"`
	Region    string `json:"region"`
	Country   string `json:"country"`
	Loc       string `json:"loc"`
	Org       string `json:"org"`
	Postal    string `json:"postal"`
	Timezone  string `json:"timezone"`
}

func GetIPInfo(ip string) (IPInfoResponse, error) {
	url := fmt.Sprintf("https://ipinfo.io/%s?token=%s", ip, viper.GetString("ipinfo.token"))
	resp, err := http.Get(url)
	if err != nil {
		return IPInfoResponse{}, err
	}
	defer resp.Body.Close()

	var ipInfo IPInfoResponse
	err = json.NewDecoder(resp.Body).Decode(&ipInfo)
	if err != nil {
		return IPInfoResponse{}, err
	}

	return ipInfo, nil
}

func UpdateServerWithIPInfo(server *models.Server, ipInfo IPInfoResponse) {
	// Parse ASN and AS org name from the "org" field
	orgParts := strings.SplitN(ipInfo.Org, " ", 2)
	if len(orgParts) == 2 {
		server.ASNumber = strings.TrimPrefix(orgParts[0], "AS")
		server.ASOrg = orgParts[1]
	} else {
		// If we can't parse it properly, store the whole string in ASOrg
		server.ASOrg = ipInfo.Org
	}

	server.City = ipInfo.City
	server.Region = ipInfo.Region
	server.Country = ipInfo.Country
}