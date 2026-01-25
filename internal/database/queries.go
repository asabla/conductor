package database

// SQL queries for database operations.
// These are organized by entity type and operation.

// Service queries
const (
	// ServiceInsert inserts a new service.
	ServiceInsert = `
		INSERT INTO services (
			name, display_name, git_url, git_provider, default_branch,
			network_zones, owner, contact_slack, contact_email
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9
		) RETURNING id, created_at, updated_at`

	// ServiceGetByID retrieves a service by ID.
	ServiceGetByID = `
		SELECT id, name, display_name, git_url, git_provider, default_branch,
			   network_zones, owner, contact_slack, contact_email, created_at, updated_at
		FROM services
		WHERE id = $1`

	// ServiceGetByName retrieves a service by name.
	ServiceGetByName = `
		SELECT id, name, display_name, git_url, git_provider, default_branch,
			   network_zones, owner, contact_slack, contact_email, created_at, updated_at
		FROM services
		WHERE name = $1`

	// ServiceUpdate updates an existing service.
	ServiceUpdate = `
		UPDATE services
		SET name = $2, display_name = $3, git_url = $4, git_provider = $5,
			default_branch = $6, network_zones = $7, owner = $8,
			contact_slack = $9, contact_email = $10
		WHERE id = $1
		RETURNING updated_at`

	// ServiceDelete deletes a service by ID.
	ServiceDelete = `DELETE FROM services WHERE id = $1`

	// ServiceList lists services with pagination.
	ServiceList = `
		SELECT id, name, display_name, git_url, git_provider, default_branch,
			   network_zones, owner, contact_slack, contact_email, created_at, updated_at
		FROM services
		ORDER BY name ASC
		LIMIT $1 OFFSET $2`

	// ServiceCount counts total services.
	ServiceCount = `SELECT COUNT(*) FROM services`

	// ServiceListByOwner lists services by owner.
	ServiceListByOwner = `
		SELECT id, name, display_name, git_url, git_provider, default_branch,
			   network_zones, owner, contact_slack, contact_email, created_at, updated_at
		FROM services
		WHERE owner = $1
		ORDER BY name ASC
		LIMIT $2 OFFSET $3`

	// ServiceSearch searches services by name pattern.
	ServiceSearch = `
		SELECT id, name, display_name, git_url, git_provider, default_branch,
			   network_zones, owner, contact_slack, contact_email, created_at, updated_at
		FROM services
		WHERE name ILIKE $1 OR display_name ILIKE $1
		ORDER BY name ASC
		LIMIT $2 OFFSET $3`
)

// Test Definition queries
const (
	// TestDefInsert inserts a new test definition.
	TestDefInsert = `
		INSERT INTO test_definitions (
			service_id, name, description, execution_type, command, args,
			timeout_seconds, result_file, result_format, artifact_patterns,
			tags, depends_on, retries, allow_failure
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14
		) RETURNING id, created_at, updated_at`

	// TestDefGetByID retrieves a test definition by ID.
	TestDefGetByID = `
		SELECT id, service_id, name, description, execution_type, command, args,
			   timeout_seconds, result_file, result_format, artifact_patterns,
			   tags, depends_on, retries, allow_failure, created_at, updated_at
		FROM test_definitions
		WHERE id = $1`

	// TestDefListByService lists test definitions for a service.
	TestDefListByService = `
		SELECT id, service_id, name, description, execution_type, command, args,
			   timeout_seconds, result_file, result_format, artifact_patterns,
			   tags, depends_on, retries, allow_failure, created_at, updated_at
		FROM test_definitions
		WHERE service_id = $1
		ORDER BY name ASC
		LIMIT $2 OFFSET $3`

	// TestDefListByTags lists test definitions matching any of the given tags.
	TestDefListByTags = `
		SELECT id, service_id, name, description, execution_type, command, args,
			   timeout_seconds, result_file, result_format, artifact_patterns,
			   tags, depends_on, retries, allow_failure, created_at, updated_at
		FROM test_definitions
		WHERE service_id = $1 AND tags && $2
		ORDER BY name ASC
		LIMIT $3 OFFSET $4`

	// TestDefUpdate updates a test definition.
	TestDefUpdate = `
		UPDATE test_definitions
		SET name = $2, description = $3, execution_type = $4, command = $5,
			args = $6, timeout_seconds = $7, result_file = $8, result_format = $9,
			artifact_patterns = $10, tags = $11, depends_on = $12, retries = $13,
			allow_failure = $14
		WHERE id = $1
		RETURNING updated_at`

	// TestDefDelete deletes a test definition.
	TestDefDelete = `DELETE FROM test_definitions WHERE id = $1`
)

