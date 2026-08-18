package tendlc

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Bandwidth/cli/internal/cmdutil"
)

// liveBrandForUpdate mirrors the shape GET /brands/{id} actually returns —
// the same fixture internal/tendlc's own (unexported) liveBrand() test helper
// uses, reproduced here because that helper is private to internal/tendlc's
// test package. It deliberately carries "someFutureField", a key the CLI does
// not model at all, so the rename-only losslessness test below has something
// genuinely unmodeled to check for.
func liveBrandForUpdate(brandType, businessContactEmail string) map[string]any {
	return map[string]any{
		"bandwidthId":             "WET8JUY8H0",
		"brandId":                 "BGJR2BA",
		"brandIdentityStatus":     "VERIFIED",
		"brandType":               brandType,
		"companyName":             "Bandwidth Inc",
		"displayName":             "Bandwidth Acceptance Test",
		"street":                  "1000 Bandwidth Way",
		"city":                    "Raleigh",
		"state":                   "NC",
		"postalCode":              "27606",
		"countryCodeA3":           "USA",
		"phone":                   "+12025551234",
		"email":                   "npatel@bandwidth.com",
		"ein":                     "562242657",
		"einIssuingCountryCodeA3": "USA",
		"website":                 "https://bandwidth.com",
		"businessContactEmail":    businessContactEmail,
		"someFutureField":         "keep me",
	}
}

// stubBrandUpdateServer answers GET /brands/{id} with getBody wrapped in the
// standard envelope, and PUT with either putStatus's error body (if
// putStatus != 200) or an echo of exactly what was sent, also enveloped. It
// records every PUT body's raw JSON so a test can assert on the request the
// CLI actually sent, not just on whether the command succeeded.
func stubBrandUpdateServer(t *testing.T, getBody map[string]any, putStatus int) (*httptest.Server, *[]string) {
	t.Helper()
	getJSON, err := json.Marshal(map[string]any{"data": getBody})
	if err != nil {
		t.Fatalf("marshaling GET fixture: %v", err)
	}
	var putBodies []string
	srv := newBrandStub(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			_, _ = w.Write(getJSON)
			return
		}
		b, _ := io.ReadAll(r.Body)
		putBodies = append(putBodies, string(b))
		if putStatus != http.StatusOK {
			w.WriteHeader(putStatus)
			_, _ = w.Write([]byte(`{"errors":[{"description":"conflict"}]}`))
			return
		}
		var sent map[string]any
		if err := json.Unmarshal(b, &sent); err != nil {
			t.Fatalf("PUT body is not JSON: %v", err)
		}
		resp, err := json.Marshal(map[string]any{"data": sent})
		if err != nil {
			t.Fatalf("marshaling PUT echo: %v", err)
		}
		_, _ = w.Write(resp)
	})
	return srv, &putBodies
}

// Test 1: no field flags at all is exit 6 and makes no request whatsoever —
// runBrandCmd's `service` seam Fatals if invoked with a nil server, so this
// cannot pass by accident even if the "nothing to update" check moved after
// the service/GET call.
func TestBrandUpdateNoFieldFlagsExitsSixWithZeroRequests(t *testing.T) {
	_, _, err := runBrandCmd(t, nil, "brand", "update", "BGJR2BA")
	if err == nil {
		t.Fatal("want an error when no field flags are passed")
	}
	if code := cmdutil.ExitCodeForError(err); code != cmdutil.ExitFlagError {
		t.Errorf("exit code = %d, want %d", code, cmdutil.ExitFlagError)
	}
	if !strings.Contains(err.Error(), "nothing to update") {
		t.Errorf("error = %q, want it to say nothing to update", err.Error())
	}
}

