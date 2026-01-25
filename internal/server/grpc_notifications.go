package server

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	conductorv1 "github.com/conductor/conductor/api/gen/conductor/v1"
	"github.com/conductor/conductor/internal/database"
	"github.com/conductor/conductor/internal/notification"
)

// NotificationServiceDeps defines the dependencies for the notification service.
type NotificationServiceDeps struct {
	// Repo handles notification persistence.
	Repo database.NotificationRepository
	// NotificationService handles sending notifications.
	NotificationService notification.NotificationService
}

// NotificationServiceServer implements the NotificationService gRPC service.
type NotificationServiceServer struct {
	conductorv1.UnimplementedNotificationServiceServer

	deps   NotificationServiceDeps
	logger zerolog.Logger
}

// NewNotificationServiceServer creates a new notification service server.
func NewNotificationServiceServer(deps NotificationServiceDeps, logger zerolog.Logger) *NotificationServiceServer {
	return &NotificationServiceServer{
		deps:   deps,
		logger: logger.With().Str("service", "NotificationService").Logger(),
	}
}

// CreateChannel creates a new notification channel.
func (s *NotificationServiceServer) CreateChannel(ctx context.Context, req *conductorv1.CreateChannelRequest) (*conductorv1.CreateChannelResponse, error) {
	if req.Name == "" {
		return nil, status.Error(codes.InvalidArgument, "name is required")
	}
	if req.Type == conductorv1.ChannelType_CHANNEL_TYPE_UNSPECIFIED {
		return nil, status.Error(codes.InvalidArgument, "type is required")
	}

	// Validate and convert config
	config, err := channelConfigToJSON(req.Type, req.Config)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid config: %v", err)
	}

	channel := &database.NotificationChannel{
		Name:      req.Name,
		Type:      channelTypeFromProto(req.Type),
		Config:    config,
		Enabled:   req.Enabled,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	if err := s.deps.Repo.CreateChannel(ctx, channel); err != nil {
		s.logger.Error().Err(err).Str("name", req.Name).Msg("failed to create channel")
		return nil, status.Errorf(codes.Internal, "failed to create channel: %v", err)
	}

	s.logger.Info().
		Str("channel_id", channel.ID.String()).
		Str("name", channel.Name).
		Str("type", string(channel.Type)).
		Msg("notification channel created")

	return &conductorv1.CreateChannelResponse{
		Channel: channelToProto(channel),
	}, nil
}

// GetChannel retrieves a notification channel by ID.
func (s *NotificationServiceServer) GetChannel(ctx context.Context, req *conductorv1.GetChannelRequest) (*conductorv1.GetChannelResponse, error) {
	channelID, err := uuid.Parse(req.ChannelId)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid channel ID: %v", err)
	}

	channel, err := s.deps.Repo.GetChannel(ctx, channelID)
	if err != nil {
		if database.IsNotFound(err) {
			return nil, status.Errorf(codes.NotFound, "channel not found: %s", req.ChannelId)
		}
		return nil, status.Errorf(codes.Internal, "failed to get channel: %v", err)
	}

	return &conductorv1.GetChannelResponse{
		Channel: channelToProto(channel),
	}, nil
}

// ListChannels returns all notification channels.
func (s *NotificationServiceServer) ListChannels(ctx context.Context, req *conductorv1.ListChannelsRequest) (*conductorv1.ListChannelsResponse, error) {
	pagination := paginationFromProto(req.Pagination)

	channels, err := s.deps.Repo.ListChannels(ctx, pagination)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to list channels: %v", err)
	}

	// Filter by type if specified
	var filteredChannels []database.NotificationChannel
	if req.Type != conductorv1.ChannelType_CHANNEL_TYPE_UNSPECIFIED {
		requestedType := channelTypeFromProto(req.Type)
		for _, ch := range channels {
			if ch.Type == requestedType {
				filteredChannels = append(filteredChannels, ch)
			}
		}
	} else {
		filteredChannels = channels
	}

	// Filter by enabled status if specified
	if req.Enabled != nil {
		var enabledFiltered []database.NotificationChannel
		for _, ch := range filteredChannels {
			if ch.Enabled == *req.Enabled {
				enabledFiltered = append(enabledFiltered, ch)
			}
		}
		filteredChannels = enabledFiltered
	}

	protoChannels := make([]*conductorv1.NotificationChannel, len(filteredChannels))
	for i, ch := range filteredChannels {
		protoChannels[i] = channelToProto(&ch)
	}

	return &conductorv1.ListChannelsResponse{
		Channels:   protoChannels,
		Pagination: paginationResponseToProto(pagination, len(filteredChannels)),
	}, nil
}

