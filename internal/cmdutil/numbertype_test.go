package cmdutil

import "testing"

func TestClassifyNumber(t *testing.T) {
	tests := []struct {
		number string
		want   NumberType
	}{
		// 10DLC (local US numbers)
		{"+19195551234", NumberType10DLC},
		{"+17045551234", NumberType10DLC},
		{"19195551234", NumberType10DLC},

		// Toll-free
		{"+18005551234", NumberTypeTollFree},
		{"+18885551234", NumberTypeTollFree},
		{"+18775551234", NumberTypeTollFree},
		{"+18665551234", NumberTypeTollFree},
		{"+18555551234", NumberTypeTollFree},
		{"+18445551234", NumberTypeTollFree},
		{"+18335551234", NumberTypeTollFree},
		{"18005551234", NumberTypeTollFree},

		// Short codes
		{"12345", NumberTypeShortCode},
		{"123456", NumberTypeShortCode},
		{"+12345", NumberTypeShortCode},

		// Unknown / international
		{"+441234567890", NumberTypeUnknown},
		{"+49301234567", NumberTypeUnknown},
		{"", NumberTypeUnknown},
	}

	for _, tc := range tests {
		t.Run(tc.number, func(t *testing.T) {
			got := ClassifyNumber(tc.number)
			if got != tc.want {
				t.Errorf("ClassifyNumber(%q) = %v, want %v", tc.number, got, tc.want)
			}
		})
	}
}

func TestNumberType_String(t *testing.T) {
	tests := []struct {
		nt   NumberType
		want string
	}{
		{NumberType10DLC, "10DLC"},
		{NumberTypeTollFree, "toll-free"},
		{NumberTypeShortCode, "short code"},
		{NumberTypeUnknown, "unknown"},
	}
	for _, tc := range tests {
		if got := tc.nt.String(); got != tc.want {
			t.Errorf("%d.String() = %q, want %q", tc.nt, got, tc.want)
		}
	}
}
