// Package version reports the binary release version. It defaults to "dev"
// for local builds and is overridden at link time:
//
//	go build -ldflags "-X github.com/LazyGeniusMan/home-ops/projects/external-dns-netbird/internal/version.Version=$VERSION" ./...
//
// The Dockerfile wires ARG VERSION into the same flag; the value surfaces
// via GET /version and the external_dns_netbird_build_info gauge.
package version

// Version is the release version ("dev" unless overridden by ldflags).
var Version = "dev"
