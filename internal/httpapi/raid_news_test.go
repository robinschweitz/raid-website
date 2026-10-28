package httpapi_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"raid-website/internal/validation"
)

func TestRaidNewsPermissionsAndAcknowledgement(t *testing.T) {
	srv, db := setupServer(t)
	seedAccount(t, db, "news-staff", "password", validation.RoleRaidassistent)
	seedAccount(t, db, "news-member", "password", validation.RoleMember)
	staff := login(t, srv.Client(), srv, "news-staff", "password")
	member := login(t, srv.Client(), srv, "news-member", "password")
	url := srv.URL + "/api/v1/raid-news"
	payload := map[string]any{"title": "Rotation", "body": "Alle kommen mal dran.", "important": true, "published": true}

	status, body := do(t, srv.Client(), http.MethodPost, url, member, payload)
	if status != http.StatusForbidden {
		t.Fatalf("member created news: %d %s", status, body)
	}
	status, body = do(t, srv.Client(), http.MethodPost, url, staff, payload)
	if status != http.StatusCreated {
		t.Fatalf("create: %d %s", status, body)
	}
	var created struct {
		Data struct {
			ID int64 `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &created); err != nil {
		t.Fatal(err)
	}
	if created.Data.ID == 0 {
		t.Fatalf("missing ID: %s", body)
	}
	id := created.Data.ID
	path := fmt.Sprintf("%s/%d", url, id)

	status, body = do(t, srv.Client(), http.MethodPut, path, member, payload)
	if status != http.StatusForbidden {
		t.Fatalf("member edited news: %d %s", status, body)
	}
	status, body = do(t, srv.Client(), http.MethodPost, path+"/acknowledge", member, nil)
	if status != http.StatusNoContent {
		t.Fatalf("acknowledge: %d %s", status, body)
	}
	status, body = do(t, srv.Client(), http.MethodPost, path+"/acknowledge", member, nil)
	if status != http.StatusNoContent {
		t.Fatalf("duplicate acknowledgement: %d %s", status, body)
	}
	status, body = do(t, srv.Client(), http.MethodGet, url, member, nil)
	if status != http.StatusOK {
		t.Fatalf("list: %d %s", status, body)
	}
	var listed struct {
		Data []struct {
			ID           int64 `json:"id"`
			Acknowledged bool  `json:"acknowledged"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &listed); err != nil {
		t.Fatal(err)
	}
	found := false
	for _, news := range listed.Data {
		if news.ID == id {
			found = true
			if !news.Acknowledged {
				t.Fatal("acknowledgement missing")
			}
		}
	}
	if !found {
		t.Fatal("published news missing")
	}

	payload["published"] = false
	status, body = do(t, srv.Client(), http.MethodPut, path, staff, payload)
	if status != http.StatusOK {
		t.Fatalf("unpublish: %d %s", status, body)
	}
	status, body = do(t, srv.Client(), http.MethodPost, path+"/acknowledge", member, nil)
	if status != http.StatusNotFound {
		t.Fatalf("unpublished news acknowledged: %d %s", status, body)
	}
}
