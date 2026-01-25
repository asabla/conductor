package database

import (
	"context"
	"embed"
	"fmt"
	"io/fs"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

// MigrationDirection indicates the direction of a migration.
type MigrationDirection string

const (
	// MigrationUp applies migrations forward.
	MigrationUp MigrationDirection = "up"
	// MigrationDown rolls back migrations.
	MigrationDown MigrationDirection = "down"
)

// Migration represents a single database migration.
type Migration struct {
	Version   string
	Name      string
	UpSQL     string
	DownSQL   string
	AppliedAt *time.Time
}

// MigrationStatus represents the status of a migration.
type MigrationStatus struct {
	Version   string
	Name      string
	Applied   bool
	AppliedAt *time.Time
}

// Migrator handles database migrations.
type Migrator struct {
	db         *DB
	migrations []Migration
	tableName  string
}

// MigratorOption configures a Migrator.
type MigratorOption func(*Migrator)

// WithMigrationTable sets the name of the migrations tracking table.
func WithMigrationTable(name string) MigratorOption {
	return func(m *Migrator) {
		m.tableName = name
	}
}

// NewMigrator creates a new Migrator with migrations from an embedded filesystem.
func NewMigrator(db *DB, migrationsFS embed.FS, dir string, opts ...MigratorOption) (*Migrator, error) {
	m := &Migrator{
		db:        db,
		tableName: "schema_migrations",
	}

	for _, opt := range opts {
		opt(m)
	}

	migrations, err := loadMigrations(migrationsFS, dir)
	if err != nil {
		return nil, fmt.Errorf("failed to load migrations: %w", err)
	}
	m.migrations = migrations

	return m, nil
}

// NewMigratorFromFS creates a new Migrator with migrations from a standard filesystem.
func NewMigratorFromFS(db *DB, migrationsFS fs.FS, opts ...MigratorOption) (*Migrator, error) {
	m := &Migrator{
		db:        db,
		tableName: "schema_migrations",
	}

	for _, opt := range opts {
		opt(m)
	}

	migrations, err := loadMigrationsFromFS(migrationsFS)
	if err != nil {
		return nil, fmt.Errorf("failed to load migrations: %w", err)
	}
	m.migrations = migrations

	return m, nil
}

// migrationFileRegex matches migration files like "20260125000001_initial_schema.up.sql"
var migrationFileRegex = regexp.MustCompile(`^(\d+)_(.+)\.(up|down)\.sql$`)

func loadMigrations(embedFS embed.FS, dir string) ([]Migration, error) {
	subFS, err := fs.Sub(embedFS, dir)
	if err != nil {
		return nil, fmt.Errorf("failed to access migrations directory %q: %w", dir, err)
	}
	return loadMigrationsFromFS(subFS)
}

func loadMigrationsFromFS(migrationsFS fs.FS) ([]Migration, error) {
	// Map to collect up/down SQL by version
	migrationMap := make(map[string]*Migration)

	err := fs.WalkDir(migrationsFS, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}

		filename := filepath.Base(path)
		matches := migrationFileRegex.FindStringSubmatch(filename)
		if matches == nil {
			return nil // Skip non-migration files
		}

		version := matches[1]
		name := matches[2]
		direction := matches[3]

		content, err := fs.ReadFile(migrationsFS, path)
		if err != nil {
			return fmt.Errorf("failed to read migration file %q: %w", path, err)
		}

		mig, ok := migrationMap[version]
		if !ok {
			mig = &Migration{
				Version: version,
				Name:    name,
			}
			migrationMap[version] = mig
		}

		switch direction {
		case "up":
			mig.UpSQL = string(content)
		case "down":
			mig.DownSQL = string(content)
		}

		return nil
	})
	if err != nil {
		return nil, err
	}

	// Convert map to sorted slice
	migrations := make([]Migration, 0, len(migrationMap))
	for _, m := range migrationMap {
		migrations = append(migrations, *m)
	}
	sort.Slice(migrations, func(i, j int) bool {
		return migrations[i].Version < migrations[j].Version
	})

	return migrations, nil
}

// ensureMigrationsTable creates the migrations tracking table if it doesn't exist.
func (m *Migrator) ensureMigrationsTable(ctx context.Context) error {
	query := fmt.Sprintf(`
		CREATE TABLE IF NOT EXISTS %s (
			version VARCHAR(14) PRIMARY KEY,
			name VARCHAR(255) NOT NULL,
			applied_at TIMESTAMP WITH TIME ZONE DEFAULT NOW() NOT NULL
		)
	`, m.tableName)

	_, err := m.db.pool.Exec(ctx, query)
	if err != nil {
		return fmt.Errorf("failed to create migrations table: %w", err)
	}
	return nil
}

