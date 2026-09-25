// Package version carries the service version string ("dev" locally;
// release images override via ldflags ARG VERSION, surfaced on /metrics).
package version

// Version is the service version ("dev" unless overridden by ldflags).
var Version = "dev"
