package database

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"sync"
	"time"

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

// GetWorkingServers returns servers with no errors and allowed ports
func (db *DB) GetWorkingServers(ctx context.Context, allowedPorts []string) ([]models.Server, error) {
	var servers []models.Server
	err := db.NewSelect().
		Model(&servers).
		Where("(tcp_error_msg IS NULL OR tcp_error_msg = '')").
		Where("(udp_error_msg IS NULL OR udp_error_msg = '')").
		Where("port IN (?)", bun.In(allowedPorts)).
		Scan(ctx)

	if err != nil {
		return nil, fmt.Errorf("error getting working servers: %v", err)
	}

	// Log the query for debugging
	logger := slog.Default()
	logger.Debug("GetWorkingServers query",
		"allowedPorts", allowedPorts,
		"serverCount", len(servers))

	return servers, nil
}

// InitSoaxSchema creates the SOAX clients table if it doesn't exist
func (db *DB) InitClientSchema(ctx context.Context) error {
	// Create the table if it doesn't exist
	_, err := db.NewCreateTable().
		Model((*models.SoaxClient)(nil)).
		IfNotExists().
		Exec(ctx)

	if err != nil {
		return fmt.Errorf("failed to create table: %v", err)
	}

	// Create indexes if they don't exist
	_, err = db.Exec(`
		DO $$
		BEGIN
			IF NOT EXISTS (SELECT 1 FROM pg_indexes WHERE tablename = 'soax_clients' AND indexname = 'soax_clients_country_code_idx') THEN
				CREATE INDEX soax_clients_country_code_idx ON soax_clients (country_code);
			END IF;
			IF NOT EXISTS (SELECT 1 FROM pg_indexes WHERE tablename = 'soax_clients' AND indexname = 'soax_clients_last_seen_idx') THEN
				CREATE INDEX soax_clients_last_seen_idx ON soax_clients (last_seen);
			END IF;
			IF NOT EXISTS (SELECT 1 FROM pg_indexes WHERE tablename = 'soax_clients' AND indexname = 'soax_clients_client_type_idx') THEN
				CREATE INDEX soax_clients_client_type_idx ON soax_clients (client_type);
			END IF;
		END $$;
	`)

	if err != nil {
		return fmt.Errorf("failed to create indexes: %v", err)
	}

	return nil
}

// InsertSoaxClients inserts or updates SOAX clients in the database
func (db *DB) InsertSoaxClients(ctx context.Context, clients []models.SoaxClient) ([]models.SoaxClient, error) {
	if len(clients) == 0 {
		return nil, nil
	}

	now := time.Now()
	for i := range clients {
		clients[i].LastSeen = now
	}

	err := db.NewInsert().
		Model(&clients).
		Returning("*").
		Scan(ctx)

	if err != nil {
		return nil, fmt.Errorf("error inserting SOAX clients: %v", err)
	}

	return clients, nil
}

// GetSoaxClientStats returns statistics about SOAX clients
func (db *DB) GetSoaxClientStats(ctx context.Context) (map[string]interface{}, error) {
	stats := make(map[string]interface{})

	// Get clients by type
	var typeStats []struct {
		ClientType string `bun:"client_type"`
		Count      int    `bun:"count"`
	}
	err := db.NewSelect().
		Model((*models.SoaxClient)(nil)).
		Column("client_type").
		ColumnExpr("count(*) as count").
		Group("client_type").
		Scan(ctx, &typeStats)
	if err != nil {
		return nil, err
	}
	stats["clients_by_type"] = typeStats

	// Get clients by country and type
	var countryTypeStats []struct {
		CountryCode string `bun:"country_code"`
		ClientType  string `bun:"client_type"`
		Count       int    `bun:"count"`
	}
	err = db.NewSelect().
		Model((*models.SoaxClient)(nil)).
		Column("country_code", "client_type").
		ColumnExpr("count(*) as count").
		Group("country_code", "client_type").
		Order("country_code", "client_type").
		Scan(ctx, &countryTypeStats)
	if err != nil {
		return nil, err
	}
	stats["clients_by_country_and_type"] = countryTypeStats

	// Get recently active clients (last 24 hours)
	var activeStats struct {
		Total int `bun:"count"`
	}
	err = db.NewSelect().
		Model((*models.SoaxClient)(nil)).
		ColumnExpr("count(*) as count").
		Where("last_seen > ?", time.Now().Add(-24*time.Hour)).
		Scan(ctx, &activeStats)
	if err != nil {
		return nil, err
	}
	stats["active_clients_24h"] = activeStats.Total

	return stats, nil
}

// InsertMeasurement saves a measurement record

// GetActiveClientByIP returns a client by IP if it exists and hasn't expired
func (db *DB) GetActiveClientByIP(ctx context.Context, ip string) (*models.SoaxClient, error) {
	var client models.SoaxClient
	err := db.NewSelect().
		Model(&client).
		Where("ip = ?", ip).
		Scan(ctx)

	if err != nil {
		if err == sql.ErrNoRows {
			return nil, err
		}
		return nil, fmt.Errorf("error querying client: %v", err)
	}

	return &client, nil
}

// InitMeasurementSchema creates the measurements table with foreign keys
func (db *DB) InitMeasurementSchema(ctx context.Context) error {
	_, err := db.NewCreateTable().
		Model((*models.Measurement)(nil)).
		IfNotExists().
		ForeignKey(`("client_id") REFERENCES soax_clients ("id") ON DELETE CASCADE`).
		ForeignKey(`("server_id") REFERENCES servers ("id") ON DELETE CASCADE`).
		Exec(ctx)

	if err != nil {
		return fmt.Errorf("failed to create measurements table: %v", err)
	}

	return nil
}

func (db *DB) InsertMeasurement(ctx context.Context, measurement *models.Measurement) error {
	_, err := db.NewInsert().
		Model(measurement).
		Exec(ctx)

	if err != nil {
		return fmt.Errorf("error inserting measurement: %v", err)
	}

	return nil
}
