package message

import (
	"reflect"
	"testing"
)

func TestExtractMessages(t *testing.T) {
	msgs := []interface{}{
		map[string]interface{}{"messageId": "msg-1", "from": "+15551234567"},
		map[string]interface{}{"messageId": "msg-2", "from": "+15559876543"},
	}

	t.Run("direct map with messages key", func(t *testing.T) {
		input := map[string]interface{}{
			"messages":   msgs,
			"pageInfo":   map[string]interface{}{},
			"totalCount": float64(2),
		}
		got := extractMessages(input)
		if !reflect.DeepEqual(got, msgs) {
			t.Errorf("got %v, want %v", got, msgs)
		}
	})

	t.Run("array wrapping map with messages key", func(t *testing.T) {
		input := []interface{}{
			map[string]interface{}{
				"messages":   msgs,
				"pageInfo":   map[string]interface{}{},
				"totalCount": float64(2),
			},
		}
		got := extractMessages(input)
		if !reflect.DeepEqual(got, msgs) {
			t.Errorf("got %v, want %v", got, msgs)
		}
	})

	t.Run("map without messages key returns as-is", func(t *testing.T) {
		input := map[string]interface{}{"other": "data"}
		got := extractMessages(input)
		if !reflect.DeepEqual(got, input) {
			t.Errorf("got %v, want %v", got, input)
		}
	})

	t.Run("empty array returns as-is", func(t *testing.T) {
		input := []interface{}{}
		got := extractMessages(input)
		if !reflect.DeepEqual(got, input) {
			t.Errorf("got %v, want %v", got, input)
		}
	})

	t.Run("nil returns nil", func(t *testing.T) {
		got := extractMessages(nil)
		if got != nil {
			t.Errorf("got %v, want nil", got)
		}
	})

	t.Run("string returns as-is", func(t *testing.T) {
		got := extractMessages("unexpected")
		if got != "unexpected" {
			t.Errorf("got %v, want 'unexpected'", got)
		}
	})

	t.Run("array wrapping non-map returns as-is", func(t *testing.T) {
		input := []interface{}{"not a map"}
		got := extractMessages(input)
		if !reflect.DeepEqual(got, input) {
			t.Errorf("got %v, want %v", got, input)
		}
	})

	t.Run("messages value is nil", func(t *testing.T) {
		input := map[string]interface{}{
			"messages": nil,
		}
		got := extractMessages(input)
		if got != nil {
			t.Errorf("got %v, want nil", got)
		}
	})
}
