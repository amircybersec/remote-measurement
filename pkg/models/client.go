package models

import (
	"time"

	"github.com/uptrace/bun"
)

type ClientType string

const (
	ResidentialType ClientType = "residential"
	MobileType      ClientType = "mobile"
)

type Client struct {
	bun.BaseModel `bun:"table:clients,alias:sc"`

	ID             int64     `bun:",pk,autoincrement"`
	IP             string    `bun:",notnull"`
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
	Proxy          string    `bun:",notnull"` // can be soax or proxyrack
	ProxyURL       string    `bun:"-"`        // Do not store in database
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
