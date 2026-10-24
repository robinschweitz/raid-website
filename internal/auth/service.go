package auth

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1"
	"crypto/sha256"
	"database/sql"
	"encoding/base32"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"

	"raid-website/internal/player"
	"raid-website/internal/validation"
)

const (
	defaultSessionTTL         = 30 * 24 * time.Hour
	MinPasswordLength         = 12
	loginFailureLimit         = 5
	loginFailureWindow        = 15 * time.Minute
	addressFailureLimit       = 30
	mfaChallengeTTL           = 5 * time.Minute
	mfaFailureLimit           = 5
	totpPeriodSeconds   int64 = 30

	// CookieName is the name of the session cookie.
	CookieName = "raid_session"
)

// comparePassword is the login password check; tests replace it to prove that
// a rate-limited attempt never reaches the password hash (SC-008, SC-043).
var comparePassword = func(hash, password []byte) error {
	return verifyStoredPassword(string(hash), string(password))
}

// dummyPasswordHash is an Argon2id hash (current parameters) of a random secret
// that was discarded after hashing. Logins for unknown usernames and accounts
// without a password compare against it, so they take as long as a wrong
// password and response time does not reveal which usernames exist (SC-009).
const dummyPasswordHash = "$argon2id$v=19$m=65536,t=3,p=1$ydwgDnow7o4c5A0n0TInAg$/MoBjTPy4gijAXdfRlT5UutfKTMtkB5JLw6KUqRbADo"

// rejectWithoutAccount spends the time of one password comparison and fails.
func rejectWithoutAccount(password string) error {
	_ = comparePassword([]byte(dummyPasswordHash), []byte(password))
	return ErrInvalidCredentials
}

var (
	// ErrInvalidCredentials is returned when a username/password pair does not match.
	ErrInvalidCredentials = errors.New("invalid credentials")
	// ErrInactive is returned when the account exists but is deactivated.
	ErrInactive = errors.New("account inactive")
	// ErrWeakPassword is returned when a new password does not meet the policy.
	ErrWeakPassword = errors.New("password must be at least 12 characters")
	// ErrLoginRateLimited is returned when too many failed logins were recorded recently.
	ErrLoginRateLimited = errors.New("login rate limited")
	ErrMFARequired      = errors.New("mfa required")
	ErrInvalidMFA       = errors.New("invalid mfa code")
	// ErrMFAChallengeInvalid is returned when an MFA challenge is unknown,
	// expired, used, or invalidated after too many wrong codes (SC-011); the
	// user has to start again with the password.
	ErrMFAChallengeInvalid = errors.New("mfa challenge invalid")
)

type LoginStart struct {
	Token        string
	Player       *player.Player
	MFAChallenge string
	MFAFactors   MFAFactors
}

type MFAFactors struct {
	TOTPEnabled   bool `json:"totp_enabled"`
	RecoveryCodes bool `json:"recovery_codes"`
	WebAuthn      bool `json:"webauthn"`
}

type TOTPSetup struct {
	Secret string `json:"secret"`
	URI    string `json:"uri"`
}

type MFAStatus struct {
	TOTPEnabled   bool                     `json:"totp_enabled"`
	RecoveryCodes bool                     `json:"recovery_codes"`
	SecurityKeys  []WebAuthnCredentialInfo `json:"security_keys"`
}

type WebAuthnCredentialInfo struct {
	ID         int64      `json:"id"`
	Name       string     `json:"name"`
	CreatedAt  time.Time  `json:"created_at"`
	LastUsedAt *time.Time `json:"last_used_at,omitempty"`
}

type Service struct {
	players  *player.Repo
	sessions *Repo
	ttl      time.Duration
}

func NewService(players *player.Repo, sessions *Repo) *Service {
	return &Service{players: players, sessions: sessions, ttl: defaultSessionTTL}
}

// ErrPasswordTooLong is returned when a password exceeds maxPasswordBytes (SC-005).
var ErrPasswordTooLong = errors.New("password must be at most 1024 bytes")

// ValidatePassword enforces the account password policy for new or changed credentials.
func ValidatePassword(password string) error {
	if len([]rune(strings.TrimSpace(password))) < MinPasswordLength {
		return ErrWeakPassword
	}
	// The length is measured on the raw password, because the hash reads the raw bytes.
	if len(password) > maxPasswordBytes {
		return ErrPasswordTooLong
	}
	return nil
}

