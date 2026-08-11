package vcp

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/Bandwidth/cli/internal/cmdutil"
)

func TestBuildRoutePlan_SingleFQDNEndpoint(t *testing.T) {
	plan, err := BuildRoutePlan("vapi.example.sip.vapi.ai", "FQDN", "")
	if err != nil {
		t.Fatalf("BuildRoutePlan() error = %v", err)
	}
	routes := plan["routes"].([]map[string]interface{})
	if len(routes) != 1 {
		t.Fatalf("routes = %d, want 1", len(routes))
	}
	if routes[0]["priority"] != 1 {
		t.Errorf("priority = %v, want 1", routes[0]["priority"])
	}
	if routes[0]["name"] != "primary route" {
		t.Errorf("name = %v, want 'primary route'", routes[0]["name"])
	}
	// A route's type is WEIGHTED|ANI; the endpoint carries the TN/SIP/IP_V4/FQDN type.
	if routes[0]["type"] != "WEIGHTED" {
		t.Errorf("route type = %v, want WEIGHTED", routes[0]["type"])
	}
	eps := routes[0]["endpoints"].([]map[string]interface{})
	if eps[0]["endpoint"] != "vapi.example.sip.vapi.ai" || eps[0]["type"] != "FQDN" {
		t.Errorf("endpoint = %v", eps[0])
	}
}

func TestValidateEndpoint(t *testing.T) {
	ok := map[string]string{
		"TN":    "+19195551234",
		"SIP":   "sip:agent@example.com",
		"IP_V4": "192.0.2.10",
		"FQDN":  "host.example.com",
	}
	for typ, ep := range ok {
		if err := ValidateEndpoint(ep, typ); err != nil {
			t.Errorf("ValidateEndpoint(%q,%q) = %v, want nil", ep, typ, err)
		}
	}
	bad := map[string]string{
		"TN":    "19195551234",       // missing +
		"SIP":   "agent@example.com", // missing sip:
		"IP_V4": "not-an-ip",
		"FQDN":  "no spaces allowed",
	}
	for typ, ep := range bad {
		if err := ValidateEndpoint(ep, typ); err == nil {
			t.Errorf("ValidateEndpoint(%q,%q) = nil, want error", ep, typ)
		}
	}
	if err := ValidateEndpoint("x", "BOT"); err == nil {
		t.Error("unsupported endpoint type accepted, want error")
	}
}

func TestRoutePlansEqual_EmptyNormalization(t *testing.T) {
	// Live-verified: the VCP list response returns null for packages with no plan.
	empty := []interface{}{nil, map[string]interface{}{"routes": []interface{}{}}}
	for _, a := range empty {
		for _, b := range empty {
			if !RoutePlansEqual(a, b) {
				t.Errorf("RoutePlansEqual(%v,%v) = false, want true", a, b)
			}
		}
	}
}

func TestRoutePlansEqual_DetectsDifference(t *testing.T) {
	a, _ := BuildRoutePlan("host-a.example.com", "FQDN", "")
	b, _ := BuildRoutePlan("host-b.example.com", "FQDN", "")
	if RoutePlansEqual(a, b) {
		t.Error("different endpoints reported as equal")
	}
	// FQDN comparison is case-insensitive.
	c, _ := BuildRoutePlan("HOST-A.example.com", "FQDN", "")
	if !RoutePlansEqual(a, c) {
		t.Error("FQDN case difference reported as unequal")
	}
}

func TestParseRoutePlanJSON(t *testing.T) {
	plan, err := ParseRoutePlanJSON(`{"routes":[{"priority":1,"endpoints":[{"endpoint":"h.example.com","type":"FQDN"}]}]}`)
	if err != nil {
		t.Fatalf("ParseRoutePlanJSON() error = %v", err)
	}
	if _, ok := plan["routes"]; !ok {
		t.Error("parsed plan missing routes")
	}
	// Unknown top-level fields are rejected rather than silently dropped.
	if _, err := ParseRoutePlanJSON(`{"routez":[]}`); err == nil {
		t.Error("unknown field accepted, want error")
	}
}

// --- --route-plan-json structural validation ---
//
// The parsed plan is not only a request body: it is one side of the
// RoutePlansEqual comparison behind --if-not-exists and behind the
// --replace-routes guard. canonicalPlanJSON coerces a wrong-typed or absent
// field to an empty slice or 0, so a plan that got past the parser with
// "endponts" or "weigth" in it canonicalizes to a plan the caller never wrote —
// which can make a real route change read as a no-op and slip past the guard.
// Validating only that the single top-level key is "routes" left every nested
// structure unchecked.

