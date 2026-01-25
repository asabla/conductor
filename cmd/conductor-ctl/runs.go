package main

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

// runCmd is the parent command for run operations
var runCmd = &cobra.Command{
	Use:     "run",
	Aliases: []string{"runs"},
	Short:   "Manage test runs",
	Long:    `Commands for viewing, triggering, and managing test runs.`,
}

// runListCmd lists test runs
var runListCmd = &cobra.Command{
	Use:   "list",
	Short: "List test runs",
	Long: `List test runs with optional filtering.

Filters:
  --service   Filter by service ID or name
  --status    Filter by run status (pending, running, passed, failed, error, timeout, cancelled)
  --limit     Maximum number of results`,
	Example: `  # List recent runs
  conductor-ctl run list

  # List runs for a specific service
  conductor-ctl run list --service my-service

  # List only failed runs
  conductor-ctl run list --status failed

  # List runs as JSON
  conductor-ctl run list -o json`,
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		service, _ := cmd.Flags().GetString("service")
		status, _ := cmd.Flags().GetString("status")
		limit, _ := cmd.Flags().GetInt("limit")

		ShowSpinner("Fetching runs...")
		resp, err := apiClient.ListRuns(ctx, service, status, limit)
		HideSpinner()

		if err != nil {
			return fmt.Errorf("failed to list runs: %w", err)
		}

		if outputFormat == "json" {
			return printJSON(resp)
		}

		if len(resp.Runs) == 0 {
			fmt.Println(Dim("No runs found."))
			return nil
		}

		headers := []string{"ID", "SERVICE", "STATUS", "BRANCH", "TESTS", "DURATION", "CREATED"}
		rows := make([][]string, len(resp.Runs))
		for i, r := range resp.Runs {
			branch := ""
			if r.GitRef != nil {
				branch = r.GitRef.Branch
			}

			tests := "-"
			if r.Summary != nil {
				tests = fmt.Sprintf("%d/%d", r.Summary.Passed, r.Summary.Total)
			}

			duration := "-"
			if r.Summary != nil && r.Summary.Duration != nil {
				duration = formatDuration(r.Summary.Duration)
			}

			rows[i] = []string{
				truncate(r.ID, 12),
				r.ServiceName,
				formatRunStatus(r.Status),
				truncate(branch, 20),
				tests,
				duration,
				formatTimestamp(r.CreatedAt),
			}
		}

		printTable(headers, rows)

		if resp.Pagination != nil && resp.Pagination.HasMore {
			fmt.Printf("\n%s\n", Dim("More results available. Use --limit to see more."))
		}

		return nil
	},
}