func (s *Service) VerifyPassword(ctx context.Context, playerID int64, password string) error {
	p, err := s.players.Get(ctx, playerID)
	if err != nil {
		return fmt.Errorf("get password player: %w", err)
	}
	if p.PasswordHash == "" || verifyStoredPassword(p.PasswordHash, password) != nil {
		return ErrInvalidCredentials
	}
	if !p.Active {
		return ErrInactive
	}
	return nil
}

// auditSecurity records a security change with the player's name so the
// audit page can show whose account it was.
func (s *Service) auditSecurity(ctx context.Context, playerID int64, action string, details map[string]any) error {
	p, err := s.players.Get(ctx, playerID)
	if err != nil {
		return fmt.Errorf("load player for security audit: %w", err)
	}
	if details == nil {
		details = map[string]any{}
	}
	details["name"] = p.Name
	if err := s.sessions.AuditSecurity(ctx, playerID, action, details); err != nil {
		return fmt.Errorf("record security audit: %w", err)
	}
	return nil
}

// ChangeOwnPassword sets a new password and ends every other session of the
// player; the session identified by currentToken stays (FR-2.13, SC-017).
func (s *Service) ChangeOwnPassword(ctx context.Context, playerID int64, currentToken, currentPassword, newPassword string) error {
	if err := s.VerifyPassword(ctx, playerID, currentPassword); err != nil {
		return err
	}
	if err := ValidatePassword(newPassword); err != nil {
		return err
	}
	hash, err := HashPassword(newPassword)
	if err != nil {
		return fmt.Errorf("hash own password: %w", err)
	}
	keepHash := ""
	if currentToken != "" {
		keepHash = hashToken(currentToken)
	}
	return s.sessions.InAccountTransaction(ctx, func(players *player.Repo, sessions *Repo) error {
		p, err := players.Get(ctx, playerID)
		if err != nil {
			return fmt.Errorf("get own password player: %w", err)
		}
		p.PasswordHash = hash
		p.UpdatedAt = time.Now().UTC()
		if err := players.Update(ctx, p); err != nil {
			return fmt.Errorf("update own password: %w", err)
		}
		if err := sessions.DeleteSessionsOfPlayer(ctx, playerID, keepHash); err != nil {
			return fmt.Errorf("end other sessions: %w", err)
		}
		if err := sessions.AuditSecurity(ctx, playerID, "password_change", map[string]any{"name": p.Name}); err != nil {
			return fmt.Errorf("record security audit: %w", err)
		}
		return nil
	})
}

// BootstrapAdmin creates an initial account atomically without changing existing accounts.
func (s *Service) BootstrapAdmin(ctx context.Context, username, password string) error {
	if username == "" || password == "" {
		return nil
	}
	uname := strings.ToLower(strings.TrimSpace(username))
	if uname == "" {
		return errors.New("bootstrap admin: username required")
	}
	// The minimum length is not enforced here so development defaults keep
	// working; an over-long password still stops startup (NFR-1.1, SC-005).
	if len(password) > maxPasswordBytes {
		return fmt.Errorf("bootstrap admin: %w", ErrPasswordTooLong)
	}
	return s.sessions.InTransaction(ctx, func(players *player.Repo) error {
		if _, err := players.FindByUsername(ctx, uname); err == nil {
			return nil
		} else if !errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("find bootstrap admin: %w", err)
		}
		// The password is only used when the account is created; an existing account is left untouched.
		if err := ValidatePassword(password); err != nil {
			return err
		}
		hash, err := HashPassword(password)
		if err != nil {
			return fmt.Errorf("hash bootstrap password: %w", err)
		}
		now := time.Now().UTC()
		p := &player.Player{
			Name: username, Username: &uname, PasswordHash: hash,
			Active: true, Role: validation.RoleAdmin, CreatedAt: now, UpdatedAt: now,
		}
		if err := players.Create(ctx, p); err != nil {
			return fmt.Errorf("create bootstrap admin: %w", err)
		}
		return nil
	})
}

