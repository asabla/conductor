-- Conductor Test Orchestration Platform - Initial Schema
-- This migration creates the core tables for the test orchestration system

-- Enable UUID extension if not already enabled
CREATE EXTENSION IF NOT EXISTS "pgcrypto";

-- ============================================================================
-- SERVICES TABLE
-- Represents registered microservices that have tests to run
-- ============================================================================
CREATE TABLE services (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name VARCHAR(255) UNIQUE NOT NULL,
    display_name VARCHAR(255),
    git_url VARCHAR(512) NOT NULL,
    git_provider VARCHAR(50), -- github, gitlab, bitbucket
    default_branch VARCHAR(255) DEFAULT 'main',
    network_zones TEXT[], -- Array of zone names where tests can run
    owner VARCHAR(255), -- Team or individual owner
    contact_slack VARCHAR(255), -- Slack channel or user for notifications
    contact_email VARCHAR(255),
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW() NOT NULL,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW() NOT NULL
);

-- Indexes for common queries
CREATE INDEX idx_services_name ON services(name);
CREATE INDEX idx_services_git_url ON services(git_url);

COMMENT ON TABLE services IS 'Registered microservices with test suites';
COMMENT ON COLUMN services.network_zones IS 'Zones where agents can execute tests for this service';
COMMENT ON COLUMN services.git_provider IS 'Git hosting provider: github, gitlab, bitbucket';

-- ============================================================================
-- TEST_DEFINITIONS TABLE
-- Defines individual tests or test suites that can be executed
-- ============================================================================
CREATE TABLE test_definitions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    service_id UUID NOT NULL REFERENCES services(id) ON DELETE CASCADE,
    name VARCHAR(255) NOT NULL,
    description TEXT,
    execution_type VARCHAR(50) NOT NULL, -- subprocess, container
    command VARCHAR(1024) NOT NULL, -- Command to execute
    args TEXT[], -- Command arguments
    timeout_seconds INTEGER DEFAULT 1800, -- 30 minutes default
    result_file VARCHAR(512), -- Path to result file (e.g., test-results.xml)
    result_format VARCHAR(50), -- junit, jest, playwright, go_test, tap, json
    artifact_patterns TEXT[], -- Glob patterns for artifacts to collect
    tags TEXT[], -- Tags for filtering and grouping
    depends_on TEXT[], -- Test names this test depends on
    retries INTEGER DEFAULT 0, -- Number of retry attempts on failure
    allow_failure BOOLEAN DEFAULT false, -- If true, failure doesn't fail the run
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW() NOT NULL,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW() NOT NULL,
    CONSTRAINT uq_test_definitions_service_name UNIQUE (service_id, name)
);

-- Indexes for common queries
CREATE INDEX idx_test_definitions_service_id ON test_definitions(service_id);
CREATE INDEX idx_test_definitions_tags ON test_definitions USING GIN(tags);

COMMENT ON TABLE test_definitions IS 'Test definitions discovered from service repositories';
COMMENT ON COLUMN test_definitions.execution_type IS 'How to run: subprocess (direct) or container (Docker)';
COMMENT ON COLUMN test_definitions.result_format IS 'Output format: junit, jest, playwright, go_test, tap, json';
COMMENT ON COLUMN test_definitions.depends_on IS 'Other test names that must pass before this test runs';
COMMENT ON COLUMN test_definitions.allow_failure IS 'If true, test failure is recorded but does not fail the overall run';

-- ============================================================================
-- AGENTS TABLE
-- Tracks registered agents that execute tests in private networks
-- ============================================================================
CREATE TABLE agents (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name VARCHAR(255) NOT NULL,
    status VARCHAR(50) NOT NULL DEFAULT 'offline', -- idle, busy, draining, offline
    version VARCHAR(50), -- Agent software version
    network_zones TEXT[], -- Zones this agent can reach
    max_parallel INTEGER DEFAULT 4, -- Maximum concurrent test executions
    docker_available BOOLEAN DEFAULT false, -- Whether Docker is available
    last_heartbeat TIMESTAMP WITH TIME ZONE,
    registered_at TIMESTAMP WITH TIME ZONE DEFAULT NOW() NOT NULL
);

-- Indexes for common queries
CREATE INDEX idx_agents_status ON agents(status);
CREATE INDEX idx_agents_network_zones ON agents USING GIN(network_zones);

COMMENT ON TABLE agents IS 'Registered test execution agents';
COMMENT ON COLUMN agents.status IS 'Current state: idle, busy, draining, offline';
COMMENT ON COLUMN agents.network_zones IS 'Network zones this agent has access to';
COMMENT ON COLUMN agents.last_heartbeat IS 'Last heartbeat received; offline after 90s';