func TestParseRoutePlanJSON_RejectsMalformedNestedStructures(t *testing.T) {
	cases := []struct {
		name string
		in   string
		// wantInErr is the field (or indexed path) the message must name, so a
		// caller can find the mistake without diffing their JSON by eye.
		wantInErr string
	}{
		{"routes is not an array", `{"routes":"not-an-array"}`, "routes"},
		{"routes is an object", `{"routes":{}}`, "routes"},
		{"routes is null", `{"routes":null}`, "routes"},
		{"routes is absent", `{}`, "routes"},
		{"whole plan is null", `null`, "routes"},
		{"routes is empty", `{"routes":[]}`, "routes"},
		{"null route entry", `{"routes":[null]}`, "routes[0]"},
		{"misspelled endpoints plus string priority", `{"routes":[{"endponts":[],"priority":"high"}]}`, "endponts"},
		{"misspelled endpoint weight", `{"routes":[{"endpoints":[{"endpoint":"x","weigth":100}]}]}`, "weigth"},
		{"string priority", `{"routes":[{"priority":"high","endpoints":[{"endpoint":"x","type":"FQDN"}]}]}`, "priority"},
		{"string weight", `{"routes":[{"endpoints":[{"endpoint":"x","type":"FQDN","weight":"heavy"}]}]}`, "weight"},
		{"boolean route name", `{"routes":[{"name":true,"endpoints":[{"endpoint":"x","type":"FQDN"}]}]}`, "name"},
		{"endpoints is not an array", `{"routes":[{"endpoints":"nope"}]}`, "endpoints"},
		{"endpoints is null", `{"routes":[{"endpoints":null}]}`, "endpoints"},
		{"endpoints is absent", `{"routes":[{"priority":1}]}`, "endpoints"},
		{"endpoints is empty", `{"routes":[{"priority":1,"endpoints":[]}]}`, "endpoints"},
		{"null endpoint entry", `{"routes":[{"endpoints":[null]}]}`, "endpoints[0]"},
		{"endpoint value missing", `{"routes":[{"endpoints":[{"type":"FQDN"}]}]}`, "endpoint"},
		{"endpoint value empty", `{"routes":[{"endpoints":[{"endpoint":"","type":"FQDN"}]}]}`, "endpoint"},
		{"endpoint type missing", `{"routes":[{"endpoints":[{"endpoint":"h.example.com"}]}]}`, "type"},
		{"endpoint type empty", `{"routes":[{"endpoints":[{"endpoint":"h.example.com","type":""}]}]}`, "type"},
		{"unknown top-level field", `{"routez":[]}`, "routez"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			plan, err := ParseRoutePlanJSON(c.in)
			if err == nil {
				t.Fatalf("ParseRoutePlanJSON(%s) = %v, want an error", c.in, plan)
			}
			if !strings.Contains(err.Error(), c.wantInErr) {
				t.Errorf("error = %q, want it to name %q", err.Error(), c.wantInErr)
			}
		})
	}
}

// TestParseRoutePlanJSON_RejectsTrailingContent covers the decoder-specific
// hazard: json.Decoder.Decode stops at the end of the first JSON value, so
// anything after it is silently dropped unless dec.More() is checked. Two
// concatenated plans must not quietly resolve to the first one.
func TestParseRoutePlanJSON_RejectsTrailingContent(t *testing.T) {
	valid := `{"routes":[{"priority":1,"endpoints":[{"endpoint":"h.example.com","type":"FQDN"}]}]}`
	for _, in := range []string{
		valid + ` {"routes":[{"priority":2,"endpoints":[{"endpoint":"other.example.com","type":"FQDN"}]}]}`,
		valid + ` garbage`,
		valid + ` []`,
	} {
		if plan, err := ParseRoutePlanJSON(in); err == nil {
			t.Errorf("ParseRoutePlanJSON(%s) = %v, want an error for trailing content", in, plan)
		}
	}
	if _, err := ParseRoutePlanJSON(``); err == nil {
		t.Error("empty --route-plan-json accepted, want an error")
	}
}