// Agent queries
const (
	// AgentInsert inserts a new agent.
	AgentInsert = `
		INSERT INTO agents (
			name, status, version, network_zones, max_parallel, docker_available
		) VALUES (
			$1, $2, $3, $4, $5, $6
		) RETURNING id, registered_at`

	// AgentGetByID retrieves an agent by ID.
	AgentGetByID = `
		SELECT id, name, status, version, network_zones, max_parallel,
			   docker_available, last_heartbeat, registered_at
		FROM agents
		WHERE id = $1`

	// AgentGetByName retrieves an agent by name.
	AgentGetByName = `
		SELECT id, name, status, version, network_zones, max_parallel,
			   docker_available, last_heartbeat, registered_at
		FROM agents
		WHERE name = $1`

	// AgentUpdate updates an agent.
	AgentUpdate = `
		UPDATE agents
		SET name = $2, status = $3, version = $4, network_zones = $5,
			max_parallel = $6, docker_available = $7
		WHERE id = $1`

	// AgentUpdateStatus updates only the agent's status.
	AgentUpdateStatus = `
		UPDATE agents
		SET status = $2
		WHERE id = $1`

	// AgentUpdateHeartbeat updates the agent's last heartbeat time.
	AgentUpdateHeartbeat = `
		UPDATE agents
		SET last_heartbeat = NOW(), status = $2
		WHERE id = $1`

	// AgentDelete deletes an agent.
	AgentDelete = `DELETE FROM agents WHERE id = $1`

	// AgentList lists all agents with pagination.
	AgentList = `
		SELECT id, name, status, version, network_zones, max_parallel,
			   docker_available, last_heartbeat, registered_at
		FROM agents
		ORDER BY name ASC
		LIMIT $1 OFFSET $2`

	// AgentListByStatus lists agents by status.
	AgentListByStatus = `
		SELECT id, name, status, version, network_zones, max_parallel,
			   docker_available, last_heartbeat, registered_at
		FROM agents
		WHERE status = $1
		ORDER BY name ASC
		LIMIT $2 OFFSET $3`

	// AgentGetAvailable retrieves agents that can run tests for a service's network zones.
	// Agents must be idle or have capacity, and have at least one matching network zone.
	AgentGetAvailable = `
		SELECT id, name, status, version, network_zones, max_parallel,
			   docker_available, last_heartbeat, registered_at
		FROM agents
		WHERE status IN ('idle', 'busy')
		  AND last_heartbeat > NOW() - INTERVAL '90 seconds'
		  AND network_zones && $1
		ORDER BY 
			CASE status WHEN 'idle' THEN 0 ELSE 1 END,
			last_heartbeat DESC
		LIMIT $2`

	// AgentMarkOffline marks agents as offline if they haven't sent a heartbeat recently.
	AgentMarkOffline = `
		UPDATE agents
		SET status = 'offline'
		WHERE status != 'offline'
		  AND (last_heartbeat IS NULL OR last_heartbeat < NOW() - INTERVAL '90 seconds')`

	// AgentCount counts agents by status.
	AgentCount = `
		SELECT status, COUNT(*) as count
		FROM agents
		GROUP BY status`
)

