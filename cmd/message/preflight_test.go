package message

import (
	"testing"
)

func TestIsPlaceholderURL(t *testing.T) {
	tests := []struct {
		url  string
		want bool
	}{
		{"https://example.com/callbacks", true},
		{"https://www.example.com/hooks", true},
		{"http://localhost:3000/callbacks", true},
		{"http://127.0.0.1:8080/hooks", true},
		{"https://google.com", true},
		{"https://bandwidth.com", true},
		{"https://my-app.herokuapp.com/callbacks", false},
		{"https://api.mycompany.com/webhooks/bandwidth", false},
		{"https://hooks.slack.com/services/abc", false},
		{"", false}, // empty is handled separately by the caller
	}

	for _, tc := range tests {
		t.Run(tc.url, func(t *testing.T) {
			got := isPlaceholderURL(tc.url)
			if got != tc.want {
				t.Errorf("isPlaceholderURL(%q) = %v, want %v", tc.url, got, tc.want)
			}
		})
	}
}

func TestExtractCampaigns(t *testing.T) {
	t.Run("valid response", func(t *testing.T) {
		resp := map[string]interface{}{
			"data": []interface{}{
				map[string]interface{}{"campaignId": "CR8HFN0", "status": "REGISTERED"},
				map[string]interface{}{"campaignId": "CERLUDZ", "status": "DECLINED"},
			},
		}
		campaigns := extractCampaigns(resp)
		if len(campaigns) != 2 {
			t.Fatalf("expected 2 campaigns, got %d", len(campaigns))
		}
		if campaigns[0].id != "CR8HFN0" || campaigns[0].status != "REGISTERED" {
			t.Errorf("campaigns[0] = %+v, want CR8HFN0/REGISTERED", campaigns[0])
		}
		if campaigns[1].id != "CERLUDZ" || campaigns[1].status != "DECLINED" {
			t.Errorf("campaigns[1] = %+v, want CERLUDZ/DECLINED", campaigns[1])
		}
	})

	t.Run("empty data", func(t *testing.T) {
		resp := map[string]interface{}{"data": []interface{}{}}
		campaigns := extractCampaigns(resp)
		if len(campaigns) != 0 {
			t.Errorf("expected 0 campaigns, got %d", len(campaigns))
		}
	})

	t.Run("nil response", func(t *testing.T) {
		campaigns := extractCampaigns(nil)
		if campaigns != nil {
			t.Errorf("expected nil, got %v", campaigns)
		}
	})
}

func TestFindPhoneNumberInResponse(t *testing.T) {
	resp := map[string]interface{}{
		"data": []interface{}{
			map[string]interface{}{"phoneNumber": "+17752345103", "status": "SUCCESS"},
			map[string]interface{}{"phoneNumber": "+17752345191", "status": "PENDING"},
		},
	}

	t.Run("found with SUCCESS", func(t *testing.T) {
		pn := findPhoneNumberInResponse(resp, "+17752345103")
		if pn == nil {
			t.Fatal("expected to find phone number")
		}
		if pn.status != "SUCCESS" {
			t.Errorf("status = %q, want SUCCESS", pn.status)
		}
	})

	t.Run("found with PENDING", func(t *testing.T) {
		pn := findPhoneNumberInResponse(resp, "+17752345191")
		if pn == nil {
			t.Fatal("expected to find phone number")
		}
		if pn.status != "PENDING" {
			t.Errorf("status = %q, want PENDING", pn.status)
		}
	})

	t.Run("not found", func(t *testing.T) {
		pn := findPhoneNumberInResponse(resp, "+19195551234")
		if pn != nil {
			t.Errorf("expected nil, got %+v", pn)
		}
	})

	t.Run("empty response", func(t *testing.T) {
		empty := map[string]interface{}{"data": []interface{}{}}
		pn := findPhoneNumberInResponse(empty, "+17752345103")
		if pn != nil {
			t.Errorf("expected nil, got %+v", pn)
		}
	})
}

