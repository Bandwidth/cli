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
		{status: 400, body: errorBodyNaming("/website")},
		{status: 202, body: `{"data":{"bandwidthId":"BEXMPL1"}}`},
	})

	// "website" is deliberately a real, caller-settable brand field (see
	// brandFlagToField), not an invented name like the old "legacyFlag" this
	// test used to use. Every prior fixture in this file named a field no
	// caller could ever actually set, which is exactly why the retry's
	// blindness to caller-set fields went uncaught: nothing here exercised
	// that shape. This test still passes a nil neverDrop, i.e. it simulates a
	// caller who did NOT ask about website this call — some other field was
	// changed, and website merely rode along from the read-modify-write and
	// happened to be named in the error. Dropping it here is correct;
	// TestPutRetry_CallerSetField_NoRetry below is its mirror image, where
	// website WAS what the caller asked to set.
	body := map[string]any{"displayName": "Acme", "website": "not a url"}
	var raw []byte
	var err error
	stderr := captureStderr(t, func() {
		raw, err = putReplaceWithReadOnlyRetry(client, "/thing/1", body, nil)
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
	if _, present := second["website"]; present {
		t.Errorf("second request body = %v, want website stripped", second)
	}
	if second["displayName"] != "Acme" {
		t.Errorf("second request body = %v, want displayName preserved", second)
	}
	if !strings.Contains(stderr, "website") {
		t.Errorf("stderr = %q, want it to name the dropped field", stderr)
	}

	// The retry must not mutate the caller's own body map.
	if _, present := body["website"]; !present {
		t.Error("caller's body map was mutated; website should still be present in the original map")
	}
}

// TestPutRetry_CallerSetField_NoRetry is the mirror image of
// TestPutRetry_ReadOnlyFieldWeSent_RetriesOnceAndSucceeds: same field name,
// same 400, but this time neverDrop marks "website" as a field the caller
// explicitly asked this call to set (as UpdateBrand would, via
// changedBrandJSONFields). This is the CRITICAL data-loss shape: "band
// tendlc brand update BEXMPL1 --website 'not a url'" must surface the API's
// own rejection of the caller's value, never silently drop --website and
// report success with the brand's site cleared.
func TestPutRetry_CallerSetField_NoRetry(t *testing.T) {
	client, _, count := newRetryStub(t, []retryStubResponse{
		{status: 400, body: errorBodyNaming("/website")},
	})

	body := map[string]any{"displayName": "Acme", "website": "not a url"}
	_, err := putReplaceWithReadOnlyRetry(client, "/thing/1", body, map[string]bool{"website": true})

	if err == nil {
		t.Fatal("want an error, got nil")
	}
	apiErr, ok := err.(*api.APIError)
	if !ok || apiErr.StatusCode != 400 {
		t.Fatalf("err = %v, want the original 400 *api.APIError", err)
	}
	if !strings.Contains(apiErr.Body, "website") {
		t.Errorf("err body = %q, want it to still name website", apiErr.Body)
	}
	if *count != 1 {
		t.Errorf("request count = %d, want exactly 1 (no retry — the caller set this field)", *count)
	}
}

// TestPutRetry_CampaignCallerSetField_NoRetry is the campaign-path twin of
// TestPutRetry_CallerSetField_NoRetry: "description" is a real, caller-
// settable campaign field (see campaignUpdateFlagToField), and neverDrop
// simulates UpdateCampaign's changedCampaignJSONFields marking it as changed
// this call.
func TestPutRetry_CampaignCallerSetField_NoRetry(t *testing.T) {
	client, _, count := newRetryStub(t, []retryStubResponse{
		{status: 400, body: errorBodyNaming("/description")},
	})

	body := map[string]any{"campaignName": "Acme Alerts", "description": ""}
	_, err := putReplaceWithReadOnlyRetry(client, "/thing/1", body, map[string]bool{"description": true})

	if err == nil {
		t.Fatal("want an error, got nil")
	}
	if *count != 1 {
		t.Errorf("request count = %d, want exactly 1 (no retry — the caller set this field)", *count)
	}
}

// TestPutRetry_SubscriberOptin_NoRetry is the measured real-world shape: a
// direct campaign holding false for subscriberOptin returns 400 "is
// required" naming it (see campaignNeverDropFields), even though no update
// flag can ever set it and the caller changed something unrelated
// (description). Before this guard, that 400 named a field present in body
// and was indistinguishable from the retry's intended case, so the retry
// silently stripped the compliance attestation and reported success.
// neverDrop here is exactly what UpdateCampaign builds: changedCampaignJSONFields
// (which cannot include subscriberOptin — no flag ever writes it) unioned
// with campaignNeverDropFields.
func TestPutRetry_SubscriberOptin_NoRetry(t *testing.T) {
	client, _, count := newRetryStub(t, []retryStubResponse{
		{status: 400, body: errorBodyNaming("/subscriberOptin")},
	})

	body := map[string]any{"description": "Updated description", "subscriberOptin": false}
	neverDrop := changedCampaignJSONFields(map[string]bool{"description": true})
	for f := range campaignNeverDropFields {
		neverDrop[f] = true
	}
	_, err := putReplaceWithReadOnlyRetry(client, "/thing/1", body, neverDrop)

	if err == nil {
		t.Fatal("want an error, got nil")
	}
	if !strings.Contains(err.Error(), "subscriberOptin") {
		t.Errorf("err = %v, want it to still name subscriberOptin", err)
	}
	if *count != 1 {
		t.Errorf("request count = %d, want exactly 1 (no retry — subscriberOptin is never droppable)", *count)
	}
}

func TestPutRetry_FieldNotSent_PassesThroughNoSecondRequest(t *testing.T) {
	client, _, count := newRetryStub(t, []retryStubResponse{
		{status: 400, body: errorBodyNaming("/neverSent")},
	})

	body := map[string]any{"displayName": "Acme"}
	_, err := putReplaceWithReadOnlyRetry(client, "/thing/1", body, nil)

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
	_, err := putReplaceWithReadOnlyRetry(client, "/thing/1", body, nil)

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
	_, err := putReplaceWithReadOnlyRetry(client, "/thing/1", body, nil)

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
	_, err := putReplaceWithReadOnlyRetry(client, "/thing/1", body, nil)

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
	_, err := putReplaceWithReadOnlyRetry(client, "/thing/1", body, nil)

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
		_, err = putReplaceWithReadOnlyRetry(client, "/thing/1", body, nil)
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
