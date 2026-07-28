package vcp

import "testing"

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
// transitions the update command relies on. ---

func TestRequiresRouteReplaceConfirmation_EmptyExistingAllowed(t *testing.T) {
	plan, _ := BuildRoutePlan("h.example.com", "FQDN", "")
	if requiresRouteReplaceConfirmation(nil, plan) {
		t.Error("empty existing plan should not require confirmation")
	}
}

func TestRequiresRouteReplaceConfirmation_IdenticalAllowed(t *testing.T) {
	existing, _ := BuildRoutePlan("h.example.com", "FQDN", "")
	plan, _ := BuildRoutePlan("h.example.com", "FQDN", "")
	if requiresRouteReplaceConfirmation(existing, plan) {
		t.Error("identical existing plan should not require confirmation")
	}
}

func TestRequiresRouteReplaceConfirmation_DifferentBlocked(t *testing.T) {
	existing, _ := BuildRoutePlan("old.example.com", "FQDN", "")
	plan, _ := BuildRoutePlan("new.example.com", "FQDN", "")
	if !requiresRouteReplaceConfirmation(existing, plan) {
		t.Error("different non-empty existing plan should require confirmation")
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
