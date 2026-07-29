// This file is an EXTERNAL test package (cmdutil_test) rather than an
// in-package one. It asserts that *sip.APIFault and the SIP service's
// ConflictError map onto the exit-code taxonomy, which requires importing
// internal/sip — and internal/sip imports cmdutil for ConflictError. An
// in-package test would therefore form a test-only import cycle. Everything
// asserted here is exported API, so nothing is lost.
package cmdutil_test

import (
	"errors"
	"fmt"
	"testing"

	"github.com/Bandwidth/cli/internal/api"
	"github.com/Bandwidth/cli/internal/cmdutil"
	"github.com/Bandwidth/cli/internal/sip"
)

func TestExitCodeForError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want int
	}{
		{"nil error", nil, cmdutil.ExitOK},
		{"plain error", errors.New("boom"), cmdutil.ExitGeneral},
		{"401", &api.APIError{StatusCode: 401}, cmdutil.ExitAuth},
		{"403", &api.APIError{StatusCode: 403}, cmdutil.ExitAuth},
		{"402 payment required", &api.APIError{StatusCode: 402, Body: "insufficient credits"}, cmdutil.ExitConflict},
		{"404", &api.APIError{StatusCode: 404}, cmdutil.ExitNotFound},
		{"409", &api.APIError{StatusCode: 409}, cmdutil.ExitConflict},
		{"429 rate limited", &api.APIError{StatusCode: 429}, cmdutil.ExitRateLimit},
		{"500", &api.APIError{StatusCode: 500}, cmdutil.ExitGeneral},
		{"feature limit wraps 403", cmdutil.NewFeatureLimit("nope", &api.APIError{StatusCode: 403}), cmdutil.ExitConflict},
		{"feature limit precedence beats raw 401", cmdutil.NewFeatureLimit("nope", &api.APIError{StatusCode: 401}), cmdutil.ExitConflict},
		{"wrapped 429 keeps rate limit", fmt.Errorf("wrap: %w", &api.APIError{StatusCode: 429}), cmdutil.ExitRateLimit},
		{"wrapped ErrPollTimeout", fmt.Errorf("timed out: %w", cmdutil.ErrPollTimeout), cmdutil.ExitTimeout},
		// *sip.APIFault must unwrap to *api.APIError so a documented SIP
		// failure (one that carries an ErrorCode) maps onto the CLI's
		// exit-code taxonomy instead of falling back to ExitGeneral.
		{"sip APIFault 404", &sip.APIFault{Code: "33010", Description: "not found", StatusCode: 404}, cmdutil.ExitNotFound},
		{"sip APIFault 409", &sip.APIFault{Code: "23026", Description: "does already exist", StatusCode: 409}, cmdutil.ExitConflict},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := cmdutil.ExitCodeForError(tt.err)
			if got != tt.want {
				t.Errorf("ExitCodeForError(%v) = %d, want %d", tt.err, got, tt.want)
			}
		})
	}
}

func TestExitCodeForError_SecretUnavailable(t *testing.T) {
	err := &cmdutil.SecretUnavailableError{Message: "credential exists but its password cannot be recovered"}
	if got := cmdutil.ExitCodeForError(err); got != cmdutil.ExitSecretUnavailable {
		t.Errorf("ExitCodeForError() = %d, want %d", got, cmdutil.ExitSecretUnavailable)
	}
	if cmdutil.ExitSecretUnavailable != 8 {
		t.Errorf("ExitSecretUnavailable = %d, want 8", cmdutil.ExitSecretUnavailable)
	}
}

// TestExitCodeForError_Conflict is the load-bearing assertion for every
// client-side state conflict in `band sip` / `band vcp`: a ConflictError must
// exit 4 no matter what (if anything) it wraps. The wrapped cases matter most —
// a state conflict the API reported as a 400 (23022, 23026) or as a 201 body
// (bulk-create Errors envelope) previously fell through to ExitGeneral (1),
// which an agent cannot distinguish from an unexpected failure.
func TestExitCodeForError_Conflict(t *testing.T) {
	tests := []struct {
		name string
		err  error
	}{
		{"bare client-side conflict", &cmdutil.ConflictError{Message: "realm is CREATE_PENDING"}},
		{"wraps a 400 fault", &cmdutil.ConflictError{
			Message: "realm is not active yet",
			Cause:   &sip.APIFault{Code: "23022", Description: "not active", StatusCode: 400},
		}},
		{"wraps a 201 partial-success fault", &cmdutil.ConflictError{
			Message: "credential already exists",
			Cause:   &sip.APIFault{Code: "23026", Description: "does already exist", StatusCode: 201},
		}},
		{"wraps a 409 fault", &cmdutil.ConflictError{
			Message: "realm already exists",
			Cause:   &sip.APIFault{Code: "33002", Description: "exists", StatusCode: 409},
		}},
		{"wrapped by fmt.Errorf", fmt.Errorf("context: %w", &cmdutil.ConflictError{Message: "conflict"})},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := cmdutil.ExitCodeForError(tt.err); got != cmdutil.ExitConflict {
				t.Errorf("ExitCodeForError(%v) = %d, want ExitConflict (%d)", tt.err, got, cmdutil.ExitConflict)
			}
		})
	}
}

// TestConflictError_PreservesCauseChain guards the Unwrap contract: sites that
// previously wrapped with %w must keep the original fault reachable, so a
// caller can still read the API error code behind the conflict.
func TestConflictError_PreservesCauseChain(t *testing.T) {
	fault := &sip.APIFault{Code: "33006", Description: "cannot delete default", StatusCode: 409}
	err := error(&cmdutil.ConflictError{Message: "cannot delete the default realm", Cause: fault})

	var got *sip.APIFault
	if !errors.As(err, &got) {
		t.Fatalf("errors.As(%v, *sip.APIFault) = false, want the cause to stay reachable", err)
	}
	if got.Code != "33006" {
		t.Errorf("unwrapped Code = %q, want 33006", got.Code)
	}
}
