// Package testfixtures provides test fixtures and builders for integration tests.
package testfixtures

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/conductor/conductor/internal/database"
)

// ServiceBuilder helps construct test services with default values.
type ServiceBuilder struct {
	service *database.Service
}

// NewServiceBuilder creates a new service builder with default values.
func NewServiceBuilder() *ServiceBuilder {
	return &ServiceBuilder{
		service: &database.Service{
			Name:          fmt.Sprintf("test-service-%s", uuid.New().String()[:8]),
			GitURL:        "https://github.com/example/test-repo.git",
			DefaultBranch: "main",
			NetworkZones:  []string{"default"},
		},
	}
}

// WithName sets the service name.
func (b *ServiceBuilder) WithName(name string) *ServiceBuilder {
	b.service.Name = name
	return b
}

// WithDisplayName sets the display name.
func (b *ServiceBuilder) WithDisplayName(name string) *ServiceBuilder {
	b.service.DisplayName = &name
	return b
}

// WithGitURL sets the git URL.
func (b *ServiceBuilder) WithGitURL(url string) *ServiceBuilder {
	b.service.GitURL = url
	return b
}

// WithGitProvider sets the git provider.
func (b *ServiceBuilder) WithGitProvider(provider string) *ServiceBuilder {
	b.service.GitProvider = &provider
	return b
}

// WithDefaultBranch sets the default branch.
func (b *ServiceBuilder) WithDefaultBranch(branch string) *ServiceBuilder {
	b.service.DefaultBranch = branch
	return b
}

// WithNetworkZones sets the network zones.
func (b *ServiceBuilder) WithNetworkZones(zones []string) *ServiceBuilder {
	b.service.NetworkZones = zones
	return b
}

// WithOwner sets the owner.
func (b *ServiceBuilder) WithOwner(owner string) *ServiceBuilder {
	b.service.Owner = &owner
	return b
}

// WithContactSlack sets the Slack contact.
func (b *ServiceBuilder) WithContactSlack(channel string) *ServiceBuilder {
	b.service.ContactSlack = &channel
	return b
}

// WithContactEmail sets the email contact.
func (b *ServiceBuilder) WithContactEmail(email string) *ServiceBuilder {
	b.service.ContactEmail = &email
	return b
}

// Build returns the built service.
func (b *ServiceBuilder) Build() *database.Service {
	return b.service
}

// CreateService creates and persists a test service.
func CreateService(ctx context.Context, repo database.ServiceRepository, opts ...func(*ServiceBuilder)) (*database.Service, error) {
	builder := NewServiceBuilder()
	for _, opt := range opts {
		opt(builder)
	}
	svc := builder.Build()
	if err := repo.Create(ctx, svc); err != nil {
		return nil, err
	}
	return svc, nil
}

// TestRunBuilder helps construct test runs with default values.
type TestRunBuilder struct {
	run *database.TestRun
}

// NewTestRunBuilder creates a new test run builder with default values.
func NewTestRunBuilder(serviceID uuid.UUID) *TestRunBuilder {
	branch := "main"
	sha := "abc123def456"
	triggerType := database.TriggerTypeManual
	triggeredBy := "test-user"

	return &TestRunBuilder{
		run: &database.TestRun{
			ServiceID:   serviceID,
			Status:      database.RunStatusPending,
			GitRef:      &branch,
			GitSHA:      &sha,
			TriggerType: &triggerType,
			TriggeredBy: &triggeredBy,
			Priority:    0,
		},
	}
}

// WithStatus sets the run status.
func (b *TestRunBuilder) WithStatus(status database.RunStatus) *TestRunBuilder {
	b.run.Status = status
	return b
}

// WithAgent sets the agent ID.
func (b *TestRunBuilder) WithAgent(agentID uuid.UUID) *TestRunBuilder {
	b.run.AgentID = &agentID
	return b
}

// WithGitRef sets the git ref.
func (b *TestRunBuilder) WithGitRef(ref string) *TestRunBuilder {
	b.run.GitRef = &ref
	return b
}

