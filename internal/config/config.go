// Package config provides configuration management for the Conductor control plane.
// Configuration is loaded from environment variables with the CONDUCTOR_ prefix.
package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// Config holds all configuration settings for the control plane.
type Config struct {
	Server        ServerConfig
	Database      DatabaseConfig
	Storage       StorageConfig
	Redis         RedisConfig
	Auth          AuthConfig
	Agent         AgentConfig
	Git           GitConfig
	Webhook       WebhookConfig
	Notifications NotificationConfig
	Log           LogConfig
	Observability ObservabilityConfig
}

// ServerConfig holds HTTP, gRPC, and metrics server settings.
type ServerConfig struct {
	// HTTPPort is the port for the REST API and dashboard (default: 8080)
	HTTPPort int
	// GRPCPort is the port for gRPC services (default: 9090)
	GRPCPort int
	// MetricsPort is the port for Prometheus metrics (default: 9091)
	MetricsPort int
	// ShutdownTimeout is the graceful shutdown timeout (default: 30s)
	ShutdownTimeout time.Duration
}

// DatabaseConfig holds PostgreSQL connection settings.
type DatabaseConfig struct {
	// URL is the PostgreSQL connection string (required)
	URL string
	// MaxOpenConns is the maximum number of open connections (default: 25)
	MaxOpenConns int
	// MaxIdleConns is the maximum number of idle connections (default: 5)
	MaxIdleConns int
	// ConnMaxLifetime is the maximum connection lifetime (default: 5m)
	ConnMaxLifetime time.Duration
	// ConnMaxIdleTime is the maximum idle time for connections (default: 1m)
	ConnMaxIdleTime time.Duration
	// QueryTimeout is the default query timeout (default: 30s)
	QueryTimeout time.Duration
}

// StorageConfig holds S3/MinIO artifact storage settings.
type StorageConfig struct {
	// Endpoint is the S3/MinIO endpoint URL (required for MinIO, empty for AWS S3)
	Endpoint string
	// Bucket is the bucket name for artifacts (required)
	Bucket string
	// Region is the AWS region (default: us-east-1)
	Region string
	// AccessKeyID is the access key (required)
	AccessKeyID string
	// SecretAccessKey is the secret key (required)
	SecretAccessKey string
	// UseSSL enables SSL for MinIO connections (default: true)
	UseSSL bool
	// PathStyle forces path-style addressing (default: true for MinIO compatibility)
	PathStyle bool
	// CleanupEnabled enables periodic artifact cleanup (default: false)
	CleanupEnabled bool
	// CleanupInterval is how often to run cleanup (default: 1h)
	CleanupInterval time.Duration
	// RetentionPeriod controls how long to keep artifacts (default: 30d)
	RetentionPeriod time.Duration
	// CleanupBatchSize limits artifacts deleted per run (default: 100)
	CleanupBatchSize int
}

// RedisConfig holds Redis connection settings.
type RedisConfig struct {
	// URL is the Redis connection URL (optional, enables caching if set)
	URL string
	// PoolSize is the connection pool size (default: 10)
	PoolSize int
	// MinIdleConns is the minimum number of idle connections (default: 2)
	MinIdleConns int
	// DialTimeout is the connection timeout (default: 5s)
	DialTimeout time.Duration
	// ReadTimeout is the read timeout (default: 3s)
	ReadTimeout time.Duration
	// WriteTimeout is the write timeout (default: 3s)
	WriteTimeout time.Duration
}

// AuthConfig holds authentication and authorization settings.
type AuthConfig struct {
	// JWTSecret is the secret key for JWT signing (required)
	JWTSecret string
	// JWTExpiration is the JWT token expiration time (default: 24h)
	JWTExpiration time.Duration
	// OIDCEnabled enables OIDC authentication (default: false)
	OIDCEnabled bool
	// OIDCIssuerURL is the OIDC provider's issuer URL
	OIDCIssuerURL string
	// OIDCClientID is the OIDC client ID
	OIDCClientID string
	// OIDCClientSecret is the OIDC client secret
	OIDCClientSecret string
	// OIDCRedirectURL is the callback URL for OIDC
	OIDCRedirectURL string
}

