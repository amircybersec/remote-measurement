package models

import (
	"time"

	"github.com/uptrace/bun"
)

// Update the Measurement model to include session tracking
type Measurement struct {
	bun.BaseModel `bun:"table:measurement,alias:m"`

	ID              int64     `bun:",pk,autoincrement"`
	ClientID        int64     `bun:",notnull"`
	ServerID        int64     `bun:",notnull"`
	Time            time.Time `bun:",notnull"`
	Protocol        string    `bun:",notnull"`
	SessionID       string
	RetryNumber     int
	PrefixUsed      string
	ErrorMsg        string
	ErrorMsgVerbose string
	ErrorOp         string
	Duration        int64
	FullReport      []byte `bun:",type:jsonb"`

	Client *SoaxClient `bun:"rel:belongs-to,join:client_id=id"`
	Server *Server     `bun:"rel:belongs-to,join:server_id=id"`
}

// Define indexes and foreign keys
type _ struct {
	_ struct{} `bun:"index:measurements_client_id_idx,column:client_id"`
	_ struct{} `bun:"index:measurements_server_id_idx,column:server_id"`
	_ struct{} `bun:"fk:client_id,references:soax_clients(id) on delete cascade on update cascade"`
	_ struct{} `bun:"fk:server_id,references:servers(id) on delete cascade on update cascade"`
}
