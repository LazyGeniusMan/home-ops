package provider

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"testing"

	"github.com/LazyGeniusMan/home-ops/projects/eso-proton-pass/internal/passclient"
)

type fakeResolver struct {
	value   string
	err     error
	last    string
	pingErr error
	pings   int
}

func (f *fakeResolver) ResolveSecret(_ context.Context, uri, _ string) (string, error) {
	f.last = uri
	return f.value, f.err
}

func (f *fakeResolver) Ping(_ context.Context) error {
	f.pings++
	return f.pingErr
}

func testProvider(f *fakeResolver) *Provider {
	return New(f, slog.New(slog.NewTextHandler(os.Stderr, nil)))
}

func TestValidateKey(t *testing.T) {
	vault, item, field, err := ValidateKey("pass://cluster/ns/api-key")
	if err != nil {
		t.Fatalf("ValidateKey: %v", err)
	}
	if vault != "cluster" || item != "ns" || field != "api-key" {
		t.Errorf("unexpected parse: %q %q %q", vault, item, field)
	}
	for _, bad := range []string{
		"",
		"https://x/y/z",
		"pass://only-two/parts",
		"pass://a/b/c/d",
		"pass:///b/c",
		"pass://a//c",
		"pass://a/b/",
	} {
		if _, _, _, err := ValidateKey(bad); err == nil {
			t.Errorf("expected error for %q", bad)
		}
	}
}

func TestGetSecretParity(t *testing.T) {
	f := &fakeResolver{value: "s3cret"}
	p := testProvider(f)
	got, err := p.GetSecret(context.Background(), "pass://vault/item/password")
	if err != nil {
		t.Fatalf("GetSecret: %v", err)
	}
	if got != "s3cret" {
		t.Errorf("got %q", got)
	}
	if f.last != "pass://vault/item/password" {
		t.Errorf("resolver got %q", f.last)
	}
}

// TestGetSecretSentinels checks the %w chain: typed boundary sentinel and
// wrapped variants must surface the provider sentinel via errors.Is.
func TestGetSecretSentinels(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want error
	}{
		{"boundary not found", fmt.Errorf("resolve %s: %w", "pass://v/i/<field>", passclient.ErrNotFound), ErrNotFound},
		{"wrapped not found", fmt.Errorf("get: %w", fmt.Errorf("resolve: %w", passclient.ErrNotFound)), ErrNotFound},
		{"unprocessable", fmt.Errorf("resolve: %w", ErrUnprocessable), ErrUnprocessable},
		{"wrapped unprocessable", fmt.Errorf("layer: %w", fmt.Errorf("layer2: %w", ErrUnprocessable)), ErrUnprocessable},
		{"upstream exec", fmt.Errorf("get: %w", &passclient.ExecError{Op: "inject"}), ErrUpstream},
		{"plain backend", errors.New("boom"), ErrUpstream},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := &fakeResolver{err: tc.err}
			p := testProvider(f)
			if _, gerr := p.GetSecret(context.Background(), "pass://v/i/f"); !errors.Is(gerr, tc.want) {
				t.Errorf("err %v: expected %v, got %v", tc.err, tc.want, gerr)
			}
		})
	}
}

func TestGetSecretEmptyValueNotFound(t *testing.T) {
	f := &fakeResolver{value: ""}
	p := testProvider(f)
	if _, err := p.GetSecret(context.Background(), "pass://v/i/f"); !errors.Is(err, ErrNotFound) {
		t.Errorf("empty value: expected ErrNotFound, got %v", err)
	}
}

// TestGetSecretInvalidKey checks shape validation maps to ErrInvalidKey and
// never echoes the key: the field segment can be a sensitive label.
func TestGetSecretInvalidKey(t *testing.T) {
	p := testProvider(&fakeResolver{value: "x"})
	// A well-formed key resolves through the fake.
	if _, err := p.GetSecret(context.Background(), "pass://v/i/field"); err != nil {
		t.Fatalf("valid key: %v", err)
	}
	// Shape failures report only the expected form: vault/item/field input
	// values (which can be sensitive labels) must not appear.
	for _, tc := range []struct{ key, secretFrag string }{
		{"bogus", "bogus"},
		{"pass://only-two/supersecretlabel", "supersecretlabel"},
		{"pass://vaultname//c", "vaultname"},
		{"https://x/supersecretlabel/z", "supersecretlabel"},
	} {
		_, err := p.GetSecret(context.Background(), tc.key)
		if err == nil {
			t.Errorf("key %q: expected error", tc.key)
			continue
		}
		if !errors.Is(err, ErrInvalidKey) {
			t.Errorf("key %q: expected ErrInvalidKey, got %v", tc.key, err)
		}
		if strings.Contains(err.Error(), tc.secretFrag) {
			t.Errorf("key %q: error echoes input %q: %q", tc.key, tc.secretFrag, err.Error())
		}
	}
}

func TestReady(t *testing.T) {
	f := &fakeResolver{}
	p := testProvider(f)
	if err := p.Ready(context.Background()); err != nil {
		t.Fatalf("Ready: %v", err)
	}
	if f.pings != 1 {
		t.Errorf("pings = %d, want 1", f.pings)
	}
	f.pingErr = errors.New("down")
	if err := p.Ready(context.Background()); err == nil {
		t.Error("Ready: expected error when backend down")
	}
}

func TestPushUnimplemented(t *testing.T) {
	p := testProvider(&fakeResolver{})
	err := p.Push(context.Background(), "pass://v/i/f", []byte("x"))
	if !errors.Is(err, ErrPushUnimplemented) {
		t.Fatalf("expected ErrPushUnimplemented, got %v", err)
	}
}
