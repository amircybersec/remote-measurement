package measurement

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"connectivity-tester/pkg/connectivity"
	"connectivity-tester/pkg/database"
	"connectivity-tester/pkg/models"
	"connectivity-tester/pkg/soax"

	"github.com/spf13/viper"
)

type MeasurementService struct {
	db     *database.DB
	logger *slog.Logger
}

func NewMeasurementService(db *database.DB, logger *slog.Logger) *MeasurementService {
	return &MeasurementService{
		db:     db,
		logger: logger,
	}
}

// getWorkingServers returns servers with no errors and allowed ports
func (s *MeasurementService) getWorkingServers(ctx context.Context) ([]models.Server, error) {
	allowedPorts := viper.GetIntSlice("connectivity.allowed_ports")
	if len(allowedPorts) == 0 {
		return nil, fmt.Errorf("no allowed ports configured")
	}

	// Convert ports to strings for comparison
	allowedPortStrs := make([]string, len(allowedPorts))
	for i, port := range allowedPorts {
		allowedPortStrs[i] = fmt.Sprintf("%d", port)
	}

	s.logger.Debug("Getting working servers", "allowedPorts", allowedPortStrs)

	return s.db.GetWorkingServers(ctx, allowedPortStrs)
}

// measureServer performs connectivity tests from a client to a server
func (s *MeasurementService) measureServer(client models.SoaxClient, server models.Server) error {
	// Construct the combined transport config
	transport := fmt.Sprintf("%s|%s", client.TransportURL(), server.FullAccessLink)

	s.logger.Debug("Testing connectivity",
		"clientIP", client.IP,
		"serverIP", server.IP,
		"serverPort", server.Port)

	// Test TCP
	tcpReport, err := connectivity.TestConnectivity(
		transport,
		"tcp",
		viper.GetString("connectivity.resolver"),
		viper.GetString("connectivity.domain"),
	)

	measurement := models.Measurement{
		ClientID: client.ID,
		ServerID: server.ID,
		Time:     time.Now(),
	}

	if err != nil {
		s.logger.Error("TCP test failed",
			"error", err,
			"clientIP", client.IP,
			"serverIP", server.IP)
		measurement.TCPErrorMsg = err.Error()
		measurement.TCPErrorOp = "connect"
	} else if tcpReport.Test.Error != nil {
		measurement.TCPErrorMsg = tcpReport.Test.Error.Msg
		measurement.TCPErrorOp = tcpReport.Test.Error.Op
	}

	// Test UDP
	udpReport, err := connectivity.TestConnectivity(
		transport,
		"udp",
		viper.GetString("connectivity.resolver"),
		viper.GetString("connectivity.domain"),
	)

	if err != nil {
		s.logger.Error("UDP test failed",
			"error", err,
			"clientIP", client.IP,
			"serverIP", server.IP)
		measurement.UDPErrorMsg = err.Error()
		measurement.UDPErrorOp = "connect"
	} else if udpReport.Test.Error != nil {
		measurement.UDPErrorMsg = udpReport.Test.Error.Msg
		measurement.UDPErrorOp = udpReport.Test.Error.Op
	}

	// Save measurement
	err = s.db.InsertMeasurement(context.Background(), &measurement)
	if err != nil {
		return fmt.Errorf("failed to save measurement: %v", err)
	}

	return nil
}

// Update the RunMeasurements method
func (s *MeasurementService) RunMeasurements(ctx context.Context, country string, clientType soax.ClientType, maxRetries int) error {
	// Get ISP list
	isps, err := soax.GetISPList(country, clientType)
	if err != nil {
		return fmt.Errorf("failed to get ISP list: %v", err)
	}

	// Get working servers
	servers, err := s.getWorkingServers(ctx)
	if err != nil {
		return fmt.Errorf("failed to get working servers: %v", err)
	}

	if len(servers) == 0 {
		return fmt.Errorf("no working servers found")
	}

	s.logger.Info("Starting measurements",
		"country", country,
		"clientType", clientType,
		"ispCount", len(isps),
		"serverCount", len(servers))

	// Process each ISP
	for _, isp := range isps {
		client, err := soax.GetClientForISP(isp, clientType, country, maxRetries, s.db)
		if err != nil {
			// ... error handling ...
			continue
		}

		// Save client to database and get the updated client with ID
		savedClients, err := s.db.InsertSoaxClients(ctx, []models.SoaxClient{*client})
		if err != nil {
			s.logger.Error("Failed to save client",
				"error", err,
				"clientIP", client.IP)
			continue
		}

		if len(savedClients) == 0 {
			s.logger.Error("No clients returned after upsert",
				"clientIP", client.IP)
			continue
		}

		savedClient := &savedClients[0]
		s.logger.Debug("Successfully saved client",
			"clientID", savedClient.ID,
			"clientIP", savedClient.IP)

		// Measure connectivity to all servers
		for _, server := range servers {
			err = s.measureServer(*savedClient, server)
			if err != nil {
				s.logger.Error("Measurement failed",
					"error", err,
					"clientID", savedClient.ID,
					"clientIP", savedClient.IP,
					"serverID", server.ID)
			}
		}
	}

	return nil
}
