# Contributing to Conductor

Thank you for your interest in contributing to Conductor! This document provides guidelines and information for contributors.

## Code of Conduct

By participating in this project, you agree to abide by our Code of Conduct:

- **Be respectful** - Treat everyone with respect and consideration
- **Be constructive** - Focus on what is best for the community
- **Be collaborative** - Work together towards common goals
- **Be inclusive** - Welcome contributors of all backgrounds and experience levels

## Ways to Contribute

### Reporting Bugs

Before reporting a bug:

1. **Search existing issues** to avoid duplicates
2. **Use the latest version** to see if it's already fixed
3. **Gather information** for the bug report

When reporting, include:

- Conductor version (`conductor --version`)
- Operating system and version
- Deployment method (Docker, Kubernetes, binary)
- Steps to reproduce
- Expected behavior
- Actual behavior
- Relevant logs (with sensitive data redacted)

### Suggesting Features

We welcome feature suggestions! Please:

1. **Check existing issues and discussions** for similar ideas
2. **Create a discussion** to gauge community interest
3. **Describe the use case** - what problem does it solve?
4. **Consider the scope** - is it generally useful or very specific?

### Code Contributions

We accept contributions for:

- Bug fixes
- New features (please discuss first)
- Documentation improvements
- Test coverage improvements
- Performance optimizations

---

## Development Workflow

### 1. Fork and Clone

```bash
# Fork on GitHub, then:
git clone https://github.com/YOUR_USERNAME/conductor.git
cd conductor
git remote add upstream https://github.com/conductor/conductor.git
```

### 2. Create a Branch

```bash
# Sync with upstream
git fetch upstream
git checkout -b feature/your-feature upstream/main
```

Branch naming conventions:
- `feature/` - New features
- `fix/` - Bug fixes
- `docs/` - Documentation changes
- `refactor/` - Code refactoring
- `test/` - Test improvements

### 3. Set Up Development Environment

```bash
# Install dependencies
go mod download
cd web && npm install && cd ..

# Start infrastructure
docker-compose up -d postgres redis minio

# Run migrations
go run ./cmd/control-plane migrate up
```

See the [Development Guide](docs/development.md) for detailed setup instructions.

### 4. Make Your Changes

