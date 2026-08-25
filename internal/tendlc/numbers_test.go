package tendlc

import "testing"

func TestListPhoneNumbersEncodesPagination(t *testing.T) {
	var got captured
	s := stubService(t, 200, `{"data":[],"page":{"totalElements":0}}`, &got)

	if _, err := s.ListPhoneNumbers(10, 20, nil); err != nil {
		t.Fatalf("ListPhoneNumbers: %v", err)
	}
	if got.method != "GET" {
		t.Errorf("method = %q, want GET", got.method)
	}
	if want := "/api/v2/accounts/9901287/tendlc/phoneNumbers"; got.path != want {
		t.Errorf("path = %q, want %q", got.path, want)
	}
	if got.query != "limit=10&offset=20" {
		t.Errorf("query = %q, want limit=10&offset=20", got.query)
	}
}

func TestGetPhoneNumberGetsToPhoneNumberPath(t *testing.T) {
	var got captured
	s := stubService(t, 200, `{"data":{"phoneNumber":"+15555550100"}}`, &got)

	env, err := s.GetPhoneNumber("+15555550100")
	if err != nil {
		t.Fatalf("GetPhoneNumber: %v", err)
	}
	if got.method != "GET" {
		t.Errorf("method = %q, want GET", got.method)
	}
	if want := "/api/v2/accounts/9901287/tendlc/phoneNumbers/+15555550100"; got.path != want {
		t.Errorf("path = %q, want %q", got.path, want)
	}
	obj, err := env.Object()
	if err != nil {
		t.Fatalf("Object: %v", err)
	}
	if obj["phoneNumber"] != "+15555550100" {
		t.Errorf("phoneNumber = %v, want +15555550100", obj["phoneNumber"])
	}
}

func TestPhoneNumberHistoryEncodesPagination(t *testing.T) {
	var got captured
	s := stubService(t, 200, `{"data":[],"page":{"totalElements":0}}`, &got)

	if _, err := s.PhoneNumberHistory("+15555550100", 10, 20); err != nil {
		t.Fatalf("PhoneNumberHistory: %v", err)
	}
	if got.method != "GET" {
		t.Errorf("method = %q, want GET", got.method)
	}
	if want := "/api/v2/accounts/9901287/tendlc/phoneNumbers/+15555550100/history"; got.path != want {
		t.Errorf("path = %q, want %q", got.path, want)
	}
	if got.query != "limit=10&offset=20" {
		t.Errorf("query = %q, want limit=10&offset=20", got.query)
	}
}

// Every method that takes a phone number must reject an empty one before
// making a request. Without this a caller with an unset variable silently
// hits the collection endpoint — GET on /phoneNumbers rather than
// /phoneNumbers/{tn}.
func TestEmptyPhoneNumbersRejectedWithoutRequest(t *testing.T) {
	var got captured
	s := stubService(t, 200, `{"data":{}}`, &got)

	calls := map[string]func() error{
		"GetPhoneNumber": func() error { _, err := s.GetPhoneNumber(""); return err },
		"PhoneNumberHistory": func() error {
			_, err := s.PhoneNumberHistory("", 10, 0)
			return err
		},
	}
	for name, call := range calls {
		t.Run(name, func(t *testing.T) {
			got = captured{}
			if err := call(); err == nil {
				t.Fatal("want an error for an empty phone number, got nil")
			}
			if got.method != "" {
				t.Errorf("a request was made (%s %s); want none", got.method, got.path)
			}
		})
	}
}

// A phone number goes into the path, so a value containing a slash or a
// space must be escaped rather than silently changing which endpoint is
// called. The leading '+' in a real E.164 number is left alone by
// url.PathEscape (see phoneNumberPath), so a value with just a space is not
// enough to prove this test has teeth: net/url's EscapedPath derives its own
// canonical encoding from the decoded Path whenever RawPath isn't already in
// that exact form, so an unescaped space gets "corrected" to %20 on the way
// out regardless of whether url.PathEscape ran. A slash does not get that
// treatment — an unescaped '/' is a real path separator (an extra segment),
// while an escaped one is %2F, so only the slash form actually distinguishes
// "escaped" from "not escaped". Assert on got.escapedPath, not got.path:
// net/url decodes Path, so an assertion against it would pass whether or not
// url.PathEscape was called.
func TestPhoneNumberIsPathEscaped(t *testing.T) {
	var got captured
	s := stubService(t, 200, `{"data":[],"page":{"totalElements":0}}`, &got)

	if _, err := s.PhoneNumberHistory("+1/555 0100", 10, 0); err != nil {
		t.Fatalf("PhoneNumberHistory: %v", err)
	}
	if want := "/api/v2/accounts/9901287/tendlc/phoneNumbers/+1%2F555%200100/history"; got.escapedPath != want {
		t.Errorf("escaped path = %q, want %q", got.escapedPath, want)
	}
}
