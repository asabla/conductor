// Package agent provides the Conductor agent implementation.
// The agent connects to the control plane, receives work assignments,
// executes tests, and reports results.
package agent

import (
	"errors"
	"fmt"
	"os"
	"runtime"
	"strconv"
	"strings"
	"time"
)

// Config holds all configuration settings for the agent.
type Config struct {
	// AgentID is the unique identifier for this agent instance.
	// If empty, will be generated from hostname.
	AgentID string

	// AgentName is a human-readable name for the agent.
	AgentName string

	// ControlPlaneURL is the gRPC endpoint of the control plane (required).
	ControlPlaneURL string

	// AgentToken is the authentication token for the control plane (required).
	AgentToken string

	// NetworkZones are the network zones this agent can access.
	NetworkZones []string

	// Runtimes are the available runtime environments (e.g., "node18", "python3.11").
	Runtimes []string

	// Labels are key-value pairs for agent selection and filtering.
	Labels map[string]string

	// MaxParallel is the maximum number of parallel test runs (default: 4).
	MaxParallel int

	// WorkspaceDir is the base directory for test workspaces (default: /tmp/conductor/workspaces).
	WorkspaceDir string

	// CacheDir is the directory for caching repositories (default: /tmp/conductor/cache).
	CacheDir string

	// StateDir is the directory for persistent state (default: /var/lib/conductor).
	StateDir string

	// HeartbeatInterval is the interval for sending heartbeats (default: 30s).
	HeartbeatInterval time.Duration

	// ReconnectMinInterval is the minimum reconnection interval (default: 1s).
	ReconnectMinInterval time.Duration

	// ReconnectMaxInterval is the maximum reconnection interval (default: 60s).
	ReconnectMaxInterval time.Duration

	// DefaultTimeout is the default timeout for test execution (default: 30m).
	DefaultTimeout time.Duration

	// LogLevel is the log level (debug, info, warn, error) (default: info).
	LogLevel string

	// LogFormat is the log format (json, console) (default: json).
	LogFormat string

	// TLSEnabled enables TLS for the control plane connection.
	TLSEnabled bool

	// TLSCertFile is the path to the TLS certificate file.
	TLSCertFile string

	// TLSKeyFile is the path to the TLS key file.
	TLSKeyFile string

	// TLSCAFile is the path to the CA certificate file for verifying the control plane.
	TLSCAFile string

	// TLSInsecureSkipVerify skips TLS certificate verification (not recommended).
	TLSInsecureSkipVerify bool

	// DockerEnabled enables Docker container execution mode.
	DockerEnabled bool

	// DockerHost is the Docker daemon socket (default: unix:///var/run/docker.sock).
	DockerHost string

	// StorageEndpoint is the S3/MinIO endpoint for artifact uploads.
	StorageEndpoint string

	// StorageAccessKey is the access key for artifact storage.
	StorageAccessKey string

	// StorageSecretKey is the secret key for artifact storage.
	StorageSecretKey string

	// StorageBucket is the bucket name for artifacts.
	StorageBucket string

	// StorageRegion is the region for S3 storage (default: us-east-1).
	StorageRegion string

	// StorageUseSSL enables SSL for storage connections (default: true).
	StorageUseSSL bool

	// SecretsProvider enables secret resolution (vault).
	SecretsProvider string

	// VaultAddress is the Vault API address for secret resolution.
	VaultAddress string

	// VaultToken is the Vault token for secret resolution.
	VaultToken string

	// VaultNamespace is the optional Vault namespace header.
	VaultNamespace string

	// VaultMount is the KV mount path (default: secret).
	VaultMount string

	// VaultTimeout is the HTTP timeout for Vault requests (default: 10s).
	VaultTimeout time.Duration

	// ResourceCheckInterval is how often to check system resources (default: 10s).
	ResourceCheckInterval time.Duration

	// CPUThreshold is the CPU usage threshold above which no new work is accepted (default: 90).
	CPUThreshold float64

	// MemoryThreshold is the memory usage threshold percentage (default: 90).
	MemoryThreshold float64

	// DiskThreshold is the disk usage threshold percentage (default: 90).
	DiskThreshold float64
}

