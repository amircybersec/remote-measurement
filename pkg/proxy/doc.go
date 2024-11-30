/*
Package proxy provides an abstraction layer for managing different proxy service providers
(currently SOAX and ProxyRack) in the connectivity tester application.

The package implements a Provider interface that standardizes interactions with different
proxy services, allowing the application to work with multiple providers in a consistent way.

Key Components:

  - Provider: Interface that defines the contract for proxy providers
  - Config: Configuration structure for proxy providers
  - System: Enum type representing supported proxy systems
  - Factory: Creates provider instances based on configuration

Provider Interface Methods:

	GetISPList: Retrieves available ISPs for a country and client type
	GetClientForISP: Obtains a proxy client for a specific ISP
	BuildTransportURL: Constructs the proxy transport URL for a client
	GetProviderName: Returns the provider's name
	IsValidClient: Verifies if a client is still valid
	GetSessionLength: Returns the session length in seconds

Supported Providers:

 1. SOAX Provider:
    - Supports both residential and mobile proxies
    - Manages proxy sessions with automatic IP rotation
    - Provides ISP targeting capabilities

 2. ProxyRack Provider:
    - Supports country and ISP-based proxy selection
    - Manages session-based proxy connections
    - Provides automatic IP refresh functionality

Usage Example:

	config := proxy.Config{
		System:        proxy.SystemSOAX,
		APIKey:        "your-api-key",
		PackageID:     "your-package-id",
		PackageKey:    "your-package-key",
		SessionLength: 360,
		Endpoint:      "proxy.soax.com",
		MaxWorkers:    5,
	}

	provider, err := proxy.NewProvider(config, logger)
	if err != nil {
		log.Fatal(err)
	}

	// Get ISP list for a country
	isps, err := provider.GetISPList("US", models.ResidentialType)
	if err != nil {
		log.Fatal(err)
	}

	// Get a client for a specific ISP
	client, err := provider.GetClientForISP("Comcast", models.ResidentialType, "US", 3)
	if err != nil {
		log.Fatal(err)
	}

Configuration:

Both providers require specific configuration parameters:

SOAX Configuration:
  - APIKey: SOAX API key
  - PackageID: SOAX package identifier
  - PackageKey: SOAX package authentication key
  - SessionLength: Duration of proxy sessions in seconds
  - Endpoint: SOAX proxy endpoint
  - MaxWorkers: Maximum number of concurrent workers

ProxyRack Configuration:
  - Username: ProxyRack username
  - APIKey: ProxyRack API key
  - SessionLength: Duration of proxy sessions in seconds
  - Endpoint: ProxyRack proxy endpoint
  - MaxWorkers: Maximum number of concurrent workers

Error Handling:

The package implements comprehensive error handling for various scenarios:
  - Configuration validation
  - API communication errors
  - Invalid proxy responses
  - Session management issues
  - Client validation failures

Each provider implementation includes specific error handling for provider-specific
edge cases and error conditions.

Thread Safety:

The package is designed to be thread-safe, allowing concurrent access to provider
instances from multiple goroutines. This is particularly important for applications
that need to manage multiple proxy connections simultaneously.
*/
package proxy
