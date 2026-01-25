package git

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"
	"gopkg.in/yaml.v3"

	"github.com/conductor/conductor/internal/database"
)

// SyncResult contains the results of a git sync operation.
type SyncResult struct {
	TestsAdded   int
	TestsUpdated int
	TestsRemoved int
	Errors       []string
	SyncedAt     time.Time
}

const (
	// ConfigFileName is the default test configuration file name.
	ConfigFileName = ".conductor.yaml"
	// AlternateConfigFileName is an alternate config file name.
	AlternateConfigFileName = ".conductor.yml"
	// DefaultTimeout is the default test timeout if not specified.
	DefaultTimeout = "30m"
)

// Syncer handles synchronization of test definitions from git repositories.
type Syncer struct {
	provider Provider
	testRepo database.TestDefinitionRepository
	logger   *slog.Logger
}

// NewSyncer creates a new git syncer.
func NewSyncer(provider Provider, testRepo database.TestDefinitionRepository, logger *slog.Logger) *Syncer {
	if logger == nil {
		logger = slog.Default()
	}
	return &Syncer{
		provider: provider,
		testRepo: testRepo,
		logger:   logger.With("component", "git_syncer"),
	}
}

// SyncService synchronizes test definitions from a service's git repository.
// It implements the server.GitSyncer interface.
func (s *Syncer) SyncService(ctx context.Context, service *database.Service, branch string) (*SyncResult, error) {
	s.logger.Info("starting sync",
		"service_id", service.ID,
		"service_name", service.Name,
		"git_url", service.GitURL,
		"branch", branch,
	)

	result := &SyncResult{
		SyncedAt: time.Now().UTC(),
	}

	// Parse repository URL to get owner/repo
	owner, repo, err := parseRepositoryURL(service.GitURL)
	if err != nil {
		result.Errors = append(result.Errors, fmt.Sprintf("invalid repository URL: %v", err))
		return result, err
	}

	// Get branch if not specified
	if branch == "" {
		branch = service.DefaultBranch
		if branch == "" {
			branch, err = s.provider.GetDefaultBranch(ctx, owner, repo)
			if err != nil {
				result.Errors = append(result.Errors, fmt.Sprintf("failed to get default branch: %v", err))
				return result, err
			}
		}
	}

	// Try to load configuration file
	config, configPath, err := s.loadConfig(ctx, owner, repo, branch)
	if err != nil {
		s.logger.Warn("failed to load config file",
			"error", err,
			"owner", owner,
			"repo", repo,
		)
		result.Errors = append(result.Errors, fmt.Sprintf("failed to load config: %v", err))
		return result, nil // Return without error - service can still work without config
	}

	s.logger.Debug("loaded configuration",
		"config_path", configPath,
		"test_count", len(config.Tests),
	)

	// Get existing test definitions
	existingTests, err := s.testRepo.ListByService(ctx, service.ID, database.Pagination{Limit: 1000})
	if err != nil {
		result.Errors = append(result.Errors, fmt.Sprintf("failed to list existing tests: %v", err))
		return result, err
	}

	// Create map of existing tests by name
	existingByName := make(map[string]database.TestDefinition)
	for _, t := range existingTests {
		existingByName[t.Name] = t
	}

	// Track which tests are in config
	configTestNames := make(map[string]bool)

	// Process each test from config
	for _, testCfg := range config.Tests {
		configTestNames[testCfg.Name] = true

		test, err := s.configToTestDefinition(testCfg, service.ID, configPath, branch)
		if err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("invalid test config '%s': %v", testCfg.Name, err))
			continue
		}

		if existing, ok := existingByName[testCfg.Name]; ok {
			// Update existing test
			test.ID = existing.ID
			test.CreatedAt = existing.CreatedAt
			test.UpdatedAt = time.Now().UTC()

			if err := s.testRepo.Update(ctx, test); err != nil {
				result.Errors = append(result.Errors, fmt.Sprintf("failed to update test '%s': %v", testCfg.Name, err))
				continue
			}
			result.TestsUpdated++
		} else {
			// Create new test
			test.ID = uuid.New()
			test.CreatedAt = time.Now().UTC()
			test.UpdatedAt = time.Now().UTC()

			if err := s.testRepo.Create(ctx, test); err != nil {
				result.Errors = append(result.Errors, fmt.Sprintf("failed to create test '%s': %v", testCfg.Name, err))
				continue
			}
			result.TestsAdded++
		}
	}

	// Optionally mark tests not in config as disabled (using AllowFailure as a proxy)
	// Note: The actual database model doesn't have a Disabled field,
	// so we can't disable tests - just track what's been updated
	for name := range existingByName {
		if !configTestNames[name] {
			// Test was removed from config - it remains in DB but won't be updated
			result.TestsRemoved++
		}
	}

	s.logger.Info("sync completed",
		"service_id", service.ID,
		"added", result.TestsAdded,
		"updated", result.TestsUpdated,
		"removed", result.TestsRemoved,
		"errors", len(result.Errors),
	)

	return result, nil
}

