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

// readLimit caps how many bytes readPassword ever reads before deciding an
// input is oversized. It allows maxPasswordBytes plus up to two bytes of
// trailing CRLF (so a max-length password with a trailing newline is not
// rejected — the cap is checked AFTER trimming, not before) plus one sentinel
// byte so a genuinely oversized input still exceeds maxPasswordBytes once
// trimmed and is reported as such rather than silently truncated.
const readLimit = maxPasswordBytes + 3

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
			raw, err = io.ReadAll(io.LimitReader(cmd.InOrStdin(), readLimit))
		}
	} else {
		// Bounded via os.Open + LimitReader rather than os.ReadFile: the latter
		// reads the whole file into memory before any size check runs, so
		// --password-file /dev/zero (or any oversized/unbounded file) would
		// materialize in full before being rejected.
		f, oerr := os.Open(file)
		if oerr != nil {
			return "", false, fmt.Errorf("reading password: %w", oerr)
		}
		raw, err = io.ReadAll(io.LimitReader(f, readLimit))
		f.Close()
	}
	if err != nil {
		return "", false, fmt.Errorf("reading password: %w", err)
	}

	// The size cap is enforced AFTER trimming trailing CRLF: checking the raw
	// byte count first would reject a legitimate maxPasswordBytes-length
	// password that happens to end in a newline (the common case for both a
	// piped secret and a file written by `echo`).
	pw := strings.TrimSuffix(strings.TrimSuffix(string(raw), "\n"), "\r")
	if len(pw) > maxPasswordBytes {
		return "", false, fmt.Errorf("password exceeds %d bytes", maxPasswordBytes)
	}
	if pw == "" {
		return "", false, fmt.Errorf("password is empty")
	}
	return pw, false, nil
}
