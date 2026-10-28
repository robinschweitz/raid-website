package httpapi_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"raid-website/internal/validation"
)

type editedPoll struct {
	ID       int64   `json:"id"`
	Title    string  `json:"title"`
	Status   string  `json:"status"`
	ClosesAt *string `json:"closes_at"`
	Options  []struct {
		ID    int64  `json:"id"`
		Label string `json:"label"`
	} `json:"options"`
}

func decodePoll(t *testing.T, body []byte) editedPoll {
	t.Helper()
	var out struct {
		Data editedPoll `json:"data"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		t.Fatalf("decode poll: %v body %s", err, body)
	}
	return out.Data
}

// TestReqPoll007_AC01: staff edit title, description, closing time and
// options; a renamed option keeps its votes under the new label (FR-24.7).
func TestReqPoll007_AC01(t *testing.T) {
	srv, bdb := setupServer(t)
	client := srv.Client()
	seedAccount(t, bdb, "lead", "lead-password", validation.RoleRaidassistent)
	seedAccount(t, bdb, "member", "member-password", validation.RoleMember)
	lead := login(t, client, srv, "lead", "lead-password")
	member := login(t, client, srv, "member", "member-password")

	status, body := do(t, client, http.MethodPost, srv.URL+"/api/v1/polls", lead, map[string]any{
		"title": "Raidtag", "poll_type": "CHOICE", "options": []string{"Mittwoch", "Donnerstag", "Freitag"},
	})
	if status != http.StatusCreated {
		t.Fatalf("create: %d %s", status, body)
	}
	created := decodePoll(t, body)
	pollURL := fmt.Sprintf("%s/api/v1/polls/%d", srv.URL, created.ID)
	if status, body := do(t, client, http.MethodPost, pollURL+"/vote", member, map[string]any{"choices": []string{"Donnerstag"}}); status != http.StatusOK {
		t.Fatalf("vote: %d %s", status, body)
	}

	closesAt := time.Now().UTC().Add(48 * time.Hour).Format(time.RFC3339)
	status, body = do(t, client, http.MethodPut, pollURL, lead, map[string]any{
		"title": "Raidtag im Dezember", "description": "Bitte bis Sonntag", "closes_at": closesAt, "status": "OPEN",
		"options": []map[string]any{
			{"id": created.Options[1].ID, "label": "Donnerstag (20 Uhr)"},
			{"id": created.Options[0].ID, "label": "Mittwoch"},
			{"id": 0, "label": "Samstag"},
		},
	})
	if status != http.StatusOK {
		t.Fatalf("edit: %d %s", status, body)
	}
	edited := decodePoll(t, body)
	labels := []string{}
	for _, option := range edited.Options {
		labels = append(labels, option.Label)
	}
	if edited.Title != "Raidtag im Dezember" || strings.Join(labels, "|") != "Donnerstag (20 Uhr)|Mittwoch|Samstag" || edited.ClosesAt == nil {
		t.Fatalf("edited poll: %+v", edited)
	}

	status, body = do(t, client, http.MethodGet, pollURL+"/votes", member, nil)
	if status != http.StatusOK || !strings.Contains(string(body), `"choices":["Donnerstag (20 Uhr)"]`) || !strings.Contains(string(body), `"choice":"Donnerstag (20 Uhr)"`) {
		t.Fatalf("vote must follow the renamed option: %d %s", status, body)
	}
	if status, body := do(t, client, http.MethodPost, pollURL+"/vote", member, map[string]any{"choices": []string{"Samstag"}}); status != http.StatusOK {
		t.Fatalf("vote for the new option: %d %s", status, body)
	}

	var auditCount int
	if err := bdb.NewSelect().Table("audit_log").Where("entity_type = 'poll' AND action = 'update'").ColumnExpr("count(*)").Scan(t.Context(), &auditCount); err != nil {
		t.Fatalf("audit: %v", err)
	}
	if auditCount != 1 {
		t.Fatalf("want one update audit entry, got %d", auditCount)
	}
}

// TestReqPoll007_AC02: an option that has votes cannot be removed, and the
// whole edit is rejected without changes (FR-24.7).
func TestReqPoll007_AC02(t *testing.T) {
	srv, bdb := setupServer(t)
	client := srv.Client()
	seedAccount(t, bdb, "lead", "lead-password", validation.RoleRaidassistent)
	lead := login(t, client, srv, "lead", "lead-password")

	_, body := do(t, client, http.MethodPost, srv.URL+"/api/v1/polls", lead, map[string]any{
		"title": "Boss", "poll_type": "CHOICE", "options": []string{"Lich King", "Sindragosa"},
	})
	created := decodePoll(t, body)
	pollURL := fmt.Sprintf("%s/api/v1/polls/%d", srv.URL, created.ID)
	do(t, client, http.MethodPost, pollURL+"/vote", lead, map[string]any{"choices": []string{"Sindragosa"}})

	status, body := do(t, client, http.MethodPut, pollURL, lead, map[string]any{
		"title": "Geändert", "description": "", "closes_at": "", "status": "OPEN",
		"options": []map[string]any{{"id": created.Options[0].ID, "label": "Lich King"}, {"id": 0, "label": "Putricide"}},
	})
	if status != http.StatusConflict || !strings.Contains(string(body), `"code":"poll_option_has_votes"`) {
		t.Fatalf("want 409 poll_option_has_votes, got %d %s", status, body)
	}
	_, body = do(t, client, http.MethodGet, pollURL, lead, nil)
	if after := decodePoll(t, body); after.Title != "Boss" || len(after.Options) != 2 {
		t.Fatalf("rejected edit must change nothing: %+v", after)
	}
}

// TestReqPoll007_AC03: a closed poll can be reopened and then takes votes again (FR-24.7).
func TestReqPoll007_AC03(t *testing.T) {
	srv, bdb := setupServer(t)
	client := srv.Client()
	seedAccount(t, bdb, "lead", "lead-password", validation.RoleRaidassistent)
	lead := login(t, client, srv, "lead", "lead-password")

	_, body := do(t, client, http.MethodPost, srv.URL+"/api/v1/polls", lead, map[string]any{"title": "Termin"})
	pollURL := fmt.Sprintf("%s/api/v1/polls/%d", srv.URL, decodePoll(t, body).ID)
	do(t, client, http.MethodPost, pollURL+"/close", lead, nil)

	past := time.Now().UTC().Add(-time.Hour).Format(time.RFC3339)
	status, body := do(t, client, http.MethodPut, pollURL, lead, map[string]any{"title": "Termin", "description": "", "closes_at": past, "status": "OPEN", "options": []any{}})
	if status != http.StatusBadRequest || !strings.Contains(string(body), "invalid_request") {
		t.Fatalf("an open poll with a past closing time: want 400, got %d %s", status, body)
	}
	status, body = do(t, client, http.MethodPut, pollURL, lead, map[string]any{"title": "Termin", "description": "", "closes_at": "", "status": "OPEN", "options": []any{}})
	if status != http.StatusOK || decodePoll(t, body).Status != "OPEN" {
		t.Fatalf("reopen: %d %s", status, body)
	}
	if status, body := do(t, client, http.MethodPost, pollURL+"/vote", lead, map[string]any{"choice": "Freitag"}); status != http.StatusOK {
		t.Fatalf("vote after reopen: %d %s", status, body)
	}
}

// TestReqPoll007_AC04: invalid edits are rejected (FR-24.7, SC-020).
func TestReqPoll007_AC04(t *testing.T) {
	srv, bdb := setupServer(t)
	client := srv.Client()
	seedAccount(t, bdb, "lead", "lead-password", validation.RoleRaidassistent)
	lead := login(t, client, srv, "lead", "lead-password")
	_, body := do(t, client, http.MethodPost, srv.URL+"/api/v1/polls", lead, map[string]any{"title": "Boss", "poll_type": "CHOICE", "options": []string{"A", "B"}})
	choice := decodePoll(t, body)
	choiceURL := fmt.Sprintf("%s/api/v1/polls/%d", srv.URL, choice.ID)
	_, body = do(t, client, http.MethodPost, srv.URL+"/api/v1/polls", lead, map[string]any{"title": "Text"})
	textURL := fmt.Sprintf("%s/api/v1/polls/%d", srv.URL, decodePoll(t, body).ID)

	keep := func(labels ...string) []map[string]any {
		rows := []map[string]any{}
		for i, label := range labels {
			id := int64(0)
			if i < len(choice.Options) {
				id = choice.Options[i].ID
			}
			rows = append(rows, map[string]any{"id": id, "label": label})
		}
		return rows
	}
	cases := []struct {
		name string
		url  string
		body map[string]any
		want int
	}{
		{"empty title", choiceURL, map[string]any{"title": " ", "description": "", "closes_at": "", "status": "OPEN", "options": keep("A", "B")}, http.StatusBadRequest},
		{"one option", choiceURL, map[string]any{"title": "Boss", "description": "", "closes_at": "", "status": "OPEN", "options": keep("A")}, http.StatusBadRequest},
		{"duplicate labels", choiceURL, map[string]any{"title": "Boss", "description": "", "closes_at": "", "status": "OPEN", "options": keep("A", "A")}, http.StatusBadRequest},
		{"foreign option id", choiceURL, map[string]any{"title": "Boss", "description": "", "closes_at": "", "status": "OPEN", "options": []map[string]any{{"id": 999999, "label": "X"}, {"id": 0, "label": "Y"}}}, http.StatusBadRequest},
		{"bad status", choiceURL, map[string]any{"title": "Boss", "description": "", "closes_at": "", "status": "ARCHIVED", "options": keep("A", "B")}, http.StatusBadRequest},
		{"options on a text poll", textURL, map[string]any{"title": "Text", "description": "", "closes_at": "", "status": "OPEN", "options": []map[string]any{{"id": 0, "label": "A"}}}, http.StatusBadRequest},
		{"unknown poll", srv.URL + "/api/v1/polls/999999", map[string]any{"title": "x", "description": "", "closes_at": "", "status": "OPEN", "options": []any{}}, http.StatusNotFound},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if status, body := do(t, client, http.MethodPut, tc.url, lead, tc.body); status != tc.want {
				t.Fatalf("want %d got %d %s", tc.want, status, body)
			}
		})
	}
	// Swapping two labels must not trip the unique label constraint.
	if status, body := do(t, client, http.MethodPut, choiceURL, lead, map[string]any{"title": "Boss", "description": "", "closes_at": "", "status": "OPEN", "options": keep("B", "A")}); status != http.StatusOK {
		t.Fatalf("swap labels: %d %s", status, body)
	}
}

// TestReqPoll007_AC05: editing a poll needs RAIDASSISTENT; a MEMBER gets 403
// and a request without a session 401 (FR-24.7, SC-001).
func TestReqPoll007_AC05(t *testing.T) {
	srv, bdb := setupServer(t)
	client := srv.Client()
	seedAccount(t, bdb, "lead", "lead-password", validation.RoleRaidassistent)
	seedAccount(t, bdb, "member", "member-password", validation.RoleMember)
	lead := login(t, client, srv, "lead", "lead-password")
	member := login(t, client, srv, "member", "member-password")
	_, body := do(t, client, http.MethodPost, srv.URL+"/api/v1/polls", lead, map[string]any{"title": "Termin"})
	pollURL := fmt.Sprintf("%s/api/v1/polls/%d", srv.URL, decodePoll(t, body).ID)
	edit := map[string]any{"title": "Neu", "description": "", "closes_at": "", "status": "OPEN", "options": []any{}}
	if status, body := do(t, client, http.MethodPut, pollURL, member, edit); status != http.StatusForbidden {
		t.Fatalf("member: want 403 got %d %s", status, body)
	}
	if status, body := do(t, client, http.MethodPut, pollURL, "", edit); status != http.StatusUnauthorized {
		t.Fatalf("no session: want 401 got %d %s", status, body)
	}
}
