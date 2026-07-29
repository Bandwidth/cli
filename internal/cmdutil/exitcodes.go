package cmdutil

import (
	"errors"

	"github.com/Bandwidth/cli/internal/api"
)

// Exit code constants for the bw CLI.
const (
	ExitOK        = 0
	ExitGeneral   = 1
	ExitAuth      = 2
	ExitNotFound  = 3
	ExitConflict  = 4
	ExitTimeout   = 5
	ExitFlagError = 6
	ExitRateLimit = 7
	// ExitSecretUnavailable signals that a resource exists but its secret cannot
	// be recovered — an agent must not proceed as though provisioning succeeded.
	ExitSecretUnavailable = 8
)

// SecretUnavailableError reports that a credential exists but its password is
// unrecoverable, so the caller cannot use it.
type SecretUnavailableError struct{ Message string }

func (e *SecretUnavailableError) Error() string { return e.Message }

// ConflictError reports that the target resource exists but is not in a state
// where the requested operation can succeed — a duplicate, a wrong lifecycle
// state, a mismatched existing setting. It maps to ExitConflict (4).
//
// It exists because the alternative — deriving 4 from the HTTP status — is
// unreliable: the same logical conflict is returned as 409, 400, or even 201
// by different Bandwidth endpoints, and client-side conflicts have no status at
// all. Agents branch on exit 4, so the code must follow the *meaning*, not the
// transport.
type ConflictError struct {
	Message string
	// Cause, when non-nil, is preserved through Unwrap so callers that need the
	// original fault (its error code or status) can still reach it.
	Cause error
}

func (e *ConflictError) Error() string { return e.Message }
func (e *ConflictError) Unwrap() error { return e.Cause }

// ExitCodeForError maps an error to the appropriate exit code.
// FeatureLimitError takes precedence over the raw API status code so a
// 403 caused by a plan/role limit maps to ExitConflict (4) rather than
// ExitAuth (2) — agents can then distinguish "stop, escalate" from
// "re-auth or retry." ConflictError takes precedence for the same reason: a
// state conflict must exit 4 even when the API reported it as a 400 or a 201.
// All other errors fall back to status-code mapping, then ExitGeneral.
func ExitCodeForError(err error) int {
	if err == nil {
		return ExitOK
	}
	if errors.Is(err, ErrPollTimeout) {
		return ExitTimeout
	}
	var fle *FeatureLimitError
	if errors.As(err, &fle) {
		return ExitConflict
	}
	var sue *SecretUnavailableError
	if errors.As(err, &sue) {
		return ExitSecretUnavailable
	}
	var ce *ConflictError
	if errors.As(err, &ce) {
		return ExitConflict
	}
	var apiErr *api.APIError
	if errors.As(err, &apiErr) {
		switch apiErr.StatusCode {
		case 401, 403:
			return ExitAuth
		case 402:
			return ExitConflict
		case 404:
			return ExitNotFound
		case 409:
			return ExitConflict
		case 429:
			return ExitRateLimit
		}
	}
	return ExitGeneral
}
