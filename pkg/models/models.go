package models

import (
	"time"

	"github.com/uptrace/bun"
)

type Server struct {
	bun.BaseModel `bun:"table:servers,alias:s"`

	IP             string    `bun:",pk"`
	Port           string    `bun:",pk"`
	UserInfo       string    `bun:",pk"`
	FullAccessLink string    `bun:",unique,notnull"`
	Scheme         string    `bun:",notnull"`
	DomainName     string	 `bun:",pk"`
	IPType         string
	ASNumber       string
	ASOrg          string
	City           string
	Region         string
	Country        string
	LastTestTime   time.Time `bun:",notnull"`
	TCPErrorMsg   string
	TCPErrorOp    string
	UDPErrorMsg   string
	UDPErrorOp    string
	CreatedAt      time.Time `bun:",nullzero,notnull,default:current_timestamp"`
	UpdatedAt      time.Time `bun:",nullzero,notnull,default:current_timestamp"`
}