// WithGitSHA sets the git SHA.
func (b *TestRunBuilder) WithGitSHA(sha string) *TestRunBuilder {
	b.run.GitSHA = &sha
	return b
}

// WithTriggerType sets the trigger type.
func (b *TestRunBuilder) WithTriggerType(triggerType database.TriggerType) *TestRunBuilder {
	b.run.TriggerType = &triggerType
	return b
}

// WithTriggeredBy sets who triggered the run.
func (b *TestRunBuilder) WithTriggeredBy(by string) *TestRunBuilder {
	b.run.TriggeredBy = &by
	return b
}

// WithPriority sets the priority.
func (b *TestRunBuilder) WithPriority(priority int) *TestRunBuilder {
	b.run.Priority = priority
	return b
}

// WithTestCounts sets the test count fields.
func (b *TestRunBuilder) WithTestCounts(total, passed, failed, skipped int) *TestRunBuilder {
	b.run.TotalTests = total
	b.run.PassedTests = passed
	b.run.FailedTests = failed
	b.run.SkippedTests = skipped
	return b
}

// WithDuration sets the duration in milliseconds.
func (b *TestRunBuilder) WithDuration(durationMs int64) *TestRunBuilder {
	b.run.DurationMs = &durationMs
	return b
}

// WithError sets the error message.
func (b *TestRunBuilder) WithError(msg string) *TestRunBuilder {
	b.run.ErrorMessage = &msg
	return b
}

// Build returns the built test run.
func (b *TestRunBuilder) Build() *database.TestRun {
	return b.run
}

// CreateTestRun creates and persists a test run.
func CreateTestRun(ctx context.Context, repo database.TestRunRepository, serviceID uuid.UUID, opts ...func(*TestRunBuilder)) (*database.TestRun, error) {
	builder := NewTestRunBuilder(serviceID)
	for _, opt := range opts {
		opt(builder)
	}
	run := builder.Build()
	if err := repo.Create(ctx, run); err != nil {
		return nil, err
	}
	return run, nil
}

// AgentBuilder helps construct test agents with default values.
type AgentBuilder struct {
	agent *database.Agent
}

// NewAgentBuilder creates a new agent builder with default values.
func NewAgentBuilder() *AgentBuilder {
	return &AgentBuilder{
		agent: &database.Agent{
			Name:            fmt.Sprintf("test-agent-%s", uuid.New().String()[:8]),
			Status:          database.AgentStatusIdle,
			NetworkZones:    []string{"default"},
			MaxParallel:     4,
			DockerAvailable: true,
		},
	}
}

// WithName sets the agent name.
func (b *AgentBuilder) WithName(name string) *AgentBuilder {
	b.agent.Name = name
	return b
}

// WithStatus sets the agent status.
func (b *AgentBuilder) WithStatus(status database.AgentStatus) *AgentBuilder {
	b.agent.Status = status
	return b
}

// WithVersion sets the agent version.
func (b *AgentBuilder) WithVersion(version string) *AgentBuilder {
	b.agent.Version = &version
	return b
}

// WithNetworkZones sets the network zones.
func (b *AgentBuilder) WithNetworkZones(zones []string) *AgentBuilder {
	b.agent.NetworkZones = zones
	return b
}

// WithMaxParallel sets the max parallel value.
func (b *AgentBuilder) WithMaxParallel(max int) *AgentBuilder {
	b.agent.MaxParallel = max
	return b
}

// WithDockerAvailable sets whether Docker is available.
func (b *AgentBuilder) WithDockerAvailable(available bool) *AgentBuilder {
	b.agent.DockerAvailable = available
	return b
}

// WithLastHeartbeat sets the last heartbeat time.
func (b *AgentBuilder) WithLastHeartbeat(t time.Time) *AgentBuilder {
	b.agent.LastHeartbeat = &t
	return b
}

// Build returns the built agent.
func (b *AgentBuilder) Build() *database.Agent {
	return b.agent
}

