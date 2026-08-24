package customerprofile

import (
	"net/http"
	"strings"
	"testing"
)

// Split into list and get so --plain output shape never depends on whether an
// optional argument was supplied: list is always an array, get always an object.
func TestHistoryListReturnsArray(t *testing.T) {
	out, err := runCmd(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"data":[
			{"data":{"id":"abc","name":"Acme"},"metadata":{"version":1,"operation":"CREATED","userName":"someone","createdDate":"2026-01-01T00:00:00Z"}},
			{"data":{"id":"abc","name":"Acme Renamed"},"metadata":{"version":2,"operation":"UPDATED","userName":"someone","createdDate":"2026-01-02T00:00:00Z"}}
		],"page":{"pageSize":50,"totalElements":2}}`))
	}, "history", "list", "abc", "--plain")
	if err != nil {
		t.Fatalf("history list: %v", err)
	}
	if !strings.HasPrefix(strings.TrimSpace(out), "[") {
		t.Errorf("stdout = %q, want a JSON array", out)
	}
}

// The real API nests the profile snapshot under "data" and audit fields under
// "metadata" — the version lives at metadata.version, not top-level, unlike
// 'customer-profile get'.
func TestHistoryGetReturnsObject(t *testing.T) {
	var gotPath string
	out, err := runCmd(t, func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.EscapedPath()
		_, _ = w.Write([]byte(`{"data":{"data":{"id":"abc","name":"Acme"},"metadata":{"version":2,"operation":"UPDATED","userName":"someone","createdDate":"2026-01-02T00:00:00Z"}}}`))
	}, "history", "get", "abc", "2", "--plain")
	if err != nil {
		t.Fatalf("history get: %v", err)
	}
	if !strings.HasPrefix(strings.TrimSpace(out), "{") {
		t.Errorf("stdout = %q, want a JSON object", out)
	}
	if !strings.Contains(out, `"version":2`) && !strings.Contains(out, `"version": 2`) {
		t.Errorf("stdout = %q, want metadata.version at 2", out)
	}
	if !strings.HasSuffix(gotPath, "/history/2") {
		t.Errorf("path = %q, want it to end in /history/2", gotPath)
	}
}

func TestHistoryGetRequiresBothArgs(t *testing.T) {
	if _, err := runCmd(t, nil, "history", "get", "abc"); err == nil {
		t.Fatal("expected an error when the version argument is missing")
	}
}

func TestHistoryListAllWalksPages(t *testing.T) {
	calls := 0
	out, err := runCmd(t, func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls == 1 {
			_, _ = w.Write([]byte(`{"data":[{"data":{"id":"abc"},"metadata":{"version":1,"operation":"CREATED"}}],"page":{"pageSize":1,"totalElements":2}}`))
			return
		}
		_, _ = w.Write([]byte(`{"data":[{"data":{"id":"abc"},"metadata":{"version":2,"operation":"UPDATED"}}],"page":{"pageSize":1,"totalElements":2}}`))
	}, "history", "list", "abc", "--all", "--limit", "1", "--plain")
	if err != nil {
		t.Fatalf("history list --all: %v", err)
	}
	if calls != 2 {
		t.Errorf("fetched %d pages, want 2", calls)
	}
	if !strings.Contains(out, `"version":1`) && !strings.Contains(out, `"version": 1`) {
		t.Errorf("stdout = %q, want items from the first page", out)
	}
	if !strings.Contains(out, `"version":2`) && !strings.Contains(out, `"version": 2`) {
		t.Errorf("stdout = %q, want items from the second page too", out)
	}
}

// TestHistoryListWarnsWhenTruncated guards against list.go's warnIfTruncated
// pattern silently not being mirrored here: a paginated, non-`--all` history
// list must warn on stderr when more versions exist than the page returned,
// exactly like 'customer-profile list' does.
func TestHistoryListWarnsWhenTruncated(t *testing.T) {
	_, stderr, err := runCmdCapturingStderr(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"data":[{"data":{"id":"abc"},"metadata":{"version":1}}],"page":{"pageSize":1,"totalElements":2}}`))
	}, "history", "list", "abc", "--limit", "1", "--plain")
	if err != nil {
		t.Fatalf("history list: %v", err)
	}
	if !strings.Contains(stderr, "pass --all to fetch every page") {
		t.Errorf("stderr = %q, want a truncation warning", stderr)
	}
}

func TestHistoryListNoWarningWhenNotTruncated(t *testing.T) {
	_, stderr, err := runCmdCapturingStderr(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"data":[{"data":{"id":"abc"},"metadata":{"version":1}},{"data":{"id":"abc"},"metadata":{"version":2}}],"page":{"pageSize":50,"totalElements":2}}`))
	}, "history", "list", "abc", "--plain")
	if err != nil {
		t.Fatalf("history list: %v", err)
	}
	if stderr != "" {
		t.Errorf("stderr = %q, want no truncation warning", stderr)
	}
}
