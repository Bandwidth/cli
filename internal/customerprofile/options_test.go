package customerprofile

import (
	"errors"
	"testing"

	"github.com/Bandwidth/cli/internal/cmdutil"
)

func TestValidateCreateRequiresName(t *testing.T) {
	err := ValidateCreate(CreateOptions{})
	if err == nil {
		t.Fatal("expected an error when --name is missing")
	}
	var fe *cmdutil.FlagError
	if !errors.As(err, &fe) {
		t.Fatalf("error type = %T, want *cmdutil.FlagError so it exits 6", err)
	}
	if got := cmdutil.ExitCodeForError(err); got != cmdutil.ExitFlagError {
		t.Errorf("exit code = %d, want %d", got, cmdutil.ExitFlagError)
	}
}

// A contact is optional, but the API requires a name inside one if any
// contact field is supplied. Partial contacts must fail locally, not at the API.
func TestValidateCreateRejectsPartialContact(t *testing.T) {
	err := ValidateCreate(CreateOptions{Name: "Acme", ContactEmail: "ops@acme.com"})
	if err == nil {
		t.Fatal("expected an error when a contact field is set without --contact-name")
	}
	if got := cmdutil.ExitCodeForError(err); got != cmdutil.ExitFlagError {
		t.Errorf("exit code = %d, want %d", got, cmdutil.ExitFlagError)
	}
}

func TestValidateCreateAcceptsNameOnly(t *testing.T) {
	if err := ValidateCreate(CreateOptions{Name: "Acme"}); err != nil {
		t.Fatalf("name alone should be valid: %v", err)
	}
}

func TestBuildCreateRequestOmitsEmptyFields(t *testing.T) {
	got := BuildCreateRequest(CreateOptions{Name: "Acme"})
	if got["name"] != "Acme" {
		t.Errorf("name = %v", got["name"])
	}
	if _, present := got["website"]; present {
		t.Error("empty website should be omitted, not sent as an empty string")
	}
	if _, present := got["contact"]; present {
		t.Error("no contact fields set, so contact should be omitted entirely")
	}
}

func TestBuildCreateRequestNestsContact(t *testing.T) {
	got := BuildCreateRequest(CreateOptions{
		Name: "Acme", ContactName: "Ops", ContactEmail: "ops@acme.com"})
	c, ok := got["contact"].(map[string]any)
	if !ok {
		t.Fatalf("contact = %#v, want a nested object", got["contact"])
	}
	if c["name"] != "Ops" || c["email"] != "ops@acme.com" {
		t.Errorf("contact = %#v", c)
	}
	if _, present := c["phoneNumber"]; present {
		t.Error("unset contact phone should be omitted")
	}
}

// THE CENTRAL TEST OF THIS PR. PUT is a full replacement, so anything the
// outgoing body omits is nulled server-side. Fields the CLI has never heard of
// must survive an update untouched.
func TestBuildUpdateRequestPreservesUnknownFields(t *testing.T) {
	current := map[string]any{
		"id":             "abc",
		"accountId":      "9901287",
		"name":           "Acme",
		"website":        "https://acme.com",
		"version":        float64(3),
		"createdDate":    "2026-01-01T00:00:00Z",
		"modifiedDate":   "2026-01-02T00:00:00Z",
		"totalCampaigns": float64(2),
		"softDeleted":    false,
		"futureField":    "the CLI has never heard of this",
	}
	got, err := BuildUpdateRequest(current, UpdateOptions{Name: "Acme Renamed"},
		map[string]bool{"name": true})
	if err != nil {
		t.Fatalf("BuildUpdateRequest: %v", err)
	}

	if got["futureField"] != "the CLI has never heard of this" {
		t.Errorf("unknown field was dropped — PUT would null it server-side: %#v", got)
	}
	if got["website"] != "https://acme.com" {
		t.Errorf("unchanged website was dropped: %v", got["website"])
	}
	if got["name"] != "Acme Renamed" {
		t.Errorf("name = %v, want the changed value", got["name"])
	}
	if got["version"] != float64(3) {
		t.Errorf("version = %v, want it carried through — the API rejects updates without it", got["version"])
	}
	for _, ro := range []string{"id", "accountId", "createdDate", "modifiedDate", "totalCampaigns"} {
		if _, present := got[ro]; present {
			t.Errorf("read-only field %q must be stripped before PUT", ro)
		}
	}
}

