package notification

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/conductor/conductor/internal/database"
)

// NotificationService defines the interface for the notification service.
type NotificationService interface {
	// SendNotification sends a notification through appropriate channels.
	SendNotification(ctx context.Context, event *Event) error
	// ProcessRules evaluates notification rules and sends matching notifications.
	ProcessRules(ctx context.Context, event *Event) ([]SendResult, error)
	// Start starts the notification service background workers.
	Start(ctx context.Context) error
	// Stop gracefully stops the notification service.
	Stop(ctx context.Context) error
	// TestChannel sends a test notification to a specific channel.
	TestChannel(ctx context.Context, channelID uuid.UUID, message string) (*SendResult, error)
}

// Config holds configuration for the notification service.
type Config struct {
	// WorkerCount is the number of worker goroutines.
	WorkerCount int
	// QueueSize is the size of the notification queue.
	QueueSize int
	// DefaultTimeout is the default timeout for sending notifications.
	DefaultTimeout time.Duration
	// ThrottleDuration is how long to throttle duplicate notifications.
	ThrottleDuration time.Duration
	// RetryAttempts is the number of retry attempts for failed sends.
	RetryAttempts int
	// BaseURL is the base URL for notification links.
	BaseURL string
	// Email contains SMTP configuration for email notifications.
	Email EmailSettings
}

// EmailSettings contains SMTP configuration for email notifications.
type EmailSettings struct {
	SMTPHost    string
	SMTPPort    int
	Username    string
	Password    string
	FromAddress string
	FromName    string
	UseTLS      bool
	SkipVerify  bool
	ConnTimeout time.Duration
}

// DefaultConfig returns a default configuration.
func DefaultConfig() Config {
	return Config{
		WorkerCount:      5,
		QueueSize:        1000,
		DefaultTimeout:   30 * time.Second,
		ThrottleDuration: 5 * time.Minute,
		RetryAttempts:    3,
		BaseURL:          "",
		Email: EmailSettings{
			ConnTimeout: 30 * time.Second,
		},
	}
}

// Service implements the NotificationService interface.
type Service struct {
	config     Config
	repo       database.NotificationRepository
	ruleEngine *RuleEngine
	channels   map[uuid.UUID]Channel
	channelsMu sync.RWMutex
	queue      chan *notificationJob
	logger     *slog.Logger
	wg         sync.WaitGroup
	cancel     context.CancelFunc
	started    bool
	startMu    sync.Mutex
}

// notificationJob represents a notification job in the queue.
type notificationJob struct {
	notification *Notification
	channel      Channel
	channelID    uuid.UUID
	resultCh     chan<- SendResult
}

// NewService creates a new notification service.
func NewService(config Config, repo database.NotificationRepository, logger *slog.Logger) *Service {
	if logger == nil {
		logger = slog.Default()
	}

	if config.WorkerCount <= 0 {
		config.WorkerCount = DefaultConfig().WorkerCount
	}
	if config.QueueSize <= 0 {
		config.QueueSize = DefaultConfig().QueueSize
	}
	if config.DefaultTimeout <= 0 {
		config.DefaultTimeout = DefaultConfig().DefaultTimeout
	}

	return &Service{
		config:     config,
		repo:       repo,
		ruleEngine: NewRuleEngine(config.ThrottleDuration),
		channels:   make(map[uuid.UUID]Channel),
		queue:      make(chan *notificationJob, config.QueueSize),
		logger:     logger.With("component", "notification_service"),
	}
}

// Start starts the notification service background workers.
func (s *Service) Start(ctx context.Context) error {
	s.startMu.Lock()
	defer s.startMu.Unlock()

	if s.started {
		return fmt.Errorf("service already started")
	}

	ctx, cancel := context.WithCancel(ctx)
	s.cancel = cancel

	// Start worker goroutines
	for i := 0; i < s.config.WorkerCount; i++ {
		s.wg.Add(1)
		go s.worker(ctx, i)
	}

	// Start throttle cache cleanup goroutine
	s.wg.Add(1)
	go s.throttleCleaner(ctx)

	// Load channels from database
	if err := s.loadChannels(ctx); err != nil {
		s.logger.Warn("failed to load channels on startup", "error", err)
	}

	s.started = true
	s.logger.Info("notification service started",
		"workers", s.config.WorkerCount,
		"queue_size", s.config.QueueSize,
	)

	return nil
}

