package agent

import (
	"os"
	"testing"
	"time"
)

// Helper to set and restore environment variables
func setEnv(t *testing.T, key, value string) func() {
	t.Helper()
	old, existed := os.LookupEnv(key)
	os.Setenv(key, value)
	return func() {
		if existed {
			os.Setenv(key, old)
		} else {
			os.Unsetenv(key)
		}
	}
}

// clearEnvs clears all CONDUCTOR_AGENT_ environment variables
func clearEnvs(t *testing.T) func() {
	t.Helper()
	saved := make(map[string]string)
	for _, env := range os.Environ() {
		for i := 0; i < len(env); i++ {
			if env[i] == '=' {
				key := env[:i]
				if len(key) > 16 && key[:16] == "CONDUCTOR_AGENT_" {
					saved[key] = env[i+1:]
					os.Unsetenv(key)
				}
				break
			}
		}
	}
	return func() {
		for key, val := range saved {
			os.Setenv(key, val)
		}
	}
}

func TestConfig_Validate_Required(t *testing.T) {
	tests := []struct {
		name     string
		config   Config
		wantErrs []string
	}{
		{
			name:   "missing all required fields",
			config: Config{},
			wantErrs: []string{
				"CONDUCTOR_AGENT_ID is required",
				"CONDUCTOR_AGENT_CONTROL_PLANE_URL is required",
				"CONDUCTOR_AGENT_TOKEN is required",
			},
		},
		{
			name: "missing control plane URL",
			config: Config{
				AgentID:              "test-agent",
				AgentToken:           "token",
				MaxParallel:          4,
				WorkspaceDir:         "/tmp/workspaces",
				CacheDir:             "/tmp/cache",
				StateDir:             "/var/lib/conductor",
				HeartbeatInterval:    30 * time.Second,
				ReconnectMinInterval: 1 * time.Second,
				ReconnectMaxInterval: 60 * time.Second,
				DefaultTimeout:       30 * time.Minute,
				LogLevel:             "info",
				LogFormat:            "json",
				CPUThreshold:         90,
				MemoryThreshold:      90,
				DiskThreshold:        90,
			},
			wantErrs: []string{"CONDUCTOR_AGENT_CONTROL_PLANE_URL is required"},
		},
		{
			name: "all required fields present",
			config: Config{
				AgentID:              "test-agent",
				AgentToken:           "token",
				ControlPlaneURL:      "localhost:50051",
				MaxParallel:          4,
				WorkspaceDir:         "/tmp/workspaces",
				CacheDir:             "/tmp/cache",
				StateDir:             "/var/lib/conductor",
				HeartbeatInterval:    30 * time.Second,
				ReconnectMinInterval: 1 * time.Second,
				ReconnectMaxInterval: 60 * time.Second,
				DefaultTimeout:       30 * time.Minute,
				LogLevel:             "info",
				LogFormat:            "json",
				CPUThreshold:         90,
				MemoryThreshold:      90,
				DiskThreshold:        90,
			},
			wantErrs: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if tt.wantErrs == nil {
				if err != nil {
					t.Errorf("Validate() error = %v, want nil", err)
				}
				return
			}

			if err == nil {
				t.Errorf("Validate() error = nil, want errors containing %v", tt.wantErrs)
				return
			}

			errStr := err.Error()
			for _, want := range tt.wantErrs {
				if !containsSubstring(errStr, want) {
					t.Errorf("Validate() error = %q, want to contain %q", errStr, want)
				}
			}
		})
	}
}

