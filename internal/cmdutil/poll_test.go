package cmdutil

import (
	"context"
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

func TestPollRespectsContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	calls := 0
	go func() { time.Sleep(30 * time.Millisecond); cancel() }()

	_, err := Poll(PollConfig{
		Context:  ctx,
		Interval: 10 * time.Millisecond,
		Timeout:  10 * time.Second,
		Check: func() (bool, interface{}, error) {
			calls++
			return false, nil, nil
		},
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
	// Poll always calls Check at least once before it can observe
	// cancellation, so calls == 0 could never fail here. Assert instead
	// that the loop actually iterated more than once — the interval
	// (10ms) is well under the 30ms cancellation delay, so a working
	// loop calls Check several times before ctx.Done() is observed.
	if calls < 2 {
		t.Errorf("calls = %d, want > 1 (loop should iterate before cancellation)", calls)
	}
}

// A nil Context must behave exactly as before — all existing callers omit it.
func TestPollNilContextStillTimesOut(t *testing.T) {
	_, err := Poll(PollConfig{
		Interval: 5 * time.Millisecond,
		Timeout:  20 * time.Millisecond,
		Check:    func() (bool, interface{}, error) { return false, nil, nil },
	})
	if !errors.Is(err, ErrPollTimeout) {
		t.Fatalf("err = %v, want ErrPollTimeout", err)
	}
}