// An unchanged flag must not overwrite a set value with empty string.
func TestBuildUpdateRequestIgnoresUnchangedFlags(t *testing.T) {
	current := map[string]any{"name": "Acme", "website": "https://acme.com", "version": float64(1)}
	got, err := BuildUpdateRequest(current, UpdateOptions{}, map[string]bool{})
	if err != nil {
		t.Fatalf("BuildUpdateRequest: %v", err)
	}
	if got["website"] != "https://acme.com" {
		t.Errorf("website = %v, want it untouched when --website was not passed", got["website"])
	}
}

// Explicitly clearing a field is different from not passing it.
func TestBuildUpdateRequestAllowsExplicitClear(t *testing.T) {
	current := map[string]any{"name": "Acme", "website": "https://acme.com", "version": float64(1)}
	got, err := BuildUpdateRequest(current, UpdateOptions{Website: ""},
		map[string]bool{"website": true})
	if err != nil {
		t.Fatalf("BuildUpdateRequest: %v", err)
	}
	if got["website"] != "" {
		t.Errorf("website = %v, want an explicit empty string", got["website"])
	}
}

func TestBuildUpdateRequestRequiresVersion(t *testing.T) {
	_, err := BuildUpdateRequest(map[string]any{"name": "Acme"}, UpdateOptions{}, map[string]bool{})
	if err == nil {
		t.Fatal("expected an error when the current resource has no version")
	}
}

// BuildUpdateRequest must not hand back a body that shares nested mutable
// structure with current — current may be a cached resource the command
// layer reuses, and callers that mutate the returned body in place (rather
// than going through overlayIfChanged) must not corrupt it.
func TestBuildUpdateRequestDoesNotAliasNestedMaps(t *testing.T) {
	contact := map[string]any{"name": "Ops", "email": "ops@acme.com"}
	current := map[string]any{"name": "Acme", "version": float64(1), "contact": contact}

	got, err := BuildUpdateRequest(current, UpdateOptions{}, map[string]bool{})
	if err != nil {
		t.Fatalf("BuildUpdateRequest: %v", err)
	}

	gotContact, ok := got["contact"].(map[string]any)
	if !ok {
		t.Fatalf("contact = %#v, want a nested object", got["contact"])
	}
	gotContact["email"] = "mutated@acme.com"

	if contact["email"] != "ops@acme.com" {
		t.Errorf("mutating the returned body's contact changed current's contact: %#v", contact)
	}
}

// Partial contact updates must preserve the fields not being changed — this
// is the exact path Finding 1 touches, since the surviving contact fields
// come from a copy of current's nested contact map.
func TestBuildUpdateRequestPreservesUnchangedContactFields(t *testing.T) {
	current := map[string]any{
		"name":    "Acme",
		"version": float64(1),
		"contact": map[string]any{"name": "Ops", "phoneNumber": "+15555550100"},
	}
	got, err := BuildUpdateRequest(current, UpdateOptions{ContactEmail: "new@acme.com"},
		map[string]bool{"contact-email": true})
	if err != nil {
		t.Fatalf("BuildUpdateRequest: %v", err)
	}

	c, ok := got["contact"].(map[string]any)
	if !ok {
		t.Fatalf("contact = %#v, want a nested object", got["contact"])
	}
	if c["name"] != "Ops" {
		t.Errorf("contact name = %v, want it preserved from current", c["name"])
	}
	if c["phoneNumber"] != "+15555550100" {
		t.Errorf("contact phoneNumber = %v, want it preserved from current", c["phoneNumber"])
	}
	if c["email"] != "new@acme.com" {
		t.Errorf("contact email = %v, want the newly set value", c["email"])
	}
}

func TestBuildRestoreRequestClearsSoftDeleted(t *testing.T) {
	current := map[string]any{"name": "Acme", "version": float64(3), "softDeleted": true, "id": "abc"}
	got, err := BuildRestoreRequest(current)
	if err != nil {
		t.Fatalf("BuildRestoreRequest: %v", err)
	}
	if got["softDeleted"] != false {
		t.Errorf("softDeleted = %v, want false", got["softDeleted"])
	}
	if _, present := got["deleted"]; present {
		t.Error(`must not send "deleted" — the documented form returns 404`)
	}
	if got["version"] != float64(3) {
		t.Errorf("version = %v, want it carried through", got["version"])
	}
}
