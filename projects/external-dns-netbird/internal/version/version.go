// Package version reports the binary release version ("dev" locally;
// release images override via ldflags ARG VERSION).
package version

// Version is the release version ("dev" unless overridden by ldflags).
var Version = "dev"
