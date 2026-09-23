// Package version carries the service version string.
//
// The default is "dev" for local builds. Release images override it at link
// time:
//
//	go build -ldflags "-X github.com/LazyGeniusMan/home-ops/projects/eso-proton-pass/internal/version.Version=$VERSION" ./cmd/eso-proton-pass
//
// The Dockerfile wires ARG VERSION through exactly this flag, and the value
// is surfaced via the eso_proton_pass_build_info{version="..."} gauge on
// /metrics (no separate /version endpoint).
package version

// Version is the service version ("dev" unless overridden by ldflags).
var Version = "dev"
