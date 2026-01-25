-- Rollback shard tracking additions

DROP INDEX IF EXISTS idx_test_results_shard_id;
ALTER TABLE test_results DROP COLUMN IF EXISTS shard_id;

ALTER TABLE test_runs
    DROP COLUMN IF EXISTS max_parallel_tests,
    DROP COLUMN IF EXISTS shards_failed,
    DROP COLUMN IF EXISTS shards_completed,
    DROP COLUMN IF EXISTS shard_count;

DROP INDEX IF EXISTS idx_run_shards_status;
DROP INDEX IF EXISTS idx_run_shards_run_id;
DROP TABLE IF EXISTS run_shards;