// CreateAgent creates and persists a test agent.
func CreateAgent(ctx context.Context, repo database.AgentRepository, opts ...func(*AgentBuilder)) (*database.Agent, error) {
	builder := NewAgentBuilder()
	for _, opt := range opts {
		opt(builder)
	}
	agent := builder.Build()
	if err := repo.Create(ctx, agent); err != nil {
		return nil, err
	}
	return agent, nil
}

// TestResultBuilder helps construct test results with default values.
type TestResultBuilder struct {
	result *database.TestResult
}

// NewTestResultBuilder creates a new test result builder.
func NewTestResultBuilder(runID uuid.UUID) *TestResultBuilder {
	return &TestResultBuilder{
		result: &database.TestResult{
			RunID:      runID,
			TestName:   fmt.Sprintf("Test_%s", uuid.New().String()[:8]),
			Status:     database.ResultStatusPass,
			RetryCount: 0,
		},
	}
}

// WithTestName sets the test name.
func (b *TestResultBuilder) WithTestName(name string) *TestResultBuilder {
	b.result.TestName = name
	return b
}

// WithSuiteName sets the suite name.
func (b *TestResultBuilder) WithSuiteName(name string) *TestResultBuilder {
	b.result.SuiteName = &name
	return b
}

// WithStatus sets the status.
func (b *TestResultBuilder) WithStatus(status database.ResultStatus) *TestResultBuilder {
	b.result.Status = status
	return b
}

// WithDuration sets the duration in milliseconds.
func (b *TestResultBuilder) WithDuration(durationMs int64) *TestResultBuilder {
	b.result.DurationMs = &durationMs
	return b
}

// WithError sets the error message.
func (b *TestResultBuilder) WithError(msg string) *TestResultBuilder {
	b.result.ErrorMessage = &msg
	return b
}

// WithStackTrace sets the stack trace.
func (b *TestResultBuilder) WithStackTrace(trace string) *TestResultBuilder {
	b.result.StackTrace = &trace
	return b
}

// WithStdout sets the stdout.
func (b *TestResultBuilder) WithStdout(stdout string) *TestResultBuilder {
	b.result.Stdout = &stdout
	return b
}

// WithStderr sets the stderr.
func (b *TestResultBuilder) WithStderr(stderr string) *TestResultBuilder {
	b.result.Stderr = &stderr
	return b
}

// WithRetryCount sets the retry count.
func (b *TestResultBuilder) WithRetryCount(count int) *TestResultBuilder {
	b.result.RetryCount = count
	return b
}

// Build returns the built test result.
func (b *TestResultBuilder) Build() *database.TestResult {
	return b.result
}

// CreateTestResult creates and persists a test result.
func CreateTestResult(ctx context.Context, repo database.ResultRepository, runID uuid.UUID, opts ...func(*TestResultBuilder)) (*database.TestResult, error) {
	builder := NewTestResultBuilder(runID)
	for _, opt := range opts {
		opt(builder)
	}
	result := builder.Build()
	if err := repo.Create(ctx, result); err != nil {
		return nil, err
	}
	return result, nil
}

// CreateTestResults creates multiple test results for a run.
func CreateTestResults(ctx context.Context, repo database.ResultRepository, runID uuid.UUID, count int, failCount int) ([]database.TestResult, error) {
	results := make([]database.TestResult, count)
	for i := 0; i < count; i++ {
		status := database.ResultStatusPass
		if i < failCount {
			status = database.ResultStatusFail
		}

		durationMs := int64((i + 1) * 100)
		results[i] = database.TestResult{
			RunID:      runID,
			TestName:   fmt.Sprintf("Test_%d", i+1),
			Status:     status,
			DurationMs: &durationMs,
		}
	}

	if err := repo.BatchCreate(ctx, results); err != nil {
		return nil, err
	}
	return results, nil
}

// NotificationChannelBuilder helps construct notification channels.
type NotificationChannelBuilder struct {
	channel *database.NotificationChannel
}

// NewNotificationChannelBuilder creates a new notification channel builder.
func NewNotificationChannelBuilder() *NotificationChannelBuilder {
	config, _ := json.Marshal(map[string]string{
		"webhook_url": "https://hooks.example.com/test",
	})
	return &NotificationChannelBuilder{
		channel: &database.NotificationChannel{
			Name:    fmt.Sprintf("test-channel-%s", uuid.New().String()[:8]),
			Type:    database.ChannelTypeSlack,
			Config:  config,
			Enabled: true,
		},
	}
}

