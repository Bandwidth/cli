package ui

import (
	"fmt"
	"os"
	"time"

	"github.com/briandowns/spinner"
	"github.com/fatih/color"
)

var (
	Success = color.New(color.FgGreen).SprintFunc()
	Error   = color.New(color.FgRed).SprintFunc()
	Warn    = color.New(color.FgYellow).SprintFunc()
	Muted   = color.New(color.Faint).SprintFunc()
	Bold    = color.New(color.Bold).SprintFunc()
	ID      = color.New(color.FgCyan).SprintFunc()
)

// Successf prints a green success message to stderr
func Successf(format string, a ...interface{}) {
	fmt.Fprintf(os.Stderr, "%s %s\n", Success("✓"), fmt.Sprintf(format, a...))
}

// Errorf prints a red error message to stderr
func Errorf(format string, a ...interface{}) {
	fmt.Fprintf(os.Stderr, "%s %s\n", Error("✗"), fmt.Sprintf(format, a...))
}

// Warnf prints a yellow warning message to stderr
func Warnf(format string, a ...interface{}) {
	fmt.Fprintf(os.Stderr, "%s %s\n", Warn("⚠"), fmt.Sprintf(format, a...))
}

// Infof prints a muted info message to stderr
func Infof(format string, a ...interface{}) {
	fmt.Fprintf(os.Stderr, "  %s\n", fmt.Sprintf(format, a...))
}

// Headerf prints a bold header to stderr
func Headerf(format string, a ...interface{}) {
	fmt.Fprintf(os.Stderr, "%s\n", Bold(fmt.Sprintf(format, a...)))
}

// NewSpinner creates a spinner with a message. Call .Start() and .Stop().
func NewSpinner(msg string) *spinner.Spinner {
	s := spinner.New(spinner.CharSets[14], 100*time.Millisecond, spinner.WithWriter(os.Stderr))
	s.Suffix = " " + msg
	return s
}