// runGetCmd gets details for a specific run
var runGetCmd = &cobra.Command{
	Use:   "get <run-id>",
	Short: "Get run details",
	Long: `Display detailed information about a specific test run.

Shows run configuration, test results, and artifacts.`,
	Example: `  # Get run details
  conductor-ctl run get run-123

  # Include test results
  conductor-ctl run get run-123 --results

  # Include artifacts
  conductor-ctl run get run-123 --artifacts`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		runID := args[0]
		includeResults, _ := cmd.Flags().GetBool("results")
		includeArtifacts, _ := cmd.Flags().GetBool("artifacts")

		ShowSpinner("Fetching run details...")
		run, results, artifacts, err := apiClient.GetRun(ctx, runID, includeResults, includeArtifacts)
		HideSpinner()

		if err != nil {
			return fmt.Errorf("failed to get run: %w", err)
		}

		if outputFormat == "json" {
			return printJSON(map[string]interface{}{
				"run":       run,
				"results":   results,
				"artifacts": artifacts,
			})
		}

		// Print run details
		fmt.Printf("%s\n", Bold("Run Details"))
		fmt.Printf("  ID:             %s\n", run.ID)
		fmt.Printf("  Service:        %s (%s)\n", run.ServiceName, run.ServiceID)
		fmt.Printf("  Status:         %s\n", formatRunStatus(run.Status))
		fmt.Printf("  Execution Type: %s\n", formatExecutionType(run.ExecutionType))
		if run.AgentID != "" {
			fmt.Printf("  Agent:          %s\n", run.AgentID)
		}
		fmt.Printf("  Created:        %s\n", formatTimestamp(run.CreatedAt))
		if run.StartedAt != "" {
			fmt.Printf("  Started:        %s\n", formatTimestamp(run.StartedAt))
		}
		if run.FinishedAt != "" {
			fmt.Printf("  Finished:       %s\n", formatTimestamp(run.FinishedAt))
		}
		if run.ErrorMessage != "" {
			fmt.Printf("  Error:          %s\n", Red(run.ErrorMessage))
		}

		if run.GitRef != nil {
			fmt.Printf("\n%s\n", Bold("Git Reference"))
			fmt.Printf("  Repository: %s\n", run.GitRef.RepositoryURL)
			fmt.Printf("  Branch:     %s\n", run.GitRef.Branch)
			if run.GitRef.CommitSHA != "" {
				fmt.Printf("  Commit:     %s\n", run.GitRef.CommitSHAShort)
			}
			if run.GitRef.PullRequestNumber > 0 {
				fmt.Printf("  PR:         #%d\n", run.GitRef.PullRequestNumber)
			}
		}

		if run.Trigger != nil {
			fmt.Printf("\n%s\n", Bold("Trigger"))
			fmt.Printf("  Type: %s\n", formatTriggerType(run.Trigger.Type))
			if run.Trigger.User != "" {
				fmt.Printf("  User: %s\n", run.Trigger.User)
			}
		}

		if run.Summary != nil {
			fmt.Printf("\n%s\n", Bold("Summary"))
			fmt.Printf("  Total:   %d\n", run.Summary.Total)
			fmt.Printf("  Passed:  %s\n", Green(fmt.Sprintf("%d", run.Summary.Passed)))
			fmt.Printf("  Failed:  %s\n", colorizeNonZero(run.Summary.Failed, Red))
			fmt.Printf("  Skipped: %s\n", colorizeNonZero(run.Summary.Skipped, Yellow))
			fmt.Printf("  Errored: %s\n", colorizeNonZero(run.Summary.Errored, Red))
			if run.Summary.Duration != nil {
				fmt.Printf("  Duration: %s\n", formatDuration(run.Summary.Duration))
			}
		}

		if len(run.Labels) > 0 {
			fmt.Printf("\n%s\n", Bold("Labels"))
			for k, v := range run.Labels {
				fmt.Printf("  %s: %s\n", k, v)
			}
		}

		if includeResults && len(results) > 0 {
			fmt.Printf("\n%s\n", Bold("Test Results"))
			headers := []string{"TEST", "STATUS", "DURATION", "ERROR"}
			rows := make([][]string, len(results))
			for i, r := range results {
				errorMsg := "-"
				if r.ErrorMessage != "" {
					errorMsg = truncate(r.ErrorMessage, 40)
				}
				rows[i] = []string{
					truncate(r.TestName, 40),
					formatTestStatus(r.Status),
					formatDuration(r.Duration),
					errorMsg,
				}
			}
			printTable(headers, rows)
		}

		if includeArtifacts && len(artifacts) > 0 {
			fmt.Printf("\n%s\n", Bold("Artifacts"))
			headers := []string{"NAME", "TYPE", "SIZE", "PATH"}
			rows := make([][]string, len(artifacts))
			for i, a := range artifacts {
				rows[i] = []string{
					a.Name,
					a.ContentType,
					formatBytes(a.Size),
					truncate(a.Path, 40),
				}
			}
			printTable(headers, rows)
		}

		return nil
	},
}

// runTriggerCmd triggers a new test run
var runTriggerCmd = &cobra.Command{
	Use:   "trigger <service>",
	Short: "Trigger a new test run",
	Long: `Trigger a new test run for a service.

By default, runs all tests on the default branch.`,
	Example: `  # Trigger tests for a service
  conductor-ctl run trigger my-service

  # Trigger tests on a specific branch
  conductor-ctl run trigger my-service --ref feature/new-feature

  # Trigger specific tests
  conductor-ctl run trigger my-service --tests test-1,test-2

  # Trigger with higher priority
  conductor-ctl run trigger my-service --priority 10`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		serviceID := args[0]
		ref, _ := cmd.Flags().GetString("ref")
		testsStr, _ := cmd.Flags().GetString("tests")
		tagsStr, _ := cmd.Flags().GetString("tags")
		priority, _ := cmd.Flags().GetInt("priority")

		req := &CreateRunRequest{
			ServiceID: serviceID,
			Priority:  priority,
			Trigger: &RunTrigger{
				Type: "TRIGGER_TYPE_MANUAL",
			},
		}

		if ref != "" {
			req.GitRef = &GitRef{
				Branch: ref,
			}
		}

		if testsStr != "" {
			req.TestIDs = strings.Split(testsStr, ",")
		}

		if tagsStr != "" {
			req.Tags = strings.Split(tagsStr, ",")
		}

		ShowSpinner("Triggering run...")
		run, err := apiClient.CreateRun(ctx, req)
		HideSpinner()

		if err != nil {
			return fmt.Errorf("failed to trigger run: %w", err)
		}

		if outputFormat == "json" {
			return printJSON(run)
		}

		fmt.Printf("%s Run triggered successfully\n", Green("✓"))
		fmt.Printf("  Run ID:  %s\n", Bold(run.ID))
		fmt.Printf("  Service: %s\n", run.ServiceName)
		fmt.Printf("  Status:  %s\n", formatRunStatus(run.Status))

		return nil
	},
}