// Test 2: THE load-bearing regression test for this task. A rename-only
// update must preserve every other field on the PUT body — including
// companyName, website, and someFutureField, a key the CLI does not model at
// all. This is not a spot check: losing any of these silently would be
// exactly the failure a customer renaming a brand must never hit.
func TestBrandUpdateRenameOnlyPreservesEveryOtherField(t *testing.T) {
	srv, bodies := stubBrandUpdateServer(t, liveBrandForUpdate("PRIVATE_PROFIT", "biz@acme.com"), http.StatusOK)

	_, _, err := runBrandCmd(t, srv, "brand", "update", "BGJR2BA", "--display-name", "Renamed", "--plain")
	if err != nil {
		t.Fatalf("brand update: %v", err)
	}
	if len(*bodies) != 1 {
		t.Fatalf("want exactly one PUT, got %d", len(*bodies))
	}
	var sent map[string]any
	if err := json.Unmarshal([]byte((*bodies)[0]), &sent); err != nil {
		t.Fatalf("PUT body is not JSON: %v", err)
	}
	if sent["displayName"] != "Renamed" {
		t.Errorf("sent displayName = %v, want Renamed", sent["displayName"])
	}
	if sent["companyName"] != "Bandwidth Inc" {
		t.Errorf("sent companyName = %v, want it preserved as Bandwidth Inc", sent["companyName"])
	}
	if sent["website"] != "https://bandwidth.com" {
		t.Errorf("sent website = %v, want it preserved", sent["website"])
	}
	if sent["someFutureField"] != "keep me" {
		t.Errorf("sent someFutureField = %v, want the unmodeled field preserved", sent["someFutureField"])
	}
}

// Test 3: changing --company-name without --confirm exits 6. The GET must
// have happened (the stub would otherwise never see the PUT-absence
// assertion meaningfully), but no PUT is sent.
func TestBrandUpdateCompanyNameWithoutConfirmRefuses(t *testing.T) {
	srv, bodies := stubBrandUpdateServer(t, liveBrandForUpdate("PRIVATE_PROFIT", "biz@acme.com"), http.StatusOK)

	_, _, err := runBrandCmd(t, srv, "brand", "update", "BGJR2BA", "--company-name", "New Co", "--plain")
	if err == nil {
		t.Fatal("want an error when an identity field changes without --confirm")
	}
	if code := cmdutil.ExitCodeForError(err); code != cmdutil.ExitFlagError {
		t.Errorf("exit code = %d, want %d", code, cmdutil.ExitFlagError)
	}
	if !strings.Contains(err.Error(), "company-name") {
		t.Errorf("error = %q, want it to name company-name", err.Error())
	}
	if !strings.Contains(err.Error(), "$4 fee") {
		t.Errorf("error = %q, want it to mention the $4 fee", err.Error())
	}
	if len(*bodies) != 0 {
		t.Errorf("want zero PUT requests, got %d", len(*bodies))
	}
}

// Test 4: the same change with --confirm goes through and issues the PUT.
func TestBrandUpdateCompanyNameWithConfirmProceeds(t *testing.T) {
	srv, bodies := stubBrandUpdateServer(t, liveBrandForUpdate("PRIVATE_PROFIT", "biz@acme.com"), http.StatusOK)

	_, _, err := runBrandCmd(t, srv, "brand", "update", "BGJR2BA", "--company-name", "New Co", "--confirm", "--plain")
	if err != nil {
		t.Fatalf("brand update --confirm: %v", err)
	}
	if len(*bodies) != 1 {
		t.Fatalf("want exactly one PUT, got %d", len(*bodies))
	}
	var sent map[string]any
	if err := json.Unmarshal([]byte((*bodies)[0]), &sent); err != nil {
		t.Fatalf("PUT body is not JSON: %v", err)
	}
	if sent["companyName"] != "New Co" {
		t.Errorf("sent companyName = %v, want New Co", sent["companyName"])
	}
}

// Test 5: changing only --website needs no --confirm at all.
func TestBrandUpdateWebsiteOnlyNeedsNoConfirm(t *testing.T) {
	srv, bodies := stubBrandUpdateServer(t, liveBrandForUpdate("PRIVATE_PROFIT", "biz@acme.com"), http.StatusOK)

	_, _, err := runBrandCmd(t, srv, "brand", "update", "BGJR2BA", "--website", "https://acme.example", "--plain")
	if err != nil {
		t.Fatalf("brand update --website: %v", err)
	}
	if len(*bodies) != 1 {
		t.Fatalf("want exactly one PUT, got %d", len(*bodies))
	}
}

