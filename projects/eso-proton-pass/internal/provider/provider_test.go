package provider

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"testing"
)

type fakeResolver struct {
	value string
	err   error
	last  string
}

func (f *fakeResolver) ResolveSecret(_ context.Context, uri, _ string) (string, error) {
	f.last = uri
	return f.value, f.err
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

func TestGetSecretNotFound(t *testing.T) {
	for _, err := range []error{
		errors.New("item not found"),
		errors.New("404 from backend"),
	} {
		f := &fakeResolver{err: err}
		p := testProvider(f)
		if _, gerr := p.GetSecret(context.Background(), "pass://v/i/f"); !errors.Is(gerr, ErrNotFound) {
			t.Errorf("err %v: expected ErrNotFound, got %v", err, gerr)
		}
	}
	f := &fakeResolver{value: ""}
	p := testProvider(f)
	if _, err := p.GetSecret(context.Background(), "pass://v/i/f"); !errors.Is(err, ErrNotFound) {
		t.Errorf("empty value: expected ErrNotFound, got %v", err)
	}
}

func TestGetSecretInvalidKey(t *testing.T) {
	p := testProvider(&fakeResolver{value: "x"})
	if _, err := p.GetSecret(context.Background(), "bogus"); err == nil {
		t.Error("expected error for invalid key")
	}
}

func TestPushUnimplemented(t *testing.T) {
	p := testProvider(&fakeResolver{})
	err := p.Push(context.Background(), "pass://v/i/f", []byte("x"))
	if !errors.Is(err, ErrPushUnimplemented) {
		t.Fatalf("expected ErrPushUnimplemented, got %v", err)
	}
}