// Stop gracefully stops the notification service.
func (s *Service) Stop(ctx context.Context) error {
	s.startMu.Lock()
	defer s.startMu.Unlock()

	if !s.started {
		return nil
	}

	s.logger.Info("stopping notification service")

	// Signal workers to stop
	if s.cancel != nil {
		s.cancel()
	}

	// Wait for workers with timeout
	done := make(chan struct{})
	go func() {
		s.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		s.logger.Info("notification service stopped gracefully")
	case <-ctx.Done():
		s.logger.Warn("notification service stop timed out")
		return ctx.Err()
	}

	s.started = false
	return nil
}

// worker processes notification jobs from the queue.
func (s *Service) worker(ctx context.Context, id int) {
	defer s.wg.Done()

	s.logger.Debug("notification worker started", "worker_id", id)

	for {
		select {
		case <-ctx.Done():
			s.logger.Debug("notification worker stopping", "worker_id", id)
			return
		case job, ok := <-s.queue:
			if !ok {
				return
			}
			s.processJob(ctx, job)
		}
	}
}

// processJob processes a single notification job.
func (s *Service) processJob(ctx context.Context, job *notificationJob) {
	start := time.Now()

	// Create timeout context
	sendCtx, cancel := context.WithTimeout(ctx, s.config.DefaultTimeout)
	defer cancel()

	result := SendResult{
		ChannelID:   job.channelID,
		ChannelType: job.channel.Type(),
		SentAt:      start,
	}

	err := job.channel.Send(sendCtx, job.notification)
	result.LatencyMs = time.Since(start).Milliseconds()

	if err != nil {
		result.Success = false
		result.Error = err.Error()
		s.logger.Error("failed to send notification",
			"channel_id", job.channelID,
			"channel_type", job.channel.Type(),
			"notification_type", job.notification.Type,
			"error", err,
		)
	} else {
		result.Success = true
		s.logger.Info("notification sent",
			"channel_id", job.channelID,
			"channel_type", job.channel.Type(),
			"notification_type", job.notification.Type,
			"latency_ms", result.LatencyMs,
		)
	}

	// Send result if channel provided
	if job.resultCh != nil {
		select {
		case job.resultCh <- result:
		default:
			// Result channel full, skip
		}
	}
}

// throttleCleaner periodically cleans up the throttle cache.
func (s *Service) throttleCleaner(ctx context.Context) {
	defer s.wg.Done()

	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.ruleEngine.CleanupThrottleCache()
		}
	}
}

// loadChannels loads all enabled channels from the database.
func (s *Service) loadChannels(ctx context.Context) error {
	dbChannels, err := s.repo.ListEnabledChannels(ctx)
	if err != nil {
		return fmt.Errorf("failed to list channels: %w", err)
	}

	s.channelsMu.Lock()
	defer s.channelsMu.Unlock()

	for _, dbChannel := range dbChannels {
		channel, err := s.createChannelFromDB(&dbChannel)
		if err != nil {
			s.logger.Warn("failed to create channel",
				"channel_id", dbChannel.ID,
				"channel_type", dbChannel.Type,
				"error", err,
			)
			continue
		}
		s.channels[dbChannel.ID] = channel
	}

	s.logger.Info("loaded notification channels", "count", len(s.channels))
	return nil
}

// createChannelFromDB creates a Channel implementation from a database model.
func (s *Service) createChannelFromDB(dbChannel *database.NotificationChannel) (Channel, error) {
	switch dbChannel.Type {
	case database.ChannelTypeSlack:
		var cfg database.SlackChannelConfig
		if err := json.Unmarshal(dbChannel.Config, &cfg); err != nil {
			return nil, fmt.Errorf("failed to parse slack config: %w", err)
		}
		return NewSlackChannel(SlackConfig{
			WebhookURL: cfg.WebhookURL,
			Channel:    cfg.Channel,
			Username:   cfg.Username,
			IconEmoji:  cfg.IconEmoji,
			Token:      cfg.Token,
		}, s.logger), nil

	case database.ChannelTypeEmail:
		var cfg database.EmailChannelConfig
		if err := json.Unmarshal(dbChannel.Config, &cfg); err != nil {
			return nil, fmt.Errorf("failed to parse email config: %w", err)
		}
		if s.config.Email.SMTPHost == "" {
			return nil, fmt.Errorf("email channel requires SMTP configuration")
		}

		emailConfig := EmailConfig{
			SMTPHost:    s.config.Email.SMTPHost,
			SMTPPort:    s.config.Email.SMTPPort,
			Username:    s.config.Email.Username,
			Password:    s.config.Email.Password,
			FromAddress: s.config.Email.FromAddress,
			FromName:    s.config.Email.FromName,
			Recipients:  cfg.Recipients,
			CC:          cfg.CC,
			UseTLS:      s.config.Email.UseTLS,
			SkipVerify:  s.config.Email.SkipVerify,
			IncludeLogs: cfg.IncludeLogs,
			ConnTimeout: s.config.Email.ConnTimeout,
		}

		return NewEmailChannel(emailConfig, s.logger)

	case database.ChannelTypeWebhook:
		var cfg database.WebhookChannelConfig
		if err := json.Unmarshal(dbChannel.Config, &cfg); err != nil {
			return nil, fmt.Errorf("failed to parse webhook config: %w", err)
		}
		return NewWebhookChannel(WebhookConfig{
			URL:     cfg.URL,
			Headers: cfg.Headers,
		}, s.logger), nil

	case database.ChannelTypeTeams:
		var cfg struct {
			WebhookURL string `json:"webhook_url"`
		}
		if err := json.Unmarshal(dbChannel.Config, &cfg); err != nil {
			return nil, fmt.Errorf("failed to parse teams config: %w", err)
		}
		return NewTeamsChannel(TeamsConfig{
			WebhookURL: cfg.WebhookURL,
		}, s.logger), nil

	default:
		return nil, fmt.Errorf("unsupported channel type: %s", dbChannel.Type)
	}
}

