package tendlc

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Bandwidth/cli/internal/cmdutil"
)

// stubCampaignDeactivateServer answers DELETE /campaigns/{id} with
// deleteStatus (and an empty body, matching a 204), and any subsequent GET
// (used by --wait's follow-up poll) with 404 — the deactivation having
// actually taken effect. It records every request method it sees. Modeled on
// stubBrandDeleteServer in tendlc_test.go.
func stubCampaignDeactivateServer(t *testing.T, deleteStatus int) (*httptest.Server, *[]string) {
	t.Helper()
	var methods []string
	srv := newBrandStub(t, func(w http.ResponseWriter, r *http.Request) {
		methods = append(methods, r.Method)
		if r.Method == http.MethodDelete {
			w.WriteHeader(deleteStatus)
			return
		}
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"errors":[{"description":"campaign not found"}]}`))
	})
	return srv, &methods
}

// stubCampaignDeactivateStillExistsServer answers DELETE with deleteStatus,
// but every subsequent GET with 200 and a body proving the campaign is still
// there — exercising the --wait timeout path, where the follow-up read never
// 404s before the deadline. Modeled on stubBrandDeleteStillExistsServer.
func stubCampaignDeactivateStillExistsServer(t *testing.T, deleteStatus int) (*httptest.Server, *[]string) {
	t.Helper()
	var methods []string
	srv := newBrandStub(t, func(w http.ResponseWriter, r *http.Request) {
		methods = append(methods, r.Method)
		if r.Method == http.MethodDelete {
			w.WriteHeader(deleteStatus)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"data":{"campaignId":"CEXMPL1","bandwidthId":"CEXMPL1"}}`))
	})
	return srv, &methods
}

// stubCampaignNudgeCapturing records the method, path, and raw request body
// of every request, so a test can assert on exactly what 'nudge' posts.
func stubCampaignNudgeCapturing(t *testing.T, status int) (*httptest.Server, *[]string, *[]string) {
	t.Helper()
	var methods, bodies []string
	srv := newBrandStub(t, func(w http.ResponseWriter, r *http.Request) {
		methods = append(methods, r.Method+" "+r.URL.Path)
		buf, _ := io.ReadAll(r.Body)
		bodies = append(bodies, string(buf))
		w.WriteHeader(status)
	})
	return srv, &methods, &bodies
}

// Test: deactivate without --confirm exits 6 and makes ZERO HTTP requests —
// runBrandCmd's `service` seam Fatals if it is ever invoked with a nil
// server, so this fails loudly if the confirm gate ever moves after the
// service/DELETE call.
func TestCampaignDeactivateWithoutConfirmMakesNoRequests(t *testing.T) {
	_, _, err := runBrandCmd(t, nil, "campaign", "deactivate", "CEXMPL1")
	if err == nil {
		t.Fatal("want an error when --confirm is missing")
	}
	if code := cmdutil.ExitCodeForError(err); code != cmdutil.ExitFlagError {
		t.Errorf("exit code = %d, want %d", code, cmdutil.ExitFlagError)
	}
	if !strings.Contains(err.Error(), "cannot be undone") {
		t.Errorf("error = %q, want it to say the deactivation cannot be undone", err.Error())
	}
	if !strings.Contains(err.Error(), "ends") && !strings.Contains(err.Error(), "message delivery") {
		t.Errorf("error = %q, want it to name message delivery as a consequence", err.Error())
	}
	if !strings.Contains(err.Error(), "removes it from Bandwidth") {
		t.Errorf("error = %q, want it to say the campaign is removed from Bandwidth", err.Error())
	}
	if !strings.Contains(err.Error(), "stop working") {
		t.Errorf("error = %q, want it to say assigned numbers stop working", err.Error())
	}
}

// Test: deactivate --confirm without --wait issues the DELETE and prints an
// honest, unconfirmed receipt. Deactivation is asynchronous, so "deactivated"
// must be false here — it has not been confirmed, only accepted.
func TestCampaignDeactivateWithConfirmIssuesDeleteAndPrintsReceipt(t *testing.T) {
	srv, methods := stubCampaignDeactivateServer(t, http.StatusNoContent)

	out, _, err := runBrandCmd(t, srv, "campaign", "deactivate", "CEXMPL1", "--confirm", "--plain")
	if err != nil {
		t.Fatalf("campaign deactivate --confirm: %v", err)
	}
	if len(*methods) != 1 || (*methods)[0] != http.MethodDelete {
		t.Fatalf("want exactly one DELETE, got %v", *methods)
	}
	got := decodeStdout(t, out)
	if got["id"] != "CEXMPL1" {
		t.Errorf("stdout = %v, want id CEXMPL1", got)
	}
	if got["deactivated"] != false {
		t.Errorf("stdout = %v, want deactivated false — accepted is not confirmed, and there was no --wait", got)
	}
	if got["status"] != "accepted" {
		t.Errorf("stdout = %v, want status accepted", got)
	}
	note, _ := got["note"].(string)
	if !strings.Contains(note, "campaign get") {
		t.Errorf("note = %q, want it to point at how to confirm completion", note)
	}
}

