package bxml

import (
	"bytes"
	"strings"
	"testing"
)

// executeCommand runs a bxml subcommand and captures stdout.
func executeCommand(args ...string) (string, error) {
	buf := new(bytes.Buffer)
	Cmd.SetOut(buf)
	Cmd.SetErr(buf)
	Cmd.SetArgs(args)
	err := Cmd.Execute()
	return buf.String(), err
}

// --- speak ---

func TestSpeakBasic(t *testing.T) {
	out, err := executeCommand("speak", "Hello world")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "<SpeakSentence>Hello world</SpeakSentence>") {
		t.Errorf("expected SpeakSentence with text, got:\n%s", out)
	}
	if !strings.Contains(out, `<?xml version="1.0" encoding="UTF-8"?>`) {
		t.Errorf("expected XML declaration, got:\n%s", out)
	}
	if !strings.Contains(out, "<Response>") {
		t.Errorf("expected Response wrapper, got:\n%s", out)
	}
}

func TestSpeakWithVoice(t *testing.T) {
	out, err := executeCommand("speak", "--voice", "julie", "Press 1 for sales")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, `voice="julie"`) {
		t.Errorf("expected voice attribute, got:\n%s", out)
	}
	if !strings.Contains(out, "Press 1 for sales") {
		t.Errorf("expected text content, got:\n%s", out)
	}
}

func TestSpeakXMLEscaping(t *testing.T) {
	out, err := executeCommand("speak", `He said "hello" & <goodbye>`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if strings.Contains(out, "&") && !strings.Contains(out, "&amp;") {
		t.Errorf("expected & to be escaped, got:\n%s", out)
	}
	if strings.Contains(out, "<goodbye>") && !strings.Contains(out, "&lt;goodbye&gt;") {
		t.Errorf("expected angle brackets to be escaped, got:\n%s", out)
	}
}

func TestSpeakRequiresArg(t *testing.T) {
	_, err := executeCommand("speak")
	if err == nil {
		t.Fatal("expected error for missing arg")
	}
}

// --- gather ---

func TestGatherBasic(t *testing.T) {
	out, err := executeCommand("gather", "--url", "https://example.com/gather")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, `gatherUrl="https://example.com/gather"`) {
		t.Errorf("expected gatherUrl attribute, got:\n%s", out)
	}
	if !strings.Contains(out, "<Gather") {
		t.Errorf("expected Gather element, got:\n%s", out)
	}
}

func TestGatherWithOptions(t *testing.T) {
	out, err := executeCommand("gather", "--url", "https://example.com/gather", "--max-digits", "4", "--prompt", "Enter your PIN")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, `maxDigits="4"`) {
		t.Errorf("expected maxDigits attribute, got:\n%s", out)
	}
	if !strings.Contains(out, "<SpeakSentence>Enter your PIN</SpeakSentence>") {
		t.Errorf("expected prompt SpeakSentence, got:\n%s", out)
	}
}

func TestGatherPromptEscaping(t *testing.T) {
	out, err := executeCommand("gather", "--url", "https://example.com", "--prompt", "Press 1 & 2")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "Press 1 &amp; 2") {
		t.Errorf("expected escaped prompt text, got:\n%s", out)
	}
}

// --- record ---

func TestRecordBasic(t *testing.T) {
	out, err := executeCommand("record")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "<Record/>") {
		t.Errorf("expected bare Record element, got:\n%s", out)
	}
}

func TestRecordWithOptions(t *testing.T) {
	out, err := executeCommand("record", "--url", "https://example.com/done", "--max-duration", "60")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, `recordCompleteUrl="https://example.com/done"`) {
		t.Errorf("expected recordCompleteUrl attribute, got:\n%s", out)
	}
	if !strings.Contains(out, `maxDuration="60"`) {
		t.Errorf("expected maxDuration attribute, got:\n%s", out)
	}
}

// --- transfer ---

func TestTransferBasic(t *testing.T) {
	out, err := executeCommand("transfer", "+19195551234")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "<PhoneNumber>+19195551234</PhoneNumber>") {
		t.Errorf("expected PhoneNumber element, got:\n%s", out)
	}
	if !strings.Contains(out, "<Transfer>") {
		t.Errorf("expected Transfer element, got:\n%s", out)
	}
}

func TestTransferWithCallerID(t *testing.T) {
	out, err := executeCommand("transfer", "+19195551234", "--caller-id", "+19195550000")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, `transferCallerId="+19195550000"`) {
		t.Errorf("expected transferCallerId attribute, got:\n%s", out)
	}
}

func TestTransferRequiresArg(t *testing.T) {
	_, err := executeCommand("transfer")
	if err == nil {
		t.Fatal("expected error for missing phone number arg")
	}
}

// --- raw ---

func TestRawValidXML(t *testing.T) {
	xml := `<Response><SpeakSentence>Hello</SpeakSentence></Response>`
	out, err := executeCommand("raw", xml)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// raw pretty-prints the XML with indentation
	if !strings.Contains(out, "<Response>") {
		t.Errorf("expected <Response> element, got:\n%s", out)
	}
	if !strings.Contains(out, "  <SpeakSentence>Hello</SpeakSentence>") {
		t.Errorf("expected indented <SpeakSentence>, got:\n%s", out)
	}
}

func TestRawInvalidXML(t *testing.T) {
	_, err := executeCommand("raw", "<Response><Unclosed>")
	if err == nil {
		t.Fatal("expected error for invalid XML")
	}
	if !strings.Contains(err.Error(), "invalid XML") {
		t.Errorf("expected 'invalid XML' error, got: %v", err)
	}
}

func TestRawRequiresArg(t *testing.T) {
	_, err := executeCommand("raw")
	if err == nil {
		t.Fatal("expected error for missing arg")
	}
}

// --- xmlEscape ---

func TestXmlEscape(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"hello", "hello"},
		{"a & b", "a &amp; b"},
		{"<tag>", "&lt;tag&gt;"},
		{`"quoted"`, "&#34;quoted&#34;"},
		{"it's", "it&#39;s"},
	}

	for _, tc := range tests {
		got := xmlEscape(tc.input)
		if got != tc.expected {
			t.Errorf("xmlEscape(%q) = %q, want %q", tc.input, got, tc.expected)
		}
	}
}
