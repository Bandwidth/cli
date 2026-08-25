package api

import (
	"encoding/json"
	"fmt"
)

// Page is the pagination block present on every Bandwidth v2 response that
// supports pagination (10DLC A2P registration and customer profiles, among
// others).
type Page struct {
	Number        int `json:"pageNumber"`
	Size          int `json:"pageSize"`
	TotalElements int `json:"totalElements"`
	TotalPages    int `json:"totalPages"`
}

// Truncated reports whether more records exist beyond the ones retrieved so
// far. returnedSoFar must be the CUMULATIVE count of records collected across
// every page fetched up to this point — not the length of the current page.
// Passing a per-page count gives a correct answer on the first page and a
// wrong one on every subsequent page: e.g. on the last of 5 pages of 2
// records each, a per-page count of 2 against TotalElements 10 would report
// "truncated" even though nothing remains.
// Driven by totalElements rather than by observing a short page, because a
// full page is not evidence of more and a short page is not evidence of none.
func (p *Page) Truncated(returnedSoFar int) bool {
	if p == nil {
		return false
	}
	return returnedSoFar < p.TotalElements
}

// Envelope is the {data, page, errors, links} wrapper the API returns.
// Data is kept as any because its shape varies per endpoint: an object for
// brand detail, an array for lists and history. The published spec says
// array in both cases; production disagrees, so callers state which they
// expect and get an error rather than a silent zero value if wrong.
//
// Responses are returned as map[string]any rather than typed structs:
// production returns fields absent from the published spec, and
// encoding/json silently drops unknown fields when decoding into a struct.
// Requests are typed; responses are lossless.
type Envelope struct {
	Data any
	Page *Page
}

// ParseEnvelope decodes a response body into an Envelope.
func ParseEnvelope(body []byte) (*Envelope, error) {
	var raw struct {
		Data any   `json:"data"`
		Page *Page `json:"page"`
	}
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("decoding response envelope: %w", err)
	}
	return &Envelope{Data: raw.Data, Page: raw.Page}, nil
}

// List returns data as an array. Errors if the endpoint returned an object,
// or if data was null or absent — genuinely empty lists come back from the
// API as "data":[], so a nil data field is an anomaly, not an empty result,
// and must not be silently treated as one.
func (e *Envelope) List() ([]any, error) {
	arr, ok := e.Data.([]any)
	if !ok {
		return nil, fmt.Errorf("response data must be an array, got %s", dataShape(e.Data))
	}
	return arr, nil
}

// Object returns data as a single resource. Errors if the endpoint returned
// an array, or if data was null or absent.
func (e *Envelope) Object() (map[string]any, error) {
	obj, ok := e.Data.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("response data must be an object, got %s", dataShape(e.Data))
	}
	return obj, nil
}

// dataShape describes the JSON shape of data in terms a CLI user can act on,
// rather than a Go type name.
func dataShape(data any) string {
	switch data.(type) {
	case nil:
		return "null"
	case []any:
		return "an array"
	case map[string]any:
		return "an object"
	default:
		return fmt.Sprintf("%T", data)
	}
}
