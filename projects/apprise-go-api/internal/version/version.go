// Package version carries the service version string.
//
// The default is "dev" for local builds. Release images override it at link
// time:
//
//	go build -ldflags "-X github.com/LazyGeniusMan/home-ops/projects/apprise-go-api/internal/version.Version=$VERSION" ./cmd/apprise-go-api
//
// The Dockerfile wires ARG VERSION through exactly this flag, and the value
// is surfaced via the apprise_go_api_build_info{version="..."} gauge on
// /metrics (no separate /version endpoint).
package version

// Version is the service version ("dev" unless overridden by ldflags).
var Version = "dev"
