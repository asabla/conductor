// Package repo provides repository management for the agent.
package repo

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/rs/zerolog"
)

// Manager handles git repository operations with caching.
type Manager struct {
	cacheDir string
	logger   zerolog.Logger
	mu       sync.RWMutex
	cache    map[string]*cachedRepo
}

// cachedRepo represents a cached repository.
type cachedRepo struct {
	path      string
	url       string
	lastUsed  time.Time
	clonedAt  time.Time
	fetchedAt time.Time
}

// CloneOptions contains options for cloning a repository.
type CloneOptions struct {
	URL         string
	Branch      string
	CommitSHA   string
	Tag         string
	Depth       int
	Credentials *Credentials
}

// Credentials contains authentication credentials for git operations.
type Credentials struct {
	// Username for HTTPS auth (often "x-access-token" for tokens)
	Username string
	// Password or token for HTTPS auth
	Password string
	// SSHKey is the private SSH key content
	SSHKey string
	// SSHKeyPath is the path to the SSH key file
	SSHKeyPath string
}

// NewManager creates a new repository manager.
func NewManager(cacheDir string, logger zerolog.Logger) (*Manager, error) {
	if err := os.MkdirAll(cacheDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create cache directory: %w", err)
	}

	return &Manager{
		cacheDir: cacheDir,
		logger:   logger.With().Str("component", "repo_manager").Logger(),
		cache:    make(map[string]*cachedRepo),
	}, nil
}

// Clone clones or updates a repository to the target path.
func (m *Manager) Clone(ctx context.Context, opts *CloneOptions, targetPath string) error {
	if opts.URL == "" {
		return errors.New("repository URL is required")
	}

	m.logger.Debug().
		Str("url", opts.URL).
		Str("branch", opts.Branch).
		Str("commit", opts.CommitSHA).
		Str("target", targetPath).
		Msg("Cloning repository")

	// Check if we have a cached copy
	cached := m.GetCached(opts.URL)
	if cached != "" {
		// Copy from cache
		if err := m.copyFromCache(ctx, cached, targetPath); err != nil {
			m.logger.Warn().Err(err).Msg("Failed to copy from cache, cloning fresh")
		} else {
			// Checkout the specific ref
			if err := m.Checkout(ctx, targetPath, opts.Branch, opts.CommitSHA, opts.Tag); err != nil {
				return fmt.Errorf("failed to checkout: %w", err)
			}
			return nil
		}
	}

	// Fresh clone
	if err := m.cloneFresh(ctx, opts, targetPath); err != nil {
		return err
	}

	// Update cache
	m.updateCache(opts.URL, targetPath)

	return nil
}

// cloneFresh performs a fresh git clone.
func (m *Manager) cloneFresh(ctx context.Context, opts *CloneOptions, targetPath string) error {
	// Build clone command
	args := []string{"clone"}

	// Add depth if specified
	if opts.Depth > 0 {
		args = append(args, "--depth", fmt.Sprintf("%d", opts.Depth))
	}

	// Add branch if specified
	if opts.Branch != "" && opts.CommitSHA == "" {
		args = append(args, "--branch", opts.Branch)
	}

	// Add single-branch for efficiency when branch is specified
	if opts.Branch != "" && opts.CommitSHA == "" {
		args = append(args, "--single-branch")
	}

	// Add URL and target path
	cloneURL := m.buildAuthenticatedURL(opts.URL, opts.Credentials)
	args = append(args, cloneURL, targetPath)

	// Execute clone
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Env = m.buildGitEnv(opts.Credentials)

	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("git clone failed: %w\nOutput: %s", err, string(output))
	}

	// If specific commit is requested, checkout that commit
	if opts.CommitSHA != "" {
		if err := m.Checkout(ctx, targetPath, opts.Branch, opts.CommitSHA, opts.Tag); err != nil {
			return fmt.Errorf("failed to checkout commit: %w", err)
		}
	}

	return nil
}

// Checkout checks out a specific ref in the repository.
func (m *Manager) Checkout(ctx context.Context, repoPath, branch, commitSHA, tag string) error {
	// Determine what to checkout
	ref := commitSHA
	if ref == "" && tag != "" {
		ref = tag
	}
	if ref == "" && branch != "" {
		ref = branch
	}
	if ref == "" {
		ref = "HEAD"
	}

	m.logger.Debug().
		Str("path", repoPath).
		Str("ref", ref).
		Msg("Checking out ref")

	// Fetch if we need a specific commit that might not be in shallow clone
	if commitSHA != "" {
		fetchArgs := []string{"fetch", "origin", commitSHA}
		fetchCmd := exec.CommandContext(ctx, "git", fetchArgs...)
		fetchCmd.Dir = repoPath
		if output, err := fetchCmd.CombinedOutput(); err != nil {
			m.logger.Debug().Err(err).Str("output", string(output)).Msg("Fetch specific commit failed, trying full fetch")

			// Try full fetch
			fetchCmd = exec.CommandContext(ctx, "git", "fetch", "--unshallow")
			fetchCmd.Dir = repoPath
			_, _ = fetchCmd.CombinedOutput() // Ignore error if already unshallowed
		}
	}

	// Checkout
	checkoutArgs := []string{"checkout", ref}
	cmd := exec.CommandContext(ctx, "git", checkoutArgs...)
	cmd.Dir = repoPath

	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("git checkout failed: %w\nOutput: %s", err, string(output))
	}

	// Reset to clean state
	resetCmd := exec.CommandContext(ctx, "git", "reset", "--hard", "HEAD")
	resetCmd.Dir = repoPath
	_, _ = resetCmd.CombinedOutput()

	// Clean untracked files
	cleanCmd := exec.CommandContext(ctx, "git", "clean", "-fdx")
	cleanCmd.Dir = repoPath
	_, _ = cleanCmd.CombinedOutput()

	return nil
}

