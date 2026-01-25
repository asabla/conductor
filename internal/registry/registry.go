// Package registry provides test definition registry and service management.
package registry

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"time"

	"github.com/google/uuid"

	"github.com/conductor/conductor/internal/database"
)

// RegistryService defines the interface for the test registry service.
type RegistryService interface {
	// SyncService syncs test definitions from the service's Git repository.
	SyncService(ctx context.Context, serviceID uuid.UUID, branch string) (*SyncResult, error)

	// GetService retrieves a service by ID.
	GetService(ctx context.Context, id uuid.UUID) (*database.Service, error)

	// ListServices lists services with optional filters.
	ListServices(ctx context.Context, filter ServiceFilter) ([]database.Service, error)

	// GetTestDefinitions retrieves test definitions for a service.
	GetTestDefinitions(ctx context.Context, serviceID uuid.UUID, filter TestFilter) ([]database.TestDefinition, error)

	// CreateService creates a new service in the registry.
	CreateService(ctx context.Context, service *database.Service) error

	// UpdateService updates an existing service.
	UpdateService(ctx context.Context, service *database.Service) error

	// DeleteService removes a service from the registry.
	DeleteService(ctx context.Context, id uuid.UUID) error
}

// ServiceFilter defines filters for listing services.
type ServiceFilter struct {
	Owner       string
	NetworkZone string
	NamePattern string
	Pagination  database.Pagination
}

// TestFilter defines filters for listing test definitions.
type TestFilter struct {
	Tags       []string
	Enabled    *bool
	Pagination database.Pagination
}

// SyncResult contains the results of a service sync operation.
type SyncResult struct {
	TestsAdded   int
	TestsUpdated int
	TestsRemoved int
	Errors       []string
	SyncedAt     time.Time
}

// GitProvider defines the interface for interacting with Git repositories.
type GitProvider interface {
	// CloneOrPull clones or pulls the repository to a local path.
	CloneOrPull(ctx context.Context, repoURL, branch, localPath string) error

	// ReadFile reads a file from the repository at the given ref.
	ReadFile(ctx context.Context, repoURL, ref, path string) (io.ReadCloser, error)

	// ListFiles lists files matching a pattern in the repository.
	ListFiles(ctx context.Context, repoURL, ref, pattern string) ([]string, error)
}

// Registry implements the RegistryService interface.
type Registry struct {
	serviceRepo database.ServiceRepository
	testRepo    database.TestDefinitionRepository
	gitProvider GitProvider
	logger      *slog.Logger

	configPath string // Path to .testharness.yaml in repos
}

// NewRegistry creates a new Registry instance.
func NewRegistry(
	serviceRepo database.ServiceRepository,
	testRepo database.TestDefinitionRepository,
	gitProvider GitProvider,
	logger *slog.Logger,
) *Registry {
	if logger == nil {
		logger = slog.Default()
	}

	return &Registry{
		serviceRepo: serviceRepo,
		testRepo:    testRepo,
		gitProvider: gitProvider,
		logger:      logger.With("component", "registry"),
		configPath:  ".testharness.yaml",
	}
}