// UpdateChannel updates a notification channel.
func (s *NotificationServiceServer) UpdateChannel(ctx context.Context, req *conductorv1.UpdateChannelRequest) (*conductorv1.UpdateChannelResponse, error) {
	channelID, err := uuid.Parse(req.ChannelId)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid channel ID: %v", err)
	}

	channel, err := s.deps.Repo.GetChannel(ctx, channelID)
	if err != nil {
		if database.IsNotFound(err) {
			return nil, status.Errorf(codes.NotFound, "channel not found: %s", req.ChannelId)
		}
		return nil, status.Errorf(codes.Internal, "failed to get channel: %v", err)
	}

	// Apply updates
	if req.Name != nil {
		channel.Name = *req.Name
	}
	if req.Config != nil {
		config, err := channelConfigToJSON(channelTypeToProto(channel.Type), req.Config)
		if err != nil {
			return nil, status.Errorf(codes.InvalidArgument, "invalid config: %v", err)
		}
		channel.Config = config
	}
	if req.Enabled != nil {
		channel.Enabled = *req.Enabled
	}

	channel.UpdatedAt = time.Now()

	if err := s.deps.Repo.UpdateChannel(ctx, channel); err != nil {
		return nil, status.Errorf(codes.Internal, "failed to update channel: %v", err)
	}

	// Refresh channel in notification service
	if s.deps.NotificationService != nil {
		if svc, ok := s.deps.NotificationService.(*notification.Service); ok {
			if err := svc.RefreshChannel(ctx, channelID); err != nil {
				s.logger.Warn().Err(err).Str("channel_id", channelID.String()).Msg("failed to refresh channel")
			}
		}
	}

	s.logger.Info().
		Str("channel_id", channelID.String()).
		Msg("notification channel updated")

	return &conductorv1.UpdateChannelResponse{
		Channel: channelToProto(channel),
	}, nil
}

// DeleteChannel removes a notification channel.
func (s *NotificationServiceServer) DeleteChannel(ctx context.Context, req *conductorv1.DeleteChannelRequest) (*conductorv1.DeleteChannelResponse, error) {
	channelID, err := uuid.Parse(req.ChannelId)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid channel ID: %v", err)
	}

	if err := s.deps.Repo.DeleteChannel(ctx, channelID); err != nil {
		if database.IsNotFound(err) {
			return nil, status.Errorf(codes.NotFound, "channel not found: %s", req.ChannelId)
		}
		return nil, status.Errorf(codes.Internal, "failed to delete channel: %v", err)
	}

	// Remove channel from notification service
	if s.deps.NotificationService != nil {
		if svc, ok := s.deps.NotificationService.(*notification.Service); ok {
			svc.RemoveChannel(channelID)
		}
	}

	s.logger.Info().
		Str("channel_id", channelID.String()).
		Msg("notification channel deleted")

	return &conductorv1.DeleteChannelResponse{
		Success: true,
	}, nil
}

// TestChannel sends a test notification to verify channel configuration.
func (s *NotificationServiceServer) TestChannel(ctx context.Context, req *conductorv1.TestChannelRequest) (*conductorv1.TestChannelResponse, error) {
	channelID, err := uuid.Parse(req.ChannelId)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid channel ID: %v", err)
	}

	if s.deps.NotificationService == nil {
		return nil, status.Error(codes.Unavailable, "notification service not available")
	}

	result, err := s.deps.NotificationService.TestChannel(ctx, channelID, req.Message)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to test channel: %v", err)
	}

	return &conductorv1.TestChannelResponse{
		Success:      result.Success,
		ErrorMessage: result.Error,
		LatencyMs:    result.LatencyMs,
	}, nil
}

