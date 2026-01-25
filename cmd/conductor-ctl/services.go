package main

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

// serviceCmd is the parent command for service operations
var serviceCmd = &cobra.Command{
	Use:     "service",
	Aliases: []string{"services", "svc"},
	Short:   "Manage services",
	Long:    `Commands for viewing and managing services in the test registry.`,
}

// serviceListCmd lists all services
var serviceListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all services",
	Long: `List all registered services.

Filters:
  --owner   Filter by owner
  --zone    Filter by network zone
  --query   Search by name or owner`,
	Example: `  # List all services
  conductor-ctl service list

  # List services owned by a team
  conductor-ctl service list --owner platform-team

  # Search for services
  conductor-ctl service list --query auth`,
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		owner, _ := cmd.Flags().GetString("owner")
		zone, _ := cmd.Flags().GetString("zone")
		query, _ := cmd.Flags().GetString("query")
		limit, _ := cmd.Flags().GetInt("limit")

		ShowSpinner("Fetching services...")
		resp, err := apiClient.ListServices(ctx, owner, zone, query, limit)
		HideSpinner()

		if err != nil {
			return fmt.Errorf("failed to list services: %w", err)
		}

		if outputFormat == "json" {
			return printJSON(resp)
		}

		if len(resp.Services) == 0 {
			fmt.Println(Dim("No services found."))
			return nil
		}

		headers := []string{"ID", "NAME", "OWNER", "BRANCH", "TESTS", "LAST SYNC", "ACTIVE"}
		rows := make([][]string, len(resp.Services))
		for i, s := range resp.Services {
			active := Green("yes")
			if !s.Active {
				active = Red("no")
			}

			lastSync := "-"
			if s.LastSyncedAt != "" {
				lastSync = formatTimestamp(s.LastSyncedAt)
			}

			rows[i] = []string{
				truncate(s.ID, 12),
				s.Name,
				s.Owner,
				s.DefaultBranch,
				fmt.Sprintf("%d", s.TestCount),
				lastSync,
				active,
			}
		}

		printTable(headers, rows)

		if resp.Pagination != nil && resp.Pagination.HasMore {
			fmt.Printf("\n%s\n", Dim("More results available. Use --limit to see more."))
		}

		return nil
	},
}