-- ============================================================================
-- TEST_RUNS TABLE
-- Records each test execution run
-- ============================================================================
CREATE TABLE test_runs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    service_id UUID NOT NULL REFERENCES services(id) ON DELETE CASCADE,
    agent_id UUID REFERENCES agents(id) ON DELETE SET NULL,
    status VARCHAR(50) NOT NULL DEFAULT 'pending', -- pending, running, passed, failed, error, timeout, cancelled
    git_ref VARCHAR(255), -- Branch, tag, or ref
    git_sha VARCHAR(64), -- Full commit SHA
    trigger_type VARCHAR(50), -- manual, webhook, schedule
    triggered_by VARCHAR(255), -- User or system that triggered
    priority INTEGER DEFAULT 0, -- Higher priority runs first
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW() NOT NULL,
    started_at TIMESTAMP WITH TIME ZONE,
    finished_at TIMESTAMP WITH TIME ZONE,
    total_tests INTEGER DEFAULT 0,
    passed_tests INTEGER DEFAULT 0,
    failed_tests INTEGER DEFAULT 0,
    skipped_tests INTEGER DEFAULT 0,
    duration_ms BIGINT, -- Total run duration in milliseconds
    error_message TEXT -- Error message if status is 'error'
);

-- Indexes for common queries
CREATE INDEX idx_test_runs_service_id ON test_runs(service_id);
CREATE INDEX idx_test_runs_agent_id ON test_runs(agent_id);
CREATE INDEX idx_test_runs_status ON test_runs(status);
CREATE INDEX idx_test_runs_created_at ON test_runs(created_at DESC);
CREATE INDEX idx_test_runs_git_sha ON test_runs(git_sha);

-- Composite index for dashboard queries (recent runs by status)
CREATE INDEX idx_test_runs_service_status_created ON test_runs(service_id, status, created_at DESC);

COMMENT ON TABLE test_runs IS 'Individual test execution runs';
COMMENT ON COLUMN test_runs.status IS 'Run status: pending, running, passed, failed, error, timeout, cancelled';
COMMENT ON COLUMN test_runs.trigger_type IS 'What triggered this run: manual, webhook, schedule';
COMMENT ON COLUMN test_runs.priority IS 'Scheduling priority; higher values run first';

-- ============================================================================
-- TEST_RESULTS TABLE
-- Stores individual test case results within a run
-- ============================================================================
CREATE TABLE test_results (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    run_id UUID NOT NULL REFERENCES test_runs(id) ON DELETE CASCADE,
    test_definition_id UUID REFERENCES test_definitions(id) ON DELETE SET NULL,
    test_name VARCHAR(512) NOT NULL, -- Full test name/path
    suite_name VARCHAR(255), -- Test suite or class name
    status VARCHAR(50) NOT NULL, -- pass, fail, skip, error
    duration_ms BIGINT, -- Test duration in milliseconds
    error_message TEXT, -- Failure message
    stack_trace TEXT, -- Stack trace on failure
    stdout TEXT, -- Captured stdout
    stderr TEXT, -- Captured stderr
    retry_count INTEGER DEFAULT 0, -- Which retry attempt this was
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW() NOT NULL
);

-- Indexes for common queries
CREATE INDEX idx_test_results_run_id ON test_results(run_id);
CREATE INDEX idx_test_results_status ON test_results(status);
CREATE INDEX idx_test_results_test_name ON test_results(test_name);

-- Composite index for finding failures in a run
CREATE INDEX idx_test_results_run_status ON test_results(run_id, status);

COMMENT ON TABLE test_results IS 'Individual test case results from parsed output';
COMMENT ON COLUMN test_results.status IS 'Test outcome: pass, fail, skip, error';
COMMENT ON COLUMN test_results.retry_count IS 'Retry attempt number (0 = first attempt)';

-- ============================================================================
-- ARTIFACTS TABLE
-- Tracks test artifacts stored in S3/MinIO
-- ============================================================================
CREATE TABLE artifacts (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    run_id UUID NOT NULL REFERENCES test_runs(id) ON DELETE CASCADE,
    name VARCHAR(255) NOT NULL, -- Display name
    path VARCHAR(1024) NOT NULL, -- S3 object path
    content_type VARCHAR(255), -- MIME type
    size_bytes BIGINT, -- File size
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW() NOT NULL
);

-- Index for listing artifacts by run
CREATE INDEX idx_artifacts_run_id ON artifacts(run_id);

COMMENT ON TABLE artifacts IS 'Test artifacts (logs, screenshots, etc.) stored in S3';
COMMENT ON COLUMN artifacts.path IS 'S3/MinIO object path';

-- ============================================================================
-- TRIGGER FOR UPDATED_AT
-- Automatically updates updated_at timestamp on row modification
-- ============================================================================
CREATE OR REPLACE FUNCTION update_updated_at_column()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ language 'plpgsql';

CREATE TRIGGER update_services_updated_at
    BEFORE UPDATE ON services
    FOR EACH ROW
    EXECUTE FUNCTION update_updated_at_column();

CREATE TRIGGER update_test_definitions_updated_at
    BEFORE UPDATE ON test_definitions
    FOR EACH ROW
    EXECUTE FUNCTION update_updated_at_column();
