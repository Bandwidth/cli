// Command band is the official Bandwidth CLI for managing voice, messaging,
// numbers, and more from the command line.
//
// It lets you build and debug voice applications, send messages, manage phone
// numbers, and control calls without leaving your terminal. Every command
// supports --plain for flat JSON output, --if-not-exists for safe retries, and
// --wait for async operations, so it works equally well interactively and from
// scripts or AI agents.
//
// Install with Homebrew (brew install Bandwidth/tap/band) or go install
// (go install github.com/Bandwidth/cli/cmd/band@latest), then run "band --help"
// to get started.
//
// For the full command reference and guides, see https://dev.bandwidth.com and
// the project README at https://github.com/Bandwidth/cli.
package main

import (
	"os"

	"github.com/Bandwidth/cli/cmd"
	"github.com/Bandwidth/cli/internal/cmdutil"
)

func main() {
	if err := cmd.Execute(); err != nil {
		os.Exit(cmdutil.ExitCodeForError(err))
	}
}