// AgentConfig holds agent-related settings.
type AgentConfig struct {
	// HeartbeatTimeout is how long before an agent is considered offline (default: 90s)
	HeartbeatTimeout time.Duration
	// DefaultTestTimeout is the default timeout for test execution (default: 30m)
	DefaultTestTimeout time.Duration
	// MaxTestTimeout is the maximum allowed test timeout (default: 4h)
	MaxTestTimeout time.Duration
	// ResultStreamBufferSize is the buffer size for result streaming (default: 100)
	ResultStreamBufferSize int
}

// GitConfig holds git provider settings.
type GitConfig struct {
	// Provider is the git provider type (github, gitlab, bitbucket) (default: github)
	Provider string
	// Token is the personal access token for the git provider
	Token string
	// BaseURL is the API base URL for enterprise/self-hosted instances
	BaseURL string
	// WebhookSecret is the secret for validating incoming webhooks (GitHub)
	WebhookSecret string
	// GitLabWebhookSecret is the secret token for GitLab webhooks
	GitLabWebhookSecret string
	// BitbucketWebhookSecret is the secret for Bitbucket webhooks
	BitbucketWebhookSecret string
	// AppID is the GitHub App ID (optional, for app authentication)
	AppID int64
	// AppPrivateKeyPath is the path to the GitHub App private key file
	AppPrivateKeyPath string
	// AppInstallationID is the GitHub App installation ID
	AppInstallationID int64
}

// WebhookConfig holds configuration for webhook handling.
type WebhookConfig struct {
	// Enabled enables webhook handling (default: true if any secret is set)
	Enabled bool
	// BaseURL is the external URL for the control plane (used in status URLs)
	BaseURL string
}

// NotificationConfig holds notification-related settings.
type NotificationConfig struct {
	Email EmailConfig
}

// EmailConfig holds SMTP settings for email notifications.
type EmailConfig struct {
	SMTPHost    string
	SMTPPort    int
	Username    string
	Password    string
	FromAddress string
	FromName    string
	UseTLS      bool
	SkipVerify  bool
	ConnTimeout time.Duration
}

// LogConfig holds logging settings.
type LogConfig struct {
	// Level is the log level (debug, info, warn, error) (default: info)
	Level string
	// Format is the log format (json, console) (default: json)
	Format string
}

// ObservabilityConfig holds observability settings.
type ObservabilityConfig struct {
	// TracingEnabled enables OpenTelemetry tracing (default: false)
	TracingEnabled bool
	// TracingEndpoint is the OTLP collector endpoint (e.g., "localhost:4318")
	TracingEndpoint string
	// TracingInsecure disables TLS for the tracing connection (default: true)
	TracingInsecure bool
	// TracingSampleRate is the sampling rate (0.0 to 1.0) (default: 1.0)
	TracingSampleRate float64
	// Environment is the deployment environment (e.g., "production", "staging")
	Environment string
}

