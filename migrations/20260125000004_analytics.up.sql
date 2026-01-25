-- Conductor Test Orchestration Platform - Analytics Tables
-- This migration creates tables for aggregated statistics and flaky test tracking

-- ============================================================================
-- DAILY_STATS TABLE
-- Pre-aggregated daily statistics per service for dashboard performance
-- ============================================================================
CREATE TABLE daily_stats (
    id SERIAL PRIMARY KEY,
    service_id UUID NOT NULL REFERENCES services(id) ON DELETE CASCADE,
    date DATE NOT NULL,
    total_runs INTEGER DEFAULT 0,
    passed_runs INTEGER DEFAULT 0,
    failed_runs INTEGER DEFAULT 0,
    total_tests INTEGER DEFAULT 0,
    passed_tests INTEGER DEFAULT 0,
    failed_tests INTEGER DEFAULT 0,
    avg_duration_ms BIGINT, -- Average run duration
    p50_duration_ms BIGINT, -- Median run duration
    p95_duration_ms BIGINT, -- 95th percentile run duration
    CONSTRAINT uq_daily_stats_service_date UNIQUE (service_id, date)
);

-- Indexes for analytics queries
CREATE INDEX idx_daily_stats_date ON daily_stats(date);
CREATE INDEX idx_daily_stats_service_date ON daily_stats(service_id, date DESC);

COMMENT ON TABLE daily_stats IS 'Pre-aggregated daily statistics for dashboard performance';
COMMENT ON COLUMN daily_stats.avg_duration_ms IS 'Average test run duration in milliseconds';
COMMENT ON COLUMN daily_stats.p50_duration_ms IS 'Median (50th percentile) run duration';
COMMENT ON COLUMN daily_stats.p95_duration_ms IS '95th percentile run duration';

-- ============================================================================
-- FLAKY_TESTS TABLE
-- Tracks tests that exhibit flaky behavior (intermittent failures)
-- ============================================================================
CREATE TABLE flaky_tests (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    service_id UUID NOT NULL REFERENCES services(id) ON DELETE CASCADE,
    test_name VARCHAR(512) NOT NULL,
    flakiness_score FLOAT DEFAULT 0, -- 0 to 1, where 1 is always flaky
    total_runs INTEGER DEFAULT 0, -- Total times this test has run
    flaky_runs INTEGER DEFAULT 0, -- Times it showed flaky behavior
    first_detected_at TIMESTAMP WITH TIME ZONE DEFAULT NOW() NOT NULL,
    last_flaky_at TIMESTAMP WITH TIME ZONE, -- Most recent flaky occurrence
    quarantined BOOLEAN DEFAULT false, -- If true, failures don't fail the build
    quarantined_at TIMESTAMP WITH TIME ZONE,
    quarantined_by VARCHAR(255), -- User who quarantined
    notes TEXT, -- Notes about the flaky test
    CONSTRAINT uq_flaky_tests_service_test UNIQUE (service_id, test_name)
);

-- Indexes for flaky test queries
CREATE INDEX idx_flaky_tests_service_id ON flaky_tests(service_id);
CREATE INDEX idx_flaky_tests_flakiness_score ON flaky_tests(flakiness_score DESC);
CREATE INDEX idx_flaky_tests_quarantined ON flaky_tests(quarantined) WHERE quarantined = true;
CREATE INDEX idx_flaky_tests_last_flaky ON flaky_tests(last_flaky_at DESC);

COMMENT ON TABLE flaky_tests IS 'Tracks tests with intermittent/flaky behavior';
COMMENT ON COLUMN flaky_tests.flakiness_score IS 'Ratio of flaky runs to total runs (0-1)';
COMMENT ON COLUMN flaky_tests.quarantined IS 'If true, test failures are recorded but do not fail the overall run';

-- ============================================================================
-- TEST_HISTORY TABLE
-- Stores recent test execution history for trend analysis
-- Keeps a rolling window of results per test for flakiness detection
-- ============================================================================
CREATE TABLE test_history (
    id SERIAL PRIMARY KEY,
    service_id UUID NOT NULL REFERENCES services(id) ON DELETE CASCADE,
    test_name VARCHAR(512) NOT NULL,
    run_id UUID NOT NULL REFERENCES test_runs(id) ON DELETE CASCADE,
    status VARCHAR(50) NOT NULL, -- pass, fail, skip, error
    duration_ms BIGINT,
    executed_at TIMESTAMP WITH TIME ZONE DEFAULT NOW() NOT NULL
);