// Test Run queries
const (
	// RunInsert inserts a new test run.
	RunInsert = `
		INSERT INTO test_runs (
			service_id, status, git_ref, git_sha, trigger_type, triggered_by, priority
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7
		) RETURNING id, created_at`

	// RunGetByID retrieves a test run by ID.
	RunGetByID = `
		SELECT id, service_id, agent_id, status, git_ref, git_sha, trigger_type,
			   triggered_by, priority, created_at, started_at, finished_at,
			   total_tests, passed_tests, failed_tests, skipped_tests,
			   duration_ms, error_message
		FROM test_runs
		WHERE id = $1`

	// RunUpdate updates a test run.
	RunUpdate = `
		UPDATE test_runs
		SET agent_id = $2, status = $3, started_at = $4, finished_at = $5,
			total_tests = $6, passed_tests = $7, failed_tests = $8, skipped_tests = $9,
			duration_ms = $10, error_message = $11
		WHERE id = $1`

	// RunUpdateStatus updates only the run's status.
	RunUpdateStatus = `
		UPDATE test_runs
		SET status = $2
		WHERE id = $1`

	// RunStart marks a run as started.
	RunStart = `
		UPDATE test_runs
		SET status = 'running', agent_id = $2, started_at = NOW()
		WHERE id = $1`

	// RunFinish marks a run as finished.
	RunFinish = `
		UPDATE test_runs
		SET status = $2, finished_at = NOW(),
			total_tests = $3, passed_tests = $4, failed_tests = $5, skipped_tests = $6,
			duration_ms = $7, error_message = $8
		WHERE id = $1`

	// RunList lists test runs with pagination.
	RunList = `
		SELECT id, service_id, agent_id, status, git_ref, git_sha, trigger_type,
			   triggered_by, priority, created_at, started_at, finished_at,
			   total_tests, passed_tests, failed_tests, skipped_tests,
			   duration_ms, error_message
		FROM test_runs
		ORDER BY created_at DESC
		LIMIT $1 OFFSET $2`

	// RunListByService lists test runs for a service.
	RunListByService = `
		SELECT id, service_id, agent_id, status, git_ref, git_sha, trigger_type,
			   triggered_by, priority, created_at, started_at, finished_at,
			   total_tests, passed_tests, failed_tests, skipped_tests,
			   duration_ms, error_message
		FROM test_runs
		WHERE service_id = $1
		ORDER BY created_at DESC
		LIMIT $2 OFFSET $3`

	// RunListByStatus lists test runs by status.
	RunListByStatus = `
		SELECT id, service_id, agent_id, status, git_ref, git_sha, trigger_type,
			   triggered_by, priority, created_at, started_at, finished_at,
			   total_tests, passed_tests, failed_tests, skipped_tests,
			   duration_ms, error_message
		FROM test_runs
		WHERE status = $1
		ORDER BY priority DESC, created_at ASC
		LIMIT $2 OFFSET $3`

	// RunGetPending retrieves pending runs ordered by priority.
	RunGetPending = `
		SELECT id, service_id, agent_id, status, git_ref, git_sha, trigger_type,
			   triggered_by, priority, created_at, started_at, finished_at,
			   total_tests, passed_tests, failed_tests, skipped_tests,
			   duration_ms, error_message
		FROM test_runs
		WHERE status = 'pending'
		ORDER BY priority DESC, created_at ASC
		LIMIT $1`

	// RunGetRunning retrieves currently running tests.
	RunGetRunning = `
		SELECT id, service_id, agent_id, status, git_ref, git_sha, trigger_type,
			   triggered_by, priority, created_at, started_at, finished_at,
			   total_tests, passed_tests, failed_tests, skipped_tests,
			   duration_ms, error_message
		FROM test_runs
		WHERE status = 'running'
		ORDER BY started_at ASC`

	// RunListByServiceAndStatus lists runs for a service with a specific status.
	RunListByServiceAndStatus = `
		SELECT id, service_id, agent_id, status, git_ref, git_sha, trigger_type,
			   triggered_by, priority, created_at, started_at, finished_at,
			   total_tests, passed_tests, failed_tests, skipped_tests,
			   duration_ms, error_message
		FROM test_runs
		WHERE service_id = $1 AND status = $2
		ORDER BY created_at DESC
		LIMIT $3 OFFSET $4`

	// RunListByDateRange lists runs within a date range.
	RunListByDateRange = `
		SELECT id, service_id, agent_id, status, git_ref, git_sha, trigger_type,
			   triggered_by, priority, created_at, started_at, finished_at,
			   total_tests, passed_tests, failed_tests, skipped_tests,
			   duration_ms, error_message
		FROM test_runs
		WHERE created_at >= $1 AND created_at < $2
		ORDER BY created_at DESC
		LIMIT $3 OFFSET $4`

	// RunCount counts total runs.
	RunCount = `SELECT COUNT(*) FROM test_runs`

	// RunCountByStatus counts runs by status.
	RunCountByStatus = `
		SELECT status, COUNT(*) as count
		FROM test_runs
		GROUP BY status`
)

