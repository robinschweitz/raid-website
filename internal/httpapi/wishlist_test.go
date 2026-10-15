package httpapi_test

import (
	"context"
	"fmt"
	"net/http"
	"testing"
	"time"

	"raid-website/internal/character"
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
