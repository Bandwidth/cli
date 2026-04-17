package api

import (
	"strings"
	"testing"
)

// ---- MapToXML tests ----

func TestMapToXML_FlatMap(t *testing.T) {
	data := map[string]interface{}{
		"PeerName":      "My Location",
		"IsDefaultPeer": "true",
	}
	got, err := MapToXML("SipPeer", data)
	if err != nil {
		t.Fatalf("MapToXML() error: %v", err)
	}

	s := string(got)
	if !strings.Contains(s, "<SipPeer>") {
		t.Errorf("expected <SipPeer> root element, got:\n%s", s)
	}
	if !strings.Contains(s, "<PeerName>My Location</PeerName>") {
		t.Errorf("expected PeerName element, got:\n%s", s)
	}
	if !strings.Contains(s, "<IsDefaultPeer>true</IsDefaultPeer>") {
		t.Errorf("expected IsDefaultPeer element, got:\n%s", s)
	}
	if !strings.Contains(s, "</SipPeer>") {
		t.Errorf("expected closing </SipPeer>, got:\n%s", s)
	}
}

func TestMapToXML_NestedMap(t *testing.T) {
	data := map[string]interface{}{
		"TelephoneNumberList": map[string]interface{}{
			"TelephoneNumber": "5551234567",
		},
	}
	got, err := MapToXML("Order", data)
	if err != nil {
		t.Fatalf("MapToXML() error: %v", err)
	}

	s := string(got)
	if !strings.Contains(s, "<Order>") {
		t.Errorf("expected <Order> root, got:\n%s", s)
	}
	if !strings.Contains(s, "<TelephoneNumberList>") {
		t.Errorf("expected <TelephoneNumberList>, got:\n%s", s)
	}
	if !strings.Contains(s, "<TelephoneNumber>5551234567</TelephoneNumber>") {
		t.Errorf("expected <TelephoneNumber>, got:\n%s", s)
	}
}

func TestMapToXML_SliceValues(t *testing.T) {
	data := map[string]interface{}{
		"TelephoneNumberList": map[string]interface{}{
			"TelephoneNumber": []string{"5551111111", "5552222222"},
		},
	}
	got, err := MapToXML("Order", data)
	if err != nil {
		t.Fatalf("MapToXML() error: %v", err)
	}

	s := string(got)
	if !strings.Contains(s, "<TelephoneNumber>5551111111</TelephoneNumber>") {
		t.Errorf("expected first TelephoneNumber, got:\n%s", s)
	}
	if !strings.Contains(s, "<TelephoneNumber>5552222222</TelephoneNumber>") {
		t.Errorf("expected second TelephoneNumber, got:\n%s", s)
	}
}

func TestMapToXML_ApplicationFields(t *testing.T) {
	data := map[string]interface{}{
		"AppName":                  "My App",
		"CallInitiatedCallbackUrl": "https://example.com/voice",
		"ServiceType":              "Voice-V2",
	}
	got, err := MapToXML("Application", data)
	if err != nil {
		t.Fatalf("MapToXML() error: %v", err)
	}

	s := string(got)
	if !strings.Contains(s, "<Application>") {
		t.Errorf("expected <Application> root, got:\n%s", s)
	}
	if !strings.Contains(s, "<AppName>My App</AppName>") {
		t.Errorf("expected AppName, got:\n%s", s)
	}
	if !strings.Contains(s, "<ServiceType>Voice-V2</ServiceType>") {
		t.Errorf("expected ServiceType, got:\n%s", s)
	}
}

func TestMapToXML_XMLEscaping(t *testing.T) {
	data := map[string]interface{}{
		"Name": "Site <>&\"",
	}
	got, err := MapToXML("Site", data)
	if err != nil {
		t.Fatalf("MapToXML() error: %v", err)
	}

	s := string(got)
	// The special chars should be XML-escaped.
	if strings.Contains(s, "<>&\"") {
		t.Errorf("expected XML-escaped content, got unescaped:\n%s", s)
	}
	if !strings.Contains(s, "&lt;") && !strings.Contains(s, "&#") {
		t.Errorf("expected escaped < in output, got:\n%s", s)
	}
}

// ---- XMLToMap tests ----

