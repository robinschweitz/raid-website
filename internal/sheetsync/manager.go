package sheetsync

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/uptrace/bun"
)

const AuthorizedCopyID = "10aqVmmODH9Q8F6_RiGoVM2CXK3qtwtjz9hcsQN8_NO8"

var ErrDisabled = errors.New("Google-Sheets-Synchronisierung ist nicht konfiguriert oder aktiviert")
var ErrInvalidSettings = errors.New("ungültige Sheet-Sync-Konfiguration")

// Settings contains only public-to-admin settings. Credentials are never sent back to clients.
type Settings struct {
	Enabled               bool   `json:"enabled"`
	Automatic             bool   `json:"automatic"`
	SheetID               string `json:"sheet_id"`
	Reason                string `json:"reason"`
	Interval              string `json:"interval"`
	CredentialsConfigured bool   `json:"credentials_configured"`
}

// SettingsUpdate allows a credential replacement. An empty credential keeps the old one.
type SettingsUpdate struct {
	Settings
	ServiceAccountJSON string `json:"service_account_json"`
}

type persistedSettings struct {
	Settings
	Ciphertext string `json:"-"`
}

type serviceFactory func(Settings, string) (*Service, error)

// Manager serializes configuration changes with preflights and sync runs.
// The original legacy import remains independent of the authenticated sync importer.
type Manager struct {
	mu             sync.Mutex
	db             *bun.DB
	factory        serviceFactory
	keyPath        string
	envCredentials string
	config         persistedSettings
	service        *Service
	configError    string
	cancel         context.CancelFunc
}

// NewManager loads GUI settings if present, otherwise uses the existing env defaults.
// The key file must live in persistent private storage beside the SQLite database.
func NewManager(ctx context.Context, db *bun.DB, defaults Settings, envCredentials, keyPath string, factory func(Settings, string) (*Service, error)) (*Manager, error) {
	if db == nil || factory == nil || keyPath == "" {
		return nil, errors.New("sheet sync manager requires db, factory and key path")
	}
	m := &Manager{db: db, factory: factory, keyPath: keyPath, envCredentials: envCredentials}
	m.config.Settings = defaults
	var raw, secret string
	err := db.NewSelect().TableExpr("sheet_sync_config").ColumnExpr("config_json", "credentials_ciphertext").Where("id = 1").Scan(ctx, &raw, &secret)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("load sheet sync settings: %w", err)
	}
	if err == nil {
		if err := json.Unmarshal([]byte(raw), &m.config.Settings); err != nil {
			return nil, fmt.Errorf("decode sheet sync settings: %w", err)
		}
		m.config.Ciphertext = secret
	}
	if m.config.SheetID == "" {
		m.config.SheetID = AuthorizedCopyID
	}
	if m.config.Interval == "" {
		m.config.Interval = "10m"
	}
	// An invalid saved configuration must not prevent an administrator logging in
	// and repairing it from the GUI. No sync runs while it is invalid.
	if err := m.activateLocked(m.config, ""); err != nil {
		m.configError = err.Error()
	}
	m.startTimerLocked()
	return m, nil
}

func (m *Manager) Configuration() Settings {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := m.config.Settings
	out.CredentialsConfigured = m.config.Ciphertext != "" || strings.TrimSpace(m.envCredentials) != ""
	return out
}

func validateSettings(s Settings) (time.Duration, error) {
	if s.SheetID != AuthorizedCopyID {
		return 0, fmt.Errorf("%w: nur die ausdrücklich freigegebene Sheet-Kopie ist erlaubt", ErrInvalidSettings)
	}
	d, err := time.ParseDuration(s.Interval)
	if err != nil || d < time.Minute {
		return 0, fmt.Errorf("%w: Intervall muss mindestens 1m sein", ErrInvalidSettings)
	}
	if s.Automatic && !s.Enabled {
		return 0, fmt.Errorf("%w: automatische Läufe erfordern Aktivierung", ErrInvalidSettings)
	}
	if s.Enabled && strings.TrimSpace(s.Reason) == "" {
		return 0, fmt.Errorf("%w: Begründung ist erforderlich", ErrInvalidSettings)
	}
	return d, nil
}

func (m *Manager) credentials(stored persistedSettings, replacement string) (string, error) {
	if replacement != "" {
		return replacement, nil
	}
	if stored.Ciphertext != "" {
		return decryptCredential(m.keyPath, stored.Ciphertext)
	}
	return m.envCredentials, nil
}

func (m *Manager) build(stored persistedSettings, replacement string) (*Service, error) {
	interval, err := validateSettings(stored.Settings)
	if err != nil {
		return nil, err
	}
	if !stored.Enabled {
		return nil, nil
	}
	creds, err := m.credentials(stored, replacement)
	if err != nil {
		return nil, fmt.Errorf("gespeichertes Servicekonto konnte nicht gelesen werden: %w", err)
	}
	if strings.TrimSpace(creds) == "" {
		return nil, fmt.Errorf("%w: Google-Servicekonto-JSON fehlt", ErrInvalidSettings)
	}
	s, err := m.factory(stored.Settings, creds)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidSettings, err)
	}
	s.automatic = stored.Automatic
	s.interval = interval
	return s, nil
}

