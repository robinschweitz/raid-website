package httpapi_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"

	"raid-website/internal/loot"
	"raid-website/internal/raid"
	"raid-website/internal/validation"
)

func TestLootListsNameTheCharacterWhenNoNameIsStored(t *testing.T) {
	f := newCharacterCorrectionFixture(t)
	defer f.srv.Close()
	ctx := context.Background()
	now := time.Now().UTC()

	rd := &raid.Raid{Name: "ICC25-01", RaidLeaderID: f.actorID, Status: validation.RaidStatusCompleted, CreatedAt: now, UpdatedAt: now}
	if err := raid.NewRepo(f.db).Create(ctx, rd); err != nil {
		t.Fatal(err)
	}
	newAward := func(characterID *int64, storedName string) *loot.Award {
		return &loot.Award{RaidID: rd.ID, Boss: "Boss", ItemID: "1", ItemName: "Item", AwardedToPlayerID: f.actorID, AwardedToCharacterID: characterID, AwardedToCharacterName: storedName, AwardedByPlayerID: f.actorID, Source: "manual", AwardMethod: loot.Main, AwardedAt: now, CreatedAt: now, UpdatedAt: now}
	}
	websiteAward := newAward(&f.initial.ID, "")
	historicalAward := newAward(&f.initial.ID, "Alter Name")
	unlinkedAward := newAward(nil, "")
	for _, award := range []*loot.Award{websiteAward, historicalAward, unlinkedAward} {
		if err := loot.NewRepo(f.db).CreateAward(ctx, award); err != nil {
			t.Fatal(err)
		}
	}

	for _, path := range []string{
		fmt.Sprintf("/api/v1/players/%d/loot", f.actorID),
		fmt.Sprintf("/api/v1/raids/%d/loot", rd.ID),
	} {
		status, body := do(t, f.srv.Client(), http.MethodGet, f.srv.URL+path, f.token, nil)
		if status != http.StatusOK {
			t.Fatalf("%s: status %d body %s", path, status, body)
		}
		var out struct {
			Data []struct {
				ID                     int64  `json:"id"`
				AwardedToCharacterName string `json:"awarded_to_character_name"`
			} `json:"data"`
		}
		if err := json.Unmarshal(body, &out); err != nil {
			t.Fatal(err)
		}
		names := map[int64]string{}
		for _, row := range out.Data {
			names[row.ID] = row.AwardedToCharacterName
		}
		if names[websiteAward.ID] != f.initial.Name {
			t.Fatalf("%s: empty name not resolved: %q", path, names[websiteAward.ID])
		}
		if names[historicalAward.ID] != "Alter Name" {
			t.Fatalf("%s: stored name changed: %q", path, names[historicalAward.ID])
		}
		if names[unlinkedAward.ID] != "" {
			t.Fatalf("%s: award without character got a name: %q", path, names[unlinkedAward.ID])
		}
	}

	stored, err := loot.NewRepo(f.db).Get(ctx, websiteAward.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.AwardedToCharacterName != "" {
		t.Fatalf("reading loot changed the stored row: %q", stored.AwardedToCharacterName)
	}
}
