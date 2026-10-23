package httpapi_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	"raid-website/internal/validation"
)

// TestReqRaid013_AC01: a poll write without a session answers 401 login_required
// and the error text never mentions the removed X-Actor-ID header (FR-24.6, SC-003).
func TestReqRaid013_AC01(t *testing.T) {
	srv, bdb := setupServer(t)
	client := srv.Client()
	seedAccount(t, bdb, "admin", "admin-pw", validation.RoleAdmin)
	adminToken := login(t, client, srv, "admin", "admin-pw")

	status, body := do(t, client, http.MethodPost, srv.URL+"/api/v1/polls", adminToken, map[string]any{"title": "Termin"})
	if status != http.StatusCreated {
		t.Fatalf("create poll: status %d body %s", status, body)
	}
	var pollOut struct {
		Data struct {
			ID int64 `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &pollOut); err != nil {
		t.Fatalf("decode poll: %v", err)
	}
	pollURL := fmt.Sprintf("%s/api/v1/polls/%d", srv.URL, pollOut.Data.ID)

	cases := []struct {
		name   string
		method string
		url    string
		body   any
		header map[string]string
	}{
		{"vote", http.MethodPost, pollURL + "/vote", map[string]any{"choice": "yes"}, nil},
		{"vote with legacy actor header", http.MethodPost, pollURL + "/vote", map[string]any{"choice": "yes"}, map[string]string{"X-Actor-ID": "1"}},
		{"create", http.MethodPost, srv.URL + "/api/v1/polls", map[string]any{"title": "x"}, nil},
		{"close", http.MethodPost, pollURL + "/close", nil, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var reader io.Reader
			if tc.body != nil {
				b, err := json.Marshal(tc.body)
				if err != nil {
					t.Fatalf("marshal body: %v", err)
				}
				reader = bytes.NewReader(b)
			}
			req, err := http.NewRequest(tc.method, tc.url, reader)
			if err != nil {
				t.Fatalf("new request: %v", err)
			}
			req.Header.Set("Content-Type", "application/json")
			for k, v := range tc.header {
				req.Header.Set(k, v)
			}
			resp, err := client.Do(req)
			if err != nil {
				t.Fatalf("%s: %v", tc.name, err)
			}
			defer resp.Body.Close()
			raw, err := io.ReadAll(resp.Body)
			if err != nil {
				t.Fatalf("read body: %v", err)
			}
			if resp.StatusCode != http.StatusUnauthorized {
				t.Fatalf("want 401 got %d body %s", resp.StatusCode, raw)
			}
			var out struct {
				Error struct {
					Code    string `json:"code"`
					Message string `json:"message"`
				} `json:"error"`
			}
			if err := json.Unmarshal(raw, &out); err != nil {
				t.Fatalf("decode error body: %v", err)
			}
			if out.Error.Code != "login_required" {
				t.Fatalf("want code login_required got %q body %s", out.Error.Code, raw)
			}
			if strings.Contains(string(raw), "X-Actor-ID") {
				t.Fatalf("error body mentions X-Actor-ID: %s", raw)
			}
		})
	}
}

// TestReqRaid013_AC02: a MEMBER session can still vote on an open poll.
func TestReqRaid013_AC02(t *testing.T) {
	srv, bdb := setupServer(t)
	client := srv.Client()
	seedAccount(t, bdb, "admin", "admin-pw", validation.RoleAdmin)
	seedAccount(t, bdb, "member", "member-pw", validation.RoleMember)
	adminToken := login(t, client, srv, "admin", "admin-pw")
	memberToken := login(t, client, srv, "member", "member-pw")

	status, body := do(t, client, http.MethodPost, srv.URL+"/api/v1/polls", adminToken, map[string]any{"title": "Termin"})
	if status != http.StatusCreated {
		t.Fatalf("create poll: status %d body %s", status, body)
	}
	var pollOut struct {
		Data struct {
			ID int64 `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &pollOut); err != nil {
		t.Fatalf("decode poll: %v", err)
	}

	status, body = do(t, client, http.MethodPost, fmt.Sprintf("%s/api/v1/polls/%d/vote", srv.URL, pollOut.Data.ID), memberToken, map[string]any{"choice": "ja"})
	if status != http.StatusOK {
		t.Fatalf("member vote: want 200 got %d body %s", status, body)
	}
}
