//go:build !windows

package cmdutil

import (
	"syscall"

	"golang.org/x/term"
)

// IsInteractive reports whether stdin is a terminal.
func IsInteractive() bool {
	return term.IsTerminal(syscall.Stdin)
}

// ReadPassword reads a password from stdin without echoing.
func ReadPassword() ([]byte, error) {
	return term.ReadPassword(syscall.Stdin)
}
