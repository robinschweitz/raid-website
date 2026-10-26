package httpapi_test

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"

	"raid-website/internal/httpapi"
	"raid-website/internal/registration"
	"raid-website/internal/validation"
)

type visitorFeed struct{ calls atomic.Int32 }

func (f *visitorFeed) SearchCharacters(context.Context, string) ([]registration.ExternalCharacter, error) {
	f.calls.Add(1)
	return []registration.ExternalCharacter{}, nil
}
func (f *visitorFeed) LoadCharacter(context.Context, string) (registration.ExternalCharacter, string, error) {
	f.calls.Add(1)
	return registration.ExternalCharacter{}, "", registration.ErrExternalNotFound
}

func TestReqAccount014_AC03(t *testing.T) {
	feed := &visitorFeed{}
	srv, db := setupServerWithOptions(t, httpapi.Options{RegistrationFeed: feed})
	client := srv.Client()
	// A failed registration still started a lookup, and spends the same budget.
	for i := 0; i < 5; i++ {
		do(t, client, http.MethodPost, srv.URL+"/api/v1/auth/registration-requests", "", map[string]any{"profile_id": fmt.Sprint(i)})
	}
	for i := 0; i < 5; i++ {
		status, _ := do(t, client, http.MethodGet, srv.URL+"/api/v1/external/rising-gods/characters?q=Mage", "", nil)
		if status != 200 {
			t.Fatalf("search %d = %d", i, status)
		}
	}
	status, body := do(t, client, http.MethodGet, srv.URL+"/api/v1/external/rising-gods/characters?q=Mage", "", nil)
	if status != 429 || !strings.Contains(string(body), `"code":"search_limit"`) || feed.calls.Load() != 10 {
		t.Fatalf("limit = %d %s, calls=%d", status, body, feed.calls.Load())
	}
	seedAccount(t, db, "member-014", "member-014-password", validation.RoleMember)
	token := login(t, client, srv, "member-014", "member-014-password")
	for i := 0; i < 12; i++ {
		if status, _ := do(t, client, http.MethodGet, srv.URL+"/api/v1/external/rising-gods/characters?q=Mage", token, nil); status != 200 {
			t.Fatalf("member search=%d", status)
		}
	}
}
