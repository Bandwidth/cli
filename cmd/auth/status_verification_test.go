package auth

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/Bandwidth/cli/internal/cmdutil"
	"github.com/Bandwidth/cli/internal/config"
	"github.com/Bandwidth/cli/internal/testutil"
	"github.com/spf13/cobra"
)

func TestStatusVerification(t *testing.T) {
	claims := base64.RawURLEncoding.EncodeToString([]byte(`{"accounts":["fresh-account"],"roles":["Campaign Management","SIP Credentials"],"express":true}`))
	jwt := "header." + claims + ".signature"
	for _, tc := range []struct {
		name       string
		status     int
		body       string
		offline    bool
		missing    string
		cancel     bool
		wantStatus string
		wantReason string
		wantExit   int
	}{
		{"valid", 200, `{"access_token":"` + jwt + `","expires_in":3600}`, false, "", false, "valid", "", 0},
		{"rejected", 401, `{"error":"invalid_client","error_description":"secret-value"}`, false, "", false, "rejected", "invalid_client", 2},
		{"OAuth 400 rejection", 400, `{"error":"invalid_client"}`, false, "", false, "rejected", "invalid_client", 2},
		{"rate limited", 429, `{}`, false, "", false, "unknown", "probe_failed", 7},
		{"server error", 500, `{}`, false, "", false, "unknown", "probe_failed", 1},
		{"forbidden is not credential rejection", 403, `{}`, false, "", false, "unknown", "probe_failed", 1},
		{"malformed token response", 200, `<html>proxy</html>`, false, "", false, "unknown", "probe_failed", 1},
		{"bad claims", 200, `{"access_token":"not-a-jwt"}`, false, "", false, "unknown", "probe_failed", 1},
		{"offline", 0, "", true, "", false, "unknown", "not_verified", 0},
		{"not logged in", 0, "", false, "id", false, "unknown", "not_logged_in", 2},
		{"keychain unavailable", 0, "", false, "secret", false, "unknown", "credentials_unavailable", 2},
		{"offline missing secret", 0, "", true, "secret", false, "unknown", "credentials_unavailable", 0},
		{"cancelled", 0, "", false, "", true, "unknown", "probe_failed", 1},
		{"network failure", -1, "", false, "", false, "unknown", "probe_failed", 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			taskHome := t.TempDir()
			t.Setenv("HOME", taskHome)
			t.Setenv("USERPROFILE", taskHome)
			for _, name := range []string{"BW_CLIENT_ID", "BW_ACCOUNT_ID", "BW_ENVIRONMENT"} {
				t.Setenv(name, "")
			}
			oldOverride := cmdutil.EnvironmentOverride
			cmdutil.EnvironmentOverride = ""
			t.Cleanup(func() { cmdutil.EnvironmentOverride = oldOverride })
			cfg := &config.Config{}
			id := "test-id"
			if tc.missing == "id" {
				id = ""
			}
			cfg.SetProfile("admin", &config.Profile{ClientID: id, Roles: []string{"stale-role"}})
			path, err := config.DefaultPath()
			if err != nil {
				t.Fatal(err)
			}
			if err := config.Save(path, cfg); err != nil {
				t.Fatal(err)
			}
			before, _ := os.ReadFile(path)
			oldPassword := statusPassword
			statusPassword = func(string) (string, error) {
				if tc.missing == "secret" {
					return "", errors.New("keychain unavailable")
				}
				return "secret-value", nil
			}
			t.Cleanup(func() { statusPassword = oldPassword })
			calls := 0
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				if tc.status == 0 {
					t.Error("unexpected network request")
					w.WriteHeader(500)
					return
				}
				if r.URL.Path != "/api/v1/oauth2/token" {
					t.Errorf("path = %s", r.URL.Path)
				}
				w.WriteHeader(tc.status)
				fmt.Fprint(w, tc.body)
			}))
			defer srv.Close()
			if tc.status == -1 {
				srv.Close()
			}
			t.Setenv("BW_API_URL", srv.URL)
			child := &cobra.Command{Use: "status", RunE: runStatus}
			child.Flags().Bool("no-verify", false, "")
			root := testutil.NewTestRoot(child)
			var stdout, stderr bytes.Buffer
			root.SetOut(&stdout)
			root.SetErr(&stderr)
			args := []string{"status", "--plain"}
			if tc.offline {
				args = append(args, "--no-verify")
			}
			root.SetArgs(args)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if tc.cancel {
				cancel()
			}
			err = root.ExecuteContext(ctx)
			if code := cmdutil.ExitCodeForError(err); code != tc.wantExit {
				t.Fatalf("exit %d, want %d: %v", code, tc.wantExit, err)
			}
			var out statusJSON
			if err := json.Unmarshal(stdout.Bytes(), &out); err != nil {
				t.Fatalf("invalid JSON: %v: %s", err, stdout.String())
			}
			if out.Token.Status != tc.wantStatus || out.Token.Reason != tc.wantReason {
				t.Fatalf("token = %+v", out.Token)
			}
			if out.Authenticated != (tc.wantStatus == "valid") {
				t.Fatalf("authenticated = %v", out.Authenticated)
			}
			if out.CredentialsStored != (tc.missing == "") {
				t.Fatalf("credentials_stored = %v", out.CredentialsStored)
			}
			if strings.Contains(stdout.String()+stderr.String(), "secret-value") || strings.Contains(stdout.String(), jwt) {
				t.Fatal("secret leaked")
			}
			if out.Authenticated {
				if out.Token.ExpiresIn == nil || *out.Token.ExpiresIn < 3590 || !out.Capabilities["campaign_management"] || !out.Build || out.Accounts[0] != "fresh-account" {
					t.Fatalf("stale/incomplete verification output: %+v", out)
				}
			}
			wantCalls := 1
			if tc.offline || tc.missing != "" || tc.cancel || tc.status == -1 {
				wantCalls = 0
			}
			if calls != wantCalls {
				t.Errorf("calls = %d, want %d", calls, wantCalls)
			}
			after, _ := os.ReadFile(path)
			if !bytes.Equal(before, after) {
				t.Fatal("status mutated stored config")
			}
		})
	}
}