// WithName sets the channel name.
func (b *NotificationChannelBuilder) WithName(name string) *NotificationChannelBuilder {
	b.channel.Name = name
	return b
}

// WithType sets the channel type.
func (b *NotificationChannelBuilder) WithType(t database.ChannelType) *NotificationChannelBuilder {
	b.channel.Type = t
	return b
}

// WithConfig sets the channel config.
func (b *NotificationChannelBuilder) WithConfig(config map[string]interface{}) *NotificationChannelBuilder {
	data, _ := json.Marshal(config)
	b.channel.Config = data
	return b
}

// WithEnabled sets whether the channel is enabled.
func (b *NotificationChannelBuilder) WithEnabled(enabled bool) *NotificationChannelBuilder {
	b.channel.Enabled = enabled
	return b
}

// Build returns the built channel.
func (b *NotificationChannelBuilder) Build() *database.NotificationChannel {
	return b.channel
}

// CreateNotificationChannel creates and persists a notification channel.
func CreateNotificationChannel(ctx context.Context, repo database.NotificationRepository, opts ...func(*NotificationChannelBuilder)) (*database.NotificationChannel, error) {
	builder := NewNotificationChannelBuilder()
	for _, opt := range opts {
		opt(builder)
	}
	channel := builder.Build()
	if err := repo.CreateChannel(ctx, channel); err != nil {
		return nil, err
	}
	return channel, nil
}

// MockGitProvider provides a mock git provider for testing.
type MockGitProvider struct {
	Repositories map[string]*MockRepository
	StatusChecks map[string]string
	OnClone      func(ctx context.Context, url, ref, dest string) error
	OnStatus     func(ctx context.Context, owner, repo, sha, state string) error
}

// MockRepository represents a mock repository.
type MockRepository struct {
	URL           string
	DefaultBranch string
	Commits       map[string]*MockCommit
	Branches      []string
	Tags          []string
}

// MockCommit represents a mock commit.
type MockCommit struct {
	SHA     string
	Message string
	Author  string
	Date    time.Time
}

// NewMockGitProvider creates a new mock git provider.
func NewMockGitProvider() *MockGitProvider {
	return &MockGitProvider{
		Repositories: make(map[string]*MockRepository),
		StatusChecks: make(map[string]string),
	}
}

// AddRepository adds a mock repository.
func (p *MockGitProvider) AddRepository(url string, defaultBranch string) *MockRepository {
	repo := &MockRepository{
		URL:           url,
		DefaultBranch: defaultBranch,
		Commits:       make(map[string]*MockCommit),
		Branches:      []string{defaultBranch},
		Tags:          []string{},
	}
	p.Repositories[url] = repo
	return repo
}

// AddCommit adds a commit to a repository.
func (r *MockRepository) AddCommit(sha, message, author string) *MockCommit {
	commit := &MockCommit{
		SHA:     sha,
		Message: message,
		Author:  author,
		Date:    time.Now(),
	}
	r.Commits[sha] = commit
	return commit
}

// TestDefinitionBuilder helps construct test definitions with default values.
type TestDefinitionBuilder struct {
	def *database.TestDefinition
}

// NewTestDefinitionBuilder creates a new test definition builder.
func NewTestDefinitionBuilder(serviceID uuid.UUID) *TestDefinitionBuilder {
	return &TestDefinitionBuilder{
		def: &database.TestDefinition{
			ServiceID:      serviceID,
			Name:           fmt.Sprintf("test-def-%s", uuid.New().String()[:8]),
			ExecutionType:  "subprocess",
			Command:        "go",
			Args:           []string{"test", "-v", "./..."},
			TimeoutSeconds: 300,
			Tags:           []string{},
			Retries:        0,
			AllowFailure:   false,
		},
	}
}

// WithName sets the test definition name.
func (b *TestDefinitionBuilder) WithName(name string) *TestDefinitionBuilder {
	b.def.Name = name
	return b
}

