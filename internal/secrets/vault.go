package secrets

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const defaultVaultMount = "secret"

// VaultConfig configures a Vault-backed secret store.
type VaultConfig struct {
	Address   string
	Token     string
	Namespace string
	Mount     string
	Timeout   time.Duration
}

// VaultStore resolves secrets from HashiCorp Vault KV v2.
type VaultStore struct {
	address   string
	token     string
	namespace string
	mount     string
	client    *http.Client
}

// NewVaultStore creates a new Vault-based store.
func NewVaultStore(cfg VaultConfig) (*VaultStore, error) {
	if cfg.Address == "" {
		return nil, errors.New("vault address is required")
	}
	if cfg.Token == "" {
		return nil, errors.New("vault token is required")
	}
	mount := cfg.Mount
	if mount == "" {
		mount = defaultVaultMount
	}
	if cfg.Timeout == 0 {
		cfg.Timeout = 10 * time.Second
	}

	return &VaultStore{
		address:   strings.TrimRight(cfg.Address, "/"),
		token:     cfg.Token,
		namespace: cfg.Namespace,
		mount:     strings.Trim(mount, "/"),
		client: &http.Client{
			Timeout: cfg.Timeout,
		},
	}, nil
}

// Resolve fetches a secret value from Vault.
func (v *VaultStore) Resolve(ctx context.Context, ref Reference) (string, error) {
	if ref.Path == "" {
		return "", errors.New("secret path is required")
	}
	if ref.Key == "" {
		return "", errors.New("secret key is required")
	}

	endpoint := fmt.Sprintf("%s/v1/%s/data/%s", v.address, v.mount, strings.TrimLeft(ref.Path, "/"))
	reqURL, err := url.Parse(endpoint)
	if err != nil {
		return "", fmt.Errorf("invalid vault url: %w", err)
	}
	if ref.Version > 0 {
		query := reqURL.Query()
		query.Set("version", fmt.Sprintf("%d", ref.Version))
		reqURL.RawQuery = query.Encode()
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL.String(), nil)
	if err != nil {
		return "", fmt.Errorf("create vault request: %w", err)
	}
	req.Header.Set("X-Vault-Token", v.token)
	if v.namespace != "" {
		req.Header.Set("X-Vault-Namespace", v.namespace)
	}

	resp, err := v.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("vault request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("vault returned status %d", resp.StatusCode)
	}

	var payload struct {
		Data struct {
			Data map[string]any `json:"data"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return "", fmt.Errorf("decode vault response: %w", err)
	}

	value, ok := payload.Data.Data[ref.Key]
	if !ok {
		return "", fmt.Errorf("vault key not found: %s", ref.Key)
	}
	stringValue, ok := value.(string)
	if !ok {
		return "", fmt.Errorf("vault key %s is not a string", ref.Key)
	}

	return stringValue, nil
}
