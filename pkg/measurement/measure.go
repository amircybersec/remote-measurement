package measurement

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"connectivity-tester/pkg/connectivity"
	"connectivity-tester/pkg/database"
	"connectivity-tester/pkg/models"
	"connectivity-tester/pkg/soax"

	"github.com/spf13/viper"
)

const maxWorkers = 100 // Adjust based on your system capabilities

type MeasurementService struct {
	db     *database.DB
	logger *slog.Logger
}

// measurementJob represents a single measurement task
type measurementJob struct {
	client *models.SoaxClient
	server models.Server
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

// worker processes measurement jobs from the jobs channel
func (s *MeasurementService) worker(wg *sync.WaitGroup, jobs <-chan measurementJob, results chan<- error) {
	defer wg.Done()
	for job := range jobs {
		err := s.measureServer(*job.client, job.server)
		results <- err
	}
}

// processMeasurements handles parallel processing of measurements for a client
func (s *MeasurementService) processMeasurements(client *models.SoaxClient, servers []models.Server) {
	jobs := make(chan measurementJob, len(servers))
	results := make(chan error, len(servers))

	// Start worker pool
	var wg sync.WaitGroup
	for i := 0; i < maxWorkers; i++ {
		wg.Add(1)
		go s.worker(&wg, jobs, results)
	}

	// Send jobs to workers
	for _, server := range servers {
		jobs <- measurementJob{
			client: client,
			server: server,
		}
	}
	close(jobs)

	// Wait for all workers to finish
	go func() {
		wg.Wait()
		close(results)
	}()

	// Process results
	var errorCount int
	for err := range results {
		if err != nil {
			errorCount++
			s.logger.Error("Measurement failed",
				"error", err,
				"clientID", client.ID,
				"clientIP", client.IP,
				"errorCount", errorCount)
		}
	}
}

// RunMeasurements performs measurements for all clients
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
			s.logger.Error("Failed to get client for ISP",
				"isp", isp,
				"error", err)
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

		// Process measurements in parallel
		s.processMeasurements(savedClient, servers)
	}

	return nil
}
