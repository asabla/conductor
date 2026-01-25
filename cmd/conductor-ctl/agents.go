package main

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

// agentCmd is the parent command for agent operations
var agentCmd = &cobra.Command{
	Use:     "agent",
	Aliases: []string{"agents"},
	Short:   "Manage Conductor agents",
	Long:    `Commands for viewing and managing Conductor test execution agents.`,
}

// agentListCmd lists all agents
var agentListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all agents",
	Long: `List all registered agents with their current status.

Filters:
  --status    Filter by agent status (idle, busy, draining, offline)
  --zone      Filter by network zone
  --limit     Maximum number of results`,
	Example: `  # List all agents
  conductor-ctl agent list

  # List only busy agents
  conductor-ctl agent list --status busy

  # List agents in production zone
  conductor-ctl agent list --zone production`,
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		status, _ := cmd.Flags().GetString("status")
		zone, _ := cmd.Flags().GetString("zone")
		limit, _ := cmd.Flags().GetInt("limit")

		ShowSpinner("Fetching agents...")
		resp, err := apiClient.ListAgents(ctx, status, zone, limit)
		HideSpinner()

		if err != nil {
			return fmt.Errorf("failed to list agents: %w", err)
		}

		if outputFormat == "json" {
			return printJSON(resp)
		}

		if len(resp.Agents) == 0 {
			fmt.Println(Dim("No agents found."))
			return nil
		}

		// Build table
		headers := []string{"ID", "NAME", "STATUS", "VERSION", "ZONES", "RUNS", "LAST HEARTBEAT"}
		rows := make([][]string, len(resp.Agents))
		for i, a := range resp.Agents {
			status := formatAgentStatus(a.Status)
			zones := strings.Join(a.NetworkZones, ", ")
			if zones == "" {
				zones = Dim("-")
			}
			runs := fmt.Sprintf("%d/%d", a.ActiveRunCnt, a.MaxParallel)
			heartbeat := formatTimestamp(a.LastHeartbeat)

			rows[i] = []string{
				truncate(a.ID, 12),
				a.Name,
				status,
				a.Version,
				zones,
				runs,
				heartbeat,
			}
		}

		printTable(headers, rows)

		if resp.Pagination != nil && resp.Pagination.HasMore {
			fmt.Printf("\n%s\n", Dim("More results available. Use --limit to see more."))
		}

		return nil
	},
}

// agentGetCmd gets details for a specific agent
var agentGetCmd = &cobra.Command{
	Use:   "get <agent-id>",
	Short: "Get agent details",
	Long: `Display detailed information about a specific agent.

Shows agent configuration, capabilities, resource usage, and current runs.`,
	Example: `  # Get agent details
  conductor-ctl agent get agent-123

  # Get agent details with current runs
  conductor-ctl agent get agent-123 --runs`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		agentID := args[0]
		includeRuns, _ := cmd.Flags().GetBool("runs")

		ShowSpinner("Fetching agent details...")
		agent, runs, err := apiClient.GetAgent(ctx, agentID, includeRuns)
		HideSpinner()

		if err != nil {
			return fmt.Errorf("failed to get agent: %w", err)
		}

		if outputFormat == "json" {
			return printJSON(map[string]interface{}{
				"agent":        agent,
				"current_runs": runs,
			})
		}

		// Print agent details
		fmt.Printf("%s\n", Bold("Agent Details"))
		fmt.Printf("  ID:             %s\n", agent.ID)
		fmt.Printf("  Name:           %s\n", agent.Name)
		fmt.Printf("  Status:         %s\n", formatAgentStatus(agent.Status))
		fmt.Printf("  Version:        %s\n", agent.Version)
		fmt.Printf("  Hostname:       %s\n", agent.Hostname)
		fmt.Printf("  IP Address:     %s\n", agent.IPAddress)
		fmt.Printf("  OS/Arch:        %s/%s\n", agent.OS, agent.Arch)
		fmt.Printf("  Network Zones:  %s\n", strings.Join(agent.NetworkZones, ", "))
		fmt.Printf("  Max Parallel:   %d\n", agent.MaxParallel)
		fmt.Printf("  Active Runs:    %d\n", agent.ActiveRunCnt)
		fmt.Printf("  Registered:     %s\n", formatTimestamp(agent.RegisteredAt))
		fmt.Printf("  Last Heartbeat: %s\n", formatTimestamp(agent.LastHeartbeat))

		if agent.Capabilities != nil {
			fmt.Printf("\n%s\n", Bold("Capabilities"))
			fmt.Printf("  Docker:    %s\n", formatBool(agent.Capabilities.DockerAvailable))
			fmt.Printf("  CPU Cores: %d\n", agent.Capabilities.CPUCores)
			fmt.Printf("  Memory:    %s\n", formatBytes(agent.Capabilities.MemoryBytes))
			fmt.Printf("  Disk:      %s\n", formatBytes(agent.Capabilities.DiskBytes))
			if len(agent.Capabilities.Runtimes) > 0 {
				fmt.Printf("  Runtimes:  %s\n", strings.Join(agent.Capabilities.Runtimes, ", "))
			}
		}

		if agent.ResourceUsage != nil {
			fmt.Printf("\n%s\n", Bold("Resource Usage"))
			fmt.Printf("  CPU:    %.1f%%\n", agent.ResourceUsage.CPUPercent)
			fmt.Printf("  Memory: %s / %s\n",
				formatBytes(agent.ResourceUsage.MemoryBytes),
				formatBytes(agent.ResourceUsage.MemoryTotalBytes))
			fmt.Printf("  Disk:   %s / %s\n",
				formatBytes(agent.ResourceUsage.DiskBytes),
				formatBytes(agent.ResourceUsage.DiskTotalBytes))
		}

		if len(agent.Labels) > 0 {
			fmt.Printf("\n%s\n", Bold("Labels"))
			for k, v := range agent.Labels {
				fmt.Printf("  %s: %s\n", k, v)
			}
		}

		if includeRuns && len(runs) > 0 {
			fmt.Printf("\n%s\n", Bold("Current Runs"))
			headers := []string{"RUN ID", "SERVICE", "STARTED", "PROGRESS"}
			rows := make([][]string, len(runs))
			for i, r := range runs {
				rows[i] = []string{
					truncate(r.RunID, 12),
					r.ServiceName,
					formatTimestamp(r.StartedAt),
					fmt.Sprintf("%d%%", r.ProgressPercent),
				}
			}
			printTable(headers, rows)
		}

		return nil
	},
}