// CreateRule creates a new notification rule.
func (s *NotificationServiceServer) CreateRule(ctx context.Context, req *conductorv1.CreateRuleRequest) (*conductorv1.CreateRuleResponse, error) {
	if req.Name == "" {
		return nil, status.Error(codes.InvalidArgument, "name is required")
	}
	if len(req.ChannelIds) == 0 {
		return nil, status.Error(codes.InvalidArgument, "at least one channel ID is required")
	}
	if len(req.Events) == 0 {
		return nil, status.Error(codes.InvalidArgument, "at least one event is required")
	}

	// Verify channel exists
	channelID, err := uuid.Parse(req.ChannelIds[0])
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid channel ID: %v", err)
	}

	_, err = s.deps.Repo.GetChannel(ctx, channelID)
	if err != nil {
		if database.IsNotFound(err) {
			return nil, status.Errorf(codes.NotFound, "channel not found: %s", req.ChannelIds[0])
		}
		return nil, status.Errorf(codes.Internal, "failed to verify channel: %v", err)
	}

	// Parse service ID filter if provided
	var serviceID *uuid.UUID
	if req.Filter != nil && len(req.Filter.ServiceIds) > 0 {
		id, err := uuid.Parse(req.Filter.ServiceIds[0])
		if err != nil {
			return nil, status.Errorf(codes.InvalidArgument, "invalid service ID: %v", err)
		}
		serviceID = &id
	}

	// Convert events to trigger events
	triggerOn := make([]database.TriggerEvent, 0, len(req.Events))
	for _, event := range req.Events {
		triggerOn = append(triggerOn, notificationEventToTrigger(event))
	}

	rule := &database.NotificationRule{
		ChannelID: channelID,
		ServiceID: serviceID,
		TriggerOn: triggerOn,
		Enabled:   req.Enabled,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	if err := s.deps.Repo.CreateRule(ctx, rule); err != nil {
		s.logger.Error().Err(err).Str("name", req.Name).Msg("failed to create rule")
		return nil, status.Errorf(codes.Internal, "failed to create rule: %v", err)
	}

	s.logger.Info().
		Str("rule_id", rule.ID.String()).
		Str("channel_id", channelID.String()).
		Msg("notification rule created")

	return &conductorv1.CreateRuleResponse{
		Rule: ruleToProto(rule, req.Name),
	}, nil
}

// GetRule retrieves a notification rule by ID.
func (s *NotificationServiceServer) GetRule(ctx context.Context, req *conductorv1.GetRuleRequest) (*conductorv1.GetRuleResponse, error) {
	ruleID, err := uuid.Parse(req.RuleId)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid rule ID: %v", err)
	}

	rule, err := s.deps.Repo.GetRule(ctx, ruleID)
	if err != nil {
		if database.IsNotFound(err) {
			return nil, status.Errorf(codes.NotFound, "rule not found: %s", req.RuleId)
		}
		return nil, status.Errorf(codes.Internal, "failed to get rule: %v", err)
	}

	return &conductorv1.GetRuleResponse{
		Rule: ruleToProto(rule, ""),
	}, nil
}