// getAppliedMigrations returns a map of applied migration versions.
func (m *Migrator) getAppliedMigrations(ctx context.Context) (map[string]time.Time, error) {
	query := fmt.Sprintf(`SELECT version, applied_at FROM %s`, m.tableName)

	rows, err := m.db.pool.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to query applied migrations: %w", err)
	}
	defer rows.Close()

	applied := make(map[string]time.Time)
	for rows.Next() {
		var version string
		var appliedAt time.Time
		if err := rows.Scan(&version, &appliedAt); err != nil {
			return nil, fmt.Errorf("failed to scan migration row: %w", err)
		}
		applied[version] = appliedAt
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate migration rows: %w", err)
	}

	return applied, nil
}

// Up applies all pending migrations.
func (m *Migrator) Up(ctx context.Context) (int, error) {
	return m.UpTo(ctx, "")
}

// UpTo applies migrations up to and including the specified version.
// If version is empty, all pending migrations are applied.
func (m *Migrator) UpTo(ctx context.Context, targetVersion string) (int, error) {
	if err := m.ensureMigrationsTable(ctx); err != nil {
		return 0, err
	}

	applied, err := m.getAppliedMigrations(ctx)
	if err != nil {
		return 0, err
	}

	count := 0
	for _, mig := range m.migrations {
		if _, ok := applied[mig.Version]; ok {
			continue // Already applied
		}

		if mig.UpSQL == "" {
			return count, fmt.Errorf("migration %s has no up SQL", mig.Version)
		}

		if err := m.applyMigration(ctx, mig, MigrationUp); err != nil {
			return count, fmt.Errorf("failed to apply migration %s: %w", mig.Version, err)
		}
		count++

		if targetVersion != "" && mig.Version == targetVersion {
			break
		}
	}

	return count, nil
}

// Down rolls back the last applied migration.
func (m *Migrator) Down(ctx context.Context) error {
	return m.DownN(ctx, 1)
}

// DownN rolls back the last n applied migrations.
func (m *Migrator) DownN(ctx context.Context, n int) error {
	if err := m.ensureMigrationsTable(ctx); err != nil {
		return err
	}

	applied, err := m.getAppliedMigrations(ctx)
	if err != nil {
		return err
	}

	// Get applied migrations in reverse order
	var appliedMigrations []Migration
	for _, mig := range m.migrations {
		if _, ok := applied[mig.Version]; ok {
			appliedMigrations = append(appliedMigrations, mig)
		}
	}

	// Reverse to rollback most recent first
	for i, j := 0, len(appliedMigrations)-1; i < j; i, j = i+1, j-1 {
		appliedMigrations[i], appliedMigrations[j] = appliedMigrations[j], appliedMigrations[i]
	}

	count := 0
	for _, mig := range appliedMigrations {
		if count >= n {
			break
		}

		if mig.DownSQL == "" {
			return fmt.Errorf("migration %s has no down SQL", mig.Version)
		}

		if err := m.applyMigration(ctx, mig, MigrationDown); err != nil {
			return fmt.Errorf("failed to rollback migration %s: %w", mig.Version, err)
		}
		count++
	}

	return nil
}

// DownTo rolls back migrations down to (but not including) the specified version.
func (m *Migrator) DownTo(ctx context.Context, targetVersion string) error {
	if err := m.ensureMigrationsTable(ctx); err != nil {
		return err
	}

	applied, err := m.getAppliedMigrations(ctx)
	if err != nil {
		return err
	}

	// Get applied migrations in reverse order that are after targetVersion
	var toRollback []Migration
	for _, mig := range m.migrations {
		if _, ok := applied[mig.Version]; ok && mig.Version > targetVersion {
			toRollback = append(toRollback, mig)
		}
	}

	// Reverse to rollback most recent first
	for i, j := 0, len(toRollback)-1; i < j; i, j = i+1, j-1 {
		toRollback[i], toRollback[j] = toRollback[j], toRollback[i]
	}

	for _, mig := range toRollback {
		if mig.DownSQL == "" {
			return fmt.Errorf("migration %s has no down SQL", mig.Version)
		}

		if err := m.applyMigration(ctx, mig, MigrationDown); err != nil {
			return fmt.Errorf("failed to rollback migration %s: %w", mig.Version, err)
		}
	}

	return nil
}

