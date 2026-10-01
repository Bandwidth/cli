package account

import (
	"testing"

	"github.com/Bandwidth/cli/internal/api"
	"github.com/Bandwidth/cli/internal/testutil"
)

// swapRegistrationClient substitutes registrationClient with fake, restoring it on cleanup; no t.Parallel() on callers (mutates a global, like cmdutil.VoiceClient).
func swapRegistrationClient(t *testing.T, fake *testutil.FakeClient) {
	t.Helper()
	orig := registrationClient
	t.Cleanup(func() { registrationClient = orig })
	registrationClient = func(string) (api.Requester, string, error) {
		return fake, "", nil
	}
}

func TestRegisterPlainOutput(t *testing.T) {
	fake := &testutil.FakeClient{PostResult: map[string]interface{}{
		"links": []interface{}{},
		"data": map[string]interface{}{
			"message":        "Onboarding request received successfully",
			"status":         "USER_CREATION_PENDING",
			"registrationId": "reg-123",
		},
		"errors": []interface{}{},
	}}
	swapRegistrationClient(t, fake)

	root := testutil.NewTestRoot(registerCmd)
	root.SetArgs([]string{
		"register",
		"--phone", "+19195551234",
		"--email", "user@example.com",
		"--first-name", "Jane",
		"--last-name", "Doe",
		"--accept-tos",
		"--sms-opt-in",
		"--plain",
	})

	out := testutil.CaptureStdout(t, func() {
		if err := root.Execute(); err != nil {
			t.Fatalf("execute: %v", err)
		}
	})

	want := "{\n  \"message\": \"Onboarding request received successfully\",\n  \"registrationId\": \"reg-123\",\n  \"status\": \"USER_CREATION_PENDING\"\n}\n"
	if out != want {
		t.Fatalf("golden mismatch:\n got: %q\nwant: %q", out, want)
	}

	if fake.PostPath != "/registration" {
		t.Errorf("PostPath = %q, want %q", fake.PostPath, "/registration")
	}
	body, ok := fake.PostBody.(map[string]interface{})
	if !ok {
		t.Fatalf("PostBody is %T, want map[string]interface{}", fake.PostBody)
	}
	if body["phoneNumber"] != "+19195551234" || body["email"] != "user@example.com" {
		t.Errorf("unexpected phone/email in request body: %+v", body)
	}
	if body["tosAccepted"] != true {
		t.Errorf("tosAccepted = %v, want true", body["tosAccepted"])
	}
	if body["promotionalCommsAccepted"] != true {
		t.Errorf("promotionalCommsAccepted = %v, want true (--sms-opt-in was passed)", body["promotionalCommsAccepted"])
	}
}

func TestSendCodePlainOutput(t *testing.T) {
	fake := &testutil.FakeClient{PostResult: map[string]interface{}{
		"links": []interface{}{},
		"data": map[string]interface{}{
			"message": "Code successfully sent to +19195551234",
			"status":  "VERIFICATION_CODE_SENT",
		},
		"errors": []interface{}{},
	}}
	swapRegistrationClient(t, fake)

	root := testutil.NewTestRoot(sendCodeCmd)
	root.SetArgs([]string{
		"send-code",
		"--phone", "+19195551234",
		"--email", "user@example.com",
		"--delivery-channel", " Voice ", // normalization: mixed case + whitespace
		"--plain",
	})

	out := testutil.CaptureStdout(t, func() {
		if err := root.Execute(); err != nil {
			t.Fatalf("execute: %v", err)
		}
	})

	want := "{\n  \"message\": \"Code successfully sent to +19195551234\",\n  \"status\": \"VERIFICATION_CODE_SENT\"\n}\n"
	if out != want {
		t.Fatalf("golden mismatch:\n got: %q\nwant: %q", out, want)
	}

	if fake.PostPath != "/registration/code" {
		t.Errorf("PostPath = %q, want %q", fake.PostPath, "/registration/code")
	}
	body, ok := fake.PostBody.(map[string]interface{})
	if !ok {
		t.Fatalf("PostBody is %T, want map[string]interface{}", fake.PostBody)
	}
	if body["deliveryChannel"] != "VOICE" {
		t.Errorf("deliveryChannel = %v, want normalized %q", body["deliveryChannel"], "VOICE")
	}
}

func TestVerifyPlainOutput(t *testing.T) {
	fake := &testutil.FakeClient{PostResult: map[string]interface{}{
		"links": []interface{}{},
		"data": map[string]interface{}{
			"message":        "+19195551234 successfully verified",
			"status":         "PHONE_VERIFIED",
			"registrationId": "reg-123",
		},
		"errors": []interface{}{},
	}}
	swapRegistrationClient(t, fake)

	root := testutil.NewTestRoot(verifyCmd)
	root.SetArgs([]string{
		"verify",
		"--phone", "+19195551234",
		"--email", "user@example.com",
		"--code", "123456",
		"--plain",
	})

	out := testutil.CaptureStdout(t, func() {
		if err := root.Execute(); err != nil {
			t.Fatalf("execute: %v", err)
		}
	})

	want := "{\n  \"message\": \"+19195551234 successfully verified\",\n  \"registrationId\": \"reg-123\",\n  \"status\": \"PHONE_VERIFIED\"\n}\n"
	if out != want {
		t.Fatalf("golden mismatch:\n got: %q\nwant: %q", out, want)
	}

	if fake.PostPath != "/registration/code/verify" {
		t.Errorf("PostPath = %q, want %q", fake.PostPath, "/registration/code/verify")
	}
	body, ok := fake.PostBody.(map[string]interface{})
	if !ok {
		t.Fatalf("PostBody is %T, want map[string]interface{}", fake.PostBody)
	}
	if body["code"] != "123456" {
		t.Errorf("code = %v, want %q", body["code"], "123456")
	}
}
