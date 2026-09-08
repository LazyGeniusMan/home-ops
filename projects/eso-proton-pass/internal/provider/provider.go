// Package provider implements the ESO webhook provider pull/push contract
// backed by Proton Pass (pass-cli).
//
// ESO generic webhook provider contract (see README.md):
//
//   - Pull (GetSecret): GET /get?key=pass://vault/item/field (also POST with a
//     JSON body {"remoteRef": {"key": ...}}) → 200 {"value": "<secret>"}.
//     Unknown keys → 404 so ESO applies the ExternalSecret deletionPolicy.
//   - Validate: HEAD / (also GET /) → 200.
//   - Push: not implemented; POST /push → 501 with an explicit TODO message.
package provider

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
)

// ErrNotFound is returned when the referenced secret does not exist.
var ErrNotFound = errors.New("secret not found")

// ErrPushUnimplemented is returned for push attempts (TODO: push support).
var ErrPushUnimplemented = errors.New("TODO: push is not implemented (pull-only provider)")

// Resolver resolves a pass:// URI to its secret value.
type Resolver interface {
	ResolveSecret(ctx context.Context, uri, reasonPrefix string) (string, error)
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
func ValidateKey(key string) (vault, item, field string, err error) {
	const prefix = "pass://"
	if !strings.HasPrefix(key, prefix) {
		return "", "", "", fmt.Errorf("invalid key %q: must start with %q", key, prefix)
	}
	parts := strings.Split(strings.TrimPrefix(key, prefix), "/")
	if len(parts) != 3 || parts[0] == "" || parts[1] == "" || parts[2] == "" {
		return "", "", "", fmt.Errorf("invalid key %q: want pass://{vault}/{item}/{field}", key)
	}
	return parts[0], parts[1], parts[2], nil
}

// GetSecret resolves key to its secret value. It returns ErrNotFound when the
// backend reports the secret as missing.
func (p *Provider) GetSecret(ctx context.Context, key string) (string, error) {
	if _, _, _, err := ValidateKey(key); err != nil {
		return "", err
	}
	value, err := p.resolver.ResolveSecret(ctx, key, "eso-proton-pass-get")
	if err != nil {
		if isNotFound(err) {
			return "", fmt.Errorf("%w: %s", ErrNotFound, key)
		}
		return "", fmt.Errorf("get secret: %w", err)
	}
	if value == "" {
		return "", fmt.Errorf("%w: %s (empty value)", ErrNotFound, key)
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

// isNotFound matches backend "not found" style errors to ErrNotFound.
func isNotFound(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	for _, marker := range []string{"not found", "no such", "does not exist", "not exist", "404"} {
		if strings.Contains(msg, marker) {
			return true
		}
	}
	return false
}

// vaultItem returns "vault/item" for safe logging (field omitted).
func vaultItem(key string) string {
	vault, item, _, err := ValidateKey(key)
	if err != nil {
		return "<invalid>"
	}
	return vault + "/" + item
}
