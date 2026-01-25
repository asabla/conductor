package git

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseRepositoryURL(t *testing.T) {
	tests := []struct {
		name      string
		url       string
		wantOwner string
		wantRepo  string
		wantErr   bool
	}{
		{
			name:      "https github url",
			url:       "https://github.com/owner/repo",
			wantOwner: "owner",
			wantRepo:  "repo",
		},
		{
			name:      "https github url with .git",
			url:       "https://github.com/owner/repo.git",
			wantOwner: "owner",
			wantRepo:  "repo",
		},
		{
			name:      "ssh github url",
			url:       "git@github.com:owner/repo.git",
			wantOwner: "owner",
			wantRepo:  "repo",
		},
		{
			name:      "simple owner/repo format",
			url:       "owner/repo",
			wantOwner: "owner",
			wantRepo:  "repo",
		},
		{
			name:      "gitlab https url",
			url:       "https://gitlab.com/owner/repo",
			wantOwner: "owner",
			wantRepo:  "repo",
		},
		{
			name:      "bitbucket https url",
			url:       "https://bitbucket.org/owner/repo",
			wantOwner: "owner",
			wantRepo:  "repo",
		},
		{
			name:    "invalid url - no repo",
			url:     "owner",
			wantErr: true,
		},
		{
			name:    "invalid url - empty",
			url:     "",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			owner, repo, err := parseRepositoryURL(tt.url)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantOwner, owner)
			assert.Equal(t, tt.wantRepo, repo)
		})
	}
}

func TestNewProvider(t *testing.T) {
	t.Run("creates github provider by default", func(t *testing.T) {
		cfg := Config{Token: "test-token"}
		provider, err := NewProvider(cfg)
		require.NoError(t, err)
		assert.NotNil(t, provider)
	})

	t.Run("creates github provider explicitly", func(t *testing.T) {
		cfg := Config{Provider: "github", Token: "test-token"}
		provider, err := NewProvider(cfg)
		require.NoError(t, err)
		assert.NotNil(t, provider)
	})

	t.Run("creates gitlab provider", func(t *testing.T) {
		cfg := Config{Provider: "gitlab", Token: "test-token"}
		provider, err := NewProvider(cfg)
		require.NoError(t, err)
		assert.NotNil(t, provider)
	})

	t.Run("creates bitbucket provider", func(t *testing.T) {
		cfg := Config{Provider: "bitbucket", Token: "test-token"}
		provider, err := NewProvider(cfg)
		require.NoError(t, err)
		assert.NotNil(t, provider)
	})

	t.Run("returns error for unknown provider", func(t *testing.T) {
		cfg := Config{Provider: "unknown", Token: "test-token"}
		_, err := NewProvider(cfg)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "unsupported")
	})
}

func TestNewGitHubProvider(t *testing.T) {
	t.Run("uses default API URL", func(t *testing.T) {
		cfg := Config{Token: "test-token"}
		provider, err := NewGitHubProvider(cfg)
		require.NoError(t, err)
		assert.Equal(t, "https://api.github.com", provider.baseURL)
	})

	t.Run("uses custom base URL", func(t *testing.T) {
		cfg := Config{
			Token:   "test-token",
			BaseURL: "https://github.example.com/api/v3",
		}
		provider, err := NewGitHubProvider(cfg)
		require.NoError(t, err)
		assert.Equal(t, "https://github.example.com/api/v3", provider.baseURL)
	})

	t.Run("strips trailing slash from base URL", func(t *testing.T) {
		cfg := Config{
			Token:   "test-token",
			BaseURL: "https://api.github.com/",
		}
		provider, err := NewGitHubProvider(cfg)
		require.NoError(t, err)
		assert.Equal(t, "https://api.github.com", provider.baseURL)
	})
}

func TestDecodeBase64Content(t *testing.T) {
	t.Run("decodes base64 content", func(t *testing.T) {
		// "Hello, World!" in base64
		encoded := "SGVsbG8sIFdvcmxkIQ=="
		decoded, err := decodeBase64Content(encoded)
		require.NoError(t, err)
		assert.Equal(t, "Hello, World!", string(decoded))
	})

	t.Run("handles content with newlines", func(t *testing.T) {
		// "Hello" in base64 with newlines (as GitHub returns it)
		encoded := "SGVs\nbG8="
		decoded, err := decodeBase64Content(encoded)
		require.NoError(t, err)
		assert.Equal(t, "Hello", string(decoded))
	})
}