// TestParseRoutePlanJSON_WireShapeUnchanged pins the request body. Typed
// decoding is only safe if converting the typed value back into the request map
// reproduces exactly what the previous raw-map implementation sent: the plan
// goes straight onto the wire as body["originationRoutePlan"], and it is also
// one side of the --replace-routes comparison, so a changed shape is both a
// changed request and a changed guard verdict. referenceWireShape is the old
// implementation's marshaling — decode into a generic map, re-encode — so this
// asserts byte identity against the behavior being replaced, not against a
// literal someone could quietly re-baseline.
func TestParseRoutePlanJSON_WireShapeUnchanged(t *testing.T) {
	referenceWireShape := func(t *testing.T, s string) string {
		t.Helper()
		var m map[string]interface{}
		if err := json.Unmarshal([]byte(s), &m); err != nil {
			t.Fatalf("reference unmarshal(%s) error = %v", s, err)
		}
		b, err := json.Marshal(m)
		if err != nil {
			t.Fatalf("reference marshal(%s) error = %v", s, err)
		}
		return string(b)
	}

	inputs := []string{
		// Minimal: omitted route name/type and endpoint weight must stay
		// omitted, not be filled in with "" and 0.
		`{"routes":[{"priority":1,"endpoints":[{"endpoint":"h.example.com","type":"FQDN"}]}]}`,
		// Every field the shape allows.
		`{"routes":[{"priority":1,"name":"primary route","type":"WEIGHTED","endpoints":[{"endpoint":"h.example.com","type":"FQDN","weight":100}]}]}`,
		// Multiple routes and multiple endpoints, order preserved.
		`{"routes":[{"priority":1,"name":"a","type":"WEIGHTED","endpoints":[{"endpoint":"one.example.com","type":"FQDN","weight":70},{"endpoint":"+19195551234","type":"TN","weight":30}]},{"priority":2,"name":"b","type":"ANI","endpoints":[{"endpoint":"sip:agent@example.com","type":"SIP","weight":100}]}]}`,
		// Non-integer and exponent-notation numerics still marshal the way
		// encoding/json rendered them when they were plain interface{} values.
		`{"routes":[{"priority":1.5,"endpoints":[{"endpoint":"h.example.com","type":"FQDN","weight":1e2}]}]}`,
	}
	for _, in := range inputs {
		plan, err := ParseRoutePlanJSON(in)
		if err != nil {
			t.Fatalf("ParseRoutePlanJSON(%s) error = %v", in, err)
		}
		got, err := json.Marshal(plan)
		if err != nil {
			t.Fatalf("marshaling parsed plan error = %v", err)
		}
		if want := referenceWireShape(t, in); string(got) != want {
			t.Errorf("wire shape changed for %s:\n got  %s\n want %s", in, got, want)
		}
	}

	// One literal, spelled out, so the pinned bytes are visible in the test
	// and not only derivable from the reference implementation.
	plan, err := ParseRoutePlanJSON(`{"routes":[{"priority":1,"name":"primary route","type":"WEIGHTED","endpoints":[{"endpoint":"h.example.com","type":"FQDN","weight":100}]}]}`)
	if err != nil {
		t.Fatalf("ParseRoutePlanJSON() error = %v", err)
	}
	b, err := json.Marshal(plan)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	const want = `{"routes":[{"endpoints":[{"endpoint":"h.example.com","type":"FQDN","weight":100}],"name":"primary route","priority":1,"type":"WEIGHTED"}]}`
	if string(b) != want {
		t.Errorf("wire shape = %s, want %s", b, want)
	}
}

// TestParseRoutePlanJSON_StillIdempotentAgainstBuiltPlan guards the seam
// between the two plan sources: a --route-plan-json plan that describes exactly
// what BuildRoutePlan produces must still compare equal, or --if-not-exists
// would report a spurious conflict and --replace-routes would demand
// confirmation for a no-op write.
func TestParseRoutePlanJSON_StillIdempotentAgainstBuiltPlan(t *testing.T) {
	built, err := BuildRoutePlan("h.example.com", "FQDN", "primary route")
	if err != nil {
		t.Fatalf("BuildRoutePlan() error = %v", err)
	}
	parsed := mustParsePlan(t, `{"routes":[{"priority":1,"name":"primary route","type":"WEIGHTED","endpoints":[{"endpoint":"h.example.com","type":"FQDN","weight":100}]}]}`)
	if !RoutePlansEqual(built, parsed) {
		t.Error("a --route-plan-json plan identical to the flag-built plan compared unequal")
	}
	if requiresRouteReplaceConfirmation(built, parsed, false) {
		t.Error("re-writing an identical plan via --route-plan-json demanded --replace-routes")
	}
	// And a genuine difference must still be caught after the rewrite.
	different := mustParsePlan(t, `{"routes":[{"priority":1,"name":"primary route","type":"WEIGHTED","endpoints":[{"endpoint":"other.example.com","type":"FQDN","weight":100}]}]}`)
	if !requiresRouteReplaceConfirmation(built, different, false) {
		t.Error("a different plan via --route-plan-json did not require --replace-routes")
	}
}

