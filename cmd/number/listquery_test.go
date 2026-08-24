package number

import (
	"testing"
)

func TestListOptionsValidate(t *testing.T) {
	tests := []struct {
		name    string
		opts    listOptions
		wantErr bool
	}{
		{name: "no flags", opts: listOptions{}},
		{name: "status only", opts: listOptions{Status: "Aging"}},
		{name: "geo filters", opts: listOptions{NpaNxx: "919555", State: "NC"}},
		{name: "ratecenter with state", opts: listOptions{RateCenter: "RALEIGH", State: "NC"}},
		{name: "subaccount", opts: listOptions{Subaccount: "407"}},
		{name: "subaccount and location", opts: listOptions{Subaccount: "407", Location: "500017"}},
		{name: "subaccount with geo", opts: listOptions{Subaccount: "407", State: "NC"}},
		{name: "disconnected alone", opts: listOptions{Disconnected: true}},

		{name: "ratecenter without state", opts: listOptions{RateCenter: "RALEIGH"}, wantErr: true},
		{name: "location without subaccount", opts: listOptions{Location: "500017"}, wantErr: true},
		{name: "disconnected with status", opts: listOptions{Disconnected: true, Status: "Aging"}, wantErr: true},
		{name: "disconnected with geo", opts: listOptions{Disconnected: true, State: "NC"}, wantErr: true},
		{name: "disconnected with subaccount", opts: listOptions{Disconnected: true, Subaccount: "407"}, wantErr: true},
		{name: "status with geo", opts: listOptions{Status: "Aging", State: "NC"}, wantErr: true},
		{name: "status with subaccount", opts: listOptions{Status: "Aging", Subaccount: "407"}, wantErr: true},
		{name: "geo with location", opts: listOptions{Subaccount: "407", Location: "500017", State: "NC"}, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.opts.validate()
			if tt.wantErr && err == nil {
				t.Errorf("validate(%+v) = nil, want error", tt.opts)
			}
			if !tt.wantErr && err != nil {
				t.Errorf("validate(%+v) = %v, want nil", tt.opts, err)
			}
		})
	}
}

func TestBuildListQuery(t *testing.T) {
	tests := []struct {
		name      string
		opts      listOptions
		wantNil   bool
		wantPath  string
		wantQuery map[string]string
	}{
		{name: "no flags falls back to /tns", opts: listOptions{}, wantNil: true},
		{name: "status only falls back to /tns", opts: listOptions{Status: "Aging"}, wantNil: true},
		{
			name:      "geo filters hit account inserviceNumbers",
			opts:      listOptions{NpaNxx: "919555", State: "NC", RateCenter: "RALEIGH", Lata: "426"},
			wantPath:  "/accounts/123/inserviceNumbers",
			wantQuery: map[string]string{"npaNxx": "919555", "state": "NC", "ratecenter": "RALEIGH", "lata": "426"},
		},
		{
			name:     "subaccount hits site inserviceNumbers",
			opts:     listOptions{Subaccount: "407"},
			wantPath: "/accounts/123/sites/407/inserviceNumbers",
		},
		{
			name:      "subaccount with geo keeps filters",
			opts:      listOptions{Subaccount: "407", State: "NC"},
			wantPath:  "/accounts/123/sites/407/inserviceNumbers",
			wantQuery: map[string]string{"state": "NC"},
		},
		{
			name:      "site-level ratecenter uses documented rateCenter casing",
			opts:      listOptions{Subaccount: "407", State: "NC", RateCenter: "RALEIGH"},
			wantPath:  "/accounts/123/sites/407/inserviceNumbers",
			wantQuery: map[string]string{"state": "NC", "rateCenter": "RALEIGH"},
		},
		{
			name:     "subaccount and location hit sippeer tns",
			opts:     listOptions{Subaccount: "407", Location: "500017"},
			wantPath: "/accounts/123/sites/407/sippeers/500017/tns",
		},
		{
			name:     "disconnected hits discnumbers",
			opts:     listOptions{Disconnected: true},
			wantPath: "/accounts/123/discnumbers",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := buildListQuery("123", tt.opts)
			if tt.wantNil {
				if got != nil {
					t.Fatalf("buildListQuery = %+v, want nil", got)
				}
				return
			}
			if got == nil {
				t.Fatal("buildListQuery = nil, want query")
			}
			if got.Path != tt.wantPath {
				t.Errorf("Path = %q, want %q", got.Path, tt.wantPath)
			}
			for k, v := range tt.wantQuery {
				if got.Query.Get(k) != v {
					t.Errorf("Query[%s] = %q, want %q", k, got.Query.Get(k), v)
				}
			}
		})
	}
}