// Load reads configuration from environment variables.
// Environment variables use the CONDUCTOR_ prefix.
func Load() (*Config, error) {
	cfg := &Config{
		Server: ServerConfig{
			HTTPPort:        getEnvInt("CONDUCTOR_HTTP_PORT", 8080),
			GRPCPort:        getEnvInt("CONDUCTOR_GRPC_PORT", 9090),
			MetricsPort:     getEnvInt("CONDUCTOR_METRICS_PORT", 9091),
			ShutdownTimeout: getEnvDuration("CONDUCTOR_SHUTDOWN_TIMEOUT", 30*time.Second),
		},
		Database: DatabaseConfig{
			URL:             getEnv("CONDUCTOR_DATABASE_URL", ""),
			MaxOpenConns:    getEnvInt("CONDUCTOR_DATABASE_MAX_OPEN_CONNS", 25),
			MaxIdleConns:    getEnvInt("CONDUCTOR_DATABASE_MAX_IDLE_CONNS", 5),
			ConnMaxLifetime: getEnvDuration("CONDUCTOR_DATABASE_CONN_MAX_LIFETIME", 5*time.Minute),
			ConnMaxIdleTime: getEnvDuration("CONDUCTOR_DATABASE_CONN_MAX_IDLE_TIME", 1*time.Minute),
			QueryTimeout:    getEnvDuration("CONDUCTOR_DATABASE_QUERY_TIMEOUT", 30*time.Second),
		},
		Storage: StorageConfig{
			Endpoint:         getEnv("CONDUCTOR_STORAGE_ENDPOINT", ""),
			Bucket:           getEnv("CONDUCTOR_STORAGE_BUCKET", ""),
			Region:           getEnv("CONDUCTOR_STORAGE_REGION", "us-east-1"),
			AccessKeyID:      getEnv("CONDUCTOR_STORAGE_ACCESS_KEY_ID", ""),
			SecretAccessKey:  getEnv("CONDUCTOR_STORAGE_SECRET_ACCESS_KEY", ""),
			UseSSL:           getEnvBool("CONDUCTOR_STORAGE_USE_SSL", true),
			PathStyle:        getEnvBool("CONDUCTOR_STORAGE_PATH_STYLE", true),
			CleanupEnabled:   getEnvBool("CONDUCTOR_STORAGE_CLEANUP_ENABLED", false),
			CleanupInterval:  getEnvDuration("CONDUCTOR_STORAGE_CLEANUP_INTERVAL", time.Hour),
			RetentionPeriod:  getEnvDuration("CONDUCTOR_STORAGE_RETENTION", 30*24*time.Hour),
			CleanupBatchSize: getEnvInt("CONDUCTOR_STORAGE_CLEANUP_BATCH_SIZE", 100),
		},
		Redis: RedisConfig{
			URL:          getEnv("CONDUCTOR_REDIS_URL", ""),
			PoolSize:     getEnvInt("CONDUCTOR_REDIS_POOL_SIZE", 10),
			MinIdleConns: getEnvInt("CONDUCTOR_REDIS_MIN_IDLE_CONNS", 2),
			DialTimeout:  getEnvDuration("CONDUCTOR_REDIS_DIAL_TIMEOUT", 5*time.Second),
			ReadTimeout:  getEnvDuration("CONDUCTOR_REDIS_READ_TIMEOUT", 3*time.Second),
			WriteTimeout: getEnvDuration("CONDUCTOR_REDIS_WRITE_TIMEOUT", 3*time.Second),
		},
		Auth: AuthConfig{
			JWTSecret:        getEnv("CONDUCTOR_AUTH_JWT_SECRET", ""),
			JWTExpiration:    getEnvDuration("CONDUCTOR_AUTH_JWT_EXPIRATION", 24*time.Hour),
			OIDCEnabled:      getEnvBool("CONDUCTOR_AUTH_OIDC_ENABLED", false),
			OIDCIssuerURL:    getEnv("CONDUCTOR_AUTH_OIDC_ISSUER_URL", ""),
			OIDCClientID:     getEnv("CONDUCTOR_AUTH_OIDC_CLIENT_ID", ""),
			OIDCClientSecret: getEnv("CONDUCTOR_AUTH_OIDC_CLIENT_SECRET", ""),
			OIDCRedirectURL:  getEnv("CONDUCTOR_AUTH_OIDC_REDIRECT_URL", ""),
		},
		Agent: AgentConfig{
			HeartbeatTimeout:       getEnvDuration("CONDUCTOR_AGENT_HEARTBEAT_TIMEOUT", 90*time.Second),
			DefaultTestTimeout:     getEnvDuration("CONDUCTOR_AGENT_DEFAULT_TEST_TIMEOUT", 30*time.Minute),
			MaxTestTimeout:         getEnvDuration("CONDUCTOR_AGENT_MAX_TEST_TIMEOUT", 4*time.Hour),
			ResultStreamBufferSize: getEnvInt("CONDUCTOR_AGENT_RESULT_STREAM_BUFFER_SIZE", 100),
		},
		Git: GitConfig{
			Provider:               getEnv("CONDUCTOR_GIT_PROVIDER", "github"),
			Token:                  getEnv("CONDUCTOR_GIT_TOKEN", ""),
			BaseURL:                getEnv("CONDUCTOR_GIT_BASE_URL", ""),
			WebhookSecret:          getEnv("CONDUCTOR_GIT_WEBHOOK_SECRET", ""),
			GitLabWebhookSecret:    getEnv("CONDUCTOR_GITLAB_WEBHOOK_SECRET", ""),
			BitbucketWebhookSecret: getEnv("CONDUCTOR_BITBUCKET_WEBHOOK_SECRET", ""),
			AppID:                  int64(getEnvInt("CONDUCTOR_GIT_APP_ID", 0)),
			AppPrivateKeyPath:      getEnv("CONDUCTOR_GIT_APP_PRIVATE_KEY_PATH", ""),
			AppInstallationID:      int64(getEnvInt("CONDUCTOR_GIT_APP_INSTALLATION_ID", 0)),
		},
		Webhook: WebhookConfig{
			Enabled: getEnvBool("CONDUCTOR_WEBHOOK_ENABLED", true),
			BaseURL: getEnv("CONDUCTOR_WEBHOOK_BASE_URL", ""),
		},
		Notifications: NotificationConfig{
			Email: EmailConfig{
				SMTPHost:    getEnv("CONDUCTOR_NOTIFICATIONS_EMAIL_SMTP_HOST", ""),
				SMTPPort:    getEnvInt("CONDUCTOR_NOTIFICATIONS_EMAIL_SMTP_PORT", 0),
				Username:    getEnv("CONDUCTOR_NOTIFICATIONS_EMAIL_USERNAME", ""),
				Password:    getEnv("CONDUCTOR_NOTIFICATIONS_EMAIL_PASSWORD", ""),
				FromAddress: getEnv("CONDUCTOR_NOTIFICATIONS_EMAIL_FROM_ADDRESS", ""),
				FromName:    getEnv("CONDUCTOR_NOTIFICATIONS_EMAIL_FROM_NAME", ""),
				UseTLS:      getEnvBool("CONDUCTOR_NOTIFICATIONS_EMAIL_USE_TLS", true),
				SkipVerify:  getEnvBool("CONDUCTOR_NOTIFICATIONS_EMAIL_SKIP_VERIFY", false),
				ConnTimeout: getEnvDuration("CONDUCTOR_NOTIFICATIONS_EMAIL_CONN_TIMEOUT", 30*time.Second),
			},
		},
		Log: LogConfig{
			Level:  getEnv("CONDUCTOR_LOG_LEVEL", "info"),
			Format: getEnv("CONDUCTOR_LOG_FORMAT", "json"),
		},
		Observability: ObservabilityConfig{
			TracingEnabled:    getEnvBool("CONDUCTOR_TRACING_ENABLED", false),
			TracingEndpoint:   getEnv("CONDUCTOR_TRACING_ENDPOINT", ""),
			TracingInsecure:   getEnvBool("CONDUCTOR_TRACING_INSECURE", true),
			TracingSampleRate: getEnvFloat("CONDUCTOR_TRACING_SAMPLE_RATE", 1.0),
			Environment:       getEnv("CONDUCTOR_ENVIRONMENT", "development"),
		},
	}

	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("config validation failed: %w", err)
	}

	return cfg, nil
}

