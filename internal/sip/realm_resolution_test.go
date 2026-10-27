package sip

import (
	"context"
	"fmt"
	"net/http"
	"reflect"
	"testing"

	"github.com/Bandwidth/cli/internal/cmdutil"
)

func TestGetRealmResolution(t *testing.T) {
	const base = "/accounts/9901361/realms"
	const host = "my-vapi-3efeaa.auth.bandwidth.com"
	const record = `<Realm><Id>1103</Id><Realm>` + host + `</Realm><Status>ACTIVE</Status><Description>full detail</Description></Realm>`
	for _, tc := range []struct {
		name       string
		ref        string
		status     int
		listStatus int
		list       string
		wantExit   int
		paths      []string
	}{
		{"name", "my-vapi", 404, 200, record, 0, []string{base + "/my-vapi", base, base + "/1103"}},
		{"case insensitive", "MY-VAPI", 404, 200, record, 0, []string{base + "/MY-VAPI", base, base + "/1103"}},
		{"ID", "1103", 200, 0, "", 0, []string{base + "/1103"}},
		{"FQDN", host, 200, 0, "", 0, []string{base + "/" + host}},
		{"direct name supported", "my-vapi", 200, 0, "", 0, []string{base + "/my-vapi"}},
		{"missing name", "missing", 404, 200, record, 3, []string{base + "/missing", base}},
		{"missing ID", "9999", 404, 0, "", 3, []string{base + "/9999"}},
		{"missing FQDN", host, 404, 0, "", 3, []string{base + "/" + host}},
		{"unauthorized", "my-vapi", 401, 0, "", 2, []string{base + "/my-vapi"}},
		{"forbidden", "my-vapi", 403, 0, "", 2, []string{base + "/my-vapi"}},
		{"server failure", "my-vapi", 500, 0, "", 1, []string{base + "/my-vapi"}},
		{"list forbidden", "my-vapi", 404, 403, "", 2, []string{base + "/my-vapi", base}},
		{"ambiguous name", "my-vapi", 404, 200, record + record, 6, []string{base + "/my-vapi", base}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var paths []string
			svc, done := newTestService(t, func(w http.ResponseWriter, r *http.Request) {
				paths = append(paths, r.URL.Path)
				if r.Method != http.MethodGet {
					t.Errorf("unexpected method %s", r.Method)
				}
				status := tc.status
				body := `<RealmResponse>` + record + `</RealmResponse>`
				if r.URL.Path == base {
					status = tc.listStatus
					body = `<RealmsResponse><Realms>` + tc.list + `</Realms></RealmsResponse>`
				} else if len(paths) == 3 && r.URL.Path == base+"/1103" {
					status = 200
				}
				if status == 0 {
					status = 500
				}
				w.WriteHeader(status)
				if status != 200 {
					body = `<RealmResponse><ResponseStatus><Description>request failed</Description></ResponseStatus></RealmResponse>`
				}
				fmt.Fprint(w, body)
			})
			defer done()
			realm, err := svc.GetRealm(context.Background(), tc.ref)
			if got := cmdutil.ExitCodeForError(err); got != tc.wantExit {
				t.Fatalf("exit = %d, want %d; error = %v", got, tc.wantExit, err)
			}
			if err == nil && (realm.ID != "1103" || realm.Hostname != host || realm.Description != "full detail") {
				t.Errorf("unexpected realm: %+v", realm)
			}
			if !reflect.DeepEqual(paths, tc.paths) {
				t.Errorf("requests = %v, want %v", paths, tc.paths)
			}
		})
	}
}
