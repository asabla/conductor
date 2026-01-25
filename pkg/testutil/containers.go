// Package testutil provides test utilities and helpers for integration tests.
package testutil

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/minio"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/modules/redis"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/conductor/conductor/internal/config"
)

// PostgresContainer wraps a testcontainers postgres instance.
type PostgresContainer struct {
	Container *postgres.PostgresContainer
	ConnStr   string
	Host      string
	Port      string
	Database  string
	Username  string
	Password  string
}

// PostgresContainerConfig holds configuration for creating a postgres container.
type PostgresContainerConfig struct {
	Database string
	Username string
	Password string
	ImageTag string
}

// DefaultPostgresConfig returns a default postgres container configuration.
func DefaultPostgresConfig() PostgresContainerConfig {
	return PostgresContainerConfig{
		Database: "conductor_test",
		Username: "conductor",
		Password: "conductor_test_pass",
		ImageTag: "16-alpine",
	}
}

// NewPostgresContainer creates a new postgres testcontainer.
func NewPostgresContainer(ctx context.Context, cfg PostgresContainerConfig) (*PostgresContainer, error) {
	if cfg.Database == "" {
		cfg = DefaultPostgresConfig()
	}

	container, err := postgres.Run(ctx,
		fmt.Sprintf("postgres:%s", cfg.ImageTag),
		postgres.WithDatabase(cfg.Database),
		postgres.WithUsername(cfg.Username),
		postgres.WithPassword(cfg.Password),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).
				WithStartupTimeout(60*time.Second),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to start postgres container: %w", err)
	}

	connStr, err := container.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		container.Terminate(ctx)
		return nil, fmt.Errorf("failed to get connection string: %w", err)
	}

	host, err := container.Host(ctx)
	if err != nil {
		container.Terminate(ctx)
		return nil, fmt.Errorf("failed to get host: %w", err)
	}

	mappedPort, err := container.MappedPort(ctx, "5432")
	if err != nil {
		container.Terminate(ctx)
		return nil, fmt.Errorf("failed to get mapped port: %w", err)
	}

	return &PostgresContainer{
		Container: container,
		ConnStr:   connStr,
		Host:      host,
		Port:      mappedPort.Port(),
		Database:  cfg.Database,
		Username:  cfg.Username,
		Password:  cfg.Password,
	}, nil
}

// Terminate stops and removes the container.
func (c *PostgresContainer) Terminate(ctx context.Context) error {
	if c.Container != nil {
		return c.Container.Terminate(ctx)
	}
	return nil
}

// MinioContainer wraps a testcontainers minio instance.
type MinioContainer struct {
	Container       *minio.MinioContainer
	Endpoint        string
	AccessKeyID     string
	SecretAccessKey string
}

// MinioContainerConfig holds configuration for creating a minio container.
type MinioContainerConfig struct {
	Username string
	Password string
	ImageTag string
}

// DefaultMinioConfig returns a default minio container configuration.
func DefaultMinioConfig() MinioContainerConfig {
	return MinioContainerConfig{
		Username: "minioadmin",
		Password: "minioadmin",
		ImageTag: "latest",
	}
}

// NewMinioContainer creates a new minio testcontainer.
func NewMinioContainer(ctx context.Context, cfg MinioContainerConfig) (*MinioContainer, error) {
	if cfg.Username == "" {
		cfg = DefaultMinioConfig()
	}

	container, err := minio.Run(ctx,
		fmt.Sprintf("minio/minio:%s", cfg.ImageTag),
		minio.WithUsername(cfg.Username),
		minio.WithPassword(cfg.Password),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to start minio container: %w", err)
	}

	endpoint, err := container.ConnectionString(ctx)
	if err != nil {
		container.Terminate(ctx)
		return nil, fmt.Errorf("failed to get minio endpoint: %w", err)
	}

	return &MinioContainer{
		Container:       container,
		Endpoint:        endpoint,
		AccessKeyID:     cfg.Username,
		SecretAccessKey: cfg.Password,
	}, nil
}

// Terminate stops and removes the container.
func (c *MinioContainer) Terminate(ctx context.Context) error {
	if c.Container != nil {
		return c.Container.Terminate(ctx)
	}
	return nil
}

// RedisContainer wraps a testcontainers redis instance.
type RedisContainer struct {
	Container *redis.RedisContainer
	URL       string
	Host      string
	Port      string
}

// RedisContainerConfig holds configuration for creating a redis container.
type RedisContainerConfig struct {
	ImageTag string
}

// DefaultRedisConfig returns a default redis container configuration.
func DefaultRedisConfig() RedisContainerConfig {
	return RedisContainerConfig{
		ImageTag: "7-alpine",
	}
}

