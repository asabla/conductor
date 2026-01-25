-- Conductor Test Orchestration Platform - Notification Tables
-- This migration creates tables for notification channels and rules

-- ============================================================================
-- NOTIFICATION_CHANNELS TABLE
-- Defines notification destinations (Slack, email, webhooks, etc.)
-- ============================================================================
CREATE TABLE notification_channels (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name VARCHAR(255) NOT NULL,
    type VARCHAR(50) NOT NULL, -- slack, email, webhook, teams
    config JSONB NOT NULL DEFAULT '{}', -- Channel-specific configuration
    enabled BOOLEAN DEFAULT true,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW() NOT NULL,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW() NOT NULL
);

-- Index for listing enabled channels by type
CREATE INDEX idx_notification_channels_type ON notification_channels(type);
CREATE INDEX idx_notification_channels_enabled ON notification_channels(enabled) WHERE enabled = true;

COMMENT ON TABLE notification_channels IS 'Notification delivery channels';
COMMENT ON COLUMN notification_channels.type IS 'Channel type: slack, email, webhook, teams';
COMMENT ON COLUMN notification_channels.config IS 'Channel-specific config (webhook URL, email addresses, etc.)';

/*
Example config structures:

Slack:
{
    "webhook_url": "https://hooks.slack.com/services/...",
    "channel": "#test-results",
    "username": "Conductor Bot",
    "icon_emoji": ":test_tube:"
}

Email:
{
    "recipients": ["team@example.com"],
    "smtp_config_id": "default",
    "include_logs": true
}

Webhook:
{
    "url": "https://api.example.com/webhook",
    "method": "POST",
    "headers": {"Authorization": "Bearer ..."},
    "template": "default"
}

Teams:
{
    "webhook_url": "https://outlook.office.com/webhook/...",
    "title": "Test Results"
}
*/

-- ============================================================================
-- NOTIFICATION_RULES TABLE
-- Defines when and what notifications to send
-- ============================================================================
CREATE TABLE notification_rules (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    channel_id UUID NOT NULL REFERENCES notification_channels(id) ON DELETE CASCADE,
    service_id UUID REFERENCES services(id) ON DELETE CASCADE, -- NULL means all services
    trigger_on VARCHAR(50)[] NOT NULL, -- Array of triggers: failure, recovery, flaky, always
    enabled BOOLEAN DEFAULT true,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW() NOT NULL,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW() NOT NULL
);

-- Indexes for looking up rules
CREATE INDEX idx_notification_rules_channel_id ON notification_rules(channel_id);
CREATE INDEX idx_notification_rules_service_id ON notification_rules(service_id);
CREATE INDEX idx_notification_rules_enabled ON notification_rules(enabled) WHERE enabled = true;

COMMENT ON TABLE notification_rules IS 'Rules for when to send notifications';
COMMENT ON COLUMN notification_rules.service_id IS 'Service to notify for; NULL means all services';
COMMENT ON COLUMN notification_rules.trigger_on IS 'Events that trigger notification: failure, recovery, flaky, always';

-- ============================================================================
-- TRIGGERS FOR UPDATED_AT
-- ============================================================================
CREATE TRIGGER update_notification_channels_updated_at
    BEFORE UPDATE ON notification_channels
    FOR EACH ROW
    EXECUTE FUNCTION update_updated_at_column();

CREATE TRIGGER update_notification_rules_updated_at
    BEFORE UPDATE ON notification_rules
    FOR EACH ROW
    EXECUTE FUNCTION update_updated_at_column();
