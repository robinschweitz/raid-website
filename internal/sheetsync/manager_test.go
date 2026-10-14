package sheetsync

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func testCredential(t *testing.T) string {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(map[string]string{
		"client_email": "sync-test@example.iam.gserviceaccount.com",
		"private_key":  string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: raw})),
	})
	if err != nil {
		t.Fatal(err)
	}
	return string(encoded)
}

func TestManagerSettingsSurviveRestartWithoutEnv(t *testing.T) {
	ctx := context.Background()
	db, importer, _, actorID := validationDB(t)
	keyPath := filepath.Join(t.TempDir(), "sync.sqlite.sheet-sync.key")
	secret := testCredential(t)
	var factoryCalls int
	factory := func(settings Settings, received string) (*Service, error) {
		if received != secret {
			t.Fatal("wrong decrypted credential")
		}
		factoryCalls++
		return New(db, importer, &validationWriter{}, settings.SheetID, actorID, settings.Reason, time.Minute)
	}
	m, err := NewManager(ctx, db, Settings{}, "", keyPath, factory)
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()
	if m.Configuration().Enabled || m.Configuration().CredentialsConfigured {
		t.Fatal("sync must be disabled until explicitly configured")
	}
	enabled := Settings{Enabled: true, SheetID: AuthorizedCopyID, Reason: "GUI test", Interval: "5m"}
	got, err := m.Save(ctx, SettingsUpdate{Settings: enabled, ServiceAccountJSON: secret})
	if err != nil || !got.CredentialsConfigured || !got.Enabled || factoryCalls != 1 {
		t.Fatalf("save: %+v %v, calls=%d", got, err, factoryCalls)
	}
	if info, err := os.Stat(keyPath); err != nil || info.Mode().Perm() != 0600 {
		t.Fatalf("secret key permissions: %v %v", info, err)
	}
	var stored string
	if err := db.NewSelect().TableExpr("sheet_sync_config").Column("credentials_ciphertext").Where("id = 1").Scan(ctx, &stored); err != nil {
		t.Fatal(err)
	}
	if stored == "" || strings.Contains(stored, "private_key") || strings.Contains(stored, "PRIVATE KEY") {
		t.Fatal("service-account secret must be encrypted at rest")
	}
	withoutReplacement, err := m.Save(ctx, SettingsUpdate{Settings: Settings{Enabled: true, SheetID: AuthorizedCopyID, Reason: "Updated", Interval: "10m"}})
	if err != nil || !withoutReplacement.CredentialsConfigured || factoryCalls != 2 {
		t.Fatal("saving settings must preserve existing credentials", err)
	}
	m.Close()
	restarted, err := NewManager(ctx, db, Settings{}, "", keyPath, factory)
	if err != nil {
		t.Fatal(err)
	}
	defer restarted.Close()
	if !restarted.Configuration().Enabled || !restarted.Configuration().CredentialsConfigured || factoryCalls != 3 {
		t.Fatal("saved GUI configuration not restored after restart")
	}
	status, err := restarted.GetStatus(ctx)
	if err != nil || !status.Enabled || status.Automatic {
		t.Fatalf("unexpected status: %+v %v", status, err)
	}
}

func TestManagerRejectsUnsafeChangesWithoutReplacingRunningSync(t *testing.T) {
	ctx := context.Background()
	db, importer, _, actorID := validationDB(t)
	secret := testCredential(t)
	factory := func(settings Settings, _ string) (*Service, error) {
		return New(db, importer, &validationWriter{}, settings.SheetID, actorID, settings.Reason, time.Minute)
	}
	m, err := NewManager(ctx, db, Settings{}, "", filepath.Join(t.TempDir(), "sync.key"), factory)
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()
	valid := Settings{SheetID: AuthorizedCopyID, Interval: "10m", Reason: "Test", Enabled: true}
	if _, err := m.Save(ctx, SettingsUpdate{Settings: valid, ServiceAccountJSON: secret}); err != nil {
		t.Fatal(err)
	}
	for _, invalid := range []Settings{
		{SheetID: "1N5EgBixB1RNxZagK5d0ie_DMTcYmznpvW5UzuyEWgRU", Enabled: true, Interval: "10m", Reason: "bad"},
		{SheetID: AuthorizedCopyID, Enabled: true, Interval: "10s", Reason: "bad"},
		{SheetID: AuthorizedCopyID, Enabled: true, Interval: "10m"},
		{SheetID: AuthorizedCopyID, Enabled: false, Automatic: true, Interval: "10m"},
	} {
		if _, err := m.Save(ctx, SettingsUpdate{Settings: invalid}); err == nil {
			t.Fatalf("accepted unsafe settings: %+v", invalid)
		}
		if !m.Configuration().Enabled || m.Configuration().Interval != "10m" {
			t.Fatal("invalid settings overwrote active configuration")
		}
	}
	// A missing key after moving only the database may not silently fall
	// back to old env credentials or expose the ciphertext.
	if err := os.Remove(m.keyPath); err != nil {
		t.Fatal(err)
	}
	restarted, err := NewManager(ctx, db, Settings{}, "fallback-env-json", m.keyPath, factory)
	if err != nil {
		t.Fatal(err)
	}
	defer restarted.Close()
	status, err := restarted.GetStatus(ctx)
	if err != nil || status.Enabled || status.LastError == "" {
		t.Fatal("missing encryption key must fail closed", err, status)
	}
}
