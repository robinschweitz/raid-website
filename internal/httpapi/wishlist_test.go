package httpapi_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"testing"
	"time"

	"raid-website/internal/character"
	"raid-website/internal/enhancepoints"
	"raid-website/internal/validation"
)

// An owner must be able to read the wishlist of their own character even when it is
// empty; other non-staff players must not, and staff may read any wishlist.
func TestWishlistReadIsDecidedByCharacterOwnership(t *testing.T) {
	srv, db := setupServer(t)
	ownerID := seedAccount(t, db, "wish-owner", "password", validation.RoleMember)
	seedAccount(t, db, "wish-other", "password", validation.RoleMember)
	seedAccount(t, db, "wish-assistant", "password", validation.RoleRaidassistent)
	now := time.Now().UTC()
	c := character.Character{PlayerID: ownerID, Name: "Wunschling", Class: "Magier", MainSpec: "Feuer", Active: true, CreatedAt: now, UpdatedAt: now}
	if err := character.NewRepo(db).Create(context.Background(), &c); err != nil {
		t.Fatal(err)
	}
	url := fmt.Sprintf("%s/api/v1/characters/%d/wishlist", srv.URL, c.ID)
	missing := fmt.Sprintf("%s/api/v1/characters/%d/wishlist", srv.URL, c.ID+1000)
	for _, tc := range []struct {
		user   string
		url    string
		status int
	}{
		{"wish-owner", url, http.StatusOK},
		{"wish-other", url, http.StatusForbidden},
		{"wish-assistant", url, http.StatusOK},
		{"wish-owner", missing, http.StatusNotFound},
	} {
		token := login(t, srv.Client(), srv, tc.user, "password")
		if status, body := do(t, srv.Client(), http.MethodGet, tc.url, token, nil); status != tc.status {
			t.Fatalf("%s %s: status %d, want %d: %s", tc.user, tc.url, status, tc.status, body)
		}
	}
}

// Wishes carry the roll the owner wants (main or second spec, enhanced only for
// main spec). Only the owner changes them; the addon export is readable like
// the list and carries the owner's EP balance.
func TestWishlistRollChoiceAndAddonExport(t *testing.T) {
	srv, db := setupServer(t)
	ownerID := seedAccount(t, db, "roll-owner", "password", validation.RoleMember)
	seedAccount(t, db, "roll-other", "password", validation.RoleMember)
	adminID := seedAccount(t, db, "roll-assistant", "password", validation.RoleRaidassistent)
	ctx := context.Background()
	now := time.Now().UTC()
	c := character.Character{PlayerID: ownerID, Name: "Wurfling", Class: "Magier", MainSpec: "Feuer", Active: true, CreatedAt: now, UpdatedAt: now}
	if err := character.NewRepo(db).Create(ctx, &c); err != nil {
		t.Fatal(err)
	}
	if err := enhancepoints.NewRepo(db).InsertTransaction(ctx, &enhancepoints.Transaction{PlayerID: ownerID, Amount: 70, Type: "adjustment", Reason: "test balance", ActorID: adminID, CreatedAt: now}); err != nil {
		t.Fatal(err)
	}
	owner := login(t, srv.Client(), srv, "roll-owner", "password")
	other := login(t, srv.Client(), srv, "roll-other", "password")
	assistant := login(t, srv.Client(), srv, "roll-assistant", "password")
	base := fmt.Sprintf("%s/api/v1/characters/%d/wishlist", srv.URL, c.ID)

	status, body := do(t, srv.Client(), http.MethodPost, base+"/items", owner, map[string]any{"item_id": "50415", "item_name": "Bryntroll", "priority": 5, "notes": "BiS; zuerst", "is_enhanced": true})
	var added struct{ Data map[string]any }
	if err := json.Unmarshal([]byte(body), &added); status != http.StatusCreated || err != nil || added.Data["spec"] != "MAIN" || added.Data["is_enhanced"] != true || added.Data["item_id"] != "50415" {
		t.Fatalf("add with default spec: %d %s", status, body)
	}
	if status, body := do(t, srv.Client(), http.MethodPost, base+"/items", owner, map[string]any{"item_id": "50274", "item_name": "Splitter", "spec": "SECOND", "is_enhanced": true}); status != http.StatusBadRequest {
		t.Fatalf("enhanced second spec must be rejected: %d %s", status, body)
	}
	if status, body := do(t, srv.Client(), http.MethodPost, base+"/items", owner, map[string]any{"item_id": "50274", "item_name": "Splitter", "priority": 1, "spec": "SECOND"}); status != http.StatusCreated {
		t.Fatalf("add second spec: %d %s", status, body)
	}

	update := map[string]any{"priority": 1, "notes": "", "spec": "SECOND", "is_enhanced": false}
	for _, tc := range []struct {
		token  string
		item   string
		body   map[string]any
		status int
	}{
		{other, "50415", update, http.StatusForbidden},
		{assistant, "50415", update, http.StatusForbidden},
		{owner, "1", update, http.StatusNotFound},
		{owner, "50415", map[string]any{"priority": 1, "notes": "", "spec": "OFF", "is_enhanced": false}, http.StatusBadRequest},
		{owner, "50415", map[string]any{"priority": 1, "spec": "MAIN", "is_enhanced": false, "points": 10}, http.StatusBadRequest},
		{owner, "50274", map[string]any{"priority": 1, "notes": "nur | wenn frei", "spec": "SECOND", "is_enhanced": false}, http.StatusOK},
	} {
		if status, body := do(t, srv.Client(), http.MethodPut, base+"/items/"+tc.item, tc.token, tc.body); status != tc.status {
			t.Fatalf("update %s %v: %d, want %d: %s", tc.item, tc.body, status, tc.status, body)
		}
	}

	pattern := regexp.MustCompile(`^WL2;Wurfling;\d+;70\n50415;ms;1;1;BiS, zuerst\n50274;os;0;2;nur / wenn frei$`)
	for _, tc := range []struct {
		token  string
		status int
	}{{owner, http.StatusOK}, {assistant, http.StatusOK}, {other, http.StatusForbidden}} {
		status, body := do(t, srv.Client(), http.MethodGet, base+"/addon-export", tc.token, nil)
		if status != tc.status {
			t.Fatalf("export: %d, want %d: %s", status, tc.status, body)
		}
		if status != http.StatusOK {
			continue
		}
		var out struct{ Data struct{ Text string } }
		if err := json.Unmarshal([]byte(body), &out); err != nil || !pattern.MatchString(out.Data.Text) {
			t.Fatalf("export text:\n%s", out.Data.Text)
		}
	}
}
