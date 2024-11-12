package models

import (
	"time"

	"github.com/uptrace/bun"
)

type Server struct {
	bun.BaseModel `bun:"table:servers,alias:s"`

	ID             int64  `bun:",pk,autoincrement"`
	IP             string `bun:",unique:servers_ip_full_access_link_key,notnull"`
	Port           string `bun:",notnull"`
	UserInfo       string `bun:",notnull"`
	FullAccessLink string `bun:",unique:servers_ip_full_access_link_key,notnull"`
	Name           string
	Fragment       string
	Scheme         string `bun:",notnull"`
	DomainName     string `bun:",notnull"`
	IPType         string
	ASNumber       string
	ASOrg          string
	City           string
	Region         string
	Country        string
	LastTestTime   time.Time `bun:",notnull"`
	TCPErrorMsg    string
	TCPErrorOp     string
	UDPErrorMsg    string
	UDPErrorOp     string
	CreatedAt      time.Time `bun:",nullzero,notnull,default:current_timestamp"`
	UpdatedAt      time.Time `bun:",nullzero,notnull,default:current_timestamp"`
}
