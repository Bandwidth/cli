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
