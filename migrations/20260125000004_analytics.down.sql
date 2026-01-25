-- Conductor Test Orchestration Platform - Analytics Tables Rollback

-- Drop view first
DROP VIEW IF EXISTS service_health_summary;

-- Drop function
DROP FUNCTION IF EXISTS calculate_flakiness_score(UUID, VARCHAR, INTEGER);

-- Drop tables in reverse order
DROP TABLE IF EXISTS test_history;
DROP TABLE IF EXISTS flaky_tests;
DROP TABLE IF EXISTS daily_stats;
