package tendlc

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/Bandwidth/cli/internal/api"
)

// retryStubResponse is one canned response in a sequence a stub server hands
// back, one per request received, in order.
type retryStubResponse struct {
	status int
	body   string
}

// newRetryStub serves responses in sequence and records each request's
// decoded JSON body plus a running count, so tests can assert on what the
// SECOND request actually sent — not just on the error the first one
// returned.
func newRetryStub(t *testing.T, responses []retryStubResponse) (client *api.Client, bodies *[]map[string]any, count *int) {
	t.Helper()
	var gotBodies []map[string]any
	n := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		var decoded map[string]any
		if len(b) > 0 {
			_ = json.Unmarshal(b, &decoded)
		}
		gotBodies = append(gotBodies, decoded)
		idx := n
		n++
		if idx >= len(responses) {
			t.Fatalf("unexpected request #%d; only %d responses configured", idx+1, len(responses))
		}
		resp := responses[idx]
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(resp.status)
		if resp.body != "" {
			_, _ = io.WriteString(w, resp.body)
		}
	}))
	t.Cleanup(srv.Close)
	return api.NewClientNoAuth(srv.URL), &gotBodies, &n
}

// captureStderr redirects os.Stderr for the duration of fn and returns what
// was written to it. Not run in parallel with other tests: os.Stderr is
// process-global.
func captureStderr(t *testing.T, fn func()) string {
	t.Helper()
	orig := os.Stderr
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	os.Stderr = w
	defer func() { os.Stderr = orig }()

	fn()

	if cerr := w.Close(); cerr != nil {
		t.Fatalf("closing pipe writer: %v", cerr)
	}
	out, _ := io.ReadAll(r)
	return string(out)
}

// errorBodyNaming builds an API error body naming the given source pointers,
// in the shape documented on the brief: {"errors":[{...,"source":{"POINTER":
// "/phone"}}],"links":[]}.
func errorBodyNaming(pointers ...string) string {
	var errs []string
	for _, p := range pointers {
		errs = append(errs, `{"type":"bad request","description":"must not be blank","source":{"POINTER":"`+p+`"}}`)
	}
	return `{"errors":[` + strings.Join(errs, ",") + `],"links":[]}`
}

func TestPutRetry_ReadOnlyFieldWeSent_RetriesOnceAndSucceeds(t *testing.T) {
	client, bodies, count := newRetryStub(t, []retryStubResponse{
		{status: 400, body: errorBodyNaming("/legacyFlag")},
		{status: 202, body: `{"data":{"bandwidthId":"BEXMPL1"}}`},
	})

	body := map[string]any{"displayName": "Acme", "legacyFlag": true}
	var raw []byte
	var err error
	stderr := captureStderr(t, func() {
		raw, err = putReplaceWithReadOnlyRetry(client, "/thing/1", body)
	})

	if err != nil {
		t.Fatalf("putReplaceWithReadOnlyRetry: %v", err)
	}
	if !strings.Contains(string(raw), "BEXMPL1") {
		t.Errorf("raw response = %s, want it to contain the success body", raw)
	}
	if *count != 2 {
		t.Fatalf("request count = %d, want exactly 2", *count)
	}
	second := (*bodies)[1]
	if _, present := second["legacyFlag"]; present {
		t.Errorf("second request body = %v, want legacyFlag stripped", second)
	}
	if second["displayName"] != "Acme" {
		t.Errorf("second request body = %v, want displayName preserved", second)
	}
	if !strings.Contains(stderr, "legacyFlag") {
		t.Errorf("stderr = %q, want it to name the dropped field", stderr)
	}

	// The retry must not mutate the caller's own body map.
	if _, present := body["legacyFlag"]; !present {
		t.Error("caller's body map was mutated; legacyFlag should still be present in the original map")
	}
}

func TestPutRetry_FieldNotSent_PassesThroughNoSecondRequest(t *testing.T) {
	client, _, count := newRetryStub(t, []retryStubResponse{
		{status: 400, body: errorBodyNaming("/neverSent")},
	})

	body := map[string]any{"displayName": "Acme"}
	_, err := putReplaceWithReadOnlyRetry(client, "/thing/1", body)

	if err == nil {
		t.Fatal("want an error, got nil")
	}
	apiErr, ok := err.(*api.APIError)
	if !ok {
		t.Fatalf("err = %T, want *api.APIError", err)
	}
	if apiErr.StatusCode != 400 {
		t.Errorf("StatusCode = %d, want 400", apiErr.StatusCode)
	}
	if *count != 1 {
		t.Errorf("request count = %d, want exactly 1 (no retry)", *count)
	}
}