// ListRules returns all notification rules.
func (s *NotificationServiceServer) ListRules(ctx context.Context, req *conductorv1.ListRulesRequest) (*conductorv1.ListRulesResponse, error) {
	var rules []database.NotificationRule
	var err error

	if req.ServiceId != "" {
		serviceID, parseErr := uuid.Parse(req.ServiceId)
		if parseErr != nil {
			return nil, status.Errorf(codes.InvalidArgument, "invalid service ID: %v", parseErr)
		}
		rules, err = s.deps.Repo.ListRulesByService(ctx, serviceID)
	} else {
		// List all rules - need to get by all channels
		// For now, use a reasonable default
		pagination := database.DefaultPagination()
		pagination.Limit = 100
		channels, chErr := s.deps.Repo.ListChannels(ctx, pagination)
		if chErr != nil {
			return nil, status.Errorf(codes.Internal, "failed to list channels: %v", chErr)
		}

		ruleSet := make(map[uuid.UUID]database.NotificationRule)
		for _, ch := range channels {
			channelRules, ruleErr := s.deps.Repo.ListRulesByChannel(ctx, ch.ID)
			if ruleErr != nil {
				continue
			}
			for _, r := range channelRules {
				ruleSet[r.ID] = r
			}
		}

		for _, r := range ruleSet {
			rules = append(rules, r)
		}
	}

	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to list rules: %v", err)
	}

	// Filter by enabled status if specified
	if req.Enabled != nil {
		var filtered []database.NotificationRule
		for _, r := range rules {
			if r.Enabled == *req.Enabled {
				filtered = append(filtered, r)
			}
		}
		rules = filtered
	}

	protoRules := make([]*conductorv1.NotificationRule, len(rules))
	for i, r := range rules {
		protoRules[i] = ruleToProto(&r, "")
	}

	return &conductorv1.ListRulesResponse{
		Rules:      protoRules,
		Pagination: paginationResponseToProto(database.DefaultPagination(), len(rules)),
	}, nil
}

// UpdateRule updates a notification rule.
func (s *NotificationServiceServer) UpdateRule(ctx context.Context, req *conductorv1.UpdateRuleRequest) (*conductorv1.UpdateRuleResponse, error) {
	ruleID, err := uuid.Parse(req.RuleId)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid rule ID: %v", err)
	}

	rule, err := s.deps.Repo.GetRule(ctx, ruleID)
	if err != nil {
		if database.IsNotFound(err) {
			return nil, status.Errorf(codes.NotFound, "rule not found: %s", req.RuleId)
		}
		return nil, status.Errorf(codes.Internal, "failed to get rule: %v", err)
	}

	// Apply updates
	if len(req.ChannelIds) > 0 {
		channelID, err := uuid.Parse(req.ChannelIds[0])
		if err != nil {
			return nil, status.Errorf(codes.InvalidArgument, "invalid channel ID: %v", err)
		}
		rule.ChannelID = channelID
	}

	if len(req.Events) > 0 {
		triggerOn := make([]database.TriggerEvent, 0, len(req.Events))
		for _, event := range req.Events {
			triggerOn = append(triggerOn, notificationEventToTrigger(event))
		}
		rule.TriggerOn = triggerOn
	}

	if req.Filter != nil && len(req.Filter.ServiceIds) > 0 {
		serviceID, err := uuid.Parse(req.Filter.ServiceIds[0])
		if err != nil {
			return nil, status.Errorf(codes.InvalidArgument, "invalid service ID: %v", err)
		}
		rule.ServiceID = &serviceID
	}

	if req.Enabled != nil {
		rule.Enabled = *req.Enabled
	}

	rule.UpdatedAt = time.Now()

	if err := s.deps.Repo.UpdateRule(ctx, rule); err != nil {
		return nil, status.Errorf(codes.Internal, "failed to update rule: %v", err)
	}

	s.logger.Info().
		Str("rule_id", ruleID.String()).
		Msg("notification rule updated")

	name := ""
	if req.Name != nil {
		name = *req.Name
	}

	return &conductorv1.UpdateRuleResponse{
		Rule: ruleToProto(rule, name),
	}, nil
}

// DeleteRule removes a notification rule.
func (s *NotificationServiceServer) DeleteRule(ctx context.Context, req *conductorv1.DeleteRuleRequest) (*conductorv1.DeleteRuleResponse, error) {
	ruleID, err := uuid.Parse(req.RuleId)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid rule ID: %v", err)
	}

	if err := s.deps.Repo.DeleteRule(ctx, ruleID); err != nil {
		if database.IsNotFound(err) {
			return nil, status.Errorf(codes.NotFound, "rule not found: %s", req.RuleId)
		}
		return nil, status.Errorf(codes.Internal, "failed to delete rule: %v", err)
	}

	s.logger.Info().
		Str("rule_id", ruleID.String()).
		Msg("notification rule deleted")

	return &conductorv1.DeleteRuleResponse{
		Success: true,
	}, nil
}

