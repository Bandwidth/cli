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
		_, _ = w.Write([]byte(`{"data":[{"version":1},{"version":2}],"page":{"pageSize":50,"totalElements":2}}`))
	}, "history", "list", "abc", "--plain")
	if err != nil {
		t.Fatalf("history list: %v", err)
	}
	if !strings.HasPrefix(strings.TrimSpace(out), "[") {
		t.Errorf("stdout = %q, want a JSON array", out)
	}
}

func TestHistoryGetReturnsObject(t *testing.T) {
	var gotPath string
	out, err := runCmd(t, func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.EscapedPath()
		_, _ = w.Write([]byte(`{"data":{"version":2,"name":"Acme"}}`))
	}, "history", "get", "abc", "2", "--plain")
	if err != nil {
		t.Fatalf("history get: %v", err)
	}
	if !strings.HasPrefix(strings.TrimSpace(out), "{") {
		t.Errorf("stdout = %q, want a JSON object", out)
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
			_, _ = w.Write([]byte(`{"data":[{"version":1}],"page":{"pageSize":1,"totalElements":2}}`))
			return
		}
		_, _ = w.Write([]byte(`{"data":[{"version":2}],"page":{"pageSize":1,"totalElements":2}}`))
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
}
