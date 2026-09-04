// Package version holds the build version, injected at link time by the
// Makefile and goreleaser (-X github.com/stranix79/deckhand/internal/version.Version=…).
package version

// Version is "dev" for local builds and the git tag for releases.
var Version = "dev"
