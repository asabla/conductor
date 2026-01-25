package config

import (
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// setTestEnv sets environment variables for testing and returns a cleanup function.
func setTestEnv(t *testing.T, envVars map[string]string) {
	t.Helper()

	// Store original values
	original := make(map[string]string)
	for key := range envVars {
		original[key] = os.Getenv(key)
	}

	// Set new values
	for key, value := range envVars {
		os.Setenv(key, value)
	}

	// Register cleanup
	t.Cleanup(func() {
		for key, value := range original {
			if value == "" {
				os.Unsetenv(key)
			} else {
				os.Setenv(key, value)
			}
		}
	})
}

// minimalValidEnv returns the minimum required environment variables for a valid config.
func minimalValidEnv() map[string]string {
	return map[string]string{
		"CONDUCTOR_DATABASE_URL":              "postgres://localhost/conductor",
		"CONDUCTOR_STORAGE_BUCKET":            "test-bucket",
		"CONDUCTOR_STORAGE_ACCESS_KEY_ID":     "minioadmin",
		"CONDUCTOR_STORAGE_SECRET_ACCESS_KEY": "minioadmin123",
		"CONDUCTOR_AUTH_JWT_SECRET":           "this-is-a-secret-key-at-least-32-chars",
	}
}

func TestLoad_WithValidConfig(t *testing.T) {
	env := minimalValidEnv()
	env["CONDUCTOR_HTTP_PORT"] = "8081"
	env["CONDUCTOR_GRPC_PORT"] = "9091"
	env["CONDUCTOR_LOG_LEVEL"] = "debug"
	env["CONDUCTOR_LOG_FORMAT"] = "console"
	setTestEnv(t, env)

	cfg, err := Load()
	require.NoError(t, err)

	assert.Equal(t, 8081, cfg.Server.HTTPPort)
	assert.Equal(t, 9091, cfg.Server.GRPCPort)
	assert.Equal(t, "debug", cfg.Log.Level)
	assert.Equal(t, "console", cfg.Log.Format)
}

func TestLoad_Defaults(t *testing.T) {
	setTestEnv(t, minimalValidEnv())

	cfg, err := Load()
	require.NoError(t, err)

	// Server defaults
	assert.Equal(t, 8080, cfg.Server.HTTPPort)
	assert.Equal(t, 9090, cfg.Server.GRPCPort)
	assert.Equal(t, 9091, cfg.Server.MetricsPort)
	assert.Equal(t, 30*time.Second, cfg.Server.ShutdownTimeout)

	// Database defaults
	assert.Equal(t, 25, cfg.Database.MaxOpenConns)
	assert.Equal(t, 5, cfg.Database.MaxIdleConns)
	assert.Equal(t, 5*time.Minute, cfg.Database.ConnMaxLifetime)
	assert.Equal(t, 30*time.Second, cfg.Database.QueryTimeout)

	// Storage defaults
	assert.Equal(t, "us-east-1", cfg.Storage.Region)
	assert.True(t, cfg.Storage.UseSSL)
	assert.True(t, cfg.Storage.PathStyle)

	// Redis defaults
	assert.Equal(t, 10, cfg.Redis.PoolSize)
	assert.Equal(t, 5*time.Second, cfg.Redis.DialTimeout)

	// Auth defaults
	assert.Equal(t, 24*time.Hour, cfg.Auth.JWTExpiration)
	assert.False(t, cfg.Auth.OIDCEnabled)

	// Agent defaults
	assert.Equal(t, 90*time.Second, cfg.Agent.HeartbeatTimeout)
	assert.Equal(t, 30*time.Minute, cfg.Agent.DefaultTestTimeout)
	assert.Equal(t, 4*time.Hour, cfg.Agent.MaxTestTimeout)
	assert.Equal(t, 100, cfg.Agent.ResultStreamBufferSize)

	// Log defaults
	assert.Equal(t, "info", cfg.Log.Level)
	assert.Equal(t, "json", cfg.Log.Format)
}

func TestLoad_MissingDatabaseURL(t *testing.T) {
	env := minimalValidEnv()
	delete(env, "CONDUCTOR_DATABASE_URL")
	setTestEnv(t, env)

	_, err := Load()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "CONDUCTOR_DATABASE_URL is required")
}

func TestLoad_MissingStorageBucket(t *testing.T) {
	env := minimalValidEnv()
	delete(env, "CONDUCTOR_STORAGE_BUCKET")
	setTestEnv(t, env)

	_, err := Load()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "CONDUCTOR_STORAGE_BUCKET is required")
}

func TestLoad_MissingStorageCredentials(t *testing.T) {
	env := minimalValidEnv()
	delete(env, "CONDUCTOR_STORAGE_ACCESS_KEY_ID")
	setTestEnv(t, env)

	_, err := Load()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "CONDUCTOR_STORAGE_ACCESS_KEY_ID is required")
}

