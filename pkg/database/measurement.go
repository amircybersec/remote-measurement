package database

import (
	"context"
	"fmt"

	"connectivity-tester/pkg/models"
)

// InitMeasurementSchema creates the measurements table with foreign keys
func (db *DB) InitMeasurementSchema(ctx context.Context) error {
	_, err := db.NewCreateTable().
		Model((*models.Measurement)(nil)).
		IfNotExists().
		ForeignKey(`("client_id") REFERENCES clients ("id") ON DELETE CASCADE`).
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

// Add this helper method to the MeasurementService
func (db *DB) GetMeasurementsBySession(ctx context.Context, sessionID string, retryNumber int) ([]models.Measurement, error) {
	var measurements []models.Measurement
	err := db.NewSelect().
		Model(&measurements).
		Where("session_id = ?", sessionID).
		Where("retry_number = ?", retryNumber).
		Scan(ctx)

	if err != nil {
		return nil, fmt.Errorf("error retrieving measurements: %v", err)
	}

	return measurements, nil
}
