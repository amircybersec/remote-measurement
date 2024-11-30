package database

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"connectivity-tester/pkg/models"
)

// InitSoaxSchema creates the SOAX clients table if it doesn't exist
func (db *DB) InitClientSchema(ctx context.Context) error {
	// Create the table if it doesn't exist
	_, err := db.NewCreateTable().
		Model((*models.Client)(nil)).
		IfNotExists().
		Exec(ctx)

	if err != nil {
		return fmt.Errorf("failed to create table: %v", err)
	}

	return nil
}

// InsertClients inserts or updates proxy clients in the database
func (db *DB) InsertClients(ctx context.Context, clients []models.Client) ([]models.Client, error) {
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

// GetActiveClientByIP returns a client by IP if it exists and hasn't expired
func (db *DB) GetActiveClientByIP(ctx context.Context, ip string) (*models.Client, error) {
	var client models.Client
	err := db.NewSelect().
		Model(&client).
		Where("ip = ?", ip).
		Where("expiration_time > ?", time.Now()).
		Scan(ctx)

	if err != nil {
		if err == sql.ErrNoRows {
			return nil, err
		}
		return nil, fmt.Errorf("error querying client: %v", err)
	}

	return &client, nil
}

// UpdateClientExpiration updates the expiration time of a client using bun ORM
func (db *DB) UpdateClientExpiration(ctx context.Context, clientID int64, expirationTime time.Time) error {
	_, err := db.NewUpdate().
		Model((*models.Client)(nil)).
		Set("expiration_time = ?", expirationTime).
		Where("id = ?", clientID).
		Exec(ctx)

	if err != nil {
		return fmt.Errorf("failed to update client expiration: %w", err)
	}

	return nil
}