// SyncService syncs test definitions from the service's Git repository.
func (r *Registry) SyncService(ctx context.Context, serviceID uuid.UUID, branch string) (*SyncResult, error) {
	service, err := r.serviceRepo.Get(ctx, serviceID)
	if err != nil {
		return nil, fmt.Errorf("failed to get service: %w", err)
	}
	if service == nil {
		return nil, fmt.Errorf("service not found: %s", serviceID)
	}

	// Use default branch if not specified
	if branch == "" {
		branch = service.DefaultBranch
	}

	r.logger.Info("syncing service",
		"service_id", serviceID,
		"service_name", service.Name,
		"branch", branch,
	)

	// Read manifest file from repository
	reader, err := r.gitProvider.ReadFile(ctx, service.GitURL, branch, r.configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read manifest file: %w", err)
	}
	defer reader.Close()

	// Parse manifest
	manifest, err := ParseManifest(reader)
	if err != nil {
		return nil, fmt.Errorf("failed to parse manifest: %w", err)
	}

	// Validate manifest
	if err := ValidateManifest(manifest); err != nil {
		return nil, fmt.Errorf("invalid manifest: %w", err)
	}

	// Get existing test definitions
	existingTests, err := r.testRepo.ListByService(ctx, serviceID, database.Pagination{Limit: 10000})
	if err != nil {
		return nil, fmt.Errorf("failed to list existing tests: %w", err)
	}

	existingByName := make(map[string]database.TestDefinition)
	for _, t := range existingTests {
		existingByName[t.Name] = t
	}

	result := &SyncResult{
		SyncedAt: time.Now().UTC(),
	}

	// Process test definitions from manifest
	seenNames := make(map[string]bool)

	for _, testDef := range manifest.Tests {
		seenNames[testDef.Name] = true

		dbTest := manifestTestToDBTest(serviceID, testDef)

		if existing, ok := existingByName[testDef.Name]; ok {
			// Update existing test
			dbTest.ID = existing.ID
			dbTest.CreatedAt = existing.CreatedAt

			if err := r.testRepo.Update(ctx, &dbTest); err != nil {
				result.Errors = append(result.Errors, fmt.Sprintf("failed to update test %s: %v", testDef.Name, err))
				continue
			}
			result.TestsUpdated++
		} else {
			// Create new test
			dbTest.ID = uuid.New()
			dbTest.CreatedAt = time.Now().UTC()

			if err := r.testRepo.Create(ctx, &dbTest); err != nil {
				result.Errors = append(result.Errors, fmt.Sprintf("failed to create test %s: %v", testDef.Name, err))
				continue
			}
			result.TestsAdded++
		}
	}

	// Remove tests that are no longer in the manifest
	for name, existing := range existingByName {
		if !seenNames[name] {
			if err := r.testRepo.Delete(ctx, existing.ID); err != nil {
				result.Errors = append(result.Errors, fmt.Sprintf("failed to delete test %s: %v", name, err))
				continue
			}
			result.TestsRemoved++
		}
	}

	r.logger.Info("sync completed",
		"service_id", serviceID,
		"added", result.TestsAdded,
		"updated", result.TestsUpdated,
		"removed", result.TestsRemoved,
		"errors", len(result.Errors),
	)

	return result, nil
}

// GetService retrieves a service by ID.
func (r *Registry) GetService(ctx context.Context, id uuid.UUID) (*database.Service, error) {
	return r.serviceRepo.Get(ctx, id)
}

// ListServices lists services with optional filters.
func (r *Registry) ListServices(ctx context.Context, filter ServiceFilter) ([]database.Service, error) {
	if filter.Owner != "" {
		return r.serviceRepo.ListByOwner(ctx, filter.Owner, filter.Pagination)
	}

	if filter.NamePattern != "" {
		return r.serviceRepo.Search(ctx, filter.NamePattern, filter.Pagination)
	}

	return r.serviceRepo.List(ctx, filter.Pagination)
}

// GetTestDefinitions retrieves test definitions for a service.
func (r *Registry) GetTestDefinitions(ctx context.Context, serviceID uuid.UUID, filter TestFilter) ([]database.TestDefinition, error) {
	if len(filter.Tags) > 0 {
		return r.testRepo.ListByTags(ctx, serviceID, filter.Tags, filter.Pagination)
	}

	return r.testRepo.ListByService(ctx, serviceID, filter.Pagination)
}

// CreateService creates a new service in the registry.
func (r *Registry) CreateService(ctx context.Context, service *database.Service) error {
	if service.ID == uuid.Nil {
		service.ID = uuid.New()
	}
	service.CreatedAt = time.Now().UTC()
	service.UpdatedAt = service.CreatedAt

	r.logger.Info("creating service",
		"service_id", service.ID,
		"name", service.Name,
		"git_url", service.GitURL,
	)

	return r.serviceRepo.Create(ctx, service)
}

// UpdateService updates an existing service.
func (r *Registry) UpdateService(ctx context.Context, service *database.Service) error {
	service.UpdatedAt = time.Now().UTC()

	r.logger.Info("updating service",
		"service_id", service.ID,
		"name", service.Name,
	)

	return r.serviceRepo.Update(ctx, service)
}

// DeleteService removes a service from the registry.
func (r *Registry) DeleteService(ctx context.Context, id uuid.UUID) error {
	r.logger.Info("deleting service", "service_id", id)
	return r.serviceRepo.Delete(ctx, id)
}

// manifestTestToDBTest converts a manifest test definition to a database model.
func manifestTestToDBTest(serviceID uuid.UUID, test TestDefinition) database.TestDefinition {
	return database.TestDefinition{
		ServiceID:        serviceID,
		Name:             test.Name,
		Description:      database.NullString(test.Description),
		ExecutionType:    test.ExecutionType,
		Command:          test.Command,
		Args:             test.Args,
		TimeoutSeconds:   test.TimeoutSeconds,
		ResultFile:       database.NullString(test.ResultFile),
		ResultFormat:     database.NullString(test.ResultFormat),
		ArtifactPatterns: test.ArtifactPatterns,
		Tags:             test.Tags,
		DependsOn:        test.DependsOn,
		Retries:          test.Retries,
		AllowFailure:     test.AllowFailure,
		UpdatedAt:        time.Now().UTC(),
	}
}
