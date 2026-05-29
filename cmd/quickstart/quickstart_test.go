package quickstart

import (
	"testing"
)

func TestExtractIDFromResponse(t *testing.T) {
	tests := []struct {
		name string
		resp interface{}
		keys []string
		want string
	}{
		{
			name: "top-level string ID",
			resp: map[string]interface{}{"ApplicationId": "abc-123"},
			keys: []string{"ApplicationId"},
			want: "abc-123",
		},
		{
			name: "top-level numeric ID",
			resp: map[string]interface{}{"Id": float64(12345)},
			keys: []string{"Id"},
			want: "12345",
		},
		{
			name: "nested in data wrapper",
			resp: map[string]interface{}{
				"data": map[string]interface{}{
					"voiceConfigurationPackageId": "vcp-456",
				},
			},
			keys: []string{"voiceConfigurationPackageId"},
			want: "vcp-456",
		},
		{
			name: "deeply nested",
			resp: map[string]interface{}{
				"SipPeerResponse": map[string]interface{}{
					"SipPeer": map[string]interface{}{
						"PeerId": "99",
					},
				},
			},
			keys: []string{"PeerId"},
			want: "99",
		},
		{
			name: "first matching key wins",
			resp: map[string]interface{}{"ApplicationId": "first", "applicationId": "second"},
			keys: []string{"ApplicationId", "applicationId"},
			want: "first",
		},
		{
			name: "fallback to second key",
			resp: map[string]interface{}{"id": "fallback"},
			keys: []string{"ApplicationId", "id"},
			want: "fallback",
		},
		{
			name: "no matching key",
			resp: map[string]interface{}{"unrelated": "value"},
			keys: []string{"ApplicationId"},
			want: "",
		},
		{
			name: "nil response",
			resp: nil,
			keys: []string{"Id"},
			want: "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := extractIDFromResponse(tc.resp, tc.keys...)
			if got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestExtractPhoneNumber(t *testing.T) {
	tests := []struct {
		name string
		resp interface{}
		want string
	}{
		{
			name: "nested TelephoneNumber",
			resp: map[string]interface{}{
				"SearchResult": map[string]interface{}{
					"TelephoneNumber": "+19195551234",
				},
			},
			want: "+19195551234",
		},
		{
			name: "top-level TelephoneNumber",
			resp: map[string]interface{}{"TelephoneNumber": "+19195559876"},
			want: "+19195559876",
		},
		{
			name: "array response",
			resp: []interface{}{"+19195551234", "+19195551235"},
			want: "+19195551234",
		},
		{
			name: "no phone number",
			resp: map[string]interface{}{"status": "ok"},
			want: "",
		},
		{
			name: "nil response",
			resp: nil,
			want: "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := extractPhoneNumber(tc.resp)
			if got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestBuildQuickstartOrderBody(t *testing.T) {
	t.Run("mirrors number.BuildOrderBody shape (top-level TelephoneNumberList)", func(t *testing.T) {
		body := buildQuickstartOrderBody("+19195551234", "")
		if _, wrong := body.Data["ExistingTelephoneNumberOrderType"]; wrong {
			t.Fatalf("must not use the ExistingTelephoneNumberOrderType wrapper: %#v", body.Data)
		}
		tnl, ok := body.Data["TelephoneNumberList"].(map[string]interface{})
		if !ok {
			t.Fatalf("missing top-level TelephoneNumberList: %#v", body.Data)
		}
		// number.BuildOrderBody uses a []string even for a single TN — mirror it.
		got, ok := tnl["TelephoneNumber"].([]string)
		if !ok || len(got) != 1 || got[0] != "+19195551234" {
			t.Fatalf("TelephoneNumber = %#v, want []string{\"+19195551234\"}", tnl["TelephoneNumber"])
		}
	})
	t.Run("vcp path omits SiteId", func(t *testing.T) {
		body := buildQuickstartOrderBody("+19195551234", "")
		if _, ok := body.Data["SiteId"]; ok {
			t.Fatalf("vcp order body must not include SiteId: %#v", body.Data)
		}
	})
	t.Run("legacy path scopes to the created sub-account", func(t *testing.T) {
		body := buildQuickstartOrderBody("+19195551234", "site-9")
		if body.Data["SiteId"] != "site-9" {
			t.Fatalf("legacy order body SiteId = %v, want site-9", body.Data["SiteId"])
		}
	})
}

func TestFindInMap(t *testing.T) {
	tests := []struct {
		name string
		m    map[string]interface{}
		key  string
		want string
	}{
		{
			name: "flat string",
			m:    map[string]interface{}{"name": "hello"},
			key:  "name",
			want: "hello",
		},
		{
			name: "flat numeric",
			m:    map[string]interface{}{"count": float64(42)},
			key:  "count",
			want: "42",
		},
		{
			name: "nested",
			m: map[string]interface{}{
				"outer": map[string]interface{}{
					"inner": "found",
				},
			},
			key:  "inner",
			want: "found",
		},
		{
			name: "empty string value",
			m:    map[string]interface{}{"id": ""},
			key:  "id",
			want: "",
		},
		{
			name: "missing key",
			m:    map[string]interface{}{"other": "value"},
			key:  "id",
			want: "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := findInMap(tc.m, tc.key)
			if got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}
