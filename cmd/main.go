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
	Use:   "add-servers [file]",
	Short: "Add servers from a file to the database",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		db, err := initDB()
		if err != nil {
			logger.Error("Error initializing database", "error", err)
			os.Exit(1)
		}
		defer db.Close()

		err = server.AddServersFromFile(db, args[0])
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

var getClientsCmd = &cobra.Command{
	Use:   "get-clients [country] [type] [count]",
	Short: "Get SOAX clients from a specific country",
	Args:  cobra.ExactArgs(3),
	Run: func(cmd *cobra.Command, args []string) {
		country := args[0]
		clientType := args[1]
		count, err := strconv.Atoi(args[2])
		if err != nil {
			logger.Error("Invalid count value", "error", err)
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

		db, err := initDB()
		if err != nil {
			logger.Error("Error initializing database", "error", err)
			os.Exit(1)
		}
		defer db.Close()

		// Initialize SOAX schema
		err = db.InitSoaxSchema(context.Background())
		if err != nil {
			logger.Error("Error initializing SOAX schema", "error", err)
			os.Exit(1)
		}

		clients, stats, err := soax.GetUniqueClients(cType, country, count)

		// Always try to insert clients if we have any, regardless of error
		if len(clients) > 0 {
			insertErr := db.InsertSoaxClients(context.Background(), clients)
			if insertErr != nil {
				logger.Error("Error inserting SOAX clients",
					"error", insertErr,
					"clientsFound", len(clients))
				os.Exit(1)
			}

			logger.Info("Saved SOAX clients to database",
				"savedCount", len(clients),
				"requestedCount", count)
		}

		// After saving, report any error from the original operation
		if err != nil {
			logger.Error("Could not get requested number of unique clients",
				"error", err,
				"uniqueFound", stats.UniqueClients,
				"totalAttempts", stats.TotalAttempts,
				"duplicates", stats.DuplicateIPs,
				"requestedCount", count,
				"savedCount", len(clients))
			os.Exit(1)
		}

		logger.Info("Operation completed successfully",
			"uniqueClients", stats.UniqueClients,
			"totalAttempts", stats.TotalAttempts,
			"duplicateIPs", stats.DuplicateIPs,
			"successRate", float64(stats.UniqueClients)/float64(stats.TotalAttempts))
	},
}

func init() {
	cobra.OnInitialize(initConfig)

	rootCmd.PersistentFlags().BoolVarP(&debugFlag, "debug", "d", false, "Enable debug logging")
	testServersCmd.Flags().Bool("tcp", false, "Retest servers with TCP errors (excluding 'connect' errors)")
	testServersCmd.Flags().Bool("udp", false, "Retest servers with UDP errors")

	rootCmd.AddCommand(addServersCmd)
	rootCmd.AddCommand(testServersCmd)
	rootCmd.AddCommand(getClientsCmd)
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