// SetCredentials assigns login credentials and role to an existing player.
func (s *Service) SetCredentials(ctx context.Context, playerID int64, username, password string, role validation.Role, actorID int64) (*player.Player, error) {
	var updated *player.Player
	err := s.sessions.InAccountTransaction(ctx, func(players *player.Repo, sessions *Repo) error {
		p, err := players.Get(ctx, playerID)
		if err != nil {
			return fmt.Errorf("get credential player: %w", err)
		}
		uname := strings.ToLower(strings.TrimSpace(username))
		if uname == "" {
			return errors.New("username required")
		}
		if !role.Valid() {
			return errors.New("invalid role")
		}
		if p.Role == validation.RoleAdmin && p.Active && role != validation.RoleAdmin {
			admins, err := players.CountActiveAdmins(ctx)
			if err != nil {
				return fmt.Errorf("count active admins: %w", err)
			}
			if admins <= 1 {
				return player.ErrLastAdmin
			}
		}
		before := *p
		hash := p.PasswordHash
		if password != "" {
			if err := ValidatePassword(password); err != nil {
				return err
			}
			hash, err = HashPassword(password)
			if err != nil {
				return fmt.Errorf("hash credential password: %w", err)
			}
		}
		endSessions := password != "" || p.Role != role || p.Username == nil || *p.Username != uname
		p.Username = &uname
		p.PasswordHash = hash
		p.Role = role
		p.UpdatedAt = time.Now().UTC()
		if err := players.Update(ctx, p); err != nil {
			return fmt.Errorf("update player credentials: %w", err)
		}
		if endSessions {
			if err := sessions.DeleteSessionsOfPlayer(ctx, playerID, ""); err != nil {
				return fmt.Errorf("end player sessions: %w", err)
			}
		}
		updated = p
		return players.Audit(ctx, actorID, p.ID, "set_account", &before, player.AccountAudit{Player: p, PasswordChanged: password != ""})
	})
	if err != nil {
		return nil, err
	}
	return updated, nil
}

// Login verifies credentials and creates a session, returning the raw token.
func (s *Service) Login(ctx context.Context, username, password string) (string, *player.Player, error) {
	return s.LoginWithClient(ctx, username, password, "")
}

// LoginWithClient verifies credentials with rate limiting per username and
// client address (SC-008) and per client address (SC-043).
func (s *Service) LoginWithClient(ctx context.Context, username, password, clientIP string) (string, *player.Player, error) {
	result, err := s.StartLogin(ctx, username, password, clientIP)
	if err != nil {
		return "", nil, err
	}
	if result.MFAChallenge != "" {
		return "", nil, ErrMFARequired
	}
	return result.Token, result.Player, nil
}

