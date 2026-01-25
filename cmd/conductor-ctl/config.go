package main

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

// Config represents the CLI configuration
type Config struct {
	Server       string `yaml:"server"`
	Token        string `yaml:"token"`
	OutputFormat string `yaml:"output_format"`
}

// DefaultConfigPath returns the default config file path
func DefaultConfigPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".conductor", "config.yaml")
}

// LoadConfig loads configuration from file
func LoadConfig(path string) (*Config, error) {
	if path == "" {
		path = DefaultConfigPath()
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("invalid config file: %w", err)
	}

	return &cfg, nil
}

// SaveConfig saves configuration to file
func SaveConfig(cfg *Config, path string) error {
	if path == "" {
		path = DefaultConfigPath()
	}

	// Ensure directory exists
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	data, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	if err := os.WriteFile(path, data, 0600); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	return nil
}

// configCmd is the parent command for config operations
var configCmd = &cobra.Command{
	Use:   "config",
	Short: "Manage CLI configuration",
	Long:  `Commands for viewing and managing conductor-ctl configuration.`,
}

// configShowCmd shows the current configuration
var configShowCmd = &cobra.Command{
	Use:   "show",
	Short: "Show current configuration",
	Long: `Display the current CLI configuration.

Shows settings from the config file and environment variables.`,
	Example: `  # Show current config
  conductor-ctl config show`,
	RunE: func(cmd *cobra.Command, args []string) error {
		InitColor(!noColor)

		path := configFile
		if path == "" {
			path = DefaultConfigPath()
		}

		cfg, err := LoadConfig(path)
		if err != nil {
			cfg = &Config{}
		}

		if outputFormat == "json" {
			return printJSON(map[string]interface{}{
				"file":          path,
				"server":        resolveConfigValue(cfg.Server, serverAddr, os.Getenv("CONDUCTOR_SERVER"), "localhost:8080"),
				"token_set":     cfg.Token != "" || authToken != "" || os.Getenv("CONDUCTOR_TOKEN") != "",
				"output_format": resolveConfigValue(cfg.OutputFormat, outputFormat, os.Getenv("CONDUCTOR_OUTPUT"), "table"),
			})
		}

		fmt.Printf("%s\n", Bold("Configuration"))
		fmt.Printf("  Config file: %s\n", path)
		fmt.Println()

		fmt.Printf("%s\n", Bold("Settings"))

		// Server
		server := resolveConfigValue(cfg.Server, serverAddr, os.Getenv("CONDUCTOR_SERVER"), "localhost:8080")
		serverSource := resolveSource(cfg.Server, serverAddr, os.Getenv("CONDUCTOR_SERVER"))
		fmt.Printf("  Server:        %s %s\n", server, Dim("("+serverSource+")"))

		// Token
		tokenSet := cfg.Token != "" || authToken != "" || os.Getenv("CONDUCTOR_TOKEN") != ""
		tokenSource := resolveSource(cfg.Token, authToken, os.Getenv("CONDUCTOR_TOKEN"))
		if tokenSet {
			fmt.Printf("  Token:         %s %s\n", Dim("****"), Dim("("+tokenSource+")"))
		} else {
			fmt.Printf("  Token:         %s\n", Dim("not set"))
		}

		// Output format
		output := resolveConfigValue(cfg.OutputFormat, outputFormat, os.Getenv("CONDUCTOR_OUTPUT"), "table")
		outputSource := resolveSource(cfg.OutputFormat, outputFormat, os.Getenv("CONDUCTOR_OUTPUT"))
		fmt.Printf("  Output Format: %s %s\n", output, Dim("("+outputSource+")"))

		return nil
	},
}