func TestLoad_MissingJWTSecret(t *testing.T) {
	env := minimalValidEnv()
	delete(env, "CONDUCTOR_AUTH_JWT_SECRET")
	setTestEnv(t, env)

	_, err := Load()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "CONDUCTOR_AUTH_JWT_SECRET is required")
}

func TestLoad_ShortJWTSecret(t *testing.T) {
	env := minimalValidEnv()
	env["CONDUCTOR_AUTH_JWT_SECRET"] = "short"
	setTestEnv(t, env)

	_, err := Load()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "at least 32 characters")
}

func TestLoad_InvalidPort(t *testing.T) {
	tests := []struct {
		name    string
		envVar  string
		value   string
		wantErr string
	}{
		{
			name:    "HTTP port too high",
			envVar:  "CONDUCTOR_HTTP_PORT",
			value:   "99999",
			wantErr: "CONDUCTOR_HTTP_PORT must be between 1 and 65535",
		},
		{
			name:    "HTTP port zero",
			envVar:  "CONDUCTOR_HTTP_PORT",
			value:   "0",
			wantErr: "CONDUCTOR_HTTP_PORT must be between 1 and 65535",
		},
		{
			name:    "gRPC port invalid",
			envVar:  "CONDUCTOR_GRPC_PORT",
			value:   "0",
			wantErr: "CONDUCTOR_GRPC_PORT must be between 1 and 65535",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env := minimalValidEnv()
			env[tt.envVar] = tt.value
			setTestEnv(t, env)

			_, err := Load()
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}

func TestLoad_InvalidLogLevel(t *testing.T) {
	env := minimalValidEnv()
	env["CONDUCTOR_LOG_LEVEL"] = "invalid"
	setTestEnv(t, env)

	_, err := Load()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "CONDUCTOR_LOG_LEVEL must be one of")
}

func TestLoad_InvalidLogFormat(t *testing.T) {
	env := minimalValidEnv()
	env["CONDUCTOR_LOG_FORMAT"] = "xml"
	setTestEnv(t, env)

	_, err := Load()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "CONDUCTOR_LOG_FORMAT must be one of")
}

func TestLoad_OIDCEnabled_MissingFields(t *testing.T) {
	tests := []struct {
		name       string
		missingVar string
		wantErr    string
	}{
		{
			name:       "missing issuer URL",
			missingVar: "CONDUCTOR_AUTH_OIDC_ISSUER_URL",
			wantErr:    "CONDUCTOR_AUTH_OIDC_ISSUER_URL is required",
		},
		{
			name:       "missing client ID",
			missingVar: "CONDUCTOR_AUTH_OIDC_CLIENT_ID",
			wantErr:    "CONDUCTOR_AUTH_OIDC_CLIENT_ID is required",
		},
		{
			name:       "missing client secret",
			missingVar: "CONDUCTOR_AUTH_OIDC_CLIENT_SECRET",
			wantErr:    "CONDUCTOR_AUTH_OIDC_CLIENT_SECRET is required",
		},
		{
			name:       "missing redirect URL",
			missingVar: "CONDUCTOR_AUTH_OIDC_REDIRECT_URL",
			wantErr:    "CONDUCTOR_AUTH_OIDC_REDIRECT_URL is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env := minimalValidEnv()
			env["CONDUCTOR_AUTH_OIDC_ENABLED"] = "true"
			env["CONDUCTOR_AUTH_OIDC_ISSUER_URL"] = "https://issuer.example.com"
			env["CONDUCTOR_AUTH_OIDC_CLIENT_ID"] = "client-id"
			env["CONDUCTOR_AUTH_OIDC_CLIENT_SECRET"] = "client-secret"
			env["CONDUCTOR_AUTH_OIDC_REDIRECT_URL"] = "https://conductor.example.com/callback"
			delete(env, tt.missingVar)
			setTestEnv(t, env)

			_, err := Load()
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}

func TestLoad_OIDCEnabled_AllFieldsPresent(t *testing.T) {
	env := minimalValidEnv()
	env["CONDUCTOR_AUTH_OIDC_ENABLED"] = "true"
	env["CONDUCTOR_AUTH_OIDC_ISSUER_URL"] = "https://issuer.example.com"
	env["CONDUCTOR_AUTH_OIDC_CLIENT_ID"] = "client-id"
	env["CONDUCTOR_AUTH_OIDC_CLIENT_SECRET"] = "client-secret"
	env["CONDUCTOR_AUTH_OIDC_REDIRECT_URL"] = "https://conductor.example.com/callback"
	setTestEnv(t, env)

	cfg, err := Load()
	require.NoError(t, err)

	assert.True(t, cfg.Auth.OIDCEnabled)
	assert.Equal(t, "https://issuer.example.com", cfg.Auth.OIDCIssuerURL)
	assert.Equal(t, "client-id", cfg.Auth.OIDCClientID)
	assert.Equal(t, "client-secret", cfg.Auth.OIDCClientSecret)
	assert.Equal(t, "https://conductor.example.com/callback", cfg.Auth.OIDCRedirectURL)
}

func TestLoad_AgentHeartbeatTooShort(t *testing.T) {
	env := minimalValidEnv()
	env["CONDUCTOR_AGENT_HEARTBEAT_TIMEOUT"] = "5s"
	setTestEnv(t, env)

	_, err := Load()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "at least 10 seconds")
}

func TestLoad_AgentTestTimeoutTooShort(t *testing.T) {
	env := minimalValidEnv()
	env["CONDUCTOR_AGENT_DEFAULT_TEST_TIMEOUT"] = "30s"
	setTestEnv(t, env)

	_, err := Load()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "at least 1 minute")
}

