/*
Package models defines the core data structures used throughout the connectivity-tester
application. It provides the foundational types that represent clients, servers,
measurements, and their relationships.

Core Types:

ClientType represents the type of proxy client:

	type ClientType string
	const (
		ResidentialType ClientType = "residential"
		MobileType     ClientType = "mobile"
	)

Client represents a proxy client with its properties:

	type Client struct {
		ID             int64     // Unique identifier
		IP             string    // IP address of the client
		ClientType     string    // Type of client (residential/mobile)
		SessionID      int       // Proxy session identifier
		SessionLength  int       // Session duration in seconds
		Time           time.Time // Time when client was created
		ExpirationTime time.Time // When the client session expires
		IPVersion      string    // IP version (v4/v6)
		Carrier        string    // Mobile carrier if applicable
		City           string    // Geographic city location
		CountryCode    string    // ISO country code
		CountryName    string    // Full country name
		ASNumber       string    // Autonomous System number
		ASOrg          string    // AS organization name
		LastSeen       time.Time // Last activity timestamp
		ISP            string    // Internet Service Provider
		Proxy          string    // Proxy provider name
		ProxyURL       string    // Full proxy URL for connections
	}

Server represents a target server for testing:

	type Server struct {
		ID            int64     // Unique identifier
		IP            string    // Server IP address
		Port          string    // Server port
		Name          string    // Server name/identifier
		UserInfo      string    // Additional server info
		LastTestTime  time.Time // Last test timestamp
		TCPErrorMsg   string    // Last TCP error message
		TCPErrorOp    string    // Last TCP error operation
		UDPErrorMsg   string    // Last UDP error message
		UDPErrorOp    string    // Last UDP error operation
		IPType        string    // IP version
		ASNumber      string    // AS number
		ASOrg         string    // AS organization
		City          string    // Server city location
		Region        string    // Server region
		Country       string    // Server country
		CreatedAt     time.Time // Creation timestamp
		UpdatedAt     time.Time // Last update timestamp
		FullAccessLink string   // Complete server access URL
	}

Measurement represents a connectivity test result:

	type Measurement struct {
		ID              int64     // Unique identifier
		ClientID        int64     // Reference to client
		ServerID        int64     // Reference to server
		Time            time.Time // Measurement timestamp
		Protocol        string    // Test protocol (TCP/UDP)
		Duration        float64   // Test duration in milliseconds
		ErrorMsg        string    // Error message if any
		ErrorMsgVerbose string    // Detailed error information
		ErrorOp         string    // Error operation type
		SessionID       string    // Test session identifier
		RetryNumber     int       // Retry attempt number
		PrefixUsed      string    // Network prefix used
		FullReport      []byte    // Complete test report
	}

SoaxIPInfo represents IP information from SOAX API:

	type SoaxIPInfo struct {
		Status string
		Data   struct {
			IP          string
			Port        string
			City        string
			CountryCode string
			CountryName string
			Carrier     string
		}
	}

Database Integration:

The models package includes database tags and relationships:
  - Clients table with unique constraints on IP
  - Servers table with composite unique constraints
  - Measurements table with foreign key relationships
  - Timestamps for tracking record lifecycle

Usage Example:

	// Create a new client
	client := &models.Client{
		IP:         "192.168.1.1",
		ClientType: string(models.ResidentialType),
		SessionID:  12345,
		Country:    "US",
		ISP:        "Comcast",
	}

	// Create a new server
	server := &models.Server{
		IP:            "10.0.0.1",
		Port:          "443",
		Name:          "test-server",
		FullAccessLink: "https://test-server:443",
	}

	// Create a measurement
	measurement := &models.Measurement{
		ClientID:    client.ID,
		ServerID:    server.ID,
		Protocol:    "tcp",
		Duration:    150.5,
		SessionID:   "test-session",
		RetryNumber: 0,
	}

Relationships:

The models maintain several key relationships:
  - One Client can have many Measurements
  - One Server can have many Measurements
  - Each Measurement belongs to one Client and one Server

Thread Safety:

The model structures themselves are not thread-safe. Synchronization should be
handled at the database layer when performing concurrent operations on these
models.

Validation:

While the models package defines the structures, validation is typically handled
at the database or service layer. Users of these models should ensure proper
validation before persistence.
*/
package models
