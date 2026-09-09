package cmd

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Bandwidth/cli/internal/api"
	"github.com/Bandwidth/cli/internal/auth"
	"github.com/Bandwidth/cli/internal/cmdutil"
	"github.com/spf13/cobra"
)

func TestExecuteOAuthFailure(t *testing.T) {
	for _, tc := range []struct {
		status   int
		body     string
		wantExit int
	}{
		{401, `{"error":"invalid_client","error_description":"never-print-secret"}`, 2},
		{400, `{"error":"invalid_client"}`, 2},
		{400, `{"error":"invalid_request"}`, 1},
		{429, `{}`, 7},
		{500, `{"error":"invalid_client"}`, 1},
	} {
		t.Run(fmt.Sprint(tc.status, tc.wantExit), func(t *testing.T) {
			apiCalls := 0
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/api/v1/oauth2/token" {
					apiCalls++
					t.Error("API called despite token failure")
				}
				w.WriteHeader(tc.status)
				fmt.Fprint(w, tc.body)
			}))
			defer srv.Close()
			tm := auth.NewTokenManager("id", "never-print-secret", srv.URL)
			tm.ProfileName = "admin"
			client := api.NewClient(srv.URL, tm)
			root := &cobra.Command{Use: "band"}
			root.AddCommand(&cobra.Command{Use: "probe", RunE: func(c *cobra.Command, args []string) error {
				var result any
				return client.Get(c.Context(), "/resource", &result)
			}})
			var stdout, stderr bytes.Buffer
			root.SetOut(&stdout)
			root.SetErr(&stderr)
			root.SetArgs([]string{"probe"})
			err := executeCommand(context.Background(), root)
			if got := cmdutil.ExitCodeForError(err); got != tc.wantExit {
				t.Fatalf("exit = %d: %v", got, err)
			}
			if apiCalls != 0 || stdout.Len() != 0 {
				t.Fatalf("API calls or unexpected stdout: %s", stdout.String())
			}
			if strings.Contains(stderr.String(), "Usage:") || strings.Contains(stderr.String(), "never-print-secret") || strings.Contains(stderr.String(), "obtaining auth token") {
				t.Fatalf("unexpected stderr: %s", stderr.String())
			}
			if !strings.Contains(stderr.String(), "Error:") {
				t.Fatal("missing error message")
			}
			if tc.wantExit == 2 && !strings.Contains(stderr.String(), "band auth login --profile admin") {
				t.Fatalf("missing remediation: %s", stderr.String())
			}
		})
	}
}

func TestExecuteUsageAndRepeatedInvocations(t *testing.T) {
	root := &cobra.Command{Use: "band"}
	child := &cobra.Command{Use: "probe", Args: cobra.NoArgs, RunE: func(c *cobra.Command, args []string) error {
		if c.Flags().Changed("invalid") {
			return cmdutil.NewFlagError("invalid field")
		}
		return fmt.Errorf("runtime failure")
	}}
	child.Flags().String("required", "", "required value")
	child.Flags().Bool("invalid", false, "")
	_ = child.MarkFlagRequired("required")
	root.AddCommand(child)
	var stdout, stderr bytes.Buffer
	root.SetOut(&stdout)
	root.SetErr(&stderr)
	for _, tc := range []struct {
		args      []string
		wantUsage bool
	}{
		{[]string{"probe"}, true},
		{[]string{"probe", "--required", "x"}, false},
		{[]string{"probe", "--unknown"}, true},
		{[]string{"probe", "extra"}, true},
		{[]string{"probe", "--invalid"}, true},
	} {
		stdout.Reset()
		stderr.Reset()
		root.SetArgs(tc.args)
		if err := executeCommand(context.Background(), root); err == nil {
			t.Fatal("expected failure")
		}
		if got := strings.Contains(stderr.String(), "Usage:"); got != tc.wantUsage {
			t.Fatalf("args %v: stderr %s", tc.args, stderr.String())
		}
		if root.SilenceUsage || root.SilenceErrors {
			t.Fatal("rendering state leaked")
		}
	}
}