// Load reads agent configuration from environment variables.
// Environment variables use the CONDUCTOR_AGENT_ prefix.
func Load() (*Config, error) {
	hostname, _ := os.Hostname()
	if hostname == "" {
		hostname = "unknown"
	}

	cfg := &Config{
		AgentID:               getEnv("CONDUCTOR_AGENT_ID", hostname),
		AgentName:             getEnv("CONDUCTOR_AGENT_NAME", hostname),
		ControlPlaneURL:       getEnv("CONDUCTOR_AGENT_CONTROL_PLANE_URL", ""),
		AgentToken:            getEnv("CONDUCTOR_AGENT_TOKEN", ""),
		NetworkZones:          getEnvStringSlice("CONDUCTOR_AGENT_NETWORK_ZONES", []string{"default"}),
		Runtimes:              getEnvStringSlice("CONDUCTOR_AGENT_RUNTIMES", nil),
		Labels:                getEnvMap("CONDUCTOR_AGENT_LABELS"),
		MaxParallel:           getEnvInt("CONDUCTOR_AGENT_MAX_PARALLEL", 4),
		WorkspaceDir:          getEnv("CONDUCTOR_AGENT_WORKSPACE_DIR", "/tmp/conductor/workspaces"),
		CacheDir:              getEnv("CONDUCTOR_AGENT_CACHE_DIR", "/tmp/conductor/cache"),
		StateDir:              getEnv("CONDUCTOR_AGENT_STATE_DIR", "/var/lib/conductor"),
		HeartbeatInterval:     getEnvDuration("CONDUCTOR_AGENT_HEARTBEAT_INTERVAL", 30*time.Second),
		ReconnectMinInterval:  getEnvDuration("CONDUCTOR_AGENT_RECONNECT_MIN_INTERVAL", 1*time.Second),
		ReconnectMaxInterval:  getEnvDuration("CONDUCTOR_AGENT_RECONNECT_MAX_INTERVAL", 60*time.Second),
		DefaultTimeout:        getEnvDuration("CONDUCTOR_AGENT_DEFAULT_TIMEOUT", 30*time.Minute),
		LogLevel:              getEnv("CONDUCTOR_AGENT_LOG_LEVEL", "info"),
		LogFormat:             getEnv("CONDUCTOR_AGENT_LOG_FORMAT", "json"),
		TLSEnabled:            getEnvBool("CONDUCTOR_AGENT_TLS_ENABLED", false),
		TLSCertFile:           getEnv("CONDUCTOR_AGENT_TLS_CERT_FILE", ""),
		TLSKeyFile:            getEnv("CONDUCTOR_AGENT_TLS_KEY_FILE", ""),
		TLSCAFile:             getEnv("CONDUCTOR_AGENT_TLS_CA_FILE", ""),
		TLSInsecureSkipVerify: getEnvBool("CONDUCTOR_AGENT_TLS_INSECURE_SKIP_VERIFY", false),
		DockerEnabled:         getEnvBool("CONDUCTOR_AGENT_DOCKER_ENABLED", true),
		DockerHost:            getEnv("CONDUCTOR_AGENT_DOCKER_HOST", "unix:///var/run/docker.sock"),
		StorageEndpoint:       getEnv("CONDUCTOR_AGENT_STORAGE_ENDPOINT", ""),
		StorageAccessKey:      getEnv("CONDUCTOR_AGENT_STORAGE_ACCESS_KEY", ""),
		StorageSecretKey:      getEnv("CONDUCTOR_AGENT_STORAGE_SECRET_KEY", ""),
		StorageBucket:         getEnv("CONDUCTOR_AGENT_STORAGE_BUCKET", ""),
		StorageRegion:         getEnv("CONDUCTOR_AGENT_STORAGE_REGION", "us-east-1"),
		StorageUseSSL:         getEnvBool("CONDUCTOR_AGENT_STORAGE_USE_SSL", true),
		SecretsProvider:       getEnv("CONDUCTOR_AGENT_SECRETS_PROVIDER", ""),
		VaultAddress:          getEnv("CONDUCTOR_AGENT_SECRETS_VAULT_ADDR", ""),
		VaultToken:            getEnv("CONDUCTOR_AGENT_SECRETS_VAULT_TOKEN", ""),
		VaultNamespace:        getEnv("CONDUCTOR_AGENT_SECRETS_VAULT_NAMESPACE", ""),
		VaultMount:            getEnv("CONDUCTOR_AGENT_SECRETS_VAULT_MOUNT", "secret"),
		VaultTimeout:          getEnvDuration("CONDUCTOR_AGENT_SECRETS_VAULT_TIMEOUT", 10*time.Second),
		ResourceCheckInterval: getEnvDuration("CONDUCTOR_AGENT_RESOURCE_CHECK_INTERVAL", 10*time.Second),
		CPUThreshold:          getEnvFloat64("CONDUCTOR_AGENT_CPU_THRESHOLD", 90.0),
		MemoryThreshold:       getEnvFloat64("CONDUCTOR_AGENT_MEMORY_THRESHOLD", 90.0),
		DiskThreshold:         getEnvFloat64("CONDUCTOR_AGENT_DISK_THRESHOLD", 90.0),
	}

	if cfg.SecretsProvider == "" && cfg.VaultAddress != "" {
		cfg.SecretsProvider = "vault"
	}

	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("config validation failed: %w", err)
	}

	return cfg, nil
}