// NewRedisContainer creates a new redis testcontainer.
func NewRedisContainer(ctx context.Context, cfg RedisContainerConfig) (*RedisContainer, error) {
	if cfg.ImageTag == "" {
		cfg = DefaultRedisConfig()
	}

	container, err := redis.Run(ctx,
		fmt.Sprintf("redis:%s", cfg.ImageTag),
		testcontainers.WithWaitStrategy(
			wait.ForLog("Ready to accept connections").
				WithStartupTimeout(30*time.Second),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to start redis container: %w", err)
	}

	connStr, err := container.ConnectionString(ctx)
	if err != nil {
		container.Terminate(ctx)
		return nil, fmt.Errorf("failed to get redis connection string: %w", err)
	}

	host, err := container.Host(ctx)
	if err != nil {
		container.Terminate(ctx)
		return nil, fmt.Errorf("failed to get host: %w", err)
	}

	mappedPort, err := container.MappedPort(ctx, "6379")
	if err != nil {
		container.Terminate(ctx)
		return nil, fmt.Errorf("failed to get mapped port: %w", err)
	}

	return &RedisContainer{
		Container: container,
		URL:       connStr,
		Host:      host,
		Port:      mappedPort.Port(),
	}, nil
}

// Terminate stops and removes the container.
func (c *RedisContainer) Terminate(ctx context.Context) error {
	if c.Container != nil {
		return c.Container.Terminate(ctx)
	}
	return nil
}

// MailhogContainer wraps a testcontainers mailhog instance.
type MailhogContainer struct {
	Container testcontainers.Container
	SMTPHost  string
	SMTPPort  string
	HTTPHost  string
	HTTPPort  string
	APIURL    string
}

// NewMailhogContainer creates a new mailhog testcontainer.
func NewMailhogContainer(ctx context.Context) (*MailhogContainer, error) {
	req := testcontainers.ContainerRequest{
		Image:        "mailhog/mailhog:latest",
		ExposedPorts: []string{"1025/tcp", "8025/tcp"},
		WaitingFor:   wait.ForHTTP("/api/v2/messages").WithPort("8025").WithStartupTimeout(30 * time.Second),
	}

	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to start mailhog container: %w", err)
	}

	host, err := container.Host(ctx)
	if err != nil {
		container.Terminate(ctx)
		return nil, fmt.Errorf("failed to get host: %w", err)
	}

	smtpPort, err := container.MappedPort(ctx, "1025")
	if err != nil {
		container.Terminate(ctx)
		return nil, fmt.Errorf("failed to get SMTP port: %w", err)
	}

	httpPort, err := container.MappedPort(ctx, "8025")
	if err != nil {
		container.Terminate(ctx)
		return nil, fmt.Errorf("failed to get HTTP port: %w", err)
	}

	return &MailhogContainer{
		Container: container,
		SMTPHost:  host,
		SMTPPort:  smtpPort.Port(),
		HTTPHost:  host,
		HTTPPort:  httpPort.Port(),
		APIURL:    fmt.Sprintf("http://%s:%s/api/v2", host, httpPort.Port()),
	}, nil
}

// Terminate stops and removes the container.
func (c *MailhogContainer) Terminate(ctx context.Context) error {
	if c.Container != nil {
		return c.Container.Terminate(ctx)
	}
	return nil
}

// MailhogMessage represents a message from Mailhog API.
type MailhogMessage struct {
	ID      string `json:"ID"`
	Content struct {
		Headers struct {
			Subject []string `json:"Subject"`
			To      []string `json:"To"`
			From    []string `json:"From"`
		} `json:"Headers"`
		Body string `json:"Body"`
	} `json:"Content"`
}

// MailhogResponse represents the response from Mailhog API.
type MailhogResponse struct {
	Total int              `json:"total"`
	Items []MailhogMessage `json:"items"`
}

// GetMessages retrieves messages from the Mailhog API.
func (c *MailhogContainer) GetMessages(ctx context.Context) ([]MailhogMessage, error) {
	client := &http.Client{Timeout: 10 * time.Second}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.APIURL+"/messages", nil)
	if err != nil {
		return nil, err
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("mailhog returned status %d", resp.StatusCode)
	}

	var result MailhogResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	return result.Items, nil
}

// TestConfig creates a test configuration using the provided containers.
type TestConfig struct {
	Postgres *PostgresContainer
	Minio    *MinioContainer
	Redis    *RedisContainer
	Mailhog  *MailhogContainer
}

