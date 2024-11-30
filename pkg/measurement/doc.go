/*
Package measurement provides functionality for conducting and managing network connectivity
measurements using various proxy providers. It orchestrates the process of testing
connectivity between proxy clients and target servers.

Key Components:

  - MeasurementService: Core service that manages measurement operations
  - Settings: Configuration structure for measurement parameters
  - measurementJob: Internal structure representing a single measurement task

MeasurementService Methods:

	RunMeasurements: Executes measurements for configured clients and servers
	Shutdown: Gracefully stops all measurement operations
	processMeasurements: Handles parallel processing of measurements
	measureServer: Performs connectivity tests from a client to a server

Settings Configuration:

	type Settings struct {
		Country     string              // Target country for measurements
		ISP         string              // Specific ISP to test (optional)
		ClientType  models.ClientType   // Type of client (residential/mobile)
		ServerIDs   []int64            // Specific server IDs to test (optional)
		ServerNames []string           // Specific server names to test (optional)
		MaxRetries  int                // Maximum retry attempts
		MaxClients  int                // Maximum number of concurrent clients
	}

Usage Example:

	// Initialize the measurement service
	measurementSvc := measurement.NewMeasurementService(
		db,
		logger,
		config,
		proxyProvider,
	)

	// Configure measurement settings
	settings := measurement.Settings{
		Country:    "US",
		ClientType: models.ResidentialType,
		MaxRetries: 3,
		MaxClients: 5,
	}

	// Run measurements
	err := measurementSvc.RunMeasurements(
		context.Background(),
		proxyProvider,
		settings,
	)
	if err != nil {
		log.Fatal(err)
	}

	// Cleanup when done
	defer measurementSvc.Shutdown()

Measurement Process:

1. Client Acquisition:
  - Obtains proxy clients from the configured provider
  - Validates client connectivity and characteristics
  - Stores client information in the database

2. Server Selection:
  - Retrieves target servers based on configuration
  - Filters servers based on allowed ports and working status
  - Supports both specific server selection and automatic discovery

3. Connectivity Testing:
  - Performs TCP and UDP connectivity tests
  - Handles automatic retries for failed connections
  - Supports custom prefix testing for enhanced connectivity

4. Result Management:
  - Records detailed measurement results in the database
  - Captures timing, errors, and full connectivity reports
  - Maintains historical measurement data

Monitoring and Management:

The service includes built-in monitoring capabilities:
  - Active client monitoring
  - Session expiration handling
  - Concurrent measurement management
  - Resource cleanup

Error Handling:

Comprehensive error handling for:
  - Client acquisition failures
  - Connectivity test failures
  - Database operations
  - Resource management
  - Protocol-specific errors

Thread Safety:

The package is designed for concurrent operation:
  - Safe for parallel measurements
  - Synchronized client monitoring
  - Thread-safe database operations
  - Controlled resource sharing

Performance Considerations:

The service implements several optimizations:
  - Worker pool for parallel measurements
  - Configurable concurrency limits
  - Efficient resource utilization
  - Smart retry mechanisms

Dependencies:

  - database: For storing measurement results and client information
  - proxy: For managing proxy providers and clients
  - models: For data structures and types
  - connectivity: For network testing functionality
*/
package measurement
