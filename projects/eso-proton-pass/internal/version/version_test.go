package version

import (
	"strings"
	"testing"
)

func TestVersionDefault(t *testing.T) {
	if Version == "" {
		t.Fatal("Version must not be empty (want dev for local builds)")
	}
	if strings.ContainsAny(Version, " \n\t") {
		t.Errorf("Version %q must be a single token for ldflags injection", Version)
	}
}
