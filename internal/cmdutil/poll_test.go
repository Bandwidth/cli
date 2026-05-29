package cmdutil

import (
	"errors"
	"testing"
	"time"
)

func TestPollReturnsErrPollTimeout(t *testing.T) {
	_, err := Poll(PollConfig{
		Interval: time.Millisecond,
		Timeout:  5 * time.Millisecond,
		Check: func() (bool, interface{}, error) {
			return false, nil, nil // never done
		},
	})
	if !errors.Is(err, ErrPollTimeout) {
		t.Fatalf("expected ErrPollTimeout, got %v", err)
	}
}

func TestPollReturnsResultWhenDone(t *testing.T) {
	got, err := Poll(PollConfig{
		Interval: time.Millisecond,
		Timeout:  time.Second,
		Check: func() (bool, interface{}, error) {
			return true, "done", nil
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "done" {
		t.Fatalf("got %v, want done", got)
	}
}

func TestPollPropagatesCheckError(t *testing.T) {
	sentinel := errors.New("boom")
	_, err := Poll(PollConfig{
		Interval: time.Millisecond,
		Timeout:  time.Second,
		Check: func() (bool, interface{}, error) {
			return false, nil, sentinel
		},
	})
	if !errors.Is(err, sentinel) {
		t.Fatalf("expected boom, got %v", err)
	}
}
