package quickstart

import (
	"errors"
	"testing"

	"github.com/Bandwidth/cli/internal/api"
)

func TestAssignErrIsRetryable(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{"VCS-0044 (number provisioning)", &api.APIError{StatusCode: 400, Body: `{"errors":[{"code":"VCS-0044"}]}`}, true},
		{"plain 400", &api.APIError{StatusCode: 400, Body: "bad request"}, false},
		{"401 auth", &api.APIError{StatusCode: 401, Body: ""}, false},
		{"403 forbidden", &api.APIError{StatusCode: 403, Body: ""}, false},
		{"404 not found", &api.APIError{StatusCode: 404, Body: ""}, false},
		{"422 unprocessable (not ready)", &api.APIError{StatusCode: 422, Body: ""}, true},
		{"429 rate limited", &api.APIError{StatusCode: 429, Body: ""}, true},
		{"500 server error", &api.APIError{StatusCode: 500, Body: ""}, true},
		{"transport error", errors.New("connection reset"), true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := assignErrIsRetryable(c.err); got != c.want {
				t.Errorf("assignErrIsRetryable(%v) = %v, want %v", c.err, got, c.want)
			}
		})
	}
}

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

// Order-body construction now lives in number.BuildOrderBody (SiteId +
// ExistingTelephoneNumberOrderType wrapper, live-verified) and is covered by
// cmd/number/number_test.go's TestBuildOrderBody.

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
