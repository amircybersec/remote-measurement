// File: main.go

package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strconv"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"connectivity-tester/pkg/database"
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

		err = server.AddServersFromFile(db, args[0], name)
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
	Use:   "measure [country] [type] [proxy] [max-retries] [max-clients]",
	Short: "Measure connectivity from clients to servers",
	Long: `Measure connectivity from proxy clients to working servers.
[country] is the two-letter country code
[type] must be either 'residential' or 'mobile'
[proxy] must be either 'soax' or 'proxyrack'
[max-retries] is the maximum number of attempts to get a new IP from an ISP
[max-clients] is the maximum number of clients to try to get from each ISP`,
	Example: "measure ir mobile soax 5 10",
	Args:    cobra.ExactArgs(5),
	Run: func(cmd *cobra.Command, args []string) {
		country := args[0]
		clientType := args[1]
		proxyName := args[2]
		maxRetries, err := strconv.Atoi(args[3])
		if err != nil {
			logger.Error("Invalid max-retries value", "error", err)
			os.Exit(1)
		}
		maxClients, err := strconv.Atoi(args[4])
		if err != nil {
			logger.Error("Invalid max-clients value", "error", err)
			os.Exit(1)
		}

		// Validate proxy name and create provider config
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
			if clientType == "residential" {
				providerConfig.PackageID = viper.GetString("soax.residential_package_id")
				providerConfig.PackageKey = viper.GetString("soax.residential_package_key")
			}
			if clientType == "mobile" {
				providerConfig.PackageID = viper.GetString("soax.mobile_package_id")
				providerConfig.PackageKey = viper.GetString("soax.mobile_package_key")
			}
		case "proxyrack":
			if clientType == "mobile" {
				logger.Error("ProxyRack does not support mobile clients")
				os.Exit(1)
			}
			providerConfig = proxy.Config{
				System:        proxy.SystemProxyRack,
				Username:      viper.GetString("proxyrack.username"),
				APIKey:        viper.GetString("proxyrack.api_key"),
				SessionLength: viper.GetInt("proxyrack.session_length"),
				Endpoint:      viper.GetString("proxyrack.endpoint"),
				MaxWorkers:    viper.GetInt("proxyrack.max_workers"),
			}
		default:
			logger.Error("Invalid proxy name. Must be 'soax' or 'proxyrack'")
			os.Exit(1)
		}

		// Validate and set client type
		var cType models.ClientType
		switch clientType {
		case "residential":
			cType = models.ResidentialType
		case "mobile":
			if proxyName == "proxyrack" {
				logger.Error("ProxyRack does not support mobile clients")
				os.Exit(1)
			}
			cType = models.MobileType
		default:
			logger.Error("Invalid client type. Must be 'residential' or 'mobile'")
			os.Exit(1)
		}

		// Create provider
		provider, err := proxy.NewProvider(providerConfig, logger)
		if err != nil {
			logger.Error("Failed to create proxy provider", "error", err)
			os.Exit(1)
		}

		logger.Debug("Initializing measurement process",
			"country", country,
			"clientType", clientType,
			"proxy", proxyName,
			"maxRetries", maxRetries,
			"maxClients", maxClients)

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

		measurementService := measurement.NewMeasurementService(db, logger, viper.GetViper())
		err = measurementService.RunMeasurements(context.Background(), provider, country, cType, maxRetries, maxClients)
		if err != nil {
			logger.Error("Error running measurements", "error", err)
			os.Exit(1)
		}

		logger.Info("Measurements completed successfully")
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
