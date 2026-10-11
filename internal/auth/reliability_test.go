package auth

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"golang.org/x/crypto/bcrypt"

	"raid-website/internal/db"
	"raid-website/internal/migrations"
	"raid-website/internal/player"
	"raid-website/internal/validation"
)

func reliabilityDB(t *testing.T) *db.BunDB {
	t.Helper()
	bdb, err := db.Open(context.Background(), "file:"+filepath.Join(t.TempDir(), "auth.db"))
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
	return bdb
}

func TestLoginRepositoryFailures(t *testing.T) {
	for _, scenario := range []string{"unknown username", "closed database", "session insert failure"} {
		t.Run(scenario, func(t *testing.T) {
			ctx := context.Background()
			bdb := reliabilityDB(t)
			svc := NewService(player.NewRepo(bdb.DB), NewRepo(bdb.DB))
			username := "unknown"
			if scenario == "session insert failure" {
				username = "owner"
				if err := svc.BootstrapAdmin(ctx, username, "valid-password"); err != nil {
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
			token, p, err := svc.Login(ctx, username, "valid-password")
			if err == nil || token != "" || p != nil {
				t.Fatalf("Login = %q, %v, %v; want no session or identity", token, p, err)
			}
			if errors.Is(err, ErrInvalidCredentials) != (scenario == "unknown username") {
				t.Fatalf("wrong error classification: %v", err)
			}
			if scenario == "session insert failure" {
				var count int
				if err := bdb.DB.QueryRowContext(ctx, "SELECT COUNT(*) FROM sessions").Scan(&count); err != nil || count != 0 {
					t.Fatalf("sessions = %d, error %v", count, err)
				}
			}
		})
	}
}

func TestAuthenticateExpectedMisses(t *testing.T) {
	ctx := context.Background()
	bdb := reliabilityDB(t)
	svc := NewService(player.NewRepo(bdb.DB), NewRepo(bdb.DB))
	for _, token := range []string{"", "unknown", "invalid-token"} {
		if p, err := svc.Authenticate(ctx, token); p != nil || err != nil {
			t.Fatalf("Authenticate(%q) = %v, %v", token, p, err)
		}
	}
	if err := svc.BootstrapAdmin(ctx, "owner", "valid-password"); err != nil {
		t.Fatal(err)
	}
	token, p, err := svc.Login(ctx, "owner", "valid-password")
	if err != nil {
		t.Fatal(err)
	}
	if authenticated, err := svc.Authenticate(ctx, token); err != nil || authenticated == nil || authenticated.ID != p.ID {
		t.Fatalf("valid session = %v, %v", authenticated, err)
	}
	// A missing referenced player is an authentication miss, not an operational error.
	emptyDB := reliabilityDB(t)
	svc.players = player.NewRepo(emptyDB.DB)
	if p, err := svc.Authenticate(ctx, token); p != nil || err != nil {
		t.Fatalf("missing session player = %v, %v", p, err)
	}
	svc.players = player.NewRepo(bdb.DB)
	if _, err := bdb.DB.ExecContext(ctx, "UPDATE sessions SET expires_at = ?", time.Now().UTC().Add(-time.Minute)); err != nil {
		t.Fatal(err)
	}
	if p, err := svc.Authenticate(ctx, token); p != nil || err != nil {
		t.Fatalf("expired session = %v, %v", p, err)
	}
}

func TestAuthenticateOperationalFailures(t *testing.T) {
	for _, scenario := range []string{"session lookup", "player lookup", "session touch"} {
		t.Run(scenario, func(t *testing.T) {
			ctx := context.Background()
			bdb := reliabilityDB(t)
			svc := NewService(player.NewRepo(bdb.DB), NewRepo(bdb.DB))
			if err := svc.BootstrapAdmin(ctx, "owner", "valid-password"); err != nil {
				t.Fatal(err)
			}
			token, _, err := svc.Login(ctx, "owner", "valid-password")
			if err != nil {
				t.Fatal(err)
			}
			switch scenario {
			case "session lookup":
				if err := bdb.Close(); err != nil {
					t.Fatal(err)
				}
			case "player lookup":
				closedDB := reliabilityDB(t)
				if err := closedDB.Close(); err != nil {
					t.Fatal(err)
				}
				svc.players = player.NewRepo(closedDB.DB)
			case "session touch":
				if _, err := bdb.DB.ExecContext(ctx, "CREATE TRIGGER reject_touch BEFORE UPDATE ON sessions BEGIN SELECT RAISE(ABORT, 'injected failure'); END"); err != nil {
					t.Fatal(err)
				}
			}
			p, err := svc.Authenticate(ctx, token)
			if p != nil || err == nil || errors.Is(err, sql.ErrNoRows) {
				t.Fatalf("Authenticate = %v, %v; want operational error without identity", p, err)
			}
			if errors.Unwrap(err) == nil || (scenario == "session touch" && !strings.Contains(err.Error(), "injected failure")) {
				t.Fatalf("repository error not preserved: %v", err)
			}
		})
	}
}

func TestConcurrentCredentialsPreservePassword(t *testing.T) {
	ctx := context.Background()
	bdb := reliabilityDB(t)
	players := player.NewRepo(bdb.DB)
	svc := NewService(players, NewRepo(bdb.DB))
	if err := svc.BootstrapAdmin(ctx, "owner", "initial-password"); err != nil {
		t.Fatal(err)
	}
	before, err := players.FindByUsername(ctx, "owner")
	if err != nil {
		t.Fatal(err)
	}
	start := make(chan struct{})
	results := make(chan error, 2)
	for _, password := range []string{"replacement-password", ""} {
		go func(password string) {
			<-start
			_, err := svc.SetCredentials(ctx, before.ID, "owner", password, validation.RoleMember)
			results <- err
		}(password)
	}
	close(start)
	for range 2 {
		if err := <-results; err != nil {
			t.Error(err)
		}
	}
	after, err := players.Get(ctx, before.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := bcrypt.CompareHashAndPassword([]byte(after.PasswordHash), []byte("replacement-password")); err != nil {
		t.Fatalf("blank-password update lost the replacement hash: %v", err)
	}
	if after.Name != before.Name || after.Active != before.Active || !after.CreatedAt.Equal(before.CreatedAt) || after.Role != validation.RoleMember {
		t.Fatal("credential update changed unrelated player fields or lost its role")
	}
}

func TestSetCredentialsRollback(t *testing.T) {
	ctx := context.Background()
	bdb := reliabilityDB(t)
	players := player.NewRepo(bdb.DB)
	svc := NewService(players, NewRepo(bdb.DB))
	if err := svc.BootstrapAdmin(ctx, "owner", "initial-password"); err != nil {
		t.Fatal(err)
	}
	before, err := players.FindByUsername(ctx, "owner")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := bdb.DB.ExecContext(ctx, "CREATE TRIGGER reject_credentials AFTER UPDATE ON players BEGIN SELECT RAISE(ABORT, 'injected failure'); END"); err != nil {
		t.Fatal(err)
	}
	if p, err := svc.SetCredentials(ctx, before.ID, "renamed", "replacement-password", validation.RoleMember); err == nil || p != nil {
		t.Fatalf("SetCredentials = %v, %v; want failure without player", p, err)
	}
	after, err := players.Get(ctx, before.ID)
	if err != nil {
		t.Fatal(err)
	}
	if *after.Username != *before.Username || after.PasswordHash != before.PasswordHash || after.Role != before.Role || !after.UpdatedAt.Equal(before.UpdatedAt) {
		t.Fatal("failed credential update left partial changes")
	}
}

func TestMFAStatusAndFactorRemoval(t *testing.T) {
	ctx := context.Background()
	bdb := reliabilityDB(t)
	players := player.NewRepo(bdb.DB)
	repo := NewRepo(bdb.DB)
	svc := NewService(players, repo)
	if err := svc.BootstrapAdmin(ctx, "owner", "initial-password"); err != nil {
		t.Fatal(err)
	}
	owner, err := players.FindByUsername(ctx, "owner")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	if err := repo.UpsertMFASettings(ctx, &MFASettings{PlayerID: owner.ID, TOTPSecret: "secret", TOTPEnabledAt: &now, CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatal(err)
	}
	if err := repo.CreateRecoveryCode(ctx, &MFARecoveryCode{PlayerID: owner.ID, CodeHash: hashToken("recovery"), CreatedAt: now}); err != nil {
		t.Fatal(err)
	}
	if err := repo.CreateWebAuthnCredential(ctx, &WebAuthnCredential{PlayerID: owner.ID, CredentialIDHash: "credential-hash", Name: "YubiKey", CredentialJSON: "{}", CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatal(err)
	}

	status, err := svc.MFAStatus(ctx, owner.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !status.TOTPEnabled || len(status.SecurityKeys) != 1 || status.SecurityKeys[0].Name != "YubiKey" {
		t.Fatalf("unexpected MFA status: %+v", status)
	}

	if err := svc.DisableTOTP(ctx, owner.ID, "wrong-password"); !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("DisableTOTP with wrong password = %v, want invalid credentials", err)
	}
	if err := svc.DisableTOTP(ctx, owner.ID, "initial-password"); err != nil {
		t.Fatal(err)
	}
	status, err = svc.MFAStatus(ctx, owner.ID)
	if err != nil {
		t.Fatal(err)
	}
	if status.TOTPEnabled || len(status.SecurityKeys) != 1 {
		t.Fatalf("unexpected MFA status after disabling TOTP: %+v", status)
	}
	if _, err := repo.FindUnusedRecoveryCode(ctx, owner.ID, hashToken("recovery")); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("recovery code after disabling TOTP = %v, want removed", err)
	}

	if err := svc.DeleteWebAuthnCredential(ctx, owner.ID, status.SecurityKeys[0].ID, "wrong-password"); !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("DeleteWebAuthnCredential with wrong password = %v, want invalid credentials", err)
	}
	if err := svc.DeleteWebAuthnCredential(ctx, owner.ID, status.SecurityKeys[0].ID, "initial-password"); err != nil {
		t.Fatal(err)
	}
	status, err = svc.MFAStatus(ctx, owner.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(status.SecurityKeys) != 0 {
		t.Fatalf("security keys after deletion: %+v", status.SecurityKeys)
	}
}

func TestStartLoginReportsAvailableMFAFactors(t *testing.T) {
	ctx := context.Background()
	bdb := reliabilityDB(t)
	svc := NewService(player.NewRepo(bdb.DB), NewRepo(bdb.DB))
	repo := NewRepo(bdb.DB)
	if err := svc.BootstrapAdmin(ctx, "owner", "valid-password"); err != nil {
		t.Fatal(err)
	}
	owner, err := player.NewRepo(bdb.DB).FindByUsername(ctx, "owner")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()

	if err := repo.CreateWebAuthnCredential(ctx, &WebAuthnCredential{PlayerID: owner.ID, CredentialIDHash: "credential-hash", Name: "YubiKey", CredentialJSON: "{}", CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatal(err)
	}
	result, err := svc.StartLogin(ctx, "owner", "valid-password", "")
	if err != nil {
		t.Fatal(err)
	}
	if result.MFAChallenge == "" || !result.MFAFactors.WebAuthn || result.MFAFactors.TOTPEnabled || result.MFAFactors.RecoveryCodes {
		t.Fatalf("webauthn-only factors = challenge %q, %+v", result.MFAChallenge, result.MFAFactors)
	}

	if err := repo.UpsertMFASettings(ctx, &MFASettings{PlayerID: owner.ID, TOTPSecret: "secret", TOTPEnabledAt: &now, CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatal(err)
	}
	if err := repo.CreateRecoveryCode(ctx, &MFARecoveryCode{PlayerID: owner.ID, CodeHash: hashToken("recovery"), CreatedAt: now}); err != nil {
		t.Fatal(err)
	}
	result, err = svc.StartLogin(ctx, "owner", "valid-password", "")
	if err != nil {
		t.Fatal(err)
	}
	if result.MFAChallenge == "" || !result.MFAFactors.WebAuthn || !result.MFAFactors.TOTPEnabled || !result.MFAFactors.RecoveryCodes {
		t.Fatalf("combined factors = challenge %q, %+v", result.MFAChallenge, result.MFAFactors)
	}
}

func TestDeleteWebAuthnCredentialRejectsOtherOwner(t *testing.T) {
	ctx := context.Background()
	bdb := reliabilityDB(t)
	players := player.NewRepo(bdb.DB)
	repo := NewRepo(bdb.DB)
	svc := NewService(players, repo)
	if err := svc.BootstrapAdmin(ctx, "owner", "initial-password"); err != nil {
		t.Fatal(err)
	}
	owner, err := players.FindByUsername(ctx, "owner")
	if err != nil {
		t.Fatal(err)
	}
	otherPassword, err := HashPassword("other-password")
	if err != nil {
		t.Fatal(err)
	}
	otherUsername := "other"
	now := time.Now().UTC()
	other := &player.Player{Name: "Other", Username: &otherUsername, PasswordHash: otherPassword, Role: validation.RoleMember, Active: true, CreatedAt: now, UpdatedAt: now}
	if err := players.Create(ctx, other); err != nil {
		t.Fatal(err)
	}
	credential := &WebAuthnCredential{PlayerID: other.ID, CredentialIDHash: "other-credential", Name: "Other key", CredentialJSON: "{}", CreatedAt: now, UpdatedAt: now}
	if err := repo.CreateWebAuthnCredential(ctx, credential); err != nil {
		t.Fatal(err)
	}

	if err := svc.DeleteWebAuthnCredential(ctx, owner.ID, credential.ID, "initial-password"); !errors.Is(err, ErrInvalidMFA) {
		t.Fatalf("DeleteWebAuthnCredential for another owner = %v, want invalid MFA", err)
	}
	if _, err := repo.GetWebAuthnCredential(ctx, credential.ID); err != nil {
		t.Fatalf("credential was removed or became unreadable: %v", err)
	}
}
