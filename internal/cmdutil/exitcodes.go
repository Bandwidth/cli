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
)

// ExitCodeForError maps an error to the appropriate exit code.
// API errors are mapped by HTTP status code; all other errors get ExitGeneral.
func ExitCodeForError(err error) int {
	if err == nil {
		return ExitOK
	}
	var apiErr *api.APIError
	if errors.As(err, &apiErr) {
		switch apiErr.StatusCode {
		case 401, 403:
			return ExitAuth
		case 404:
			return ExitNotFound
		case 409:
			return ExitConflict
		}
	}
	return ExitGeneral
}