func TestCollectBareNumbers(t *testing.T) {
	// XML-decoded shape of the inserviceNumbers/discnumbers response:
	// TNs > TelephoneNumbers > TelephoneNumber as bare string(s).
	single := map[string]interface{}{
		"TNs": map[string]interface{}{
			"TotalCount": "1",
			"TelephoneNumbers": map[string]interface{}{
				"Count":           "1",
				"TelephoneNumber": "+14158714245",
			},
		},
	}
	if got := extractFullNumbers(single); len(got) != 1 || got[0] != "+14158714245" {
		t.Errorf("single bare number: got %v", got)
	}

	multi := map[string]interface{}{
		"TelephoneNumbers": map[string]interface{}{
			"TelephoneNumber": []interface{}{"+14158714245", "4352154439"},
		},
	}
	got := extractFullNumbers(multi)
	if len(got) != 2 || got[0] != "+14158714245" || got[1] != "+14352154439" {
		t.Errorf("bare number list: got %v", got)
	}

	// The /tns object shape must keep working.
	objects := map[string]interface{}{
		"TelephoneNumbers": map[string]interface{}{
			"TelephoneNumber": []interface{}{
				map[string]interface{}{"FullNumber": "9195551234"},
				map[string]interface{}{"FullNumber": "9195551235"},
			},
		},
	}
	got = extractFullNumbers(objects)
	if len(got) != 2 || got[0] != "+19195551234" {
		t.Errorf("object list: got %v", got)
	}
}

func TestCountPath(t *testing.T) {
	tests := []struct {
		name         string
		subaccount   string
		location     string
		disconnected bool
		want         string
		wantErr      bool
	}{
		{name: "default", want: "/accounts/123/inserviceNumbers/totals"},
		{name: "disconnected", disconnected: true, want: "/accounts/123/discnumbers/totals"},
		{name: "subaccount", subaccount: "407", want: "/accounts/123/sites/407/totaltns"},
		{name: "subaccount and location", subaccount: "407", location: "500017", want: "/accounts/123/sites/407/sippeers/500017/totaltns"},
		{name: "location without subaccount", location: "500017", wantErr: true},
		{name: "disconnected with subaccount", subaccount: "407", disconnected: true, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := countPath("123", tt.subaccount, tt.location, tt.disconnected)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("countPath = %q, want error", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("countPath error: %v", err)
			}
			if got != tt.want {
				t.Errorf("countPath = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestUnwrapTelephoneNumberDetails(t *testing.T) {
	details := map[string]interface{}{"FullNumber": "9195551234", "Lata": "426"}
	wrapped := map[string]interface{}{
		"TelephoneNumberResponse": map[string]interface{}{
			"TelephoneNumberDetails": details,
		},
	}
	got, ok := unwrapTelephoneNumberDetails(wrapped).(map[string]interface{})
	if !ok || got["Lata"] != "426" {
		t.Errorf("unwrap = %#v, want details map", unwrapTelephoneNumberDetails(wrapped))
	}

	// Already-unwrapped and unexpected shapes pass through.
	if got := unwrapTelephoneNumberDetails(details).(map[string]interface{}); got["Lata"] != "426" {
		t.Errorf("pass-through failed: %#v", got)
	}
}
