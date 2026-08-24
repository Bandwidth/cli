package api

import "testing"

func TestParseEnvelopeArrayData(t *testing.T) {
	body := []byte(`{"data":[{"brandId":"B1"},{"brandId":"B2"}],
		"page":{"pageNumber":0,"pageSize":2,"totalElements":10,"totalPages":5}}`)
	env, err := ParseEnvelope(body)
	if err != nil {
		t.Fatalf("ParseEnvelope: %v", err)
	}
	list, err := env.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 2 {
		t.Errorf("len(list) = %d, want 2", len(list))
	}
	if env.Page == nil || env.Page.TotalElements != 10 || env.Page.TotalPages != 5 {
		t.Errorf("Page = %+v, want TotalElements 10 / TotalPages 5", env.Page)
	}
}

// brand get returns data as an OBJECT, contradicting the published spec.
// Verified live on 9901287. Prod wins.
func TestParseEnvelopeObjectData(t *testing.T) {
	body := []byte(`{"data":{"brandId":"BEXMPL8","universalEin":"US_123456789"}}`)
	env, err := ParseEnvelope(body)
	if err != nil {
		t.Fatalf("ParseEnvelope: %v", err)
	}
	obj, err := env.Object()
	if err != nil {
		t.Fatalf("Object: %v", err)
	}
	if obj["brandId"] != "BEXMPL8" {
		t.Errorf("brandId = %v, want BEXMPL8", obj["brandId"])
	}
	// Undocumented field must survive — responses are lossless.
	if obj["universalEin"] != "US_123456789" {
		t.Errorf("universalEin was dropped: %v", obj["universalEin"])
	}
}

func TestEnvelopeShapeMismatchIsAnError(t *testing.T) {
	env, _ := ParseEnvelope([]byte(`{"data":{"brandId":"B1"}}`))
	if _, err := env.List(); err == nil {
		t.Error("List() on object data should error, not silently return empty")
	}
	env2, _ := ParseEnvelope([]byte(`{"data":[]}`))
	if _, err := env2.Object(); err == nil {
		t.Error("Object() on array data should error")
	}
}

// Data that is null, or a data key that is entirely absent, must fail
// closed rather than being read as "no results" — genuinely empty lists
// come back from the API as "data":[], so nil is anomalous, not normal.
func TestEnvelopeNilDataIsAnError(t *testing.T) {
	env, err := ParseEnvelope([]byte(`{"data":null}`))
	if err != nil {
		t.Fatalf("ParseEnvelope: %v", err)
	}
	if _, err := env.List(); err == nil {
		t.Error("List() on explicit null data should error, not report an empty list")
	}

	env2, err := ParseEnvelope([]byte(`{}`))
	if err != nil {
		t.Fatalf("ParseEnvelope: %v", err)
	}
	if _, err := env2.List(); err == nil {
		t.Error("List() on a body with no data key should error, not report an empty list")
	}
}

func TestPageTruncated(t *testing.T) {
	tests := []struct {
		name          string
		page          *Page
		returnedSoFar int
		want          bool
	}{
		{"more pages remain", &Page{TotalElements: 10, TotalPages: 5, Size: 2}, 2, true},
		{"everything returned", &Page{TotalElements: 2, TotalPages: 1, Size: 50}, 2, false},
		{"empty result", &Page{TotalElements: 0, TotalPages: 0, Size: 50}, 0, false},
		// Cumulative count across all pages walked equals the total: nothing
		// left, even though a naive per-page count would have said otherwise
		// on earlier pages.
		{"cumulative count reaches total on last page", &Page{TotalElements: 10, TotalPages: 5, Size: 2}, 10, false},
		{"nil page is never truncated", nil, 2, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.page.Truncated(tt.returnedSoFar); got != tt.want {
				t.Errorf("Truncated(%d) = %v, want %v", tt.returnedSoFar, got, tt.want)
			}
		})
	}
}

// Missing page metadata must be detectable so callers can fail closed
// rather than silently emitting a partial list.
func TestParseEnvelopeMissingPageIsNil(t *testing.T) {
	env, err := ParseEnvelope([]byte(`{"data":[]}`))
	if err != nil {
		t.Fatalf("ParseEnvelope: %v", err)
	}
	if env.Page != nil {
		t.Errorf("Page = %+v, want nil when absent", env.Page)
	}
}