// configSetCmd sets a configuration value
var configSetCmd = &cobra.Command{
	Use:   "set <key> <value>",
	Short: "Set a configuration value",
	Long: `Set a configuration value in the config file.

Available keys:
  server        - Conductor server address
  token         - Authentication token
  output_format - Default output format (json, table)`,
	Example: `  # Set server address
  conductor-ctl config set server localhost:8080

  # Set authentication token
  conductor-ctl config set token my-secret-token

  # Set default output format
  conductor-ctl config set output_format json`,
	Args: cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		InitColor(!noColor)

		key := args[0]
		value := args[1]

		path := configFile
		if path == "" {
			path = DefaultConfigPath()
		}

		cfg, err := LoadConfig(path)
		if err != nil {
			cfg = &Config{}
		}

		switch strings.ToLower(key) {
		case "server":
			cfg.Server = value
		case "token":
			cfg.Token = value
		case "output_format", "output":
			if value != "json" && value != "table" {
				return fmt.Errorf("invalid output format: %s (must be 'json' or 'table')", value)
			}
			cfg.OutputFormat = value
		default:
			return fmt.Errorf("unknown configuration key: %s", key)
		}

		if err := SaveConfig(cfg, path); err != nil {
			return fmt.Errorf("failed to save config: %w", err)
		}

		fmt.Printf("%s Set %s = %s\n", Green("✓"), Bold(key), value)

		return nil
	},
}

// configInitCmd initializes configuration interactively
var configInitCmd = &cobra.Command{
	Use:   "init",
	Short: "Initialize configuration interactively",
	Long: `Initialize conductor-ctl configuration with an interactive setup wizard.

This will guide you through setting up the basic configuration options.`,
	Example: `  # Run interactive setup
  conductor-ctl config init`,
	RunE: func(cmd *cobra.Command, args []string) error {
		InitColor(!noColor)

		path := configFile
		if path == "" {
			path = DefaultConfigPath()
		}

		fmt.Printf("%s\n\n", Bold("Conductor CLI Configuration"))

		reader := bufio.NewReader(os.Stdin)
		cfg := &Config{}

		// Server
		fmt.Print("Server address [localhost:8080]: ")
		server, _ := reader.ReadString('\n')
		server = strings.TrimSpace(server)
		if server == "" {
			server = "localhost:8080"
		}
		cfg.Server = server

		// Token
		fmt.Print("Authentication token (leave empty to skip): ")
		token, _ := reader.ReadString('\n')
		token = strings.TrimSpace(token)
		cfg.Token = token

		// Output format
		fmt.Print("Default output format (table/json) [table]: ")
		output, _ := reader.ReadString('\n')
		output = strings.TrimSpace(output)
		if output == "" {
			output = "table"
		}
		if output != "json" && output != "table" {
			fmt.Printf("%s Invalid output format, using 'table'\n", Yellow("!"))
			output = "table"
		}
		cfg.OutputFormat = output

		// Save
		if err := SaveConfig(cfg, path); err != nil {
			return fmt.Errorf("failed to save config: %w", err)
		}

		fmt.Printf("\n%s Configuration saved to %s\n", Green("✓"), path)

		return nil
	},
}

// configPathCmd shows the config file path
var configPathCmd = &cobra.Command{
	Use:   "path",
	Short: "Show config file path",
	Long:  `Display the path to the configuration file.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		path := configFile
		if path == "" {
			path = DefaultConfigPath()
		}
		fmt.Println(path)
		return nil
	},
}

func init() {
	configCmd.AddCommand(configShowCmd)
	configCmd.AddCommand(configSetCmd)
	configCmd.AddCommand(configInitCmd)
	configCmd.AddCommand(configPathCmd)
}

// resolveConfigValue returns the first non-empty value from the given options
func resolveConfigValue(configValue, flagValue, envValue, defaultValue string) string {
	if flagValue != "" {
		return flagValue
	}
	if envValue != "" {
		return envValue
	}
	if configValue != "" {
		return configValue
	}
	return defaultValue
}

// resolveSource returns the source of the configuration value
func resolveSource(configValue, flagValue, envValue string) string {
	if flagValue != "" {
		return "flag"
	}
	if envValue != "" {
		return "env"
	}
	if configValue != "" {
		return "config"
	}
	return "default"
}
