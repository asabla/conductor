package secrets

import "context"

// Provider identifies the secret backend.
type Provider string

const (
	ProviderVault Provider = "vault"
)

// Reference identifies a single secret value in a store.
type Reference struct {
	Name     string
	Provider Provider
	Path     string
	Key      string
	Version  int
}

// Store resolves secret references to plaintext values.
type Store interface {
	Resolve(ctx context.Context, ref Reference) (string, error)
}