// ListNotificationHistory returns recent notifications sent.
func (s *NotificationServiceServer) ListNotificationHistory(ctx context.Context, req *conductorv1.ListNotificationHistoryRequest) (*conductorv1.ListNotificationHistoryResponse, error) {
	// TODO: Implement notification history tracking
	return &conductorv1.ListNotificationHistoryResponse{
		Records:    []*conductorv1.NotificationRecord{},
		Pagination: paginationResponseToProto(database.DefaultPagination(), 0),
	}, nil
}

// Helper functions

func channelToProto(channel *database.NotificationChannel) *conductorv1.NotificationChannel {
	if channel == nil {
		return nil
	}

	return &conductorv1.NotificationChannel{
		Id:        channel.ID.String(),
		Name:      channel.Name,
		Type:      channelTypeToProto(channel.Type),
		Enabled:   channel.Enabled,
		Config:    channelConfigFromJSON(channel.Type, channel.Config),
		CreatedAt: timestamppb.New(channel.CreatedAt),
		UpdatedAt: timestamppb.New(channel.UpdatedAt),
	}
}

func channelTypeFromProto(t conductorv1.ChannelType) database.ChannelType {
	switch t {
	case conductorv1.ChannelType_CHANNEL_TYPE_SLACK:
		return database.ChannelTypeSlack
	case conductorv1.ChannelType_CHANNEL_TYPE_EMAIL:
		return database.ChannelTypeEmail
	case conductorv1.ChannelType_CHANNEL_TYPE_WEBHOOK:
		return database.ChannelTypeWebhook
	case conductorv1.ChannelType_CHANNEL_TYPE_TEAMS:
		return database.ChannelTypeTeams
	default:
		return database.ChannelTypeWebhook
	}
}

func channelTypeToProto(t database.ChannelType) conductorv1.ChannelType {
	switch t {
	case database.ChannelTypeSlack:
		return conductorv1.ChannelType_CHANNEL_TYPE_SLACK
	case database.ChannelTypeEmail:
		return conductorv1.ChannelType_CHANNEL_TYPE_EMAIL
	case database.ChannelTypeWebhook:
		return conductorv1.ChannelType_CHANNEL_TYPE_WEBHOOK
	case database.ChannelTypeTeams:
		return conductorv1.ChannelType_CHANNEL_TYPE_TEAMS
	default:
		return conductorv1.ChannelType_CHANNEL_TYPE_UNSPECIFIED
	}
}

func channelConfigToJSON(channelType conductorv1.ChannelType, config *conductorv1.ChannelConfig) (json.RawMessage, error) {
	if config == nil {
		return json.RawMessage("{}"), nil
	}

	var data interface{}
	switch channelType {
	case conductorv1.ChannelType_CHANNEL_TYPE_SLACK:
		if config.Slack != nil {
			data = database.SlackChannelConfig{
				WebhookURL: config.Slack.WebhookUrl,
				Channel:    config.Slack.Channel,
				Username:   config.Slack.Username,
				IconEmoji:  config.Slack.Icon,
				Token:      config.Slack.Token,
			}
		}
	case conductorv1.ChannelType_CHANNEL_TYPE_EMAIL:
		if config.Email != nil {
			data = database.EmailChannelConfig{
				Recipients: config.Email.To,
				CC:         config.Email.Cc,
			}
		}
	case conductorv1.ChannelType_CHANNEL_TYPE_WEBHOOK:
		if config.Webhook != nil {
			data = database.WebhookChannelConfig{
				URL:     config.Webhook.Url,
				Headers: config.Webhook.Headers,
			}
		}
	case conductorv1.ChannelType_CHANNEL_TYPE_TEAMS:
		if config.Teams != nil {
			data = map[string]string{
				"webhook_url": config.Teams.WebhookUrl,
			}
		}
	}

	if data == nil {
		return json.RawMessage("{}"), nil
	}

	return json.Marshal(data)
}