// Reset rolls back all migrations and then re-applies them.
func (m *Migrator) Reset(ctx context.Context) error {
	if err := m.ensureMigrationsTable(ctx); err != nil {
		return err
	}

	// Roll back all
	applied, err := m.getAppliedMigrations(ctx)
	if err != nil {
		return err
	}

	if err := m.DownN(ctx, len(applied)); err != nil {
		return fmt.Errorf("failed to rollback all migrations: %w", err)
	}

	// Re-apply all
	if _, err := m.Up(ctx); err != nil {
		return fmt.Errorf("failed to re-apply migrations: %w", err)
	}

	return nil
}

// applyMigration applies or rolls back a single migration within a transaction.
func (m *Migrator) applyMigration(ctx context.Context, mig Migration, direction MigrationDirection) error {
	return m.db.WithTx(ctx, func(tx pgx.Tx) error {
		var sql string
		switch direction {
		case MigrationUp:
			sql = mig.UpSQL
		case MigrationDown:
			sql = mig.DownSQL
		}

		// Execute migration SQL
		if _, err := tx.Exec(ctx, sql); err != nil {
			return fmt.Errorf("failed to execute migration SQL: %w", err)
		}

		// Update migrations table
		switch direction {
		case MigrationUp:
			insertQuery := fmt.Sprintf(
				`INSERT INTO %s (version, name) VALUES ($1, $2)`,
				m.tableName,
			)
			if _, err := tx.Exec(ctx, insertQuery, mig.Version, mig.Name); err != nil {
				return fmt.Errorf("failed to record migration: %w", err)
			}
		case MigrationDown:
			deleteQuery := fmt.Sprintf(
				`DELETE FROM %s WHERE version = $1`,
				m.tableName,
			)
			if _, err := tx.Exec(ctx, deleteQuery, mig.Version); err != nil {
				return fmt.Errorf("failed to remove migration record: %w", err)
			}
		}

		return nil
	})
}

// Status returns the status of all migrations.
func (m *Migrator) Status(ctx context.Context) ([]MigrationStatus, error) {
	if err := m.ensureMigrationsTable(ctx); err != nil {
		return nil, err
	}

	applied, err := m.getAppliedMigrations(ctx)
	if err != nil {
		return nil, err
	}

	statuses := make([]MigrationStatus, len(m.migrations))
	for i, mig := range m.migrations {
		status := MigrationStatus{
			Version: mig.Version,
			Name:    mig.Name,
		}
		if appliedAt, ok := applied[mig.Version]; ok {
			status.Applied = true
			status.AppliedAt = &appliedAt
		}
		statuses[i] = status
	}

	return statuses, nil
}

// Version returns the current migration version.
// Returns empty string if no migrations have been applied.
func (m *Migrator) Version(ctx context.Context) (string, error) {
	if err := m.ensureMigrationsTable(ctx); err != nil {
		return "", err
	}

	query := fmt.Sprintf(`SELECT version FROM %s ORDER BY version DESC LIMIT 1`, m.tableName)

	var version string
	err := m.db.pool.QueryRow(ctx, query).Scan(&version)
	if err != nil {
		if err == pgx.ErrNoRows {
			return "", nil
		}
		return "", fmt.Errorf("failed to get current version: %w", err)
	}

	return version, nil
}

// Pending returns the list of pending migrations.
func (m *Migrator) Pending(ctx context.Context) ([]Migration, error) {
	if err := m.ensureMigrationsTable(ctx); err != nil {
		return nil, err
	}

	applied, err := m.getAppliedMigrations(ctx)
	if err != nil {
		return nil, err
	}

	var pending []Migration
	for _, mig := range m.migrations {
		if _, ok := applied[mig.Version]; !ok {
			pending = append(pending, mig)
		}
	}

	return pending, nil
}

// FormatStatus formats the migration status for display.
func FormatStatus(statuses []MigrationStatus) string {
	if len(statuses) == 0 {
		return "No migrations found"
	}

	var b strings.Builder
	b.WriteString("Migration Status:\n")
	b.WriteString(strings.Repeat("-", 80) + "\n")
	b.WriteString(fmt.Sprintf("%-14s %-40s %-10s %s\n", "Version", "Name", "Status", "Applied At"))
	b.WriteString(strings.Repeat("-", 80) + "\n")

	for _, s := range statuses {
		status := "pending"
		appliedAt := ""
		if s.Applied {
			status = "applied"
			if s.AppliedAt != nil {
				appliedAt = s.AppliedAt.Format(time.RFC3339)
			}
		}
		b.WriteString(fmt.Sprintf("%-14s %-40s %-10s %s\n", s.Version, s.Name, status, appliedAt))
	}

	return b.String()
}
