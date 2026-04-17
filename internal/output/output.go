package output

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sort"

	"github.com/olekukonko/tablewriter"
)

// Print writes data to w in the specified format ("json" or "table").
func Print(w io.Writer, format string, data interface{}) error {
	switch format {
	case "json":
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		return enc.Encode(data)

	case "table":
		return printTable(w, data)

	default:
		return fmt.Errorf("unknown output format %q: supported formats are json, table", format)
	}
}

// printTable renders data as a table, handling the various types that come
// out of JSON deserialization as well as pre-formatted []map[string]string.
func printTable(w io.Writer, data interface{}) error {
	if data == nil {
		return nil
	}

	switch v := data.(type) {

	// Fast path: caller already prepared []map[string]string rows.
	case []map[string]string:
		if len(v) == 0 {
			return nil
		}
		columns := sortedKeys(v[0])
		PrintTable(w, columns, v)
		return nil

	// Array of objects (the common JSON-deserialized case).
	case []interface{}:
		if len(v) == 0 {
			return nil
		}
		// Check the first element to decide rendering strategy.
		switch first := v[0].(type) {
		case map[string]interface{}:
			columns := sortedKeysIface(first)
			rows := make([]map[string]string, 0, len(v))
			for _, item := range v {
				m, ok := item.(map[string]interface{})
				if !ok {
					continue
				}
				rows = append(rows, stringifyMap(m, columns))
			}
			if len(rows) == 0 {
				return nil
			}
			PrintTable(w, columns, rows)
			return nil
		default:
			// Array of primitives — single-column table.
			columns := []string{"Value"}
			rows := make([]map[string]string, 0, len(v))
			for _, item := range v {
				rows = append(rows, map[string]string{"Value": fmt.Sprintf("%v", item)})
			}
			PrintTable(w, columns, rows)
			return nil
		}

	// Single object — render as a two-column key/value table.
	case map[string]interface{}:
		if len(v) == 0 {
			return nil
		}
		columns := []string{"Field", "Value"}
		keys := sortedKeysIface(v)
		rows := make([]map[string]string, 0, len(keys))
		for _, k := range keys {
			rows = append(rows, map[string]string{
				"Field": k,
				"Value": stringifyValue(v[k]),
			})
		}
		PrintTable(w, columns, rows)
		return nil

	// Bare string or other primitive — just print it.
	case string:
		fmt.Fprintln(w, v)
		return nil
	default:
		fmt.Fprintf(w, "%v\n", v)
		return nil
	}
}

// stringifyMap converts a map[string]interface{} to map[string]string using
// the provided column order. Nested objects/arrays are rendered as truncated JSON.
func stringifyMap(m map[string]interface{}, columns []string) map[string]string {
	out := make(map[string]string, len(columns))
	for _, k := range columns {
		out[k] = stringifyValue(m[k])
	}
	return out
}

// stringifyValue converts an arbitrary value to a table-friendly string.
// Maps and slices are rendered as compact JSON, truncated to 50 characters.
func stringifyValue(val interface{}) string {
	if val == nil {
		return ""
	}
	switch v := val.(type) {
	case map[string]interface{}, []interface{}:
		b, err := json.Marshal(v)
		if err != nil {
			return fmt.Sprintf("%v", v)
		}
		s := string(b)
		if len(s) > 50 {
			return s[:47] + "..."
		}
		return s
	default:
		return fmt.Sprintf("%v", v)
	}
}

// sortedKeys returns the keys of a map[string]string in sorted order.
func sortedKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// sortedKeysIface returns the keys of a map[string]interface{} in sorted order.
func sortedKeysIface(m map[string]interface{}) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// PrintTable writes a formatted table to w with the given columns and rows.
func PrintTable(w io.Writer, columns []string, rows []map[string]string) {
	table := tablewriter.NewWriter(w)
	table.SetHeader(columns)
	table.SetBorder(false)
	table.SetAutoWrapText(false)

	for _, row := range rows {
		record := make([]string, len(columns))
		for i, col := range columns {
			record[i] = row[col]
		}
		table.Append(record)
	}

	table.Render()
}

// Stdout prints data to os.Stdout in the specified format.
func Stdout(format string, data interface{}) error {
	return Print(os.Stdout, format, data)
}

// StdoutPlain prints data to os.Stdout after flattening it to a simplified
// structure. Intended for script and agent use where deep nesting is noise.
func StdoutPlain(data interface{}) error {
	flat := FlattenResponse(data)
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(flat)
}

// StdoutAuto prints data to os.Stdout. If plain is true it flattens first;
// otherwise it uses the specified format string. Table format also flattens
// to unwrap API response wrappers before rendering. This is the preferred
// helper for command RunE functions.
func StdoutAuto(format string, plain bool, data interface{}) error {
	if plain {
		return StdoutPlain(data)
	}
	if format == "table" {
		return Stdout(format, FlattenResponse(data))
	}
	return Stdout(format, data)
}

// FlattenAndNormalize flattens a raw API response and normalizes it to an array.
// Convenience wrapper for callers that need the processed value (e.g. polling loops).
func FlattenAndNormalize(data interface{}) interface{} {
	return NormalizeToArray(FlattenResponse(data))
}

// StdoutPlainList is like StdoutAuto for list commands — it always normalizes
// the result to an array when plain is true, preventing single-item ambiguity.
// Table format also flattens and normalizes to unwrap API wrappers.
func StdoutPlainList(format string, plain bool, data interface{}) error {
	if plain {
		flat := FlattenResponse(data)
		normalized := NormalizeToArray(flat)
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(normalized)
	}
	if format == "table" {
		flat := FlattenResponse(data)
		normalized := NormalizeToArray(flat)
		return Stdout(format, normalized)
	}
	return Stdout(format, data)
}

// Error prints a formatted error message to os.Stderr.
func Error(format string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, "Error: "+format+"\n", args...)
}