// serviceGetCmd gets details for a specific service
var serviceGetCmd = &cobra.Command{
	Use:   "get <service-id>",
	Short: "Get service details",
	Long: `Display detailed information about a specific service.

Shows service configuration, test definitions, and recent runs.`,
	Example: `  # Get service details
  conductor-ctl service get my-service

  # Include test definitions
  conductor-ctl service get my-service --tests

  # Include recent runs
  conductor-ctl service get my-service --runs`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		serviceID := args[0]
		includeTests, _ := cmd.Flags().GetBool("tests")
		includeRuns, _ := cmd.Flags().GetBool("runs")

		ShowSpinner("Fetching service details...")
		service, tests, runs, err := apiClient.GetService(ctx, serviceID, includeTests, includeRuns)
		HideSpinner()

		if err != nil {
			return fmt.Errorf("failed to get service: %w", err)
		}

		if outputFormat == "json" {
			return printJSON(map[string]interface{}{
				"service":     service,
				"tests":       tests,
				"recent_runs": runs,
			})
		}

		// Print service details
		fmt.Printf("%s\n", Bold("Service Details"))
		fmt.Printf("  ID:             %s\n", service.ID)
		fmt.Printf("  Name:           %s\n", service.Name)
		fmt.Printf("  Git URL:        %s\n", service.GitURL)
		fmt.Printf("  Default Branch: %s\n", service.DefaultBranch)
		fmt.Printf("  Owner:          %s\n", service.Owner)
		fmt.Printf("  Active:         %s\n", formatBool(service.Active))
		fmt.Printf("  Test Count:     %d\n", service.TestCount)
		fmt.Printf("  Created:        %s\n", formatTimestamp(service.CreatedAt))
		fmt.Printf("  Updated:        %s\n", formatTimestamp(service.UpdatedAt))
		if service.LastSyncedAt != "" {
			fmt.Printf("  Last Sync:      %s\n", formatTimestamp(service.LastSyncedAt))
		}

		if service.Contact != nil {
			fmt.Printf("\n%s\n", Bold("Contact"))
			if service.Contact.Name != "" {
				fmt.Printf("  Name:  %s\n", service.Contact.Name)
			}
			if service.Contact.Email != "" {
				fmt.Printf("  Email: %s\n", service.Contact.Email)
			}
			if service.Contact.Slack != "" {
				fmt.Printf("  Slack: %s\n", service.Contact.Slack)
			}
		}

		fmt.Printf("\n%s\n", Bold("Configuration"))
		fmt.Printf("  Execution Type:    %s\n", formatExecutionType(service.DefaultExecutionType))
		if service.DefaultContainerImg != "" {
			fmt.Printf("  Container Image:   %s\n", service.DefaultContainerImg)
		}
		if service.ConfigPath != "" {
			fmt.Printf("  Config Path:       %s\n", service.ConfigPath)
		}
		if len(service.NetworkZones) > 0 {
			fmt.Printf("  Network Zones:     %s\n", strings.Join(service.NetworkZones, ", "))
		}
		if service.DefaultTimeout != nil {
			fmt.Printf("  Default Timeout:   %s\n", formatDuration(service.DefaultTimeout))
		}

		if len(service.Labels) > 0 {
			fmt.Printf("\n%s\n", Bold("Labels"))
			for k, v := range service.Labels {
				fmt.Printf("  %s: %s\n", k, v)
			}
		}

		if includeTests && len(tests) > 0 {
			fmt.Printf("\n%s\n", Bold("Test Definitions"))
			headers := []string{"ID", "NAME", "TYPE", "TAGS", "ENABLED"}
			rows := make([][]string, len(tests))
			for i, t := range tests {
				enabled := Green("yes")
				if !t.Enabled {
					enabled = Red("no")
				}
				tags := strings.Join(t.Tags, ", ")
				if tags == "" {
					tags = "-"
				}
				rows[i] = []string{
					truncate(t.ID, 12),
					truncate(t.Name, 30),
					formatTestType(t.Type),
					truncate(tags, 20),
					enabled,
				}
			}
			printTable(headers, rows)
		}

		if includeRuns && len(runs) > 0 {
			fmt.Printf("\n%s\n", Bold("Recent Runs"))
			headers := []string{"ID", "STATUS", "BRANCH", "TESTS", "DURATION", "CREATED"}
			rows := make([][]string, len(runs))
			for i, r := range runs {
				tests := fmt.Sprintf("%d/%d", r.Passed, r.Total)
				duration := "-"
				if r.Duration != nil {
					duration = formatDuration(r.Duration)
				}
				rows[i] = []string{
					truncate(r.ID, 12),
					formatRunStatus(r.Status),
					truncate(r.Branch, 20),
					tests,
					duration,
					formatTimestamp(r.CreatedAt),
				}
			}
			printTable(headers, rows)
		}

		return nil
	},
}

// serviceSyncCmd syncs test definitions from git
var serviceSyncCmd = &cobra.Command{
	Use:   "sync <service-id>",
	Short: "Sync service tests from git",
	Long: `Trigger discovery of test definitions from the service's git repository.

This will read the Conductor manifest file and update test definitions.`,
	Example: `  # Sync tests from default branch
  conductor-ctl service sync my-service

  # Sync from a specific branch
  conductor-ctl service sync my-service --branch feature/new-tests

  # Delete tests not found in repo
  conductor-ctl service sync my-service --delete-missing`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()

		serviceID := args[0]
		branch, _ := cmd.Flags().GetString("branch")
		deleteMissing, _ := cmd.Flags().GetBool("delete-missing")

		ShowSpinner("Syncing service...")
		result, err := apiClient.SyncService(ctx, serviceID, branch, deleteMissing)
		HideSpinner()

		if err != nil {
			return fmt.Errorf("failed to sync service: %w", err)
		}

		if outputFormat == "json" {
			return printJSON(result)
		}

		fmt.Printf("%s Sync completed\n", Green("✓"))
		fmt.Printf("  Tests Added:   %d\n", result.TestsAdded)
		fmt.Printf("  Tests Updated: %d\n", result.TestsUpdated)
		fmt.Printf("  Tests Removed: %d\n", result.TestsRemoved)

		if len(result.Errors) > 0 {
			fmt.Printf("\n%s\n", Yellow("Warnings:"))
			for _, e := range result.Errors {
				fmt.Printf("  - %s\n", e)
			}
		}

		return nil
	},
}