func TestXMLToMap_Simple(t *testing.T) {
	input := `<?xml version="1.0" encoding="UTF-8"?>
<SipPeer>
  <PeerName>My Location</PeerName>
  <IsDefaultPeer>false</IsDefaultPeer>
</SipPeer>`

	got, err := XMLToMap([]byte(input))
	if err != nil {
		t.Fatalf("XMLToMap() error: %v", err)
	}

	root, ok := got["SipPeer"]
	if !ok {
		t.Fatalf("expected SipPeer key in result, got: %v", got)
	}
	m, ok := root.(map[string]interface{})
	if !ok {
		t.Fatalf("expected SipPeer value to be a map, got %T", root)
	}
	pn, ok := m["PeerName"]
	if !ok {
		t.Fatal("expected PeerName in SipPeer")
	}
	if pn != "My Location" {
		t.Errorf("PeerName = %q, want %q", pn, "My Location")
	}
}

func TestXMLToMap_Nested(t *testing.T) {
	input := `<?xml version="1.0" encoding="UTF-8"?>
<OrderResponse>
  <Order>
    <TelephoneNumberList>
      <TelephoneNumber>5551234567</TelephoneNumber>
    </TelephoneNumberList>
  </Order>
</OrderResponse>`

	got, err := XMLToMap([]byte(input))
	if err != nil {
		t.Fatalf("XMLToMap() error: %v", err)
	}

	root, ok := got["OrderResponse"]
	if !ok {
		t.Fatalf("expected OrderResponse key, got: %v", got)
	}
	rootMap, ok := root.(map[string]interface{})
	if !ok {
		t.Fatalf("expected map under OrderResponse, got %T", root)
	}
	order, ok := rootMap["Order"]
	if !ok {
		t.Fatalf("expected Order key, got: %v", rootMap)
	}
	_ = order // nested structure present
}

func TestXMLToMap_RepeatedElements(t *testing.T) {
	input := `<?xml version="1.0" encoding="UTF-8"?>
<TelephoneNumberList>
  <TelephoneNumber>5551111111</TelephoneNumber>
  <TelephoneNumber>5552222222</TelephoneNumber>
</TelephoneNumberList>`

	got, err := XMLToMap([]byte(input))
	if err != nil {
		t.Fatalf("XMLToMap() error: %v", err)
	}

	root, ok := got["TelephoneNumberList"]
	if !ok {
		t.Fatalf("expected TelephoneNumberList key, got: %v", got)
	}
	rootMap, ok := root.(map[string]interface{})
	if !ok {
		t.Fatalf("expected map under TelephoneNumberList, got %T", root)
	}
	nums, ok := rootMap["TelephoneNumber"]
	if !ok {
		t.Fatal("expected TelephoneNumber key")
	}
	slice, ok := nums.([]interface{})
	if !ok {
		t.Fatalf("expected []interface{} for repeated elements, got %T", nums)
	}
	if len(slice) != 2 {
		t.Errorf("expected 2 TelephoneNumber entries, got %d", len(slice))
	}
}

func TestXMLToMap_EmptyElement(t *testing.T) {
	input := `<Site><Name>My Site</Name></Site>`
	got, err := XMLToMap([]byte(input))
	if err != nil {
		t.Fatalf("XMLToMap() error: %v", err)
	}
	if _, ok := got["Site"]; !ok {
		t.Errorf("expected Site key, got: %v", got)
	}
}

// ---- Round-trip test ----

func TestMapToXML_XMLToMap_Roundtrip(t *testing.T) {
	original := map[string]interface{}{
		"AppName":     "Test App",
		"ServiceType": "Voice-V2",
	}

	xmlBytes, err := MapToXML("Application", original)
	if err != nil {
		t.Fatalf("MapToXML() error: %v", err)
	}

	parsed, err := XMLToMap(xmlBytes)
	if err != nil {
		t.Fatalf("XMLToMap() error: %v", err)
	}

	root, ok := parsed["Application"]
	if !ok {
		t.Fatalf("expected Application key after roundtrip, got: %v", parsed)
	}
	m, ok := root.(map[string]interface{})
	if !ok {
		t.Fatalf("expected map under Application, got %T", root)
	}
	if m["AppName"] != "Test App" {
		t.Errorf("AppName = %v, want %q", m["AppName"], "Test App")
	}
	if m["ServiceType"] != "Voice-V2" {
		t.Errorf("ServiceType = %v, want %q", m["ServiceType"], "Voice-V2")
	}
}
