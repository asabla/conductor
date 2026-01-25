# Dashboard

## Overview

The dashboard provides visibility into test execution and system health. It serves multiple user personas with different needs: developers checking their test results, platform engineers managing the infrastructure, and managers tracking quality metrics.

## User Personas

### Developer

**Goals**:
- See if my tests passed
- Debug failures quickly
- Understand test history for my service

**Key workflows**:
- Check status of recent commit
- View failure details and artifacts
- Rerun failed tests

### Platform Engineer

**Goals**:
- Monitor system health
- Manage agent fleet
- Configure services and integrations

**Key workflows**:
- Monitor agent status
- Drain/restart agents
- Configure notifications
- Troubleshoot infrastructure issues

### Engineering Manager

**Goals**:
- Track quality trends
- Identify problematic areas
- Report on testing metrics

**Key workflows**:
- View pass rate trends
- Identify flaky tests
- Generate reports

## Views and Features

### 1. Home / Overview

**Purpose**: At-a-glance system status.

**Content**:
- Active test runs (count, progress indicators)
- Recent completions (last 10, with status)
- Agent status summary (X connected, Y busy, Z offline)
- Queue depth (pending test runs)
- System alerts (if any)

**Refresh**: Real-time via WebSocket.

### 2. Test Runs List

**Purpose**: Browse and filter test runs.

**Content**:
- Table/list of test runs
- Columns: ID, service, ref, status, duration, triggered by, started at
- Status indicators: running (with progress), passed, failed, error

**Filtering**:
- By service
- By status (running, passed, failed, all)
- By branch/tag
- By date range
- By trigger type (webhook, scheduled, manual)

**Actions**:
- Click to view details
- Rerun button
- Cancel button (for running)

**Pagination**: Cursor-based for large result sets.

### 3. Test Run Details

**Purpose**: Deep dive into a single test run.

**Header**:
- Run ID
- Service name (link to service view)
- Git ref (branch/tag, commit SHA, link to commit)
- Overall status
- Duration
- Triggered by (webhook event, user, schedule)
- Agent that executed

**Test Results Section**:
- List of all tests in the run
- For each test: name, status, duration
- Expand to see: error message, stack trace, logs
- Filter by status (show only failures)

**Artifacts Section**:
- List of collected artifacts
- Preview for images (screenshots)
- Download links for all
- Inline log viewer

**Timeline Section**:
- Visual timeline of test execution
- When each test started/ended
- Useful for understanding parallelism and bottlenecks

**Actions**:
- Rerun entire run
- Rerun failed tests only
- Download all artifacts as zip
- Copy link to run

### 4. Service View

**Purpose**: Health and history for a specific service.

**Header**:
- Service name
- Owner/team
- Repository link
- Network zone(s)

**Health Summary**:
- Pass rate (last 7 days, last 30 days)
- Trend indicator (improving, declining, stable)
- Last run status and time

**Recent Runs**:
- List of recent test runs for this service
- Same columns as runs list, filtered to service

**Test Breakdown**:
- List of tests defined for this service
- For each test: name, recent pass rate, average duration
- Identify flaky tests (high variance in pass/fail)

**Trends Charts**:
- Pass rate over time (line chart)
- Test duration over time (line chart)
- Failure distribution by test (bar chart)

### 5. Agents View

**Purpose**: Fleet management and monitoring.

**Agent List**:
- Table of all agents
- Columns: name/ID, status, network zone, version, last heartbeat, current work

**Status indicators**:
- Connected/Idle (green)
- Busy (blue, with run link)
- Draining (yellow)
- Offline (red)

**Agent Details** (click to expand or separate page):
- Full capabilities (runtimes, resources)
- Resource usage (CPU, memory, disk)
- Recent execution history
- Logs (if available)

**Actions**:
- Drain agent (stop accepting work)
- Resume agent (after drain)
- View agent logs
- Refresh status

**Fleet Summary**:
- Total agents by status
- Agents by network zone
- Version distribution
- Resource utilization across fleet

### 6. Failure Analysis

**Purpose**: Help debug and triage failures.

**Failure Groups**:
- Group failures by error signature (similar stack traces)
- Show count of occurrences
- Show affected services/tests
- First seen / last seen

**For each failure group**:
- Representative error message
- Representative stack trace
- List of affected test runs (links)
- Trend: increasing, decreasing, stable

**Filters**:
- Date range
- Service
- Test name
- Severity (error vs. assertion failure)

**Triage actions**:
- Mark as known issue (link to ticket)
- Mark as flaky
- Mark as resolved

### 7. Flaky Tests

**Purpose**: Identify and track unreliable tests.

**Detection criteria**:
- Tests that flip between pass and fail
- Configurable threshold (e.g., >10% flip rate in last 7 days)

**Flaky Test List**:
- Test name, service
- Flakiness score (percentage of flips)
- Recent results (visual: pass/fail sequence)