func TestConfig_Validate_MaxParallel(t *testing.T) {
	baseConfig := func() Config {
		return Config{
			AgentID:              "test-agent",
			AgentToken:           "token",
			ControlPlaneURL:      "localhost:50051",
			WorkspaceDir:         "/tmp/workspaces",
			CacheDir:             "/tmp/cache",
			StateDir:             "/var/lib/conductor",
			HeartbeatInterval:    30 * time.Second,
			ReconnectMinInterval: 1 * time.Second,
			ReconnectMaxInterval: 60 * time.Second,
			DefaultTimeout:       30 * time.Minute,
			LogLevel:             "info",
			LogFormat:            "json",
			CPUThreshold:         90,
			MemoryThreshold:      90,
			DiskThreshold:        90,
		}
	}

	tests := []struct {
		name        string
		maxParallel int
		wantErr     bool
		errContains string
	}{
		{"zero", 0, true, "must be at least 1"},
		{"negative", -1, true, "must be at least 1"},
		{"one", 1, false, ""},
		{"fifty", 50, false, ""},
		{"hundred", 100, false, ""},
		{"over hundred", 101, true, "cannot exceed 100"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := baseConfig()
			cfg.MaxParallel = tt.maxParallel

			err := cfg.Validate()
			if tt.wantErr {
				if err == nil {
					t.Errorf("Validate() error = nil, want error containing %q", tt.errContains)
					return
				}
				if !containsSubstring(err.Error(), tt.errContains) {
					t.Errorf("Validate() error = %q, want to contain %q", err.Error(), tt.errContains)
				}
			} else if err != nil {
				t.Errorf("Validate() error = %v, want nil", err)
			}
		})
	}
}

func TestConfig_Validate_Intervals(t *testing.T) {
	baseConfig := func() Config {
		return Config{
			AgentID:         "test-agent",
			AgentToken:      "token",
			ControlPlaneURL: "localhost:50051",
			MaxParallel:     4,
			WorkspaceDir:    "/tmp/workspaces",
			CacheDir:        "/tmp/cache",
			StateDir:        "/var/lib/conductor",
			LogLevel:        "info",
			LogFormat:       "json",
			CPUThreshold:    90,
			MemoryThreshold: 90,
			DiskThreshold:   90,
		}
	}

	t.Run("heartbeat interval too short", func(t *testing.T) {
		cfg := baseConfig()
		cfg.HeartbeatInterval = 1 * time.Second
		cfg.ReconnectMinInterval = 1 * time.Second
		cfg.ReconnectMaxInterval = 60 * time.Second
		cfg.DefaultTimeout = 30 * time.Minute

		err := cfg.Validate()
		if err == nil || !containsSubstring(err.Error(), "HEARTBEAT_INTERVAL must be at least 5 seconds") {
			t.Errorf("expected heartbeat interval error, got %v", err)
		}
	})

	t.Run("reconnect min interval too short", func(t *testing.T) {
		cfg := baseConfig()
		cfg.HeartbeatInterval = 30 * time.Second
		cfg.ReconnectMinInterval = 10 * time.Millisecond
		cfg.ReconnectMaxInterval = 60 * time.Second
		cfg.DefaultTimeout = 30 * time.Minute

		err := cfg.Validate()
		if err == nil || !containsSubstring(err.Error(), "RECONNECT_MIN_INTERVAL must be at least 100ms") {
			t.Errorf("expected reconnect min interval error, got %v", err)
		}
	})

	t.Run("reconnect max less than min", func(t *testing.T) {
		cfg := baseConfig()
		cfg.HeartbeatInterval = 30 * time.Second
		cfg.ReconnectMinInterval = 10 * time.Second
		cfg.ReconnectMaxInterval = 5 * time.Second
		cfg.DefaultTimeout = 30 * time.Minute

		err := cfg.Validate()
		if err == nil || !containsSubstring(err.Error(), "RECONNECT_MAX_INTERVAL must be >= MIN_INTERVAL") {
			t.Errorf("expected reconnect max interval error, got %v", err)
		}
	})

	t.Run("default timeout too short", func(t *testing.T) {
		cfg := baseConfig()
		cfg.HeartbeatInterval = 30 * time.Second
		cfg.ReconnectMinInterval = 1 * time.Second
		cfg.ReconnectMaxInterval = 60 * time.Second
		cfg.DefaultTimeout = 30 * time.Second

		err := cfg.Validate()
		if err == nil || !containsSubstring(err.Error(), "DEFAULT_TIMEOUT must be at least 1 minute") {
			t.Errorf("expected default timeout error, got %v", err)
		}
	})
}