// Test Result queries
const (
	// ResultInsert inserts a new test result.
	ResultInsert = `
		INSERT INTO test_results (
			run_id, test_definition_id, test_name, suite_name, status,
			duration_ms, error_message, stack_trace, stdout, stderr, retry_count
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11
		) RETURNING id, created_at`

	// ResultGetByID retrieves a test result by ID.
	ResultGetByID = `
		SELECT id, run_id, test_definition_id, test_name, suite_name, status,
			   duration_ms, error_message, stack_trace, stdout, stderr,
			   retry_count, created_at
		FROM test_results
		WHERE id = $1`

	// ResultListByRun lists results for a test run.
	ResultListByRun = `
		SELECT id, run_id, test_definition_id, test_name, suite_name, status,
			   duration_ms, error_message, stack_trace, stdout, stderr,
			   retry_count, created_at
		FROM test_results
		WHERE run_id = $1
		ORDER BY test_name ASC`

	// ResultListByRunAndStatus lists results for a run with a specific status.
	ResultListByRunAndStatus = `
		SELECT id, run_id, test_definition_id, test_name, suite_name, status,
			   duration_ms, error_message, stack_trace, stdout, stderr,
			   retry_count, created_at
		FROM test_results
		WHERE run_id = $1 AND status = $2
		ORDER BY test_name ASC`

	// ResultCountByRun counts results by status for a run.
	ResultCountByRun = `
		SELECT status, COUNT(*) as count
		FROM test_results
		WHERE run_id = $1
		GROUP BY status`

	// ResultDelete deletes results for a run.
	ResultDelete = `DELETE FROM test_results WHERE run_id = $1`
)

// Artifact queries
const (
	// ArtifactInsert inserts a new artifact.
	ArtifactInsert = `
		INSERT INTO artifacts (run_id, name, path, content_type, size_bytes)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id, created_at`

	// ArtifactGetByID retrieves an artifact by ID.
	ArtifactGetByID = `
		SELECT id, run_id, name, path, content_type, size_bytes, created_at
		FROM artifacts
		WHERE id = $1`

	// ArtifactListByRun lists artifacts for a run.
	ArtifactListByRun = `
		SELECT id, run_id, name, path, content_type, size_bytes, created_at
		FROM artifacts
		WHERE run_id = $1
		ORDER BY name ASC`

	// ArtifactListOlderThan lists artifacts older than a timestamp.
	ArtifactListOlderThan = `
		SELECT id, run_id, name, path, content_type, size_bytes, created_at
		FROM artifacts
		WHERE created_at < $1
		ORDER BY created_at ASC
		LIMIT $2`

	// ArtifactDelete deletes an artifact.
	ArtifactDelete = `DELETE FROM artifacts WHERE id = $1`

	// ArtifactDeleteByRun deletes all artifacts for a run.
	ArtifactDeleteByRun = `DELETE FROM artifacts WHERE run_id = $1`
)

