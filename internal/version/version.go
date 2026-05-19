// Package version exposes build-time metadata.
package version

var (
	// Version is the semantic version, injected at build time.
	Version = "dev"
	// Commit is the git commit SHA, injected at build time.
	Commit = "none"
	// Date is the build date, injected at build time.
	Date = "unknown"
)

// String returns a single-line summary suitable for /version output.
func String() string {
	return Version + " (" + Commit + ", " + Date + ")"
}