func TestConfig_Validate_LogSettings(t *testing.T) {
	baseConfig := func() Config {
		return Config{
			AgentID:              "test-agent",
			AgentToken:           "token",
			ControlPlaneURL:      "localhost:50051",
			MaxParallel:          4,
			WorkspaceDir:         "/tmp/workspaces",
			CacheDir:             "/tmp/cache",
			StateDir:             "/var/lib/conductor",
			HeartbeatInterval:    30 * time.Second,
			ReconnectMinInterval: 1 * time.Second,
			ReconnectMaxInterval: 60 * time.Second,
			DefaultTimeout:       30 * time.Minute,
			CPUThreshold:         90,
			MemoryThreshold:      90,
			DiskThreshold:        90,
		}
	}

	t.Run("invalid log level", func(t *testing.T) {
		cfg := baseConfig()
		cfg.LogLevel = "invalid"
		cfg.LogFormat = "json"

		err := cfg.Validate()
		if err == nil || !containsSubstring(err.Error(), "LOG_LEVEL must be one of") {
			t.Errorf("expected log level error, got %v", err)
		}
	})

	t.Run("invalid log format", func(t *testing.T) {
		cfg := baseConfig()
		cfg.LogLevel = "info"
		cfg.LogFormat = "xml"

		err := cfg.Validate()
		if err == nil || !containsSubstring(err.Error(), "LOG_FORMAT must be one of") {
			t.Errorf("expected log format error, got %v", err)
		}
	})

	t.Run("valid log settings", func(t *testing.T) {
		for _, level := range []string{"debug", "info", "warn", "error"} {
			for _, format := range []string{"json", "console"} {
				cfg := baseConfig()
				cfg.LogLevel = level
				cfg.LogFormat = format

				if err := cfg.Validate(); err != nil {
					t.Errorf("Validate() with level=%s, format=%s: error = %v, want nil", level, format, err)
				}
			}
		}
	})
}

func TestConfig_Validate_TLS(t *testing.T) {
	baseConfig := func() Config {
		return Config{
			AgentID:              "test-agent",
			AgentToken:           "token",
			ControlPlaneURL:      "localhost:50051",
			MaxParallel:          4,
			WorkspaceDir:         "/tmp/workspaces",
			CacheDir:             "/tmp/cache",
			StateDir:             "/var/lib/conductor",
			HeartbeatInterval:    30 * time.Second,
			ReconnectMinInterval: 1 * time.Second,
			ReconnectMaxInterval: 60 * time.Second,
			DefaultTimeout:       30 * time.Minute,
			LogLevel:             "info",
			LogFormat:            "json",
			CPUThreshold:         90,
			MemoryThreshold:      90,
			DiskThreshold:        90,
		}
	}

	t.Run("TLS enabled without cert", func(t *testing.T) {
		cfg := baseConfig()
		cfg.TLSEnabled = true
		cfg.TLSKeyFile = "/path/to/key"

		err := cfg.Validate()
		if err == nil || !containsSubstring(err.Error(), "TLS_CERT_FILE is required") {
			t.Errorf("expected TLS cert file error, got %v", err)
		}
	})

	t.Run("TLS enabled without key", func(t *testing.T) {
		cfg := baseConfig()
		cfg.TLSEnabled = true
		cfg.TLSCertFile = "/path/to/cert"

		err := cfg.Validate()
		if err == nil || !containsSubstring(err.Error(), "TLS_KEY_FILE is required") {
			t.Errorf("expected TLS key file error, got %v", err)
		}
	})

	t.Run("TLS enabled with both files", func(t *testing.T) {
		cfg := baseConfig()
		cfg.TLSEnabled = true
		cfg.TLSCertFile = "/path/to/cert"
		cfg.TLSKeyFile = "/path/to/key"

		if err := cfg.Validate(); err != nil {
			t.Errorf("Validate() error = %v, want nil", err)
		}
	})

	t.Run("TLS disabled without files", func(t *testing.T) {
		cfg := baseConfig()
		cfg.TLSEnabled = false

		if err := cfg.Validate(); err != nil {
			t.Errorf("Validate() error = %v, want nil", err)
		}
	})
}

