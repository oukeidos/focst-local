package version

import "fmt"

// Version is the release version embedded in the binary.
// It can be overridden at build time via:
// go build -ldflags "-X github.com/oukeidos/focst-local/internal/version.Version=0.1.0"
var Version = "0.1.0"

// Commit is the git commit hash embedded in the binary.
// It can be overridden at build time via:
// go build -ldflags "-X github.com/oukeidos/focst-local/internal/version.Commit=abcdef1"
var Commit = "unknown"

// BuildDate is the RFC3339 build timestamp embedded in the binary.
// It can be overridden at build time via:
// go build -ldflags "-X github.com/oukeidos/focst-local/internal/version.BuildDate=2026-01-30T12:00:00Z"
var BuildDate = "unknown"

// Info returns a multi-line version string for CLI output.
func Info() string {
	return fmt.Sprintf("focst-local %s\ncommit: %s\nbuild: %s", Version, Commit, BuildDate)
}
