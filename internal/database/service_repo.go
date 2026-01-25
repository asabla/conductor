package database

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// serviceRepo implements ServiceRepository.
type serviceRepo struct {
	db *DB
}

// NewServiceRepo creates a new service repository.
func NewServiceRepo(db *DB) ServiceRepository {
	return &serviceRepo{db: db}
}

// Create creates a new service.
func (r *serviceRepo) Create(ctx context.Context, svc *Service) error {
	err := r.db.pool.QueryRow(ctx, ServiceInsert,
		svc.Name,
		svc.DisplayName,
		svc.GitURL,
		svc.GitProvider,
		svc.DefaultBranch,
		svc.NetworkZones,
		svc.Owner,
		svc.ContactSlack,
		svc.ContactEmail,
	).Scan(&svc.ID, &svc.CreatedAt, &svc.UpdatedAt)

	if err != nil {
		return fmt.Errorf("failed to create service: %w", WrapDBError(err))
	}
	return nil
}

// Get retrieves a service by ID.
func (r *serviceRepo) Get(ctx context.Context, id uuid.UUID) (*Service, error) {
	svc := &Service{}
	err := r.db.pool.QueryRow(ctx, ServiceGetByID, id).Scan(
		&svc.ID,
		&svc.Name,
		&svc.DisplayName,
		&svc.GitURL,
		&svc.GitProvider,
		&svc.DefaultBranch,
		&svc.NetworkZones,
		&svc.Owner,
		&svc.ContactSlack,
		&svc.ContactEmail,
		&svc.CreatedAt,
		&svc.UpdatedAt,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("failed to get service: %w", err)
	}
	return svc, nil
}

// GetByName retrieves a service by name.
func (r *serviceRepo) GetByName(ctx context.Context, name string) (*Service, error) {
	svc := &Service{}
	err := r.db.pool.QueryRow(ctx, ServiceGetByName, name).Scan(
		&svc.ID,
		&svc.Name,
		&svc.DisplayName,
		&svc.GitURL,
		&svc.GitProvider,
		&svc.DefaultBranch,
		&svc.NetworkZones,
		&svc.Owner,
		&svc.ContactSlack,
		&svc.ContactEmail,
		&svc.CreatedAt,
		&svc.UpdatedAt,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("failed to get service by name: %w", err)
	}
	return svc, nil
}

// Update updates an existing service.
func (r *serviceRepo) Update(ctx context.Context, svc *Service) error {
	err := r.db.pool.QueryRow(ctx, ServiceUpdate,
		svc.ID,
		svc.Name,
		svc.DisplayName,
		svc.GitURL,
		svc.GitProvider,
		svc.DefaultBranch,
		svc.NetworkZones,
		svc.Owner,
		svc.ContactSlack,
		svc.ContactEmail,
	).Scan(&svc.UpdatedAt)

	if err != nil {
		if err == pgx.ErrNoRows {
			return ErrNotFound
		}
		return fmt.Errorf("failed to update service: %w", WrapDBError(err))
	}
	return nil
}

// Delete deletes a service by ID.
func (r *serviceRepo) Delete(ctx context.Context, id uuid.UUID) error {
	result, err := r.db.pool.Exec(ctx, ServiceDelete, id)
	if err != nil {
		return fmt.Errorf("failed to delete service: %w", err)
	}
	if result.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// List returns services with pagination.
func (r *serviceRepo) List(ctx context.Context, page Pagination) ([]Service, error) {
	rows, err := r.db.pool.Query(ctx, ServiceList, page.Limit, page.Offset)
	if err != nil {
		return nil, fmt.Errorf("failed to list services: %w", err)
	}
	defer rows.Close()

	return scanServices(rows)
}

// Count returns the total number of services.
func (r *serviceRepo) Count(ctx context.Context) (int64, error) {
	var count int64
	err := r.db.pool.QueryRow(ctx, ServiceCount).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to count services: %w", err)
	}
	return count, nil
}

// ListByOwner returns services owned by a specific owner.
func (r *serviceRepo) ListByOwner(ctx context.Context, owner string, page Pagination) ([]Service, error) {
	rows, err := r.db.pool.Query(ctx, ServiceListByOwner, owner, page.Limit, page.Offset)
	if err != nil {
		return nil, fmt.Errorf("failed to list services by owner: %w", err)
	}
	defer rows.Close()

	return scanServices(rows)
}

// Search searches services by name pattern.
func (r *serviceRepo) Search(ctx context.Context, query string, page Pagination) ([]Service, error) {
	// Add wildcards for ILIKE pattern matching
	pattern := "%" + query + "%"
	rows, err := r.db.pool.Query(ctx, ServiceSearch, pattern, page.Limit, page.Offset)
	if err != nil {
		return nil, fmt.Errorf("failed to search services: %w", err)
	}
	defer rows.Close()

	return scanServices(rows)
}

