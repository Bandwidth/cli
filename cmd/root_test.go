//go:build !windows

package cmd

import (
	"context"
	"errors"
	"syscall"
	"testing"
	"time"

	"github.com/spf13/cobra"
)

// Execute must run commands under a context that is cancelled by SIGINT, so
// --wait loops can exit through their receipt-emitting cancellation paths and
// in-flight requests abort. The test command blocks until its context ends;
// a SIGINT sent to our own process must unblock it.
func TestExecuteCancelsCommandContextOnSIGINT(t *testing.T) {
	sawCancel := make(chan error, 1)
	testCmd := newTestSignalCommand(sawCancel)
	rootCmd.AddCommand(testCmd)
	defer rootCmd.RemoveCommand(testCmd)

	rootCmd.SetArgs([]string{testCmd.Use})
	defer rootCmd.SetArgs(nil)

	execDone := make(chan error, 1)
	go func() { execDone <- Execute() }()

	// Give Execute time to install the signal handler and start the command,
	// then interrupt ourselves. The handler traps the signal, so the test
	// process survives and the command's context is cancelled.
	time.Sleep(100 * time.Millisecond)
	if err := syscall.Kill(syscall.Getpid(), syscall.SIGINT); err != nil {
		t.Fatalf("sending SIGINT: %v", err)
	}

	select {
	case err := <-sawCancel:
		if !errors.Is(err, context.Canceled) {
			t.Errorf("command context ended with %v, want context.Canceled", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("command context was not cancelled after SIGINT")
	}
	select {
	case <-execDone:
	case <-time.After(5 * time.Second):
		t.Fatal("Execute did not return after SIGINT")
	}
}

// newTestSignalCommand returns a hidden command that blocks until its context
// is cancelled and reports the context error on done.
func newTestSignalCommand(done chan<- error) *cobra.Command {
	return &cobra.Command{
		Use:    "test-signal-wait",
		Hidden: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			<-cmd.Context().Done()
			done <- cmd.Context().Err()
			return nil
		},
	}
}
