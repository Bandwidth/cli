package sample

import (
	"context"
	"strings"
	"testing"

	"github.com/Bandwidth/cli/internal/api"
	"github.com/Bandwidth/cli/internal/testutil"
)

func TestCollectExtraEnvReadsEnvironment(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "sk-test")
	t.Setenv("TRANSFER_TO", "+19195550100")

	got, err := collectExtraEnv("live-assistant", "python", catalog["live-assistant"])
	if err != nil {
		t.Fatalf("collectExtraEnv: %v", err)
	}
	if got["OPENAI_API_KEY"] != "sk-test" {
		t.Errorf("OPENAI_API_KEY = %q, want %q", got["OPENAI_API_KEY"], "sk-test")
	}
	if got["TRANSFER_TO"] != "+19195550100" {
		t.Errorf("TRANSFER_TO = %q, want %q", got["TRANSFER_TO"], "+19195550100")
	}
}

func TestCollectExtraEnvReportsMissingByEnvVarName(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "sk-test")
	t.Setenv("TRANSFER_TO", "")

	_, err := collectExtraEnv("live-assistant", "python", catalog["live-assistant"])
	if err == nil {
		t.Fatal("expected an error when TRANSFER_TO is unset, got nil")
	}
	msg := err.Error()
	if !strings.Contains(msg, "TRANSFER_TO") {
		t.Errorf("error should name the missing variable, got: %s", msg)
	}
	if strings.Contains(msg, "OPENAI_API_KEY   (") {
		t.Errorf("error should not list variables that are set, got: %s", msg)
	}
	// Extra configuration is environment-only; the message must not send users
	// back to a flag that no longer exists.
	for _, gone := range []string{"--openai-key", "--transfer-to"} {
		if strings.Contains(msg, gone) {
			t.Errorf("error should not suggest removed flag %s, got: %s", gone, msg)
		}
	}
	// The hint should name the variable that is actually missing.
	if !strings.Contains(msg, "TRANSFER_TO=...") {
		t.Errorf("hint should show the missing variable, got: %s", msg)
	}
	if strings.Contains(msg, "OPENAI_API_KEY=...") {
		t.Errorf("hint should not include variables that are already set, got: %s", msg)
	}
}

func TestCollectExtraEnvNoRequirements(t *testing.T) {
	got, err := collectExtraEnv("voice-gather", "python", catalog["voice-gather"])
	if err != nil {
		t.Fatalf("collectExtraEnv: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("got %v, want empty map", got)
	}
}

func TestCollectExtraEnvReportsMissingForVoiceBridge(t *testing.T) {
	t.Setenv("USER_NUMBER", "")

	_, err := collectExtraEnv("voice-bridge", "python", catalog["voice-bridge"])
	if err == nil {
		t.Fatal("expected an error when USER_NUMBER is unset, got nil")
	}
	msg := err.Error()
	if !strings.Contains(msg, "USER_NUMBER") {
		t.Errorf("error should name the missing variable, got: %s", msg)
	}
	if !strings.Contains(msg, "USER_NUMBER=...") {
		t.Errorf("hint should show the missing variable, got: %s", msg)
	}
}

type failingRequester struct {
	getErr  error
	postErr error
}

func (f failingRequester) Get(context.Context, string, interface{}) error { return f.getErr }
func (f failingRequester) Post(context.Context, string, interface{}, interface{}) error {
	return f.postErr
}
func (f failingRequester) Put(context.Context, string, interface{}, interface{}) error { return nil }
func (f failingRequester) Patch(context.Context, string, interface{}, interface{}) error {
	return nil
}
func (f failingRequester) Delete(context.Context, string, interface{}) error { return nil }
func (f failingRequester) GetRaw(context.Context, string) ([]byte, error)    { return nil, nil }
func (f failingRequester) PutRaw(context.Context, string, []byte, string) error {
	return nil
}

func TestEnsureVoiceAppOnNumberLocation_403WarnsAndContinues(t *testing.T) {
	req := failingRequester{getErr: &api.APIError{StatusCode: 403, Body: "Forbidden"}}

	stderr := testutil.CaptureStderr(t, func() {
		ensureVoiceAppOnNumberLocationBestEffort(context.Background(), req, "acct", "+19404060874", "", "", "app-id", "band-sample-voice-record")
	})

	if !strings.Contains(stderr, "Could not auto-assign voice routing") {
		t.Fatalf("expected warning, got: %s", stderr)
	}
	if !strings.Contains(stderr, "band-sample-voice-record") {
		t.Fatalf("expected app name in warning, got: %s", stderr)
	}
	if !strings.Contains(stderr, "Bandwidth Dashboard") {
		t.Fatalf("expected manual remediation hint, got: %s", stderr)
	}
}