-- Indexes for history queries
CREATE INDEX idx_test_history_service_test ON test_history(service_id, test_name);
CREATE INDEX idx_test_history_run_id ON test_history(run_id);
CREATE INDEX idx_test_history_executed_at ON test_history(executed_at DESC);

-- Partial index for recent failures (useful for flakiness detection)
CREATE INDEX idx_test_history_recent_failures ON test_history(service_id, test_name, executed_at DESC)
    WHERE status IN ('fail', 'error');

COMMENT ON TABLE test_history IS 'Rolling history of test executions for trend analysis';
COMMENT ON COLUMN test_history.status IS 'Test outcome for this execution';

-- ============================================================================
-- FUNCTION: Calculate flakiness score
-- A test is considered flaky if it alternates between pass and fail
-- ============================================================================
CREATE OR REPLACE FUNCTION calculate_flakiness_score(
    p_service_id UUID,
    p_test_name VARCHAR,
    p_window_size INTEGER DEFAULT 20
) RETURNS FLOAT AS $$
DECLARE
    v_results TEXT[];
    v_transitions INTEGER := 0;
    v_total INTEGER;
    i INTEGER;
BEGIN
    -- Get recent results as an array
    SELECT ARRAY_AGG(status ORDER BY executed_at DESC)
    INTO v_results
    FROM (
        SELECT status, executed_at
        FROM test_history
        WHERE service_id = p_service_id
          AND test_name = p_test_name
        ORDER BY executed_at DESC
        LIMIT p_window_size
    ) recent;

    IF v_results IS NULL OR array_length(v_results, 1) < 2 THEN
        RETURN 0;
    END IF;

    v_total := array_length(v_results, 1);

    -- Count status transitions (pass->fail or fail->pass)
    FOR i IN 2..v_total LOOP
        IF (v_results[i-1] IN ('pass') AND v_results[i] IN ('fail', 'error'))
           OR (v_results[i-1] IN ('fail', 'error') AND v_results[i] IN ('pass')) THEN
            v_transitions := v_transitions + 1;
        END IF;
    END LOOP;

    -- Flakiness score is the ratio of transitions to possible transitions
    RETURN v_transitions::FLOAT / (v_total - 1)::FLOAT;
END;
$$ LANGUAGE plpgsql;

COMMENT ON FUNCTION calculate_flakiness_score IS 'Calculates flakiness score based on status transitions in recent history';

-- ============================================================================
-- VIEW: Service health summary
-- Provides a quick overview of service health metrics
-- ============================================================================
CREATE OR REPLACE VIEW service_health_summary AS
SELECT
    s.id AS service_id,
    s.name AS service_name,
    s.display_name,
    -- Recent run stats (last 7 days)
    COUNT(tr.id) FILTER (WHERE tr.created_at > NOW() - INTERVAL '7 days') AS runs_last_7_days,
    COUNT(tr.id) FILTER (WHERE tr.status = 'passed' AND tr.created_at > NOW() - INTERVAL '7 days') AS passed_last_7_days,
    COUNT(tr.id) FILTER (WHERE tr.status = 'failed' AND tr.created_at > NOW() - INTERVAL '7 days') AS failed_last_7_days,
    -- Pass rate
    CASE
        WHEN COUNT(tr.id) FILTER (WHERE tr.created_at > NOW() - INTERVAL '7 days') > 0
        THEN ROUND(
            100.0 * COUNT(tr.id) FILTER (WHERE tr.status = 'passed' AND tr.created_at > NOW() - INTERVAL '7 days') /
            COUNT(tr.id) FILTER (WHERE tr.created_at > NOW() - INTERVAL '7 days'),
            1
        )
        ELSE NULL
    END AS pass_rate_7_days,
    -- Flaky test count
    (SELECT COUNT(*) FROM flaky_tests ft WHERE ft.service_id = s.id AND ft.flakiness_score > 0.1) AS flaky_test_count,
    -- Most recent run
    MAX(tr.created_at) AS last_run_at,
    -- Most recent run status
    (SELECT status FROM test_runs WHERE service_id = s.id ORDER BY created_at DESC LIMIT 1) AS last_run_status
FROM services s
LEFT JOIN test_runs tr ON tr.service_id = s.id
GROUP BY s.id, s.name, s.display_name;

COMMENT ON VIEW service_health_summary IS 'Aggregated health metrics per service for dashboard';
