// Package database provides PostgreSQL database connectivity and repositories
// for the Conductor control plane.
package database

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Config holds database connection configuration.
type Config struct {
	// URL is the PostgreSQL connection string.
	// Format: postgres://user:password@host:port/database?sslmode=disable
	URL string

	// MaxConns is the maximum number of connections in the pool.
	// Default: 25
	MaxConns int32

	// MinConns is the minimum number of connections to keep open.
	// Default: 5
	MinConns int32

	// MaxConnLifetime is the maximum lifetime of a connection.
	// Default: 1 hour
	MaxConnLifetime time.Duration

	// MaxConnIdleTime is the maximum time a connection can be idle.
	// Default: 30 minutes
	MaxConnIdleTime time.Duration

	// HealthCheckPeriod is the interval between health checks.
	// Default: 1 minute
	HealthCheckPeriod time.Duration
}

// DefaultConfig returns a Config with sensible defaults.
func DefaultConfig(url string) Config {
	return Config{
		URL:               url,
		MaxConns:          25,
		MinConns:          5,
		MaxConnLifetime:   time.Hour,
		MaxConnIdleTime:   30 * time.Minute,
		HealthCheckPeriod: time.Minute,
	}
}

// DB wraps a pgxpool.Pool and provides database operations.
type DB struct {
	pool *pgxpool.Pool
}

// New creates a new database connection pool.
func New(ctx context.Context, cfg Config) (*DB, error) {
	poolConfig, err := pgxpool.ParseConfig(cfg.URL)
	if err != nil {
		return nil, fmt.Errorf("failed to parse database URL: %w", err)
	}

	// Apply configuration
	if cfg.MaxConns > 0 {
		poolConfig.MaxConns = cfg.MaxConns
	}
	if cfg.MinConns > 0 {
		poolConfig.MinConns = cfg.MinConns
	}
	if cfg.MaxConnLifetime > 0 {
		poolConfig.MaxConnLifetime = cfg.MaxConnLifetime
	}
	if cfg.MaxConnIdleTime > 0 {
		poolConfig.MaxConnIdleTime = cfg.MaxConnIdleTime
	}
	if cfg.HealthCheckPeriod > 0 {
		poolConfig.HealthCheckPeriod = cfg.HealthCheckPeriod
	}

	pool, err := pgxpool.NewWithConfig(ctx, poolConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create connection pool: %w", err)
	}

	// Verify connectivity
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	return &DB{pool: pool}, nil
}

// Close closes the database connection pool.
func (db *DB) Close() {
	if db.pool != nil {
		db.pool.Close()
	}
}

// Pool returns the underlying connection pool.
// Use this for advanced operations that require direct pool access.
func (db *DB) Pool() *pgxpool.Pool {
	return db.pool
}

// Health performs a health check on the database connection.
func (db *DB) Health(ctx context.Context) error {
	if err := db.pool.Ping(ctx); err != nil {
		return fmt.Errorf("database health check failed: %w", err)
	}
	return nil
}

// HealthStats returns detailed health statistics about the connection pool.
type HealthStats struct {
	TotalConns        int32
	AcquiredConns     int32
	IdleConns         int32
	MaxConns          int32
	AcquireCount      int64
	AcquireDuration   time.Duration
	EmptyAcquireCount int64
}

// Stats returns connection pool statistics.
func (db *DB) Stats() HealthStats {
	stat := db.pool.Stat()
	return HealthStats{
		TotalConns:        stat.TotalConns(),
		AcquiredConns:     stat.AcquiredConns(),
		IdleConns:         stat.IdleConns(),
		MaxConns:          stat.MaxConns(),
		AcquireCount:      stat.AcquireCount(),
		AcquireDuration:   stat.AcquireDuration(),
		EmptyAcquireCount: stat.EmptyAcquireCount(),
	}
}

// TxFunc is a function that executes within a transaction.
type TxFunc func(tx pgx.Tx) error

// WithTx executes a function within a database transaction.
// If the function returns an error, the transaction is rolled back.
// If the function succeeds, the transaction is committed.
func (db *DB) WithTx(ctx context.Context, fn TxFunc) error {
	return db.WithTxOptions(ctx, pgx.TxOptions{}, fn)
}

// WithTxOptions executes a function within a database transaction with custom options.
func (db *DB) WithTxOptions(ctx context.Context, opts pgx.TxOptions, fn TxFunc) error {
	tx, err := db.pool.BeginTx(ctx, opts)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}

	defer func() {
		// Rollback is a no-op if the transaction was already committed
		_ = tx.Rollback(ctx)
	}()

	if err := fn(tx); err != nil {
		return err
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}

// Querier is an interface for database query operations.
// Both *pgxpool.Pool and pgx.Tx implement this interface.
type Querier interface {
	Exec(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// Common database errors
var (
	// ErrNotFound is returned when a requested record is not found.
	ErrNotFound = errors.New("record not found")

	// ErrDuplicate is returned when a unique constraint is violated.
	ErrDuplicate = errors.New("duplicate record")

	// ErrForeignKey is returned when a foreign key constraint is violated.
	ErrForeignKey = errors.New("foreign key violation")
)

// IsNotFound returns true if the error is ErrNotFound or pgx.ErrNoRows.
func IsNotFound(err error) bool {
	return errors.Is(err, ErrNotFound) || errors.Is(err, pgx.ErrNoRows)
}

// IsDuplicate returns true if the error is a unique constraint violation.
func IsDuplicate(err error) bool {
	if errors.Is(err, ErrDuplicate) {
		return true
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		// 23505 is the PostgreSQL error code for unique_violation
		return pgErr.Code == "23505"
	}
	return false
}

// IsForeignKeyViolation returns true if the error is a foreign key constraint violation.
func IsForeignKeyViolation(err error) bool {
	if errors.Is(err, ErrForeignKey) {
		return true
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		// 23503 is the PostgreSQL error code for foreign_key_violation
		return pgErr.Code == "23503"
	}
	return false
}

// WrapDBError wraps database errors with more meaningful error types.
func WrapDBError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch pgErr.Code {
		case "23505":
			return fmt.Errorf("%w: %s", ErrDuplicate, pgErr.Detail)
		case "23503":
			return fmt.Errorf("%w: %s", ErrForeignKey, pgErr.Detail)
		}
	}
	return err
}
