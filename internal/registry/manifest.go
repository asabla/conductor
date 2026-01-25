package registry

import (
	"fmt"
	"io"
	"strings"

	"gopkg.in/yaml.v3"
)

// Manifest represents the .testharness.yaml configuration file.
type Manifest struct {
	Version   string            `yaml:"version"`
	Service   ServiceConfig     `yaml:"service"`
	Defaults  DefaultConfig     `yaml:"defaults,omitempty"`
	Tests     []TestDefinition  `yaml:"tests"`
	Hooks     *HooksConfig      `yaml:"hooks,omitempty"`
	Variables map[string]string `yaml:"variables,omitempty"`
}

// ServiceConfig contains service-level configuration.
type ServiceConfig struct {
	Name        string   `yaml:"name"`
	Description string   `yaml:"description,omitempty"`
	Owner       string   `yaml:"owner,omitempty"`
	Contact     *Contact `yaml:"contact,omitempty"`
}

// Contact contains contact information for a service owner.
type Contact struct {
	Email string `yaml:"email,omitempty"`
	Slack string `yaml:"slack,omitempty"`
}

// DefaultConfig contains default values applied to all tests.
type DefaultConfig struct {
	ExecutionType    string            `yaml:"execution_type,omitempty"`
	TimeoutSeconds   int               `yaml:"timeout_seconds,omitempty"`
	Retries          int               `yaml:"retries,omitempty"`
	ContainerImage   string            `yaml:"container_image,omitempty"`
	WorkingDirectory string            `yaml:"working_directory,omitempty"`
	Environment      map[string]string `yaml:"environment,omitempty"`
}

// TestDefinition defines a single test or test suite.
type TestDefinition struct {
	Name             string            `yaml:"name"`
	Description      string            `yaml:"description,omitempty"`
	ExecutionType    string            `yaml:"execution_type,omitempty"` // subprocess, container
	Command          string            `yaml:"command"`
	Args             []string          `yaml:"args,omitempty"`
	TimeoutSeconds   int               `yaml:"timeout_seconds,omitempty"`
	ResultFile       string            `yaml:"result_file,omitempty"`
	ResultFormat     string            `yaml:"result_format,omitempty"` // junit, jest, playwright, go_test, tap, json
	ArtifactPatterns []string          `yaml:"artifact_patterns,omitempty"`
	Tags             []string          `yaml:"tags,omitempty"`
	DependsOn        []string          `yaml:"depends_on,omitempty"`
	Retries          int               `yaml:"retries,omitempty"`
	AllowFailure     bool              `yaml:"allow_failure,omitempty"`
	ContainerImage   string            `yaml:"container_image,omitempty"`
	WorkingDirectory string            `yaml:"working_directory,omitempty"`
	Environment      map[string]string `yaml:"environment,omitempty"`
	Setup            []string          `yaml:"setup,omitempty"`
	Teardown         []string          `yaml:"teardown,omitempty"`
}

// HooksConfig contains lifecycle hook commands.
type HooksConfig struct {
	BeforeAll  []string `yaml:"before_all,omitempty"`
	AfterAll   []string `yaml:"after_all,omitempty"`
	BeforeEach []string `yaml:"before_each,omitempty"`
	AfterEach  []string `yaml:"after_each,omitempty"`
}

// ParseManifest parses a .testharness.yaml manifest from a reader.
func ParseManifest(r io.Reader) (*Manifest, error) {
	var manifest Manifest

	decoder := yaml.NewDecoder(r)
	decoder.KnownFields(true) // Error on unknown fields

	if err := decoder.Decode(&manifest); err != nil {
		return nil, fmt.Errorf("failed to decode manifest: %w", err)
	}

	// Apply defaults to tests
	applyDefaults(&manifest)

	return &manifest, nil
}

// ValidateManifest validates a parsed manifest.
func ValidateManifest(m *Manifest) error {
	var errors []string

	// Validate version
	if m.Version == "" {
		errors = append(errors, "version is required")
	} else if !isValidVersion(m.Version) {
		errors = append(errors, fmt.Sprintf("unsupported version: %s (supported: 1, 1.0)", m.Version))
	}

	// Validate service config
	if m.Service.Name == "" {
		errors = append(errors, "service.name is required")
	}

	// Validate tests
	if len(m.Tests) == 0 {
		errors = append(errors, "at least one test is required")
	}

	testNames := make(map[string]bool)
	for i, test := range m.Tests {
		prefix := fmt.Sprintf("tests[%d]", i)

		if test.Name == "" {
			errors = append(errors, fmt.Sprintf("%s.name is required", prefix))
		} else if testNames[test.Name] {
			errors = append(errors, fmt.Sprintf("%s.name '%s' is duplicated", prefix, test.Name))
		}
		testNames[test.Name] = true

		if test.Command == "" {
			errors = append(errors, fmt.Sprintf("%s.command is required", prefix))
		}

		if test.ExecutionType != "" && !isValidExecutionType(test.ExecutionType) {
			errors = append(errors, fmt.Sprintf("%s.execution_type must be 'subprocess' or 'container', got '%s'", prefix, test.ExecutionType))
		}

		if test.ResultFormat != "" && !isValidResultFormat(test.ResultFormat) {
			errors = append(errors, fmt.Sprintf("%s.result_format must be one of: junit, jest, playwright, go_test, tap, json; got '%s'", prefix, test.ResultFormat))
		}

		if test.TimeoutSeconds < 0 {
			errors = append(errors, fmt.Sprintf("%s.timeout_seconds cannot be negative", prefix))
		}

		if test.Retries < 0 {
			errors = append(errors, fmt.Sprintf("%s.retries cannot be negative", prefix))
		}

		// Validate dependencies exist
		for _, dep := range test.DependsOn {
			if !testNames[dep] && !containsTestNamed(m.Tests, dep) {
				errors = append(errors, fmt.Sprintf("%s.depends_on references unknown test '%s'", prefix, dep))
			}
		}
	}

	// Check for circular dependencies
	if err := checkCircularDependencies(m.Tests); err != nil {
		errors = append(errors, err.Error())
	}

	if len(errors) > 0 {
		return &ValidationError{Errors: errors}
	}

	return nil
}

