package cmdutil

import (
	"errors"
	"fmt"

	"github.com/Bandwidth/cli/internal/api"
	"github.com/Bandwidth/cli/internal/config"
)

// FeatureLimitError indicates that a command failed because the active
// account or credential lacks the role/feature needed to complete it.
// It is mapped to ExitConflict (4) by ExitCodeForError, so an agent can
// branch on "stop and tell the user" without conflating these failures
// with true auth errors (exit 2).
//
// Wraps the underlying error so errors.As(err, &apiErr) still finds the
// original *api.APIError when callers want the raw status code or body.
type FeatureLimitError struct {
	msg   string
	cause error
}

func (e *FeatureLimitError) Error() string { return e.msg }
func (e *FeatureLimitError) Unwrap() error { return e.cause }

// NewFeatureLimit wraps a richer, endpoint-specific message in the typed
// error. Use this from existing wrappers that already produce tailored
// guidance for a 403 (e.g. tendlc, tfv, shortcode) so the exit code
// becomes 4 without changing the displayed text.
func NewFeatureLimit(msg string, cause error) error {
	return &FeatureLimitError{msg: msg, cause: cause}
}

// Wrap403 inspects err and, when it is a 403, returns a FeatureLimitError
// shaped to the active profile.
//
// On Build (express) accounts, the message points at the plan limit and
// the upgrade path. On other accounts, it tells the user which role to
// request from their Bandwidth account manager.
//
// `feature` is a human noun phrase describing what the user was trying
// to do (e.g. "VCPs", "phone number search"). `role` is the role to
// suggest for non-Build users; pass "" if unknown.
//
// Non-403 errors pass through as `<feature>: <err>` so they retain their
// original status code for ExitCodeForError to interpret.
func Wrap403(err error, feature, role string) error {
	var apiErr *api.APIError
	if !errors.As(err, &apiErr) || apiErr.StatusCode != 403 {
		return fmt.Errorf("%s: %w", feature, err)
	}

	if ActiveExpress() {
		return NewFeatureLimit(fmt.Sprintf("%s: Bandwidth Build accounts are voice-only — this requires a full Bandwidth account.\n"+
			"Talk to an expert: https://www.bandwidth.com/talk-to-an-expert/",
			feature), err)
	}

	if role != "" {
		return NewFeatureLimit(fmt.Sprintf("%s: credential lacks the %s role.\n"+
			"Contact your Bandwidth account manager to assign this role.",
			feature, role), err)
	}
	return NewFeatureLimit(fmt.Sprintf("%s: credential lacks the required role for this operation.\n"+
		"Contact your Bandwidth account manager.", feature), err)
}

// ActiveExpress reports whether the active profile is a Bandwidth Build
// account. Returns false on any config-load failure — best-effort, used
// only to shape error messages.
func ActiveExpress() bool {
	p := loadActiveProfile()
	return p != nil && p.Express
}

func loadActiveProfile() *config.Profile {
	path, err := config.DefaultPath()
	if err != nil {
		return nil
	}
	cfg, err := config.Load(path)
	if err != nil {
		return nil
	}
	return cfg.ActiveProfileConfig()
}
