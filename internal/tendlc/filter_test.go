package tendlc

import "testing"

func TestEncodeQueryDeepObjectForm(t *testing.T) {
	tests := []struct {
		name    string
		limit   int
		offset  int
		filters []Filter
		want    string
	}{
		{
			name: "no params", want: "",
		},
		{
			name: "limit and offset only", limit: 50, offset: 100,
			want: "?limit=50&offset=100",
		},
		{
			name:    "eq filter uses bracket form",
			limit:   10,
			filters: []Filter{{Field: "status", Op: OpEq, Value: "REGISTERED"}},
			want:    "?limit=10&status%5Beq%5D=REGISTERED",
		},
		{
			name:    "contains filter",
			filters: []Filter{{Field: "brandId", Op: OpContains, Value: "B0I"}},
			want:    "?brandId%5Bcontains%5D=B0I",
		},
		{
			name:  "filters sort by field for determinism",
			limit: 5,
			filters: []Filter{
				{Field: "vettingStatus", Op: OpEq, Value: "APPROVED"},
				{Field: "brandId", Op: OpEq, Value: "B53K4I0"},
			},
			want: "?brandId%5Beq%5D=B53K4I0&limit=5&vettingStatus%5Beq%5D=APPROVED",
		},
		{
			name:    "empty value is omitted, not sent blank",
			filters: []Filter{{Field: "status", Op: OpEq, Value: ""}},
			want:    "",
		},
		{
			name:    "value is URL-escaped",
			filters: []Filter{{Field: "campaignName", Op: OpContains, Value: "Acme Corp&Co"}},
			want:    "?campaignName%5Bcontains%5D=Acme+Corp%26Co",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := EncodeQuery(tt.limit, tt.offset, tt.filters); got != tt.want {
				t.Errorf("EncodeQuery() =\n  %q\nwant\n  %q", got, tt.want)
			}
		})
	}
}
