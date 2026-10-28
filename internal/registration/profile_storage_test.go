package registration

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"raid-website/internal/audit"
	"raid-website/internal/character"
	"raid-website/internal/db"
	"raid-website/internal/migrations"
	"raid-website/internal/player"
	"raid-website/internal/validation"
)

type profileStorageClient struct{}

func (profileStorageClient) SearchCharacters(context.Context, string) ([]ExternalCharacter, error) {
	return nil, nil
}
func (profileStorageClient) LoadCharacter(context.Context, string) (ExternalCharacter, string, error) {
	score := 6000
	return ExternalCharacter{ProfileID: "123", Name: "Mage", Class: "Magier", MainSpec: "Feuer", SecondSpec: "Frost", GearScore: &score, Level: 80, Race: "Human", Faction: "Alliance", ProfileURL: "https://db.rising-gods.de/?profile=123"}, `{"private":"unneeded"}`, nil
}

func profileStorageService(t *testing.T) (*db.BunDB, *Service) {
	t.Helper()
	ctx := context.Background()
	bdb, err := db.Open(ctx, "file:"+filepath.Join(t.TempDir(), "profiles.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = bdb.Close() })
	m, err := migrations.NewMigrator(bdb.DB)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = m.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	return bdb, NewService(bdb.DB, player.NewRepo(bdb.DB), character.NewRepo(bdb.DB), audit.NewRepo(bdb.DB), nil, profileStorageClient{})
}

func TestReqAccount013_AC01(t *testing.T) {
	_, svc := profileStorageService(t)
	ctx := context.Background()
	req, err := svc.CreateRequest(ctx, nil, "123")
	if err != nil {
		t.Fatal(err)
	}
	saved, err := svc.repo.GetRequest(ctx, req.ID)
	if err != nil {
		t.Fatal(err)
	}
	if saved.RawProfileJSON != "" || saved.Level != 0 || saved.Race != "" || saved.Faction != "" || saved.CharacterName != "Mage" || saved.Class != "Magier" || saved.MainSpec != "Feuer" || saved.SecondSpec != "Frost" || saved.GearScore == nil || *saved.GearScore != 6000 {
		t.Fatalf("stored profile = %+v", saved)
	}
	body, err := json.Marshal(saved)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(body), "raw_profile_json") || strings.Contains(string(body), "unneeded") {
		t.Fatalf("raw profile leaked: %s", body)
	}
}

func TestReqAccount013_AC02(t *testing.T) {
	bdb, svc := profileStorageService(t)
	ctx := context.Background()
	req, err := svc.CreateRequest(ctx, nil, "123")
	if err != nil {
		t.Fatal(err)
	}
	p := &player.Player{Name: "Player", Active: true}
	if err := svc.players.Create(ctx, p); err != nil {
		t.Fatal(err)
	}
	c := &character.Character{PlayerID: p.ID, Name: "Mage", Class: "Magier", MainSpec: "Feuer", Active: true}
	if err := svc.characters.Create(ctx, c); err != nil {
		t.Fatal(err)
	}
	if _, err := bdb.DB.ExecContext(ctx, "UPDATE registration_requests SET raw_profile_json = ?, level = 80, race = 'Human', faction = 'Alliance' WHERE id = ?", `{"private":"old"}`, req.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := bdb.DB.ExecContext(ctx, "UPDATE characters SET external_profile_json = ? WHERE id = ?", `{"private":"old"}`, c.ID); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"083_minimise_profile_storage.up.sql", "083_minimise_profile_storage.down.sql"} {
		sql, err := migrations.MigrationFiles.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := bdb.DB.ExecContext(ctx, string(sql)); err != nil {
			t.Fatal(err)
		}
	}
	saved, err := svc.repo.GetRequest(ctx, req.ID)
	if err != nil {
		t.Fatal(err)
	}
	ch, err := svc.characters.Get(ctx, c.ID)
	if err != nil {
		t.Fatal(err)
	}
	if saved.RawProfileJSON != "" || saved.Level != 0 || saved.Race != "" || saved.Faction != "" || ch.ExternalProfileJSON != "" || ch.Name != "Mage" || saved.CharacterName != "Mage" || saved.GearScore == nil || *saved.GearScore != 6000 {
		t.Fatalf("cleanup lost useful fields or kept raw data: %+v %+v", saved, ch)
	}
}

func TestReqAccount013_AC03(t *testing.T) {
	_, svc := profileStorageService(t)
	ctx := context.Background()
	admin := &player.Player{Name: "Admin", Role: validation.RoleAdmin, Active: true}
	if err := svc.players.Create(ctx, admin); err != nil {
		t.Fatal(err)
	}
	c := &character.Character{PlayerID: admin.ID, Name: "Mage", Class: "Magier", MainSpec: "Frost", Active: true, ExternalSource: ExternalSourceRisingGods, ExternalProfileID: "123"}
	if err := svc.characters.Create(ctx, c); err != nil {
		t.Fatal(err)
	}
	got, err := svc.ReloadLinkedCharacter(ctx, admin.ID, c.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.ExternalProfileJSON != "" || got.Name != c.Name || got.Class != c.Class || got.MainSpec != "Feuer" || got.GearScore == nil || *got.GearScore != 6000 {
		t.Fatalf("reload = %+v", got)
	}
}
