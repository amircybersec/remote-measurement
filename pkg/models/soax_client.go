package models

import (
	"time"

	"github.com/uptrace/bun"
)

type SoaxClient struct {
	bun.BaseModel `bun:"table:soax_clients,alias:sc"`

	IP             string    `bun:",pk"`
	UUID           string    `bun:",unique,notnull"`
	SessionID      int64     `bun:",notnull"`
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