// Validate checks that all required configuration fields are set and valid.
func (c *Config) Validate() error {
	var errs []error

	// Server validation
	if c.Server.HTTPPort < 1 || c.Server.HTTPPort > 65535 {
		errs = append(errs, errors.New("CONDUCTOR_HTTP_PORT must be between 1 and 65535"))
	}

	if c.Notifications.Email.SMTPHost != "" {
		if c.Notifications.Email.SMTPPort <= 0 {
			errs = append(errs, errors.New("CONDUCTOR_NOTIFICATIONS_EMAIL_SMTP_PORT must be set when SMTP host is configured"))
		}
		if c.Notifications.Email.FromAddress == "" {
			errs = append(errs, errors.New("CONDUCTOR_NOTIFICATIONS_EMAIL_FROM_ADDRESS must be set when SMTP host is configured"))
		}
	}
	if c.Server.GRPCPort < 1 || c.Server.GRPCPort > 65535 {
		errs = append(errs, errors.New("CONDUCTOR_GRPC_PORT must be between 1 and 65535"))
	}
	if c.Server.MetricsPort < 1 || c.Server.MetricsPort > 65535 {
		errs = append(errs, errors.New("CONDUCTOR_METRICS_PORT must be between 1 and 65535"))
	}

	// Database validation (required)
	if c.Database.URL == "" {
		errs = append(errs, errors.New("CONDUCTOR_DATABASE_URL is required"))
	}
	if c.Database.MaxOpenConns < 1 {
		errs = append(errs, errors.New("CONDUCTOR_DATABASE_MAX_OPEN_CONNS must be at least 1"))
	}
	if c.Database.MaxIdleConns < 0 {
		errs = append(errs, errors.New("CONDUCTOR_DATABASE_MAX_IDLE_CONNS cannot be negative"))
	}
	if c.Database.MaxIdleConns > c.Database.MaxOpenConns {
		errs = append(errs, errors.New("CONDUCTOR_DATABASE_MAX_IDLE_CONNS cannot exceed MAX_OPEN_CONNS"))
	}

	// Storage validation (required)
	if c.Storage.Bucket == "" {
		errs = append(errs, errors.New("CONDUCTOR_STORAGE_BUCKET is required"))
	}
	if c.Storage.CleanupEnabled {
		if c.Storage.RetentionPeriod <= 0 {
			errs = append(errs, errors.New("CONDUCTOR_STORAGE_RETENTION must be greater than 0 when cleanup is enabled"))
		}
		if c.Storage.CleanupInterval <= 0 {
			errs = append(errs, errors.New("CONDUCTOR_STORAGE_CLEANUP_INTERVAL must be greater than 0 when cleanup is enabled"))
		}
		if c.Storage.CleanupBatchSize <= 0 {
			errs = append(errs, errors.New("CONDUCTOR_STORAGE_CLEANUP_BATCH_SIZE must be greater than 0 when cleanup is enabled"))
		}
	}
	if c.Storage.AccessKeyID == "" {
		errs = append(errs, errors.New("CONDUCTOR_STORAGE_ACCESS_KEY_ID is required"))
	}
	if c.Storage.SecretAccessKey == "" {
		errs = append(errs, errors.New("CONDUCTOR_STORAGE_SECRET_ACCESS_KEY is required"))
	}

	// Auth validation (required)
	if c.Auth.JWTSecret == "" {
		errs = append(errs, errors.New("CONDUCTOR_AUTH_JWT_SECRET is required"))
	}
	if len(c.Auth.JWTSecret) < 32 {
		errs = append(errs, errors.New("CONDUCTOR_AUTH_JWT_SECRET must be at least 32 characters"))
	}

	// OIDC validation (conditional)
	if c.Auth.OIDCEnabled {
		if c.Auth.OIDCIssuerURL == "" {
			errs = append(errs, errors.New("CONDUCTOR_AUTH_OIDC_ISSUER_URL is required when OIDC is enabled"))
		}
		if c.Auth.OIDCClientID == "" {
			errs = append(errs, errors.New("CONDUCTOR_AUTH_OIDC_CLIENT_ID is required when OIDC is enabled"))
		}
		if c.Auth.OIDCClientSecret == "" {
			errs = append(errs, errors.New("CONDUCTOR_AUTH_OIDC_CLIENT_SECRET is required when OIDC is enabled"))
		}
		if c.Auth.OIDCRedirectURL == "" {
			errs = append(errs, errors.New("CONDUCTOR_AUTH_OIDC_REDIRECT_URL is required when OIDC is enabled"))
		}
	}

	// Agent validation
	if c.Agent.HeartbeatTimeout < 10*time.Second {
		errs = append(errs, errors.New("CONDUCTOR_AGENT_HEARTBEAT_TIMEOUT must be at least 10 seconds"))
	}
	if c.Agent.DefaultTestTimeout < 1*time.Minute {
		errs = append(errs, errors.New("CONDUCTOR_AGENT_DEFAULT_TEST_TIMEOUT must be at least 1 minute"))
	}
	if c.Agent.MaxTestTimeout < c.Agent.DefaultTestTimeout {
		errs = append(errs, errors.New("CONDUCTOR_AGENT_MAX_TEST_TIMEOUT must be >= DEFAULT_TEST_TIMEOUT"))
	}

	// Log validation
	validLevels := map[string]bool{"debug": true, "info": true, "warn": true, "error": true}
	if !validLevels[strings.ToLower(c.Log.Level)] {
		errs = append(errs, errors.New("CONDUCTOR_LOG_LEVEL must be one of: debug, info, warn, error"))
	}
	validFormats := map[string]bool{"json": true, "console": true}
	if !validFormats[strings.ToLower(c.Log.Format)] {
		errs = append(errs, errors.New("CONDUCTOR_LOG_FORMAT must be one of: json, console"))
	}

	// GitHub App validation (conditional)
	appConfigured := c.Git.AppID > 0 || c.Git.AppPrivateKeyPath != "" || c.Git.AppInstallationID > 0
	if appConfigured {
		if c.Git.Provider != "" && strings.ToLower(c.Git.Provider) != "github" {
			errs = append(errs, errors.New("CONDUCTOR_GIT_APP_* settings require CONDUCTOR_GIT_PROVIDER to be github"))
		}
		if c.Git.AppID <= 0 {
			errs = append(errs, errors.New("CONDUCTOR_GIT_APP_ID is required when GitHub App authentication is enabled"))
		}
		if c.Git.AppPrivateKeyPath == "" {
			errs = append(errs, errors.New("CONDUCTOR_GIT_APP_PRIVATE_KEY_PATH is required when GitHub App authentication is enabled"))
		}
		if c.Git.AppInstallationID <= 0 {
			errs = append(errs, errors.New("CONDUCTOR_GIT_APP_INSTALLATION_ID is required when GitHub App authentication is enabled"))
		}
	}

	if len(errs) > 0 {
		return &ValidationError{Errors: errs}
	}

	return nil
}

