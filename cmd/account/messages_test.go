package account

import (
	"errors"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/Bandwidth/cli/internal/api"
	"github.com/Bandwidth/cli/internal/testutil"
)

// runForStderr executes child against fake and returns what it wrote to stderr
// along with its error. Stdout is captured too so it doesn't leak into test output.
func runForStderr(t *testing.T, fake *testutil.FakeClient, child *cobra.Command, args ...string) (string, error) {
	t.Helper()
	swapRegistrationClient(t, fake)
	root := testutil.NewTestRoot(child)
	root.SetArgs(append(args, "--plain"))
	var err error
	stderr := testutil.CaptureStderr(t, func() {
		testutil.CaptureStdout(t, func() { err = root.Execute() })
	})
	return stderr, err
}

func okResult() *testutil.FakeClient {
	return &testutil.FakeClient{PostResult: map[string]interface{}{
		"data":   map[string]interface{}{"status": "OK"},
		"errors": []interface{}{},
	}}
}

func assertContains(t *testing.T, got string, wants ...string) {
	t.Helper()
	for _, want := range wants {
		if !strings.Contains(got, want) {
			t.Errorf("stderr missing %q\n got: %q", want, got)
		}
	}
}

func assertNotContains(t *testing.T, got string, unwanted ...string) {
	t.Helper()
	for _, s := range unwanted {
		if strings.Contains(got, s) {
			t.Errorf("stderr unexpectedly contains %q\n got: %q", s, got)
		}
	}
}

// Success only means the request was accepted, so the wording must stay
// conditional and point existing customers somewhere useful.
func TestRegisterOutputIsConditional(t *testing.T) {
	stderr, err := runForStderr(t, okResult(), registerCmd,
		"register",
		"--phone", "+19195551234",
		"--email", "user@example.com",
		"--first-name", "Jane",
		"--last-name", "Doe",
		"--accept-tos",
	)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}

	assertContains(t, stderr,
		"If this email can be used to create an account",
		"user@example.com",
		"band auth login",
		"https://passport.bandwidth.com/forgot-password",
	)
	assertNotContains(t, stderr, "Registration submitted!")
}

func TestSendCodeOutputIsConditional(t *testing.T) {
	stderr, err := runForStderr(t, okResult(), sendCodeCmd,
		"send-code",
		"--phone", "+19195551234",
		"--email", "user@example.com",
		"--delivery-channel", "sms",
	)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}

	assertContains(t, stderr,
		"If a pending registration matches",
		"via sms",
		"band account register",
	)
	assertNotContains(t, stderr, "Verification code sent via")
}

func TestVerifyFailureHint(t *testing.T) {
	const hint = "match the ones you registered with"
	tests := []struct {
		name     string
		err      error
		wantHint bool
	}{
		{name: "rejected code", err: &api.APIError{StatusCode: 400, Body: "Code supplied is not valid."}, wantHint: true},
		{name: "server error", err: &api.APIError{StatusCode: 500}, wantHint: false},
		{name: "network error", err: errors.New("connection refused"), wantHint: false},
		{name: "success", err: nil, wantHint: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fake := okResult()
			fake.PostErr = tt.err

			stderr, err := runForStderr(t, fake, verifyCmd,
				"verify",
				"--phone", "+19195551234",
				"--email", "user@example.com",
				"--code", "123456",
			)

			if (err != nil) != (tt.err != nil) {
				t.Fatalf("execute error = %v, want error: %v", err, tt.err != nil)
			}
			if tt.wantHint {
				assertContains(t, stderr, hint, "band account register")
			} else {
				assertNotContains(t, stderr, hint)
			}
		})
	}
}
