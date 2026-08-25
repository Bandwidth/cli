package customerprofile

import (
	"errors"

	"github.com/spf13/cobra"

	"github.com/Bandwidth/cli/internal/api"
	"github.com/Bandwidth/cli/internal/cmdutil"
)

// roleGateError maps a 403 from the customer-profile endpoints onto an
// actionable exit 4, instead of letting it fall through to ExitCodeForError's
// default 401/403 -> ExitAuth (2) mapping.
//
// The common cause of a 403 here is that the active credential lacks the
// Customer Profiles Access role (see AGENTS.md's Customer Profiles section) —
// re-authenticating will not fix that, so exit 2's "reauth" signal would send
// an agent down a dead end. Modeled on cmd/tendlc/helpers.go's roleGateError,
// which solved the same problem for 10DLC.
//
// Non-403 errors, and errors that are not (or no longer, once already
// wrapped by something like conflictHint) an *api.APIError, pass through
// unchanged.
func roleGateError(err error) error {
	var apiErr *api.APIError
	if err == nil || !errors.As(err, &apiErr) || apiErr.StatusCode != 403 {
		return err
	}
	return cmdutil.NewFeatureLimit(
		"your credentials don't have the Customer Profiles Access role.\n"+
			"Contact your Bandwidth account manager to have it assigned to your API user — retrying will not help.",
		err)
}

// warnIfTruncated tells the caller on stderr when more records exist than the
// page just returned. stdout stays clean so a pipeline sees only data. Shared
// by 'list' (noun "profiles") and 'history list' (noun "versions") — they
// differ only in which offset they paginate on and what they call a record.
func warnIfTruncated(cmd *cobra.Command, env *api.Envelope, offset, returned int, noun string) {
	if env.Page != nil && env.Page.Truncated(offset+returned) {
		cmd.PrintErrf("showing %d of %d %s; pass --all to fetch every page\n",
			returned, env.Page.TotalElements, noun)
	}
}

// requireConfirm enforces a --confirm gate before any HTTP request is made.
// Centralized so a future destructive command reuses the same
// zero-request-on-refusal guarantee instead of reimplementing the check
// inline next to its own service(cmd) call — and so the gate itself is
// testable independent of any one command's wiring.
func requireConfirm(confirm bool, message string) error {
	if confirm {
		return nil
	}
	return cmdutil.NewFlagError(message)
}
