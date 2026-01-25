-- =============================================================================
-- PostgreSQL Initialization Script for Conductor
-- =============================================================================
-- This script runs on first database initialization
-- It sets up extensions and initial schema requirements
-- =============================================================================

-- Enable useful extensions
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";      -- UUID generation
CREATE EXTENSION IF NOT EXISTS "pg_trgm";         -- Trigram matching for search
CREATE EXTENSION IF NOT EXISTS "btree_gin";       -- GIN index support

-- Create schemas for organization
CREATE SCHEMA IF NOT EXISTS conductor;

-- Set default search path
ALTER DATABASE conductor SET search_path TO conductor, public;

-- Create readonly role for reporting/analytics
DO $$
BEGIN
    IF NOT EXISTS (SELECT FROM pg_roles WHERE rolname = 'conductor_readonly') THEN
        CREATE ROLE conductor_readonly;
    END IF;
END
$$;

GRANT USAGE ON SCHEMA conductor TO conductor_readonly;
GRANT USAGE ON SCHEMA public TO conductor_readonly;

-- Grant default privileges for future tables
ALTER DEFAULT PRIVILEGES IN SCHEMA conductor 
    GRANT SELECT ON TABLES TO conductor_readonly;

-- Log successful initialization
DO $$
BEGIN
    RAISE NOTICE 'Conductor database initialized successfully';
END
$$;
