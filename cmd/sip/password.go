package sip

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/Bandwidth/cli/internal/cmdutil"
	sipsvc "github.com/Bandwidth/cli/internal/sip"
)

const maxPasswordBytes = 1024

// readPassword resolves exactly one password source. Secrets are never accepted
// via argv: command-line arguments leak through shell history, process listings,
// CI logs, and agent transcripts.
//
// Returns the password and whether the CLI generated it (callers only print
// generated passwords — a caller-supplied secret is already known to the caller).
func readPassword(cmd *cobra.Command, stdin bool, file string, generate bool) (string, bool, error) {
	sources := 0
	for _, on := range []bool{stdin, file != "", generate} {
		if on {
			sources++
		}
	}
	if sources != 1 {
		return "", false, fmt.Errorf("specify exactly one of --password-stdin, --password-file, or --generate-password")
	}

	if generate {
		pw, err := sipsvc.GeneratePassword()
		return pw, true, err
	}

	var raw []byte
	var err error
	if stdin {
		if cmdutil.IsInteractive() {
			// The prompt goes to stderr, never stdout — stdout is the
			// machine-readable output channel for `band` commands.
			fmt.Fprint(cmd.ErrOrStderr(), "SIP password: ")
			pw, perr := cmdutil.ReadPassword()
			fmt.Fprintln(cmd.ErrOrStderr())
			if perr != nil {
				return "", false, fmt.Errorf("reading password: %w", perr)
			}
			raw = pw
		} else {
			raw, err = io.ReadAll(io.LimitReader(cmd.InOrStdin(), maxPasswordBytes+1))
		}
	} else {
		raw, err = os.ReadFile(file)
	}
	if err != nil {
		return "", false, fmt.Errorf("reading password: %w", err)
	}
	if len(raw) > maxPasswordBytes {
		return "", false, fmt.Errorf("password exceeds %d bytes", maxPasswordBytes)
	}

	pw := strings.TrimSuffix(strings.TrimSuffix(string(raw), "\n"), "\r")
	if pw == "" {
		return "", false, fmt.Errorf("password is empty")
	}
	return pw, false, nil
}
