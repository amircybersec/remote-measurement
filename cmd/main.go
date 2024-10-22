package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"connectivity-tester/pkg/database"
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

func init() {
	cobra.OnInitialize(initConfig)

	rootCmd.PersistentFlags().BoolVarP(&debugFlag, "debug", "d", false, "Enable debug logging")
	testServersCmd.Flags().Bool("tcp", false, "Retest servers with TCP errors (excluding 'connect' errors)")
	testServersCmd.Flags().Bool("udp", false, "Retest servers with UDP errors")

	rootCmd.AddCommand(addServersCmd)
	rootCmd.AddCommand(testServersCmd)
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