// --- Canonicalization gap: route type, priority, and endpoint weight are all
// user-controllable via --route-plan-json (BuildRoutePlan hardcodes them, so
// only the JSON path can vary them). RoutePlansEqual/canonicalPlanJSON must
// treat a difference in any of the three as a real difference, or the
// --replace-routes guard never fires for it. ---

func mustParsePlan(t *testing.T, s string) map[string]interface{} {
	t.Helper()
	plan, err := ParseRoutePlanJSON(s)
	if err != nil {
		t.Fatalf("ParseRoutePlanJSON(%q) error = %v", s, err)
	}
	return plan
}

func TestRoutePlansEqual_DetectsRouteTypeDifference(t *testing.T) {
	weighted := mustParsePlan(t, `{"routes":[{"priority":1,"name":"primary route","type":"WEIGHTED","endpoints":[{"endpoint":"h.example.com","type":"FQDN","weight":100}]}]}`)
	ani := mustParsePlan(t, `{"routes":[{"priority":1,"name":"primary route","type":"ANI","endpoints":[{"endpoint":"h.example.com","type":"FQDN","weight":100}]}]}`)
	if RoutePlansEqual(weighted, ani) {
		t.Error("plans differing only in route type (WEIGHTED vs ANI) reported as equal — a real failover-semantics change was missed")
	}
}

func TestRoutePlansEqual_DetectsPriorityDifference(t *testing.T) {
	p1 := mustParsePlan(t, `{"routes":[{"priority":1,"name":"primary route","type":"WEIGHTED","endpoints":[{"endpoint":"h.example.com","type":"FQDN","weight":100}]}]}`)
	p2 := mustParsePlan(t, `{"routes":[{"priority":2,"name":"primary route","type":"WEIGHTED","endpoints":[{"endpoint":"h.example.com","type":"FQDN","weight":100}]}]}`)
	if RoutePlansEqual(p1, p2) {
		t.Error("plans differing only in route priority reported as equal")
	}
}

func TestRoutePlansEqual_DetectsWeightDifference(t *testing.T) {
	p1 := mustParsePlan(t, `{"routes":[{"priority":1,"name":"primary route","type":"WEIGHTED","endpoints":[{"endpoint":"h.example.com","type":"FQDN","weight":100}]}]}`)
	p2 := mustParsePlan(t, `{"routes":[{"priority":1,"name":"primary route","type":"WEIGHTED","endpoints":[{"endpoint":"h.example.com","type":"FQDN","weight":50}]}]}`)
	if RoutePlansEqual(p1, p2) {
		t.Error("plans differing only in endpoint weight reported as equal")
	}
}

func TestRoutePlansEqual_NumericComparisonNotStringComparison(t *testing.T) {
	// BuildRoutePlan hand-builds int(1)/int(100); ParseRoutePlanJSON decodes
	// float64(1)/float64(100) via encoding/json. These represent the same
	// plan and must compare equal despite the differing Go types.
	built, err := BuildRoutePlan("h.example.com", "FQDN", "")
	if err != nil {
		t.Fatalf("BuildRoutePlan() error = %v", err)
	}
	parsed := mustParsePlan(t, `{"routes":[{"priority":1,"name":"primary route","type":"WEIGHTED","endpoints":[{"endpoint":"h.example.com","type":"FQDN","weight":100}]}]}`)
	if !RoutePlansEqual(built, parsed) {
		t.Error("int vs float64 priority/weight representations of the same plan reported as unequal")
	}
}

// --- resolveRoutePlan: --route-name must not be a silent no-op ---

func TestResolveRoutePlan_RouteNameAloneErrors(t *testing.T) {
	// Setting --route-name without --route-endpoint/--route-endpoint-type must
	// error, not silently return (nil, nil) as if no route flags were set.
	plan, err := resolveRoutePlan("", "", "custom name", "")
	if err == nil {
		t.Errorf("expected error for --route-name alone, got plan=%v", plan)
	}
}

