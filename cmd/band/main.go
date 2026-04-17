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