func (s *Service) StartLogin(ctx context.Context, username, password, clientIP string) (*LoginStart, error) {
	uname := strings.ToLower(strings.TrimSpace(username))
	now := time.Now().UTC()
	if clientIP != "" {
		addressCount, err := s.sessions.CountFailedLoginAttemptsFromAddress(ctx, clientIP, now.Add(-loginFailureWindow))
		if err != nil {
			return nil, fmt.Errorf("count address login attempts: %w", err)
		}
		if addressCount >= addressFailureLimit {
			return nil, ErrLoginRateLimited
		}
		count, err := s.sessions.CountFailedLoginAttempts(ctx, uname, clientIP, now.Add(-loginFailureWindow))
		if err != nil {
			return nil, fmt.Errorf("count login attempts: %w", err)
		}
		if count >= loginFailureLimit {
			return nil, ErrLoginRateLimited
		}
	}
	p, err := s.players.FindByUsername(ctx, uname)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			s.recordLoginAttempt(ctx, uname, clientIP, false, now)
			return nil, rejectWithoutAccount(password)
		}
		return nil, fmt.Errorf("find login player: %w", err)
	}
	if p.PasswordHash == "" {
		s.recordLoginAttempt(ctx, uname, clientIP, false, now)
		return nil, rejectWithoutAccount(password)
	}
	if err := comparePassword([]byte(p.PasswordHash), []byte(password)); err != nil {
		s.recordLoginAttempt(ctx, uname, clientIP, false, now)
		return nil, ErrInvalidCredentials
	}
	if !p.Active {
		s.recordLoginAttempt(ctx, uname, clientIP, false, now)
		return nil, ErrInactive
	}
	// The password was just verified, so a legacy bcrypt hash (or Argon2id with
	// old parameters) is replaced now. The password itself does not change, so
	// sessions stay valid and no audit entry is written (ADR 100).
	if needsRehash(p.PasswordHash) {
		replacement, err := HashPassword(password)
		if err != nil {
			return nil, fmt.Errorf("rehash password: %w", err)
		}
		if err := s.sessions.UpgradePasswordHash(ctx, p.ID, p.PasswordHash, replacement); err != nil {
			return nil, fmt.Errorf("upgrade password hash: %w", err)
		}
	}
	status, err := s.MFAStatus(ctx, p.ID)
	if err != nil {
		return nil, err
	}
	factors := status.Factors()
	if factors.Any() {
		challenge, err := newToken()
		if err != nil {
			return nil, fmt.Errorf("generate mfa challenge: %w", err)
		}
		if err := s.sessions.CreateMFAChallenge(ctx, &MFAChallenge{ChallengeHash: hashToken(challenge), PlayerID: p.ID, CreatedAt: now, ExpiresAt: now.Add(mfaChallengeTTL)}); err != nil {
			return nil, fmt.Errorf("create mfa challenge: %w", err)
		}
		s.recordLoginAttempt(ctx, uname, clientIP, true, now)
		return &LoginStart{Player: p, MFAChallenge: challenge, MFAFactors: factors}, nil
	}
	token, err := newToken()
	if err != nil {
		return nil, fmt.Errorf("generate session token: %w", err)
	}
	sess := &Session{
		TokenHash:  hashToken(token),
		PlayerID:   p.ID,
		CreatedAt:  now,
		ExpiresAt:  now.Add(s.ttl),
		LastSeenAt: now,
	}
	if err := s.sessions.CreateSession(ctx, sess); err != nil {
		return nil, fmt.Errorf("create login session: %w", err)
	}
	s.recordLoginAttempt(ctx, uname, clientIP, true, now)
	return &LoginStart{Token: token, Player: p}, nil
}

func (s *Service) recordLoginAttempt(ctx context.Context, username, clientIP string, success bool, at time.Time) {
	if clientIP == "" {
		return
	}
	if err := s.sessions.CreateLoginAttempt(ctx, &LoginAttempt{Username: username, ClientIP: clientIP, Success: success, CreatedAt: at}); err != nil {
		// Login correctness must not depend on best-effort throttle bookkeeping after the credential decision.
		return
	}
}

// Authenticate resolves a raw session token. Missing, unknown, or expired sessions return nil, nil.
func (s *Service) Authenticate(ctx context.Context, token string) (*player.Player, error) {
	if token == "" {
		return nil, nil
	}
	sess, err := s.sessions.FindSessionByHash(ctx, hashToken(token))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("find authentication session: %w", err)
	}
	now := time.Now().UTC()
	if !now.Before(sess.ExpiresAt) {
		return nil, nil
	}
	p, err := s.players.Get(ctx, sess.PlayerID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("get session player: %w", err)
	}
	if !p.Active {
		return nil, ErrInactive
	}
	if err := s.sessions.TouchSession(ctx, sess.ID, now); err != nil {
		return nil, fmt.Errorf("touch authentication session: %w", err)
	}
	return p, nil
}

// Logout deletes the session associated with a raw token.
func (s *Service) Logout(ctx context.Context, token string) error {
	if token == "" {
		return nil
	}
	if err := s.sessions.DeleteByHash(ctx, hashToken(token)); err != nil {
		return fmt.Errorf("delete logout session: %w", err)
	}
	return nil
}

func (s *Service) MFAEnabled(ctx context.Context, playerID int64) (bool, error) {
	status, err := s.MFAStatus(ctx, playerID)
	if err != nil {
		return false, err
	}
	return status.Factors().Any(), nil
}

