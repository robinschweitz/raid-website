package httpapi_test

import (
	"context"
	"fmt"
	"errors"
	"testing"

	"raid-website/internal/audit"
		"raid-website/internal/character"
	"raid-website/internal/player"
	"raid-website/internal/registration"
)

type retryExternalClient struct {
	matches []registration.ExternalCharacter
}

func (client retryExternalClient) SearchCharacters(_ context.Context, _ string) ([]registration.ExternalCharacter, error) {
	return client.matches, nil
}

func (client retryExternalClient) LoadCharacter(_ context.Context, profileID string) (registration.ExternalCharacter, string, error) {
	for _, match := range client.matches {
		if match.ProfileID == profileID {
			return match, "{}", nil
		}
	}
	return registration.ExternalCharacter{}, "", registration.ErrExternalNotFound
}

func TestRetryCharacterLinkPreservesCharacterAndAudits(t *testing.T) {
	f := newCharacterCorrectionFixture(t)
	defer f.srv.Close()
	service := registration.NewService(f.db, player.NewRepo(f.db), character.NewRepo(f.db), audit.NewRepo(f.db), nil, retryExternalClient{
		matches: []registration.ExternalCharacter{{ProfileID: "123", Name: "Original", Class: "Magier", MainSpec: "Feuer", ProfileURL: "https://db.rising-gods.de/?profile=123"}},
	})
	linked, err := service.RetryCharacterLink(context.Background(), f.actorID, f.initial.ID)
	if err != nil {
		t.Fatal(err)
	}
	if linked.ID != f.initial.ID || linked.PlayerID != f.initial.PlayerID || linked.ExternalProfileID != "123" {
		t.Fatalf("incorrect linked character: %+v", linked)
	}
	logs, err := audit.NewRepo(f.db).ListByEntity(context.Background(), "character", fmt.Sprint(f.initial.ID))
	if err != nil {
		t.Fatal(err)
	}
	if len(logs) == 0 {
		t.Fatal("expected audit record")
	}
	if _, err := service.RetryCharacterLink(context.Background(), f.actorID, f.initial.ID); !errors.Is(err, registration.ErrProfileLinked) {
		t.Fatalf("expected linked error, got %v", err)
	}
}

func TestRetryCharacterLinkDoesNotGuessAmbiguousProfile(t *testing.T) {
	f := newCharacterCorrectionFixture(t)
	defer f.srv.Close()
	service := registration.NewService(f.db, player.NewRepo(f.db), character.NewRepo(f.db), audit.NewRepo(f.db), nil, retryExternalClient{
		matches: []registration.ExternalCharacter{
			{ProfileID: "123", Name: "Original", MainSpec: "Feuer"},
			{ProfileID: "456", Name: "Original", MainSpec: "Feuer"},
		},
	})
	if _, err := service.RetryCharacterLink(context.Background(), f.actorID, f.initial.ID); !errors.Is(err, registration.ErrProfileAmbiguous) {
		t.Fatalf("expected ambiguous error, got %v", err)
	}
	stored, err := character.NewRepo(f.db).Get(context.Background(), f.initial.ID)
	if err != nil || stored.ExternalProfileID != "" {
		t.Fatalf("unexpected profile link: %+v, %v", stored, err)
	}
}