// runCancelCmd cancels a run
var runCancelCmd = &cobra.Command{
	Use:   "cancel <run-id>",
	Short: "Cancel a test run",
	Long: `Cancel a pending or running test run.

Only runs in pending or running status can be cancelled.`,
	Example: `  # Cancel a run
  conductor-ctl run cancel run-123

  # Cancel with a reason
  conductor-ctl run cancel run-123 --reason "no longer needed"`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		runID := args[0]
		reason, _ := cmd.Flags().GetString("reason")

		ShowSpinner("Cancelling run...")
		run, err := apiClient.CancelRun(ctx, runID, reason)
		HideSpinner()

		if err != nil {
			return fmt.Errorf("failed to cancel run: %w", err)
		}

		if outputFormat == "json" {
			return printJSON(run)
		}

		fmt.Printf("%s Run cancelled\n", Green("✓"))
		fmt.Printf("  Run ID: %s\n", Bold(run.ID))
		fmt.Printf("  Status: %s\n", formatRunStatus(run.Status))

		return nil
	},
}

// runRetryCmd retries a failed run
var runRetryCmd = &cobra.Command{
	Use:   "retry <run-id>",
	Short: "Retry a failed run",
	Long: `Create a new run from a previous run.

By default, retries all tests. Use --failed-only to retry only failed tests.`,
	Example: `  # Retry all tests from a run
  conductor-ctl run retry run-123

  # Retry only failed tests
  conductor-ctl run retry run-123 --failed-only`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		runID := args[0]
		failedOnly, _ := cmd.Flags().GetBool("failed-only")

		ShowSpinner("Creating retry run...")
		run, originalID, err := apiClient.RetryRun(ctx, runID, failedOnly, nil)
		HideSpinner()

		if err != nil {
			return fmt.Errorf("failed to retry run: %w", err)
		}

		if outputFormat == "json" {
			return printJSON(map[string]interface{}{
				"run":             run,
				"original_run_id": originalID,
			})
		}

		fmt.Printf("%s Retry run created\n", Green("✓"))
		fmt.Printf("  New Run ID:      %s\n", Bold(run.ID))
		fmt.Printf("  Original Run ID: %s\n", originalID)
		fmt.Printf("  Service:         %s\n", run.ServiceName)
		fmt.Printf("  Status:          %s\n", formatRunStatus(run.Status))

		return nil
	},
}

// runLogsCmd shows run logs
var runLogsCmd = &cobra.Command{
	Use:   "logs <run-id>",
	Short: "Show run logs",
	Long: `Display logs from a test run.

For running tests, use --follow to stream logs in real-time.`,
	Example: `  # Show logs for a run
  conductor-ctl run logs run-123

  # Follow logs in real-time
  conductor-ctl run logs run-123 --follow

  # Show only stderr
  conductor-ctl run logs run-123 --stream stderr`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		runID := args[0]
		follow, _ := cmd.Flags().GetBool("follow")
		stream, _ := cmd.Flags().GetString("stream")
		testID, _ := cmd.Flags().GetString("test")
		limit, _ := cmd.Flags().GetInt("limit")

		if follow {
			return streamLogs(runID, stream, testID)
		}

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		ShowSpinner("Fetching logs...")
		entries, err := apiClient.GetRunLogs(ctx, runID, stream, testID, limit)
		HideSpinner()

		if err != nil {
			return fmt.Errorf("failed to get logs: %w", err)
		}

		if outputFormat == "json" {
			return printJSON(entries)
		}

		if len(entries) == 0 {
			fmt.Println(Dim("No logs found."))
			return nil
		}

		for _, e := range entries {
			prefix := ""
			if e.Stream == "stderr" || e.Stream == "LOG_STREAM_STDERR" {
				prefix = Red("[stderr] ")
			}
			fmt.Printf("%s%s\n", prefix, e.Message)
		}

		return nil
	},
}

// streamLogs streams logs in real-time (simplified polling implementation)
func streamLogs(runID, stream, testID string) error {
	fmt.Printf("%s Streaming logs for run %s (press Ctrl+C to stop)\n\n", Dim("→"), runID)

	lastSeq := int64(0)
	reader := bufio.NewReader(os.Stdin)

	for {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		entries, err := apiClient.GetRunLogs(ctx, runID, stream, testID, 100)
		cancel()

		if err != nil {
			fmt.Printf("%s Error fetching logs: %v\n", Red("!"), err)
			time.Sleep(2 * time.Second)
			continue
		}

		for _, e := range entries {
			if e.Sequence > lastSeq {
				prefix := ""
				if e.Stream == "stderr" || e.Stream == "LOG_STREAM_STDERR" {
					prefix = Red("[stderr] ")
				}
				fmt.Printf("%s%s\n", prefix, e.Message)
				lastSeq = e.Sequence
			}
		}

		// Check if run is complete
		runCtx, runCancel := context.WithTimeout(context.Background(), 10*time.Second)
		run, _, _, err := apiClient.GetRun(runCtx, runID, false, false)
		runCancel()

		if err == nil && isTerminalStatus(run.Status) {
			fmt.Printf("\n%s Run completed with status: %s\n", Dim("→"), formatRunStatus(run.Status))
			return nil
		}

		// Check for user input (non-blocking)
		go func() {
			reader.ReadByte()
		}()

		time.Sleep(2 * time.Second)
	}
}

