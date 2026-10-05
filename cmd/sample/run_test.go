package sample

import (
	"strings"
	"testing"
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
