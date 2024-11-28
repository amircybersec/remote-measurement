package database

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"sync"

	"connectivity-tester/pkg/models"

	"github.com/uptrace/bun"
)

var (
	updateMutex sync.Mutex
	removeMutex sync.Mutex
)

func (db *DB) UpsertServer(ctx context.Context, server *models.Server) error {
	_, err := db.NewInsert().
		Model(server).
		On("CONFLICT (ip, full_access_link) DO UPDATE").
		Set("udp_error_msg = EXCLUDED.udp_error_msg").
		Set("udp_error_op = EXCLUDED.udp_error_op").
		Set("tcp_error_msg = EXCLUDED.tcp_error_msg").
		Set("tcp_error_op = EXCLUDED.tcp_error_op").
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
	query := db.NewSelect().
		Model(&servers).
		Where("((tcp_error_msg IS NULL OR tcp_error_msg = '') OR (udp_error_msg IS NULL OR udp_error_msg = ''))")

	// Only add port restriction if allowedPorts is not nil
	if allowedPorts != nil {
		query = query.Where("port IN (?)", bun.In(allowedPorts))
	}
	// add a mechasnism to get all servers except ones on rejected port list

	err := query.Scan(ctx)
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

func (db *DB) GetServersByIDs(ctx context.Context, ids []int64) ([]models.Server, error) {
	var servers []models.Server

	// If no IDs provided, return empty slice
	if len(ids) == 0 {
		return servers, nil
	}

	err := db.NewSelect().
		Model(&servers).
		Where("id IN (?)", bun.In(ids)).
		Scan(ctx)

	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("no servers found for IDs %v", ids)
		}
		return nil, fmt.Errorf("error getting servers by IDs %v: %w", ids, err)
	}

	// Check if we found all requested servers
	if len(servers) != len(ids) {
		// Create a map of found IDs for easy lookup
		foundIDs := make(map[int64]bool)
		for _, server := range servers {
			foundIDs[server.ID] = true
		}

		// Find missing IDs
		var missingIDs []int64
		for _, id := range ids {
			if !foundIDs[id] {
				missingIDs = append(missingIDs, id)
			}
		}

		logger := slog.Default()
		logger.Warn("Some requested servers were not found",
			"requestedIDs", ids,
			"missingIDs", missingIDs)
	}

	// Log the results
	logger := slog.Default()
	logger.Debug("Retrieved servers by IDs",
		"requestedIDs", ids,
		"foundCount", len(servers))

	for _, server := range servers {
		logger.Debug("Server details",
			"id", server.ID,
			"ip", server.IP,
			"port", server.Port,
			"name", server.Name)
	}

	return servers, nil
}

func (db *DB) GetServersByNames(ctx context.Context, names []string) ([]models.Server, error) {
	var servers []models.Server

	// If no names provided, return empty slice
	if len(names) == 0 {
		return servers, nil
	}

	err := db.NewSelect().
		Model(&servers).
		Where("name IN (?)", bun.In(names)).
		Scan(ctx)

	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("no servers found for names %v", names)
		}
		return nil, fmt.Errorf("error getting servers by names %v: %w", names, err)
	}

	// Check if we found all requested servers
	if len(servers) != len(names) {
		// Create a map of found names for easy lookup
		foundNames := make(map[string]bool)
		for _, server := range servers {
			foundNames[server.Name] = true
		}

		// Find missing names
		var missingNames []string
		for _, name := range names {
			if !foundNames[name] {
				missingNames = append(missingNames, name)
			}
		}

		logger := slog.Default()
		logger.Warn("Some requested servers were not found",
			"requestedNames", names,
			"missingNames", missingNames)
	}

	// Log the results
	logger := slog.Default()
	logger.Debug("Retrieved servers by names",
		"requestedNames", names,
		"foundCount", len(servers))

	for _, server := range servers {
		logger.Debug("Server details",
			"id", server.ID,
			"ip", server.IP,
			"port", server.Port,
			"name", server.Name)
	}

	return servers, nil
}