// SendNotification sends a notification through appropriate channels.
func (s *Service) SendNotification(ctx context.Context, event *Event) error {
	results, err := s.ProcessRules(ctx, event)
	if err != nil {
		return err
	}

	// Check if any sends failed
	for _, result := range results {
		if !result.Success {
			s.logger.Warn("notification send failed",
				"channel_id", result.ChannelID,
				"error", result.Error,
			)
		}
	}

	return nil
}

// ProcessRules evaluates notification rules and sends matching notifications.
func (s *Service) ProcessRules(ctx context.Context, event *Event) ([]SendResult, error) {
	// Get rules for this service
	rules, err := s.repo.ListRulesByService(ctx, event.ServiceID)
	if err != nil {
		return nil, fmt.Errorf("failed to list rules: %w", err)
	}

	if len(rules) == 0 {
		return nil, nil
	}

	// Build channel map
	s.channelsMu.RLock()
	channelMap := make(map[uuid.UUID]*database.NotificationChannel)
	for _, rule := range rules {
		if _, exists := s.channels[rule.ChannelID]; exists {
			// Get DB channel for rule matching
			dbChannel, err := s.repo.GetChannel(ctx, rule.ChannelID)
			if err == nil && dbChannel != nil {
				channelMap[rule.ChannelID] = dbChannel
			}
		}
	}
	s.channelsMu.RUnlock()

	// Evaluate rules
	matches := s.ruleEngine.Evaluate(rules, channelMap, event)
	if len(matches) == 0 {
		return nil, nil
	}

	// Create notification from event
	notification := s.createNotificationFromEvent(event)

	// Send to all matched channels
	resultCh := make(chan SendResult, len(matches))
	for _, match := range matches {
		s.channelsMu.RLock()
		channel, exists := s.channels[match.Channel.ID]
		s.channelsMu.RUnlock()

		if !exists {
			continue
		}

		job := &notificationJob{
			notification: notification,
			channel:      channel,
			channelID:    match.Channel.ID,
			resultCh:     resultCh,
		}

		select {
		case s.queue <- job:
			// Mark as sent for throttling
			s.ruleEngine.MarkSent(match.Rule.ID, event)
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
			s.logger.Warn("notification queue full, dropping notification",
				"channel_id", match.Channel.ID,
			)
		}
	}

	// Collect results
	results := make([]SendResult, 0, len(matches))
	timeout := time.After(s.config.DefaultTimeout)

	for i := 0; i < len(matches); i++ {
		select {
		case result := <-resultCh:
			results = append(results, result)
		case <-timeout:
			s.logger.Warn("timeout waiting for notification results")
			return results, nil
		case <-ctx.Done():
			return results, ctx.Err()
		}
	}

	return results, nil
}

