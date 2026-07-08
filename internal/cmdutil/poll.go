package cmdutil

import (
	"errors"
	"fmt"
	"time"
)

// ErrPollTimeout is returned (wrapped) by Poll when the timeout expires
// before the checked condition is met. Callers can errors.Is against it to
// distinguish "gave up waiting" from a hard failure; ExitCodeForError maps
// it to ExitTimeout (5).
var ErrPollTimeout = errors.New("timed out")

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
			return nil, fmt.Errorf("%w after %s waiting for operation to complete", ErrPollTimeout, cfg.Timeout)
		}
		time.Sleep(cfg.Interval)
	}
}