// Validate checks that all required configuration fields are set and valid.
func (c *Config) Validate() error {
	var errs []error

	// Required fields
	if c.AgentID == "" {
		errs = append(errs, errors.New("CONDUCTOR_AGENT_ID is required"))
	}
	if c.ControlPlaneURL == "" {
		errs = append(errs, errors.New("CONDUCTOR_AGENT_CONTROL_PLANE_URL is required"))
	}
	if c.AgentToken == "" {
		errs = append(errs, errors.New("CONDUCTOR_AGENT_TOKEN is required"))
	}

	// Validate MaxParallel
	if c.MaxParallel < 1 {
		errs = append(errs, errors.New("CONDUCTOR_AGENT_MAX_PARALLEL must be at least 1"))
	}
	if c.MaxParallel > 100 {
		errs = append(errs, errors.New("CONDUCTOR_AGENT_MAX_PARALLEL cannot exceed 100"))
	}

	// Validate directories are absolute paths
	if c.WorkspaceDir != "" && !strings.HasPrefix(c.WorkspaceDir, "/") && runtime.GOOS != "windows" {
		errs = append(errs, errors.New("CONDUCTOR_AGENT_WORKSPACE_DIR must be an absolute path"))
	}
	if c.CacheDir != "" && !strings.HasPrefix(c.CacheDir, "/") && runtime.GOOS != "windows" {
		errs = append(errs, errors.New("CONDUCTOR_AGENT_CACHE_DIR must be an absolute path"))
	}
	if c.StateDir != "" && !strings.HasPrefix(c.StateDir, "/") && runtime.GOOS != "windows" {
		errs = append(errs, errors.New("CONDUCTOR_AGENT_STATE_DIR must be an absolute path"))
	}

	// Validate intervals
	if c.HeartbeatInterval < 5*time.Second {
		errs = append(errs, errors.New("CONDUCTOR_AGENT_HEARTBEAT_INTERVAL must be at least 5 seconds"))
	}
	if c.ReconnectMinInterval < 100*time.Millisecond {
		errs = append(errs, errors.New("CONDUCTOR_AGENT_RECONNECT_MIN_INTERVAL must be at least 100ms"))
	}
	if c.ReconnectMaxInterval < c.ReconnectMinInterval {
		errs = append(errs, errors.New("CONDUCTOR_AGENT_RECONNECT_MAX_INTERVAL must be >= MIN_INTERVAL"))
	}
	if c.DefaultTimeout < 1*time.Minute {
		errs = append(errs, errors.New("CONDUCTOR_AGENT_DEFAULT_TIMEOUT must be at least 1 minute"))
	}

	// Validate log settings
	validLevels := map[string]bool{"debug": true, "info": true, "warn": true, "error": true}
	if !validLevels[strings.ToLower(c.LogLevel)] {
		errs = append(errs, errors.New("CONDUCTOR_AGENT_LOG_LEVEL must be one of: debug, info, warn, error"))
	}
	validFormats := map[string]bool{"json": true, "console": true}
	if !validFormats[strings.ToLower(c.LogFormat)] {
		errs = append(errs, errors.New("CONDUCTOR_AGENT_LOG_FORMAT must be one of: json, console"))
	}

	// Validate TLS settings
	if c.TLSEnabled {
		if c.TLSCertFile == "" {
			errs = append(errs, errors.New("CONDUCTOR_AGENT_TLS_CERT_FILE is required when TLS is enabled"))
		}
		if c.TLSKeyFile == "" {
			errs = append(errs, errors.New("CONDUCTOR_AGENT_TLS_KEY_FILE is required when TLS is enabled"))
		}
	}

	// Validate secrets settings
	if c.SecretsProvider != "" && c.SecretsProvider != "vault" {
		errs = append(errs, errors.New("CONDUCTOR_AGENT_SECRETS_PROVIDER must be empty or 'vault'"))
	}
	if c.SecretsProvider == "vault" {
		if c.VaultAddress == "" {
			errs = append(errs, errors.New("CONDUCTOR_AGENT_SECRETS_VAULT_ADDR is required when secrets provider is vault"))
		}
		if c.VaultToken == "" {
			errs = append(errs, errors.New("CONDUCTOR_AGENT_SECRETS_VAULT_TOKEN is required when secrets provider is vault"))
		}
	}

	// Validate resource thresholds
	if c.CPUThreshold <= 0 || c.CPUThreshold > 100 {
		errs = append(errs, errors.New("CONDUCTOR_AGENT_CPU_THRESHOLD must be between 0 and 100"))
	}
	if c.MemoryThreshold <= 0 || c.MemoryThreshold > 100 {
		errs = append(errs, errors.New("CONDUCTOR_AGENT_MEMORY_THRESHOLD must be between 0 and 100"))
	}
	if c.DiskThreshold <= 0 || c.DiskThreshold > 100 {
		errs = append(errs, errors.New("CONDUCTOR_AGENT_DISK_THRESHOLD must be between 0 and 100"))
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

func getEnvFloat64(key string, defaultValue float64) float64 {
	if value := os.Getenv(key); value != "" {
		if floatVal, err := strconv.ParseFloat(value, 64); err == nil {
			return floatVal
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

func getEnvStringSlice(key string, defaultValue []string) []string {
	if value := os.Getenv(key); value != "" {
		parts := strings.Split(value, ",")
		result := make([]string, 0, len(parts))
		for _, part := range parts {
			trimmed := strings.TrimSpace(part)
			if trimmed != "" {
				result = append(result, trimmed)
			}
		}
		if len(result) > 0 {
			return result
		}
	}
	return defaultValue
}

func getEnvMap(key string) map[string]string {
	result := make(map[string]string)
	if value := os.Getenv(key); value != "" {
		pairs := strings.Split(value, ",")
		for _, pair := range pairs {
			kv := strings.SplitN(strings.TrimSpace(pair), "=", 2)
			if len(kv) == 2 {
				result[strings.TrimSpace(kv[0])] = strings.TrimSpace(kv[1])
			}
		}
	}
	return result
}