func TestResolveRoutePlan_RouteNameWithPlanJSONErrors(t *testing.T) {
	_, err := resolveRoutePlan("", "", "custom name", `{"routes":[]}`)
	if err == nil {
		t.Error("expected mutual-exclusivity error for --route-name with --route-plan-json")
	}
}

func TestResolveRoutePlan_MutualExclusivity(t *testing.T) {
	_, err := resolveRoutePlan("h.example.com", "FQDN", "", `{"routes":[]}`)
	if err == nil {
		t.Error("expected mutual-exclusivity error combining individual flags with --route-plan-json")
	}
}

func TestResolveRoutePlan_PartialFlagsError(t *testing.T) {
	if _, err := resolveRoutePlan("h.example.com", "", "", ""); err == nil {
		t.Error("expected error when --route-endpoint is given without --route-endpoint-type")
	}
}

func TestResolveRoutePlan_NoFlagsReturnsNil(t *testing.T) {
	plan, err := resolveRoutePlan("", "", "", "")
	if err != nil || plan != nil {
		t.Errorf("resolveRoutePlan() = (%v, %v), want (nil, nil)", plan, err)
	}
}

func TestResolveRoutePlan_MissingFile(t *testing.T) {
	_, err := resolveRoutePlan("", "", "", "@/no/such/file-for-testing.json")
	if err == nil {
		t.Error("expected error reading a nonexistent --route-plan-json file")
	}
}

// --- requiresRouteReplaceConfirmation: the four --replace-routes guard
// transitions the update command relies on, including the --replace-routes
// override itself (previously a bare "&& !updateReplaceRoutes" left outside
// the function and untested). ---

func TestRequiresRouteReplaceConfirmation_EmptyExistingAllowed(t *testing.T) {
	plan, _ := BuildRoutePlan("h.example.com", "FQDN", "")
	if requiresRouteReplaceConfirmation(nil, plan, false) {
		t.Error("empty existing plan should not require confirmation")
	}
}

func TestRequiresRouteReplaceConfirmation_IdenticalAllowed(t *testing.T) {
	existing, _ := BuildRoutePlan("h.example.com", "FQDN", "")
	plan, _ := BuildRoutePlan("h.example.com", "FQDN", "")
	if requiresRouteReplaceConfirmation(existing, plan, false) {
		t.Error("identical existing plan should not require confirmation")
	}
}

func TestRequiresRouteReplaceConfirmation_DifferentBlockedWithoutFlag(t *testing.T) {
	existing, _ := BuildRoutePlan("old.example.com", "FQDN", "")
	plan, _ := BuildRoutePlan("new.example.com", "FQDN", "")
	if !requiresRouteReplaceConfirmation(existing, plan, false) {
		t.Error("different non-empty existing plan should require confirmation when --replace-routes is not set")
	}
}

func TestRequiresRouteReplaceConfirmation_DifferentAllowedWithFlag(t *testing.T) {
	existing, _ := BuildRoutePlan("old.example.com", "FQDN", "")
	plan, _ := BuildRoutePlan("new.example.com", "FQDN", "")
	if requiresRouteReplaceConfirmation(existing, plan, true) {
		t.Error("--replace-routes=true should allow overwriting a different non-empty existing plan")
	}
}

// --- findAllByName: duplicate detection never guesses ---

func TestFindAllByName_ZeroOneAndMultipleMatches(t *testing.T) {
	list := []interface{}{
		map[string]interface{}{"name": "Prod VCP", "voiceConfigurationPackageId": "vcp-1"},
		map[string]interface{}{"name": "Prod VCP", "voiceConfigurationPackageId": "vcp-2"},
		map[string]interface{}{"name": "Other VCP", "voiceConfigurationPackageId": "vcp-3"},
	}
	if got := findAllByName(list, "name", "Prod VCP"); len(got) != 2 {
		t.Errorf("findAllByName() = %d matches, want 2", len(got))
	}
	if got := findAllByName(list, "name", "Other VCP"); len(got) != 1 {
		t.Errorf("findAllByName() = %d matches, want 1", len(got))
	}
	if got := findAllByName(list, "name", "No Such VCP"); len(got) != 0 {
		t.Errorf("findAllByName() = %d matches, want 0", len(got))
	}
}

// --- vcpConflict: --if-not-exists must compare, not guess ---

