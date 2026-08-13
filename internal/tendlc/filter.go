// Package tendlc wraps Bandwidth's v2 A2P Campaign Management (10DLC
// Registration Center) API.
//
// Responses are returned as map[string]any rather than typed structs:
// production returns fields absent from the published spec, and
// encoding/json silently drops unknown fields when decoding into a struct.
// Requests are typed; responses are lossless.
package tendlc

import (
	"net/url"
	"strconv"
)

// Filter operators. The API uses OpenAPI deepObject encoding, so a filter is
// sent as field[op]=value. Sending the bare field=value form is accepted by
// the server and then IGNORED — the result looks successful but is unfiltered.
const (
	OpEq       = "eq"
	OpContains = "contains"
)

// Filter is one deepObject query filter.
type Filter struct {
	Field string
	Op    string
	Value string
}

// EncodeQuery builds the query string for a list endpoint. Returns "" when
// there is nothing to send. Parameters are sorted so output is deterministic
// and testable. Filters with an empty Value are omitted entirely rather than
// sent blank, which the API treats as a match-nothing filter.
//
// Filters are keyed by Field+Op, so a later filter with the same Field and Op
// overwrites an earlier one. Callers needing multiple constraints on one field
// (e.g. createdDate[gte] and createdDate[lte]) must use distinct Ops.
func EncodeQuery(limit, offset int, filters []Filter) string {
	v := url.Values{}
	if limit > 0 {
		v.Set("limit", strconv.Itoa(limit))
	}
	if offset > 0 {
		v.Set("offset", strconv.Itoa(offset))
	}
	for _, f := range filters {
		if f.Field == "" || f.Value == "" {
			continue
		}
		op := f.Op
		if op == "" {
			op = OpEq
		}
		v.Set(f.Field+"["+op+"]", f.Value)
	}
	if len(v) == 0 {
		return ""
	}
	// url.Values.Encode sorts by key, so output is deterministic and the
	// exact-string tests above are stable.
	return "?" + v.Encode()
}
