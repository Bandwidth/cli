package output

import (
	"bytes"
	"os"
	"strings"
	"testing"
)

func TestRedactSecrets_RemovesHashesCaseInsensitively(t *testing.T) {
	in := map[string]interface{}{
		"id":       "870874",
		"Hash1":    "1be6abcaa8e9956021d30f33a3925b99",
		"hash1b":   "e028e6577a0bb1b90a33d30a110dbdfe",
		"ns:Hash1": "deadbeef",
	}
	out, ok := RedactSecrets(in).(map[string]interface{})
	if !ok {
		t.Fatalf("RedactSecrets returned %T, want map", RedactSecrets(in))
	}
	for _, k := range []string{"Hash1", "hash1b", "ns:Hash1"} {
		if _, present := out[k]; present {
			t.Errorf("key %q survived redaction", k)
		}
	}
	if out["id"] != "870874" {
		t.Errorf("id = %v, want 870874", out["id"])
	}
}

func TestRedactSecrets_PreservesGeneratedPassword(t *testing.T) {
	// The generated password is the deliverable — generic secret-key redaction
	// must not eat it.
	in := map[string]interface{}{"username": "u", "password": "Tb7xQ2mK9rL4vN8s"}
	out := RedactSecrets(in).(map[string]interface{})
	if out["password"] != "Tb7xQ2mK9rL4vN8s" {
		t.Errorf("password = %v, want it preserved", out["password"])
	}
}

func TestRedactSecrets_Nested(t *testing.T) {
	in := map[string]interface{}{
		"creds": []interface{}{
			map[string]interface{}{"UserName": "u", "Hash1": "abc"},
		},
	}
	out := RedactSecrets(in).(map[string]interface{})
	first := out["creds"].([]interface{})[0].(map[string]interface{})
	if _, present := first["Hash1"]; present {
		t.Error("nested Hash1 survived redaction")
	}
	if first["UserName"] != "u" {
		t.Errorf("UserName = %v, want u", first["UserName"])
	}
}

func TestRedactAndPrint_ScrubsBeforeWriting(t *testing.T) {
	// The real guarantee: output written by a command carries no hashes, even
	// when the command hands over a raw map.
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w
	err := RedactAndPrint("json", true, map[string]interface{}{
		"id": "870874", "Hash1": "1be6abcaa8e9956021d30f33a3925b99",
	})
	w.Close()
	os.Stdout = old
	if err != nil {
		t.Fatalf("RedactAndPrint() error = %v", err)
	}
	var buf bytes.Buffer
	buf.ReadFrom(r)
	got := buf.String()
	if strings.Contains(got, "1be6abcaa8e9956021d30f33a3925b99") || strings.Contains(got, "Hash1") {
		t.Errorf("hash reached stdout: %s", got)
	}
	if !strings.Contains(got, "870874") {
		t.Errorf("non-secret field was lost: %s", got)
	}
}

func TestScrubHashes_XMLErrorBody(t *testing.T) {
	body := `<Errors><Error><ErrorCode>23026</ErrorCode>` +
		`<SipCredential><UserName>clitest</UserName>` +
		`<Hash1>1be6abcaa8e9956021d30f33a3925b99</Hash1>` +
		`<Hash1b>e028e6577a0bb1b90a33d30a110dbdfe</Hash1b>` +
		`</SipCredential></Error></Errors>`

	got := ScrubHashes(body)
	if strings.Contains(got, "1be6abcaa8e9956021d30f33a3925b99") ||
		strings.Contains(got, "e028e6577a0bb1b90a33d30a110dbdfe") {
		t.Errorf("hash value survived scrubbing: %s", got)
	}
	if !strings.Contains(got, "23026") {
		t.Errorf("error code was lost: %s", got)
	}
	if !strings.Contains(got, "clitest") {
		t.Errorf("username was lost: %s", got)
	}
}

