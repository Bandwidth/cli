package vcp

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"regexp"
	"strings"
)

// supportedEndpointTypes are the origination-route endpoint types the CLI writes.
// INTEGRATION and BOT are Bandwidth-managed and out of scope.
var supportedEndpointTypes = map[string]bool{"TN": true, "SIP": true, "IP_V4": true, "FQDN": true}

var (
	e164Re = regexp.MustCompile(`^\+[1-9]\d{1,14}$`)
	fqdnRe = regexp.MustCompile(`^[A-Za-z0-9]([A-Za-z0-9-]*[A-Za-z0-9])?(\.[A-Za-z0-9]([A-Za-z0-9-]*[A-Za-z0-9])?)+$`)
)

// ValidateEndpoint checks an endpoint value against its declared type.
func ValidateEndpoint(endpoint, endpointType string) error {
	if !supportedEndpointTypes[endpointType] {
		return fmt.Errorf("unsupported --route-endpoint-type %q; use one of TN, SIP, IP_V4, FQDN", endpointType)
	}
	switch endpointType {
	case "TN":
		if !e164Re.MatchString(endpoint) {
			return fmt.Errorf("endpoint %q must be E.164 (e.g. +19195551234) for type TN", endpoint)
		}
	case "SIP":
		if !strings.HasPrefix(endpoint, "sip:") && !strings.HasPrefix(endpoint, "sips:") {
			return fmt.Errorf("endpoint %q must be a SIP URI (e.g. sip:agent@example.com) for type SIP", endpoint)
		}
	case "IP_V4":
		host := endpoint
		if i := strings.Index(host, "/"); i >= 0 {
			host = host[:i]
		}
		if ip := net.ParseIP(host); ip == nil || ip.To4() == nil {
			return fmt.Errorf("endpoint %q must be an IPv4 address or CIDR for type IP_V4", endpoint)
		}
	case "FQDN":
		if !fqdnRe.MatchString(endpoint) {
			return fmt.Errorf("endpoint %q must be a hostname (e.g. host.example.com) for type FQDN", endpoint)
		}
	}
	return nil
}

// BuildRoutePlan produces a single-route, single-endpoint origination route plan.
// The route's own type is always WEIGHTED; the endpoint carries the kind of
// destination. Priority and weight are not exposed as flags in v1 because
// neither is meaningful with one route and one endpoint.
func BuildRoutePlan(endpoint, endpointType, routeName string) (map[string]interface{}, error) {
	if err := ValidateEndpoint(endpoint, endpointType); err != nil {
		return nil, err
	}
	if routeName == "" {
		routeName = "primary route"
	}
	return map[string]interface{}{
		"routes": []map[string]interface{}{{
			"priority": 1,
			"name":     routeName,
			"type":     "WEIGHTED",
			"endpoints": []map[string]interface{}{{
				"endpoint": endpoint,
				"type":     endpointType,
				"weight":   100,
			}},
		}},
	}, nil
}

// ParseRoutePlanJSON parses the value of --route-plan-json, which is the
// originationRoutePlan object itself (i.e. {"routes":[...]}).
func ParseRoutePlanJSON(s string) (map[string]interface{}, error) {
	var plan map[string]interface{}
	dec := json.NewDecoder(strings.NewReader(s))
	dec.DisallowUnknownFields()
	if err := json.Unmarshal([]byte(s), &plan); err != nil {
		return nil, fmt.Errorf("parsing --route-plan-json: %w", err)
	}
	for k := range plan {
		if k != "routes" {
			return nil, fmt.Errorf("unknown field %q in --route-plan-json; expected the originationRoutePlan object, e.g. {\"routes\":[...]}", k)
		}
	}
	if _, ok := plan["routes"]; !ok {
		return nil, fmt.Errorf("--route-plan-json must contain a \"routes\" array")
	}
	return plan, nil
}

// isEmptyPlan treats absent, null, and {"routes":[]} as the same state — the
// API returns null for packages with no plan.
func isEmptyPlan(p interface{}) bool {
	if p == nil {
		return true
	}
	m, ok := normalizePlan(p)
	if !ok {
		return false
	}
	routes, _ := m["routes"].([]interface{})
	return len(routes) == 0
}

// normalizePlan converts any plan representation into a generic map via JSON so
// map[string]interface{} and map[string]any compare consistently.
func normalizePlan(p interface{}) (map[string]interface{}, bool) {
	b, err := json.Marshal(p)
	if err != nil {
		return nil, false
	}
	var m map[string]interface{}
	if err := json.Unmarshal(b, &m); err != nil {
		return nil, false
	}
	return m, true
}

// RoutePlansEqual compares two route plans for the purposes of --if-not-exists
// and the --replace-routes guard. Route and endpoint order is significant;
// FQDN endpoint values compare case-insensitively.
func RoutePlansEqual(a, b interface{}) bool {
	if isEmptyPlan(a) && isEmptyPlan(b) {
		return true
	}
	if isEmptyPlan(a) != isEmptyPlan(b) {
		return false
	}
	na, okA := normalizePlan(a)
	nb, okB := normalizePlan(b)
	if !okA || !okB {
		return false
	}
	return canonicalPlanJSON(na) == canonicalPlanJSON(nb)
}

// canonicalPlanJSON renders only the user-controllable fields, lowercasing FQDN
// endpoints so case differences do not read as changes.
func canonicalPlanJSON(m map[string]interface{}) string {
	routes, _ := m["routes"].([]interface{})
	var out []map[string]interface{}
	for _, r := range routes {
		rm, _ := r.(map[string]interface{})
		var eps []map[string]interface{}
		raw, _ := rm["endpoints"].([]interface{})
		for _, e := range raw {
			em, _ := e.(map[string]interface{})
			ep, _ := em["endpoint"].(string)
			typ, _ := em["type"].(string)
			if typ == "FQDN" {
				ep = strings.ToLower(ep)
			}
			eps = append(eps, map[string]interface{}{"endpoint": ep, "type": typ})
		}
		name, _ := rm["name"].(string)
		out = append(out, map[string]interface{}{"name": name, "endpoints": eps})
	}
	b, _ := json.Marshal(out)
	return string(b)
}

// resolveRoutePlan turns the route flags into a plan, or nil when none were set.
// The individual flags and --route-plan-json are mutually exclusive.
func resolveRoutePlan(endpoint, endpointType, routeName, planJSON string) (map[string]interface{}, error) {
	usingFlags := endpoint != "" || endpointType != ""
	if usingFlags && planJSON != "" {
		return nil, fmt.Errorf("--route-plan-json cannot be combined with --route-endpoint/--route-endpoint-type")
	}
	if planJSON != "" {
		s := planJSON
		if strings.HasPrefix(s, "@") {
			b, err := os.ReadFile(strings.TrimPrefix(s, "@"))
			if err != nil {
				return nil, fmt.Errorf("reading --route-plan-json file: %w", err)
			}
			s = string(b)
		}
		return ParseRoutePlanJSON(s)
	}
	if !usingFlags {
		return nil, nil
	}
	if endpoint == "" || endpointType == "" {
		return nil, fmt.Errorf("--route-endpoint and --route-endpoint-type must be provided together")
	}
	return BuildRoutePlan(endpoint, endpointType, routeName)
}