// NewTestConfig creates a TestConfig with all containers started.
func NewTestConfig(ctx context.Context) (*TestConfig, error) {
	cfg := &TestConfig{}

	// Start postgres
	pg, err := NewPostgresContainer(ctx, DefaultPostgresConfig())
	if err != nil {
		return nil, fmt.Errorf("failed to create postgres container: %w", err)
	}
	cfg.Postgres = pg

	// Start minio
	mc, err := NewMinioContainer(ctx, DefaultMinioConfig())
	if err != nil {
		cfg.Terminate(ctx)
		return nil, fmt.Errorf("failed to create minio container: %w", err)
	}
	cfg.Minio = mc

	return cfg, nil
}

// NewTestConfigWithRedis creates a TestConfig with postgres, minio, and redis.
func NewTestConfigWithRedis(ctx context.Context) (*TestConfig, error) {
	cfg, err := NewTestConfig(ctx)
	if err != nil {
		return nil, err
	}

	// Start redis
	rc, err := NewRedisContainer(ctx, DefaultRedisConfig())
	if err != nil {
		cfg.Terminate(ctx)
		return nil, fmt.Errorf("failed to create redis container: %w", err)
	}
	cfg.Redis = rc

	return cfg, nil
}

// NewTestConfigWithMailhog creates a TestConfig with postgres, minio, and mailhog.
func NewTestConfigWithMailhog(ctx context.Context) (*TestConfig, error) {
	cfg, err := NewTestConfig(ctx)
	if err != nil {
		return nil, err
	}

	// Start mailhog
	mh, err := NewMailhogContainer(ctx)
	if err != nil {
		cfg.Terminate(ctx)
		return nil, fmt.Errorf("failed to create mailhog container: %w", err)
	}
	cfg.Mailhog = mh

	return cfg, nil
}

// Terminate stops all containers.
func (tc *TestConfig) Terminate(ctx context.Context) {
	if tc.Postgres != nil {
		tc.Postgres.Terminate(ctx)
	}
	if tc.Minio != nil {
		tc.Minio.Terminate(ctx)
	}
	if tc.Redis != nil {
		tc.Redis.Terminate(ctx)
	}
	if tc.Mailhog != nil {
		tc.Mailhog.Terminate(ctx)
	}
}

// ToControlPlaneConfig converts TestConfig to a control plane configuration.
func (tc *TestConfig) ToControlPlaneConfig() *config.Config {
	cfg := &config.Config{
		Server: config.ServerConfig{
			HTTPPort:        0, // Random port
			GRPCPort:        0, // Random port
			MetricsPort:     0, // Random port
			ShutdownTimeout: 10 * time.Second,
		},
		Database: config.DatabaseConfig{
			URL:             tc.Postgres.ConnStr,
			MaxOpenConns:    5,
			MaxIdleConns:    2,
			ConnMaxLifetime: 5 * time.Minute,
			ConnMaxIdleTime: 1 * time.Minute,
			QueryTimeout:    30 * time.Second,
		},
		Storage: config.StorageConfig{
			Endpoint:        tc.Minio.Endpoint,
			Bucket:          "conductor-test",
			Region:          "us-east-1",
			AccessKeyID:     tc.Minio.AccessKeyID,
			SecretAccessKey: tc.Minio.SecretAccessKey,
			UseSSL:          false,
			PathStyle:       true,
		},
		Auth: config.AuthConfig{
			JWTSecret:     "test-secret-key-that-is-at-least-32-characters-long",
			JWTExpiration: 24 * time.Hour,
		},
		Agent: config.AgentConfig{
			HeartbeatTimeout:       90 * time.Second,
			DefaultTestTimeout:     30 * time.Minute,
			MaxTestTimeout:         4 * time.Hour,
			ResultStreamBufferSize: 100,
		},
		Git: config.GitConfig{
			Provider: "github",
		},
		Webhook: config.WebhookConfig{
			Enabled: true,
		},
		Log: config.LogConfig{
			Level:  "debug",
			Format: "console",
		},
		Observability: config.ObservabilityConfig{
			TracingEnabled: false,
			Environment:    "test",
		},
	}

	if tc.Redis != nil {
		cfg.Redis = config.RedisConfig{
			URL:          tc.Redis.URL,
			PoolSize:     5,
			MinIdleConns: 1,
			DialTimeout:  5 * time.Second,
			ReadTimeout:  3 * time.Second,
			WriteTimeout: 3 * time.Second,
		}
	}

	return cfg
}

// IsDockerAvailable checks if Docker is available for running containers.
func IsDockerAvailable() (available bool) {
	defer func() {
		if r := recover(); r != nil {
			// If testcontainers panics while inspecting Docker host, treat as unavailable.
			available = false
		}
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Try to get docker info
	provider, err := testcontainers.ProviderDocker.GetProvider()
	if err != nil {
		return false
	}

	err = provider.Health(ctx)
	return err == nil
}
