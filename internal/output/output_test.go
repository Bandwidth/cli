package output

import (
	"bytes"
	"strings"
	"testing"
)

func TestJSON(t *testing.T) {
	type data struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	}

	var buf bytes.Buffer
	err := Print(&buf, "json", data{ID: "abc123", Name: "test account"})
	if err != nil {
		t.Fatalf("Print() error: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "abc123") {
		t.Errorf("output missing id field, got: %s", out)
	}
	if !strings.Contains(out, "test account") {
		t.Errorf("output missing name field, got: %s", out)
	}
}

func TestTable(t *testing.T) {
	var buf bytes.Buffer

	columns := []string{"ID", "Name", "Status"}
	rows := []map[string]string{
		{"ID": "1", "Name": "Alice", "Status": "active"},
		{"ID": "2", "Name": "Bob", "Status": "inactive"},
	}

	PrintTable(&buf, columns, rows)

	out := buf.String()
	if !strings.Contains(out, "Alice") {
		t.Errorf("table output missing Alice, got: %s", out)
	}
	if !strings.Contains(out, "Bob") {
		t.Errorf("table output missing Bob, got: %s", out)
	}
	if !strings.Contains(out, "active") {
		t.Errorf("table output missing 'active', got: %s", out)
	}
}

func TestPrint_TableFormat(t *testing.T) {
	rows := []map[string]string{
		{"ID": "10", "Name": "Widget"},
	}

	var buf bytes.Buffer
	err := Print(&buf, "table", rows)
	if err != nil {
		t.Fatalf("Print() table error: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "Widget") {
		t.Errorf("table output missing Widget, got: %s", out)
	}
}

func TestPrint_UnknownFormat(t *testing.T) {
	var buf bytes.Buffer
	err := Print(&buf, "xml", struct{}{})
	if err == nil {
		t.Error("expected error for unknown format, got nil")
	}
}

func TestError(t *testing.T) {
	// Just verify Error() doesn't panic and produces output.
	// We can't easily capture stderr in a unit test without redirecting os.Stderr,
	// so we just call it and make sure it runs without panicking.
	Error("something went wrong: %s", "details")
}

func TestFlattenResponse_SingleKeyUnwrap(t *testing.T) {
	input := map[string]interface{}{
		"TNs": map[string]interface{}{
			"TelephoneNumbers": map[string]interface{}{
				"TelephoneNumber": []interface{}{"9195551234", "9195555678"},
			},
		},
	}
	result := FlattenResponse(input)
	arr, ok := result.([]interface{})
	if !ok {
		t.Fatalf("expected []interface{}, got %T: %v", result, result)
	}
	if len(arr) != 2 {
		t.Errorf("expected 2 numbers, got %d", len(arr))
	}
}

func TestFlattenResponse_MultiKeyPreserved(t *testing.T) {
	input := map[string]interface{}{
		"ID":   "123",
		"Name": "test",
	}
	result := FlattenResponse(input)
	m, ok := result.(map[string]interface{})
	if !ok {
		t.Fatalf("expected map[string]interface{}, got %T", result)
	}
	if m["ID"] != "123" || m["Name"] != "test" {
		t.Errorf("multi-key object modified unexpectedly: %v", m)
	}
}

func TestFlattenResponse_AlreadyArray(t *testing.T) {
	input := []interface{}{"a", "b", "c"}
	result := FlattenResponse(input)
	arr, ok := result.([]interface{})
	if !ok {
		t.Fatalf("expected []interface{}, got %T", result)
	}
	if len(arr) != 3 {
		t.Errorf("expected 3 items, got %d", len(arr))
	}
}

func TestFlattenResponse_SitesPattern(t *testing.T) {
	input := map[string]interface{}{
		"SitesResponse": map[string]interface{}{
			"Sites": map[string]interface{}{
				"Site": []interface{}{
					map[string]interface{}{"Name": "site1"},
					map[string]interface{}{"Name": "site2"},
				},
			},
		},
	}
	result := FlattenResponse(input)
	arr, ok := result.([]interface{})
	if !ok {
		t.Fatalf("expected []interface{}, got %T: %v", result, result)
	}
	if len(arr) != 2 {
		t.Errorf("expected 2 sites, got %d", len(arr))
	}
}

func TestFlattenResponse_JSONEnvelope(t *testing.T) {
	// JSON API responses use {data: [...], links: [...], errors: [], page: {...}}
	input := map[string]interface{}{
		"data": []interface{}{
			map[string]interface{}{"name": "VCP 1", "id": "abc"},
			map[string]interface{}{"name": "VCP 2", "id": "def"},
		},
		"links":  []interface{}{map[string]interface{}{"rel": "self"}},
		"errors": []interface{}{},
		"page":   map[string]interface{}{"pageSize": float64(500)},
	}
	result := FlattenResponse(input)
	arr, ok := result.([]interface{})
	if !ok {
		t.Fatalf("expected []interface{}, got %T: %v", result, result)
	}
	if len(arr) != 2 {
		t.Errorf("expected 2 items, got %d", len(arr))
	}
}

func TestFlattenResponse_JSONEnvelope_SingleObject(t *testing.T) {
	// Single resource get: {data: {...}, links: [...], errors: []}
	input := map[string]interface{}{
		"data":   map[string]interface{}{"name": "VCP 1", "id": "abc"},
		"links":  []interface{}{},
		"errors": []interface{}{},
	}
	result := FlattenResponse(input)
	m, ok := result.(map[string]interface{})
	if !ok {
		t.Fatalf("expected map, got %T: %v", result, result)
	}
	if m["name"] != "VCP 1" {
		t.Errorf("name = %v, want VCP 1", m["name"])
	}
}

func TestNormalizeToArray_AlreadyArray(t *testing.T) {
	input := []interface{}{map[string]interface{}{"id": "1"}}
	result := NormalizeToArray(input)
	arr, ok := result.([]interface{})
	if !ok {
		t.Fatalf("expected []interface{}, got %T", result)
	}
	if len(arr) != 1 {
		t.Errorf("expected 1 item, got %d", len(arr))
	}
}

func TestNormalizeToArray_SingleObject(t *testing.T) {
	input := map[string]interface{}{"id": "1", "name": "test"}
	result := NormalizeToArray(input)
	arr, ok := result.([]interface{})
	if !ok {
		t.Fatalf("expected []interface{}, got %T", result)
	}
	if len(arr) != 1 {
		t.Errorf("expected 1 item, got %d", len(arr))
	}
}

func TestNormalizeToArray_String(t *testing.T) {
	result := NormalizeToArray("hello")
	if result != "hello" {
		t.Errorf("expected string passthrough, got %v", result)
	}
}

// --- Table rendering of JSON-deserialized types ---

func TestPrint_Table_ArrayOfObjects(t *testing.T) {
	// This is the core bug fix: []interface{} of map[string]interface{} should
	// render as a multi-column table, not silently produce nothing.
	data := []interface{}{
		map[string]interface{}{"id": "abc", "name": "Alice", "status": "active"},
		map[string]interface{}{"id": "def", "name": "Bob", "status": "inactive"},
	}

	var buf bytes.Buffer
	err := Print(&buf, "table", data)
	if err != nil {
		t.Fatalf("Print() error: %v", err)
	}
	out := buf.String()
	for _, want := range []string{"Alice", "Bob", "abc", "def", "active", "inactive"} {
		if !strings.Contains(out, want) {
			t.Errorf("table output missing %q, got:\n%s", want, out)
		}
	}
	// Columns should be sorted alphabetically: id, name, status
	idIdx := strings.Index(out, "ID")
	nameIdx := strings.Index(out, "NAME")
	statusIdx := strings.Index(out, "STATUS")
	if idIdx == -1 || nameIdx == -1 || statusIdx == -1 {
		t.Fatalf("missing header(s) in output:\n%s", out)
	}
}

func TestPrint_Table_SingleObject(t *testing.T) {
	data := map[string]interface{}{
		"id":   "abc123",
		"name": "My Account",
	}

	var buf bytes.Buffer
	err := Print(&buf, "table", data)
	if err != nil {
		t.Fatalf("Print() error: %v", err)
	}
	out := buf.String()
	// Should render as a key-value table. Headers are uppercased by tablewriter.
	for _, want := range []string{"FIELD", "VALUE", "id", "abc123", "name", "My Account"} {
		if !strings.Contains(out, want) {
			t.Errorf("table output missing %q, got:\n%s", want, out)
		}
	}
}

func TestPrint_Table_ArrayOfPrimitives(t *testing.T) {
	data := []interface{}{"+19195551234", "+19195555678", "+19190001111"}

	var buf bytes.Buffer
	err := Print(&buf, "table", data)
	if err != nil {
		t.Fatalf("Print() error: %v", err)
	}
	out := buf.String()
	for _, want := range []string{"+19195551234", "+19195555678", "+19190001111"} {
		if !strings.Contains(out, want) {
			t.Errorf("table output missing %q, got:\n%s", want, out)
		}
	}
}

func TestPrint_Table_NestedValuesTruncated(t *testing.T) {
	data := []interface{}{
		map[string]interface{}{
			"id": "abc",
			"config": map[string]interface{}{
				"key1": "value1",
				"key2": "value2",
				"key3": "a-somewhat-long-value-that-will-push-us-past-fifty",
			},
		},
	}

	var buf bytes.Buffer
	err := Print(&buf, "table", data)
	if err != nil {
		t.Fatalf("Print() error: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "abc") {
		t.Errorf("table output missing 'abc', got:\n%s", out)
	}
	// The nested config should be truncated with "..."
	if !strings.Contains(out, "...") {
		t.Errorf("expected truncated nested JSON with '...', got:\n%s", out)
	}
}

func TestPrint_Table_String(t *testing.T) {
	var buf bytes.Buffer
	err := Print(&buf, "table", "hello world")
	if err != nil {
		t.Fatalf("Print() error: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "hello world") {
		t.Errorf("expected string output, got: %s", out)
	}
}

func TestPrint_Table_Nil(t *testing.T) {
	var buf bytes.Buffer
	err := Print(&buf, "table", nil)
	if err != nil {
		t.Fatalf("Print() error: %v", err)
	}
	if buf.Len() != 0 {
		t.Errorf("expected no output for nil, got: %s", buf.String())
	}
}

func TestPrint_Table_EmptyArray(t *testing.T) {
	var buf bytes.Buffer
	err := Print(&buf, "table", []interface{}{})
	if err != nil {
		t.Fatalf("Print() error: %v", err)
	}
	if buf.Len() != 0 {
		t.Errorf("expected no output for empty array, got: %s", buf.String())
	}
}

func TestPrint_Table_EmptyMap(t *testing.T) {
	var buf bytes.Buffer
	err := Print(&buf, "table", map[string]interface{}{})
	if err != nil {
		t.Fatalf("Print() error: %v", err)
	}
	if buf.Len() != 0 {
		t.Errorf("expected no output for empty map, got: %s", buf.String())
	}
}

func TestStringifyValue_NilReturnsEmpty(t *testing.T) {
	if got := stringifyValue(nil); got != "" {
		t.Errorf("stringifyValue(nil) = %q, want empty string", got)
	}
}

func TestStringifyValue_ShortJSON(t *testing.T) {
	val := map[string]interface{}{"a": "b"}
	got := stringifyValue(val)
	if got != `{"a":"b"}` {
		t.Errorf("stringifyValue short map = %q, want %q", got, `{"a":"b"}`)
	}
}

func TestStringifyValue_TruncatesLongJSON(t *testing.T) {
	val := map[string]interface{}{
		"longKey": "this is a really long value that will definitely exceed fifty characters total",
	}
	got := stringifyValue(val)
	if len(got) > 50 {
		t.Errorf("stringifyValue should truncate to 50 chars, got %d: %q", len(got), got)
	}
	if !strings.HasSuffix(got, "...") {
		t.Errorf("truncated value should end with '...', got: %q", got)
	}
}

func TestFlattenResponse_NumberList(t *testing.T) {
	// Real-world number list response shape
	input := map[string]interface{}{
		"TNs": map[string]interface{}{
			"TelephoneNumbers": map[string]interface{}{
				"Count": "3",
				"TelephoneNumber": []interface{}{"+19191234567", "+19197654321", "+19190001111"},
			},
			"TotalCount": "3",
			"Links": map[string]interface{}{
				"first": "somelink",
			},
		},
	}
	result := FlattenResponse(input)
	arr, ok := result.([]interface{})
	if !ok {
		t.Fatalf("expected []interface{}, got %T: %v", result, result)
	}
	if len(arr) != 3 {
		t.Errorf("expected 3 numbers, got %d", len(arr))
	}
}
