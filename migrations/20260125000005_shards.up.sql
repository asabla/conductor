-- This migration adds shard tracking for test runs

-- ============================================================================
-- RUN_SHARDS TABLE
-- Tracks individual shards for a run
-- ============================================================================
CREATE TABLE run_shards (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    run_id UUID NOT NULL REFERENCES test_runs(id) ON DELETE CASCADE,
    shard_index INT NOT NULL,
    shard_count INT NOT NULL,
    status VARCHAR(50) NOT NULL DEFAULT 'pending',
    agent_id UUID REFERENCES agents(id) ON DELETE SET NULL,
    total_tests INT DEFAULT 0,
    passed_tests INT DEFAULT 0,
    failed_tests INT DEFAULT 0,
    skipped_tests INT DEFAULT 0,
    error_message TEXT,
    started_at TIMESTAMP WITH TIME ZONE,
    finished_at TIMESTAMP WITH TIME ZONE,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW() NOT NULL
);

CREATE INDEX idx_run_shards_run_id ON run_shards(run_id);
CREATE INDEX idx_run_shards_status ON run_shards(status);

COMMENT ON TABLE run_shards IS 'Shard tracking for parallelized test runs';
COMMENT ON COLUMN run_shards.shard_index IS 'Zero-based shard index';
COMMENT ON COLUMN run_shards.shard_count IS 'Total shards for run';

-- ============================================================================
-- TEST_RUNS ADDITIONS
-- Track sharding state at the run level
-- ============================================================================
ALTER TABLE test_runs
    ADD COLUMN shard_count INT NOT NULL DEFAULT 1,
    ADD COLUMN shards_completed INT NOT NULL DEFAULT 0,
    ADD COLUMN shards_failed INT NOT NULL DEFAULT 0,
    ADD COLUMN max_parallel_tests INT NOT NULL DEFAULT 0;

COMMENT ON COLUMN test_runs.shard_count IS 'Number of shards for this run';
COMMENT ON COLUMN test_runs.shards_completed IS 'Completed shards count';
COMMENT ON COLUMN test_runs.shards_failed IS 'Failed shards count';
COMMENT ON COLUMN test_runs.max_parallel_tests IS 'Max parallel tests per shard (0 = default)';

-- ============================================================================
-- TEST_RESULTS ADDITIONS
-- Attribute results to a shard
-- ============================================================================
ALTER TABLE test_results
    ADD COLUMN shard_id UUID REFERENCES run_shards(id) ON DELETE SET NULL;

CREATE INDEX idx_test_results_shard_id ON test_results(shard_id);
