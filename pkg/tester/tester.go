package tester

import (
	"context"
	"fmt"
	"log/slog"
	"sync"

	"connectivity-tester/pkg/connectivity"
	"connectivity-tester/pkg/database"
	"connectivity-tester/pkg/models"

	"github.com/spf13/viper"
)

const maxWorkers = 1 // Adjust this based on your needs and system capabilities

func TestServers(db *database.DB, retestTCP, retestUDP bool) error {
	var servers []models.Server
	var err error

	if retestTCP || retestUDP {
		servers, err = db.GetServersForRetest(context.Background(), retestTCP, retestUDP)
	} else {
		servers, err = db.GetAllServers(context.Background())
	}

	if err != nil {
		return fmt.Errorf("failed to get servers: %v", err)
	}

	jobs := make(chan models.Server, len(servers))
	results := make(chan models.Server, len(servers))

	// Start worker pool
	var wg sync.WaitGroup
	for i := 0; i < maxWorkers; i++ {
		wg.Add(1)
		go worker(db, &wg, jobs, results, retestTCP, retestUDP)
	}

	// Send jobs to workers
	for _, server := range servers {
		jobs <- server
	}
	close(jobs)

	// Wait for all workers to finish
	go func() {
		wg.Wait()
		close(results)
	}()

	// Collect results
	for server := range results {
		slog.Debug("Server tested", "accessLink", server.FullAccessLink)
	}

	return nil
}

func worker(db *database.DB, wg *sync.WaitGroup, jobs <-chan models.Server, results chan<- models.Server, testTCP, testUDP bool) {
	defer wg.Done()
	for server := range jobs {
		err := testServer(db, &server, testTCP, testUDP)
		if err != nil {
			slog.Error("Error testing server", "accessLink", server.FullAccessLink, "error", err)
		}
		results <- server
	}
}

func testServer(db *database.DB, server *models.Server, testTCP, testUDP bool) error {
	var testFailed bool

	if testTCP || (!testTCP && !testUDP) {
		// Test TCP
		tcpReport, err := connectivity.TestConnectivity(server.FullAccessLink, "tcp", viper.GetString("connectivity.resolver"), viper.GetString("connectivity.domain"))
		if err != nil {
			slog.Error("TCP test error", "accessLink", server.FullAccessLink, "error", err)
			testFailed = true
		} else {
			connectivity.UpdateResultFromReport(server, tcpReport, "tcp")
			slog.Debug("TCP test completed", "accessLink", server.FullAccessLink, "error", server.TCPErrorMsg)
		}
	}

	if testUDP || (!testTCP && !testUDP) {
		// Test UDP
		udpReport, err := connectivity.TestConnectivity(server.FullAccessLink, "udp", viper.GetString("connectivity.resolver"), viper.GetString("connectivity.domain"))
		if err != nil {
			slog.Error("UDP test error", "accessLink", server.FullAccessLink, "error", err)
			testFailed = true
		} else {
			connectivity.UpdateResultFromReport(server, udpReport, "udp")
			slog.Debug("UDP test completed", "accessLink", server.FullAccessLink, "error", server.UDPErrorMsg)
		}
	}

	if testFailed {
		// Remove server from database if any test failed
		err := db.RemoveServer(context.Background(), server)
		if err != nil {
			return fmt.Errorf("failed to remove server after test failure: %v", err)
		}
		slog.Info("Server removed due to test failure", "accessLink", server.FullAccessLink)
	} else {
		// Update server in database if tests passed
		err := db.UpdateServerTestResults(context.Background(), server)
		if err != nil {
			return fmt.Errorf("failed to update server test results: %v", err)
		}
	}

	return nil
}