// Notification queries
const (
	// NotificationChannelInsert inserts a new notification channel.
	NotificationChannelInsert = `
		INSERT INTO notification_channels (name, type, config, enabled)
		VALUES ($1, $2, $3, $4)
		RETURNING id, created_at, updated_at`

	// NotificationChannelGetByID retrieves a channel by ID.
	NotificationChannelGetByID = `
		SELECT id, name, type, config, enabled, created_at, updated_at
		FROM notification_channels
		WHERE id = $1`

	// NotificationChannelList lists all channels.
	NotificationChannelList = `
		SELECT id, name, type, config, enabled, created_at, updated_at
		FROM notification_channels
		ORDER BY name ASC
		LIMIT $1 OFFSET $2`

	// NotificationChannelListEnabled lists enabled channels.
	NotificationChannelListEnabled = `
		SELECT id, name, type, config, enabled, created_at, updated_at
		FROM notification_channels
		WHERE enabled = true
		ORDER BY name ASC`

	// NotificationRuleInsert inserts a new notification rule.
	NotificationRuleInsert = `
		INSERT INTO notification_rules (channel_id, service_id, trigger_on, enabled)
		VALUES ($1, $2, $3, $4)
		RETURNING id, created_at, updated_at`

	// NotificationRuleListByService lists rules for a service (including global rules).
	NotificationRuleListByService = `
		SELECT id, channel_id, service_id, trigger_on, enabled, created_at, updated_at
		FROM notification_rules
		WHERE enabled = true AND (service_id IS NULL OR service_id = $1)
		ORDER BY service_id NULLS LAST`

	// NotificationRuleListByChannel lists rules for a channel.
	NotificationRuleListByChannel = `
		SELECT id, channel_id, service_id, trigger_on, enabled, created_at, updated_at
		FROM notification_rules
		WHERE channel_id = $1
		ORDER BY created_at ASC`
)

// Schedule queries
const (
	// ScheduleInsert inserts a new scheduled run.
	ScheduleInsert = `
		INSERT INTO scheduled_runs (
			service_id, name, cron_expression, git_ref, test_filter, enabled, next_run_at
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7
		) RETURNING id, created_at, updated_at`

	// ScheduleGetByID retrieves a schedule by ID.
	ScheduleGetByID = `
		SELECT id, service_id, name, cron_expression, git_ref, test_filter,
			   enabled, last_run_at, next_run_at, created_at, updated_at
		FROM scheduled_runs
		WHERE id = $1`

	// ScheduleListDue lists schedules that are due to run.
	ScheduleListDue = `
		SELECT id, service_id, name, cron_expression, git_ref, test_filter,
			   enabled, last_run_at, next_run_at, created_at, updated_at
		FROM scheduled_runs
		WHERE enabled = true AND next_run_at <= NOW()
		ORDER BY next_run_at ASC`

	// ScheduleUpdateAfterRun updates a schedule after it has run.
	ScheduleUpdateAfterRun = `
		UPDATE scheduled_runs
		SET last_run_at = NOW(), next_run_at = $2
		WHERE id = $1`

	// ScheduleListByService lists schedules for a service.
	ScheduleListByService = `
		SELECT id, service_id, name, cron_expression, git_ref, test_filter,
			   enabled, last_run_at, next_run_at, created_at, updated_at
		FROM scheduled_runs
		WHERE service_id = $1
		ORDER BY name ASC`
)

