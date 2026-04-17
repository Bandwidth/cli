package auth

import (
	"encoding/base64"
	"testing"
)

func TestEncodeBasicAuth(t *testing.T) {
	// "user:pass" base64-encodes to "dXNlcjpwYXNz"
	got := EncodeBasicAuth("user", "pass")
	want := "dXNlcjpwYXNz"
	if got != want {
		t.Errorf("EncodeBasicAuth(%q, %q) = %q, want %q", "user", "pass", got, want)
	}
}

func TestEncodeBasicAuthEmptyFields(t *testing.T) {
	got := EncodeBasicAuth("", "")
	if got == "" {
		t.Error("EncodeBasicAuth(\"\", \"\") returned empty string")
	}
}

func TestDecodeBasicAuthRoundTrip(t *testing.T) {
	username := "myuser@bandwidth.com"
	password := "s3cr3tP@ss!"

	encoded := EncodeBasicAuth(username, password)
	gotUser, gotPass, err := DecodeBasicAuth(encoded)
	if err != nil {
		t.Fatalf("DecodeBasicAuth() error: %v", err)
	}
	if gotUser != username {
		t.Errorf("username = %q, want %q", gotUser, username)
	}
	if gotPass != password {
		t.Errorf("password = %q, want %q", gotPass, password)
	}
}

func TestDecodeBasicAuthKnownValue(t *testing.T) {
	// "dXNlcjpwYXNz" decodes to "user:pass"
	user, pass, err := DecodeBasicAuth("dXNlcjpwYXNz")
	if err != nil {
		t.Fatalf("DecodeBasicAuth() error: %v", err)
	}
	if user != "user" {
		t.Errorf("user = %q, want %q", user, "user")
	}
	if pass != "pass" {
		t.Errorf("pass = %q, want %q", pass, "pass")
	}
}

func TestDecodeBasicAuthInvalidBase64(t *testing.T) {
	_, _, err := DecodeBasicAuth("not-valid-base64!!!")
	if err == nil {
		t.Error("expected error for invalid base64, got nil")
	}
}

func TestDecodeBasicAuthMissingColon(t *testing.T) {
	// Encode a string with no colon separator
	noColon := base64.StdEncoding.EncodeToString([]byte("nocolon"))
	_, _, err := DecodeBasicAuth(noColon)
	if err == nil {
		t.Error("expected error when no colon separator, got nil")
	}
}

func TestKeyringStoreAndGet(t *testing.T) {
	// Keyring operations may fail in CI environments without a keychain.
	// Skip gracefully if the keychain isn't available.
	username := "keyring-test-user"
	password := "keyring-test-pass"

	err := StorePassword(username, password)
	if err != nil {
		t.Skipf("Keychain not available (StorePassword error): %v", err)
	}

	got, err := GetPassword(username)
	if err != nil {
		t.Fatalf("GetPassword() error: %v", err)
	}
	if got != password {
		t.Errorf("GetPassword() = %q, want %q", got, password)
	}

	// Cleanup
	if err := DeletePassword(username); err != nil {
		t.Logf("DeletePassword() warning: %v", err)
	}
}

func TestKeyringDeleteNonexistent(t *testing.T) {
	// Deleting something that was never stored — should not panic.
	_ = DeletePassword("band-cli-nonexistent-user-xyz")
}
