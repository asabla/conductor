package database

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// notificationRepo implements NotificationRepository.
type notificationRepo struct {
	db *DB
}

// NewNotificationRepo creates a new notification repository.
func NewNotificationRepo(db *DB) NotificationRepository {
	return &notificationRepo{db: db}
}

// CreateChannel creates a new notification channel.
func (r *notificationRepo) CreateChannel(ctx context.Context, channel *NotificationChannel) error {
	err := r.db.pool.QueryRow(ctx, NotificationChannelInsert,
		channel.Name,
		channel.Type,
		channel.Config,
		channel.Enabled,
	).Scan(&channel.ID, &channel.CreatedAt, &channel.UpdatedAt)

	if err != nil {
		return fmt.Errorf("failed to create notification channel: %w", WrapDBError(err))
	}
	return nil
}

// GetChannel retrieves a channel by ID.
func (r *notificationRepo) GetChannel(ctx context.Context, id uuid.UUID) (*NotificationChannel, error) {
	channel := &NotificationChannel{}
	err := r.db.pool.QueryRow(ctx, NotificationChannelGetByID, id).Scan(
		&channel.ID,
		&channel.Name,
		&channel.Type,
		&channel.Config,
		&channel.Enabled,
		&channel.CreatedAt,
		&channel.UpdatedAt,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("failed to get notification channel: %w", err)
	}
	return channel, nil
}

// UpdateChannel updates a notification channel.
func (r *notificationRepo) UpdateChannel(ctx context.Context, channel *NotificationChannel) error {
	const query = `
		UPDATE notification_channels
		SET name = $2, type = $3, config = $4, enabled = $5
		WHERE id = $1
		RETURNING updated_at`

	err := r.db.pool.QueryRow(ctx, query,
		channel.ID,
		channel.Name,
		channel.Type,
		channel.Config,
		channel.Enabled,
	).Scan(&channel.UpdatedAt)

	if err != nil {
		if err == pgx.ErrNoRows {
			return ErrNotFound
		}
		return fmt.Errorf("failed to update notification channel: %w", WrapDBError(err))
	}
	return nil
}

