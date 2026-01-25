package git

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

const (
	gitHubJWTValidFor  = 9 * time.Minute
	gitHubTokenRefresh = 2 * time.Minute
)

// TokenSource provides Git auth tokens.
type TokenSource interface {
	Token(ctx context.Context) (string, error)
}

// StaticTokenSource returns a static token.
type StaticTokenSource struct {
	token string
}

// NewStaticTokenSource creates a token source backed by a static token.
func NewStaticTokenSource(token string) *StaticTokenSource {
	return &StaticTokenSource{token: token}
}

// Token returns the configured token.
func (s *StaticTokenSource) Token(_ context.Context) (string, error) {
	if s.token == "" {
		return "", errors.New("token is empty")
	}
	return s.token, nil
}

// GitHubAppConfig contains settings for GitHub App authentication.
type GitHubAppConfig struct {
	AppID          int64
	InstallationID int64
	PrivateKey     string
	BaseURL        string
	UserAgent      string
	HTTPClient     *http.Client
}

// GitHubAppTokenSource generates and caches GitHub App installation tokens.
type GitHubAppTokenSource struct {
	client         *http.Client
	baseURL        string
	userAgent      string
	appID          int64
	installationID int64
	privateKey     *rsa.PrivateKey

	mu        sync.Mutex
	token     string
	expiresAt time.Time
}

// NewGitHubAppTokenSource creates a new GitHub App token source.
func NewGitHubAppTokenSource(cfg GitHubAppConfig) (*GitHubAppTokenSource, error) {
	if cfg.AppID <= 0 {
		return nil, errors.New("app ID is required")
	}
	if cfg.InstallationID <= 0 {
		return nil, errors.New("installation ID is required")
	}
	if cfg.PrivateKey == "" {
		return nil, errors.New("private key is required")
	}

	privateKey, err := parseRSAPrivateKey(cfg.PrivateKey)
	if err != nil {
		return nil, fmt.Errorf("failed to parse private key: %w", err)
	}

	baseURL := cfg.BaseURL
	if baseURL == "" {
		baseURL = DefaultGitHubBaseURL
	}
	baseURL = strings.TrimSuffix(baseURL, "/")

	client := cfg.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: DefaultHTTPTimeout}
	}

	userAgent := cfg.UserAgent
	if userAgent == "" {
		userAgent = DefaultUserAgent
	}

	return &GitHubAppTokenSource{
		client:         client,
		baseURL:        baseURL,
		userAgent:      userAgent,
		appID:          cfg.AppID,
		installationID: cfg.InstallationID,
		privateKey:     privateKey,
	}, nil
}

// Token returns a cached or freshly generated installation token.
func (s *GitHubAppTokenSource) Token(ctx context.Context) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.token != "" && time.Until(s.expiresAt) > gitHubTokenRefresh {
		return s.token, nil
	}

	token, expiresAt, err := s.fetchInstallationToken(ctx)
	if err != nil {
		return "", err
	}

	s.token = token
	s.expiresAt = expiresAt

	return token, nil
}

func (s *GitHubAppTokenSource) fetchInstallationToken(ctx context.Context) (string, time.Time, error) {
	jwt, err := s.generateJWT(time.Now().UTC())
	if err != nil {
		return "", time.Time{}, fmt.Errorf("failed to generate JWT: %w", err)
	}

	url := fmt.Sprintf("%s/app/installations/%d/access_tokens", s.baseURL, s.installationID)
	req, err := http.NewRequestWithContext(ctx, "POST", url, nil)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("failed to create token request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+jwt)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", s.userAgent)

	resp, err := s.client.Do(req)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("failed to request installation token: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", time.Time{}, fmt.Errorf("installation token request failed (status %d): %s", resp.StatusCode, string(body))
	}

	var payload struct {
		Token     string    `json:"token"`
		ExpiresAt time.Time `json:"expires_at"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return "", time.Time{}, fmt.Errorf("failed to decode installation token response: %w", err)
	}
	if payload.Token == "" {
		return "", time.Time{}, errors.New("installation token response missing token")
	}

	return payload.Token, payload.ExpiresAt, nil
}

func (s *GitHubAppTokenSource) generateJWT(now time.Time) (string, error) {
	header := map[string]string{
		"alg": "RS256",
		"typ": "JWT",
	}

	claims := map[string]interface{}{
		"iat": now.Add(-time.Minute).Unix(),
		"exp": now.Add(gitHubJWTValidFor).Unix(),
		"iss": s.appID,
	}

	encodedHeader, err := encodeJWTPart(header)
	if err != nil {
		return "", fmt.Errorf("failed to encode JWT header: %w", err)
	}
	encodedClaims, err := encodeJWTPart(claims)
	if err != nil {
		return "", fmt.Errorf("failed to encode JWT claims: %w", err)
	}

	signingInput := encodedHeader + "." + encodedClaims
	hash := sha256.Sum256([]byte(signingInput))

	signature, err := rsa.SignPKCS1v15(rand.Reader, s.privateKey, crypto.SHA256, hash[:])
	if err != nil {
		return "", fmt.Errorf("failed to sign JWT: %w", err)
	}

	encodedSignature := base64.RawURLEncoding.EncodeToString(signature)

	return signingInput + "." + encodedSignature, nil
}

func encodeJWTPart(value interface{}) (string, error) {
	encoded, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(encoded), nil
}

func parseRSAPrivateKey(pemData string) (*rsa.PrivateKey, error) {
	block, _ := pem.Decode([]byte(pemData))
	if block == nil {
		return nil, errors.New("invalid PEM data")
	}

	switch block.Type {
	case "RSA PRIVATE KEY":
		key, err := x509.ParsePKCS1PrivateKey(block.Bytes)
		if err != nil {
			return nil, fmt.Errorf("failed to parse PKCS1 private key: %w", err)
		}
		return key, nil
	case "PRIVATE KEY":
		key, err := x509.ParsePKCS8PrivateKey(block.Bytes)
		if err != nil {
			return nil, fmt.Errorf("failed to parse PKCS8 private key: %w", err)
		}
		rsaKey, ok := key.(*rsa.PrivateKey)
		if !ok {
			return nil, errors.New("private key is not RSA")
		}
		return rsaKey, nil
	default:
		return nil, fmt.Errorf("unsupported private key type: %s", block.Type)
	}
}