// Test 6: --business-contact-email needs no --confirm on a PRIVATE_PROFIT
// brand, but does on a PUBLIC_PROFIT one — IdentityFieldsChanged reads the
// CURRENT brand's type, not anything the caller passed.
func TestBrandUpdateBusinessContactEmailConfirmDependsOnBrandType(t *testing.T) {
	t.Run("PRIVATE_PROFIT needs no confirm", func(t *testing.T) {
		srv, bodies := stubBrandUpdateServer(t, liveBrandForUpdate("PRIVATE_PROFIT", "old@acme.com"), http.StatusOK)
		_, _, err := runBrandCmd(t, srv, "brand", "update", "BGJR2BA",
			"--business-contact-email", "new@acme.com", "--plain")
		if err != nil {
			t.Fatalf("brand update: %v", err)
		}
		if len(*bodies) != 1 {
			t.Fatalf("want exactly one PUT, got %d", len(*bodies))
		}
	})

	t.Run("PUBLIC_PROFIT needs confirm", func(t *testing.T) {
		srv, bodies := stubBrandUpdateServer(t, liveBrandForUpdate("PUBLIC_PROFIT", "old@acme.com"), http.StatusOK)
		_, _, err := runBrandCmd(t, srv, "brand", "update", "BGJR2BA",
			"--business-contact-email", "new@acme.com", "--plain")
		if err == nil {
			t.Fatal("want an error requiring --confirm on a PUBLIC_PROFIT brand")
		}
		if code := cmdutil.ExitCodeForError(err); code != cmdutil.ExitFlagError {
			t.Errorf("exit code = %d, want %d", code, cmdutil.ExitFlagError)
		}
		if !strings.Contains(err.Error(), "Auth+") {
			t.Errorf("error = %q, want it to mention Auth+", err.Error())
		}
		if len(*bodies) != 0 {
			t.Errorf("want zero PUT requests, got %d", len(*bodies))
		}

		if _, _, err := runBrandCmd(t, srv, "brand", "update", "BGJR2BA",
			"--business-contact-email", "new@acme.com", "--confirm", "--plain"); err != nil {
			t.Fatalf("brand update --confirm: %v", err)
		}
		if len(*bodies) != 1 {
			t.Fatalf("want exactly one PUT after --confirm, got %d", len(*bodies))
		}
	})
}

// Test 7: clearing a universally-required field exits 6 before any PUT. This
// is not an identity field, so it must fail on ValidateBrandUpdate rather
// than the confirm gate.
func TestBrandUpdateClearingRequiredFieldExitsSixBeforePut(t *testing.T) {
	srv, bodies := stubBrandUpdateServer(t, liveBrandForUpdate("PRIVATE_PROFIT", "biz@acme.com"), http.StatusOK)

	_, _, err := runBrandCmd(t, srv, "brand", "update", "BGJR2BA", "--display-name", "", "--plain")
	if err == nil {
		t.Fatal("want an error when clearing a universally-required field")
	}
	if code := cmdutil.ExitCodeForError(err); code != cmdutil.ExitFlagError {
		t.Errorf("exit code = %d, want %d", code, cmdutil.ExitFlagError)
	}
	if len(*bodies) != 0 {
		t.Errorf("want zero PUT requests, got %d", len(*bodies))
	}
}

// Test 8: update offers no --wait flag — the write lands against a brand
// usually already in a terminal state, so polling would report success
// before the change applied.
func TestBrandUpdateHasNoWaitFlag(t *testing.T) {
	if f := brandUpdateCmd.Flags().Lookup("wait"); f != nil {
		t.Errorf("brandUpdateCmd has a --wait flag, want none: %+v", f)
	}
}

// Test 9: a 409 on the PUT means the brand exists in Bandwidth but not yet in
// TCR — exit 4, and the message points at 'band tendlc brand refresh'.
func TestBrandUpdateConflictOnPutMapsToTCRHint(t *testing.T) {
	srv, _ := stubBrandUpdateServer(t, liveBrandForUpdate("PRIVATE_PROFIT", "biz@acme.com"), http.StatusConflict)

	_, _, err := runBrandCmd(t, srv, "brand", "update", "BGJR2BA", "--website", "https://acme.example", "--plain")
	if err == nil {
		t.Fatal("want an error on a 409 PUT")
	}
	if code := cmdutil.ExitCodeForError(err); code != cmdutil.ExitConflict {
		t.Errorf("exit code = %d, want %d", code, cmdutil.ExitConflict)
	}
	if !strings.Contains(err.Error(), "TCR") {
		t.Errorf("error = %q, want it to mention TCR", err.Error())
	}
	if !strings.Contains(err.Error(), "brand refresh") {
		t.Errorf("error = %q, want it to suggest brand refresh", err.Error())
	}
}
