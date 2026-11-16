package sample

import (
	"context"
	"encoding/json"
	"os"
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

type recordingRequester struct {
	getByPath map[string]interface{}
	putCalls  []struct {
		path string
		body interface{}
	}
}

func (r *recordingRequester) Get(_ context.Context, path string, result interface{}) error {
	raw, ok := r.getByPath[path]
	if !ok {
		return &api.APIError{StatusCode: 404, Body: "not found"}
	}
	b, _ := json.Marshal(raw)
	return json.Unmarshal(b, result)
}
func (r *recordingRequester) Post(context.Context, string, interface{}, interface{}) error {
	return nil
}
func (r *recordingRequester) Put(_ context.Context, path string, body, _ interface{}) error {
	r.putCalls = append(r.putCalls, struct {
		path string
		body interface{}
	}{path: path, body: body})
	return nil
}
func (r *recordingRequester) Patch(context.Context, string, interface{}, interface{}) error {
	return nil
}
func (r *recordingRequester) Delete(context.Context, string, interface{}) error    { return nil }
func (r *recordingRequester) GetRaw(context.Context, string) ([]byte, error)       { return nil, nil }
func (r *recordingRequester) PutRaw(context.Context, string, []byte, string) error { return nil }

func TestResolveInboundVoiceApp_FallsBackToDefaultHTTPVoice(t *testing.T) {
	client := &recordingRequester{
		getByPath: map[string]interface{}{
			"/accounts/acct/applications": []interface{}{
				map[string]interface{}{
					"ApplicationId": "app-default",
					"ServiceType":   "Voice-V2",
					"AppName":       "default-http-voice",
				},
			},
			"/accounts/acct/applications/app-default": map[string]interface{}{
				"ApplicationResponse": map[string]interface{}{
					"Application": map[string]interface{}{
						"ApplicationId":            "app-default",
						"ServiceType":              "Voice-V2",
						"AppName":                  "default-http-voice",
						"CallInitiatedCallbackUrl": "https://orig.example/cb",
						"CallStatusCallbackUrl":    "https://orig.example/status",
					},
				},
			},
		},
	}

	id, name, err := resolveInboundVoiceApp(context.Background(), client, "acct", sampleNumberInfo{PhoneNumber: "+10000000000"}, "band-sample-voice-record", "")
	if err != nil {
		t.Fatalf("resolveInboundVoiceApp: %v", err)
	}
	if id != "app-default" {
		t.Fatalf("id=%q want %q", id, "app-default")
	}
	if name != "default-http-voice" {
		t.Fatalf("name=%q want %q", name, "default-http-voice")
	}
}

func TestResolveInboundVoiceApp_SingleNonSampleVoiceAppWins(t *testing.T) {
	client := &recordingRequester{
		getByPath: map[string]interface{}{
			"/accounts/acct/applications": []interface{}{
				map[string]interface{}{
					"ApplicationId": "app-sample",
					"ServiceType":   "Voice-V2",
					"AppName":       "band-sample-voice-record",
				},
				map[string]interface{}{
					"ApplicationId": "app-only",
					"ServiceType":   "Voice-V2",
					"AppName":       "some-voice-app",
				},
			},
			"/accounts/acct/applications/app-only": map[string]interface{}{
				"ApplicationResponse": map[string]interface{}{
					"Application": map[string]interface{}{
						"ApplicationId":            "app-only",
						"ServiceType":              "Voice-V2",
						"AppName":                  "some-voice-app",
						"CallInitiatedCallbackUrl": "https://orig.example/cb",
					},
				},
			},
		},
	}

	id, name, err := resolveInboundVoiceApp(context.Background(), client, "acct", sampleNumberInfo{PhoneNumber: "+10000000000"}, "band-sample-voice-record", "")
	if err != nil {
		t.Fatalf("resolveInboundVoiceApp: %v", err)
	}
	if id != "app-only" {
		t.Fatalf("id=%q want %q", id, "app-only")
	}
	if name != "some-voice-app" {
		t.Fatalf("name=%q want %q", name, "some-voice-app")
	}
}

func TestResolveInboundVoiceApp_FailureHasDiagnostics(t *testing.T) {
	client := &recordingRequester{
		getByPath: map[string]interface{}{
			"/accounts/acct/applications": []interface{}{
				map[string]interface{}{
					"ApplicationId": "app-one",
					"ServiceType":   "Voice-V2",
					"AppName":       "voice-one",
				},
				map[string]interface{}{
					"ApplicationId": "app-two",
					"ServiceType":   "Voice-V2",
					"AppName":       "voice-two",
				},
			},
		},
	}

	_, err := func() (string, error) {
		_, _, err := resolveInboundVoiceApp(context.Background(), client, "acct", sampleNumberInfo{PhoneNumber: "+10000000000"}, "band-sample-voice-record", "")
		return "", err
	}()
	if err == nil {
		t.Fatal("expected error")
	}
	msg := err.Error()
	if !strings.Contains(msg, "Voice-V2 applications found") {
		t.Fatalf("expected diagnostics listing apps, got: %s", msg)
	}
	if !strings.Contains(msg, "voice-one") || !strings.Contains(msg, "app-one") {
		t.Fatalf("expected app list in diagnostics, got: %s", msg)
	}
	if !strings.Contains(msg, "Lookups attempted") {
		t.Fatalf("expected lookup notes in diagnostics, got: %s", msg)
	}
	if !strings.Contains(msg, "--voice-app") || !strings.Contains(msg, "BW_SAMPLE_VOICE_APP_ID") {
		t.Fatalf("expected override hint in diagnostics, got: %s", msg)
	}
}

func TestOverrideVoiceAppCallbacks_RestoresAndCleansState(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	client := &recordingRequester{
		getByPath: map[string]interface{}{
			"/accounts/acct/applications/app-default": map[string]interface{}{
				"ApplicationResponse": map[string]interface{}{
					"Application": map[string]interface{}{
						"ApplicationId":            "app-default",
						"ServiceType":              "Voice-V2",
						"AppName":                  "default-http-voice",
						"CallInitiatedCallbackUrl": "https://orig.example/cb",
						"CallStatusCallbackUrl":    "https://orig.example/status",
					},
				},
			},
		},
	}

	statePath, err := voiceAppOverrideStatePath("acct", "app-default")
	if err != nil {
		t.Fatalf("voiceAppOverrideStatePath: %v", err)
	}

	restore, err := overrideVoiceAppCallbacks(context.Background(), client, "acct", "app-default", "default-http-voice", "https://ngrok.example/callbacks/inbound/voice", "", statePath)
	if err != nil {
		t.Fatalf("overrideVoiceAppCallbacks: %v", err)
	}
	if _, err := os.Stat(statePath); err != nil {
		t.Fatalf("expected state file to exist: %v", err)
	}
	if len(client.putCalls) != 1 {
		t.Fatalf("expected 1 Put call, got %d", len(client.putCalls))
	}

	if err := restore(context.Background()); err != nil {
		t.Fatalf("restore: %v", err)
	}
	if len(client.putCalls) != 2 {
		t.Fatalf("expected 2 Put calls after restore, got %d", len(client.putCalls))
	}
	if _, err := os.Stat(statePath); err == nil {
		t.Fatalf("expected state file to be removed")
	}
}
