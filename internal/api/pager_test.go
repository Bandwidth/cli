package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"testing"
)

// envFor builds an Envelope like the API returns: a data array plus page metadata.
func envFor(t *testing.T, items int, offset, total int) *Envelope {
	t.Helper()
	arr := make([]any, 0, items)
	for i := 0; i < items; i++ {
		arr = append(arr, map[string]any{"id": fmt.Sprintf("p%d", offset+i)})
	}
	body, _ := json.Marshal(map[string]any{
		"data": arr,
		"page": map[string]any{"pageSize": items, "totalElements": total},
	})
	env, err := ParseEnvelope(body)
	if err != nil {
		t.Fatalf("ParseEnvelope: %v", err)
	}
	return env
}

func TestForEachPageWalksEveryPage(t *testing.T) {
	var offsets []int
	var seen []string
	err := ForEachPage(func(limit, offset int) (*Envelope, error) {
		offsets = append(offsets, offset)
		switch offset {
		case 0:
			return envFor(t, 2, 0, 5), nil
		case 2:
			return envFor(t, 2, 2, 5), nil
		default:
			return envFor(t, 1, 4, 5), nil
		}
	}, 2, func(batch []any) error {
		for _, it := range batch {
			seen = append(seen, it.(map[string]any)["id"].(string))
		}
		return nil
	})
	if err != nil {
		t.Fatalf("ForEachPage: %v", err)
	}
	if len(seen) != 5 {
		t.Errorf("saw %d items, want 5: %v", len(seen), seen)
	}
	want := []int{0, 2, 4}
	if fmt.Sprint(offsets) != fmt.Sprint(want) {
		t.Errorf("offsets = %v, want %v", offsets, want)
	}
}

// Termination must come from totalElements, not from observing a short page.
// A full final page is the case a short-page heuristic gets wrong.
func TestForEachPageStopsOnExactMultiple(t *testing.T) {
	calls := 0
	err := ForEachPage(func(limit, offset int) (*Envelope, error) {
		calls++
		if calls > 3 {
			t.Fatal("kept fetching past totalElements")
		}
		return envFor(t, 2, offset, 4), nil
	}, 2, func([]any) error { return nil })
	if err != nil {
		t.Fatalf("ForEachPage: %v", err)
	}
	if calls != 2 {
		t.Errorf("fetched %d pages, want 2", calls)
	}
}

// Missing page metadata must fail closed rather than silently returning
// whatever the first page happened to contain.
func TestForEachPageFailsClosedWithoutPageMetadata(t *testing.T) {
	env, _ := ParseEnvelope([]byte(`{"data":[{"id":"p0"}]}`))
	err := ForEachPage(func(limit, offset int) (*Envelope, error) {
		return env, nil
	}, 2, func([]any) error { return nil })
	if err == nil {
		t.Fatal("expected an error when page metadata is absent")
	}
}

func TestForEachPagePropagatesFetchError(t *testing.T) {
	boom := errors.New("boom")
	err := ForEachPage(func(limit, offset int) (*Envelope, error) {
		return nil, boom
	}, 2, func([]any) error { return nil })
	if !errors.Is(err, boom) {
		t.Fatalf("err = %v, want it to wrap boom", err)
	}
}

func TestForEachPagePropagatesCallbackError(t *testing.T) {
	boom := errors.New("callback boom")
	err := ForEachPage(func(limit, offset int) (*Envelope, error) {
		return envFor(t, 2, 0, 10), nil
	}, 2, func([]any) error { return boom })
	if !errors.Is(err, boom) {
		t.Fatalf("err = %v, want it to wrap boom", err)
	}
}

func TestForEachPageEmptyResultIsNotAnError(t *testing.T) {
	body := []byte(`{"data":[],"page":{"pageSize":50,"totalElements":0}}`)
	env, _ := ParseEnvelope(body)
	calls := 0
	err := ForEachPage(func(limit, offset int) (*Envelope, error) {
		calls++
		return env, nil
	}, 50, func([]any) error { return nil })
	if err != nil {
		t.Fatalf("ForEachPage: %v", err)
	}
	if calls != 1 {
		t.Errorf("fetched %d pages for an empty result, want 1", calls)
	}
}

// pageSize is passed through to fetch unchanged, including zero or negative
// values. This allows the fetcher to defer to the server's default via
// EncodeQuery, which omits non-positive limits from the query string.
func TestForEachPagePassesThroughPageSize(t *testing.T) {
	var receivedSize int
	body := []byte(`{"data":[],"page":{"pageSize":0,"totalElements":0}}`)
	env, _ := ParseEnvelope(body)
	err := ForEachPage(func(limit, offset int) (*Envelope, error) {
		receivedSize = limit
		return env, nil
	}, 0, func([]any) error { return nil })
	if err != nil {
		t.Fatalf("ForEachPage: %v", err)
	}
	if receivedSize != 0 {
		t.Errorf("fetcher received pageSize %d, want 0", receivedSize)
	}
}