// WithDescription sets the description.
func (b *TestDefinitionBuilder) WithDescription(desc string) *TestDefinitionBuilder {
	b.def.Description = &desc
	return b
}

// WithExecutionType sets the execution type.
func (b *TestDefinitionBuilder) WithExecutionType(t string) *TestDefinitionBuilder {
	b.def.ExecutionType = t
	return b
}

// WithCommand sets the command.
func (b *TestDefinitionBuilder) WithCommand(cmd string) *TestDefinitionBuilder {
	b.def.Command = cmd
	return b
}

// WithArgs sets the command arguments.
func (b *TestDefinitionBuilder) WithArgs(args []string) *TestDefinitionBuilder {
	b.def.Args = args
	return b
}

// WithTimeout sets the timeout in seconds.
func (b *TestDefinitionBuilder) WithTimeout(seconds int) *TestDefinitionBuilder {
	b.def.TimeoutSeconds = seconds
	return b
}

// WithResultFile sets the result file path.
func (b *TestDefinitionBuilder) WithResultFile(path string) *TestDefinitionBuilder {
	b.def.ResultFile = &path
	return b
}

// WithResultFormat sets the result format.
func (b *TestDefinitionBuilder) WithResultFormat(format string) *TestDefinitionBuilder {
	b.def.ResultFormat = &format
	return b
}

// WithTags sets the tags.
func (b *TestDefinitionBuilder) WithTags(tags []string) *TestDefinitionBuilder {
	b.def.Tags = tags
	return b
}

// WithRetries sets the retry count.
func (b *TestDefinitionBuilder) WithRetries(retries int) *TestDefinitionBuilder {
	b.def.Retries = retries
	return b
}

// WithAllowFailure sets whether failure is allowed.
func (b *TestDefinitionBuilder) WithAllowFailure(allow bool) *TestDefinitionBuilder {
	b.def.AllowFailure = allow
	return b
}

// Build returns the built test definition.
func (b *TestDefinitionBuilder) Build() *database.TestDefinition {
	return b.def
}

// CreateTestDefinition creates and persists a test definition.
func CreateTestDefinition(ctx context.Context, repo database.TestDefinitionRepository, serviceID uuid.UUID, opts ...func(*TestDefinitionBuilder)) (*database.TestDefinition, error) {
	builder := NewTestDefinitionBuilder(serviceID)
	for _, opt := range opts {
		opt(builder)
	}
	def := builder.Build()
	if err := repo.Create(ctx, def); err != nil {
		return nil, err
	}
	return def, nil
}

// SampleJUnitXML returns a sample JUnit XML test result.
func SampleJUnitXML() string {
	return `<?xml version="1.0" encoding="UTF-8"?>
<testsuites>
  <testsuite name="TestSuite" tests="4" failures="1" errors="0" skipped="1" time="1.234">
    <testcase name="TestPass1" classname="pkg.TestSuite" time="0.123"/>
    <testcase name="TestPass2" classname="pkg.TestSuite" time="0.234"/>
    <testcase name="TestFail1" classname="pkg.TestSuite" time="0.345">
      <failure message="assertion failed" type="AssertionError">Expected 1 but got 2</failure>
    </testcase>
    <testcase name="TestSkipped1" classname="pkg.TestSuite" time="0.0">
      <skipped message="Not implemented"/>
    </testcase>
  </testsuite>
</testsuites>`
}

// SampleJestJSON returns a sample Jest JSON test result.
func SampleJestJSON() string {
	return `{
  "numTotalTests": 3,
  "numPassedTests": 2,
  "numFailedTests": 1,
  "testResults": [
    {
      "name": "/app/tests/example.test.js",
      "status": "passed",
      "startTime": 1706000000000,
      "endTime": 1706000001000,
      "assertionResults": [
        {
          "title": "should pass first test",
          "fullName": "Example tests should pass first test",
          "status": "passed",
          "duration": 50,
          "failureMessages": [],
          "ancestorTitles": ["Example tests"]
        },
        {
          "title": "should pass second test",
          "fullName": "Example tests should pass second test",
          "status": "passed",
          "duration": 30,
          "failureMessages": [],
          "ancestorTitles": ["Example tests"]
        },
        {
          "title": "should fail",
          "fullName": "Example tests should fail",
          "status": "failed",
          "duration": 100,
          "failureMessages": ["Expected true to be false"],
          "ancestorTitles": ["Example tests"]
        }
      ]
    }
  ]
}`
}

