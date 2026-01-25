package git

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStaticTokenSource(t *testing.T) {
	source := NewStaticTokenSource("test-token")
	token, err := source.Token(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "test-token", token)
}

func TestGitHubAppTokenSource_CachesToken(t *testing.T) {
	privateKey := generateTestPrivateKey(t)

	var callCount int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&callCount, 1)
		assert.Equal(t, "POST", r.Method)
		assert.Equal(t, "/app/installations/42/access_tokens", r.URL.Path)
		assert.True(t, strings.HasPrefix(r.Header.Get("Authorization"), "Bearer "))

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"token":      "installation-token",
			"expires_at": time.Now().Add(2 * time.Hour).UTC().Format(time.RFC3339),
		})
	}))
	defer server.Close()

	source, err := NewGitHubAppTokenSource(GitHubAppConfig{
		AppID:          123,
		InstallationID: 42,
		PrivateKey:     privateKey,
		BaseURL:        server.URL,
		HTTPClient:     server.Client(),
	})
	require.NoError(t, err)

	ctx := context.Background()
	firstToken, err := source.Token(ctx)
	require.NoError(t, err)
	secondToken, err := source.Token(ctx)
	require.NoError(t, err)

	assert.Equal(t, "installation-token", firstToken)
	assert.Equal(t, "installation-token", secondToken)
	assert.Equal(t, int32(1), atomic.LoadInt32(&callCount))
}

func generateTestPrivateKey(t *testing.T) string {
	t.Helper()

	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)

	encoded := pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(key),
	})

	return string(encoded)
}