func TestLoad_MaxTestTimeoutLessThanDefault(t *testing.T) {
	env := minimalValidEnv()
	env["CONDUCTOR_AGENT_DEFAULT_TEST_TIMEOUT"] = "1h"
	env["CONDUCTOR_AGENT_MAX_TEST_TIMEOUT"] = "30m"
	setTestEnv(t, env)

	_, err := Load()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "MAX_TEST_TIMEOUT must be >= DEFAULT_TEST_TIMEOUT")
}

func TestLoad_DatabaseMaxIdleExceedsMaxOpen(t *testing.T) {
	env := minimalValidEnv()
	env["CONDUCTOR_DATABASE_MAX_OPEN_CONNS"] = "5"
	env["CONDUCTOR_DATABASE_MAX_IDLE_CONNS"] = "10"
	setTestEnv(t, env)

	_, err := Load()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "MAX_IDLE_CONNS cannot exceed MAX_OPEN_CONNS")
}

func TestLoad_DurationParsing(t *testing.T) {
	env := minimalValidEnv()
	env["CONDUCTOR_SHUTDOWN_TIMEOUT"] = "45s"
	env["CONDUCTOR_DATABASE_QUERY_TIMEOUT"] = "1m30s"
	env["CONDUCTOR_AGENT_HEARTBEAT_TIMEOUT"] = "2m"
	setTestEnv(t, env)

	cfg, err := Load()
	require.NoError(t, err)

	assert.Equal(t, 45*time.Second, cfg.Server.ShutdownTimeout)
	assert.Equal(t, 90*time.Second, cfg.Database.QueryTimeout)
	assert.Equal(t, 2*time.Minute, cfg.Agent.HeartbeatTimeout)
}

func TestLoad_BoolParsing(t *testing.T) {
	env := minimalValidEnv()
	env["CONDUCTOR_STORAGE_USE_SSL"] = "false"
	env["CONDUCTOR_STORAGE_PATH_STYLE"] = "0"
	setTestEnv(t, env)

	cfg, err := Load()
	require.NoError(t, err)

	assert.False(t, cfg.Storage.UseSSL)
	assert.False(t, cfg.Storage.PathStyle)
}

func TestRedisEnabled(t *testing.T) {
	t.Run("enabled with URL", func(t *testing.T) {
		env := minimalValidEnv()
		env["CONDUCTOR_REDIS_URL"] = "redis://localhost:6379"
		setTestEnv(t, env)

		cfg, err := Load()
		require.NoError(t, err)
		assert.True(t, cfg.RedisEnabled())
	})

	t.Run("disabled without URL", func(t *testing.T) {
		setTestEnv(t, minimalValidEnv())

		cfg, err := Load()
		require.NoError(t, err)
		assert.False(t, cfg.RedisEnabled())
	})
}

func TestValidationError_SingleError(t *testing.T) {
	err := &ValidationError{
		Errors: []error{
			assert.AnError,
		},
	}
	assert.Equal(t, assert.AnError.Error(), err.Error())
}

func TestValidationError_MultipleErrors(t *testing.T) {
	err := &ValidationError{
		Errors: []error{
			assert.AnError,
			assert.AnError,
		},
	}
	msg := err.Error()
	assert.Contains(t, msg, "2 validation errors")
}

func TestValidationError_Unwrap(t *testing.T) {
	e1 := assert.AnError
	e2 := assert.AnError
	err := &ValidationError{
		Errors: []error{e1, e2},
	}

	unwrapped := err.Unwrap()
	assert.Len(t, unwrapped, 2)
	assert.Equal(t, e1, unwrapped[0])
	assert.Equal(t, e2, unwrapped[1])
}

func TestGetEnv_InvalidValues(t *testing.T) {
	t.Run("invalid int falls back to default", func(t *testing.T) {
		setTestEnv(t, map[string]string{"TEST_INT": "not-a-number"})
		assert.Equal(t, 42, getEnvInt("TEST_INT", 42))
	})

	t.Run("invalid bool falls back to default", func(t *testing.T) {
		setTestEnv(t, map[string]string{"TEST_BOOL": "not-a-bool"})
		assert.True(t, getEnvBool("TEST_BOOL", true))
	})

	t.Run("invalid duration falls back to default", func(t *testing.T) {
		setTestEnv(t, map[string]string{"TEST_DUR": "not-a-duration"})
		assert.Equal(t, 5*time.Second, getEnvDuration("TEST_DUR", 5*time.Second))
	})
}
