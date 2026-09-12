// Package main is the entry point for the Hades schema registry binary.
package main

import (
	"os"
)

// version and commit are set at build time with -ldflags. They default to
// "dev" and "unknown" so a `go run` build says so rather than claiming a
// release.
var (
	version = "dev"
	commit  = "unknown"
)

func main() {
	// Cobra already prints the error for RunE failures; printing it here too
	// would double it. What must not happen is the previous behaviour: a silent
	// exit code with nothing on stderr.
	if err := newRootCmd(os.Args[1:]).Execute(); err != nil {
		os.Exit(1)
	}
}