func TestConfig_Validate_ResourceThresholds(t *testing.T) {
	baseConfig := func() Config {
		return Config{
			AgentID:              "test-agent",
			AgentToken:           "token",
			ControlPlaneURL:      "localhost:50051",
			MaxParallel:          4,
			WorkspaceDir:         "/tmp/workspaces",
			CacheDir:             "/tmp/cache",
			StateDir:             "/var/lib/conductor",
			HeartbeatInterval:    30 * time.Second,
			ReconnectMinInterval: 1 * time.Second,
			ReconnectMaxInterval: 60 * time.Second,
			DefaultTimeout:       30 * time.Minute,
			LogLevel:             "info",
			LogFormat:            "json",
		}
	}

	tests := []struct {
		name        string
		cpu         float64
		memory      float64
		disk        float64
		wantErr     bool
		errContains string
	}{
		{"all valid", 90, 90, 90, false, ""},
		{"cpu zero", 0, 90, 90, true, "CPU_THRESHOLD must be between 0 and 100"},
		{"cpu negative", -10, 90, 90, true, "CPU_THRESHOLD must be between 0 and 100"},
		{"cpu over 100", 101, 90, 90, true, "CPU_THRESHOLD must be between 0 and 100"},
		{"memory zero", 90, 0, 90, true, "MEMORY_THRESHOLD must be between 0 and 100"},
		{"memory over 100", 90, 101, 90, true, "MEMORY_THRESHOLD must be between 0 and 100"},
		{"disk zero", 90, 90, 0, true, "DISK_THRESHOLD must be between 0 and 100"},
		{"disk over 100", 90, 90, 101, true, "DISK_THRESHOLD must be between 0 and 100"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := baseConfig()
			cfg.CPUThreshold = tt.cpu
			cfg.MemoryThreshold = tt.memory
			cfg.DiskThreshold = tt.disk

			err := cfg.Validate()
			if tt.wantErr {
				if err == nil || !containsSubstring(err.Error(), tt.errContains) {
					t.Errorf("Validate() error = %v, want error containing %q", err, tt.errContains)
				}
			} else if err != nil {
				t.Errorf("Validate() error = %v, want nil", err)
			}
		})
	}
}