func (s *Service) SetupTOTP(ctx context.Context, playerID int64, issuer string) (*TOTPSetup, error) {
	p, err := s.players.Get(ctx, playerID)
	if err != nil {
		return nil, fmt.Errorf("get totp player: %w", err)
	}
	secret, err := newTOTPSecret()
	if err != nil {
		return nil, fmt.Errorf("generate totp secret: %w", err)
	}
	now := time.Now().UTC()
	settings := &MFASettings{PlayerID: playerID, TOTPSecret: secret, CreatedAt: now, UpdatedAt: now}
	if err := s.sessions.UpsertMFASettings(ctx, settings); err != nil {
		return nil, fmt.Errorf("save totp settings: %w", err)
	}
	if issuer == "" {
		issuer = "Bratwurst des Schicksals"
	}
	return &TOTPSetup{Secret: secret, URI: totpURI(issuer, accountLabel(p), secret)}, nil
}

func (s *Service) EnableTOTP(ctx context.Context, playerID int64, code string) ([]string, error) {
	settings, err := s.sessions.GetMFASettings(ctx, playerID)
	if err != nil {
		return nil, fmt.Errorf("get totp settings: %w", err)
	}
	accepted, err := s.acceptTOTP(ctx, settings, code, time.Now().UTC())
	if err != nil {
		return nil, err
	}
	if !accepted {
		return nil, ErrInvalidMFA
	}
	recovery, err := newRecoveryCodes(10)
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	settings.TOTPEnabledAt = &now
	settings.UpdatedAt = now
	if err := s.sessions.UpsertMFASettings(ctx, settings); err != nil {
		return nil, fmt.Errorf("enable totp: %w", err)
	}
	if err := s.sessions.DeleteRecoveryCodes(ctx, playerID); err != nil {
		return nil, fmt.Errorf("replace recovery codes: %w", err)
	}
	for _, plain := range recovery {
		if err := s.sessions.CreateRecoveryCode(ctx, &MFARecoveryCode{PlayerID: playerID, CodeHash: hashToken(normalizeMFACode(plain)), CreatedAt: now}); err != nil {
			return nil, fmt.Errorf("create recovery code: %w", err)
		}
	}
	if err := s.auditSecurity(ctx, playerID, "totp_enable", nil); err != nil {
		return nil, err
	}
	return recovery, nil
}

// HasSecondFactor reports whether the player has TOTP or at least one security key.
// Recovery codes alone are a fallback, not a second factor (FR-3.12, SC-040).
func (s *Service) HasSecondFactor(ctx context.Context, playerID int64) (bool, error) {
	settings, err := s.sessions.GetMFASettings(ctx, playerID)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return false, fmt.Errorf("get mfa settings: %w", err)
	}
	if settings != nil && settings.TOTPEnabledAt != nil && settings.TOTPSecret != "" {
		return true, nil
	}
	keys, err := s.sessions.CountWebAuthnCredentials(ctx, playerID)
	if err != nil {
		return false, fmt.Errorf("count webauthn credentials: %w", err)
	}
	return keys > 0, nil
}

func (s *Service) MFAStatus(ctx context.Context, playerID int64) (*MFAStatus, error) {
	status := &MFAStatus{SecurityKeys: []WebAuthnCredentialInfo{}}
	settings, err := s.sessions.GetMFASettings(ctx, playerID)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("get mfa settings: %w", err)
	}
	if settings != nil {
		status.TOTPEnabled = settings.TOTPEnabledAt != nil && settings.TOTPSecret != ""
	}
	recoveryCodes, err := s.sessions.CountUnusedRecoveryCodes(ctx, playerID)
	if err != nil {
		return nil, fmt.Errorf("count recovery codes: %w", err)
	}
	status.RecoveryCodes = recoveryCodes > 0
	keys, err := s.sessions.ListWebAuthnCredentials(ctx, playerID)
	if err != nil {
		return nil, fmt.Errorf("list webauthn credentials: %w", err)
	}
	for _, key := range keys {
		status.SecurityKeys = append(status.SecurityKeys, WebAuthnCredentialInfo{ID: key.ID, Name: key.Name, CreatedAt: key.CreatedAt, LastUsedAt: key.LastUsedAt})
	}
	return status, nil
}

func (s *MFAStatus) Factors() MFAFactors {
	if s == nil {
		return MFAFactors{}
	}
	return MFAFactors{TOTPEnabled: s.TOTPEnabled, RecoveryCodes: s.RecoveryCodes, WebAuthn: len(s.SecurityKeys) > 0}
}

