-- Conductor Test Orchestration Platform - Scheduled Runs
-- This migration creates tables for scheduled/recurring test runs

-- ============================================================================
-- SCHEDULED_RUNS TABLE
-- Defines recurring test run schedules using cron expressions
-- ============================================================================
CREATE TABLE scheduled_runs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    service_id UUID NOT NULL REFERENCES services(id) ON DELETE CASCADE,
    name VARCHAR(255) NOT NULL,
    cron_expression VARCHAR(100) NOT NULL, -- Standard cron format (minute hour day month weekday)
    git_ref VARCHAR(255) DEFAULT 'main', -- Branch or tag to test
    test_filter TEXT[], -- Specific tests or tags to run; NULL means all
    enabled BOOLEAN DEFAULT true,
    last_run_at TIMESTAMP WITH TIME ZONE, -- When this schedule last triggered
    next_run_at TIMESTAMP WITH TIME ZONE, -- Computed next run time
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW() NOT NULL,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW() NOT NULL
);

-- Indexes for scheduler queries
CREATE INDEX idx_scheduled_runs_service_id ON scheduled_runs(service_id);
CREATE INDEX idx_scheduled_runs_enabled ON scheduled_runs(enabled) WHERE enabled = true;
CREATE INDEX idx_scheduled_runs_next_run_at ON scheduled_runs(next_run_at) WHERE enabled = true;

COMMENT ON TABLE scheduled_runs IS 'Scheduled/recurring test run configurations';
COMMENT ON COLUMN scheduled_runs.cron_expression IS 'Cron expression: minute hour day month weekday (e.g., "0 */6 * * *" for every 6 hours)';
COMMENT ON COLUMN scheduled_runs.test_filter IS 'Test names or tags to include; NULL runs all tests';
COMMENT ON COLUMN scheduled_runs.next_run_at IS 'Computed by scheduler; used for efficient polling';

/*
Example cron expressions:
- "0 0 * * *"     - Daily at midnight
- "0 */6 * * *"   - Every 6 hours
- "0 9 * * 1-5"   - Weekdays at 9 AM
- "*/30 * * * *"  - Every 30 minutes
- "0 0 * * 0"     - Weekly on Sunday at midnight
*/

-- ============================================================================
-- TRIGGER FOR UPDATED_AT
-- ============================================================================
CREATE TRIGGER update_scheduled_runs_updated_at
    BEFORE UPDATE ON scheduled_runs
    FOR EACH ROW
    EXECUTE FUNCTION update_updated_at_column();
