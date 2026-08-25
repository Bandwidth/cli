package cmdutil

import (
	"context"
	"errors"
	"fmt"
	"time"
)

// ErrPollTimeout is returned (wrapped) by Poll when cfg.Timeout elapses before
// Check reports done. ExitCodeForError maps it to ExitTimeout (5) so agents can
// distinguish "still running, re-poll" from a hard failure.
var ErrPollTimeout = errors.New("operation did not complete in time")

// PollConfig configures a polling loop.
type PollConfig struct {
	// Context cancels the poll loop. Optional — nil means context.Background().
	// Existing callers omit it and keep the previous behavior exactly.
	Context  context.Context
	Interval time.Duration
	Timeout  time.Duration
	// Check performs one poll attempt. It should return done=true when the
	// desired condition is met, along with the final result. Return an error
	// only for hard failures (not for "not ready yet").
	Check func() (done bool, result interface{}, err error)
}

// Poll runs cfg.Check repeatedly at cfg.Interval until it returns done=true or
// cfg.Timeout is exceeded. On success it returns the result from Check.
// On timeout it returns ErrPollTimeout.
//
// cfg.Timeout bounds when polling stops, not total wall-clock duration: no
// check starts after the deadline, and the sleep before the deadline is
// capped so the final check lands on the deadline rather than a full
// interval past it. A check that starts on time and completes successfully
// returns its result even if it finishes after the deadline — the operation
// genuinely completed, and discarding that would be worse than being
// slightly late. Total overshoot is bounded by one in-flight request.
func Poll(cfg PollConfig) (interface{}, error) {
	ctx := cfg.Context
	if ctx == nil {
		ctx = context.Background()
	}
	deadline := time.Now().Add(cfg.Timeout)

	for {
		done, result, err := cfg.Check()
		if err != nil {
			return nil, err
		}
		if done {
			return result, nil
		}
		remaining := time.Until(deadline)
		if remaining <= 0 {
			return nil, fmt.Errorf("timed out after %s: %w", cfg.Timeout, ErrPollTimeout)
		}
		wait := cfg.Interval
		if remaining < wait {
			wait = remaining
		}
		// A fresh timer per iteration. Go 1.23+ made timer channels
		// unbuffered and Stop() cancels any in-flight send, so there is
		// nothing left to drain after Stop() returns — draining an
		// already-stopped timer's channel would just block forever.
		// Stop() on the cancellation path is a courtesy; an abandoned
		// timer is garbage collected once unreferenced, so there is no
		// leak either way.
		timer := time.NewTimer(wait)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil, ctx.Err()
		case <-timer.C:
		}
	}
}