func channelConfigFromJSON(channelType database.ChannelType, raw json.RawMessage) *conductorv1.ChannelConfig {
	config := &conductorv1.ChannelConfig{}

	switch channelType {
	case database.ChannelTypeSlack:
		var cfg database.SlackChannelConfig
		if err := json.Unmarshal(raw, &cfg); err == nil {
			config.Slack = &conductorv1.SlackConfig{
				WebhookUrl: cfg.WebhookURL,
				Channel:    cfg.Channel,
				Username:   cfg.Username,
				Icon:       cfg.IconEmoji,
				Token:      cfg.Token,
			}
		}
	case database.ChannelTypeEmail:
		var cfg database.EmailChannelConfig
		if err := json.Unmarshal(raw, &cfg); err == nil {
			config.Email = &conductorv1.EmailConfig{
				To: cfg.Recipients,
				Cc: cfg.CC,
			}
		}
	case database.ChannelTypeWebhook:
		var cfg database.WebhookChannelConfig
		if err := json.Unmarshal(raw, &cfg); err == nil {
			config.Webhook = &conductorv1.WebhookConfig{
				Url:     cfg.URL,
				Headers: cfg.Headers,
			}
		}
	case database.ChannelTypeTeams:
		var cfg map[string]string
		if err := json.Unmarshal(raw, &cfg); err == nil {
			config.Teams = &conductorv1.TeamsConfig{
				WebhookUrl: cfg["webhook_url"],
			}
		}
	}

	return config
}

func ruleToProto(rule *database.NotificationRule, name string) *conductorv1.NotificationRule {
	if rule == nil {
		return nil
	}

	protoRule := &conductorv1.NotificationRule{
		Id:         rule.ID.String(),
		Name:       name,
		Enabled:    rule.Enabled,
		ChannelIds: []string{rule.ChannelID.String()},
		Events:     make([]conductorv1.NotificationEvent, 0, len(rule.TriggerOn)),
		CreatedAt:  timestamppb.New(rule.CreatedAt),
		UpdatedAt:  timestamppb.New(rule.UpdatedAt),
	}

	for _, trigger := range rule.TriggerOn {
		protoRule.Events = append(protoRule.Events, triggerToNotificationEvent(trigger))
	}

	if rule.ServiceID != nil {
		protoRule.Filter = &conductorv1.NotificationFilter{
			ServiceIds: []string{rule.ServiceID.String()},
		}
	}

	return protoRule
}

func notificationEventToTrigger(event conductorv1.NotificationEvent) database.TriggerEvent {
	switch event {
	case conductorv1.NotificationEvent_NOTIFICATION_EVENT_RUN_FAILED,
		conductorv1.NotificationEvent_NOTIFICATION_EVENT_RUN_TIMEOUT,
		conductorv1.NotificationEvent_NOTIFICATION_EVENT_RUN_ERROR:
		return database.TriggerEventFailure
	case conductorv1.NotificationEvent_NOTIFICATION_EVENT_RUN_PASSED:
		return database.TriggerEventRecovery
	case conductorv1.NotificationEvent_NOTIFICATION_EVENT_FLAKY_TEST:
		return database.TriggerEventFlaky
	default:
		return database.TriggerEventAlways
	}
}

func triggerToNotificationEvent(trigger database.TriggerEvent) conductorv1.NotificationEvent {
	switch trigger {
	case database.TriggerEventFailure:
		return conductorv1.NotificationEvent_NOTIFICATION_EVENT_RUN_FAILED
	case database.TriggerEventRecovery:
		return conductorv1.NotificationEvent_NOTIFICATION_EVENT_RUN_PASSED
	case database.TriggerEventFlaky:
		return conductorv1.NotificationEvent_NOTIFICATION_EVENT_FLAKY_TEST
	case database.TriggerEventAlways:
		return conductorv1.NotificationEvent_NOTIFICATION_EVENT_RUN_COMPLETED
	default:
		return conductorv1.NotificationEvent_NOTIFICATION_EVENT_UNSPECIFIED
	}
}