// ValidationError contains multiple validation errors.
type ValidationError struct {
	Errors []error
}

func (e *ValidationError) Error() string {
	if len(e.Errors) == 1 {
		return e.Errors[0].Error()
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("%d validation errors:\n", len(e.Errors)))
	for i, err := range e.Errors {
		sb.WriteString(fmt.Sprintf("  %d. %s\n", i+1, err.Error()))
	}
	return sb.String()
}

// Unwrap returns the underlying errors for errors.Is/As compatibility.
func (e *ValidationError) Unwrap() []error {
	return e.Errors
}

// RedisEnabled returns true if Redis is configured.
func (c *Config) RedisEnabled() bool {
	return c.Redis.URL != ""
}

// GitEnabled returns true if Git provider is configured with a token.
func (c *Config) GitEnabled() bool {
	return c.Git.Token != "" || c.Git.AppID > 0 || c.Git.AppPrivateKeyPath != "" || c.Git.AppInstallationID > 0
}

// WebhooksEnabled returns true if webhook handling is enabled.
func (c *Config) WebhooksEnabled() bool {
	return c.Webhook.Enabled
}

// HasWebhookSecrets returns true if any webhook secret is configured.
func (c *Config) HasWebhookSecrets() bool {
	return c.Git.WebhookSecret != "" || c.Git.GitLabWebhookSecret != "" || c.Git.BitbucketWebhookSecret != ""
}

// Helper functions for reading environment variables

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func getEnvInt(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		if intVal, err := strconv.Atoi(value); err == nil {
			return intVal
		}
	}
	return defaultValue
}

func getEnvBool(key string, defaultValue bool) bool {
	if value := os.Getenv(key); value != "" {
		if boolVal, err := strconv.ParseBool(value); err == nil {
			return boolVal
		}
	}
	return defaultValue
}

func getEnvDuration(key string, defaultValue time.Duration) time.Duration {
	if value := os.Getenv(key); value != "" {
		if duration, err := time.ParseDuration(value); err == nil {
			return duration
		}
	}
	return defaultValue
}

func getEnvFloat(key string, defaultValue float64) float64 {
	if value := os.Getenv(key); value != "" {
		if floatVal, err := strconv.ParseFloat(value, 64); err == nil {
			return floatVal
		}
	}
	return defaultValue
}
