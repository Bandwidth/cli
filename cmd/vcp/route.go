package vcp

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
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

// routePlanInput, routeInput, and endpointInput are the typed shape of the
// originationRoutePlan object that --route-plan-json accepts. Decoding into
// these with DisallowUnknownFields — rather than into a bare
// map[string]interface{} — is what turns a misspelled or wrong-typed NESTED
// field into an error instead of a silent coercion. canonicalPlanJSON reads
// only the fields it knows about and normalizes anything missing or of the
// wrong type to an empty slice or 0, so a plan carrying "endponts" or "weigth"
// canonicalizes to a *different* plan than the caller wrote — and that
// canonical form is what --if-not-exists compares and what the
// --replace-routes destructive-write guard is decided from. Rejecting the input
// up front is the only way that comparison can be trusted.
//
// Every field is a pointer so presence is distinguishable from the zero value:
// --route-plan-json is a pass-through for callers who need fields the
// individual flags do not expose, so a field the caller omitted must stay
// omitted in the request body instead of being sent as 0 or "". Numerics are
// *float64 — the exact type encoding/json produces for a JSON number decoded
// into an interface{} — so the request map built below marshals to byte-identical
// output to what the previous raw-map implementation put on the wire.
type routePlanInput struct {
	// A pointer to a slice so an absent "routes", a null "routes", and an
	// empty one are all distinguishable from a populated one.
	Routes *[]*routeInput `json:"routes"`
}

type routeInput struct {
	Priority  *float64          `json:"priority"`
	Name      *string           `json:"name"`
	Type      *string           `json:"type"`
	Endpoints *[]*endpointInput `json:"endpoints"`
}

type endpointInput struct {
	Endpoint *string  `json:"endpoint"`
	Type     *string  `json:"type"`
	Weight   *float64 `json:"weight"`
}

// routePlanShapeHint is the one place the accepted shape is spelled out, so
// every rejection points the caller at the same example.
const routePlanShapeHint = `expected the originationRoutePlan object, e.g. ` +
	`{"routes":[{"priority":1,"name":"primary route","type":"WEIGHTED",` +
	`"endpoints":[{"endpoint":"host.example.com","type":"FQDN","weight":100}]}]}`

// ParseRoutePlanJSON parses the value of --route-plan-json, which is the
// originationRoutePlan object itself (i.e. {"routes":[...]}).
//
// Validation is strict on purpose. The parsed plan is not just a request body:
// it is one side of the RoutePlansEqual comparison behind --if-not-exists and
// behind the --replace-routes guard that stands between a caller and an
// unintended overwrite of a live route plan. A field this function lets through
// unvalidated is a field that participates in that comparison with a coerced
// value, which can make a real change look like a no-op.
func ParseRoutePlanJSON(s string) (map[string]interface{}, error) {
	dec := json.NewDecoder(strings.NewReader(s))
	dec.DisallowUnknownFields()

	var in routePlanInput
	if err := dec.Decode(&in); err != nil {
		if errors.Is(err, io.EOF) {
			return nil, fmt.Errorf("--route-plan-json is empty; %s", routePlanShapeHint)
		}
		return nil, fmt.Errorf("parsing --route-plan-json: %s", describeRoutePlanDecodeError(err))
	}
	// Decode stops at the end of the first JSON value, so `{...} {...}` or
	// `{...} garbage` would otherwise be accepted with the trailing content
	// silently discarded.
	if dec.More() {
		return nil, fmt.Errorf("parsing --route-plan-json: unexpected trailing content after the JSON object; %s", routePlanShapeHint)
	}

	if in.Routes == nil {
		return nil, fmt.Errorf("--route-plan-json must contain a \"routes\" array; %s", routePlanShapeHint)
	}
	if len(*in.Routes) == 0 {
		return nil, fmt.Errorf("\"routes\" in --route-plan-json is empty; a route plan must contain at least one route")
	}

	routes := make([]interface{}, 0, len(*in.Routes))
	for i, r := range *in.Routes {
		if r == nil {
			return nil, fmt.Errorf("routes[%d] in --route-plan-json is null; each route must be an object", i)
		}
		route := make(map[string]interface{}, 4)
		if r.Priority != nil {
			route["priority"] = *r.Priority
		}
		if r.Name != nil {
			route["name"] = *r.Name
		}
		if r.Type != nil {
			route["type"] = *r.Type
		}
		if r.Endpoints == nil {
			return nil, fmt.Errorf("routes[%d] in --route-plan-json is missing its \"endpoints\" array", i)
		}
		// A route with no endpoints cannot route a call, and it canonicalizes
		// to the same empty endpoint list as a route whose endpoints were
		// typo'd away — exactly the coercion this parser exists to prevent.
		if len(*r.Endpoints) == 0 {
			return nil, fmt.Errorf("routes[%d].endpoints in --route-plan-json is empty; a route must contain at least one endpoint", i)
		}
		endpoints := make([]interface{}, 0, len(*r.Endpoints))
		for j, e := range *r.Endpoints {
			if e == nil {
				return nil, fmt.Errorf("routes[%d].endpoints[%d] in --route-plan-json is null; each endpoint must be an object", i, j)
			}
			if e.Endpoint == nil || *e.Endpoint == "" {
				return nil, fmt.Errorf("routes[%d].endpoints[%d] in --route-plan-json is missing \"endpoint\"", i, j)
			}
			if e.Type == nil || *e.Type == "" {
				return nil, fmt.Errorf("routes[%d].endpoints[%d] in --route-plan-json is missing \"type\" (TN, SIP, IP_V4, or FQDN)", i, j)
			}
			endpoint := map[string]interface{}{
				"endpoint": *e.Endpoint,
				"type":     *e.Type,
			}
			if e.Weight != nil {
				endpoint["weight"] = *e.Weight
			}
			endpoints = append(endpoints, endpoint)
		}
		route["endpoints"] = endpoints
		routes = append(routes, route)
	}
	return map[string]interface{}{"routes": routes}, nil
}

