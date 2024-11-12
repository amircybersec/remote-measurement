package measurement

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"connectivity-tester/pkg/connectivity"
	"connectivity-tester/pkg/database"
	"connectivity-tester/pkg/models"
	"connectivity-tester/pkg/soax"

	"github.com/google/uuid"
	"github.com/spf13/viper"
)

const maxWorkers = 1 // Adjust based on your system and service capabilities

// MeasurementService struct update to include configuration
type MeasurementService struct {
	db       *database.DB
	logger   *slog.Logger
	config   *viper.Viper
	prefixes []string
}

// measurementJob represents a single measurement task
type measurementJob struct {
	client *models.SoaxClient
	server models.Server
}

// NewMeasurementService constructor
func NewMeasurementService(db *database.DB, logger *slog.Logger, config *viper.Viper) *MeasurementService {
	prefixes := config.GetStringSlice("measurement.prefixes")
	if prefixes == nil {
		logger.Debug("No prefixes configured")
		prefixes = []string{} // Empty slice if no prefixes configured
	}

	return &MeasurementService{
		db:       db,
		logger:   logger,
		config:   config,
		prefixes: prefixes,
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
	// Generate a unique session ID for this measurement series
	sessionID := uuid.New().String()

	// Perform initial measurements for both protocols
	initialResults := make(map[string]bool) // map[protocol]hasError

	// Perform initial TCP and UDP measurements
	if err := s.performMeasurement(client, server, sessionID, 0, "", nil); err != nil {
		return fmt.Errorf("initial measurement failed: %v", err)
	}

	// Retrieve the initial measurements
	measurements, err := s.db.GetMeasurementsBySession(context.Background(), sessionID, 0)
	if err != nil {
		return fmt.Errorf("failed to retrieve initial measurements: %v", err)
	}

	// Check which protocols had errors
	for _, m := range measurements {
		initialResults[m.Protocol] = (m.ErrorMsg != "" || m.ErrorOp != "success")
	}

	var retryCount = 0

	// For each protocol that had errors, perform retries
	for protocol, hasError := range initialResults {
		if hasError {
			s.logger.Debug("Performing retries for failed protocol",
				"sessionID", sessionID,
				"protocol", protocol,
				"clientIP", client.IP,
				"serverIP", server.IP)

			retryCount = retryCount + 1
			// Perform retry measurement for this protocol
			if err := s.performProtocolMeasurement(client, server, sessionID, retryCount, "", nil, protocol); err != nil {
				s.logger.Warn("retry measurement failed",
					"protocol", protocol,
					"error", err)
			}

			// Try with different prefixes for this protocol
			for _, prefix := range s.prefixes {
				retryCount = retryCount + 1
				newAccessLink := server.FullAccessLink + "?prefix=" + prefix
				s.logger.Debug("Testing with prefix",
					"prefix", prefix,
					"newAccessLink", newAccessLink,
				)
				if err := s.performProtocolMeasurement(client, server, sessionID, retryCount, prefix, &newAccessLink, protocol); err != nil {
					s.logger.Warn("prefix measurement failed",
						"protocol", protocol,
						"prefix", prefix,
						"error", err)
				}
			}
		} else {
			s.logger.Debug("Skipping retries for successful protocol",
				"sessionID", sessionID,
				"protocol", protocol,
				"clientIP", client.IP,
				"serverIP", server.IP)
		}
	}

	return nil
}

// performProtocolMeasurement handles a single measurement for a specific protocol
func (s *MeasurementService) performProtocolMeasurement(
	client models.SoaxClient,
	server models.Server,
	sessionID string,
	retryNumber int,
	prefix string,
	accessLinkOverride *string,
	protocol string,
) error {
	// Construct the transport config
	var transport string
	if accessLinkOverride != nil {
		transport = fmt.Sprintf("%s|%s", client.TransportURL(), *accessLinkOverride)
	} else {
		transport = fmt.Sprintf("%s|%s", client.TransportURL(), server.FullAccessLink)
	}

	// Skip test for protocol if there is an error message for it on the server
	if s.shouldSkipProtocol(protocol, server) {
		return nil
	}

	s.logger.Debug("Testing connectivity",
		"sessionID", sessionID,
		"retryNumber", retryNumber,
		"prefix", prefix,
		"protocol", protocol,
		"clientIP", client.IP,
		"serverIP", server.IP)

	measurement := models.Measurement{
		ClientID:    client.ID,
		ServerID:    server.ID,
		Time:        time.Now(),
		Protocol:    protocol,
		SessionID:   sessionID,
		RetryNumber: retryNumber,
		PrefixUsed:  prefix,
	}

	report, err := connectivity.TestConnectivity(
		transport,
		protocol,
		viper.GetString("connectivity.resolver"),
		viper.GetString("connectivity.domain"),
	)

	if err := s.handleTestResult(err, report, &measurement); err != nil {
		return err
	}

	// Save measurement
	if err := s.db.InsertMeasurement(context.Background(), &measurement); err != nil {
		return fmt.Errorf("failed to save measurement: %v", err)
	}

	return nil
}

// Update performMeasurement to use performProtocolMeasurement for both protocols
func (s *MeasurementService) performMeasurement(
	client models.SoaxClient,
	server models.Server,
	sessionID string,
	retryNumber int,
	prefix string,
	accessLinkOverride *string,
) error {
	for _, protocol := range []string{"tcp", "udp"} {
		if err := s.performProtocolMeasurement(client, server, sessionID, retryNumber, prefix, accessLinkOverride, protocol); err != nil {
			return fmt.Errorf("measurement failed for %s: %v", protocol, err)
		}
	}
	return nil
}

// shouldSkipProtocol determines if a protocol test should be skipped
func (s *MeasurementService) shouldSkipProtocol(protocol string, server models.Server) bool {
	if protocol == "tcp" && server.TCPErrorMsg != "" {
		s.logger.Debug("Skipping TCP test",
			"serverIP", server.IP,
			"serverPort", server.Port,
			"error", server.TCPErrorMsg)
		return true
	}
	if protocol == "udp" && server.UDPErrorMsg != "" {
		s.logger.Debug("Skipping UDP test",
			"serverIP", server.IP,
			"serverPort", server.Port,
			"error", server.UDPErrorMsg)
		return true
	}
	return false
}

// handleTestResult processes the test result and updates the measurement
func (s *MeasurementService) handleTestResult(
	err error,
	report connectivity.ConnectivityReport,
	measurement *models.Measurement,
) error {
	if err != nil {
		s.logger.Error("Connectivity Test failed",
			"protocol", measurement.Protocol,
			"error", err,
			"sessionID", measurement.SessionID)
		measurement.ErrorMsg = err.Error()
		measurement.ErrorOp = "fail"
		return nil
	}

	if report.Test.Error != nil {
		s.logger.Debug("Connectivity Test Error",
			"protocol", measurement.Protocol,
			"error", report.Test.Error)
		measurement.ErrorMsg = report.Test.Error.Msg
		measurement.ErrorMsgVerbose = report.Test.Error.MsgVerbose
		measurement.ErrorOp = report.Test.Error.Op
		measurement.Duration = report.Test.DurationMs
	} else {
		s.logger.Debug("Connectivity Test successful",
			"protocol", measurement.Protocol,
			"sessionID", measurement.SessionID)
		measurement.Duration = report.Test.DurationMs
		measurement.ErrorOp = "success"
	}

	// Marshal report into JSON
	reportJson, err := json.Marshal(report)
	if err != nil {
		s.logger.Error("Failed to marshal report", "error", err)
		return nil
	}
	measurement.FullReport = reportJson

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
func (s *MeasurementService) RunMeasurements(ctx context.Context, country string, clientType soax.ClientType, maxRetries int, maxClients int) error {
	// Get ISP list shuffled
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
		// Try to get up to maximum number of clients for the ISP
		for i := 0; i < maxClients; i++ {
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
	}

	return nil
}
