package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"raid-website/internal/auth"
	"raid-website/internal/db"
	v1 "raid-website/internal/httpapi/v1"
	"raid-website/internal/migrations"
	"raid-website/internal/player"
)

func middlewareAuthDB(t *testing.T) (*db.BunDB, *auth.Service) {
	t.Helper()
	bdb, err := db.Open(context.Background(), "file:"+filepath.Join(t.TempDir(), "middleware.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := bdb.Close(); err != nil {
			t.Error(err)
		}
	})
	m, err := migrations.NewMigrator(bdb.DB)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := m.Migrate(context.Background()); err != nil {
		t.Fatal(err)
	}
	return bdb, auth.NewService(player.NewRepo(bdb.DB), auth.NewRepo(bdb.DB))
}

func TestSecurityHeadersAllowWowIconImages(t *testing.T) {
	h := securityHeaders(false)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()

	h.ServeHTTP(w, r)

	csp := w.Header().Get("Content-Security-Policy")
	if !strings.Contains(csp, "img-src 'self' data: https://wow.zamimg.com") {
		t.Fatalf("Content-Security-Policy = %q; want wow icon image host allowed", csp)
	}
}

func TestMiddlewareAuthenticationFailures(t *testing.T) {
	for _, scenario := range []string{"missing", "unknown", "expired", "inactive", "closed database", "touch failure", "valid"} {
		t.Run(scenario, func(t *testing.T) {
			ctx := context.Background()
			bdb, svc := middlewareAuthDB(t)
			token := "unknown-token"
			if scenario != "missing" && scenario != "unknown" && scenario != "closed database" {
				if err := svc.BootstrapAdmin(ctx, "owner", "valid-password"); err != nil {
					t.Fatal(err)
				}
				var err error
				token, _, err = svc.Login(ctx, "owner", "valid-password")
				if err != nil {
					t.Fatal(err)
				}
			}
			var statement string
			switch scenario {
			case "expired":
				if _, err := bdb.DB.ExecContext(ctx, "UPDATE sessions SET expires_at = ?", time.Now().UTC().Add(-time.Minute)); err != nil {
					t.Fatal(err)
				}
			case "inactive":
				statement = "UPDATE players SET active = 0"
			case "touch failure":
				statement = "CREATE TRIGGER reject_touch BEFORE UPDATE ON sessions BEGIN SELECT RAISE(ABORT, 'injected failure'); END"
			case "closed database":
				if err := bdb.Close(); err != nil {
					t.Fatal(err)
				}
			}
			if statement != "" {
				if _, err := bdb.DB.ExecContext(ctx, statement); err != nil {
					t.Fatal(err)
				}
			}
			called := false
			h := authenticate(svc)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				called = true
				_, hasActor := v1.CurrentPlayer(r)
				if hasActor != (scenario == "valid") {
					t.Fatalf("actor present = %v for %s", hasActor, scenario)
				}
				w.WriteHeader(http.StatusNoContent)
			}))
			r := httptest.NewRequest(http.MethodGet, "/", nil)
			if scenario != "missing" {
				r.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
			}
			w := httptest.NewRecorder()
			h.ServeHTTP(w, r)
			isFailure := scenario == "closed database" || scenario == "touch failure"
			if isFailure {
				if called {
					t.Fatal("downstream handler called despite authentication failure")
				}
				assertAuthError(t, w, http.StatusInternalServerError, "internal")
			} else if !called || w.Code != http.StatusNoContent {
				t.Fatalf("expected normal dispatch: called %v, status %d", called, w.Code)
			}
		})
	}
}

func TestLoginHTTPRepositoryFailures(t *testing.T) {
	for _, scenario := range []string{"unknown username", "closed database", "session insert failure"} {
		t.Run(scenario, func(t *testing.T) {
			ctx := context.Background()
			bdb, svc := middlewareAuthDB(t)
			if scenario == "session insert failure" {
				if err := svc.BootstrapAdmin(ctx, "owner", "valid-password"); err != nil {
					t.Fatal(err)
				}
				if _, err := bdb.DB.ExecContext(ctx, "CREATE TRIGGER reject_session AFTER INSERT ON sessions BEGIN SELECT RAISE(ABORT, 'injected failure'); END"); err != nil {
					t.Fatal(err)
				}
			}
			if scenario == "closed database" {
				if err := bdb.Close(); err != nil {
					t.Fatal(err)
				}
			}
			mux := http.NewServeMux()
			v1.NewAPI(v1.Deps{Auth: svc}).Register(mux)
			r := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(`{"username":"owner","password":"valid-password"}`))
			w := httptest.NewRecorder()
			authenticate(svc)(mux).ServeHTTP(w, r)
			status, code := http.StatusInternalServerError, "internal"
			if scenario == "unknown username" {
				status, code = http.StatusUnauthorized, "invalid_credentials"
			}
			assertAuthError(t, w, status, code)
			if scenario != "closed database" {
				var count int
				if err := bdb.DB.QueryRowContext(ctx, "SELECT COUNT(*) FROM sessions").Scan(&count); err != nil || count != 0 {
					t.Fatalf("failed login left %d sessions, error %v", count, err)
				}
			}
		})
	}
}

func TestLogoutHTTPRepositoryFailure(t *testing.T) {
	ctx := context.Background()
	bdb, svc := middlewareAuthDB(t)
	if err := svc.BootstrapAdmin(ctx, "owner", "valid-password"); err != nil {
		t.Fatal(err)
	}
	token, _, err := svc.Login(ctx, "owner", "valid-password")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := bdb.DB.ExecContext(ctx, "CREATE TRIGGER reject_logout BEFORE DELETE ON sessions BEGIN SELECT RAISE(ABORT, 'injected failure'); END"); err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	v1.NewAPI(v1.Deps{Auth: svc}).Register(mux)
	r := httptest.NewRequest(http.MethodPost, "/api/v1/auth/logout", nil)
	r.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
	w := httptest.NewRecorder()
	authenticate(svc)(mux).ServeHTTP(w, r)
	assertAuthError(t, w, http.StatusInternalServerError, "internal")
	var count int
	if err := bdb.DB.QueryRowContext(ctx, "SELECT COUNT(*) FROM sessions").Scan(&count); err != nil || count != 1 {
		t.Fatalf("failed logout: sessions = %d, error %v", count, err)
	}
}

func assertAuthError(t *testing.T, w *httptest.ResponseRecorder, status int, code string) {
	t.Helper()
	var response v1.ErrorResponse
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if w.Code != status || response.Error.Code != code || response.Error.Message == "" {
		t.Fatalf("response = %d %s; want %d %s", w.Code, w.Body.String(), status, code)
	}
	if len(w.Result().Cookies()) != 0 {
		t.Fatal("failure response set a session cookie")
	}
}