// GetCached returns the cached repository path if it exists.
func (m *Manager) GetCached(url string) string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	key := m.cacheKey(url)
	if cached, ok := m.cache[key]; ok {
		// Verify the path still exists
		if _, err := os.Stat(cached.path); err == nil {
			return cached.path
		}
	}
	return ""
}

// copyFromCache copies a cached repository to the target path.
func (m *Manager) copyFromCache(ctx context.Context, cachePath, targetPath string) error {
	// Create target directory
	if err := os.MkdirAll(filepath.Dir(targetPath), 0755); err != nil {
		return fmt.Errorf("failed to create parent directory: %w", err)
	}

	// Use git clone --local for efficient copy
	cmd := exec.CommandContext(ctx, "git", "clone", "--local", cachePath, targetPath)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to clone from cache: %w\nOutput: %s", err, string(output))
	}

	// Update remote to original URL
	m.mu.RLock()
	key := m.cacheKey(cachePath)
	cached, ok := m.cache[key]
	m.mu.RUnlock()

	if ok {
		cmd = exec.CommandContext(ctx, "git", "remote", "set-url", "origin", cached.url)
		cmd.Dir = targetPath
		_, _ = cmd.CombinedOutput()
	}

	return nil
}

// updateCache updates the cache with a cloned repository.
func (m *Manager) updateCache(url, path string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	key := m.cacheKey(url)

	// Store in cache directory
	cachePath := filepath.Join(m.cacheDir, key)

	// Check if we should update the cache
	if existing, ok := m.cache[key]; ok {
		// Update last used time
		existing.lastUsed = time.Now()
		return
	}

	// Copy to cache directory (background operation)
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()

		cmd := exec.CommandContext(ctx, "git", "clone", "--mirror", path, cachePath)
		if _, err := cmd.CombinedOutput(); err != nil {
			m.logger.Debug().Err(err).Str("url", url).Msg("Failed to cache repository")
			return
		}

		m.mu.Lock()
		m.cache[key] = &cachedRepo{
			path:     cachePath,
			url:      url,
			lastUsed: time.Now(),
			clonedAt: time.Now(),
		}
		m.mu.Unlock()
	}()
}

// Cleanup removes old cached repositories.
func (m *Manager) Cleanup(maxAge time.Duration) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	cutoff := time.Now().Add(-maxAge)
	var toDelete []string

	for key, cached := range m.cache {
		if cached.lastUsed.Before(cutoff) {
			toDelete = append(toDelete, key)
		}
	}

	for _, key := range toDelete {
		cached := m.cache[key]
		if err := os.RemoveAll(cached.path); err != nil {
			m.logger.Warn().Err(err).Str("path", cached.path).Msg("Failed to remove cached repo")
		} else {
			delete(m.cache, key)
			m.logger.Debug().Str("url", cached.url).Msg("Removed cached repository")
		}
	}

	return nil
}

// Glob matches files in the repository.
func (m *Manager) Glob(basePath, pattern string) ([]string, error) {
	fullPattern := filepath.Join(basePath, pattern)
	matches, err := filepath.Glob(fullPattern)
	if err != nil {
		return nil, err
	}
	return matches, nil
}

// buildAuthenticatedURL builds a URL with credentials if provided.
func (m *Manager) buildAuthenticatedURL(url string, creds *Credentials) string {
	if creds == nil || (creds.Username == "" && creds.Password == "") {
		return url
	}

	// Handle HTTPS URLs
	if strings.HasPrefix(url, "https://") {
		// Insert credentials into URL
		parts := strings.SplitN(url, "://", 2)
		if len(parts) == 2 {
			return fmt.Sprintf("https://%s:%s@%s", creds.Username, creds.Password, parts[1])
		}
	}

	return url
}

// buildGitEnv builds environment variables for git commands.
func (m *Manager) buildGitEnv(creds *Credentials) []string {
	env := os.Environ()

	// Disable prompts
	env = append(env, "GIT_TERMINAL_PROMPT=0")

	// Add SSH key if provided
	if creds != nil && creds.SSHKeyPath != "" {
		env = append(env, fmt.Sprintf("GIT_SSH_COMMAND=ssh -i %s -o StrictHostKeyChecking=no", creds.SSHKeyPath))
	}

	return env
}

// cacheKey generates a cache key for a URL.
func (m *Manager) cacheKey(url string) string {
	// Normalize URL
	url = strings.TrimSuffix(url, ".git")
	url = strings.TrimPrefix(url, "https://")
	url = strings.TrimPrefix(url, "http://")
	url = strings.TrimPrefix(url, "git@")

	// Replace special characters
	url = strings.ReplaceAll(url, "/", "_")
	url = strings.ReplaceAll(url, ":", "_")
	url = strings.ReplaceAll(url, "@", "_")

	return url
}