// describeRoutePlanDecodeError turns encoding/json's decode errors into
// messages that name the offending field instead of leaking Go type names at
// the caller. A wrong type arrives as *json.UnmarshalTypeError, whose Field is
// the dotted JSON path (e.g. "routes.endpoints.weight"); an unknown field
// arrives as a plain error already carrying the field name.
func describeRoutePlanDecodeError(err error) string {
	var typeErr *json.UnmarshalTypeError
	if errors.As(err, &typeErr) {
		if typeErr.Field != "" {
			return fmt.Sprintf("field %q has the wrong type (got JSON %s); %s", typeErr.Field, typeErr.Value, routePlanShapeHint)
		}
		return fmt.Sprintf("got JSON %s; %s", typeErr.Value, routePlanShapeHint)
	}
	msg := strings.TrimPrefix(err.Error(), "json: ")
	if strings.HasPrefix(msg, "unknown field") {
		return fmt.Sprintf("%s; %s", msg, routePlanShapeHint)
	}
	return msg
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

// canonicalPlanJSON renders the user-controllable fields — including route
// priority/type and endpoint weight, which a caller can set via
// --route-plan-json even though the individual flags never vary them — so
// the guard and --if-not-exists detect every field a caller can actually
// change. FQDN endpoint values are lowercased so case differences do not
// read as changes; priority and weight are normalized to float64 so a
// JSON-decoded 1.0 and a hand-built int 1 compare equal.
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
			eps = append(eps, map[string]interface{}{
				"endpoint": ep,
				"type":     typ,
				"weight":   toFloat(em["weight"]),
			})
		}
		name, _ := rm["name"].(string)
		routeType, _ := rm["type"].(string)
		out = append(out, map[string]interface{}{
			"name":      name,
			"type":      routeType,
			"priority":  toFloat(rm["priority"]),
			"endpoints": eps,
		})
	}
	b, _ := json.Marshal(out)
	return string(b)
}

// toFloat normalizes a route/endpoint numeric field (priority, weight) to
// float64 so values coming from JSON decoding (always float64) and values
// from a hand-built map (often int, e.g. BuildRoutePlan's literals) compare
// equal. A missing or non-numeric value normalizes to 0.
func toFloat(v interface{}) float64 {
	switch n := v.(type) {
	case float64:
		return n
	case int:
		return float64(n)
	case int64:
		return float64(n)
	case json.Number:
		f, _ := n.Float64()
		return f
	default:
		return 0
	}
}

// requiresRouteReplaceConfirmation reports whether the write must be refused:
// true only when a non-empty existing plan would be replaced by a different
// one AND the caller has not passed --replace-routes. An empty existing plan
// (nothing to destroy), an identical one (no-op write), or an explicit
// replaceConfirmed=true (the caller's override) all return false. The
// override is folded in here — not left as a separate "&&" at the call
// site — so the complete guard decision, including whether --replace-routes
// was honored, is a single unit-testable function rather than a boolean
// expression split across this file and runUpdate.
func requiresRouteReplaceConfirmation(existingPlan, plan interface{}, replaceConfirmed bool) bool {
	if replaceConfirmed {
		return false
	}
	return !isEmptyPlan(existingPlan) && !RoutePlansEqual(existingPlan, plan)
}

// resolveRoutePlan turns the route flags into a plan, or nil when none were set.
// The individual flags and --route-plan-json are mutually exclusive.
// routeName counts as "using flags" too — otherwise a caller who sets only
// --route-name gets no plan and no error, a silent no-op on a flag they
// explicitly set.
func resolveRoutePlan(endpoint, endpointType, routeName, planJSON string) (map[string]interface{}, error) {
	usingFlags := endpoint != "" || endpointType != "" || routeName != ""
	if usingFlags && planJSON != "" {
		return nil, fmt.Errorf("--route-plan-json cannot be combined with --route-endpoint/--route-endpoint-type/--route-name")
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
		return nil, fmt.Errorf("--route-endpoint and --route-endpoint-type must be provided together (--route-name alone does not build a route)")
	}
	return BuildRoutePlan(endpoint, endpointType, routeName)
}
