// File: main.go

package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"connectivity-tester/pkg/config"
	"connectivity-tester/pkg/database"
	"connectivity-tester/pkg/ipinfo"
	"connectivity-tester/pkg/measurement"
	"connectivity-tester/pkg/models"
	"connectivity-tester/pkg/proxy"
	"connectivity-tester/pkg/server"
	"connectivity-tester/pkg/tester"
)

var (
	debugFlag bool
	logger    *slog.Logger
)

var rootCmd = &cobra.Command{
	Use:   "connectivity-tester",
	Short: "A tool for testing server connectivity",
	PersistentPreRun: func(cmd *cobra.Command, args []string) {
		// Set up logging based on the debug flag
		var logLevel slog.Level
		if debugFlag {
			logLevel = slog.LevelDebug
		} else {
			logLevel = slog.LevelInfo
		}

		logger = slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: logLevel}))
		slog.SetDefault(logger)
	},
}

var addServersCmd = &cobra.Command{
	Use:   "add-servers [file] [name]",
	Short: "Add servers from a file to the database and set a common name for all of them",
	Args:  cobra.RangeArgs(1, 2), // Allow 1-2 arguments
	Run: func(cmd *cobra.Command, args []string) {
		db, err := initDB()
		if err != nil {
			logger.Error("Error initializing database", "error", err)
			os.Exit(1)
		}
		defer db.Close()

		// Default name to empty string if not provided
		name := ""
		if len(args) > 1 {
			name = args[1]
		}

		preresolve, _ := cmd.Flags().GetBool("preresolve")

		err = server.AddServersFromFile(db, args[0], name, preresolve)
		if err != nil {
			logger.Error("Error adding servers", "error", err)
			os.Exit(1)
		}
		logger.Info("Servers added successfully")
	},
}

var testServersCmd = &cobra.Command{
	Use:   "test-servers",
	Short: "Test servers in the database",
	Run: func(cmd *cobra.Command, args []string) {
		db, err := initDB()
		if err != nil {
			logger.Error("Error initializing database", "error", err)
			os.Exit(1)
		}
		defer db.Close()

		retestTCP, _ := cmd.Flags().GetBool("tcp")
		retestUDP, _ := cmd.Flags().GetBool("udp")

		err = tester.TestServers(db, retestTCP, retestUDP)
		if err != nil {
			logger.Error("Error testing servers", "error", err)
			os.Exit(1)
		}
		logger.Info("Servers tested successfully")
	},
}

