package models

import (
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/spf13/viper"
	"github.com/uptrace/bun"
)

type ClientType string

const (
	ResidentialType ClientType = "residential"
	MobileType      ClientType = "mobile"
)

type SoaxClient struct {
	bun.BaseModel `bun:"table:soax_clients,alias:sc"`

	ID             int64     `bun:",pk,autoincrement"`
	IP             string    `bun:",unique,notnull"` // Changed from pk to unique
	ClientType     string    `bun:",notnull"`
	SessionID      int       `bun:",notnull"`
	SessionLength  int       `bun:",notnull"`
	Time           time.Time `bun:",notnull"`
	ExpirationTime time.Time `bun:",notnull"`
	IPVersion      string    `bun:",notnull"`
	Carrier        string
	City           string
	CountryCode    string `bun:",notnull"`
	CountryName    string `bun:",notnull"`
	ASNumber       string
	ASOrg          string
	LastSeen       time.Time `bun:",notnull"`
	UpdateCount    int       `bun:",notnull,default:0"`
	ISP            string    `bun:",notnull"`
}

type SoaxIPInfo struct {
	Status bool   `json:"status"`
	Reason string `json:"reason"`
	Data   struct {
		Carrier     string `json:"carrier"`
		City        string `json:"city"`
		CountryCode string `json:"country_code"`
		CountryName string `json:"country_name"`
		IP          string `json:"ip"`
		ISP         string `json:"isp"`
		Region      string `json:"region"`
	} `json:"data"`
}

// TransportURL generates the SOAX proxy transport URL for this client
func (c *SoaxClient) TransportURL() string {
	var packageID, packageKey string

	// Get the appropriate package ID and key based on client type
	switch c.ClientType {
	case "residential":
		packageID = viper.GetString("soax.residential_package_id")
		packageKey = viper.GetString("soax.residential_package_key")
	case "mobile":
		packageID = viper.GetString("soax.mobile_package_id")
		packageKey = viper.GetString("soax.mobile_package_key")
	}

	// Encode ISP name properly
	encodedISP := strings.ReplaceAll(url.QueryEscape(c.ISP), "+", "%20")

	// Generate transport URL
	return fmt.Sprintf("socks5://package-%s-country-%s-sessionid-%d-sessionlength-%d-isp-%s-opt-uniqip:%s@%s",
		packageID,
		c.CountryCode,
		c.SessionID,
		c.SessionLength,
		encodedISP,
		packageKey,
		viper.GetString("soax.endpoint"))
}