// serviceCreateCmd creates a new service
var serviceCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "Create a new service",
	Long: `Register a new service in the test registry.

Creates a service with the specified git repository URL.`,
	Example: `  # Create a service
  conductor-ctl service create --name my-service --git-url https://github.com/org/repo

  # Create with all options
  conductor-ctl service create \
    --name my-service \
    --git-url https://github.com/org/repo \
    --branch main \
    --owner platform-team \
    --config-path .conductor.yaml`,
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		name, _ := cmd.Flags().GetString("name")
		gitURL, _ := cmd.Flags().GetString("git-url")
		branch, _ := cmd.Flags().GetString("branch")
		owner, _ := cmd.Flags().GetString("owner")
		configPath, _ := cmd.Flags().GetString("config-path")
		zonesStr, _ := cmd.Flags().GetString("zones")

		if name == "" {
			return fmt.Errorf("--name is required")
		}
		if gitURL == "" {
			return fmt.Errorf("--git-url is required")
		}

		req := &CreateServiceRequest{
			Name:          name,
			GitURL:        gitURL,
			DefaultBranch: branch,
			Owner:         owner,
			ConfigPath:    configPath,
		}

		if zonesStr != "" {
			req.NetworkZones = strings.Split(zonesStr, ",")
		}

		ShowSpinner("Creating service...")
		service, err := apiClient.CreateService(ctx, req)
		HideSpinner()

		if err != nil {
			return fmt.Errorf("failed to create service: %w", err)
		}

		if outputFormat == "json" {
			return printJSON(service)
		}

		fmt.Printf("%s Service created\n", Green("✓"))
		fmt.Printf("  ID:     %s\n", Bold(service.ID))
		fmt.Printf("  Name:   %s\n", service.Name)
		fmt.Printf("  Git:    %s\n", service.GitURL)
		fmt.Printf("  Branch: %s\n", service.DefaultBranch)

		return nil
	},
}

func init() {
	// List command flags
	serviceListCmd.Flags().String("owner", "", "Filter by owner")
	serviceListCmd.Flags().String("zone", "", "Filter by network zone")
	serviceListCmd.Flags().String("query", "", "Search query")
	serviceListCmd.Flags().Int("limit", 50, "Maximum number of results")

	// Get command flags
	serviceGetCmd.Flags().Bool("tests", false, "Include test definitions")
	serviceGetCmd.Flags().Bool("runs", false, "Include recent runs")

	// Sync command flags
	serviceSyncCmd.Flags().String("branch", "", "Branch to sync from")
	serviceSyncCmd.Flags().Bool("delete-missing", false, "Delete tests not found in repo")

	// Create command flags
	serviceCreateCmd.Flags().String("name", "", "Service name (required)")
	serviceCreateCmd.Flags().String("git-url", "", "Git repository URL (required)")
	serviceCreateCmd.Flags().String("branch", "main", "Default branch")
	serviceCreateCmd.Flags().String("owner", "", "Service owner")
	serviceCreateCmd.Flags().String("config-path", ".conductor.yaml", "Path to Conductor config file")
	serviceCreateCmd.Flags().String("zones", "", "Comma-separated network zones")

	// Add subcommands
	serviceCmd.AddCommand(serviceListCmd)
	serviceCmd.AddCommand(serviceGetCmd)
	serviceCmd.AddCommand(serviceSyncCmd)
	serviceCmd.AddCommand(serviceCreateCmd)
}

// formatTestType returns a human-readable test type
func formatTestType(testType string) string {
	switch strings.ToLower(testType) {
	case "test_type_unit", "unit":
		return "unit"
	case "test_type_integration", "integration":
		return "integration"
	case "test_type_e2e", "e2e":
		return "e2e"
	case "test_type_performance", "performance":
		return "performance"
	case "test_type_security", "security":
		return "security"
	case "test_type_smoke", "smoke":
		return "smoke"
	default:
		return testType
	}
}