func TestExtractTFVStatus(t *testing.T) {
	tests := []struct {
		name string
		resp interface{}
		want string
	}{
		{
			name: "verified",
			resp: map[string]interface{}{"status": "VERIFIED", "phoneNumber": "+18005551234"},
			want: "VERIFIED",
		},
		{
			name: "pending",
			resp: map[string]interface{}{"status": "PENDING"},
			want: "PENDING",
		},
		{
			name: "empty response",
			resp: map[string]interface{}{},
			want: "",
		},
		{
			name: "nil",
			resp: nil,
			want: "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := extractTFVStatus(tc.resp)
			if got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestExtractSiteIDs(t *testing.T) {
	// Simulates the XML-parsed sites response shape
	resp := map[string]interface{}{
		"SitesResponse": map[string]interface{}{
			"Sites": map[string]interface{}{
				"Site": map[string]interface{}{
					"Id":   "152681",
					"Name": "Subacct",
				},
			},
		},
	}
	ids := extractSiteIDs(resp)
	if len(ids) != 1 || ids[0] != "152681" {
		t.Errorf("got %v, want [152681]", ids)
	}
}

func TestExtractPeerIDs(t *testing.T) {
	// Simulates XML-parsed SIP peers response with multiple peers
	resp := map[string]interface{}{
		"TNSipPeersResponse": map[string]interface{}{
			"SipPeers": map[string]interface{}{
				"SipPeer": []interface{}{
					map[string]interface{}{"PeerId": "970014", "PeerName": "Test"},
					map[string]interface{}{"PeerId": "1072011", "PeerName": "Other"},
				},
			},
		},
	}
	ids := extractPeerIDs(resp)
	if len(ids) != 2 {
		t.Fatalf("expected 2 peer IDs, got %d: %v", len(ids), ids)
	}
	if ids[0] != "970014" || ids[1] != "1072011" {
		t.Errorf("got %v, want [970014, 1072011]", ids)
	}
}

func TestExtractAppIDFromSettings(t *testing.T) {
	resp := map[string]interface{}{
		"ApplicationsSettingsResponse": map[string]interface{}{
			"ApplicationsSettings": map[string]interface{}{
				"HttpMessagingV2AppId": "298e5e78-1c5f-4cc7-af8d-5c77cf2fb84c",
			},
		},
	}
	got := extractAppIDFromSettings(resp)
	if got != "298e5e78-1c5f-4cc7-af8d-5c77cf2fb84c" {
		t.Errorf("got %q, want 298e5e78-1c5f-4cc7-af8d-5c77cf2fb84c", got)
	}
}

func TestExtractAppIDFromSettings_Empty(t *testing.T) {
	resp := map[string]interface{}{}
	got := extractAppIDFromSettings(resp)
	if got != "" {
		t.Errorf("got %q, want empty", got)
	}
}

func TestFindCallbackURL(t *testing.T) {
	t.Run("messaging app", func(t *testing.T) {
		resp := map[string]interface{}{
			"Application": map[string]interface{}{
				"MsgCallbackUrl": "https://myserver.com/callbacks",
				"CallbackUrl":    "https://myserver.com/callbacks",
				"ServiceType":    "Messaging-V2",
			},
		}
		got := findCallbackURL(resp)
		if got != "https://myserver.com/callbacks" {
			t.Errorf("got %q, want https://myserver.com/callbacks", got)
		}
	})

	t.Run("no callback URL", func(t *testing.T) {
		resp := map[string]interface{}{
			"Application": map[string]interface{}{
				"ServiceType": "Messaging-V2",
			},
		}
		got := findCallbackURL(resp)
		if got != "" {
			t.Errorf("got %q, want empty", got)
		}
	})

	t.Run("nil", func(t *testing.T) {
		got := findCallbackURL(nil)
		if got != "" {
			t.Errorf("got %q, want empty", got)
		}
	})
}

func TestExtractAssociatedPeers(t *testing.T) {
	t.Run("with peers", func(t *testing.T) {
		resp := map[string]interface{}{
			"AssociatedSipPeers": map[string]interface{}{
				"AssociatedSipPeer": map[string]interface{}{
					"SiteId":   "152681",
					"PeerId":   "970014",
					"PeerName": "Test",
				},
			},
		}
		peers := extractAssociatedPeers(resp)
		if len(peers) != 1 || peers[0] != "970014" {
			t.Errorf("got %v, want [970014]", peers)
		}
	})

	t.Run("empty", func(t *testing.T) {
		resp := map[string]interface{}{}
		peers := extractAssociatedPeers(resp)
		if len(peers) != 0 {
			t.Errorf("got %v, want empty", peers)
		}
	})
}