// createNotificationFromEvent creates a Notification from an Event.
func (s *Service) createNotificationFromEvent(event *Event) *Notification {
	vars := TemplateVars{
		ServiceName: event.ServiceName,
		ServiceID:   event.ServiceID.String(),
		Timestamp:   event.Timestamp,
	}

	if event.Run != nil {
		vars.RunID = event.Run.ID.String()
		vars.TotalTests = event.Run.TotalTests
		vars.PassedTests = event.Run.PassedTests
		vars.FailedTests = event.Run.FailedTests
		vars.SkippedTests = event.Run.SkippedTests
		if event.Run.DurationMs != nil {
			vars.DurationMs = *event.Run.DurationMs
		}
		if event.Run.GitRef != nil {
			vars.Branch = *event.Run.GitRef
		}
		if event.Run.GitSHA != nil {
			vars.CommitSHA = *event.Run.GitSHA
		}
		if event.Run.ErrorMessage != nil {
			vars.ErrorMessage = *event.Run.ErrorMessage
		}
	}

	title, message := GetTemplateForType(event.Type, vars)

	notification := &Notification{
		ID:          uuid.New(),
		Type:        event.Type,
		ServiceID:   &event.ServiceID,
		ServiceName: event.ServiceName,
		RunID:       event.RunID,
		Title:       title,
		Message:     message,
		CreatedAt:   time.Now(),
		Metadata:    event.Metadata,
	}

	// Add summary if run data is available
	if event.Run != nil {
		notification.Summary = &RunSummary{
			TotalTests:   event.Run.TotalTests,
			PassedTests:  event.Run.PassedTests,
			FailedTests:  event.Run.FailedTests,
			SkippedTests: event.Run.SkippedTests,
		}
		if event.Run.DurationMs != nil {
			notification.Summary.DurationMs = *event.Run.DurationMs
		}
		if event.Run.GitRef != nil {
			notification.Summary.Branch = *event.Run.GitRef
		}
		if event.Run.GitSHA != nil {
			notification.Summary.CommitSHA = *event.Run.GitSHA
		}
		if event.Run.ErrorMessage != nil {
			notification.Summary.ErrorMessage = *event.Run.ErrorMessage
		}
	}

	// Generate URL if base URL is configured
	if s.config.BaseURL != "" && event.RunID != nil {
		notification.URL = fmt.Sprintf("%s/runs/%s", s.config.BaseURL, event.RunID.String())
	}

	return notification
}

// TestChannel sends a test notification to a specific channel.
func (s *Service) TestChannel(ctx context.Context, channelID uuid.UUID, message string) (*SendResult, error) {
	// Get channel from database
	dbChannel, err := s.repo.GetChannel(ctx, channelID)
	if err != nil {
		return nil, fmt.Errorf("failed to get channel: %w", err)
	}

	// Create channel implementation
	channel, err := s.createChannelFromDB(dbChannel)
	if err != nil {
		return nil, fmt.Errorf("failed to create channel: %w", err)
	}

	// Create test notification
	if message == "" {
		message = "This is a test notification from Conductor."
	}

	title, msg := TestNotificationTemplate(dbChannel.Name)
	if message != "" {
		msg = message
	}

	notification := &Notification{
		ID:          uuid.New(),
		Type:        NotificationTypeTest,
		Title:       title,
		Message:     msg,
		ServiceName: "Conductor",
		CreatedAt:   time.Now(),
	}

	// Send directly (bypass queue for immediate feedback)
	start := time.Now()
	err = channel.Send(ctx, notification)
	latency := time.Since(start).Milliseconds()

	result := &SendResult{
		ChannelID:   channelID,
		ChannelType: channel.Type(),
		LatencyMs:   latency,
		SentAt:      start,
	}

	if err != nil {
		result.Success = false
		result.Error = err.Error()
	} else {
		result.Success = true
	}

	return result, nil
}

// RefreshChannel reloads a channel from the database.
func (s *Service) RefreshChannel(ctx context.Context, channelID uuid.UUID) error {
	dbChannel, err := s.repo.GetChannel(ctx, channelID)
	if err != nil {
		if database.IsNotFound(err) {
			// Channel was deleted, remove from cache
			s.channelsMu.Lock()
			delete(s.channels, channelID)
			s.channelsMu.Unlock()
			return nil
		}
		return fmt.Errorf("failed to get channel: %w", err)
	}

	if !dbChannel.Enabled {
		// Channel was disabled, remove from cache
		s.channelsMu.Lock()
		delete(s.channels, channelID)
		s.channelsMu.Unlock()
		return nil
	}

	channel, err := s.createChannelFromDB(dbChannel)
	if err != nil {
		return fmt.Errorf("failed to create channel: %w", err)
	}

	s.channelsMu.Lock()
	s.channels[channelID] = channel
	s.channelsMu.Unlock()

	return nil
}

// RemoveChannel removes a channel from the cache.
func (s *Service) RemoveChannel(channelID uuid.UUID) {
	s.channelsMu.Lock()
	delete(s.channels, channelID)
	s.channelsMu.Unlock()
}
