package version

// These are set via ldflags at build time
var (
	Version = "dev"
	Commit  = "unknown"
	Date    = "unknown"
)

func Full() string {
	if Version == "dev" {
		return "codeany dev (built from source)"
	}
	return "codeany " + Version + " (" + Commit[:min(7, len(Commit))] + " " + Date + ")"
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
