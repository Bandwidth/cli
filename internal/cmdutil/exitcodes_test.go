package cmdutil

import (
	"errors"
	"fmt"
	"testing"

	"github.com/Bandwidth/cli/internal/api"
)

func TestExitCodeForError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want int
	}{
		{"nil error", nil, ExitOK},
		{"plain error", errors.New("boom"), ExitGeneral},
		{"401", &api.APIError{StatusCode: 401}, ExitAuth},
		{"403", &api.APIError{StatusCode: 403}, ExitAuth},
		{"402 payment required", &api.APIError{StatusCode: 402, Body: "insufficient credits"}, ExitConflict},
		{"404", &api.APIError{StatusCode: 404}, ExitNotFound},
		{"409", &api.APIError{StatusCode: 409}, ExitConflict},
		{"429 rate limited", &api.APIError{StatusCode: 429}, ExitRateLimit},
		{"500", &api.APIError{StatusCode: 500}, ExitGeneral},
		{"feature limit wraps 403", NewFeatureLimit("nope", &api.APIError{StatusCode: 403}), ExitConflict},
		{"feature limit precedence beats raw 401", NewFeatureLimit("nope", &api.APIError{StatusCode: 401}), ExitConflict},
		{"wrapped 429 keeps rate limit", fmt.Errorf("wrap: %w", &api.APIError{StatusCode: 429}), ExitRateLimit},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ExitCodeForError(tt.err)
			if got != tt.want {
				t.Errorf("ExitCodeForError(%v) = %d, want %d", tt.err, got, tt.want)
			}
		})
	}
}
