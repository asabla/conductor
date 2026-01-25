# Test Harness System Overview

## What Is This System?

A **distributed test orchestration platform** that coordinates test execution across polyglot microservices and frontend applications. The harness itself does not contain test logic—it discovers, schedules, executes, and reports on tests that are written natively in each service's language and framework.

## Problem Statement

The organization has:
- Multiple services written in JavaScript, TypeScript, Python, Java, and Go
- Tests spread across many Git repositories
- Systems deployed on private networks inaccessible from the public internet
- Multiple testing strategies: unit tests, E2E browser tests (Playwright), API tests, and simulation/chaos testing

There is no unified way to:
- Discover what tests exist across the organization
- Execute tests against systems on isolated networks
- Aggregate results into a single view
- Track test health and trends over time

## Solution Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                     CONTROL PLANE                           │
│  ┌─────────────┐ ┌─────────────┐ ┌─────────────────────────┐│
│  │  Scheduler  │ │  Registry   │ │   Result Aggregator     ││
│  └─────────────┘ └─────────────┘ └─────────────────────────┘│
│  ┌─────────────┐ ┌─────────────┐ ┌─────────────────────────┐│
│  │ Git Provider│ │  Dashboard  │ │   Notification Service  ││
│  │  Abstraction│ │     API     │ │                         ││
│  └─────────────┘ └─────────────┘ └─────────────────────────┘│
└───────────────────────────┬─────────────────────────────────┘
                            │
              gRPC (agents connect outbound)
                            │
        ┌───────────────────┼───────────────────┐
        │                   │                   │
        ▼                   ▼                   ▼
┌──────────────┐    ┌──────────────┐    ┌──────────────┐
│    Agent     │    │    Agent     │    │    Agent     │
│  (Zone A)    │    │  (Zone B)    │    │  (Zone C)    │
└──────┬───────┘    └──────┬───────┘    └──────┬───────┘
       │                   │                   │
┌──────┴───────┐    ┌──────┴───────┐    ┌──────┴───────┐
│Internal      │    │Internal      │    │Internal      │
│Services      │    │Services      │    │Services      │
└──────────────┘    └──────────────┘    └──────────────┘
```

## Core Components

### Control Plane

The central coordination service. Runs in a location accessible to all agents (e.g., a shared infrastructure zone or cloud).

**Key responsibilities:**
- Maintain the test registry (what tests exist, where they live)
- Schedule test runs based on triggers (CI webhooks, scheduled jobs, manual invocation)
- Accept connections from agents and dispatch work
- Collect and store test results
- Provide APIs for the dashboard and external integrations
- Manage Git provider connections for repository discovery

### Agents

Lightweight processes deployed inside private networks. Each agent can reach a specific set of internal services.

**Key responsibilities:**
- Connect outbound to the control plane (works through firewalls/NAT)
- Accept test execution requests
- Clone repositories and execute tests (via subprocess or container)
- Stream results back to control plane
- Report health and capability information

### Git Provider Abstraction

A pluggable interface for interacting with Git hosting platforms (GitHub, GitLab, Bitbucket, etc.).

**Key responsibilities:**
- Discover repositories containing test manifests
- Fetch test configuration files at specific refs
- Receive webhook events for CI triggers
- Provide clone URLs for agents

### Dashboard

A web interface for visibility into test execution and system health.

**Key features:**
- Real-time view of running tests
- Historical results and trend analysis
- Failure investigation tools
- Agent fleet management
- Configuration management

## How It Works

### Test Registration

1. Each repository contains a `.testharness.yaml` manifest declaring its tests
2. Control plane periodically scans registered repositories (via Git provider abstraction)
3. Manifests are parsed and stored in the test registry
4. Changes to manifests are detected and the registry is updated

### Test Execution Flow

1. **Trigger**: A test run is initiated (CI webhook, schedule, or manual)
2. **Discovery**: Control plane identifies which tests to run based on the trigger
3. **Scheduling**: Tests are assigned to agents based on network zone requirements and agent availability
4. **Dispatch**: Control plane sends work to agents over the gRPC stream
5. **Execution**: Agent clones the repo, runs the test command (subprocess or container)
6. **Collection**: Agent streams results back to control plane
7. **Aggregation**: Control plane normalizes results and stores them
8. **Reporting**: Dashboard updates, notifications are sent if configured

### Execution Modes

Tests can be executed in two modes:

**Subprocess**: Agent runs the test command directly on its host
- Faster startup
- Simpler debugging
- Requires runtime dependencies on the agent

**Container**: Agent runs tests inside a Docker container
- Isolated environment
- Reproducible dependencies
- Slower startup, more resource overhead

The mode is specified per-test in the manifest.

## Key Design Decisions

### Agents Connect Outbound

Agents initiate connections to the control plane rather than the reverse. This means:
- No inbound firewall rules required in private networks
- Works through NAT and restrictive network policies
- Control plane doesn't need to know agent IP addresses

### Language Agnostic

The harness doesn't care what language tests are written in. It invokes test runners via CLI and expects results in a standardized format. This means:
- Teams can use their preferred testing frameworks
- No changes required to existing test code (only adding a manifest)
- New languages/frameworks can be added without modifying the harness

### Git Provider Abstraction

The system is not coupled to any specific Git hosting platform. A clean interface allows:
- Starting with GitHub, migrating to GitLab later
- Supporting multiple providers simultaneously
- Self-hosted and cloud-hosted repositories

## Related Documents

- [01-ARCHITECTURE.md](./01-ARCHITECTURE.md) - Detailed component architecture
- [02-CONTROL-PLANE.md](./02-CONTROL-PLANE.md) - Control plane implementation details
- [03-AGENTS.md](./03-AGENTS.md) - Agent implementation details
- [04-GIT-INTEGRATION.md](./04-GIT-INTEGRATION.md) - Git provider abstraction
- [05-EXECUTION-PROTOCOL.md](./05-EXECUTION-PROTOCOL.md) - Agent-control plane communication
- [06-TEST-REGISTRY.md](./06-TEST-REGISTRY.md) - Test discovery and registration
- [07-DASHBOARD.md](./07-DASHBOARD.md) - Dashboard requirements and features
- [08-DATA-MODEL.md](./08-DATA-MODEL.md) - Database schema and data structures
- [09-DEPLOYMENT.md](./09-DEPLOYMENT.md) - Deployment considerations

## Glossary

| Term | Definition |
|------|------------|
| **Control Plane** | Central service that coordinates all test execution |
| **Agent** | Process running in a private network that executes tests |
| **Manifest** | YAML file in a repository declaring available tests |
| **Test Run** | A single invocation of one or more tests |
| **Network Zone** | A logical grouping of services that an agent can reach |
| **Git Provider** | The platform hosting Git repositories (GitHub, GitLab, etc.) |
