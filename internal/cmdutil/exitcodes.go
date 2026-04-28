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
)

// ExitCodeForError maps an error to the appropriate exit code.
// FeatureLimitError takes precedence over the raw API status code so a
// 403 caused by a plan/role limit maps to ExitConflict (4) rather than
// ExitAuth (2) — agents can then distinguish "stop, escalate" from
// "re-auth or retry."
// All other errors fall back to status-code mapping, then ExitGeneral.
func ExitCodeForError(err error) int {
	if err == nil {
		return ExitOK
	}
	var fle *FeatureLimitError
	if errors.As(err, &fle) {
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