func isTerminalStatus(status string) bool {
	switch strings.ToLower(status) {
	case "run_status_passed", "passed",
		"run_status_failed", "failed",
		"run_status_error", "error",
		"run_status_timeout", "timeout",
		"run_status_cancelled", "cancelled":
		return true
	}
	return false
}

func init() {
	// List command flags
	runListCmd.Flags().String("service", "", "Filter by service")
	runListCmd.Flags().String("status", "", "Filter by status")
	runListCmd.Flags().Int("limit", 50, "Maximum number of results")

	// Get command flags
	runGetCmd.Flags().Bool("results", false, "Include test results")
	runGetCmd.Flags().Bool("artifacts", false, "Include artifacts")

	// Trigger command flags
	runTriggerCmd.Flags().String("ref", "", "Git reference (branch, tag, or commit)")
	runTriggerCmd.Flags().String("tests", "", "Comma-separated list of test IDs")
	runTriggerCmd.Flags().String("tags", "", "Comma-separated list of tags to filter tests")
	runTriggerCmd.Flags().Int("priority", 0, "Run priority (higher = more urgent)")

	// Cancel command flags
	runCancelCmd.Flags().String("reason", "", "Cancellation reason")

	// Retry command flags
	runRetryCmd.Flags().Bool("failed-only", false, "Retry only failed tests")

	// Logs command flags
	runLogsCmd.Flags().BoolP("follow", "f", false, "Follow logs in real-time")
	runLogsCmd.Flags().String("stream", "", "Filter by stream (stdout, stderr)")
	runLogsCmd.Flags().String("test", "", "Filter by test ID")
	runLogsCmd.Flags().Int("limit", 1000, "Maximum number of log entries")

	// Add subcommands
	runCmd.AddCommand(runListCmd)
	runCmd.AddCommand(runGetCmd)
	runCmd.AddCommand(runTriggerCmd)
	runCmd.AddCommand(runCancelCmd)
	runCmd.AddCommand(runRetryCmd)
	runCmd.AddCommand(runLogsCmd)
}

// formatRunStatus returns a colored status string
func formatRunStatus(status string) string {
	switch strings.ToLower(status) {
	case "run_status_pending", "pending":
		return Yellow("pending")
	case "run_status_running", "running":
		return Cyan("running")
	case "run_status_passed", "passed":
		return Green("passed")
	case "run_status_failed", "failed":
		return Red("failed")
	case "run_status_error", "error":
		return Red("error")
	case "run_status_timeout", "timeout":
		return Red("timeout")
	case "run_status_cancelled", "cancelled":
		return Dim("cancelled")
	default:
		return Dim(status)
	}
}

// formatTestStatus returns a colored test status string
func formatTestStatus(status string) string {
	switch strings.ToLower(status) {
	case "test_status_pass", "pass":
		return Green("pass")
	case "test_status_fail", "fail":
		return Red("fail")
	case "test_status_skip", "skip":
		return Yellow("skip")
	case "test_status_error", "error":
		return Red("error")
	default:
		return Dim(status)
	}
}

// formatTriggerType returns a human-readable trigger type
func formatTriggerType(triggerType string) string {
	switch strings.ToLower(triggerType) {
	case "trigger_type_manual", "manual":
		return "manual"
	case "trigger_type_webhook", "webhook":
		return "webhook"
	case "trigger_type_scheduled", "scheduled":
		return "scheduled"
	case "trigger_type_ci", "ci":
		return "CI"
	case "trigger_type_api", "api":
		return "API"
	case "trigger_type_retry", "retry":
		return "retry"
	default:
		return triggerType
	}
}

// formatExecutionType returns a human-readable execution type
func formatExecutionType(execType string) string {
	switch strings.ToLower(execType) {
	case "execution_type_subprocess", "subprocess":
		return "subprocess"
	case "execution_type_container", "container":
		return "container"
	default:
		return execType
	}
}

// colorizeNonZero applies color only if value is non-zero
func colorizeNonZero(val int, colorFn func(string) string) string {
	if val == 0 {
		return Dim("0")
	}
	return colorFn(fmt.Sprintf("%d", val))
}