var measureCmd = &cobra.Command{
	Use:   "measure",
	Short: "Measure connectivity from clients to servers",
	Long: `Measure connectivity from proxy clients to working servers.
Examples:
  # Test with specific ISP and server:
  measure --proxy proxyrack --country us --isp Verizon --network residential --clients 5 --server-id 512
  # Test with random ISPs:
  measure --proxy soax --country ir --network mobile --clients 10
  # Test with specific ISP and server group:
  measure --proxy soax --country ir --isp MNT%20Irancell --network mobile --clients 5 --server-name shadowmere

  Flags:
  --proxy: Optional. Proxy service (soax or proxyrack); Defaul is proxyrack
  --country: Required. Country code (e.g., us, uk, ir)
  --isp: Optional. ISP name. If not provided, tests will be pick random ISPs from target country and network type
  --network: Optional. Network type (residential or mobile). Default is residential
  --clients: Required. Maximum number of clients to test with
  --server-id: Optional. Specific server ID to test. Only server id or server name can be provided at a time.
  --server-name: Optional. Specific server group name to test. Only server id or server name can be provided at a time.

  Please note either server ID or server group name can be provided`,

	Run: func(cmd *cobra.Command, args []string) {
		// Get flags
		proxyName, _ := cmd.Flags().GetString("proxy")
		country, _ := cmd.Flags().GetString("country")
		isp, _ := cmd.Flags().GetString("isp")
		network, _ := cmd.Flags().GetString("network")
		clients, _ := cmd.Flags().GetInt("clients")
		serverID, _ := cmd.Flags().GetInt64Slice("server-id")
		serverName, _ := cmd.Flags().GetStringSlice("server-name")

		// Validate required flags
		if proxyName == "" || country == "" || network == "" || clients == 0 {
			logger.Error("Required flags missing",
				"proxy", proxyName,
				"country", country,
				"network", network,
				"clients", clients)
			os.Exit(1)
		}

		// make sure only server ID or server name is provided
		if len(serverID) > 0 && len(serverName) > 0 {
			logger.Error("Only one of server ID or server name can be provided")
			os.Exit(1)
		}

		// Validate network type
		var clientType models.ClientType
		switch network {
		case "residential":
			clientType = models.ResidentialType
		case "mobile":
			if proxyName == "proxyrack" {
				logger.Error("ProxyRack does not support mobile clients")
				os.Exit(1)
			}
			clientType = models.MobileType
		default:
			logger.Error("Invalid network type. Must be 'residential' or 'mobile'")
			os.Exit(1)
		}

		// Create provider config based on proxy type
		var providerConfig proxy.Config
		switch proxyName {
		case "soax":
			providerConfig = proxy.Config{
				System:        proxy.SystemSOAX,
				APIKey:        viper.GetString("soax.api_key"),
				SessionLength: viper.GetInt("soax.session_length"),
				Endpoint:      viper.GetString("soax.endpoint"),
				MaxWorkers:    viper.GetInt("soax.max_workers"),
			}
			if network == "residential" {
				providerConfig.PackageID = viper.GetString("soax.residential_package_id")
				providerConfig.PackageKey = viper.GetString("soax.residential_package_key")
			} else {
				providerConfig.PackageID = viper.GetString("soax.mobile_package_id")
				providerConfig.PackageKey = viper.GetString("soax.mobile_package_key")
			}
		case "proxyrack":
			providerConfig = proxy.Config{
				System:        proxy.SystemProxyRack,
				Username:      viper.GetString("proxyrack.username"),
				APIKey:        viper.GetString("proxyrack.api_key"),
				SessionLength: viper.GetInt("proxyrack.session_length"),
				Endpoint:      viper.GetString("proxyrack.endpoint"),
				MaxWorkers:    viper.GetInt("proxyrack.max_workers"),
			}
		case "none":
			providerConfig = proxy.Config{
				System:        proxy.SystemNone,
				SessionLength: 86400,
				MaxWorkers:    100,
			}
		default:
			logger.Error("Invalid proxy name. Must be 'soax' or 'proxyrack'")
			os.Exit(1)
		}

		// Get max retries from config
		maxRetries := viper.GetInt(fmt.Sprintf("%s.max_retries", proxyName))
		if maxRetries == 0 {
			maxRetries = 3 // Default if not specified
		}

		settings := measurement.Settings{
			MaxClients:  clients,
			MaxRetries:  maxRetries,
			ServerIDs:   serverID,
			ServerNames: serverName,
			Country:     country,
			ISP:         isp,
			ClientType:  clientType,
		}

		// Initialize database
		db, err := initDB()
		if err != nil {
			logger.Error("Error initializing database", "error", err)
			os.Exit(1)
		}
		defer db.Close()

		// Initialize schemas
		logger.Debug("Initializing database schemas")
		err = db.InitClientSchema(context.Background())
		if err != nil {
			logger.Error("Error initializing client schema", "error", err)
			os.Exit(1)
		}

		err = db.InitMeasurementSchema(context.Background())
		if err != nil {
			logger.Error("Error initializing measurement schema", "error", err)
			os.Exit(1)
		}

		// Create provider
		provider, err := proxy.NewProvider(providerConfig, logger)
		if err != nil {
			logger.Error("Failed to create proxy provider", "error", err)
			os.Exit(1)
		}

		measurementService := measurement.NewMeasurementService(db, logger, viper.GetViper(), provider)

		// maxClients, maxRetries, Server ID, Server Group name, ISP name, country code, client type

		// Use existing measurement logic for all other cases
		err = measurementService.RunMeasurements(context.Background(), provider, settings)
		if err != nil {
			logger.Error("Error running measurements", "error", err)
			os.Exit(1)
		}

		logger.Info("Measurements completed successfully")
	},
}

