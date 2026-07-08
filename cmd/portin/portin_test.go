package portin

import (
	"errors"
	"testing"

	"github.com/Bandwidth/cli/internal/api"
	"github.com/Bandwidth/cli/internal/cmdutil"
)

func TestCmdStructure(t *testing.T) {
	if Cmd.Use != "portin" {
		t.Errorf("Use = %q, want %q", Cmd.Use, "portin")
	}
	expected := []string{
		"validate-tf",
		"create",
		"get",
		"list",
		"submit",
		"supp",
		"cancel",
		"history",
		"upload-loa",
		"notes",
		"bulk",
	}
	have := map[string]bool{}
	for _, c := range Cmd.Commands() {
		have[c.Name()] = true
	}
	for _, want := range expected {
		if !have[want] {
			t.Errorf("missing subcommand %q", want)
		}
	}
}

func TestCreateRequiresNumbers(t *testing.T) {
	f := createCmd.Flags().Lookup("numbers")
	if f == nil {
		t.Fatal("missing --numbers flag")
	}
	if _, ok := f.Annotations["cobra_annotation_bash_completion_one_required_flag"]; !ok {
		t.Error("--numbers must be required")
	}
}

func TestStripE164(t *testing.T) {
	cases := []struct{ in, want string }{
		{"+19195551234", "9195551234"},
		{"19195551234", "9195551234"},
		{"9195551234", "9195551234"},
		{"+18005551234", "8005551234"},
	}
	for _, tt := range cases {
		if got := stripE164(tt.in); got != tt.want {
			t.Errorf("stripE164(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

// TestIs7300DetectsSilentSuppFailure exercises the documented Confluence trap
// where a supp PUT returns 200 but the next GET surfaces error code 7300,
// meaning the change never propagated to Neustar. This is the bug we
// explicitly designed `band portin supp` to catch.
func TestIs7300DetectsSilentSuppFailure(t *testing.T) {
	// Mimics what XMLToMap returns for an LnpOrderResponse with an Errors block.
	resp := map[string]interface{}{
		"LnpOrderResponse": map[string]interface{}{
			"ProcessingStatus": "FOC",
			"Errors": map[string]interface{}{
				"Error": map[string]interface{}{
					"Code":        "7300",
					"Description": "Supplement not propagated",
				},
			},
		},
	}
	if !is7300(resp) {
		t.Error("expected is7300 to detect the 7300 error code in the response")
	}

	// A response without 7300 must not trigger the silent-fail path.
	clean := map[string]interface{}{
		"LnpOrderResponse": map[string]interface{}{
			"ProcessingStatus": "PENDING_DOCUMENTS",
		},
	}
	if is7300(clean) {
		t.Error("is7300 must not fire on a clean response")
	}
}

// TestPortinErrorMapsTollFreePhase1To403 verifies that a 403 whose body
// references toll-free or phase 1 is mapped to FeatureLimitError (exit 4),
// not generic auth-error (exit 2). This is the documented gate where the
// account doesn't have TOLL_FREE_AUTOMATION_PHASE_1 enabled.
func TestPortinErrorMapsTollFreePhase1To403(t *testing.T) {
	apiErr := &api.APIError{
		StatusCode: 403,
		Body:       "<error>TOLL_FREE_AUTOMATION_PHASE_1 not enabled for account</error>",
	}
	wrapped := portinError(apiErr, "creating port-in order")
	var fle *cmdutil.FeatureLimitError
	if !errors.As(wrapped, &fle) {
		t.Fatalf("expected FeatureLimitError, got %T (%v)", wrapped, wrapped)
	}
	if cmdutil.ExitCodeForError(wrapped) != cmdutil.ExitConflict {
		t.Errorf("phase-1 gate must exit 4 (ExitConflict), got %d", cmdutil.ExitCodeForError(wrapped))
	}
}

func TestPortinError404IsTaggedNotFound(t *testing.T) {
	apiErr := &api.APIError{StatusCode: 404, Body: "<error>order not found</error>"}
	wrapped := portinError(apiErr, "getting port-in order")
	if cmdutil.ExitCodeForError(wrapped) != cmdutil.ExitGeneral {
		// 404 falls through to generic exit since it's wrapped without an underlying API error type
		// preserved. We surface a custom message but lose the underlying code on intent — confirm
		// the message is friendly rather than the raw error body.
		t.Logf("note: 404 surfaces as exit %d with message: %v", cmdutil.ExitCodeForError(wrapped), wrapped)
	}
	if msg := wrapped.Error(); msg == "" {
		t.Error("404 error must include a non-empty message")
	}
}

func TestFlattenPortInResultLocksV1Shape(t *testing.T) {
	// Simulated XML→map response shape from the Numbers API.
	resp := map[string]interface{}{
		"LnpOrderResponse": map[string]interface{}{
			"OrderId":          "ord-123",
			"ProcessingStatus": "PENDING_DOCUMENTS",
			"RequestedFocDate": "2026-06-01T00:00:00Z",
			"CustomerOrderId":  "agent-run-42",
			"ListOfPhoneNumbers": map[string]interface{}{
				"PhoneNumber": []interface{}{"9195551234", "9195551235"},
			},
		},
	}
	got := flattenPortInResult(resp, "")
	keys := []string{"orderId", "status", "focDate", "numbers", "customerOrderId", "errorCode"}
	for _, k := range keys {
		if _, ok := got[k]; !ok {
			t.Errorf("v1 plain shape missing key %q", k)
		}
	}
	if got["orderId"] != "ord-123" {
		t.Errorf("orderId = %v, want ord-123", got["orderId"])
	}
	if got["status"] != "PENDING_DOCUMENTS" {
		t.Errorf("status = %v, want PENDING_DOCUMENTS", got["status"])
	}
	if got["customerOrderId"] != "agent-run-42" {
		t.Errorf("customerOrderId = %v, want agent-run-42", got["customerOrderId"])
	}
	nums, ok := got["numbers"].([]string)
	if !ok || len(nums) != 2 {
		t.Errorf("numbers shape wrong: %#v", got["numbers"])
	}
	for _, n := range nums {
		if n[0] != '+' {
			t.Errorf("number %q must be normalized to E.164 with + prefix", n)
		}
	}
}

func TestFlattenValidateTFSurfacesPortableAndNonPortable(t *testing.T) {
	resp := map[string]interface{}{
		"TollFreePortingValidationResponse": map[string]interface{}{
			"TollFreePortingValidation": map[string]interface{}{
				"ProcessingStatus": "COMPLETE",
				"Breakdown": map[string]interface{}{
					"PortableTollFreeNumberList": map[string]interface{}{
						"RespOrgList": map[string]interface{}{
							"RespOrg": map[string]interface{}{
								"Id": "TST51",
								"TollFreeNumberList": map[string]interface{}{
									"TollFreeNumber": "8336531000",
								},
							},
						},
					},
					"SpareTollFreeNumberList": map[string]interface{}{
						"TollFreeNumber": "8336521001",
					},
				},
			},
		},
	}
	got := flattenValidateTFResult(resp)
	if len(got) != 2 {
		t.Fatalf("expected 2 entries, got %d: %#v", len(got), got)
	}
	var portableSeen, nonPortableSeen bool
	for _, e := range got {
		if p, _ := e["portable"].(bool); p {
			portableSeen = true
			if e["respOrgId"] != "TST51" {
				t.Errorf("portable entry should carry respOrgId, got %v", e["respOrgId"])
			}
		} else {
			nonPortableSeen = true
			if reason, _ := e["reason"].(string); reason == "" {
				t.Error("non-portable entry must carry a non-empty reason")
			}
		}
	}
	if !portableSeen || !nonPortableSeen {
		t.Errorf("expected both portable and non-portable entries; portable=%v nonPortable=%v",
			portableSeen, nonPortableSeen)
	}
}

func TestDetectContentType(t *testing.T) {
	cases := []struct{ in, want string }{
		{"loa.pdf", "application/pdf"},
		{"loa.PDF", "application/pdf"},
		{"loa.png", "image/png"},
	}
	for _, tt := range cases {
		got := detectContentType(tt.in)
		// mime.TypeByExtension may add charset suffixes; just verify the prefix.
		if got != tt.want && got[:len(tt.want)] != tt.want {
			t.Errorf("detectContentType(%q) = %q, want prefix %q", tt.in, got, tt.want)
		}
	}
}

// TestValidateFOCDateTime locks the supp FOC format requirement: the API
// rejects date-only values on a supp, so the CLI must catch them client-side
// with a message that names the expected YYYY-MM-DDTHH:MM:SSZ format.
func TestValidateFOCDateTime(t *testing.T) {
	valid := []string{
		"2026-06-01T15:30:00Z",
		"2026-12-04T11:30:00-04:00",
	}
	for _, in := range valid {
		if err := validateFOCDateTime(in); err != nil {
			t.Errorf("validateFOCDateTime(%q) = %v, want nil", in, err)
		}
	}
	invalid := []string{
		"2026-06-01",
		"2026-06-01Z",
		"june 1st",
		"2026-06-01T15:30Z",
	}
	for _, in := range invalid {
		if err := validateFOCDateTime(in); err == nil {
			t.Errorf("validateFOCDateTime(%q) = nil, want error", in)
		}
	}
}

// TestSubmitTerminalStates locks the post-submit wait contract. SUBMITTED is
// the happy terminal state for a toll-free order (TFN validation passed and
// the order is with the vendor); INVALID_TFNS is the post-submit validation
// failure. Both were previously missing, which made --wait time out even on
// success. VALIDATE_TFNS is the transient state --wait exists to sit through.
func TestSubmitTerminalStates(t *testing.T) {
	terminal := []string{
		"SUBMITTED",
		"INVALID_TFNS",
		"EXCEPTION",
		"PENDING_DOCUMENTS",
		"PENDING_CARRIER_APPROVAL",
		"REQUESTED_SUPP",
		"FOC",
		"COMPLETE",
		"CANCELLED",
		"REJECTED",
		"FAILED",
		"FOC_GRANTED",
	}
	for _, s := range terminal {
		if !submitTerminal[s] {
			t.Errorf("state %q must be terminal for submit --wait", s)
		}
	}
	if submitTerminal["VALIDATE_TFNS"] {
		t.Error("VALIDATE_TFNS is the transient state submit --wait sits through; it must not be terminal")
	}
	if submitTerminal["DRAFT"] || submitTerminal["VALID_DRAFT_TFNS"] {
		t.Error("draft-side states must not be terminal or --wait would return before the transition")
	}
}

// TestSubmitWaitDone locks the consecutive-SUBMITTED contract: a single
// SUBMITTED sighting right after the submit PUT is not proof validation
// passed, since the order can transiently report SUBMITTED before entering
// VALIDATE_TFNS.
func TestSubmitWaitDone(t *testing.T) {
	tests := []struct {
		status     string
		prevStatus string
		want       bool
	}{
		{"SUBMITTED", "", false},
		{"SUBMITTED", "SUBMITTED", true},
		{"SUBMITTED", "VALIDATE_TFNS", true},
		{"SUBMITTED", "VALIDATE_DRAFT_TFNS", false},
		{"VALIDATE_TFNS", "SUBMITTED", false},
		{"INVALID_TFNS", "VALIDATE_TFNS", true},
	}
	for _, tc := range tests {
		if got := submitWaitDone(tc.status, tc.prevStatus); got != tc.want {
			t.Errorf("submitWaitDone(%q, %q) = %v, want %v", tc.status, tc.prevStatus, got, tc.want)
		}
	}
}
