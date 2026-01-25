//go:build integration

// Package database provides integration tests for database operations.
package database

import (
	"context"
	"io/fs"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/conductor/conductor/pkg/testutil"
)

// newDBFromContainer creates a database connection from a postgres container.
func newDBFromContainer(ctx context.Context, pg *testutil.PostgresContainer) (*DB, error) {
	cfg := DefaultConfig(pg.ConnStr)
	cfg.MaxConns = 5
	cfg.MinConns = 1
	return New(ctx, cfg)
}

// testDB holds the shared database container for tests.
var testDB struct {
	container *testutil.PostgresContainer
	db        *DB
}

func TestMain(m *testing.M) {
	if !testutil.IsDockerAvailable() {
		os.Exit(0) // Skip if Docker is not available
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	// Start postgres container
	pg, err := testutil.NewPostgresContainer(ctx, testutil.DefaultPostgresConfig())
	if err != nil {
		panic("failed to start postgres container: " + err.Error())
	}
	testDB.container = pg

	// Create database connection
	db, err := newDBFromContainer(ctx, pg)
	if err != nil {
		pg.Terminate(ctx)
		panic("failed to create database connection: " + err.Error())
	}
	testDB.db = db

	// Run migrations
	migrationsFS := os.DirFS("../../migrations")
	migrator, err := NewMigratorFromFS(db, migrationsFS)
	if err != nil {
		db.Close()
		pg.Terminate(ctx)
		panic("failed to create migrator: " + err.Error())
	}

	if _, err := migrator.Up(ctx); err != nil {
		db.Close()
		pg.Terminate(ctx)
		panic("failed to run migrations: " + err.Error())
	}

	// Run tests
	code := m.Run()

	// Cleanup
	db.Close()
	pg.Terminate(context.Background())

	os.Exit(code)
}

// ============================================================================
// MIGRATION TESTS
// ============================================================================

func TestMigrations(t *testing.T) {
	if !testutil.IsDockerAvailable() {
		t.Skip("Docker not available")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	// Create a fresh container for migration tests
	pg, err := testutil.NewPostgresContainer(ctx, testutil.DefaultPostgresConfig())
	require.NoError(t, err)
	defer pg.Terminate(ctx)

	db, err := newDBFromContainer(ctx, pg)
	require.NoError(t, err)
	defer db.Close()

	migrationsFS := os.DirFS("../../migrations")

	t.Run("Up", func(t *testing.T) {
		migrator, err := NewMigratorFromFS(db, migrationsFS)
		require.NoError(t, err)

		// Apply all migrations
		count, err := migrator.Up(ctx)
		require.NoError(t, err)
		assert.Equal(t, 4, count, "should apply 4 migrations")

		// Verify migrations were recorded
		version, err := migrator.Version(ctx)
		require.NoError(t, err)
		assert.Equal(t, "20260125000004", version)
	})

	t.Run("Status", func(t *testing.T) {
		migrator, err := NewMigratorFromFS(db, migrationsFS)
		require.NoError(t, err)

		statuses, err := migrator.Status(ctx)
		require.NoError(t, err)
		assert.Len(t, statuses, 4)

		// All should be applied
		for _, s := range statuses {
			assert.True(t, s.Applied, "migration %s should be applied", s.Version)
			assert.NotNil(t, s.AppliedAt)
		}
	})

	t.Run("Down", func(t *testing.T) {
		migrator, err := NewMigratorFromFS(db, migrationsFS)
		require.NoError(t, err)

		// Rollback one migration
		err = migrator.Down(ctx)
		require.NoError(t, err)

		version, err := migrator.Version(ctx)
		require.NoError(t, err)
		assert.Equal(t, "20260125000003", version)
	})

	t.Run("DownN", func(t *testing.T) {
		migrator, err := NewMigratorFromFS(db, migrationsFS)
		require.NoError(t, err)

		// Rollback two more migrations
		err = migrator.DownN(ctx, 2)
		require.NoError(t, err)

		version, err := migrator.Version(ctx)
		require.NoError(t, err)
		assert.Equal(t, "20260125000001", version)
	})

	t.Run("Reset", func(t *testing.T) {
		migrator, err := NewMigratorFromFS(db, migrationsFS)
		require.NoError(t, err)

		// Reset (rollback all and re-apply)
		err = migrator.Reset(ctx)
		require.NoError(t, err)

		version, err := migrator.Version(ctx)
		require.NoError(t, err)
		assert.Equal(t, "20260125000004", version)
	})

	t.Run("Pending", func(t *testing.T) {
		// Create new migrator with fresh container
		pg2, err := testutil.NewPostgresContainer(ctx, testutil.DefaultPostgresConfig())
		require.NoError(t, err)
		defer pg2.Terminate(ctx)

		db2, err := newDBFromContainer(ctx, pg2)
		require.NoError(t, err)
		defer db2.Close()

		migrator, err := NewMigratorFromFS(db2, migrationsFS)
		require.NoError(t, err)

		pending, err := migrator.Pending(ctx)
		require.NoError(t, err)
		assert.Len(t, pending, 4, "should have 4 pending migrations")
	})
}

// ============================================================================
// SERVICE REPOSITORY TESTS
// ============================================================================

func TestServiceRepository(t *testing.T) {
	ctx := context.Background()
	repo := NewServiceRepo(testDB.db)

	t.Run("Create", func(t *testing.T) {
		svc := &Service{
			Name:          "test-service-create-" + uuid.New().String()[:8],
			GitURL:        "https://github.com/example/repo.git",
			DefaultBranch: "main",
			NetworkZones:  []string{"zone-a", "zone-b"},
		}

		err := repo.Create(ctx, svc)
		require.NoError(t, err)
		assert.NotEqual(t, uuid.Nil, svc.ID)
		assert.False(t, svc.CreatedAt.IsZero())
		assert.False(t, svc.UpdatedAt.IsZero())

		// Cleanup
		t.Cleanup(func() {
			repo.Delete(ctx, svc.ID)
		})
	})

	t.Run("Create_DuplicateName", func(t *testing.T) {
		name := "test-service-dup-" + uuid.New().String()[:8]
		svc1 := &Service{
			Name:          name,
			GitURL:        "https://github.com/example/repo1.git",
			DefaultBranch: "main",
		}
		err := repo.Create(ctx, svc1)
		require.NoError(t, err)
		defer repo.Delete(ctx, svc1.ID)

		svc2 := &Service{
			Name:          name,
			GitURL:        "https://github.com/example/repo2.git",
			DefaultBranch: "main",
		}
		err = repo.Create(ctx, svc2)
		assert.Error(t, err)
		assert.True(t, IsDuplicate(err))
	})

	t.Run("Get", func(t *testing.T) {
		svc := &Service{
			Name:          "test-service-get-" + uuid.New().String()[:8],
			GitURL:        "https://github.com/example/repo.git",
			DefaultBranch: "develop",
			NetworkZones:  []string{"default"},
		}
		err := repo.Create(ctx, svc)
		require.NoError(t, err)
		defer repo.Delete(ctx, svc.ID)

		// Fetch by ID
		fetched, err := repo.Get(ctx, svc.ID)
		require.NoError(t, err)
		assert.Equal(t, svc.Name, fetched.Name)
		assert.Equal(t, svc.GitURL, fetched.GitURL)
		assert.Equal(t, svc.DefaultBranch, fetched.DefaultBranch)
		assert.Equal(t, svc.NetworkZones, fetched.NetworkZones)
	})

	t.Run("Get_NotFound", func(t *testing.T) {
		_, err := repo.Get(ctx, uuid.New())
		assert.Error(t, err)
		assert.True(t, IsNotFound(err))
	})

	t.Run("GetByName", func(t *testing.T) {
		name := "test-service-getbyname-" + uuid.New().String()[:8]
		svc := &Service{
			Name:          name,
			GitURL:        "https://github.com/example/repo.git",
			DefaultBranch: "main",
		}
		err := repo.Create(ctx, svc)
		require.NoError(t, err)
		defer repo.Delete(ctx, svc.ID)

		fetched, err := repo.GetByName(ctx, name)
		require.NoError(t, err)
		assert.Equal(t, svc.ID, fetched.ID)
	})

	t.Run("Update", func(t *testing.T) {
		svc := &Service{
			Name:          "test-service-update-" + uuid.New().String()[:8],
			GitURL:        "https://github.com/example/repo.git",
			DefaultBranch: "main",
		}
		err := repo.Create(ctx, svc)
		require.NoError(t, err)
		defer repo.Delete(ctx, svc.ID)

		originalUpdatedAt := svc.UpdatedAt
		time.Sleep(10 * time.Millisecond) // Ensure time difference

		// Update fields
		svc.DefaultBranch = "develop"
		owner := "platform-team"
		svc.Owner = &owner

		err = repo.Update(ctx, svc)
		require.NoError(t, err)
		assert.True(t, svc.UpdatedAt.After(originalUpdatedAt))

		// Verify update
		fetched, err := repo.Get(ctx, svc.ID)
		require.NoError(t, err)
		assert.Equal(t, "develop", fetched.DefaultBranch)
		assert.Equal(t, "platform-team", *fetched.Owner)
	})

	t.Run("Delete", func(t *testing.T) {
		svc := &Service{
			Name:          "test-service-delete-" + uuid.New().String()[:8],
			GitURL:        "https://github.com/example/repo.git",
			DefaultBranch: "main",
		}
		err := repo.Create(ctx, svc)
		require.NoError(t, err)

		err = repo.Delete(ctx, svc.ID)
		require.NoError(t, err)

		_, err = repo.Get(ctx, svc.ID)
		assert.True(t, IsNotFound(err))
	})

	t.Run("Delete_NotFound", func(t *testing.T) {
		err := repo.Delete(ctx, uuid.New())
		assert.True(t, IsNotFound(err))
	})

	t.Run("List", func(t *testing.T) {
		// Create multiple services
		for i := 0; i < 3; i++ {
			svc := &Service{
				Name:          "test-service-list-" + uuid.New().String()[:8],
				GitURL:        "https://github.com/example/repo.git",
				DefaultBranch: "main",
			}
			err := repo.Create(ctx, svc)
			require.NoError(t, err)
			defer repo.Delete(ctx, svc.ID)
		}

		services, err := repo.List(ctx, Pagination{Limit: 10, Offset: 0})
		require.NoError(t, err)
		assert.GreaterOrEqual(t, len(services), 3)
	})

	t.Run("Count", func(t *testing.T) {
		initialCount, err := repo.Count(ctx)
		require.NoError(t, err)

		svc := &Service{
			Name:          "test-service-count-" + uuid.New().String()[:8],
			GitURL:        "https://github.com/example/repo.git",
			DefaultBranch: "main",
		}
		err = repo.Create(ctx, svc)
		require.NoError(t, err)
		defer repo.Delete(ctx, svc.ID)

		newCount, err := repo.Count(ctx)
		require.NoError(t, err)
		assert.Equal(t, initialCount+1, newCount)
	})

	t.Run("ListByOwner", func(t *testing.T) {
		owner := "test-owner-" + uuid.New().String()[:8]
		for i := 0; i < 2; i++ {
			svc := &Service{
				Name:          "test-service-owner-" + uuid.New().String()[:8],
				GitURL:        "https://github.com/example/repo.git",
				DefaultBranch: "main",
				Owner:         &owner,
			}
			err := repo.Create(ctx, svc)
			require.NoError(t, err)
			defer repo.Delete(ctx, svc.ID)
		}

		services, err := repo.ListByOwner(ctx, owner, DefaultPagination())
		require.NoError(t, err)
		assert.Len(t, services, 2)
		for _, svc := range services {
			assert.Equal(t, owner, *svc.Owner)
		}
	})

	t.Run("Search", func(t *testing.T) {
		prefix := "searchable-" + uuid.New().String()[:4]
		svc := &Service{
			Name:          prefix + "-service",
			GitURL:        "https://github.com/example/repo.git",
			DefaultBranch: "main",
		}
		err := repo.Create(ctx, svc)
		require.NoError(t, err)
		defer repo.Delete(ctx, svc.ID)

		services, err := repo.Search(ctx, prefix, DefaultPagination())
		require.NoError(t, err)
		assert.Len(t, services, 1)
		assert.Equal(t, svc.Name, services[0].Name)
	})
}

// ============================================================================
// TEST DEFINITION REPOSITORY TESTS
// ============================================================================

func TestTestDefinitionRepository(t *testing.T) {
	ctx := context.Background()
	svcRepo := NewServiceRepo(testDB.db)
	defRepo := NewTestDefinitionRepo(testDB.db)

	// Create a service for test definitions
	svc := &Service{
		Name:          "test-def-service-" + uuid.New().String()[:8],
		GitURL:        "https://github.com/example/repo.git",
		DefaultBranch: "main",
	}
	err := svcRepo.Create(ctx, svc)
	require.NoError(t, err)
	defer svcRepo.Delete(ctx, svc.ID)

	t.Run("Create", func(t *testing.T) {
		def := &TestDefinition{
			ServiceID:      svc.ID,
			Name:           "test-definition-" + uuid.New().String()[:8],
			ExecutionType:  "subprocess",
			Command:        "go",
			Args:           []string{"test", "-v", "./..."},
			TimeoutSeconds: 300,
			Tags:           []string{"unit", "fast"},
		}

		err := defRepo.Create(ctx, def)
		require.NoError(t, err)
		assert.NotEqual(t, uuid.Nil, def.ID)

		t.Cleanup(func() {
			defRepo.Delete(ctx, def.ID)
		})
	})

	t.Run("Create_DuplicateName", func(t *testing.T) {
		name := "test-def-dup-" + uuid.New().String()[:8]
		def1 := &TestDefinition{
			ServiceID:      svc.ID,
			Name:           name,
			ExecutionType:  "subprocess",
			Command:        "go",
			Args:           []string{"test"},
			TimeoutSeconds: 300,
		}
		err := defRepo.Create(ctx, def1)
		require.NoError(t, err)
		defer defRepo.Delete(ctx, def1.ID)

		def2 := &TestDefinition{
			ServiceID:      svc.ID,
			Name:           name,
			ExecutionType:  "container",
			Command:        "npm",
			Args:           []string{"test"},
			TimeoutSeconds: 600,
		}
		err = defRepo.Create(ctx, def2)
		assert.Error(t, err)
		assert.True(t, IsDuplicate(err))
	})

	t.Run("Get", func(t *testing.T) {
		def := &TestDefinition{
			ServiceID:      svc.ID,
			Name:           "test-def-get-" + uuid.New().String()[:8],
			ExecutionType:  "subprocess",
			Command:        "pytest",
			Args:           []string{"-v"},
			TimeoutSeconds: 600,
			Tags:           []string{"integration"},
		}
		err := defRepo.Create(ctx, def)
		require.NoError(t, err)
		defer defRepo.Delete(ctx, def.ID)

		fetched, err := defRepo.Get(ctx, def.ID)
		require.NoError(t, err)
		assert.Equal(t, def.Name, fetched.Name)
		assert.Equal(t, def.Command, fetched.Command)
		assert.Equal(t, def.Args, fetched.Args)
		assert.Equal(t, def.Tags, fetched.Tags)
	})

	t.Run("Update", func(t *testing.T) {
		def := &TestDefinition{
			ServiceID:      svc.ID,
			Name:           "test-def-update-" + uuid.New().String()[:8],
			ExecutionType:  "subprocess",
			Command:        "go",
			Args:           []string{"test"},
			TimeoutSeconds: 300,
		}
		err := defRepo.Create(ctx, def)
		require.NoError(t, err)
		defer defRepo.Delete(ctx, def.ID)

		// Update
		def.TimeoutSeconds = 600
		def.Tags = []string{"updated", "slow"}

		err = defRepo.Update(ctx, def)
		require.NoError(t, err)

		fetched, err := defRepo.Get(ctx, def.ID)
		require.NoError(t, err)
		assert.Equal(t, 600, fetched.TimeoutSeconds)
		assert.Equal(t, []string{"updated", "slow"}, fetched.Tags)
	})

	t.Run("ListByService", func(t *testing.T) {
		for i := 0; i < 3; i++ {
			def := &TestDefinition{
				ServiceID:      svc.ID,
				Name:           "test-def-list-" + uuid.New().String()[:8],
				ExecutionType:  "subprocess",
				Command:        "test",
				TimeoutSeconds: 300,
			}
			err := defRepo.Create(ctx, def)
			require.NoError(t, err)
			defer defRepo.Delete(ctx, def.ID)
		}

		defs, err := defRepo.ListByService(ctx, svc.ID, DefaultPagination())
		require.NoError(t, err)
		assert.GreaterOrEqual(t, len(defs), 3)
	})

	t.Run("ListByTags", func(t *testing.T) {
		def := &TestDefinition{
			ServiceID:      svc.ID,
			Name:           "test-def-tags-" + uuid.New().String()[:8],
			ExecutionType:  "subprocess",
			Command:        "test",
			TimeoutSeconds: 300,
			Tags:           []string{"special-tag-" + uuid.New().String()[:8]},
		}
		err := defRepo.Create(ctx, def)
		require.NoError(t, err)
		defer defRepo.Delete(ctx, def.ID)

		defs, err := defRepo.ListByTags(ctx, svc.ID, def.Tags, DefaultPagination())
		require.NoError(t, err)
		assert.Len(t, defs, 1)
		assert.Equal(t, def.ID, defs[0].ID)
	})

	t.Run("Delete", func(t *testing.T) {
		def := &TestDefinition{
			ServiceID:      svc.ID,
			Name:           "test-def-delete-" + uuid.New().String()[:8],
			ExecutionType:  "subprocess",
			Command:        "test",
			TimeoutSeconds: 300,
		}
		err := defRepo.Create(ctx, def)
		require.NoError(t, err)

		err = defRepo.Delete(ctx, def.ID)
		require.NoError(t, err)

		_, err = defRepo.Get(ctx, def.ID)
		assert.True(t, IsNotFound(err))
	})
}

// ============================================================================
// AGENT REPOSITORY TESTS
// ============================================================================

func TestAgentRepository(t *testing.T) {
	ctx := context.Background()
	repo := NewAgentRepo(testDB.db)

	t.Run("Create", func(t *testing.T) {
		agent := &Agent{
			Name:            "test-agent-create-" + uuid.New().String()[:8],
			Status:          AgentStatusIdle,
			NetworkZones:    []string{"zone-a"},
			MaxParallel:     4,
			DockerAvailable: true,
		}

		err := repo.Create(ctx, agent)
		require.NoError(t, err)
		assert.NotEqual(t, uuid.Nil, agent.ID)

		t.Cleanup(func() {
			repo.Delete(ctx, agent.ID)
		})
	})

	t.Run("Get", func(t *testing.T) {
		agent := &Agent{
			Name:            "test-agent-get-" + uuid.New().String()[:8],
			Status:          AgentStatusIdle,
			NetworkZones:    []string{"zone-a", "zone-b"},
			MaxParallel:     8,
			DockerAvailable: false,
		}
		err := repo.Create(ctx, agent)
		require.NoError(t, err)
		defer repo.Delete(ctx, agent.ID)

		fetched, err := repo.Get(ctx, agent.ID)
		require.NoError(t, err)
		assert.Equal(t, agent.Name, fetched.Name)
		assert.Equal(t, agent.Status, fetched.Status)
		assert.Equal(t, agent.NetworkZones, fetched.NetworkZones)
		assert.Equal(t, agent.MaxParallel, fetched.MaxParallel)
	})

	t.Run("GetByName", func(t *testing.T) {
		name := "test-agent-byname-" + uuid.New().String()[:8]
		agent := &Agent{
			Name:         name,
			Status:       AgentStatusIdle,
			NetworkZones: []string{"default"},
			MaxParallel:  2,
		}
		err := repo.Create(ctx, agent)
		require.NoError(t, err)
		defer repo.Delete(ctx, agent.ID)

		fetched, err := repo.GetByName(ctx, name)
		require.NoError(t, err)
		assert.Equal(t, agent.ID, fetched.ID)
	})

	t.Run("UpdateStatus", func(t *testing.T) {
		agent := &Agent{
			Name:         "test-agent-status-" + uuid.New().String()[:8],
			Status:       AgentStatusIdle,
			NetworkZones: []string{"default"},
			MaxParallel:  4,
		}
		err := repo.Create(ctx, agent)
		require.NoError(t, err)
		defer repo.Delete(ctx, agent.ID)

		err = repo.UpdateStatus(ctx, agent.ID, AgentStatusBusy)
		require.NoError(t, err)

		fetched, err := repo.Get(ctx, agent.ID)
		require.NoError(t, err)
		assert.Equal(t, AgentStatusBusy, fetched.Status)
	})

	t.Run("UpdateHeartbeat", func(t *testing.T) {
		agent := &Agent{
			Name:         "test-agent-heartbeat-" + uuid.New().String()[:8],
			Status:       AgentStatusOffline,
			NetworkZones: []string{"default"},
			MaxParallel:  4,
		}
		err := repo.Create(ctx, agent)
		require.NoError(t, err)
		defer repo.Delete(ctx, agent.ID)

		err = repo.UpdateHeartbeat(ctx, agent.ID, AgentStatusIdle)
		require.NoError(t, err)

		fetched, err := repo.Get(ctx, agent.ID)
		require.NoError(t, err)
		assert.Equal(t, AgentStatusIdle, fetched.Status)
		assert.NotNil(t, fetched.LastHeartbeat)
		assert.WithinDuration(t, time.Now(), *fetched.LastHeartbeat, 5*time.Second)
	})

	t.Run("ListByStatus", func(t *testing.T) {
		status := AgentStatusDraining
		agent := &Agent{
			Name:         "test-agent-listbystatus-" + uuid.New().String()[:8],
			Status:       status,
			NetworkZones: []string{"default"},
			MaxParallel:  4,
		}
		err := repo.Create(ctx, agent)
		require.NoError(t, err)
		defer repo.Delete(ctx, agent.ID)

		agents, err := repo.ListByStatus(ctx, status, DefaultPagination())
		require.NoError(t, err)
		assert.GreaterOrEqual(t, len(agents), 1)

		found := false
		for _, a := range agents {
			if a.ID == agent.ID {
				found = true
				break
			}
		}
		assert.True(t, found, "created agent should be in list")
	})

	t.Run("GetAvailable", func(t *testing.T) {
		zone := "test-zone-" + uuid.New().String()[:8]
		agent := &Agent{
			Name:         "test-agent-available-" + uuid.New().String()[:8],
			Status:       AgentStatusIdle,
			NetworkZones: []string{zone},
			MaxParallel:  4,
		}
		err := repo.Create(ctx, agent)
		require.NoError(t, err)

		// Update heartbeat to mark as online
		err = repo.UpdateHeartbeat(ctx, agent.ID, AgentStatusIdle)
		require.NoError(t, err)
		defer repo.Delete(ctx, agent.ID)

		agents, err := repo.GetAvailable(ctx, []string{zone}, 10)
		require.NoError(t, err)

		found := false
		for _, a := range agents {
			if a.ID == agent.ID {
				found = true
				break
			}
		}
		assert.True(t, found, "idle agent with matching zone should be available")
	})

	t.Run("CountByStatus", func(t *testing.T) {
		agent := &Agent{
			Name:         "test-agent-countbystatus-" + uuid.New().String()[:8],
			Status:       AgentStatusIdle,
			NetworkZones: []string{"default"},
			MaxParallel:  4,
		}
		err := repo.Create(ctx, agent)
		require.NoError(t, err)
		defer repo.Delete(ctx, agent.ID)

		counts, err := repo.CountByStatus(ctx)
		require.NoError(t, err)
		assert.GreaterOrEqual(t, counts[AgentStatusIdle], int64(1))
	})
}

// ============================================================================
// TEST RUN REPOSITORY TESTS
// ============================================================================

func TestTestRunRepository(t *testing.T) {
	ctx := context.Background()
	svcRepo := NewServiceRepo(testDB.db)
	runRepo := NewRunRepo(testDB.db)
	agentRepo := NewAgentRepo(testDB.db)

	// Create a service
	svc := &Service{
		Name:          "test-run-service-" + uuid.New().String()[:8],
		GitURL:        "https://github.com/example/repo.git",
		DefaultBranch: "main",
	}
	err := svcRepo.Create(ctx, svc)
	require.NoError(t, err)
	defer svcRepo.Delete(ctx, svc.ID)

	t.Run("Create", func(t *testing.T) {
		gitRef := "main"
		gitSHA := "abc123"
		triggerType := TriggerTypeManual
		triggeredBy := "test-user"

		run := &TestRun{
			ServiceID:   svc.ID,
			Status:      RunStatusPending,
			GitRef:      &gitRef,
			GitSHA:      &gitSHA,
			TriggerType: &triggerType,
			TriggeredBy: &triggeredBy,
			Priority:    1,
		}

		err := runRepo.Create(ctx, run)
		require.NoError(t, err)
		assert.NotEqual(t, uuid.Nil, run.ID)
		assert.False(t, run.CreatedAt.IsZero())

		t.Cleanup(func() {
			testDB.db.Pool().Exec(ctx, "DELETE FROM test_runs WHERE id = $1", run.ID)
		})
	})

	t.Run("Get", func(t *testing.T) {
		gitRef := "feature-branch"
		run := &TestRun{
			ServiceID: svc.ID,
			Status:    RunStatusPending,
			GitRef:    &gitRef,
		}
		err := runRepo.Create(ctx, run)
		require.NoError(t, err)
		defer testDB.db.Pool().Exec(ctx, "DELETE FROM test_runs WHERE id = $1", run.ID)

		fetched, err := runRepo.Get(ctx, run.ID)
		require.NoError(t, err)
		assert.Equal(t, run.ServiceID, fetched.ServiceID)
		assert.Equal(t, run.Status, fetched.Status)
		assert.Equal(t, *run.GitRef, *fetched.GitRef)
	})

	t.Run("UpdateStatus", func(t *testing.T) {
		run := &TestRun{
			ServiceID: svc.ID,
			Status:    RunStatusPending,
		}
		err := runRepo.Create(ctx, run)
		require.NoError(t, err)
		defer testDB.db.Pool().Exec(ctx, "DELETE FROM test_runs WHERE id = $1", run.ID)

		err = runRepo.UpdateStatus(ctx, run.ID, RunStatusRunning)
		require.NoError(t, err)

		fetched, err := runRepo.Get(ctx, run.ID)
		require.NoError(t, err)
		assert.Equal(t, RunStatusRunning, fetched.Status)
	})

	t.Run("Start", func(t *testing.T) {
		// Create an agent
		agent := &Agent{
			Name:         "test-run-agent-" + uuid.New().String()[:8],
			Status:       AgentStatusIdle,
			NetworkZones: []string{"default"},
			MaxParallel:  4,
		}
		err := agentRepo.Create(ctx, agent)
		require.NoError(t, err)
		defer agentRepo.Delete(ctx, agent.ID)

		run := &TestRun{
			ServiceID: svc.ID,
			Status:    RunStatusPending,
		}
		err = runRepo.Create(ctx, run)
		require.NoError(t, err)
		defer testDB.db.Pool().Exec(ctx, "DELETE FROM test_runs WHERE id = $1", run.ID)

		err = runRepo.Start(ctx, run.ID, agent.ID)
		require.NoError(t, err)

		fetched, err := runRepo.Get(ctx, run.ID)
		require.NoError(t, err)
		assert.Equal(t, RunStatusRunning, fetched.Status)
		assert.Equal(t, agent.ID, *fetched.AgentID)
		assert.NotNil(t, fetched.StartedAt)
	})

	t.Run("Finish", func(t *testing.T) {
		run := &TestRun{
			ServiceID: svc.ID,
			Status:    RunStatusRunning,
		}
		err := runRepo.Create(ctx, run)
		require.NoError(t, err)
		defer testDB.db.Pool().Exec(ctx, "DELETE FROM test_runs WHERE id = $1", run.ID)

		results := RunResults{
			TotalTests:   10,
			PassedTests:  8,
			FailedTests:  1,
			SkippedTests: 1,
			DurationMs:   5000,
		}

		err = runRepo.Finish(ctx, run.ID, RunStatusFailed, results)
		require.NoError(t, err)

		fetched, err := runRepo.Get(ctx, run.ID)
		require.NoError(t, err)
		assert.Equal(t, RunStatusFailed, fetched.Status)
		assert.Equal(t, 10, fetched.TotalTests)
		assert.Equal(t, 8, fetched.PassedTests)
		assert.Equal(t, 1, fetched.FailedTests)
		assert.Equal(t, 1, fetched.SkippedTests)
		assert.NotNil(t, fetched.FinishedAt)
	})

	t.Run("ListByService", func(t *testing.T) {
		for i := 0; i < 3; i++ {
			run := &TestRun{
				ServiceID: svc.ID,
				Status:    RunStatusPending,
			}
			err := runRepo.Create(ctx, run)
			require.NoError(t, err)
			defer testDB.db.Pool().Exec(ctx, "DELETE FROM test_runs WHERE id = $1", run.ID)
		}

		runs, err := runRepo.ListByService(ctx, svc.ID, DefaultPagination())
		require.NoError(t, err)
		assert.GreaterOrEqual(t, len(runs), 3)
	})

	t.Run("ListByStatus", func(t *testing.T) {
		run := &TestRun{
			ServiceID: svc.ID,
			Status:    RunStatusCancelled,
		}
		err := runRepo.Create(ctx, run)
		require.NoError(t, err)
		defer testDB.db.Pool().Exec(ctx, "DELETE FROM test_runs WHERE id = $1", run.ID)

		runs, err := runRepo.ListByStatus(ctx, RunStatusCancelled, DefaultPagination())
		require.NoError(t, err)

		found := false
		for _, r := range runs {
			if r.ID == run.ID {
				found = true
				break
			}
		}
		assert.True(t, found)
	})

	t.Run("GetPending", func(t *testing.T) {
		// Create runs with different priorities
		for i := 0; i < 3; i++ {
			run := &TestRun{
				ServiceID: svc.ID,
				Status:    RunStatusPending,
				Priority:  i * 10,
			}
			err := runRepo.Create(ctx, run)
			require.NoError(t, err)
			defer testDB.db.Pool().Exec(ctx, "DELETE FROM test_runs WHERE id = $1", run.ID)
		}

		runs, err := runRepo.GetPending(ctx, 10)
		require.NoError(t, err)
		assert.GreaterOrEqual(t, len(runs), 3)

		// Should be ordered by priority (highest first)
		for i := 1; i < len(runs); i++ {
			assert.GreaterOrEqual(t, runs[i-1].Priority, runs[i].Priority)
		}
	})

	t.Run("GetRunning", func(t *testing.T) {
		run := &TestRun{
			ServiceID: svc.ID,
			Status:    RunStatusRunning,
		}
		err := runRepo.Create(ctx, run)
		require.NoError(t, err)
		defer testDB.db.Pool().Exec(ctx, "DELETE FROM test_runs WHERE id = $1", run.ID)

		runs, err := runRepo.GetRunning(ctx)
		require.NoError(t, err)

		found := false
		for _, r := range runs {
			if r.ID == run.ID {
				found = true
				break
			}
		}
		assert.True(t, found)
	})

	t.Run("CountByStatus", func(t *testing.T) {
		run := &TestRun{
			ServiceID: svc.ID,
			Status:    RunStatusPending,
		}
		err := runRepo.Create(ctx, run)
		require.NoError(t, err)
		defer testDB.db.Pool().Exec(ctx, "DELETE FROM test_runs WHERE id = $1", run.ID)

		counts, err := runRepo.CountByStatus(ctx)
		require.NoError(t, err)
		assert.GreaterOrEqual(t, counts[RunStatusPending], int64(1))
	})
}

// ============================================================================
// RESULT REPOSITORY TESTS
// ============================================================================

func TestResultRepository(t *testing.T) {
	ctx := context.Background()
	svcRepo := NewServiceRepo(testDB.db)
	runRepo := NewRunRepo(testDB.db)
	resultRepo := NewResultRepo(testDB.db)

	// Create service and run
	svc := &Service{
		Name:          "test-result-service-" + uuid.New().String()[:8],
		GitURL:        "https://github.com/example/repo.git",
		DefaultBranch: "main",
	}
	err := svcRepo.Create(ctx, svc)
	require.NoError(t, err)
	defer svcRepo.Delete(ctx, svc.ID)

	run := &TestRun{
		ServiceID: svc.ID,
		Status:    RunStatusRunning,
	}
	err = runRepo.Create(ctx, run)
	require.NoError(t, err)
	defer testDB.db.Pool().Exec(ctx, "DELETE FROM test_runs WHERE id = $1", run.ID)

	t.Run("Create", func(t *testing.T) {
		durationMs := int64(150)
		result := &TestResult{
			RunID:      run.ID,
			TestName:   "TestExample",
			SuiteName:  NullString("ExampleSuite"),
			Status:     ResultStatusPass,
			DurationMs: &durationMs,
		}

		err := resultRepo.Create(ctx, result)
		require.NoError(t, err)
		assert.NotEqual(t, uuid.Nil, result.ID)

		t.Cleanup(func() {
			testDB.db.Pool().Exec(ctx, "DELETE FROM test_results WHERE id = $1", result.ID)
		})
	})

	t.Run("BatchCreate", func(t *testing.T) {
		results := make([]TestResult, 5)
		for i := 0; i < 5; i++ {
			durationMs := int64(i * 100)
			results[i] = TestResult{
				RunID:      run.ID,
				TestName:   "TestBatch" + string(rune('A'+i)),
				Status:     ResultStatusPass,
				DurationMs: &durationMs,
			}
		}

		err := resultRepo.BatchCreate(ctx, results)
		require.NoError(t, err)

		// Verify all were created
		fetched, err := resultRepo.ListByRun(ctx, run.ID)
		require.NoError(t, err)
		assert.GreaterOrEqual(t, len(fetched), 5)

		t.Cleanup(func() {
			for _, r := range results {
				testDB.db.Pool().Exec(ctx, "DELETE FROM test_results WHERE id = $1", r.ID)
			}
		})
	})

	t.Run("Get", func(t *testing.T) {
		durationMs := int64(200)
		errMsg := "assertion failed"
		result := &TestResult{
			RunID:        run.ID,
			TestName:     "TestGetExample",
			Status:       ResultStatusFail,
			DurationMs:   &durationMs,
			ErrorMessage: &errMsg,
		}
		err := resultRepo.Create(ctx, result)
		require.NoError(t, err)
		defer testDB.db.Pool().Exec(ctx, "DELETE FROM test_results WHERE id = $1", result.ID)

		fetched, err := resultRepo.Get(ctx, result.ID)
		require.NoError(t, err)
		assert.Equal(t, result.TestName, fetched.TestName)
		assert.Equal(t, result.Status, fetched.Status)
		assert.Equal(t, *result.ErrorMessage, *fetched.ErrorMessage)
	})

	t.Run("ListByRun", func(t *testing.T) {
		// Create new run for isolation
		run2 := &TestRun{
			ServiceID: svc.ID,
			Status:    RunStatusRunning,
		}
		err := runRepo.Create(ctx, run2)
		require.NoError(t, err)
		defer testDB.db.Pool().Exec(ctx, "DELETE FROM test_runs WHERE id = $1", run2.ID)

		for i := 0; i < 3; i++ {
			result := &TestResult{
				RunID:    run2.ID,
				TestName: "TestList" + string(rune('A'+i)),
				Status:   ResultStatusPass,
			}
			err := resultRepo.Create(ctx, result)
			require.NoError(t, err)
		}

		results, err := resultRepo.ListByRun(ctx, run2.ID)
		require.NoError(t, err)
		assert.Len(t, results, 3)
	})

	t.Run("ListByRunAndStatus", func(t *testing.T) {
		// Create new run for isolation
		run3 := &TestRun{
			ServiceID: svc.ID,
			Status:    RunStatusRunning,
		}
		err := runRepo.Create(ctx, run3)
		require.NoError(t, err)
		defer testDB.db.Pool().Exec(ctx, "DELETE FROM test_runs WHERE id = $1", run3.ID)

		// Create mixed results
		statuses := []ResultStatus{ResultStatusPass, ResultStatusPass, ResultStatusFail, ResultStatusSkip}
		for i, status := range statuses {
			result := &TestResult{
				RunID:    run3.ID,
				TestName: "TestStatus" + string(rune('A'+i)),
				Status:   status,
			}
			err := resultRepo.Create(ctx, result)
			require.NoError(t, err)
		}

		// Get only failed
		failed, err := resultRepo.ListByRunAndStatus(ctx, run3.ID, ResultStatusFail)
		require.NoError(t, err)
		assert.Len(t, failed, 1)
	})

	t.Run("CountByRun", func(t *testing.T) {
		// Create new run for isolation
		run4 := &TestRun{
			ServiceID: svc.ID,
			Status:    RunStatusRunning,
		}
		err := runRepo.Create(ctx, run4)
		require.NoError(t, err)
		defer testDB.db.Pool().Exec(ctx, "DELETE FROM test_runs WHERE id = $1", run4.ID)

		// Create mixed results
		for i := 0; i < 5; i++ {
			status := ResultStatusPass
			if i < 2 {
				status = ResultStatusFail
			}
			result := &TestResult{
				RunID:    run4.ID,
				TestName: "TestCount" + string(rune('A'+i)),
				Status:   status,
			}
			err := resultRepo.Create(ctx, result)
			require.NoError(t, err)
		}

		counts, err := resultRepo.CountByRun(ctx, run4.ID)
		require.NoError(t, err)
		assert.Equal(t, int64(2), counts[ResultStatusFail])
		assert.Equal(t, int64(3), counts[ResultStatusPass])
	})

	t.Run("DeleteByRun", func(t *testing.T) {
		// Create new run for isolation
		run5 := &TestRun{
			ServiceID: svc.ID,
			Status:    RunStatusRunning,
		}
		err := runRepo.Create(ctx, run5)
		require.NoError(t, err)
		defer testDB.db.Pool().Exec(ctx, "DELETE FROM test_runs WHERE id = $1", run5.ID)

		// Create results
		for i := 0; i < 3; i++ {
			result := &TestResult{
				RunID:    run5.ID,
				TestName: "TestDelete" + string(rune('A'+i)),
				Status:   ResultStatusPass,
			}
			err := resultRepo.Create(ctx, result)
			require.NoError(t, err)
		}

		err = resultRepo.DeleteByRun(ctx, run5.ID)
		require.NoError(t, err)

		results, err := resultRepo.ListByRun(ctx, run5.ID)
		require.NoError(t, err)
		assert.Len(t, results, 0)
	})
}

// ============================================================================
// NOTIFICATION REPOSITORY TESTS
// ============================================================================

func TestNotificationRepository(t *testing.T) {
	ctx := context.Background()
	repo := NewNotificationRepo(testDB.db)
	svcRepo := NewServiceRepo(testDB.db)

	t.Run("CreateChannel", func(t *testing.T) {
		channel := &NotificationChannel{
			Name:    "test-channel-" + uuid.New().String()[:8],
			Type:    ChannelTypeSlack,
			Config:  []byte(`{"webhook_url": "https://hooks.slack.com/test"}`),
			Enabled: true,
		}

		err := repo.CreateChannel(ctx, channel)
		require.NoError(t, err)
		assert.NotEqual(t, uuid.Nil, channel.ID)

		t.Cleanup(func() {
			repo.DeleteChannel(ctx, channel.ID)
		})
	})

	t.Run("GetChannel", func(t *testing.T) {
		channel := &NotificationChannel{
			Name:    "test-channel-get-" + uuid.New().String()[:8],
			Type:    ChannelTypeEmail,
			Config:  []byte(`{"recipients": ["test@example.com"]}`),
			Enabled: true,
		}
		err := repo.CreateChannel(ctx, channel)
		require.NoError(t, err)
		defer repo.DeleteChannel(ctx, channel.ID)

		fetched, err := repo.GetChannel(ctx, channel.ID)
		require.NoError(t, err)
		assert.Equal(t, channel.Name, fetched.Name)
		assert.Equal(t, channel.Type, fetched.Type)
		assert.Equal(t, channel.Enabled, fetched.Enabled)
	})

	t.Run("UpdateChannel", func(t *testing.T) {
		channel := &NotificationChannel{
			Name:    "test-channel-update-" + uuid.New().String()[:8],
			Type:    ChannelTypeWebhook,
			Config:  []byte(`{"url": "https://example.com/webhook"}`),
			Enabled: true,
		}
		err := repo.CreateChannel(ctx, channel)
		require.NoError(t, err)
		defer repo.DeleteChannel(ctx, channel.ID)

		channel.Enabled = false
		channel.Config = []byte(`{"url": "https://example.com/webhook-v2"}`)

		err = repo.UpdateChannel(ctx, channel)
		require.NoError(t, err)

		fetched, err := repo.GetChannel(ctx, channel.ID)
		require.NoError(t, err)
		assert.False(t, fetched.Enabled)
	})

	t.Run("ListChannels", func(t *testing.T) {
		for i := 0; i < 2; i++ {
			channel := &NotificationChannel{
				Name:    "test-channel-list-" + uuid.New().String()[:8],
				Type:    ChannelTypeSlack,
				Config:  []byte(`{}`),
				Enabled: true,
			}
			err := repo.CreateChannel(ctx, channel)
			require.NoError(t, err)
			defer repo.DeleteChannel(ctx, channel.ID)
		}

		channels, err := repo.ListChannels(ctx, DefaultPagination())
		require.NoError(t, err)
		assert.GreaterOrEqual(t, len(channels), 2)
	})

	t.Run("ListEnabledChannels", func(t *testing.T) {
		enabledChannel := &NotificationChannel{
			Name:    "test-channel-enabled-" + uuid.New().String()[:8],
			Type:    ChannelTypeSlack,
			Config:  []byte(`{}`),
			Enabled: true,
		}
		err := repo.CreateChannel(ctx, enabledChannel)
		require.NoError(t, err)
		defer repo.DeleteChannel(ctx, enabledChannel.ID)

		disabledChannel := &NotificationChannel{
			Name:    "test-channel-disabled-" + uuid.New().String()[:8],
			Type:    ChannelTypeSlack,
			Config:  []byte(`{}`),
			Enabled: false,
		}
		err = repo.CreateChannel(ctx, disabledChannel)
		require.NoError(t, err)
		defer repo.DeleteChannel(ctx, disabledChannel.ID)

		channels, err := repo.ListEnabledChannels(ctx)
		require.NoError(t, err)

		foundEnabled := false
		foundDisabled := false
		for _, ch := range channels {
			if ch.ID == enabledChannel.ID {
				foundEnabled = true
			}
			if ch.ID == disabledChannel.ID {
				foundDisabled = true
			}
		}
		assert.True(t, foundEnabled, "enabled channel should be in list")
		assert.False(t, foundDisabled, "disabled channel should not be in list")
	})

	t.Run("CreateRule", func(t *testing.T) {
		channel := &NotificationChannel{
			Name:    "test-rule-channel-" + uuid.New().String()[:8],
			Type:    ChannelTypeSlack,
			Config:  []byte(`{}`),
			Enabled: true,
		}
		err := repo.CreateChannel(ctx, channel)
		require.NoError(t, err)
		defer repo.DeleteChannel(ctx, channel.ID)

		rule := &NotificationRule{
			ChannelID: channel.ID,
			TriggerOn: []TriggerEvent{TriggerEventFailure, TriggerEventRecovery},
			Enabled:   true,
		}

		err = repo.CreateRule(ctx, rule)
		require.NoError(t, err)
		assert.NotEqual(t, uuid.Nil, rule.ID)

		t.Cleanup(func() {
			repo.DeleteRule(ctx, rule.ID)
		})
	})

	t.Run("GetRule", func(t *testing.T) {
		channel := &NotificationChannel{
			Name:    "test-getrule-channel-" + uuid.New().String()[:8],
			Type:    ChannelTypeSlack,
			Config:  []byte(`{}`),
			Enabled: true,
		}
		err := repo.CreateChannel(ctx, channel)
		require.NoError(t, err)
		defer repo.DeleteChannel(ctx, channel.ID)

		rule := &NotificationRule{
			ChannelID: channel.ID,
			TriggerOn: []TriggerEvent{TriggerEventAlways},
			Enabled:   true,
		}
		err = repo.CreateRule(ctx, rule)
		require.NoError(t, err)
		defer repo.DeleteRule(ctx, rule.ID)

		fetched, err := repo.GetRule(ctx, rule.ID)
		require.NoError(t, err)
		assert.Equal(t, rule.ChannelID, fetched.ChannelID)
		assert.Equal(t, rule.TriggerOn, fetched.TriggerOn)
	})

	t.Run("ListRulesByService", func(t *testing.T) {
		// Create service
		svc := &Service{
			Name:          "test-rule-service-" + uuid.New().String()[:8],
			GitURL:        "https://github.com/example/repo.git",
			DefaultBranch: "main",
		}
		err := svcRepo.Create(ctx, svc)
		require.NoError(t, err)
		defer svcRepo.Delete(ctx, svc.ID)

		// Create channel
		channel := &NotificationChannel{
			Name:    "test-listsvc-channel-" + uuid.New().String()[:8],
			Type:    ChannelTypeSlack,
			Config:  []byte(`{}`),
			Enabled: true,
		}
		err = repo.CreateChannel(ctx, channel)
		require.NoError(t, err)
		defer repo.DeleteChannel(ctx, channel.ID)

		// Create rule for specific service
		rule := &NotificationRule{
			ChannelID: channel.ID,
			ServiceID: &svc.ID,
			TriggerOn: []TriggerEvent{TriggerEventFailure},
			Enabled:   true,
		}
		err = repo.CreateRule(ctx, rule)
		require.NoError(t, err)
		defer repo.DeleteRule(ctx, rule.ID)

		// Create global rule (nil ServiceID)
		globalRule := &NotificationRule{
			ChannelID: channel.ID,
			ServiceID: nil,
			TriggerOn: []TriggerEvent{TriggerEventAlways},
			Enabled:   true,
		}
		err = repo.CreateRule(ctx, globalRule)
		require.NoError(t, err)
		defer repo.DeleteRule(ctx, globalRule.ID)

		rules, err := repo.ListRulesByService(ctx, svc.ID)
		require.NoError(t, err)

		// Should include both service-specific and global rules
		foundSpecific := false
		foundGlobal := false
		for _, r := range rules {
			if r.ID == rule.ID {
				foundSpecific = true
			}
			if r.ID == globalRule.ID {
				foundGlobal = true
			}
		}
		assert.True(t, foundSpecific, "service-specific rule should be in list")
		assert.True(t, foundGlobal, "global rule should be in list")
	})
}

// ============================================================================
// TRANSACTION TESTS
// ============================================================================

func TestTransactions(t *testing.T) {
	ctx := context.Background()

	t.Run("Commit", func(t *testing.T) {
		var serviceID uuid.UUID

		err := testDB.db.WithTx(ctx, func(tx pgx.Tx) error {
			name := "tx-test-commit-" + uuid.New().String()[:8]
			err := tx.QueryRow(ctx, `
				INSERT INTO services (name, git_url, default_branch)
				VALUES ($1, $2, $3)
				RETURNING id
			`, name, "https://github.com/example/repo.git", "main").Scan(&serviceID)
			return err
		})
		require.NoError(t, err)
		assert.NotEqual(t, uuid.Nil, serviceID)

		// Verify committed
		repo := NewServiceRepo(testDB.db)
		svc, err := repo.Get(ctx, serviceID)
		require.NoError(t, err)
		assert.NotNil(t, svc)

		// Cleanup
		repo.Delete(ctx, serviceID)
	})

	t.Run("Rollback", func(t *testing.T) {
		name := "tx-test-rollback-" + uuid.New().String()[:8]
		var serviceID uuid.UUID

		err := testDB.db.WithTx(ctx, func(tx pgx.Tx) error {
			err := tx.QueryRow(ctx, `
				INSERT INTO services (name, git_url, default_branch)
				VALUES ($1, $2, $3)
				RETURNING id
			`, name, "https://github.com/example/repo.git", "main").Scan(&serviceID)
			if err != nil {
				return err
			}

			// Return error to trigger rollback
			return assert.AnError
		})
		require.Error(t, err)

		// Verify rolled back
		repo := NewServiceRepo(testDB.db)
		_, err = repo.GetByName(ctx, name)
		assert.True(t, IsNotFound(err), "service should not exist after rollback")
	})
}

// ============================================================================
// DATABASE CONNECTION TESTS
// ============================================================================

func TestDatabaseConnection(t *testing.T) {
	ctx := context.Background()

	t.Run("Health", func(t *testing.T) {
		err := testDB.db.Health(ctx)
		require.NoError(t, err)
	})

	t.Run("Stats", func(t *testing.T) {
		stats := testDB.db.Stats()
		assert.GreaterOrEqual(t, stats.MaxConns, int32(1))
		assert.GreaterOrEqual(t, stats.TotalConns, int32(0))
	})
}

// ============================================================================
// HELPER TEST FOR MIGRATIONS FS
// ============================================================================

// Verify migrations directory exists and has expected files
func TestMigrationsFilesystem(t *testing.T) {
	migrationsFS := os.DirFS("../../migrations")

	var foundFiles []string
	err := fs.WalkDir(migrationsFS, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() {
			foundFiles = append(foundFiles, path)
		}
		return nil
	})
	require.NoError(t, err)

	// Should have at least 8 files (4 migrations x 2 up/down)
	assert.GreaterOrEqual(t, len(foundFiles), 8, "should have at least 8 migration files")

	// Check for expected migration patterns
	expectedPatterns := []string{
		"initial_schema.up.sql",
		"initial_schema.down.sql",
		"notifications.up.sql",
		"notifications.down.sql",
	}
	for _, pattern := range expectedPatterns {
		found := false
		for _, file := range foundFiles {
			if contains(file, pattern) {
				found = true
				break
			}
		}
		assert.True(t, found, "should find migration file matching %s", pattern)
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsHelper(s, substr))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