// scanServices scans rows into a slice of services.
func scanServices(rows pgx.Rows) ([]Service, error) {
	var services []Service
	for rows.Next() {
		var svc Service
		err := rows.Scan(
			&svc.ID,
			&svc.Name,
			&svc.DisplayName,
			&svc.GitURL,
			&svc.GitProvider,
			&svc.DefaultBranch,
			&svc.NetworkZones,
			&svc.Owner,
			&svc.ContactSlack,
			&svc.ContactEmail,
			&svc.CreatedAt,
			&svc.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan service: %w", err)
		}
		services = append(services, svc)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating services: %w", err)
	}

	return services, nil
}

// testDefinitionRepo implements TestDefinitionRepository.
type testDefinitionRepo struct {
	db *DB
}

// NewTestDefinitionRepo creates a new test definition repository.
func NewTestDefinitionRepo(db *DB) TestDefinitionRepository {
	return &testDefinitionRepo{db: db}
}

// Create creates a new test definition.
func (r *testDefinitionRepo) Create(ctx context.Context, def *TestDefinition) error {
	err := r.db.pool.QueryRow(ctx, TestDefInsert,
		def.ServiceID,
		def.Name,
		def.Description,
		def.ExecutionType,
		def.Command,
		def.Args,
		def.TimeoutSeconds,
		def.ResultFile,
		def.ResultFormat,
		def.ArtifactPatterns,
		def.Tags,
		def.DependsOn,
		def.Retries,
		def.AllowFailure,
	).Scan(&def.ID, &def.CreatedAt, &def.UpdatedAt)

	if err != nil {
		return fmt.Errorf("failed to create test definition: %w", WrapDBError(err))
	}
	return nil
}

// Get retrieves a test definition by ID.
func (r *testDefinitionRepo) Get(ctx context.Context, id uuid.UUID) (*TestDefinition, error) {
	def := &TestDefinition{}
	err := r.db.pool.QueryRow(ctx, TestDefGetByID, id).Scan(
		&def.ID,
		&def.ServiceID,
		&def.Name,
		&def.Description,
		&def.ExecutionType,
		&def.Command,
		&def.Args,
		&def.TimeoutSeconds,
		&def.ResultFile,
		&def.ResultFormat,
		&def.ArtifactPatterns,
		&def.Tags,
		&def.DependsOn,
		&def.Retries,
		&def.AllowFailure,
		&def.CreatedAt,
		&def.UpdatedAt,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("failed to get test definition: %w", err)
	}
	return def, nil
}

// Update updates a test definition.
func (r *testDefinitionRepo) Update(ctx context.Context, def *TestDefinition) error {
	err := r.db.pool.QueryRow(ctx, TestDefUpdate,
		def.ID,
		def.Name,
		def.Description,
		def.ExecutionType,
		def.Command,
		def.Args,
		def.TimeoutSeconds,
		def.ResultFile,
		def.ResultFormat,
		def.ArtifactPatterns,
		def.Tags,
		def.DependsOn,
		def.Retries,
		def.AllowFailure,
	).Scan(&def.UpdatedAt)

	if err != nil {
		if err == pgx.ErrNoRows {
			return ErrNotFound
		}
		return fmt.Errorf("failed to update test definition: %w", WrapDBError(err))
	}
	return nil
}

// Delete deletes a test definition.
func (r *testDefinitionRepo) Delete(ctx context.Context, id uuid.UUID) error {
	result, err := r.db.pool.Exec(ctx, TestDefDelete, id)
	if err != nil {
		return fmt.Errorf("failed to delete test definition: %w", err)
	}
	if result.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// ListByService returns test definitions for a service.
func (r *testDefinitionRepo) ListByService(ctx context.Context, serviceID uuid.UUID, page Pagination) ([]TestDefinition, error) {
	rows, err := r.db.pool.Query(ctx, TestDefListByService, serviceID, page.Limit, page.Offset)
	if err != nil {
		return nil, fmt.Errorf("failed to list test definitions: %w", err)
	}
	defer rows.Close()

	return scanTestDefinitions(rows)
}

// ListByTags returns test definitions matching any of the given tags.
func (r *testDefinitionRepo) ListByTags(ctx context.Context, serviceID uuid.UUID, tags []string, page Pagination) ([]TestDefinition, error) {
	rows, err := r.db.pool.Query(ctx, TestDefListByTags, serviceID, tags, page.Limit, page.Offset)
	if err != nil {
		return nil, fmt.Errorf("failed to list test definitions by tags: %w", err)
	}
	defer rows.Close()

	return scanTestDefinitions(rows)
}

// scanTestDefinitions scans rows into a slice of test definitions.
func scanTestDefinitions(rows pgx.Rows) ([]TestDefinition, error) {
	var defs []TestDefinition
	for rows.Next() {
		var def TestDefinition
		err := rows.Scan(
			&def.ID,
			&def.ServiceID,
			&def.Name,
			&def.Description,
			&def.ExecutionType,
			&def.Command,
			&def.Args,
			&def.TimeoutSeconds,
			&def.ResultFile,
			&def.ResultFormat,
			&def.ArtifactPatterns,
			&def.Tags,
			&def.DependsOn,
			&def.Retries,
			&def.AllowFailure,
			&def.CreatedAt,
			&def.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan test definition: %w", err)
		}
		defs = append(defs, def)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating test definitions: %w", err)
	}

	return defs, nil
}