func (f MFAFactors) Any() bool {
	return f.TOTPEnabled || f.RecoveryCodes || f.WebAuthn
}

func (s *Service) DisableTOTP(ctx context.Context, playerID int64, currentPassword string) error {
	if err := s.VerifyPassword(ctx, playerID, currentPassword); err != nil {
		return err
	}
	settings, err := s.sessions.GetMFASettings(ctx, playerID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil
		}
		return fmt.Errorf("get mfa settings: %w", err)
	}
	settings.TOTPSecret = ""
	settings.TOTPEnabledAt = nil
	settings.UpdatedAt = time.Now().UTC()
	if err := s.sessions.UpsertMFASettings(ctx, settings); err != nil {
		return fmt.Errorf("disable totp: %w", err)
	}
	if err := s.sessions.DeleteRecoveryCodes(ctx, playerID); err != nil {
		return fmt.Errorf("delete recovery codes: %w", err)
	}
	return s.auditSecurity(ctx, playerID, "totp_disable", nil)
}

func (s *Service) DeleteWebAuthnCredential(ctx context.Context, playerID, credentialID int64, currentPassword string) error {
	if err := s.VerifyPassword(ctx, playerID, currentPassword); err != nil {
		return err
	}
	credential, err := s.sessions.GetWebAuthnCredential(ctx, credentialID)
	if err != nil {
		return fmt.Errorf("get webauthn credential: %w", err)
	}
	if credential.PlayerID != playerID {
		return ErrInvalidMFA
	}
	if err := s.sessions.DeleteWebAuthnCredential(ctx, credentialID); err != nil {
		return fmt.Errorf("delete webauthn credential: %w", err)
	}
	return s.auditSecurity(ctx, playerID, "security_key_remove", map[string]any{"security_key": credential.Name})
}

func (s *Service) VerifyMFAChallenge(ctx context.Context, challenge, code string) (string, *player.Player, error) {
	now := time.Now().UTC()
	c, err := s.sessions.FindMFAChallenge(ctx, hashToken(challenge), now)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", nil, ErrMFAChallengeInvalid
		}
		return "", nil, fmt.Errorf("find mfa challenge: %w", err)
	}
	// A challenge whose deletion failed after the last wrong code stays closed.
	if c.FailedAttempts >= mfaFailureLimit {
		return "", nil, ErrMFAChallengeInvalid
	}
	p, err := s.players.Get(ctx, c.PlayerID)
	if err != nil {
		return "", nil, fmt.Errorf("get mfa player: %w", err)
	}
	// An account may have recovery codes or security keys but no TOTP row.
	settings, err := s.sessions.GetMFASettings(ctx, p.ID)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return "", nil, fmt.Errorf("get mfa settings: %w", err)
	}
	accepted := false
	if settings != nil {
		accepted, err = s.acceptTOTP(ctx, settings, code, now)
		if err != nil {
			return "", nil, err
		}
	}
	if !accepted {
		used, err := s.useRecoveryCode(ctx, p.ID, code, now)
		if err != nil {
			return "", nil, err
		}
		if !used {
			return "", nil, s.recordWrongMFACode(ctx, c.ID)
		}
	}
	if err := s.sessions.MarkMFAChallengeUsed(ctx, c.ID, now); err != nil {
		return "", nil, fmt.Errorf("mark mfa challenge used: %w", err)
	}
	token, err := s.createSession(ctx, p.ID, now)
	if err != nil {
		return "", nil, err
	}
	return token, p, nil
}

// recordWrongMFACode counts a wrong code and deletes the challenge at the
// limit, so the user has to start again with the password (SC-011, FR-3.9).
func (s *Service) recordWrongMFACode(ctx context.Context, challengeID int64) error {
	failedAttempts, err := s.sessions.RecordMFAChallengeFailure(ctx, challengeID)
	if err != nil {
		return fmt.Errorf("record wrong mfa code: %w", err)
	}
	if failedAttempts < mfaFailureLimit {
		return ErrInvalidMFA
	}
	if err := s.sessions.DeleteMFAChallenge(ctx, challengeID); err != nil {
		return fmt.Errorf("delete mfa challenge: %w", err)
	}
	return ErrMFAChallengeInvalid
}