func TestLoad_FromEnvironment(t *testing.T) {
	cleanup := clearEnvs(t)
	defer cleanup()

	// Set required environment variables
	os.Setenv("CONDUCTOR_AGENT_ID", "test-agent-123")
	os.Setenv("CONDUCTOR_AGENT_CONTROL_PLANE_URL", "localhost:50051")
	os.Setenv("CONDUCTOR_AGENT_TOKEN", "secret-token")
	os.Setenv("CONDUCTOR_AGENT_MAX_PARALLEL", "8")
	os.Setenv("CONDUCTOR_AGENT_NETWORK_ZONES", "zone1,zone2,zone3")
	os.Setenv("CONDUCTOR_AGENT_RUNTIMES", "node18, python3.11, go1.21")
	os.Setenv("CONDUCTOR_AGENT_LABELS", "env=prod,region=us-east-1")
	os.Setenv("CONDUCTOR_AGENT_HEARTBEAT_INTERVAL", "45s")
	os.Setenv("CONDUCTOR_AGENT_LOG_LEVEL", "debug")
	os.Setenv("CONDUCTOR_AGENT_DOCKER_ENABLED", "false")
	os.Setenv("CONDUCTOR_AGENT_CPU_THRESHOLD", "85.5")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	// Verify values
	if cfg.AgentID != "test-agent-123" {
		t.Errorf("AgentID = %q, want %q", cfg.AgentID, "test-agent-123")
	}
	if cfg.ControlPlaneURL != "localhost:50051" {
		t.Errorf("ControlPlaneURL = %q, want %q", cfg.ControlPlaneURL, "localhost:50051")
	}
	if cfg.AgentToken != "secret-token" {
		t.Errorf("AgentToken = %q, want %q", cfg.AgentToken, "secret-token")
	}
	if cfg.MaxParallel != 8 {
		t.Errorf("MaxParallel = %d, want %d", cfg.MaxParallel, 8)
	}

	// Check network zones
	expectedZones := []string{"zone1", "zone2", "zone3"}
	if len(cfg.NetworkZones) != len(expectedZones) {
		t.Errorf("NetworkZones = %v, want %v", cfg.NetworkZones, expectedZones)
	}
	for i, zone := range cfg.NetworkZones {
		if zone != expectedZones[i] {
			t.Errorf("NetworkZones[%d] = %q, want %q", i, zone, expectedZones[i])
		}
	}

	// Check runtimes (trimmed)
	expectedRuntimes := []string{"node18", "python3.11", "go1.21"}
	if len(cfg.Runtimes) != len(expectedRuntimes) {
		t.Errorf("Runtimes = %v, want %v", cfg.Runtimes, expectedRuntimes)
	}

	// Check labels
	if cfg.Labels["env"] != "prod" {
		t.Errorf("Labels[env] = %q, want %q", cfg.Labels["env"], "prod")
	}
	if cfg.Labels["region"] != "us-east-1" {
		t.Errorf("Labels[region] = %q, want %q", cfg.Labels["region"], "us-east-1")
	}

	// Check parsed values
	if cfg.HeartbeatInterval != 45*time.Second {
		t.Errorf("HeartbeatInterval = %v, want %v", cfg.HeartbeatInterval, 45*time.Second)
	}
	if cfg.LogLevel != "debug" {
		t.Errorf("LogLevel = %q, want %q", cfg.LogLevel, "debug")
	}
	if cfg.DockerEnabled != false {
		t.Errorf("DockerEnabled = %v, want %v", cfg.DockerEnabled, false)
	}
	if cfg.CPUThreshold != 85.5 {
		t.Errorf("CPUThreshold = %v, want %v", cfg.CPUThreshold, 85.5)
	}
}

func TestLoad_Defaults(t *testing.T) {
	cleanup := clearEnvs(t)
	defer cleanup()

	// Set only required environment variables
	os.Setenv("CONDUCTOR_AGENT_CONTROL_PLANE_URL", "localhost:50051")
	os.Setenv("CONDUCTOR_AGENT_TOKEN", "secret-token")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	// Check defaults
	if cfg.MaxParallel != 4 {
		t.Errorf("MaxParallel = %d, want default %d", cfg.MaxParallel, 4)
	}
	if cfg.WorkspaceDir != "/tmp/conductor/workspaces" {
		t.Errorf("WorkspaceDir = %q, want default", cfg.WorkspaceDir)
	}
	if cfg.CacheDir != "/tmp/conductor/cache" {
		t.Errorf("CacheDir = %q, want default", cfg.CacheDir)
	}
	if cfg.HeartbeatInterval != 30*time.Second {
		t.Errorf("HeartbeatInterval = %v, want default %v", cfg.HeartbeatInterval, 30*time.Second)
	}
	if cfg.ReconnectMinInterval != 1*time.Second {
		t.Errorf("ReconnectMinInterval = %v, want default %v", cfg.ReconnectMinInterval, 1*time.Second)
	}
	if cfg.ReconnectMaxInterval != 60*time.Second {
		t.Errorf("ReconnectMaxInterval = %v, want default %v", cfg.ReconnectMaxInterval, 60*time.Second)
	}
	if cfg.DefaultTimeout != 30*time.Minute {
		t.Errorf("DefaultTimeout = %v, want default %v", cfg.DefaultTimeout, 30*time.Minute)
	}
	if cfg.LogLevel != "info" {
		t.Errorf("LogLevel = %q, want default %q", cfg.LogLevel, "info")
	}
	if cfg.LogFormat != "json" {
		t.Errorf("LogFormat = %q, want default %q", cfg.LogFormat, "json")
	}
	if cfg.DockerEnabled != true {
		t.Errorf("DockerEnabled = %v, want default %v", cfg.DockerEnabled, true)
	}
	if cfg.CPUThreshold != 90.0 {
		t.Errorf("CPUThreshold = %v, want default %v", cfg.CPUThreshold, 90.0)
	}
	if cfg.MemoryThreshold != 90.0 {
		t.Errorf("MemoryThreshold = %v, want default %v", cfg.MemoryThreshold, 90.0)
	}
	if cfg.StorageRegion != "us-east-1" {
		t.Errorf("StorageRegion = %q, want default %q", cfg.StorageRegion, "us-east-1")
	}
	if cfg.StorageUseSSL != true {
		t.Errorf("StorageUseSSL = %v, want default %v", cfg.StorageUseSSL, true)
	}
}

