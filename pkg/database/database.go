package database

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"sync"

	"connectivity-tester/pkg/models"

	"github.com/spf13/viper"
	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/pgdialect"
	"github.com/uptrace/bun/driver/pgdriver"
)

type DB struct {
	*bun.DB
}

func NewDB() (*DB, error) {
	dsn := fmt.Sprintf("postgres://%s:%s@%s:%d/%s?sslmode=%s",
		viper.GetString("database.user"),
		viper.GetString("database.password"),
		viper.GetString("database.host"),
		viper.GetInt("database.port"),
		viper.GetString("database.dbname"),
		viper.GetString("database.sslmode"),
	)

	sqldb := sql.OpenDB(pgdriver.NewConnector(pgdriver.WithDSN(dsn)))

	db := bun.NewDB(sqldb, pgdialect.New())

	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("failed to ping database: %v", err)
	}

	return &DB{db}, nil
}

// InitSchema creates the necessary tables if they don't exist
func (db *DB) InitSchema(ctx context.Context) error {
	_, err := db.NewCreateTable().
		Model((*models.Server)(nil)).
		IfNotExists().
		Exec(ctx)

	if err != nil {
		return fmt.Errorf("failed to create table: %v", err)
	}

	return nil
}

func (db *DB) UpsertServer(ctx context.Context, server *models.Server) error {
	_, err := db.NewInsert().
		Model(server).
		On("CONFLICT (ip, port, user_info, domain_name) DO UPDATE").
		Set("full_access_link = EXCLUDED.full_access_link").
		Set("scheme = EXCLUDED.scheme").
		Set("ip_type = EXCLUDED.ip_type").
		Set("as_number = EXCLUDED.as_number").
		Set("as_org = EXCLUDED.as_org").
		Set("city = EXCLUDED.city").
		Set("region = EXCLUDED.region").
		Set("country = EXCLUDED.country").
		Set("updated_at = CURRENT_TIMESTAMP").
		Exec(ctx)

	if err != nil {
		return fmt.Errorf("error upserting server: %v", err)
	}

	return nil
}

func (db *DB) GetAllServers(ctx context.Context) ([]models.Server, error) {
	var servers []models.Server
	err := db.NewSelect().
		Model(&servers).
		Scan(ctx)

	if err != nil {
		return nil, fmt.Errorf("error getting all servers: %v", err)
	}

	return servers, nil
}

func (db *DB) GetServersForRetest(ctx context.Context, retestTCP, retestUDP bool) ([]models.Server, error) {
	var servers []models.Server
	q := db.NewSelect().Model(&servers)

	if retestTCP && retestUDP {
		q = q.Where("(tcp_error_op IS NOT NULL AND tcp_error_op != '' AND tcp_error_op != 'connect') OR udp_error_msg IS NOT NULL")
	} else if retestTCP {
		q = q.Where("tcp_error_op IS NOT NULL AND tcp_error_op != '' AND tcp_error_op != 'connect'")
	} else if retestUDP {
		q = q.Where("udp_error_msg IS NOT NULL")
	}

	err := q.Scan(ctx)
	if err != nil {
		return nil, fmt.Errorf("error getting servers for retest: %v", err)
	}

	slog.Debug("Servers for retest", "servers", len(servers))

	return servers, nil
}

var (
	updateMutex sync.Mutex
	removeMutex sync.Mutex
)

func (db *DB) UpdateServerTestResults(ctx context.Context, server *models.Server) error {
	updateMutex.Lock()
	defer updateMutex.Unlock()

	_, err := db.NewUpdate().
		Model(server).
		Column("last_test_time", "tcp_error_msg", "tcp_error_op", "udp_error_msg", "udp_error_op").
		Where("ip = ? AND port = ? AND user_info = ?", server.IP, server.Port, server.UserInfo).
		Exec(ctx)

	if err != nil {
		return fmt.Errorf("error updating server test results: %v", err)
	}

	return nil
}

func (db *DB) RemoveServer(ctx context.Context, server *models.Server) error {
	removeMutex.Lock()
	defer removeMutex.Unlock()

	_, err := db.NewDelete().
		Model(server).
		Where("ip = ? AND port = ? AND user_info = ?", server.IP, server.Port, server.UserInfo).
		Exec(ctx)

	if err != nil {
		return fmt.Errorf("error removing server: %v", err)
	}

	return nil
}