// Analytics queries
const (
	// DailyStatsUpsert inserts or updates daily stats.
	DailyStatsUpsert = `
		INSERT INTO daily_stats (
			service_id, date, total_runs, passed_runs, failed_runs,
			total_tests, passed_tests, failed_tests,
			avg_duration_ms, p50_duration_ms, p95_duration_ms
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11
		)
		ON CONFLICT (service_id, date) DO UPDATE SET
			total_runs = daily_stats.total_runs + EXCLUDED.total_runs,
			passed_runs = daily_stats.passed_runs + EXCLUDED.passed_runs,
			failed_runs = daily_stats.failed_runs + EXCLUDED.failed_runs,
			total_tests = daily_stats.total_tests + EXCLUDED.total_tests,
			passed_tests = daily_stats.passed_tests + EXCLUDED.passed_tests,
			failed_tests = daily_stats.failed_tests + EXCLUDED.failed_tests`

	// DailyStatsGetByServiceAndRange retrieves daily stats for a service within a date range.
	DailyStatsGetByServiceAndRange = `
		SELECT id, service_id, date, total_runs, passed_runs, failed_runs,
			   total_tests, passed_tests, failed_tests,
			   avg_duration_ms, p50_duration_ms, p95_duration_ms
		FROM daily_stats
		WHERE service_id = $1 AND date >= $2 AND date <= $3
		ORDER BY date ASC`

	// FlakyTestUpsert inserts or updates a flaky test record.
	FlakyTestUpsert = `
		INSERT INTO flaky_tests (
			service_id, test_name, flakiness_score, total_runs, flaky_runs, last_flaky_at
		) VALUES (
			$1, $2, $3, $4, $5, NOW()
		)
		ON CONFLICT (service_id, test_name) DO UPDATE SET
			flakiness_score = EXCLUDED.flakiness_score,
			total_runs = flaky_tests.total_runs + EXCLUDED.total_runs,
			flaky_runs = flaky_tests.flaky_runs + EXCLUDED.flaky_runs,
			last_flaky_at = NOW()`

	// FlakyTestListByService lists flaky tests for a service.
	FlakyTestListByService = `
		SELECT id, service_id, test_name, flakiness_score, total_runs, flaky_runs,
			   first_detected_at, last_flaky_at, quarantined, quarantined_at,
			   quarantined_by, notes
		FROM flaky_tests
		WHERE service_id = $1
		ORDER BY flakiness_score DESC
		LIMIT $2 OFFSET $3`

	// FlakyTestQuarantine quarantines a flaky test.
	FlakyTestQuarantine = `
		UPDATE flaky_tests
		SET quarantined = true, quarantined_at = NOW(), quarantined_by = $2
		WHERE id = $1`

	// FlakyTestUnquarantine removes quarantine from a flaky test.
	FlakyTestUnquarantine = `
		UPDATE flaky_tests
		SET quarantined = false, quarantined_at = NULL, quarantined_by = NULL
		WHERE id = $1`

	// TestHistoryInsert inserts a test history record.
	TestHistoryInsert = `
		INSERT INTO test_history (service_id, test_name, run_id, status, duration_ms)
		VALUES ($1, $2, $3, $4, $5)`

	// TestHistoryGetRecent retrieves recent history for a test.
	TestHistoryGetRecent = `
		SELECT id, service_id, test_name, run_id, status, duration_ms, executed_at
		FROM test_history
		WHERE service_id = $1 AND test_name = $2
		ORDER BY executed_at DESC
		LIMIT $3`

	// ServiceHealthSummaryGet retrieves the health summary view data.
	ServiceHealthSummaryGet = `
		SELECT service_id, service_name, display_name,
			   runs_last_7_days, passed_last_7_days, failed_last_7_days,
			   pass_rate_7_days, flaky_test_count, last_run_at, last_run_status
		FROM service_health_summary
		ORDER BY service_name ASC`

	// ServiceHealthSummaryGetByID retrieves health summary for a specific service.
	ServiceHealthSummaryGetByID = `
		SELECT service_id, service_name, display_name,
			   runs_last_7_days, passed_last_7_days, failed_last_7_days,
			   pass_rate_7_days, flaky_test_count, last_run_at, last_run_status
		FROM service_health_summary
		WHERE service_id = $1`
)