// DeleteChannel deletes a notification channel.
func (r *notificationRepo) DeleteChannel(ctx context.Context, id uuid.UUID) error {
	const query = `DELETE FROM notification_channels WHERE id = $1`
	result, err := r.db.pool.Exec(ctx, query, id)
	if err != nil {
		return fmt.Errorf("failed to delete notification channel: %w", err)
	}
	if result.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// ListChannels returns all channels with pagination.
func (r *notificationRepo) ListChannels(ctx context.Context, page Pagination) ([]NotificationChannel, error) {
	rows, err := r.db.pool.Query(ctx, NotificationChannelList, page.Limit, page.Offset)
	if err != nil {
		return nil, fmt.Errorf("failed to list notification channels: %w", err)
	}
	defer rows.Close()

	return scanNotificationChannels(rows)
}

// ListEnabledChannels returns all enabled channels.
func (r *notificationRepo) ListEnabledChannels(ctx context.Context) ([]NotificationChannel, error) {
	rows, err := r.db.pool.Query(ctx, NotificationChannelListEnabled)
	if err != nil {
		return nil, fmt.Errorf("failed to list enabled notification channels: %w", err)
	}
	defer rows.Close()

	return scanNotificationChannels(rows)
}

// scanNotificationChannels scans rows into a slice of notification channels.
func scanNotificationChannels(rows pgx.Rows) ([]NotificationChannel, error) {
	var channels []NotificationChannel
	for rows.Next() {
		var channel NotificationChannel
		err := rows.Scan(
			&channel.ID,
			&channel.Name,
			&channel.Type,
			&channel.Config,
			&channel.Enabled,
			&channel.CreatedAt,
			&channel.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan notification channel: %w", err)
		}
		channels = append(channels, channel)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating notification channels: %w", err)
	}

	return channels, nil
}

// CreateRule creates a new notification rule.
func (r *notificationRepo) CreateRule(ctx context.Context, rule *NotificationRule) error {
	err := r.db.pool.QueryRow(ctx, NotificationRuleInsert,
		rule.ChannelID,
		rule.ServiceID,
		rule.TriggerOn,
		rule.Enabled,
	).Scan(&rule.ID, &rule.CreatedAt, &rule.UpdatedAt)

	if err != nil {
		return fmt.Errorf("failed to create notification rule: %w", WrapDBError(err))
	}
	return nil
}

// GetRule retrieves a rule by ID.
func (r *notificationRepo) GetRule(ctx context.Context, id uuid.UUID) (*NotificationRule, error) {
	const query = `
		SELECT id, channel_id, service_id, trigger_on, enabled, created_at, updated_at
		FROM notification_rules
		WHERE id = $1`

	rule := &NotificationRule{}
	err := r.db.pool.QueryRow(ctx, query, id).Scan(
		&rule.ID,
		&rule.ChannelID,
		&rule.ServiceID,
		&rule.TriggerOn,
		&rule.Enabled,
		&rule.CreatedAt,
		&rule.UpdatedAt,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("failed to get notification rule: %w", err)
	}
	return rule, nil
}

// UpdateRule updates a notification rule.
func (r *notificationRepo) UpdateRule(ctx context.Context, rule *NotificationRule) error {
	const query = `
		UPDATE notification_rules
		SET channel_id = $2, service_id = $3, trigger_on = $4, enabled = $5
		WHERE id = $1
		RETURNING updated_at`

	err := r.db.pool.QueryRow(ctx, query,
		rule.ID,
		rule.ChannelID,
		rule.ServiceID,
		rule.TriggerOn,
		rule.Enabled,
	).Scan(&rule.UpdatedAt)

	if err != nil {
		if err == pgx.ErrNoRows {
			return ErrNotFound
		}
		return fmt.Errorf("failed to update notification rule: %w", WrapDBError(err))
	}
	return nil
}

// DeleteRule deletes a notification rule.
func (r *notificationRepo) DeleteRule(ctx context.Context, id uuid.UUID) error {
	const query = `DELETE FROM notification_rules WHERE id = $1`
	result, err := r.db.pool.Exec(ctx, query, id)
	if err != nil {
		return fmt.Errorf("failed to delete notification rule: %w", err)
	}
	if result.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// ListRulesByService returns rules for a service (including global rules).
func (r *notificationRepo) ListRulesByService(ctx context.Context, serviceID uuid.UUID) ([]NotificationRule, error) {
	rows, err := r.db.pool.Query(ctx, NotificationRuleListByService, serviceID)
	if err != nil {
		return nil, fmt.Errorf("failed to list notification rules by service: %w", err)
	}
	defer rows.Close()

	return scanNotificationRules(rows)
}

// ListRulesByChannel returns rules for a channel.
func (r *notificationRepo) ListRulesByChannel(ctx context.Context, channelID uuid.UUID) ([]NotificationRule, error) {
	rows, err := r.db.pool.Query(ctx, NotificationRuleListByChannel, channelID)
	if err != nil {
		return nil, fmt.Errorf("failed to list notification rules by channel: %w", err)
	}
	defer rows.Close()

	return scanNotificationRules(rows)
}

// scanNotificationRules scans rows into a slice of notification rules.
func scanNotificationRules(rows pgx.Rows) ([]NotificationRule, error) {
	var rules []NotificationRule
	for rows.Next() {
		var rule NotificationRule
		err := rows.Scan(
			&rule.ID,
			&rule.ChannelID,
			&rule.ServiceID,
			&rule.TriggerOn,
			&rule.Enabled,
			&rule.CreatedAt,
			&rule.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan notification rule: %w", err)
		}
		rules = append(rules, rule)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating notification rules: %w", err)
	}

	return rules, nil
}

// scheduleRepo implements ScheduleRepository.
type scheduleRepo struct {
	db *DB
}

// NewScheduleRepo creates a new schedule repository.
func NewScheduleRepo(db *DB) ScheduleRepository {
	return &scheduleRepo{db: db}
}

// Create creates a new scheduled run.
func (r *scheduleRepo) Create(ctx context.Context, schedule *ScheduledRun) error {
	err := r.db.pool.QueryRow(ctx, ScheduleInsert,
		schedule.ServiceID,
		schedule.Name,
		schedule.CronExpression,
		schedule.GitRef,
		schedule.TestFilter,
		schedule.Enabled,
		schedule.NextRunAt,
	).Scan(&schedule.ID, &schedule.CreatedAt, &schedule.UpdatedAt)

	if err != nil {
		return fmt.Errorf("failed to create scheduled run: %w", WrapDBError(err))
	}
	return nil
}

// Get retrieves a scheduled run by ID.
func (r *scheduleRepo) Get(ctx context.Context, id uuid.UUID) (*ScheduledRun, error) {
	schedule := &ScheduledRun{}
	err := r.db.pool.QueryRow(ctx, ScheduleGetByID, id).Scan(
		&schedule.ID,
		&schedule.ServiceID,
		&schedule.Name,
		&schedule.CronExpression,
		&schedule.GitRef,
		&schedule.TestFilter,
		&schedule.Enabled,
		&schedule.LastRunAt,
		&schedule.NextRunAt,
		&schedule.CreatedAt,
		&schedule.UpdatedAt,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("failed to get scheduled run: %w", err)
	}
	return schedule, nil
}

// Update updates a scheduled run.
func (r *scheduleRepo) Update(ctx context.Context, schedule *ScheduledRun) error {
	const query = `
		UPDATE scheduled_runs
		SET name = $2, cron_expression = $3, git_ref = $4, test_filter = $5,
			enabled = $6, next_run_at = $7
		WHERE id = $1
		RETURNING updated_at`

	err := r.db.pool.QueryRow(ctx, query,
		schedule.ID,
		schedule.Name,
		schedule.CronExpression,
		schedule.GitRef,
		schedule.TestFilter,
		schedule.Enabled,
		schedule.NextRunAt,
	).Scan(&schedule.UpdatedAt)

	if err != nil {
		if err == pgx.ErrNoRows {
			return ErrNotFound
		}
		return fmt.Errorf("failed to update scheduled run: %w", WrapDBError(err))
	}
	return nil
}

// Delete deletes a scheduled run.
func (r *scheduleRepo) Delete(ctx context.Context, id uuid.UUID) error {
	const query = `DELETE FROM scheduled_runs WHERE id = $1`
	result, err := r.db.pool.Exec(ctx, query, id)
	if err != nil {
		return fmt.Errorf("failed to delete scheduled run: %w", err)
	}
	if result.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// ListByService returns schedules for a service.
func (r *scheduleRepo) ListByService(ctx context.Context, serviceID uuid.UUID) ([]ScheduledRun, error) {
	rows, err := r.db.pool.Query(ctx, ScheduleListByService, serviceID)
	if err != nil {
		return nil, fmt.Errorf("failed to list schedules by service: %w", err)
	}
	defer rows.Close()

	return scanScheduledRuns(rows)
}

// ListDue returns schedules that are due to run.
func (r *scheduleRepo) ListDue(ctx context.Context) ([]ScheduledRun, error) {
	rows, err := r.db.pool.Query(ctx, ScheduleListDue)
	if err != nil {
		return nil, fmt.Errorf("failed to list due schedules: %w", err)
	}
	defer rows.Close()

	return scanScheduledRuns(rows)
}

// UpdateAfterRun updates a schedule after it has executed.
func (r *scheduleRepo) UpdateAfterRun(ctx context.Context, id uuid.UUID, nextRunAt time.Time) error {
	result, err := r.db.pool.Exec(ctx, ScheduleUpdateAfterRun, id, nextRunAt)
	if err != nil {
		return fmt.Errorf("failed to update schedule after run: %w", err)
	}
	if result.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// scanScheduledRuns scans rows into a slice of scheduled runs.
func scanScheduledRuns(rows pgx.Rows) ([]ScheduledRun, error) {
	var schedules []ScheduledRun
	for rows.Next() {
		var schedule ScheduledRun
		err := rows.Scan(
			&schedule.ID,
			&schedule.ServiceID,
			&schedule.Name,
			&schedule.CronExpression,
			&schedule.GitRef,
			&schedule.TestFilter,
			&schedule.Enabled,
			&schedule.LastRunAt,
			&schedule.NextRunAt,
			&schedule.CreatedAt,
			&schedule.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan scheduled run: %w", err)
		}
		schedules = append(schedules, schedule)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating scheduled runs: %w", err)
	}

	return schedules, nil
}

// analyticsRepo implements AnalyticsRepository.
type analyticsRepo struct {
	db *DB
}

// NewAnalyticsRepo creates a new analytics repository.
func NewAnalyticsRepo(db *DB) AnalyticsRepository {
	return &analyticsRepo{db: db}
}

// UpsertDailyStats inserts or updates daily statistics.
func (r *analyticsRepo) UpsertDailyStats(ctx context.Context, stats *DailyStats) error {
	_, err := r.db.pool.Exec(ctx, DailyStatsUpsert,
		stats.ServiceID,
		stats.Date,
		stats.TotalRuns,
		stats.PassedRuns,
		stats.FailedRuns,
		stats.TotalTests,
		stats.PassedTests,
		stats.FailedTests,
		stats.AvgDurationMs,
		stats.P50DurationMs,
		stats.P95DurationMs,
	)
	if err != nil {
		return fmt.Errorf("failed to upsert daily stats: %w", err)
	}
	return nil
}

// GetDailyStats retrieves daily stats for a service within a date range.
func (r *analyticsRepo) GetDailyStats(ctx context.Context, serviceID uuid.UUID, start, end time.Time) ([]DailyStats, error) {
	rows, err := r.db.pool.Query(ctx, DailyStatsGetByServiceAndRange, serviceID, start, end)
	if err != nil {
		return nil, fmt.Errorf("failed to get daily stats: %w", err)
	}
	defer rows.Close()

	var stats []DailyStats
	for rows.Next() {
		var s DailyStats
		err := rows.Scan(
			&s.ID,
			&s.ServiceID,
			&s.Date,
			&s.TotalRuns,
			&s.PassedRuns,
			&s.FailedRuns,
			&s.TotalTests,
			&s.PassedTests,
			&s.FailedTests,
			&s.AvgDurationMs,
			&s.P50DurationMs,
			&s.P95DurationMs,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan daily stats: %w", err)
		}
		stats = append(stats, s)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating daily stats: %w", err)
	}

	return stats, nil
}

// UpsertFlakyTest inserts or updates a flaky test record.
func (r *analyticsRepo) UpsertFlakyTest(ctx context.Context, serviceID uuid.UUID, testName string, score float64, runs, flakyRuns int) error {
	_, err := r.db.pool.Exec(ctx, FlakyTestUpsert, serviceID, testName, score, runs, flakyRuns)
	if err != nil {
		return fmt.Errorf("failed to upsert flaky test: %w", err)
	}
	return nil
}

// ListFlakyTests returns flaky tests for a service.
func (r *analyticsRepo) ListFlakyTests(ctx context.Context, serviceID uuid.UUID, page Pagination) ([]FlakyTest, error) {
	rows, err := r.db.pool.Query(ctx, FlakyTestListByService, serviceID, page.Limit, page.Offset)
	if err != nil {
		return nil, fmt.Errorf("failed to list flaky tests: %w", err)
	}
	defer rows.Close()

	var tests []FlakyTest
	for rows.Next() {
		var t FlakyTest
		err := rows.Scan(
			&t.ID,
			&t.ServiceID,
			&t.TestName,
			&t.FlakinessScore,
			&t.TotalRuns,
			&t.FlakyRuns,
			&t.FirstDetected,
			&t.LastFlakyAt,
			&t.Quarantined,
			&t.QuarantinedAt,
			&t.QuarantinedBy,
			&t.Notes,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan flaky test: %w", err)
		}
		tests = append(tests, t)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating flaky tests: %w", err)
	}

	return tests, nil
}

// QuarantineTest quarantines a flaky test.
func (r *analyticsRepo) QuarantineTest(ctx context.Context, id uuid.UUID, by string) error {
	result, err := r.db.pool.Exec(ctx, FlakyTestQuarantine, id, by)
	if err != nil {
		return fmt.Errorf("failed to quarantine test: %w", err)
	}
	if result.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// QuarantineTestByName quarantines a flaky test by service and name.
func (r *analyticsRepo) QuarantineTestByName(ctx context.Context, serviceID uuid.UUID, testName string, by string) error {
	result, err := r.db.pool.Exec(ctx, FlakyTestQuarantineByName, serviceID, testName, by)
	if err != nil {
		return fmt.Errorf("failed to quarantine test: %w", err)
	}
	if result.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// UnquarantineTest removes quarantine from a test.
func (r *analyticsRepo) UnquarantineTest(ctx context.Context, id uuid.UUID) error {
	result, err := r.db.pool.Exec(ctx, FlakyTestUnquarantine, id)
	if err != nil {
		return fmt.Errorf("failed to unquarantine test: %w", err)
	}
	if result.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// RecordTestHistory records a test execution in history.
func (r *analyticsRepo) RecordTestHistory(ctx context.Context, history *TestHistory) error {
	_, err := r.db.pool.Exec(ctx, TestHistoryInsert,
		history.ServiceID,
		history.TestName,
		history.RunID,
		history.Status,
		history.DurationMs,
	)
	if err != nil {
		return fmt.Errorf("failed to record test history: %w", err)
	}
	return nil
}

// GetTestHistory retrieves recent history for a test.
func (r *analyticsRepo) GetTestHistory(ctx context.Context, serviceID uuid.UUID, testName string, limit int) ([]TestHistory, error) {
	rows, err := r.db.pool.Query(ctx, TestHistoryGetRecent, serviceID, testName, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to get test history: %w", err)
	}
	defer rows.Close()

	var history []TestHistory
	for rows.Next() {
		var h TestHistory
		err := rows.Scan(
			&h.ID,
			&h.ServiceID,
			&h.TestName,
			&h.RunID,
			&h.Status,
			&h.DurationMs,
			&h.ExecutedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan test history: %w", err)
		}
		history = append(history, h)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating test history: %w", err)
	}

	return history, nil
}

// GetServiceHealthSummary retrieves health summaries for all services.
func (r *analyticsRepo) GetServiceHealthSummary(ctx context.Context) ([]ServiceHealthSummary, error) {
	rows, err := r.db.pool.Query(ctx, ServiceHealthSummaryGet)
	if err != nil {
		return nil, fmt.Errorf("failed to get service health summary: %w", err)
	}
	defer rows.Close()

	return scanServiceHealthSummaries(rows)
}

// GetServiceHealthSummaryByID retrieves health summary for a specific service.
func (r *analyticsRepo) GetServiceHealthSummaryByID(ctx context.Context, serviceID uuid.UUID) (*ServiceHealthSummary, error) {
	summary := &ServiceHealthSummary{}
	err := r.db.pool.QueryRow(ctx, ServiceHealthSummaryGetByID, serviceID).Scan(
		&summary.ServiceID,
		&summary.ServiceName,
		&summary.DisplayName,
		&summary.RunsLast7Days,
		&summary.PassedLast7Days,
		&summary.FailedLast7Days,
		&summary.PassRate7Days,
		&summary.FlakyTestCount,
		&summary.LastRunAt,
		&summary.LastRunStatus,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("failed to get service health summary: %w", err)
	}
	return summary, nil
}

// scanServiceHealthSummaries scans rows into a slice of service health summaries.
func scanServiceHealthSummaries(rows pgx.Rows) ([]ServiceHealthSummary, error) {
	var summaries []ServiceHealthSummary
	for rows.Next() {
		var s ServiceHealthSummary
		err := rows.Scan(
			&s.ServiceID,
			&s.ServiceName,
			&s.DisplayName,
			&s.RunsLast7Days,
			&s.PassedLast7Days,
			&s.FailedLast7Days,
			&s.PassRate7Days,
			&s.FlakyTestCount,
			&s.LastRunAt,
			&s.LastRunStatus,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan service health summary: %w", err)
		}
		summaries = append(summaries, s)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating service health summaries: %w", err)
	}

	return summaries, nil
}