func TestPutRetry_GenericBadRequest_NoUsablePointers_NoRetry(t *testing.T) {
	client, _, count := newRetryStub(t, []retryStubResponse{
		{status: 400, body: `{"errors":[{"type":"bad request","description":"must not be blank"}],"links":[]}`},
	})

	body := map[string]any{"displayName": ""}
	_, err := putReplaceWithReadOnlyRetry(client, "/thing/1", body)

	if err == nil {
		t.Fatal("want an error, got nil")
	}
	if *count != 1 {
		t.Errorf("request count = %d, want exactly 1 (no retry)", *count)
	}
}

func TestPutRetry_RetryAlsoFails_OriginalErrorSurfaces(t *testing.T) {
	client, _, count := newRetryStub(t, []retryStubResponse{
		{status: 400, body: errorBodyNaming("/legacyFlag")},
		{status: 400, body: errorBodyNaming("/somethingElse")},
	})

	body := map[string]any{"displayName": "Acme", "legacyFlag": true}
	_, err := putReplaceWithReadOnlyRetry(client, "/thing/1", body)

	if err == nil {
		t.Fatal("want an error, got nil")
	}
	if !strings.Contains(err.Error(), "legacyFlag") {
		t.Errorf("err = %v, want the ORIGINAL error (naming legacyFlag), not the retry's (naming somethingElse)", err)
	}
	if strings.Contains(err.Error(), "somethingElse") {
		t.Errorf("err = %v, want the original error only, not the retry's body", err)
	}
	if *count != 2 {
		t.Errorf("request count = %d, want exactly 2 (one retry, no more)", *count)
	}
}

func TestPutRetry_NestedPointer_NoRetry(t *testing.T) {
	client, _, count := newRetryStub(t, []retryStubResponse{
		{status: 400, body: errorBodyNaming("/accounts[0]/customerProfileId")},
	})

	body := map[string]any{"accounts": []any{map[string]any{"customerProfileId": "CEXMPL1"}}}
	_, err := putReplaceWithReadOnlyRetry(client, "/thing/1", body)

	if err == nil {
		t.Fatal("want an error, got nil")
	}
	if *count != 1 {
		t.Errorf("request count = %d, want exactly 1 (dropping the top-level key would lose caller data)", *count)
	}
}

func TestPutRetry_409_NoRetry(t *testing.T) {
	client, _, count := newRetryStub(t, []retryStubResponse{
		{status: 409, body: `{"errors":[{"type":"conflict","description":"stale version","source":{"POINTER":"/legacyFlag"}}],"links":[]}`},
	})

	body := map[string]any{"legacyFlag": true}
	_, err := putReplaceWithReadOnlyRetry(client, "/thing/1", body)

	if err == nil {
		t.Fatal("want an error, got nil")
	}
	apiErr, ok := err.(*api.APIError)
	if !ok || apiErr.StatusCode != 409 {
		t.Fatalf("err = %v, want a 409 *api.APIError", err)
	}
	if *count != 1 {
		t.Errorf("request count = %d, want exactly 1 (only a 400 triggers a retry)", *count)
	}
}

func TestPutRetry_HappyPath_NoStderrNote(t *testing.T) {
	client, _, count := newRetryStub(t, []retryStubResponse{
		{status: 202, body: `{"data":{"bandwidthId":"BEXMPL1"}}`},
	})

	body := map[string]any{"displayName": "Acme"}
	var err error
	stderr := captureStderr(t, func() {
		_, err = putReplaceWithReadOnlyRetry(client, "/thing/1", body)
	})

	if err != nil {
		t.Fatalf("putReplaceWithReadOnlyRetry: %v", err)
	}
	if *count != 1 {
		t.Errorf("request count = %d, want exactly 1", *count)
	}
	if stderr != "" {
		t.Errorf("stderr = %q, want empty on the happy path (no retry fired)", stderr)
	}
}

func TestTopLevelPointerFields(t *testing.T) {
	tests := []struct {
		name string
		body string
		want []string
	}{
		{"single top-level pointer", errorBodyNaming("/phone"), []string{"phone"}},
		{"nested pointer excluded", errorBodyNaming("/accounts[0]/customerProfileId"), nil},
		{"root pointer excluded", errorBodyNaming("/"), nil},
		{"no source at all", `{"errors":[{"description":"bad"}],"links":[]}`, nil},
		{"malformed json", `not json`, nil},
		{"duplicate pointers deduped", errorBodyNaming("/phone", "/phone"), []string{"phone"}},
		{"multiple distinct pointers", errorBodyNaming("/phone", "/email"), []string{"phone", "email"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := topLevelPointerFields(tt.body)
			sort.Strings(got)
			want := append([]string(nil), tt.want...)
			sort.Strings(want)
			if !reflect.DeepEqual(got, want) {
				t.Errorf("topLevelPointerFields(%q) = %v, want %v", tt.body, got, want)
			}
		})
	}
}
