package httpapi_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"

	"raid-website/internal/character"
	"raid-website/internal/shadowmourne"
	"raid-website/internal/validation"
)

func TestShadowmourneQueueKeepsSerialOrderAndHistory(t *testing.T) {
	srv, database := setupServer(t)
	client := srv.Client()
	ctx := context.Background()
	member := seedAccount(t, database, "member", "pw", validation.RoleMember)
	seedAccount(t, database, "assistant", "pw", validation.RoleRaidassistent)
	memberToken := login(t, client, srv, "member", "pw")
	staffToken := login(t, client, srv, "assistant", "pw")
	now := time.Now().UTC()
	characters := make([]*character.Character, 4)
	for i := range characters {
		characters[i] = &character.Character{PlayerID: member, Name: fmt.Sprintf("Sammler %d", i+1), Class: "Krieger", MainSpec: "Waffen", SecondSpec: "Furor", Active: true, CreatedAt: now, UpdatedAt: now}
		if err := character.NewRepo(database).Create(ctx, characters[i]); err != nil {
			t.Fatal(err)
		}
	}
	url := srv.URL + "/api/v1/shadowmourne-queue"
	request := func(method, suffix, token string, body any, want int) []byte {
		t.Helper()
		status, response := do(t, client, method, url+suffix, token, body)
		if status != want {
			t.Fatalf("%s %s: status %d, want %d: %s", method, suffix, status, want, response)
		}
		return response
	}
	request(http.MethodGet, "", "", nil, http.StatusUnauthorized)
	request(http.MethodGet, "", memberToken, nil, http.StatusOK)
	request(http.MethodPost, "", memberToken, map[string]any{"character_id": characters[0].ID, "shards_collected": 0, "shard_target": 2}, http.StatusForbidden)

	create := func(characterID int64) shadowmourne.Entry {
		body := request(http.MethodPost, "", staffToken, map[string]any{"character_id": characterID, "shards_collected": 0, "shard_target": 2}, http.StatusCreated)
		var response struct {
			Data shadowmourne.Entry `json:"data"`
		}
		if err := json.Unmarshal(body, &response); err != nil {
			t.Fatal(err)
		}
		return response.Data
	}
	first, second := create(characters[0].ID), create(characters[1].ID)
	if first.Status != shadowmourne.StatusCollecting || second.Status != shadowmourne.StatusQueued {
		t.Fatalf("first two entries: %#v %#v", first, second)
	}
	request(http.MethodPatch, fmt.Sprintf("/%d", first.ID), staffToken, map[string]any{"shards_collected": 1, "shard_target": 2}, http.StatusOK)
	request(http.MethodPost, fmt.Sprintf("/%d/complete", first.ID), staffToken, map[string]any{}, http.StatusConflict)
	request(http.MethodPatch, fmt.Sprintf("/%d", first.ID), staffToken, map[string]any{"shards_collected": 2, "shard_target": 2}, http.StatusOK)
	request(http.MethodPost, fmt.Sprintf("/%d/complete", first.ID), staffToken, map[string]any{}, http.StatusOK)
	third, fourth := create(characters[2].ID), create(characters[3].ID)
	request(http.MethodPost, fmt.Sprintf("/%d/move", fourth.ID), staffToken, map[string]any{"direction": "UP"}, http.StatusOK)

	var list struct {
		Data []shadowmourne.Entry `json:"data"`
	}
	if err := json.Unmarshal(request(http.MethodGet, "", memberToken, nil, http.StatusOK), &list); err != nil {
		t.Fatal(err)
	}
	if len(list.Data) != 4 || list.Data[0].ID != second.ID || list.Data[0].Status != shadowmourne.StatusCollecting || list.Data[1].ID != fourth.ID || list.Data[2].ID != third.ID || list.Data[3].ID != first.ID || list.Data[3].Status != shadowmourne.StatusCompleted || list.Data[3].CompletedAt == nil {
		t.Fatalf("unexpected queue order: %#v", list.Data)
	}
	var audits int
	if err := database.NewSelect().TableExpr("audit_log").ColumnExpr("COUNT(*)").Where("entity_type = ?", "shadowmourne_queue_entry").Scan(ctx, &audits); err != nil || audits != 8 {
		t.Fatalf("queue audit entries: %d %v", audits, err)
	}
}