- Follow the [code style guidelines](#code-style)
- Write/update tests for your changes
- Update documentation as needed
- Keep commits focused and atomic

### 5. Test Your Changes

```bash
# Run all tests
go test ./...

# Run linters
golangci-lint run ./...

# Run dashboard tests
cd web && npm test
```

### 6. Commit Your Changes

Write clear commit messages:

```
Short summary (50 chars or less)

More detailed explanation if needed. Wrap at 72 characters.
Explain the problem this commit solves and why the changes
are necessary.

- Bullet points are okay
- Keep them concise

Fixes #123
```

### 7. Push and Create Pull Request

```bash
git push origin feature/your-feature
```

Then create a PR on GitHub with:
- Clear description of changes
- Link to related issues
- Screenshots for UI changes
- List of test scenarios covered

---

## Pull Request Guidelines

### PR Checklist

- [ ] Code follows style guidelines
- [ ] Tests pass locally
- [ ] New tests added for new functionality
- [ ] Documentation updated
- [ ] Commit messages are clear
- [ ] PR description explains the changes
- [ ] Related issues are linked

### PR Review Process

1. **Automated checks** - CI runs tests, lints, and builds
2. **Code review** - Maintainers review the code
3. **Feedback** - Address any requested changes
4. **Approval** - At least one maintainer approval required
5. **Merge** - Maintainer merges the PR

### Review Expectations

- Reviews typically happen within 2-3 business days
- We may ask for changes or clarification
- Be responsive to feedback
- Don't take feedback personally - we all learn from each other

---

## Code Style

### Go

Follow the [Effective Go](https://golang.org/doc/effective_go) guidelines plus:

**Naming:**
```go
// Exported (public)
func ProcessTestRun() {}
type TestRunner interface {}

// Unexported (private)
func validateInput() {}
type runConfig struct {}
```

**Imports:**
```go
import (
    // Standard library
    "context"
    "fmt"

    // External packages
    "github.com/google/uuid"
    "google.golang.org/grpc"

    // Internal packages
    "github.com/conductor/conductor/internal/scheduler"
)
```

**Error handling:**
```go
// Wrap errors with context
if err != nil {
    return fmt.Errorf("failed to process run %s: %w", runID, err)
}

// Return early
if input == nil {
    return errors.New("input cannot be nil")
}
```

**Comments:**
```go
// ProcessRun executes a test run and returns the results.
// It handles scheduling, execution, and result collection.
func ProcessRun(ctx context.Context, run *TestRun) (*Result, error) {
    // ...
}
```

### TypeScript/React

**Components:**
```typescript
// Function components with explicit types
interface Props {
  runId: string;
  onComplete?: () => void;
}

export function TestRunCard({ runId, onComplete }: Props) {
  // ...
}
```

**Hooks:**
```typescript
// Custom hooks with 'use' prefix
export function useTestRuns(serviceId: string) {
  return useQuery({
    queryKey: ['testRuns', serviceId],
    queryFn: () => api.getTestRuns(serviceId),
  });
}
```

**File naming:**
- Components: `PascalCase.tsx`
- Hooks: `useCamelCase.ts`
- Utilities: `camelCase.ts`

---

## Testing Guidelines

### Unit Tests

```go
func TestScheduler_AssignWork(t *testing.T) {
    // Arrange
    sched := NewScheduler(mockDeps)
    run := &TestRun{ID: uuid.New()}
    
    // Act
    assigned, err := sched.AssignWork(ctx, run)
    
    // Assert
    require.NoError(t, err)
    assert.True(t, assigned)
}
```

### Table-Driven Tests

```go
func TestValidateConfig(t *testing.T) {
    tests := []struct {
        name    string
        config  Config
        wantErr bool
    }{
        {
            name:    "valid config",
            config:  Config{Host: "localhost", Port: 8080},
            wantErr: false,
        },
        {
            name:    "missing host",
            config:  Config{Port: 8080},
            wantErr: true,
        },
    }
    
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            err := ValidateConfig(tt.config)
            if tt.wantErr {
                assert.Error(t, err)
            } else {
                assert.NoError(t, err)
            }
        })
    }
}
```

### Integration Tests

```go
//go:build integration

func TestDatabaseIntegration(t *testing.T) {
    db := testutil.NewTestDatabase(t)
    defer db.Cleanup()
    
    // Test database operations...
}
```

### Test Coverage

- Aim for 80%+ coverage on new code
- Focus on testing behavior, not implementation
- Include edge cases and error paths

---

## Documentation

### Code Documentation

- Document all exported types, functions, and methods
- Explain the "why" not just the "what"
- Include examples for complex functionality

### User Documentation

When adding features, update:
- Relevant guide in `docs/`
- API documentation if APIs change
- Configuration reference if config changes

### README Updates

If your change affects how users interact with Conductor:
- Update installation instructions if needed
- Add/update quick start steps
- Update feature list

---

## Issue and PR Labels

| Label | Description |
|-------|-------------|
| `bug` | Something isn't working |
| `enhancement` | New feature or request |
| `documentation` | Documentation improvements |
| `good first issue` | Good for newcomers |
| `help wanted` | Extra attention needed |
| `breaking change` | Introduces breaking changes |
| `priority: high` | High priority issue |
| `wontfix` | Won't be fixed/implemented |

---

## Release Process

Releases are managed by maintainers:

1. **Version bump** - Update version in code
2. **Changelog** - Update CHANGELOG.md
3. **Tag** - Create git tag `vX.Y.Z`
4. **Build** - CI builds and publishes artifacts
5. **Release notes** - Create GitHub release

### Versioning

We use [Semantic Versioning](https://semver.org/):

- **MAJOR** - Breaking changes
- **MINOR** - New features, backwards compatible
- **PATCH** - Bug fixes, backwards compatible

---

## Getting Help

### For Contributors

- Open a [discussion](https://github.com/conductor/conductor/discussions) for questions
- Join our [community Slack](https://conductor-community.slack.com)
- Ask in PR comments for code-specific questions

### Maintainer Contacts

- Create an issue for general inquiries
- Email security@conductor.example.com for security issues

---

## Recognition

We value all contributions! Contributors are:

- Listed in release notes for significant contributions
- Added to CONTRIBUTORS.md
- Welcomed as repeat contributors with additional trust

---

## License

By contributing, you agree that your contributions will be licensed under the same license as the project (Apache 2.0).

---

Thank you for contributing to Conductor!
