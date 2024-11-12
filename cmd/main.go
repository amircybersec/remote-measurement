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
	"connectivity-tester/pkg/server"
	"connectivity-tester/pkg/soax"
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
	Use:   "measure [country] [type] [max-retries] [max-clients]",
	Short: "Measure connectivity from clients to servers",
	Long: `Measure connectivity from SOAX clients to working servers.
[country] is the two-letter country code
[type] must be either 'residential' or 'mobile'
[max-retries] is the maximum number of attempts to get a new IP from an ISP
[max-clients] is the maximum number of clients to try to get from each ISP`,
	Example: "measure ir mobile 10",
	Args:    cobra.ExactArgs(4),
	Run: func(cmd *cobra.Command, args []string) {
		country := args[0]
		clientType := args[1]
		maxRetries, err := strconv.Atoi(args[2])
		if err != nil {
			logger.Error("Invalid max-retries value", "error", err)
			os.Exit(1)
		}
		maxClients, err := strconv.Atoi(args[3])
		if err != nil {
			logger.Error("Invalid max-clients value", "error", err)
			os.Exit(1)
		}

		// Validate client type
		var cType soax.ClientType
		switch clientType {
		case "residential":
			cType = soax.Residential
		case "mobile":
			cType = soax.Mobile
		default:
			logger.Error("Invalid client type. Must be 'residential' or 'mobile'")
			os.Exit(1)
		}

		logger.Debug("Initializing measurement process",
			"country", country,
			"clientType", clientType,
			"maxRetries", maxRetries)

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
			logger.Error("Error initializing SOAX schema", "error", err)
			os.Exit(1)
		}

		err = db.InitMeasurementSchema(context.Background())
		if err != nil {
			logger.Error("Error initializing measurement schema", "error", err)
			os.Exit(1)
		}

		measurementService := measurement.NewMeasurementService(db, logger, viper.GetViper())
		err = measurementService.RunMeasurements(context.Background(), country, cType, maxRetries, maxClients)
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