func TestVCPConflict_NoFlagsSpecifiedAlwaysCompatible(t *testing.T) {
	existing := map[string]interface{}{
		"voiceConfigurationPackageId": "vcp-1",
		"description":                 "something else entirely",
		"httpVoiceV2ApplicationId":    "other-app",
	}
	// Neither --description nor --app-id was passed, and no route plan was
	// requested: nothing the caller specified conflicts.
	if err := vcpConflict(existing, "Prod VCP", false, "", false, "", nil); err != nil {
		t.Errorf("vcpConflict() = %v, want nil when caller specified no comparable fields", err)
	}
}

func TestVCPConflict_DescriptionMismatch(t *testing.T) {
	existing := map[string]interface{}{
		"voiceConfigurationPackageId": "vcp-1",
		"description":                 "existing description",
	}
	if err := vcpConflict(existing, "Prod VCP", true, "requested description", false, "", nil); err == nil {
		t.Error("expected conflict error for mismatched description")
	}
	if err := vcpConflict(existing, "Prod VCP", true, "existing description", false, "", nil); err != nil {
		t.Errorf("matching description should not conflict, got %v", err)
	}
}

func TestVCPConflict_AppIDMismatch(t *testing.T) {
	existing := map[string]interface{}{
		"voiceConfigurationPackageId": "vcp-1",
		"httpVoiceV2ApplicationId":    "app-existing",
	}
	if err := vcpConflict(existing, "Prod VCP", false, "", true, "app-requested", nil); err == nil {
		t.Error("expected conflict error for mismatched app ID")
	}
	if err := vcpConflict(existing, "Prod VCP", false, "", true, "app-existing", nil); err != nil {
		t.Errorf("matching app ID should not conflict, got %v", err)
	}
}

func TestVCPConflict_AbsentNullEmptyAreEquivalent(t *testing.T) {
	existing := map[string]interface{}{"voiceConfigurationPackageId": "vcp-1"} // description absent
	if err := vcpConflict(existing, "Prod VCP", true, "", false, "", nil); err != nil {
		t.Errorf("absent existing description vs requested \"\" should not conflict, got %v", err)
	}
}

func TestVCPConflict_RoutePlanMismatch(t *testing.T) {
	existing := map[string]interface{}{
		"voiceConfigurationPackageId": "vcp-1",
		"originationRoutePlan":        nil,
	}
	plan, _ := BuildRoutePlan("h.example.com", "FQDN", "")
	if err := vcpConflict(existing, "Prod VCP", false, "", false, "", plan); err == nil {
		t.Error("expected conflict error when existing plan is empty but caller requested a route")
	}
}

// --- exit codes: every vcp state conflict must be exit 4, not exit 1 ---

// TestVCPConflict_ExitsConflictNotGeneral covers the agent contract. These
// errors all say "the VCP exists but differs — update it explicitly," which is
// exactly the conflict signal (4). They were bare fmt.Errorf, so they exited 1,
// indistinguishable from an unexpected failure. An agent branching on 1 vs 4
// cannot tell "go reconcile" from "something broke."
func TestVCPConflict_ExitsConflictNotGeneral(t *testing.T) {
	plan, err := BuildRoutePlan("h.example.com", "FQDN", "")
	if err != nil {
		t.Fatalf("BuildRoutePlan() error = %v", err)
	}
	existing := map[string]interface{}{
		"voiceConfigurationPackageId": "vcp-1",
		"description":                 "existing description",
		"httpVoiceV2ApplicationId":    "app-existing",
		"originationRoutePlan":        nil,
	}

	cases := []struct {
		name string
		err  error
	}{
		{"description mismatch", vcpConflict(existing, "Prod VCP", true, "requested", false, "", nil)},
		{"app id mismatch", vcpConflict(existing, "Prod VCP", false, "", true, "app-requested", nil)},
		{"route plan mismatch", vcpConflict(existing, "Prod VCP", false, "", false, "", plan)},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if c.err == nil {
				t.Fatal("vcpConflict() = nil, want a conflict")
			}
			if got := cmdutil.ExitCodeForError(c.err); got != cmdutil.ExitConflict {
				t.Errorf("ExitCodeForError() = %d, want ExitConflict (%d); err = %v", got, cmdutil.ExitConflict, c.err)
			}
			// The remediation text must be preserved verbatim: only the type changed.
			if !strings.Contains(c.err.Error(), "update it explicitly") {
				t.Errorf("error = %q, want the existing remediation text", c.err.Error())
			}
		})
	}
}
