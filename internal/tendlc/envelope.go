package tendlc

import (
	"encoding/json"
	"fmt"
)

// Page is the pagination block present on every v2 A2P response.
type Page struct {
	Number        int `json:"pageNumber"`
	Size          int `json:"pageSize"`
	TotalElements int `json:"totalElements"`
	TotalPages    int `json:"totalPages"`
}

// Truncated reports whether more records exist beyond the ones returned.
// Driven by totalElements rather than by observing a short page, because a
// full page is not evidence of more and a short page is not evidence of none.
func (p *Page) Truncated(returned int) bool {
	if p == nil {
		return false
	}
	return returned < p.TotalElements
}

// Envelope is the {data, page, errors, links} wrapper the API returns.
// Data is kept as any because its shape varies per endpoint: an object for
// brand detail, an array for lists and history. The published spec says
// array in both cases; production disagrees, so callers state which they
// expect and get an error rather than a silent zero value if wrong.
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

// List returns data as an array. Errors if the endpoint returned an object.
func (e *Envelope) List() ([]any, error) {
	if e.Data == nil {
		return []any{}, nil
	}
	arr, ok := e.Data.([]any)
	if !ok {
		return nil, fmt.Errorf("expected a list response, got %T", e.Data)
	}
	return arr, nil
}

// Object returns data as a single resource. Errors if the endpoint returned
// an array.
func (e *Envelope) Object() (map[string]any, error) {
	obj, ok := e.Data.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("expected a single-resource response, got %T", e.Data)
	}
	return obj, nil
}