func TestLoad_ValidationFailure(t *testing.T) {
	cleanup := clearEnvs(t)
	defer cleanup()

	// Set invalid configuration (missing required fields)
	os.Setenv("CONDUCTOR_AGENT_MAX_PARALLEL", "200") // Also invalid

	_, err := Load()
	if err == nil {
		t.Error("Load() error = nil, want validation error")
	}
}

func TestValidationError(t *testing.T) {
	t.Run("single error", func(t *testing.T) {
		ve := &ValidationError{
			Errors: []error{
				&simpleError{msg: "field X is required"},
			},
		}

		if ve.Error() != "field X is required" {
			t.Errorf("Error() = %q, want single error message", ve.Error())
		}
	})

	t.Run("multiple errors", func(t *testing.T) {
		ve := &ValidationError{
			Errors: []error{
				&simpleError{msg: "field X is required"},
				&simpleError{msg: "field Y is invalid"},
			},
		}

		errStr := ve.Error()
		if !containsSubstring(errStr, "2 validation errors") {
			t.Errorf("Error() = %q, want to contain '2 validation errors'", errStr)
		}
		if !containsSubstring(errStr, "field X is required") {
			t.Errorf("Error() = %q, want to contain 'field X is required'", errStr)
		}
		if !containsSubstring(errStr, "field Y is invalid") {
			t.Errorf("Error() = %q, want to contain 'field Y is invalid'", errStr)
		}
	})

	t.Run("unwrap", func(t *testing.T) {
		err1 := &simpleError{msg: "error 1"}
		err2 := &simpleError{msg: "error 2"}
		ve := &ValidationError{
			Errors: []error{err1, err2},
		}

		unwrapped := ve.Unwrap()
		if len(unwrapped) != 2 {
			t.Errorf("Unwrap() returned %d errors, want 2", len(unwrapped))
		}
	})
}

