-- Conductor Test Orchestration Platform - Initial Schema Rollback
-- Drops all core tables in reverse dependency order

-- Drop triggers first
DROP TRIGGER IF EXISTS update_test_definitions_updated_at ON test_definitions;
DROP TRIGGER IF EXISTS update_services_updated_at ON services;
DROP FUNCTION IF EXISTS update_updated_at_column();

-- Drop tables in reverse order of creation (respecting foreign key dependencies)
DROP TABLE IF EXISTS artifacts;
DROP TABLE IF EXISTS test_results;
DROP TABLE IF EXISTS test_runs;
DROP TABLE IF EXISTS agents;
DROP TABLE IF EXISTS test_definitions;
DROP TABLE IF EXISTS services;

-- Note: We don't drop the pgcrypto extension as other schemas may depend on it
