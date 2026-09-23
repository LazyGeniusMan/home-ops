// Package provider implements the ESO webhook provider pull/push contract
// backed by Proton Pass (pass-cli).
//
// ESO generic webhook provider contract (see README.md):
//
//   - Pull (GetSecret): GET /get?key=pass://vault/item/field (also POST with a
//     JSON body {"remoteRef": {"key": ...}}) → 200 {"value": "<secret>"}.
//     Unknown keys → 404 so ESO applies the ExternalSecret deletionPolicy.
//   - Validate: HEAD / (also GET /) → 200.
//   - Push: not implemented; POST /push → 501 (pull-only provider).
package provider

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/LazyGeniusMan/home-ops/projects/eso-proton-pass/internal/passclient"
)

// Sentinels classify pull-path failures. Every layer wraps with %w so
// errors.Is finds the sentinel through any chain; the server maps them to
// HTTP statuses (see internal/server/errors.go). Messages stay lowercase
// with no trailing punctuation per the error contract.
var (
	// ErrInvalidKey marks malformed remote keys (shape validation) → 400.
	ErrInvalidKey = errors.New("invalid key")
	// ErrNotFound marks missing secrets (backend miss or empty value) → 404.
	// It lets ESO apply the ExternalSecret deletionPolicy.
	ErrNotFound = errors.New("secret not found")
	// ErrUnprocessable marks well-formed references the backend permanently
	// rejects (e.g. unknown vault) → 422.
	ErrUnprocessable = errors.New("unprocessable reference")
	// ErrUpstream marks transient backend failures (exec errors) → 502.
	ErrUpstream = errors.New("upstream backend failure")
)

// ErrPushUnimplemented is returned for push attempts (pull-only provider).
var ErrPushUnimplemented = errors.New("push is not implemented (pull-only provider)")

// Resolver resolves pass:// URIs to secret values and probes backend
// reachability. The production implementation is *passclient.Client.
type Resolver interface {
	ResolveSecret(ctx context.Context, uri, reasonPrefix string) (string, error)
	Ping(ctx context.Context) error
}

// Provider serves the pull path of the ESO webhook contract.
type Provider struct {
	resolver Resolver
	logger   *slog.Logger
}

// New builds a Provider.
func New(resolver Resolver, logger *slog.Logger) *Provider {
	return &Provider{resolver: resolver, logger: logger}
}

// ValidateKey parses and validates a remote key. Expected form:
//
//	pass://{vault}/{item}/{field}
//
// The key itself never enters error strings: the field segment can be a
// sensitive label, so failures report only the expected shape (see the eso
// redactURI/vaultItem secret-safety pattern).
func ValidateKey(key string) (vault, item, field string, err error) {
	const prefix = "pass://"
	if !strings.HasPrefix(key, prefix) {
		return "", "", "", fmt.Errorf("invalid key: must start with %q", prefix)
	}
	parts := strings.Split(strings.TrimPrefix(key, prefix), "/")
	if len(parts) != 3 || parts[0] == "" || parts[1] == "" || parts[2] == "" {
		return "", "", "", fmt.Errorf("invalid key: want pass://{vault}/{item}/{field}")
	}
	return parts[0], parts[1], parts[2], nil
}

// GetSecret resolves key to its secret value, classifying failures with
// sentinels (all wrapped with %w). The key itself never enters the error
// string — only vault/item is logged (see vaultItem).
func (p *Provider) GetSecret(ctx context.Context, key string) (string, error) {
	if _, _, _, err := ValidateKey(key); err != nil {
		return "", fmt.Errorf("%w: %w", ErrInvalidKey, err)
	}
	value, err := p.resolver.ResolveSecret(ctx, key, "eso-proton-pass-get")
	if err != nil {
		// Typed match only: the resolver already classified the CLI
		// boundary into passclient.ErrNotFound; no substring matching here.
		if errors.Is(err, passclient.ErrNotFound) {
			return "", fmt.Errorf("%w: %w", ErrNotFound, err)
		}
		if errors.Is(err, ErrUnprocessable) {
			return "", fmt.Errorf("%w: %w", ErrUnprocessable, err)
		}
		return "", fmt.Errorf("%w: %w", ErrUpstream, err)
	}
	if value == "" {
		return "", fmt.Errorf("%w: empty value", ErrNotFound)
	}
	p.logger.Debug("secret resolved",
		slog.String("vault_item", vaultItem(key)),
	)
	return value, nil
}

// Push is explicitly unimplemented: this is a pull-only provider.
func (p *Provider) Push(_ context.Context, _ string, _ []byte) error {
	return ErrPushUnimplemented
}

// Ready probes pass-cli reachability without resolving a secret. A nil
// error means the backend is reachable; any error is wrapped with %w and
// classified by the caller (the /readyz probe reports only the failing
// dependency name, never the detail).
func (p *Provider) Ready(ctx context.Context) error {
	if err := p.resolver.Ping(ctx); err != nil {
		return fmt.Errorf("ready: %w", err)
	}
	return nil
}

// vaultItem returns "vault/item" for safe logging (field omitted).
func vaultItem(key string) string {
	vault, item, _, err := ValidateKey(key)
	if err != nil {
		return "<invalid>"
	}
	return vault + "/" + item
}