// Test: deactivate --confirm --wait, where the follow-up read 404s, exits 0
// and prints deactivated:true ONLY now that the 404 actually confirmed it.
// GoneIsDone is the one place in the campaign command set where a 404 means
// success rather than "not ready yet".
func TestCampaignDeactivateWaitTreats404AsSuccess(t *testing.T) {
	srv, methods := stubCampaignDeactivateServer(t, http.StatusNoContent)

	out, _, err := runBrandCmd(t, srv, "campaign", "deactivate", "CEXMPL1", "--confirm", "--wait", "--timeout", "5", "--plain")
	if err != nil {
		t.Fatalf("campaign deactivate --confirm --wait: %v", err)
	}
	if len(*methods) < 2 || (*methods)[0] != http.MethodDelete || (*methods)[1] != http.MethodGet {
		t.Fatalf("want a DELETE then at least one GET, got %v", *methods)
	}
	got := decodeStdout(t, out)
	if got["id"] != "CEXMPL1" {
		t.Errorf("stdout = %v, want id CEXMPL1", got)
	}
	if got["deactivated"] != true {
		t.Errorf("stdout = %v, want deactivated true — the follow-up 404 confirmed it", got)
	}
	if _, present := got["note"]; present {
		t.Errorf("stdout = %v, want no unconfirmed-deactivation note once the 404 confirmed completion", got)
	}
}

// Test: deactivate --confirm --wait --timeout 0, where the follow-up read
// never 404s before the deadline, exits 5 (timeout) and must NOT claim
// deactivated:true — a receipt that contradicts its own exit code is the
// exact bug this guards.
func TestCampaignDeactivateWaitTimeoutKeepsReceiptHonest(t *testing.T) {
	srv, methods := stubCampaignDeactivateStillExistsServer(t, http.StatusNoContent)

	out, _, err := runBrandCmd(t, srv, "campaign", "deactivate", "CEXMPL1", "--confirm", "--wait", "--timeout", "0", "--plain")
	if err == nil {
		t.Fatal("want a timeout error")
	}
	if code := cmdutil.ExitCodeForError(err); code != cmdutil.ExitTimeout {
		t.Errorf("exit code = %d, want %d (timeout)", code, cmdutil.ExitTimeout)
	}
	if len(*methods) < 2 || (*methods)[0] != http.MethodDelete || (*methods)[1] != http.MethodGet {
		t.Fatalf("want a DELETE then at least one GET, got %v", *methods)
	}
	got := decodeStdout(t, out)
	if got["id"] != "CEXMPL1" {
		t.Errorf("stdout = %v, want id CEXMPL1", got)
	}
	if got["deactivated"] != false {
		t.Errorf("stdout = %v, want deactivated false — timeout means completion was never confirmed, "+
			"and exit 5 must not be paired with deactivated:true", got)
	}
	if got["status"] != "accepted" {
		t.Errorf("stdout = %v, want status accepted", got)
	}
	note, _ := got["note"].(string)
	if !strings.Contains(note, "campaign get") {
		t.Errorf("note = %q, want it to point at how to confirm completion", note)
	}
}

// Test: nudge without --intent exits 6 and makes zero HTTP requests.
func TestCampaignNudgeWithoutIntentMakesNoRequests(t *testing.T) {
	_, _, err := runBrandCmd(t, nil, "campaign", "nudge", "CEXMPL1")
	if err == nil {
		t.Fatal("want an error when --intent is missing")
	}
	if code := cmdutil.ExitCodeForError(err); code != cmdutil.ExitFlagError {
		t.Errorf("exit code = %d, want %d", code, cmdutil.ExitFlagError)
	}
	if !strings.Contains(err.Error(), "intent") {
		t.Errorf("error = %q, want it to name the missing --intent flag", err.Error())
	}
}

// Test: nudge with an invalid --intent exits 6, lists the valid values, and
// makes zero HTTP requests.
func TestCampaignNudgeRejectsInvalidIntent(t *testing.T) {
	_, _, err := runBrandCmd(t, nil, "campaign", "nudge", "CEXMPL1", "--intent", "NOT_A_REAL_INTENT")
	if err == nil {
		t.Fatal("want an error for an invalid --intent")
	}
	if code := cmdutil.ExitCodeForError(err); code != cmdutil.ExitFlagError {
		t.Errorf("exit code = %d, want %d", code, cmdutil.ExitFlagError)
	}
	for _, want := range []string{"APPEAL_REJECTION", "REVIEW"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error = %q, want it to list valid intent %q", err.Error(), want)
		}
	}
}