// agentDrainCmd drains an agent
var agentDrainCmd = &cobra.Command{
	Use:   "drain <agent-id>",
	Short: "Drain an agent",
	Long: `Put an agent into draining mode.

A draining agent will complete its current runs but will not accept new work.
This is useful for graceful maintenance or shutdown.`,
	Example: `  # Drain an agent
  conductor-ctl agent drain agent-123

  # Drain and cancel active runs
  conductor-ctl agent drain agent-123 --cancel-active

  # Drain with a reason
  conductor-ctl agent drain agent-123 --reason "scheduled maintenance"`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		agentID := args[0]
		reason, _ := cmd.Flags().GetString("reason")
		cancelActive, _ := cmd.Flags().GetBool("cancel-active")

		ShowSpinner("Draining agent...")
		agent, cancelled, err := apiClient.DrainAgent(ctx, agentID, reason, cancelActive)
		HideSpinner()

		if err != nil {
			return fmt.Errorf("failed to drain agent: %w", err)
		}

		if outputFormat == "json" {
			return printJSON(map[string]interface{}{
				"agent":          agent,
				"cancelled_runs": cancelled,
			})
		}

		fmt.Printf("%s Agent %s is now draining\n", Green("✓"), Bold(agent.Name))
		if cancelled > 0 {
			fmt.Printf("  Cancelled %d active run(s)\n", cancelled)
		}
		fmt.Printf("  Status: %s\n", formatAgentStatus(agent.Status))

		return nil
	},
}

// agentUndrainCmd undrains an agent
var agentUndrainCmd = &cobra.Command{
	Use:   "undrain <agent-id>",
	Short: "Undrain an agent",
	Long: `Remove an agent from draining mode.

The agent will resume accepting new work.`,
	Example: `  # Undrain an agent
  conductor-ctl agent undrain agent-123`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		agentID := args[0]

		ShowSpinner("Undraining agent...")
		agent, err := apiClient.UndrainAgent(ctx, agentID)
		HideSpinner()

		if err != nil {
			return fmt.Errorf("failed to undrain agent: %w", err)
		}

		if outputFormat == "json" {
			return printJSON(agent)
		}

		fmt.Printf("%s Agent %s is now active\n", Green("✓"), Bold(agent.Name))
		fmt.Printf("  Status: %s\n", formatAgentStatus(agent.Status))

		return nil
	},
}

func init() {
	// List command flags
	agentListCmd.Flags().String("status", "", "Filter by status (idle, busy, draining, offline)")
	agentListCmd.Flags().String("zone", "", "Filter by network zone")
	agentListCmd.Flags().Int("limit", 50, "Maximum number of results")

	// Get command flags
	agentGetCmd.Flags().Bool("runs", false, "Include current runs")

	// Drain command flags
	agentDrainCmd.Flags().String("reason", "", "Reason for draining")
	agentDrainCmd.Flags().Bool("cancel-active", false, "Cancel active runs")

	// Add subcommands
	agentCmd.AddCommand(agentListCmd)
	agentCmd.AddCommand(agentGetCmd)
	agentCmd.AddCommand(agentDrainCmd)
	agentCmd.AddCommand(agentUndrainCmd)
}

// formatAgentStatus returns a colored status string
func formatAgentStatus(status string) string {
	switch strings.ToLower(status) {
	case "agent_status_idle", "idle":
		return Green("idle")
	case "agent_status_busy", "busy":
		return Yellow("busy")
	case "agent_status_draining", "draining":
		return Yellow("draining")
	case "agent_status_offline", "offline":
		return Red("offline")
	default:
		return Dim(status)
	}
}