// SamplePlaywrightJSON returns a sample Playwright JSON test result.
func SamplePlaywrightJSON() string {
	return `{
  "suites": [
    {
      "title": "Login tests",
      "specs": [
        {
          "title": "should login successfully",
          "file": "tests/login.spec.ts",
          "line": 10,
          "tests": [
            {
              "projectName": "chromium",
              "results": [
                {
                  "status": "passed",
                  "duration": 1500,
                  "retry": 0
                }
              ]
            }
          ]
        },
        {
          "title": "should show error on invalid password",
          "file": "tests/login.spec.ts",
          "line": 20,
          "tests": [
            {
              "projectName": "chromium",
              "results": [
                {
                  "status": "failed",
                  "duration": 2000,
                  "retry": 0,
                  "error": {
                    "message": "Locator not found",
                    "stack": "Error: Locator not found\n    at test.spec.ts:25"
                  }
                },
                {
                  "status": "passed",
                  "duration": 1800,
                  "retry": 1
                }
              ]
            }
          ]
        }
      ],
      "suites": []
    }
  ]
}`
}

// SampleGoTestJSON returns a sample Go test JSON output.
func SampleGoTestJSON() string {
	return `{"Time":"2024-01-25T10:00:00Z","Action":"run","Package":"example/pkg","Test":"TestExample1"}
{"Time":"2024-01-25T10:00:00.1Z","Action":"output","Package":"example/pkg","Test":"TestExample1","Output":"=== RUN   TestExample1\n"}
{"Time":"2024-01-25T10:00:00.2Z","Action":"output","Package":"example/pkg","Test":"TestExample1","Output":"--- PASS: TestExample1 (0.10s)\n"}
{"Time":"2024-01-25T10:00:00.2Z","Action":"pass","Package":"example/pkg","Test":"TestExample1","Elapsed":0.1}
{"Time":"2024-01-25T10:00:00.3Z","Action":"run","Package":"example/pkg","Test":"TestExample2"}
{"Time":"2024-01-25T10:00:00.4Z","Action":"output","Package":"example/pkg","Test":"TestExample2","Output":"=== RUN   TestExample2\n"}
{"Time":"2024-01-25T10:00:00.5Z","Action":"output","Package":"example/pkg","Test":"TestExample2","Output":"    example_test.go:15: Expected 1, got 2\n"}
{"Time":"2024-01-25T10:00:00.6Z","Action":"output","Package":"example/pkg","Test":"TestExample2","Output":"--- FAIL: TestExample2 (0.20s)\n"}
{"Time":"2024-01-25T10:00:00.6Z","Action":"fail","Package":"example/pkg","Test":"TestExample2","Elapsed":0.2}
{"Time":"2024-01-25T10:00:00.7Z","Action":"run","Package":"example/pkg","Test":"TestExample3"}
{"Time":"2024-01-25T10:00:00.8Z","Action":"output","Package":"example/pkg","Test":"TestExample3","Output":"=== RUN   TestExample3\n"}
{"Time":"2024-01-25T10:00:00.9Z","Action":"output","Package":"example/pkg","Test":"TestExample3","Output":"--- SKIP: TestExample3 (0.00s)\n"}
{"Time":"2024-01-25T10:00:00.9Z","Action":"skip","Package":"example/pkg","Test":"TestExample3","Elapsed":0.0}`
}

// SampleTAPOutput returns a sample TAP test output.
func SampleTAPOutput() string {
	return `TAP version 13
1..4
ok 1 - First test passes
ok 2 - Second test passes
not ok 3 - Third test fails
ok 4 - Fourth test is skipped # SKIP not implemented`
}
