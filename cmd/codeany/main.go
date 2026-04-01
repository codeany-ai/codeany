package main

import (
	"fmt"
	"os"

	"github.com/codeany-ai/codeany/internal/cli"
	"github.com/codeany-ai/codeany/internal/version"
)

func main() {
	// Set version from ldflags
	cli.SetVersion(version.Version, version.Commit, version.Date)

	if err := cli.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