var updateClientsCmd = &cobra.Command{
	Use:   "update-clients",
	Short: "Update missing information for clients in the database",
	Long: `Update missing information for clients in the database using ipinfo.io API.
Examples:
  # Update missing city information
  update-clients --city
  # Update missing country information
  update-clients --country
  # Update all missing information
  update-clients --all`,

	Run: func(cmd *cobra.Command, args []string) {
		updateCity, _ := cmd.Flags().GetBool("city")
		updateCountry, _ := cmd.Flags().GetBool("country")
		updateAll, _ := cmd.Flags().GetBool("all")

		if !updateCity && !updateCountry && !updateAll {
			logger.Error("At least one update flag must be specified (--city, --country, or --all)")
			os.Exit(1)
		}

		db, err := initDB()
		if err != nil {
			logger.Error("Error initializing database", "error", err)
			os.Exit(1)
		}
		defer db.Close()

		// Get all clients from database
		clients, err := db.GetClientsWithMissingInfo(context.Background())
		if err != nil {
			logger.Error("Error getting clients", "error", err)
			os.Exit(1)
		}

		logger.Info("Found clients with missing information", "count", len(clients))

		// Update each client
		for _, client := range clients {
			if needsUpdate(&client, updateCity, updateCountry, updateAll) {
				ipInfo, err := ipinfo.GetIPInfo(client.IP)
				if err != nil {
					logger.Error("Failed to get IP info",
						"clientID", client.ID,
						"ip", client.IP,
						"error", err)
					continue
				}

				updated := false
				if (updateCity || updateAll) && client.City == "" {
					client.City = ipInfo.City
					updated = true
				}
				if (updateCountry || updateAll) && client.CountryCode == "" {
					client.CountryCode = ipInfo.Country
					updated = true
				}

				if updated {
					if err := db.UpdateClientInfo(context.Background(), &client); err != nil {
						logger.Error("Failed to update client",
							"clientID", client.ID,
							"error", err)
						continue
					}
					logger.Info("Updated client information",
						"clientID", client.ID,
						"ip", client.IP,
						"city", client.City,
						"country", client.CountryCode)
				}
			}
		}

		logger.Info("Client update completed")
	},
}

var jsonToURLCmd = &cobra.Command{
	Use:   "json-to-url [file]",
	Short: "Convert SS JSON config file to URL format",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		// Read the JSON file
		jsonData, err := os.ReadFile(args[0])
		if err != nil {
			logger.Error("Error reading file", "error", err)
			os.Exit(1)
		}

		// Convert JSON to URL
		url, err := config.ParseSSConfig(string(jsonData))
		if err != nil {
			logger.Error("Error parsing JSON config", "error", err)
			os.Exit(1)
		}

		// Print URL to stdout
		fmt.Println(url)
	},
}

func init() {
	cobra.OnInitialize(initConfig)

	rootCmd.PersistentFlags().BoolVarP(&debugFlag, "debug", "d", false, "Enable debug logging")
	testServersCmd.Flags().Bool("tcp", false, "Retest servers with TCP errors (excluding 'connect' errors)")
	testServersCmd.Flags().Bool("udp", false, "Retest servers with UDP errors")

	rootCmd.AddCommand(addServersCmd)
	rootCmd.AddCommand(testServersCmd)
	rootCmd.AddCommand(measureCmd)
	rootCmd.AddCommand(updateClientsCmd)
	rootCmd.AddCommand(jsonToURLCmd)

	// Add new flags to measureCmd
	measureCmd.Flags().String("proxy", "none", "Proxy service (soax, proxyrack, or none)")
	measureCmd.Flags().String("country", "us", "Country code (e.g., us, uk)")
	measureCmd.Flags().String("isp", "", "ISP name (optional)")
	measureCmd.Flags().String("network", "residential", "Network type (residential or mobile)")
	measureCmd.Flags().Int("clients", 1, "Maximum number of clients to test with")
	measureCmd.Flags().Int64Slice("server-id", []int64{}, "Specific server ID to test (optional)")
	measureCmd.Flags().StringSlice("server-name", []string{}, "Specific server group names to test (optional)")

	// Remove the Args requirement since we're using flags
	measureCmd.Args = cobra.NoArgs

	// Add update-clients command
	updateClientsCmd.Flags().Bool("city", false, "Update missing city information")
	updateClientsCmd.Flags().Bool("country", false, "Update missing country information")
	updateClientsCmd.Flags().Bool("all", false, "Update all missing information")

	// Add preresolve flag to addServersCmd
	addServersCmd.Flags().Bool("preresolve", true, "Pre-resolve domain names to IP addresses (default: true)")
}

func initConfig() {
	viper.SetConfigName("config")
	viper.SetConfigType("yaml")
	viper.AddConfigPath("../")
	viper.AddConfigPath("$HOME/.connectivity-tester")
	viper.AddConfigPath("/etc/connectivity-tester/")

	if err := viper.ReadInConfig(); err != nil {
		fmt.Printf("Error reading config file: %v\n", err)
		os.Exit(1)
	}
}

func initDB() (*database.DB, error) {
	db, err := database.NewDB()
	if err != nil {
		return nil, fmt.Errorf("error connecting to database: %v", err)
	}

	err = db.InitSchema(context.Background())
	if err != nil {
		db.Close()
		return nil, fmt.Errorf("error initializing database schema: %v", err)
	}

	return db, nil
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

func needsUpdate(client *models.Client, updateCity, updateCountry, updateAll bool) bool {
	if updateAll {
		return client.City == "" || client.CountryCode == ""
	}
	return (updateCity && client.City == "") || (updateCountry && client.CountryCode == "")
}
