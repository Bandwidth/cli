package api

import (
	"net/url"
	"strconv"
)

// Filter operators. The API uses OpenAPI deepObject encoding, so a filter is
// sent as field[op]=value. Sending the bare field=value form is accepted by
// the server and then IGNORED — the result looks successful but is unfiltered.
//
// This is the full set of operators the tendlc and customerprofile deepObject
// filter parameters document (see ~/Developer/api-specs/external/tendlc.yml,
// components.parameters.*Param): string/enum fields support eq and/or
// contains depending on the field, and date fields (createdDate,
// modifiedDate) support gt and lt — NOT gte/lte, which do not appear
// anywhere in the spec.
const (
	OpEq       = "eq"
	OpContains = "contains"
	OpGt       = "gt"
	OpLt       = "lt"
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