func (m *Manager) activateLocked(stored persistedSettings, replacement string) error {
	s, err := m.build(stored, replacement)
	if err != nil {
		return err
	}
	m.service = s
	m.configError = ""
	return nil
}

// Save validates and builds the new runtime before persisting it. An invalid
// update cannot affect the current active sync. It never writes to Google.
func (m *Manager) Save(ctx context.Context, req SettingsUpdate) (Settings, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	next := persistedSettings{Settings: req.Settings, Ciphertext: m.config.Ciphertext}
	next.Reason = strings.TrimSpace(next.Reason)
	next.SheetID = strings.TrimSpace(next.SheetID)
	next.CredentialsConfigured = false
	if len(req.ServiceAccountJSON) > 32<<10 {
		return Settings{}, fmt.Errorf("%w: Servicekonto-JSON zu groß", ErrInvalidSettings)
	}
	secret := strings.TrimSpace(req.ServiceAccountJSON)
	if _, err := validateSettings(next.Settings); err != nil {
		return Settings{}, err
	}
	if secret != "" {
		// Check the credential structure even when sync is disabled.
		if _, err := NewGoogleWriter(secret); err != nil {
			return Settings{}, fmt.Errorf("%w: ungültiges Servicekonto-JSON", ErrInvalidSettings)
		}
	}
	updated, err := m.build(next, secret)
	if err != nil {
		return Settings{}, err
	}
	if secret != "" {
		next.Ciphertext, err = encryptCredential(m.keyPath, secret)
		if err != nil {
			return Settings{}, fmt.Errorf("Servicekonto konnte nicht sicher gespeichert werden: %w", err)
		}
	}
	raw, err := json.Marshal(next.Settings)
	if err != nil {
		return Settings{}, err
	}
	_, err = m.db.ExecContext(ctx, "INSERT INTO sheet_sync_config (id, config_json, credentials_ciphertext) VALUES (1, ?, ?) ON CONFLICT(id) DO UPDATE SET config_json = excluded.config_json, credentials_ciphertext = excluded.credentials_ciphertext", string(raw), next.Ciphertext)
	if err != nil {
		return Settings{}, err
	}
	m.config = next
	m.service = updated
	m.configError = ""
	m.startTimerLocked()
	out := next.Settings
	out.CredentialsConfigured = next.Ciphertext != "" || strings.TrimSpace(m.envCredentials) != ""
	return out, nil
}

func (m *Manager) GetStatus(ctx context.Context) (Status, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.service == nil {
		return Status{Enabled: false, SheetID: m.config.SheetID, LastError: m.configError}, nil
	}
	return m.service.GetStatus(ctx)
}

func (m *Manager) Preflight(ctx context.Context) (PreflightReport, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.service == nil {
		return PreflightReport{}, ErrDisabled
	}
	return m.service.Preflight(ctx)
}

func (m *Manager) RunOnce(ctx context.Context) (Status, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.service == nil {
		return Status{}, ErrDisabled
	}
	return m.service.RunOnce(ctx)
}

func (m *Manager) startTimerLocked() {
	if m.cancel != nil {
		m.cancel()
		m.cancel = nil
	}
	if m.service == nil || !m.config.Automatic {
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	m.cancel = cancel
	interval := m.service.interval
	go func() {
		run := func() {
			if ctx.Err() != nil {
				return
			}
			runCtx, done := context.WithTimeout(ctx, 4*time.Minute)
			defer done()
			_, _ = m.RunOnce(runCtx)
		}
		run()
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				run()
			}
		}
	}()
}

// Close is useful in tests and during controlled server shutdown.
func (m *Manager) Close() {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.cancel != nil {
		m.cancel()
		m.cancel = nil
	}
}

func readCredentialKey(path string, create bool) ([]byte, error) {
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) && create {
		key := make([]byte, 32)
		if _, err := rand.Read(key); err != nil {
			return nil, err
		}
		f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
		if err == nil {
			_, writeErr := f.Write(key)
			err = errors.Join(writeErr, f.Close())
			if err != nil {
				return nil, err
			}
			return key, nil
		}
		if !errors.Is(err, os.ErrExist) {
			return nil, err
		}
		info, err = os.Lstat(path)
	}
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Mode().Perm() != 0600 {
		return nil, errors.New("service-account key file must be regular and mode 0600")
	}
	key, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if len(key) != 32 {
		return nil, errors.New("invalid service-account key length")
	}
	return key, nil
}

func encryptCredential(path, secret string) (string, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return "", err
	}
	key, err := readCredentialKey(path, true)
	if err != nil {
		return "", err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", err
	}
	encrypted := gcm.Seal(nonce, nonce, []byte(secret), nil)
	return base64.StdEncoding.EncodeToString(encrypted), nil
}

func decryptCredential(path, encoded string) (string, error) {
	key, err := readCredentialKey(path, false)
	if err != nil {
		return "", err
	}
	blob, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return "", err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	if len(blob) < gcm.NonceSize() {
		return "", errors.New("encrypted service-account payload too short")
	}
	plain, err := gcm.Open(nil, blob[:gcm.NonceSize()], blob[gcm.NonceSize():], nil)
	if err != nil {
		return "", err
	}
	return string(plain), nil
}
