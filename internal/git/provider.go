// Package git provides git provider integration for syncing test definitions.
package git

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// Provider defines the interface for git hosting providers (GitHub, GitLab, etc.).
type Provider interface {
	// GetFile retrieves file content from a repository.
	GetFile(ctx context.Context, owner, repo, path, ref string) ([]byte, error)

	// ListFiles lists files in a directory.
	ListFiles(ctx context.Context, owner, repo, path, ref string) ([]FileInfo, error)

	// GetDefaultBranch returns the default branch for a repository.
	GetDefaultBranch(ctx context.Context, owner, repo string) (string, error)

	// CreateCommitStatus creates a commit status (for CI integration).
	CreateCommitStatus(ctx context.Context, owner, repo, sha string, status CommitStatus) error

	// CreateComment posts a comment on a pull request.
	CreateComment(ctx context.Context, owner, repo string, prNumber int, body string) error

	// GetPullRequest retrieves pull request details.
	GetPullRequest(ctx context.Context, owner, repo string, number int) (*PullRequest, error)
}

// FileInfo contains information about a file in a repository.
type FileInfo struct {
	Name    string
	Path    string
	Type    string // "file" or "dir"
	Size    int64
	SHA     string
	Content []byte // Only populated if explicitly requested
}

// CommitStatus represents a commit status check.
type CommitStatus struct {
	State       string // pending, success, error, failure
	Context     string // Status check name
	Description string
	TargetURL   string
}

// PullRequest contains pull request information.
type PullRequest struct {
	Number      int
	Title       string
	Body        string
	State       string // open, closed, merged
	HeadRef     string
	HeadSHA     string
	BaseRef     string
	BaseSHA     string
	Author      string
	CreatedAt   time.Time
	UpdatedAt   time.Time
	MergedAt    *time.Time
	MergeCommit string
}

// Webhook represents an incoming webhook event.
type Webhook struct {
	Type      WebhookType
	Action    string
	Delivery  string
	Signature string
	Payload   WebhookPayload
}

// WebhookType identifies the type of webhook event.
type WebhookType string

const (
	WebhookPush         WebhookType = "push"
	WebhookPullRequest  WebhookType = "pull_request"
	WebhookCheckRun     WebhookType = "check_run"
	WebhookCheckSuite   WebhookType = "check_suite"
	WebhookWorkflowRun  WebhookType = "workflow_run"
	WebhookIssueComment WebhookType = "issue_comment"
)

// WebhookPayload contains the parsed webhook event data.
type WebhookPayload struct {
	// Repository information
	RepoOwner    string
	RepoName     string
	RepoFullName string
	RepoURL      string

	// Push event data
	Ref        string // refs/heads/branch or refs/tags/tag
	Before     string
	After      string
	Commits    []CommitInfo
	Pusher     string
	HeadCommit *CommitInfo

	// Pull request event data
	PullRequest *PullRequest
	PRAction    string // opened, closed, synchronize, reopened, etc.

	// Sender information
	Sender string
}

// CommitInfo contains information about a commit.
type CommitInfo struct {
	ID        string
	Message   string
	Author    string
	Timestamp time.Time
	Added     []string
	Removed   []string
	Modified  []string
}

// Config holds configuration for git provider clients.
type Config struct {
	// Provider is the git provider type (github, gitlab, bitbucket)
	Provider string
	// BaseURL is the API base URL (for enterprise/self-hosted)
	BaseURL string
	// Token is the personal access token or app installation token
	Token string
	// WebhookSecret is the secret for validating webhooks
	WebhookSecret string
	// AppID is the GitHub App ID (optional, for app authentication)
	AppID int64
	// AppPrivateKey is the GitHub App private key (optional)
	AppPrivateKey string
	// AppInstallationID is the GitHub App installation ID (optional)
	AppInstallationID int64
}

// TestConfig represents test configuration discovered from a repository.
type TestConfig struct {
	// Version is the config file version
	Version string `yaml:"version" json:"version"`
	// Service is the service metadata
	Service ServiceConfig `yaml:"service" json:"service"`
	// Tests defines the test suites
	Tests []TestSuiteConfig `yaml:"tests" json:"tests"`
}

// ServiceConfig holds service-level configuration.
type ServiceConfig struct {
	Name        string            `yaml:"name" json:"name"`
	Description string            `yaml:"description" json:"description"`
	Owner       string            `yaml:"owner" json:"owner"`
	Repository  string            `yaml:"repository" json:"repository"`
	Language    string            `yaml:"language" json:"language"`
	Labels      map[string]string `yaml:"labels" json:"labels"`
}

// TestSuiteConfig defines a test suite configuration.
type TestSuiteConfig struct {
	Name             string            `yaml:"name" json:"name"`
	Description      string            `yaml:"description" json:"description"`
	Command          string            `yaml:"command" json:"command"`
	Args             []string          `yaml:"args" json:"args"`
	WorkDir          string            `yaml:"workdir" json:"workdir"`
	Env              map[string]string `yaml:"env" json:"env"`
	Timeout          string            `yaml:"timeout" json:"timeout"`
	ExecutionMode    string            `yaml:"execution_mode" json:"execution_mode"` // subprocess, container
	DockerImage      string            `yaml:"docker_image" json:"docker_image"`
	ResultFormat     string            `yaml:"result_format" json:"result_format"` // junit, jest, go_test, etc.
	ResultPath       string            `yaml:"result_path" json:"result_path"`
	Tags             []string          `yaml:"tags" json:"tags"`
	RequiredLabels   map[string]string `yaml:"required_labels" json:"required_labels"`
	Disabled         bool              `yaml:"disabled" json:"disabled"`
	Priority         int               `yaml:"priority" json:"priority"`
	MaxRetries       int               `yaml:"max_retries" json:"max_retries"`
	Parallelizable   bool              `yaml:"parallelizable" json:"parallelizable"`
	ArtifactPaths    []string          `yaml:"artifact_paths" json:"artifact_paths"`
	SetupCommands    []string          `yaml:"setup_commands" json:"setup_commands"`
	TeardownCommands []string          `yaml:"teardown_commands" json:"teardown_commands"`
}

// DiscoveredTest represents a test discovered from a repository.
type DiscoveredTest struct {
	ID               uuid.UUID
	ServiceID        uuid.UUID
	Name             string
	Description      string
	Command          string
	Args             []string
	WorkDir          string
	Env              map[string]string
	TimeoutSeconds   int
	ExecutionMode    string
	DockerImage      string
	ResultFormat     string
	ResultPath       string
	Tags             []string
	RequiredLabels   map[string]string
	Disabled         bool
	Priority         int
	MaxRetries       int
	Parallelizable   bool
	ArtifactPaths    []string
	SetupCommands    []string
	TeardownCommands []string
	SourceFile       string
	SourceCommit     string
}

// NewProvider creates a git provider based on the configuration.
func NewProvider(cfg Config) (Provider, error) {
	switch cfg.Provider {
	case "github", "":
		return NewGitHubProvider(cfg)
	case "gitlab":
		return NewGitLabProvider(cfg)
	case "bitbucket":
		return NewBitbucketProvider(cfg)
	default:
		return nil, fmt.Errorf("unsupported git provider: %s", cfg.Provider)
	}
}