// loadConfig attempts to load the conductor configuration file from the repository.
func (s *Syncer) loadConfig(ctx context.Context, owner, repo, ref string) (*TestConfig, string, error) {
	// Try primary config file name
	content, err := s.provider.GetFile(ctx, owner, repo, ConfigFileName, ref)
	if err == nil {
		var config TestConfig
		if err := yaml.Unmarshal(content, &config); err != nil {
			return nil, ConfigFileName, fmt.Errorf("failed to parse %s: %w", ConfigFileName, err)
		}
		return &config, ConfigFileName, nil
	}

	// Try alternate config file name
	content, err = s.provider.GetFile(ctx, owner, repo, AlternateConfigFileName, ref)
	if err == nil {
		var config TestConfig
		if err := yaml.Unmarshal(content, &config); err != nil {
			return nil, AlternateConfigFileName, fmt.Errorf("failed to parse %s: %w", AlternateConfigFileName, err)
		}
		return &config, AlternateConfigFileName, nil
	}

	return nil, "", fmt.Errorf("config file not found (tried %s and %s)", ConfigFileName, AlternateConfigFileName)
}

// configToTestDefinition converts a TestSuiteConfig to a database.TestDefinition.
func (s *Syncer) configToTestDefinition(cfg TestSuiteConfig, serviceID uuid.UUID, configPath, ref string) (*database.TestDefinition, error) {
	if cfg.Name == "" {
		return nil, fmt.Errorf("test name is required")
	}
	if cfg.Command == "" {
		return nil, fmt.Errorf("test command is required")
	}

	// Parse timeout
	timeout := DefaultTimeout
	if cfg.Timeout != "" {
		timeout = cfg.Timeout
	}
	timeoutDuration, err := time.ParseDuration(timeout)
	if err != nil {
		return nil, fmt.Errorf("invalid timeout: %w", err)
	}

	// Determine execution mode
	execType := cfg.ExecutionMode
	if execType == "" {
		execType = "subprocess"
	}
	if execType != "subprocess" && execType != "container" {
		return nil, fmt.Errorf("invalid execution_mode: %s (must be subprocess or container)", execType)
	}

	// Container mode requires docker image (note: we store this info elsewhere if needed)
	if execType == "container" && cfg.DockerImage == "" {
		return nil, fmt.Errorf("docker_image is required for container execution mode")
	}

	test := &database.TestDefinition{
		ServiceID:        serviceID,
		Name:             cfg.Name,
		Description:      database.NullString(cfg.Description),
		ExecutionType:    execType,
		Command:          cfg.Command,
		Args:             cfg.Args,
		TimeoutSeconds:   int(timeoutDuration.Seconds()),
		ResultFormat:     database.NullString(cfg.ResultFormat),
		ResultFile:       database.NullString(cfg.ResultPath),
		Tags:             cfg.Tags,
		Retries:          cfg.MaxRetries,
		AllowFailure:     cfg.Disabled, // Use AllowFailure to indicate disabled tests
		ArtifactPatterns: cfg.ArtifactPaths,
		DependsOn:        nil, // Could be derived from config if needed
	}

	return test, nil
}

// parseRepositoryURL extracts owner and repo from a git repository URL.
func parseRepositoryURL(url string) (owner, repo string, err error) {
	// Handle various URL formats:
	// - https://github.com/owner/repo
	// - https://github.com/owner/repo.git
	// - git@github.com:owner/repo.git
	// - owner/repo

	url = strings.TrimSuffix(url, ".git")
	url = strings.TrimPrefix(url, "https://github.com/")
	url = strings.TrimPrefix(url, "http://github.com/")
	url = strings.TrimPrefix(url, "git@github.com:")

	// Also handle GitLab, Bitbucket patterns
	url = strings.TrimPrefix(url, "https://gitlab.com/")
	url = strings.TrimPrefix(url, "git@gitlab.com:")
	url = strings.TrimPrefix(url, "https://bitbucket.org/")
	url = strings.TrimPrefix(url, "git@bitbucket.org:")

	parts := strings.Split(url, "/")
	if len(parts) < 2 {
		return "", "", fmt.Errorf("invalid repository URL format: %s", url)
	}

	owner = parts[0]
	repo = parts[1]

	if owner == "" || repo == "" {
		return "", "", fmt.Errorf("invalid repository URL: owner or repo is empty")
	}

	return owner, repo, nil
}