func (s *Service) useRecoveryCode(ctx context.Context, playerID int64, code string, now time.Time) (bool, error) {
	row, err := s.sessions.FindUnusedRecoveryCode(ctx, playerID, hashToken(normalizeMFACode(code)))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return false, nil
		}
		return false, fmt.Errorf("find recovery code: %w", err)
	}
	if err := s.sessions.MarkRecoveryCodeUsed(ctx, row.ID, now); err != nil {
		return false, fmt.Errorf("mark recovery code used: %w", err)
	}
	return true, nil
}

func (s *Service) createSession(ctx context.Context, playerID int64, now time.Time) (string, error) {
	token, err := newToken()
	if err != nil {
		return "", fmt.Errorf("generate session token: %w", err)
	}
	if err := s.sessions.CreateSession(ctx, &Session{TokenHash: hashToken(token), PlayerID: playerID, CreatedAt: now, ExpiresAt: now.Add(s.ttl), LastSeenAt: now}); err != nil {
		return "", fmt.Errorf("create login session: %w", err)
	}
	return token, nil
}

func newTOTPSecret() (string, error) {
	b := make([]byte, 20)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return strings.TrimRight(base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(b), "="), nil
}

// acceptTOTP checks a TOTP code and claims its time step, so each code works
// at most once per account (SC-012).
func (s *Service) acceptTOTP(ctx context.Context, settings *MFASettings, code string, at time.Time) (bool, error) {
	step, ok := matchTOTP(settings.TOTPSecret, code, at)
	if !ok {
		return false, nil
	}
	claimed, err := s.sessions.ClaimTOTPStep(ctx, settings.PlayerID, step)
	if err != nil {
		return false, fmt.Errorf("claim totp step: %w", err)
	}
	return claimed, nil
}

// matchTOTP returns the time step whose code equals code, allowing one step
// of drift in either direction.
func matchTOTP(secret, code string, at time.Time) (int64, bool) {
	code = normalizeMFACode(code)
	if len(code) != 6 {
		return 0, false
	}
	current := at.Unix() / totpPeriodSeconds
	for offset := int64(-1); offset <= 1; offset++ {
		if generateTOTP(secret, current+offset) == code {
			return current + offset, true
		}
	}
	return 0, false
}

func generateTOTP(secret string, counter int64) string {
	key, err := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(strings.ToUpper(strings.TrimSpace(secret)))
	if err != nil {
		return ""
	}
	var msg [8]byte
	binary.BigEndian.PutUint64(msg[:], uint64(counter))
	mac := hmac.New(sha1.New, key)
	mac.Write(msg[:])
	sum := mac.Sum(nil)
	offset := sum[len(sum)-1] & 0x0f
	bin := (uint32(sum[offset])&0x7f)<<24 | (uint32(sum[offset+1])&0xff)<<16 | (uint32(sum[offset+2])&0xff)<<8 | (uint32(sum[offset+3]) & 0xff)
	return fmt.Sprintf("%06d", bin%1000000)
}

func normalizeMFACode(code string) string {
	return strings.ToUpper(strings.ReplaceAll(strings.ReplaceAll(strings.TrimSpace(code), " ", ""), "-", ""))
}

func newRecoveryCodes(count int) ([]string, error) {
	result := make([]string, 0, count)
	for range count {
		b := make([]byte, 8)
		if _, err := rand.Read(b); err != nil {
			return nil, err
		}
		encoded := strings.ToUpper(base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(b))
		result = append(result, encoded[:4]+"-"+encoded[4:8]+"-"+encoded[8:12])
	}
	return result, nil
}

func totpURI(issuer, label, secret string) string {
	v := url.Values{}
	v.Set("secret", secret)
	v.Set("issuer", issuer)
	v.Set("algorithm", "SHA1")
	v.Set("digits", "6")
	v.Set("period", strconv.FormatInt(totpPeriodSeconds, 10))
	return "otpauth://totp/" + url.PathEscape(issuer+":"+label) + "?" + v.Encode()
}

func accountLabel(p *player.Player) string {
	if p.Username != nil && *p.Username != "" {
		return *p.Username
	}
	return p.Name
}

func newToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

func hashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}
