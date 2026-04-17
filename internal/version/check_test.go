package version

import "testing"

func TestIsNewer(t *testing.T) {
	tests := []struct {
		current string
		latest  string
		want    bool
	}{
		{"0.0.3", "0.0.4", true},
		{"0.0.3", "0.0.3", false},
		{"0.0.4", "0.0.3", false},
		{"0.1.0", "0.2.0", true},
		{"1.0.0", "2.0.0", true},
		{"0.0.3-beta", "0.0.3", true},
		{"0.0.3-beta", "0.0.3-beta", false},
		{"0.0.3", "0.0.3-beta", false},
		{"0.0.3-alpha", "0.0.3-beta", true},
		{"v0.0.3", "v0.0.4", true},
		{"v0.0.3", "0.0.4", true},
	}

	for _, tt := range tests {
		t.Run(tt.current+"_vs_"+tt.latest, func(t *testing.T) {
			got := isNewer(tt.current, tt.latest)
			if got != tt.want {
				t.Errorf("isNewer(%q, %q) = %v, want %v", tt.current, tt.latest, got, tt.want)
			}
		})
	}
}

func TestCompareSemver(t *testing.T) {
	tests := []struct {
		a, b string
		want int
	}{
		{"0.0.1", "0.0.2", -1},
		{"0.0.2", "0.0.1", 1},
		{"1.0.0", "1.0.0", 0},
		{"0.0.3-beta", "0.0.3", -1},
		{"0.0.3", "0.0.3-beta", 1},
		{"1.2.3", "1.2.4", -1},
		{"2.0.0", "1.9.9", 1},
	}

	for _, tt := range tests {
		t.Run(tt.a+"_vs_"+tt.b, func(t *testing.T) {
			got := compareSemver(tt.a, tt.b)
			if got != tt.want {
				t.Errorf("compareSemver(%q, %q) = %d, want %d", tt.a, tt.b, got, tt.want)
			}
		})
	}
}
