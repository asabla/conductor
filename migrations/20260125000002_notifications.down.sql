-- Conductor Test Orchestration Platform - Notification Tables Rollback

-- Drop triggers first
DROP TRIGGER IF EXISTS update_notification_rules_updated_at ON notification_rules;
DROP TRIGGER IF EXISTS update_notification_channels_updated_at ON notification_channels;

-- Drop tables in reverse order
DROP TABLE IF EXISTS notification_rules;
DROP TABLE IF EXISTS notification_channels;
