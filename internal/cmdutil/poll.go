package cmdutil

import (
	"fmt"
	"time"
)

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
			return nil, fmt.Errorf("timed out after %s waiting for operation to complete", cfg.Timeout)
		}
		time.Sleep(cfg.Interval)
	}
}
