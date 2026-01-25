package main

import (
	"fmt"
	"os"
	"runtime"

	"github.com/spf13/cobra"
)

// Build information (set from main.go)
var (
	Version   = "dev"
	Commit    = "unknown"
	BuildTime = "unknown"
)

// Global flags
var (
	serverAddr   string
	authToken    string
	outputFormat string
	noColor      bool
	configFile   string
)

// Global client instance
var apiClient *Client

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:   "conductor-ctl",
	Short: "CLI tool for managing the Conductor test orchestration platform",
	Long: `conductor-ctl is a command-line interface for administering the Conductor
distributed test orchestration platform.

It provides commands for managing:
  - Agents: View status, drain/undrain nodes
  - Test runs: Trigger, monitor, cancel, and retry test executions
  - Services: Register and manage services in the test registry
  - Configuration: Manage CLI settings

Environment variables:
  CONDUCTOR_SERVER   Server address (default: localhost:8080)
  CONDUCTOR_TOKEN    Authentication token
  CONDUCTOR_OUTPUT   Output format: json, table (default: table)
  CONDUCTOR_CONFIG   Config file path (default: ~/.conductor/config.yaml)`,
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		// Skip client initialization for completion and config commands
		if cmd.Name() == "completion" || cmd.Name() == "version" ||
			(cmd.Parent() != nil && cmd.Parent().Name() == "completion") ||
			(cmd.Parent() != nil && cmd.Parent().Name() == "config") {
			return nil
		}

		// Initialize color output
		InitColor(!noColor)

		// Load configuration
		cfg, err := LoadConfig(configFile)
		if err != nil {
			// Config file not found is OK, we'll use defaults/flags
			cfg = &Config{}
		}

		// Resolve server address (flag > env > config > default)
		server := serverAddr
		if server == "" {
			server = os.Getenv("CONDUCTOR_SERVER")
		}
		if server == "" && cfg.Server != "" {
			server = cfg.Server
		}
		if server == "" {
			server = "localhost:8080"
		}

		// Resolve auth token (flag > env > config)
		token := authToken
		if token == "" {
			token = os.Getenv("CONDUCTOR_TOKEN")
		}
		if token == "" && cfg.Token != "" {
			token = cfg.Token
		}

		// Resolve output format (flag > env > config > default)
		output := outputFormat
		if output == "" {
			output = os.Getenv("CONDUCTOR_OUTPUT")
		}
		if output == "" && cfg.OutputFormat != "" {
			output = cfg.OutputFormat
		}
		if output == "" {
			output = "table"
		}
		outputFormat = output

		// Initialize API client
		apiClient = NewClient(server, token)

		return nil
	},
	SilenceUsage:  true,
	SilenceErrors: true,
}

// versionCmd represents the version command
var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print version information",
	Long:  `Display the version, commit hash, and build time of conductor-ctl.`,
	Run: func(cmd *cobra.Command, args []string) {
		InitColor(!noColor)

		if outputFormat == "json" {
			formatter := &JSONFormatter{}
			info := map[string]string{
				"version":    Version,
				"commit":     Commit,
				"build_time": BuildTime,
				"go_version": runtime.Version(),
				"platform":   runtime.GOOS + "/" + runtime.GOARCH,
			}
			output, _ := formatter.Format(info)
			fmt.Println(output)
			return
		}

		fmt.Printf("%s\n", Bold("conductor-ctl"))
		fmt.Printf("  Version:    %s\n", Version)
		fmt.Printf("  Commit:     %s\n", Commit)
		fmt.Printf("  Built:      %s\n", BuildTime)
		fmt.Printf("  Go version: %s\n", runtime.Version())
		fmt.Printf("  Platform:   %s/%s\n", runtime.GOOS, runtime.GOARCH)
	},
}

// Execute adds all child commands to the root command and sets flags appropriately.
func Execute() error {
	return rootCmd.Execute()
}

func init() {
	// Persistent flags available to all commands
	rootCmd.PersistentFlags().StringVarP(&serverAddr, "server", "s", "", "Conductor server address (default: localhost:8080)")
	rootCmd.PersistentFlags().StringVarP(&authToken, "token", "t", "", "Authentication token")
	rootCmd.PersistentFlags().StringVarP(&outputFormat, "output", "o", "", "Output format: json, table (default: table)")
	rootCmd.PersistentFlags().BoolVar(&noColor, "no-color", false, "Disable colored output")
	rootCmd.PersistentFlags().StringVar(&configFile, "config", "", "Config file (default: ~/.conductor/config.yaml)")

	// Add subcommands
	rootCmd.AddCommand(versionCmd)
	rootCmd.AddCommand(agentCmd)
	rootCmd.AddCommand(runCmd)
	rootCmd.AddCommand(serviceCmd)
	rootCmd.AddCommand(configCmd)
	rootCmd.AddCommand(completionCmd)
}