**Actions**:
- Quarantine test (run but don't fail pipeline)
- Link to issue tracker
- View test history

### 8. Trends and Analytics

**Purpose**: Long-term quality metrics.

**Organization-wide metrics**:
- Overall pass rate trend
- Total test runs per day/week
- Mean time to fix failing tests
- Test suite duration trends

**Breakdowns**:
- By team/owner
- By service
- By test type (unit, integration, e2e)

**Charts**:
- Pass rate over time (line)
- Test volume over time (bar)
- Duration distribution (histogram)
- Failure causes (pie chart)

**Export**:
- Download CSV of metrics
- Schedule email reports

### 9. Configuration

**Purpose**: Manage harness settings.

**Sections**:

**Git Providers**:
- List configured providers
- Add/edit/remove provider connections
- Test connection button
- Sync status

**Registered Services**:
- List of discovered services
- Manual registration option
- Force sync button
- Enable/disable service

**Notification Rules**:
- List of notification rules
- Add/edit/remove rules
- Rule conditions (failure, recovery, threshold)
- Destinations (Slack channel, email, webhook)

**Scheduled Runs**:
- List of scheduled test runs
- Add/edit/remove schedules
- Cron expression or simple scheduler
- Target services/tags

**User Management** (if applicable):
- List users
- Roles and permissions
- Invite users

### 10. Search

**Purpose**: Find anything quickly.

**Global search bar**:
- Search test runs by ID
- Search services by name
- Search tests by name
- Search commits by SHA

**Results grouped by type**.

## Real-Time Updates

The following should update in real-time without page refresh:

- Test run status and progress
- Agent status changes
- New test runs appearing
- Active run count on home page
- Results streaming in during execution

**Implementation**: WebSocket connection to control plane, subscribe to relevant channels.

## Authentication and Authorization

**Authentication**:
- SSO/OIDC integration
- Support common providers (Okta, Auth0, Google, etc.)

**Authorization levels**:
- Viewer: see everything, cannot modify
- Operator: can trigger runs, manage agents
- Admin: full configuration access

**Service-level permissions** (optional):
- Users can only see services they own
- Or: all services visible, but notifications scoped

## Performance Requirements

| Metric | Target |
|--------|--------|
| Page load time | < 2s |
| Real-time update latency | < 1s |
| Search response time | < 500ms |
| Support concurrent users | 100+ |

**Strategies**:
- Pagination for large lists
- Lazy loading for details
- Caching for expensive queries (trends)
- Efficient WebSocket fan-out

## Mobile Considerations

Dashboard should be usable on mobile devices:
- Responsive layout
- Key workflows accessible: check run status, view failures
- Not required: full configuration, complex filtering

## Accessibility

- Keyboard navigation
- Screen reader support
- Color-blind friendly status indicators
- Sufficient contrast ratios

## API for Dashboard

Dashboard consumes REST API from control plane.

**Key endpoints**:

| Endpoint | Purpose |
|----------|---------|
| `GET /api/runs` | List runs with filters |
| `GET /api/runs/{id}` | Run details |
| `GET /api/runs/{id}/results` | Test results for run |
| `GET /api/runs/{id}/artifacts` | Artifacts for run |
| `POST /api/runs` | Trigger new run |
| `POST /api/runs/{id}/cancel` | Cancel run |
| `GET /api/services` | List services |
| `GET /api/services/{id}` | Service details |
| `GET /api/services/{id}/runs` | Runs for service |
| `GET /api/services/{id}/stats` | Stats for service |
| `GET /api/agents` | List agents |
| `GET /api/agents/{id}` | Agent details |
| `POST /api/agents/{id}/drain` | Drain agent |
| `GET /api/stats/overview` | System overview stats |
| `GET /api/stats/trends` | Trend data |
| `WS /api/ws` | WebSocket for real-time updates |

## Technology Recommendations

| Component | Recommendation | Notes |
|-----------|----------------|-------|
| Framework | React or SvelteKit | Team familiarity with TypeScript |
| Routing | React Router or SvelteKit built-in | Standard approaches |
| State | TanStack Query | Caching, polling, real-time |
| Styling | Tailwind CSS | Utility-first, consistent |
| Components | shadcn/ui | Accessible, customizable |
| Charts | Recharts or Apache ECharts | Test trends, metrics |
| Real-time | Native WebSocket | Simple, sufficient |
| Testing | Vitest + Playwright | Unit + E2E for dashboard itself |

## Phased Delivery

**Phase 1 (MVP)**:
- Home overview
- Test runs list and details
- Basic service view
- Agent list

**Phase 2**:
- Failure analysis
- Flaky test detection
- Trends and analytics

**Phase 3**:
- Full configuration UI
- Advanced filtering
- Export and reports
- Mobile optimization