// Test: a valid nudge posts to /nudge with body key nudgeIntent (NOT
// "intent" — the flag name and the wire key deliberately differ), and omits
// description when the flag was never passed.
func TestCampaignNudgePostsNudgeIntentWithoutDescription(t *testing.T) {
	srv, methods, bodies := stubCampaignNudgeCapturing(t, http.StatusNoContent)

	out, _, err := runBrandCmd(t, srv, "campaign", "nudge", "CEXMPL1", "--intent", "REVIEW", "--plain")
	if err != nil {
		t.Fatalf("campaign nudge: %v", err)
	}
	if len(*methods) != 1 || !strings.HasSuffix((*methods)[0], "/campaigns/CEXMPL1/nudge") ||
		!strings.HasPrefix((*methods)[0], http.MethodPost) {
		t.Fatalf("methods = %v, want one POST to .../campaigns/CEXMPL1/nudge", *methods)
	}
	body := (*bodies)[0]
	if !strings.Contains(body, `"nudgeIntent"`) {
		t.Errorf("body = %q, want the wire key nudgeIntent", body)
	}
	if strings.Contains(body, `"intent"`) {
		t.Errorf("body = %q, must not use the flag name \"intent\" as the wire key", body)
	}
	if !strings.Contains(body, "REVIEW") {
		t.Errorf("body = %q, want the REVIEW intent value", body)
	}
	if strings.Contains(body, "description") {
		t.Errorf("body = %q, want no description key when --description was never passed", body)
	}

	got := decodeStdout(t, out)
	if got["id"] != "CEXMPL1" || got["nudgeIntent"] != "REVIEW" {
		t.Errorf("stdout = %v, want id CEXMPL1 and nudgeIntent REVIEW", got)
	}
	if _, present := got["description"]; present {
		t.Errorf("stdout = %v, want no description key in the receipt either", got)
	}
}

// Test: passing --description includes both nudgeIntent and description in
// the POST body.
func TestCampaignNudgeIncludesDescriptionWhenPassed(t *testing.T) {
	srv, methods, bodies := stubCampaignNudgeCapturing(t, http.StatusNoContent)

	out, _, err := runBrandCmd(t, srv, "campaign", "nudge", "CEXMPL1",
		"--intent", "APPEAL_REJECTION", "--description", "Appealing a rejected review.", "--plain")
	if err != nil {
		t.Fatalf("campaign nudge: %v", err)
	}
	if len(*methods) != 1 {
		t.Fatalf("methods = %v, want exactly one request", *methods)
	}
	body := (*bodies)[0]
	if !strings.Contains(body, `"nudgeIntent"`) || !strings.Contains(body, "APPEAL_REJECTION") {
		t.Errorf("body = %q, want nudgeIntent APPEAL_REJECTION", body)
	}
	if !strings.Contains(body, `"description"`) || !strings.Contains(body, "Appealing a rejected review.") {
		t.Errorf("body = %q, want the description key and value", body)
	}

	got := decodeStdout(t, out)
	if got["description"] != "Appealing a rejected review." {
		t.Errorf("stdout = %v, want the description echoed in the receipt", got)
	}
}

// Test: nudge has no --confirm flag — it is not destructive or billable, so
// gating it behind --confirm would be an unjustified over-gate.
func TestCampaignNudgeHasNoConfirmFlag(t *testing.T) {
	c, _, err := Cmd.Find([]string{"campaign", "nudge"})
	if err != nil {
		t.Fatalf("Find(campaign nudge): %v", err)
	}
	if f := c.Flags().Lookup("confirm"); f != nil {
		t.Errorf("campaign nudge has a --confirm flag, want none")
	}
}

func TestCampaignLifecycleCommandsRejectStrayPositionals(t *testing.T) {
	cases := [][]string{
		{"campaign", "deactivate"},
		{"campaign", "deactivate", "C1", "STRAY"},
		{"campaign", "nudge"},
		{"campaign", "nudge", "C1", "STRAY"},
	}
	for _, args := range cases {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			if _, _, err := runBrandCmd(t, nil, args...); err == nil {
				t.Fatal("want an argument error")
			}
		})
	}
}

// Test: a 403 on deactivate maps to exit 4 via roleGateError.
func TestCampaignDeactivateRoleGate403MapsToExitFour(t *testing.T) {
	_, _, err := runBrandCmd(t, stubBrandErr(t, 403,
		`{"errors":[{"description":"does not have access rights"}]}`),
		"campaign", "deactivate", "CEXMPL1", "--confirm")
	if err == nil {
		t.Fatal("want an error on 403")
	}
	if code := cmdutil.ExitCodeForError(err); code != cmdutil.ExitConflict {
		t.Errorf("exit code = %d, want %d — re-authenticating cannot add a role", code, cmdutil.ExitConflict)
	}
}

// Test: a 403 on nudge maps to exit 4 via roleGateError.
func TestCampaignNudgeRoleGate403MapsToExitFour(t *testing.T) {
	_, _, err := runBrandCmd(t, stubBrandErr(t, 403,
		`{"errors":[{"description":"does not have access rights"}]}`),
		"campaign", "nudge", "CEXMPL1", "--intent", "REVIEW")
	if err == nil {
		t.Fatal("want an error on 403")
	}
	if code := cmdutil.ExitCodeForError(err); code != cmdutil.ExitConflict {
		t.Errorf("exit code = %d, want %d — re-authenticating cannot add a role", code, cmdutil.ExitConflict)
	}
}
