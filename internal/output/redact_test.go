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