// ValidationError contains multiple validation errors.
type ValidationError struct {
	Errors []string
}

func (e *ValidationError) Error() string {
	if len(e.Errors) == 1 {
		return e.Errors[0]
	}
	return fmt.Sprintf("%d validation errors: %s", len(e.Errors), strings.Join(e.Errors, "; "))
}

// applyDefaults applies default configuration to test definitions.
func applyDefaults(m *Manifest) {
	for i := range m.Tests {
		test := &m.Tests[i]

		// Apply execution type default
		if test.ExecutionType == "" {
			if m.Defaults.ExecutionType != "" {
				test.ExecutionType = m.Defaults.ExecutionType
			} else {
				test.ExecutionType = "subprocess"
			}
		}

		// Apply timeout default
		if test.TimeoutSeconds == 0 {
			if m.Defaults.TimeoutSeconds > 0 {
				test.TimeoutSeconds = m.Defaults.TimeoutSeconds
			} else {
				test.TimeoutSeconds = 1800 // 30 minutes default
			}
		}

		// Apply retries default
		if test.Retries == 0 && m.Defaults.Retries > 0 {
			test.Retries = m.Defaults.Retries
		}

		// Apply container image default
		if test.ContainerImage == "" && m.Defaults.ContainerImage != "" {
			test.ContainerImage = m.Defaults.ContainerImage
		}

		// Apply working directory default
		if test.WorkingDirectory == "" && m.Defaults.WorkingDirectory != "" {
			test.WorkingDirectory = m.Defaults.WorkingDirectory
		}

		// Merge environment variables (test overrides defaults)
		if len(m.Defaults.Environment) > 0 {
			if test.Environment == nil {
				test.Environment = make(map[string]string)
			}
			for k, v := range m.Defaults.Environment {
				if _, exists := test.Environment[k]; !exists {
					test.Environment[k] = v
				}
			}
		}
	}
}

// isValidVersion checks if the manifest version is supported.
func isValidVersion(v string) bool {
	switch v {
	case "1", "1.0":
		return true
	default:
		return false
	}
}

// isValidExecutionType checks if the execution type is valid.
func isValidExecutionType(t string) bool {
	switch t {
	case "subprocess", "container":
		return true
	default:
		return false
	}
}

// isValidResultFormat checks if the result format is valid.
func isValidResultFormat(f string) bool {
	switch f {
	case "junit", "jest", "playwright", "go_test", "tap", "json":
		return true
	default:
		return false
	}
}

// containsTestNamed checks if a test with the given name exists in the list.
func containsTestNamed(tests []TestDefinition, name string) bool {
	for _, t := range tests {
		if t.Name == name {
			return true
		}
	}
	return false
}

// checkCircularDependencies detects circular dependencies in test definitions.
func checkCircularDependencies(tests []TestDefinition) error {
	// Build adjacency list
	deps := make(map[string][]string)
	for _, t := range tests {
		deps[t.Name] = t.DependsOn
	}

	// Track visited nodes for cycle detection
	visited := make(map[string]int) // 0 = unvisited, 1 = visiting, 2 = visited

	var visit func(name string, path []string) error
	visit = func(name string, path []string) error {
		switch visited[name] {
		case 1:
			// Currently visiting - cycle detected
			cycle := append(path, name)
			return fmt.Errorf("circular dependency detected: %s", strings.Join(cycle, " -> "))
		case 2:
			// Already visited and validated
			return nil
		}

		visited[name] = 1
		path = append(path, name)

		for _, dep := range deps[name] {
			if err := visit(dep, path); err != nil {
				return err
			}
		}

		visited[name] = 2
		return nil
	}

	for _, t := range tests {
		if visited[t.Name] == 0 {
			if err := visit(t.Name, nil); err != nil {
				return err
			}
		}
	}

	return nil
}

// ParseManifestBytes parses a manifest from bytes.
func ParseManifestBytes(data []byte) (*Manifest, error) {
	return ParseManifest(strings.NewReader(string(data)))
}

// ExampleManifest returns an example manifest for documentation.
func ExampleManifest() string {
	return `version: "1"

service:
  name: my-service
  description: My microservice
  owner: platform-team
  contact:
    email: platform@example.com
    slack: "#platform-alerts"

defaults:
  execution_type: subprocess
  timeout_seconds: 300
  retries: 2
  environment:
    CI: "true"

tests:
  - name: unit-tests
    description: Run unit tests
    command: go
    args: ["test", "-v", "./..."]
    result_format: go_test
    tags: ["unit", "fast"]

  - name: integration-tests
    description: Run integration tests
    command: go
    args: ["test", "-v", "-tags=integration", "./..."]
    result_format: go_test
    timeout_seconds: 600
    tags: ["integration", "slow"]
    depends_on: ["unit-tests"]
    environment:
      DATABASE_URL: "postgres://test:test@localhost:5432/test"

  - name: e2e-tests
    description: Run end-to-end tests
    execution_type: container
    container_image: playwright:latest
    command: npx
    args: ["playwright", "test"]
    result_format: playwright
    timeout_seconds: 1800
    artifact_patterns:
      - "test-results/**"
      - "playwright-report/**"
    tags: ["e2e", "slow"]
    depends_on: ["integration-tests"]

hooks:
  before_all:
    - make deps
  after_all:
    - make clean
`
}