func TestGetEnvHelpers(t *testing.T) {
	t.Run("getEnv", func(t *testing.T) {
		cleanup := setEnv(t, "TEST_ENV_VAR", "test-value")
		defer cleanup()

		if got := getEnv("TEST_ENV_VAR", "default"); got != "test-value" {
			t.Errorf("getEnv() = %q, want %q", got, "test-value")
		}
		if got := getEnv("NONEXISTENT_VAR", "default"); got != "default" {
			t.Errorf("getEnv() = %q, want %q", got, "default")
		}
	})

	t.Run("getEnvInt", func(t *testing.T) {
		cleanup := setEnv(t, "TEST_INT_VAR", "42")
		defer cleanup()

		if got := getEnvInt("TEST_INT_VAR", 0); got != 42 {
			t.Errorf("getEnvInt() = %d, want %d", got, 42)
		}
		if got := getEnvInt("NONEXISTENT_VAR", 10); got != 10 {
			t.Errorf("getEnvInt() = %d, want %d", got, 10)
		}

		// Invalid int
		os.Setenv("TEST_INT_VAR", "not-an-int")
		if got := getEnvInt("TEST_INT_VAR", 99); got != 99 {
			t.Errorf("getEnvInt() with invalid value = %d, want default %d", got, 99)
		}
	})

	t.Run("getEnvBool", func(t *testing.T) {
		cleanup := setEnv(t, "TEST_BOOL_VAR", "true")
		defer cleanup()

		if got := getEnvBool("TEST_BOOL_VAR", false); got != true {
			t.Errorf("getEnvBool() = %v, want %v", got, true)
		}

		os.Setenv("TEST_BOOL_VAR", "false")
		if got := getEnvBool("TEST_BOOL_VAR", true); got != false {
			t.Errorf("getEnvBool() = %v, want %v", got, false)
		}

		if got := getEnvBool("NONEXISTENT_VAR", true); got != true {
			t.Errorf("getEnvBool() = %v, want default %v", got, true)
		}
	})

	t.Run("getEnvFloat64", func(t *testing.T) {
		cleanup := setEnv(t, "TEST_FLOAT_VAR", "3.14")
		defer cleanup()

		if got := getEnvFloat64("TEST_FLOAT_VAR", 0); got != 3.14 {
			t.Errorf("getEnvFloat64() = %v, want %v", got, 3.14)
		}
		if got := getEnvFloat64("NONEXISTENT_VAR", 2.71); got != 2.71 {
			t.Errorf("getEnvFloat64() = %v, want default %v", got, 2.71)
		}
	})

	t.Run("getEnvDuration", func(t *testing.T) {
		cleanup := setEnv(t, "TEST_DUR_VAR", "5m30s")
		defer cleanup()

		expected := 5*time.Minute + 30*time.Second
		if got := getEnvDuration("TEST_DUR_VAR", 0); got != expected {
			t.Errorf("getEnvDuration() = %v, want %v", got, expected)
		}

		defaultDur := 10 * time.Second
		if got := getEnvDuration("NONEXISTENT_VAR", defaultDur); got != defaultDur {
			t.Errorf("getEnvDuration() = %v, want default %v", got, defaultDur)
		}
	})

	t.Run("getEnvStringSlice", func(t *testing.T) {
		cleanup := setEnv(t, "TEST_SLICE_VAR", "a, b ,c, d")
		defer cleanup()

		got := getEnvStringSlice("TEST_SLICE_VAR", nil)
		expected := []string{"a", "b", "c", "d"}
		if len(got) != len(expected) {
			t.Errorf("getEnvStringSlice() = %v, want %v", got, expected)
		}
		for i, v := range got {
			if v != expected[i] {
				t.Errorf("getEnvStringSlice()[%d] = %q, want %q", i, v, expected[i])
			}
		}

		// Empty after trimming
		os.Setenv("TEST_SLICE_VAR", "  ,  ,  ")
		got = getEnvStringSlice("TEST_SLICE_VAR", []string{"default"})
		if len(got) != 1 || got[0] != "default" {
			t.Errorf("getEnvStringSlice() with empty values = %v, want default", got)
		}
	})

	t.Run("getEnvMap", func(t *testing.T) {
		cleanup := setEnv(t, "TEST_MAP_VAR", "key1=value1,key2=value2, key3 = value3 ")
		defer cleanup()

		got := getEnvMap("TEST_MAP_VAR")
		if got["key1"] != "value1" {
			t.Errorf("getEnvMap()[key1] = %q, want %q", got["key1"], "value1")
		}
		if got["key2"] != "value2" {
			t.Errorf("getEnvMap()[key2] = %q, want %q", got["key2"], "value2")
		}
		if got["key3"] != "value3" {
			t.Errorf("getEnvMap()[key3] = %q, want %q", got["key3"], "value3")
		}

		// Invalid format
		os.Setenv("TEST_MAP_VAR", "invalid,also-invalid")
		got = getEnvMap("TEST_MAP_VAR")
		if len(got) != 0 {
			t.Errorf("getEnvMap() with invalid format = %v, want empty map", got)
		}
	})
}

// Helper types and functions

type simpleError struct {
	msg string
}

func (e *simpleError) Error() string {
	return e.msg
}

func containsSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
