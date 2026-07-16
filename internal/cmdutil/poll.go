package cmdutil

import (
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
func Poll(cfg PollConfig) (interface{}, error) {
	deadline := time.Now().Add(cfg.Timeout)
	for {
		done, result, err := cfg.Check()
		if err != nil {
			return nil, err
		}
		if done {
			return result, nil
		}
		if time.Now().After(deadline) {
			return nil, fmt.Errorf("timed out after %s: %w", cfg.Timeout, ErrPollTimeout)
		}
		time.Sleep(cfg.Interval)
	}
}