func TestScrubHashes_AttributedHashElement(t *testing.T) {
	// Hash elements may carry attributes (xmlns, namespace declarations, etc.).
	// The regex must handle these to ensure secrets don't leak.
	body := `<Errors><Error><ErrorCode>23026</ErrorCode>` +
		`<SipCredential>` +
		`<Hash1 xmlns:i="http://example.com">1be6abcaa8e9956021d30f33a3925b99</Hash1>` +
		`<Hash1b xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance">e028e6577a0bb1b90a33d30a110dbdfe</Hash1b>` +
		`<UserName>testuser</UserName>` +
		`</SipCredential></Error></Errors>`

	got := ScrubHashes(body)
	if strings.Contains(got, "1be6abcaa8e9956021d30f33a3925b99") ||
		strings.Contains(got, "e028e6577a0bb1b90a33d30a110dbdfe") {
		t.Errorf("hash value survived scrubbing (attributed element): %s", got)
	}
	if !strings.Contains(got, "23026") {
		t.Errorf("error code was lost: %s", got)
	}
	if !strings.Contains(got, "testuser") {
		t.Errorf("username was lost: %s", got)
	}
}

// TestScrubHashes_BareHashInProse covers what the element-anchored regex
// structurally cannot: a digest echoed as prose rather than as an XML element.
// The live 23026 response echoes the submitted hashes inside the error
// Description, which is printed to stderr and captured in agent transcripts.
func TestScrubHashes_BareHashInProse(t *testing.T) {
	got := ScrubHashes("Invalid Hash1 value d41d8cd98f00b204e9800998ecf8427e for user clitest")
	if strings.Contains(got, "d41d8cd98f00b204e9800998ecf8427e") {
		t.Errorf("bare hash survived scrubbing: %s", got)
	}
	if !strings.Contains(got, "[REDACTED]") {
		t.Errorf("missing [REDACTED] marker: %s", got)
	}
	if !strings.Contains(got, "clitest") {
		t.Errorf("surrounding diagnostic content was lost: %s", got)
	}
}

// TestScrubHashes_TruncatedHashElement is the truncated-body case: the closing
// tag never arrived, so hashElementRe cannot match, and the value used to pass
// through completely unredacted.
func TestScrubHashes_TruncatedHashElement(t *testing.T) {
	got := ScrubHashes(`<Errors><Error><SipCredential><Hash1>1be6abcaa8e9956021d30f33a3925b99`)
	if strings.Contains(got, "1be6abcaa8e9956021d30f33a3925b99") {
		t.Errorf("hash in a truncated element survived scrubbing: %s", got)
	}
}

// TestScrubHashes_LeavesShorterHexAlone bounds the bare-hex sweep: it must not
// eat ordinary identifiers. Only a full 32-hex run — the rendered length of an
// MD5 digest — is treated as secret material.
func TestScrubHashes_LeavesShorterHexAlone(t *testing.T) {
	in := `<Realm>vapi-3efeaa.auth.bandwidth.com</Realm><Id>1103</Id><Trace>deadbeefcafe</Trace>`
	if got := ScrubHashes(in); got != in {
		t.Errorf("ScrubHashes mangled non-secret content:\n got  %s\n want %s", got, in)
	}
}

// TestRedactSecrets_KeepsBareHexValues pins the deliberate scoping decision:
// the bare-hex sweep belongs to ScrubHashes (raw bodies) only. A 32-hex value in
// a structured output field is far more likely to be a legitimate identifier,
// and RedactSecrets keys off field NAMES instead.
func TestRedactSecrets_KeepsBareHexValues(t *testing.T) {
	in := map[string]interface{}{"requestId": "d41d8cd98f00b204e9800998ecf8427e"}
	out := RedactSecrets(in).(map[string]interface{})
	if out["requestId"] != "d41d8cd98f00b204e9800998ecf8427e" {
		t.Errorf("requestId = %v, want it preserved — RedactSecrets redacts by key, not by value shape", out["requestId"])
	}
}
