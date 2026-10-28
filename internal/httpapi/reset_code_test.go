package httpapi_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"raid-website/internal/registration"
	"raid-website/internal/validation"
)

func TestReqAccount017(t *testing.T) {
	srv, db := setupServer(t)
	admin := seedAccount(t, db, "reset-admin", "reset-admin-password", validation.RoleAdmin)
	target := seedAccount(t, db, "reset-target", "reset-target-password", validation.RoleMember)
	seedAccount(t, db, "reset-leader", "reset-leader-password", validation.RoleRaidleader)
	token := login(t, srv.Client(), srv, "reset-admin", "reset-admin-password")
	leader := login(t, srv.Client(), srv, "reset-leader", "reset-leader-password")
	url := fmt.Sprintf("%s/api/v1/players/%d/reset-code", srv.URL, target)
	status, _ := do(t, srv.Client(), http.MethodPost, url, leader, nil)
	if status != 403 {
		t.Fatalf("AC03 leader=%d", status)
	}
	var first registration.GeneratedResetCode
	for i := 0; i < 2; i++ {
		status, body := do(t, srv.Client(), http.MethodPost, url, token, nil)
		if status != 201 {
			t.Fatalf("create=%d %s", status, body)
		}
		var result struct {
			Data registration.GeneratedResetCode `json:"data"`
		}
		if err := json.Unmarshal(body, &result); err != nil {
			t.Fatal(err)
		}
		if result.Data.Code == "" || result.Data.Record.ExpiresAt.Sub(result.Data.Record.CreatedAt) != 72*time.Hour || strings.Contains(string(body), "code_hash") {
			t.Fatalf("AC01 invalid metadata=%s", body)
		}
		var stored registration.ResetCode
		if err := db.NewSelect().Model(&stored).Where("id = ?", result.Data.Record.ID).Scan(context.Background()); err != nil {
			t.Fatal(err)
		}
		if len(stored.CodeHash) != 64 || strings.Contains(stored.CodeHash, result.Data.Code) {
			t.Fatal("plaintext stored")
		}
		if i == 0 {
			first = result.Data
		}
	}
	var old registration.ResetCode
	if err := db.NewSelect().Model(&old).Where("id = ?", first.Record.ID).Scan(context.Background()); err != nil {
		t.Fatal(err)
	}
	if old.RevokedAt == nil {
		t.Fatal("AC02 old code remains usable")
	}
	status, body := do(t, srv.Client(), http.MethodPut, fmt.Sprintf("%s/api/v1/players/%d/account", srv.URL, target), token, map[string]any{"username": "reset-target", "password": "replacement-password", "role": "MEMBER"})
	if status != 403 || !strings.Contains(string(body), "password_reset_required") {
		t.Fatalf("AC04 direct reset=%d %s", status, body)
	}
	var logs string
	if err := db.NewSelect().TableExpr("audit_log").ColumnExpr("group_concat(after_json)").Where("action='reset_code_created' AND actor_id = ?", admin).Scan(context.Background(), &logs); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(logs, first.Code) || strings.Contains(logs, "code_hash") {
		t.Fatal("secret in audit")
	}
}
