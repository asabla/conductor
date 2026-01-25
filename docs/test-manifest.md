# Test Manifest Reference

This document describes the `.testharness.yaml` manifest file format used to define tests for Conductor.

## Table of Contents

- [Overview](#overview)
- [Schema](#schema)
- [Field Reference](#field-reference)
- [Examples](#examples)
- [Variable Substitution](#variable-substitution)
- [Best Practices](#best-practices)

## Overview

Each repository that contains tests includes a `.testharness.yaml` file at the repository root. This manifest declares:

- Service metadata (name, owner, contact)
- Default configuration for all tests
- Individual test definitions
- Lifecycle hooks

## Schema

### Complete Schema

```yaml
version: "1"                          # Required: manifest version

service:                              # Required: service metadata
  name: string                        # Required: unique service identifier
  description: string                 # Optional: human-readable description
  owner: string                       # Optional: team or individual owner
  contact:                            # Optional: contact information
    email: string
    slack: string

defaults:                             # Optional: default values for tests
  execution_type: string              # "subprocess" or "container"
  timeout_seconds: integer            # default timeout
  retries: integer                    # default retry count
  container_image: string             # default container image
  working_directory: string           # default working directory
  environment:                        # default environment variables
    KEY: value

tests:                                # Required: list of test definitions
  - name: string                      # Required: unique test name
    description: string               # Optional: test description
    execution_type: string            # "subprocess" or "container"
    command: string                   # Required: command to run
    args: [string]                    # Optional: command arguments
    timeout_seconds: integer          # Optional: test timeout
    result_file: string               # Optional: path to result file
    result_format: string             # Optional: result format
    artifact_patterns: [string]       # Optional: artifact collection patterns
    tags: [string]                    # Optional: tags for filtering
    depends_on: [string]              # Optional: test dependencies
    retries: integer                  # Optional: retry count
    allow_failure: boolean            # Optional: allow failure
    container_image: string           # Optional: container image
    working_directory: string         # Optional: working directory
    environment:                      # Optional: environment variables
      KEY: value
    setup: [string]                   # Optional: setup commands
    teardown: [string]                # Optional: teardown commands

hooks:                                # Optional: lifecycle hooks
  before_all: [string]                # Run before any tests
  after_all: [string]                 # Run after all tests
  before_each: [string]               # Run before each test
  after_each: [string]                # Run after each test

variables:                            # Optional: custom variables
  KEY: value
```

## Field Reference

### version

**Required.** The manifest schema version.

```yaml
version: "1"
```

Supported versions: `"1"`, `"1.0"`

### service

**Required.** Service-level metadata.

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `name` | string | Yes | Unique service identifier |
| `description` | string | No | Human-readable description |
| `owner` | string | No | Team or individual owner |
| `contact.email` | string | No | Contact email |
| `contact.slack` | string | No | Slack channel |

### defaults

Optional default values applied to all tests.

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `execution_type` | string | `subprocess` | Default execution mode |
| `timeout_seconds` | integer | `1800` | Default timeout (30 minutes) |
| `retries` | integer | `0` | Default retry count |
| `container_image` | string | - | Default container image |
| `working_directory` | string | `.` | Default working directory |
| `environment` | map | - | Default environment variables |

### tests

**Required.** List of test definitions.

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `name` | string | Yes | Unique test name within service |
| `description` | string | No | Test description |
| `execution_type` | string | No | `subprocess` or `container` |
| `command` | string | Yes | Command to execute |
| `args` | list | No | Command arguments |
| `timeout_seconds` | integer | No | Test timeout in seconds |
| `result_file` | string | No | Path to result output file |
| `result_format` | string | No | Result file format |
| `artifact_patterns` | list | No | Glob patterns for artifacts |
| `tags` | list | No | Tags for filtering |
| `depends_on` | list | No | Names of dependent tests |
| `retries` | integer | No | Retry count for flaky tests |
| `allow_failure` | boolean | No | Don't fail run if test fails |
| `container_image` | string | No | Docker image for container mode |
| `working_directory` | string | No | Working directory (relative to repo) |
| `environment` | map | No | Environment variables |
| `setup` | list | No | Commands to run before test |
| `teardown` | list | No | Commands to run after test |

#### execution_type

- `subprocess` - Run command directly on the agent
- `container` - Run command in a Docker container

#### result_format

Supported formats:
- `junit` - JUnit XML format
- `jest` - Jest JSON format
- `playwright` - Playwright JSON format
- `go_test` - Go test JSON output
- `tap` - Test Anything Protocol
- `json` - Generic JSON format

### hooks

Optional lifecycle hooks.

| Field | Type | Description |
|-------|------|-------------|
| `before_all` | list | Commands to run before any tests |
| `after_all` | list | Commands to run after all tests |
| `before_each` | list | Commands to run before each test |
| `after_each` | list | Commands to run after each test |

## Examples

### Go Service

```yaml
version: "1"

service:
  name: user-service
  description: User management microservice
  owner: platform-team
  contact:
    email: platform@example.com
    slack: "#platform-alerts"

defaults:
  execution_type: subprocess
  timeout_seconds: 300
  environment:
    CGO_ENABLED: "0"

tests:
  - name: unit-tests
    description: Run unit tests
    command: go
    args: ["test", "-v", "-race", "-coverprofile=coverage.out", "./..."]
    result_format: go_test
    artifact_patterns:
      - "coverage.out"
    tags:
      - unit
      - fast

  - name: integration-tests
    description: Run integration tests with database
    command: go
    args: ["test", "-v", "-tags=integration", "./integration/..."]
    result_format: go_test
    timeout_seconds: 600
    environment:
      DATABASE_URL: "${secrets.TEST_DATABASE_URL}"
    tags:
      - integration
    depends_on:
      - unit-tests

  - name: lint
    description: Run linter
    command: golangci-lint
    args: ["run", "./..."]
    result_format: json
    tags:
      - lint
      - fast

hooks:
  before_all:
    - go mod download
```

### Node.js Service

```yaml
version: "1"

service:
  name: api-gateway
  description: API Gateway service
  owner: backend-team

defaults:
  execution_type: subprocess
  timeout_seconds: 300
  working_directory: .

tests:
  - name: unit-tests
    description: Jest unit tests
    command: npm
    args: ["test", "--", "--ci", "--coverage"]
    result_file: coverage/jest-results.json
    result_format: jest
    artifact_patterns:
      - "coverage/**"
    tags:
      - unit

  - name: lint
    description: ESLint check
    command: npm
    args: ["run", "lint"]
    tags:
      - lint
      - fast

  - name: type-check
    description: TypeScript type checking
    command: npm
    args: ["run", "typecheck"]
    tags:
      - types
      - fast

  - name: e2e-tests
    description: End-to-end tests
    execution_type: container
    container_image: mcr.microsoft.com/playwright:v1.40.0-focal
    command: npx
    args: ["playwright", "test"]
    result_file: playwright-report/results.json
    result_format: playwright
    timeout_seconds: 1800
    artifact_patterns:
      - "playwright-report/**"
      - "test-results/**"
    tags:
      - e2e
      - slow
    depends_on:
      - unit-tests

hooks:
  before_all:
    - npm ci
```

### Python Service

```yaml
version: "1"

service:
  name: ml-service
  description: Machine learning prediction service
  owner: ml-team
  contact:
    slack: "#ml-alerts"

defaults:
  execution_type: container
  container_image: python:3.11-slim
  timeout_seconds: 600

tests:
  - name: unit-tests
    description: pytest unit tests
    command: pytest
    args: ["tests/unit", "-v", "--junitxml=results/unit.xml"]
    result_file: results/unit.xml
    result_format: junit
    artifact_patterns:
      - "results/**"
    tags:
      - unit

  - name: integration-tests
    description: Integration tests with ML models
    command: pytest
    args: ["tests/integration", "-v", "--junitxml=results/integration.xml"]
    result_file: results/integration.xml
    result_format: junit
    timeout_seconds: 1200
    environment:
      MODEL_PATH: "/models/latest"
    tags:
      - integration
      - slow
    depends_on:
      - unit-tests

  - name: lint
    description: Ruff linter
    command: ruff
    args: ["check", "."]
    execution_type: subprocess
    tags:
      - lint
      - fast

  - name: type-check
    description: MyPy type checking
    command: mypy
    args: ["src/", "--strict"]
    execution_type: subprocess
    tags:
      - types
      - fast

hooks:
  before_all:
    - pip install -r requirements.txt
    - pip install -r requirements-dev.txt
```

### Multi-language Monorepo

```yaml
version: "1"

service:
  name: monorepo-tests
  description: Tests for monorepo services
  owner: platform-team

variables:
  NODE_VERSION: "20"
  GO_VERSION: "1.22"

tests:
  - name: frontend-unit
    description: Frontend unit tests
    working_directory: packages/frontend
    command: npm
    args: ["test", "--", "--ci"]
    result_format: jest
    tags:
      - frontend
      - unit

  - name: frontend-e2e
    description: Frontend E2E tests
    working_directory: packages/frontend
    execution_type: container
    container_image: mcr.microsoft.com/playwright:latest
    command: npx
    args: ["playwright", "test"]
    result_format: playwright
    artifact_patterns:
      - "packages/frontend/test-results/**"
    tags:
      - frontend
      - e2e
    depends_on:
      - frontend-unit

  - name: backend-unit
    description: Backend unit tests
    working_directory: services/api
    command: go
    args: ["test", "-v", "./..."]
    result_format: go_test
    tags:
      - backend
      - unit

  - name: backend-integration
    description: Backend integration tests
    working_directory: services/api
    command: go
    args: ["test", "-v", "-tags=integration", "./..."]
    result_format: go_test
    timeout_seconds: 900
    environment:
      DATABASE_URL: "${env.TEST_DATABASE_URL}"
    tags:
      - backend
      - integration
    depends_on:
      - backend-unit
```

## Variable Substitution

Manifests support variable substitution for dynamic values.

### Built-in Variables

| Variable | Description |
|----------|-------------|
| `${GIT_SHA}` | Full commit SHA being tested |
| `${GIT_SHORT_SHA}` | Short (7 char) commit SHA |
| `${GIT_REF}` | Branch or tag name |
| `${GIT_BRANCH}` | Branch name (empty for tags) |
| `${GIT_TAG}` | Tag name (empty for branches) |
| `${SERVICE_NAME}` | Service name from manifest |
| `${RUN_ID}` | Unique test run identifier |
| `${TIMESTAMP}` | ISO timestamp of run start |

### Secret Variables

Secrets are referenced but not stored in manifests:

```yaml
environment:
  DATABASE_URL: "${secrets.TEST_DATABASE_URL}"
  API_KEY: "${secrets.EXTERNAL_API_KEY}"
```

Secrets are:
- Stored in the control plane's secret store
- Injected at execution time
- Never logged or displayed

### Environment Variables

Reference environment variables provided by the control plane:

```yaml
environment:
  BASE_URL: "${env.BASE_URL}"
  DEPLOYMENT_ENV: "${env.DEPLOYMENT_ENV}"
```

### Custom Variables

Define custom variables in the manifest:

```yaml
variables:
  COVERAGE_THRESHOLD: "80"
  TEST_TIMEOUT: "300"

tests:
  - name: unit-tests
    command: npm
    args: ["test", "--coverage", "--coverageThreshold=${COVERAGE_THRESHOLD}"]
```

## Best Practices

### 1. Use Descriptive Names

```yaml
# Good
tests:
  - name: api-contract-tests
  - name: database-migration-tests
  - name: authentication-unit-tests

# Avoid
tests:
  - name: test1
  - name: tests
  - name: check
```

### 2. Set Appropriate Timeouts

```yaml
tests:
  - name: unit-tests
    timeout_seconds: 300      # 5 minutes for unit tests
    
  - name: e2e-tests
    timeout_seconds: 1800     # 30 minutes for E2E tests
    
  - name: load-tests
    timeout_seconds: 3600     # 1 hour for load tests
```

### 3. Use Tags for Filtering

```yaml
tests:
  - name: critical-path-tests
    tags:
      - critical
      - smoke
      - fast
      
  - name: full-regression
    tags:
      - regression
      - slow
      - nightly
```

Run specific tags:
```bash
conductor-ctl runs create --service my-service --tags critical,fast
```

### 4. Define Dependencies

```yaml
tests:
  - name: lint
    tags: [fast]
    
  - name: unit-tests
    depends_on: [lint]
    tags: [unit]
    
  - name: integration-tests
    depends_on: [unit-tests]
    tags: [integration]
    
  - name: e2e-tests
    depends_on: [integration-tests]
    tags: [e2e]
```

### 5. Collect Artifacts

```yaml
tests:
  - name: e2e-tests
    artifact_patterns:
      - "test-results/**/*.png"      # Screenshots
      - "test-results/**/*.webm"     # Videos
      - "playwright-report/**"       # HTML report
      - "coverage/**"                # Coverage reports
```

### 6. Handle Flaky Tests

```yaml
tests:
  - name: flaky-integration-test
    retries: 2                       # Retry up to 2 times
    
  - name: experimental-test
    allow_failure: true              # Don't fail the run
```

### 7. Use Defaults

```yaml
defaults:
  execution_type: subprocess
  timeout_seconds: 300
  environment:
    CI: "true"
    NODE_ENV: "test"

tests:
  - name: test-1    # Inherits defaults
  - name: test-2    # Inherits defaults
  - name: test-3
    timeout_seconds: 600    # Override default
```

### 8. Separate Concerns with Hooks

```yaml
hooks:
  before_all:
    - npm ci
    - npm run build
    
  after_all:
    - npm run cleanup

tests:
  - name: unit-tests
    # Tests run in clean environment
```
