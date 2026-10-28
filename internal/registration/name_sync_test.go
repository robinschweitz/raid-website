package registration

import (
	"context"
	"testing"

	"raid-website/internal/character"
	"raid-website/internal/player"
	"raid-website/internal/validation"
)

type renameProfileClient struct {
	name string
}
func (c *renameProfileClient) SearchCharacters(context.Context, string) ([]ExternalCharacter, error) {
	return nil, nil
}
func (c *renameProfileClient) LoadCharacter(_ context.Context, id string) (ExternalCharacter, string, error) {
	return ExternalCharacter{ProfileID: id, Name: c.name, Class: "Magier", MainSpec: "Feuer", Level: 80, Faction: "Alliance"}, "", nil
}

func TestAutomaticRisingGodsRenamePreservesIdentityAndHistory(t *testing.T) {
	_, svc := profileStorageService(t)
	ctx := context.Background()
	client := &renameProfileClient{name: "Neuername"}
	svc.external = client
	owner := &player.Player{Name: "Owner", Role: validation.RoleAdmin, Active: true}
	if err := svc.players.Create(ctx, owner); err != nil { t.Fatal(err) }
	c := &character.Character{PlayerID: owner.ID, Name: "Altername", Class: "Magier", MainSpec: "Frost", Active: true, ExternalSource: ExternalSourceRisingGods, ExternalProfileID: "319098320"}
	if err := svc.characters.Create(ctx, c); err != nil { t.Fatal(err) }
	renamed, err := svc.SyncLinkedCharacterNames(ctx)
	if err != nil { t.Fatal(err) }
	if renamed != 1 { t.Fatalf("renamed = %d, want 1", renamed) }
	got, err := svc.characters.Get(ctx, c.ID)
	if err != nil { t.Fatal(err) }
	if got.ID != c.ID || got.PlayerID != owner.ID || got.ExternalProfileID != "319098320" || got.Name != "Neuername" || got.NameChangedAt == nil || got.MainSpec != "Frost" {
		t.Fatalf("synced character = %+v", got)
	}
	history, err := svc.characters.ListNameHistory(ctx, c.ID)
	if err != nil { t.Fatal(err) }
	if len(history) != 1 || history[0].OldName != "Altername" || history[0].NewName != "Neuername" {
		t.Fatalf("history = %+v", history)
	}
	renamed, err = svc.SyncLinkedCharacterNames(ctx)
	if err != nil || renamed != 0 { t.Fatalf("second sync: renamed=%d error=%v", renamed, err) }
	history, err = svc.characters.ListNameHistory(ctx, c.ID)
	if err != nil || len(history) != 1 { t.Fatalf("duplicate history: %+v, %v", history, err) }
}

func TestReloadLinkedCharacterUpdatesNameHistory(t *testing.T) {
	_, svc := profileStorageService(t)
	ctx := context.Background()
	svc.external = &renameProfileClient{name: "Neuername"}
	admin := &player.Player{Name: "Admin", Role: validation.RoleAdmin, Active: true}
	if err := svc.players.Create(ctx, admin); err != nil { t.Fatal(err) }
	c := &character.Character{PlayerID: admin.ID, Name: "Altername", Class: "Magier", MainSpec: "Frost", Active: true, ExternalSource: ExternalSourceRisingGods, ExternalProfileID: "319098320"}
	if err := svc.characters.Create(ctx, c); err != nil { t.Fatal(err) }
	got, err := svc.ReloadLinkedCharacter(ctx, admin.ID, c.ID)
	if err != nil { t.Fatal(err) }
	if got.Name != "Neuername" || got.NameChangedAt == nil { t.Fatalf("reload = %+v", got) }
	history, err := svc.characters.ListNameHistory(ctx, c.ID)
	if err != nil || len(history) != 1 || history[0].OldName != "Altername" || history[0].NewName != "Neuername" {
		t.Fatalf("history = %+v, error=%v", history, err)
	}
}
