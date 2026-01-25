-- Conductor Test Orchestration Platform - Scheduled Runs Rollback

-- Drop trigger first
DROP TRIGGER IF EXISTS update_scheduled_runs_updated_at ON scheduled_runs;

-- Drop table
DROP TABLE IF EXISTS scheduled_runs;
