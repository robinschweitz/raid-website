package auth

import (
	"context"
	"time"

	"github.com/uptrace/bun"

	"raid-website/internal/audit"
	"raid-website/internal/player"
)

type Repo struct {
	db bun.IDB
}

func NewRepo(db bun.IDB) *Repo {
	return &Repo{db: db}
}

func (r *Repo) WithTx(tx bun.Tx) *Repo {
	return &Repo{db: tx}
}

// InTransaction runs account changes using a transaction-bound player repository.
func (r *Repo) InTransaction(ctx context.Context, fn func(*player.Repo) error) error {
	return r.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		return fn(player.NewRepo(tx))
	})
}

// AuditSecurity records an account-security change the player made for
// themselves. details never contain secrets, hashes or codes (ADR 077).
func (r *Repo) AuditSecurity(ctx context.Context, playerID int64, action string, details map[string]any) error {
	return audit.NewRepo(r.db).Record(ctx, playerID, "player", playerID, action, nil, details, "")
}

func (r *Repo) CreateSession(ctx context.Context, s *Session) error {
	_, err := r.db.NewInsert().Model(s).Exec(ctx)
	return err
}

func (r *Repo) FindSessionByHash(ctx context.Context, tokenHash string) (*Session, error) {
	s := new(Session)
	err := r.db.NewSelect().Model(s).Where("token_hash = ?", tokenHash).Scan(ctx)
	if err != nil {
		return nil, err
	}
	return s, nil
}

func (r *Repo) TouchSession(ctx context.Context, id int64, at time.Time) error {
	_, err := r.db.NewUpdate().
		Model((*Session)(nil)).
		Set("last_seen_at = ?", at).
		Where("id = ?", id).
		Exec(ctx)
	return err
}

func (r *Repo) DeleteSession(ctx context.Context, id int64) error {
	_, err := r.db.NewDelete().Model((*Session)(nil)).Where("id = ?", id).Exec(ctx)
	return err
}

func (r *Repo) DeleteByHash(ctx context.Context, tokenHash string) error {
	_, err := r.db.NewDelete().Model((*Session)(nil)).Where("token_hash = ?", tokenHash).Exec(ctx)
	return err
}

func (r *Repo) DeleteExpired(ctx context.Context, before time.Time) error {
	_, err := r.db.NewDelete().Model((*Session)(nil)).Where("expires_at < ?", before).Exec(ctx)
	return err
}

func (r *Repo) CreateLoginAttempt(ctx context.Context, a *LoginAttempt) error {
	_, err := r.db.NewInsert().Model(a).Exec(ctx)
	return err
}

func (r *Repo) CountFailedLoginAttempts(ctx context.Context, username, clientIP string, since time.Time) (int, error) {
	count, err := r.db.NewSelect().Model((*LoginAttempt)(nil)).
		Where("username = ?", username).
		Where("client_ip = ?", clientIP).
		Where("success = 0").
		Where("created_at >= ?", since).
		Count(ctx)
	return int(count), err
}

// CountFailedLoginAttemptsFromAddress counts failed logins from one client
// address for any username since the given time (SC-043).
func (r *Repo) CountFailedLoginAttemptsFromAddress(ctx context.Context, clientIP string, since time.Time) (int, error) {
	count, err := r.db.NewSelect().Model((*LoginAttempt)(nil)).
		Where("client_ip = ?", clientIP).
		Where("success = 0").
		Where("created_at >= ?", since).
		Count(ctx)
	return int(count), err
}

func (r *Repo) GetMFASettings(ctx context.Context, playerID int64) (*MFASettings, error) {
	s := new(MFASettings)
	err := r.db.NewSelect().Model(s).Where("player_id = ?", playerID).Scan(ctx)
	if err != nil {
		return nil, err
	}
	return s, nil
}

func (r *Repo) UpsertMFASettings(ctx context.Context, s *MFASettings) error {
	_, err := r.db.NewInsert().Model(s).
		On("CONFLICT(player_id) DO UPDATE").
		Set("totp_secret = EXCLUDED.totp_secret").
		Set("totp_enabled_at = EXCLUDED.totp_enabled_at").
		Set("updated_at = EXCLUDED.updated_at").
		Exec(ctx)
	return err
}

func (r *Repo) DeleteRecoveryCodes(ctx context.Context, playerID int64) error {
	_, err := r.db.NewDelete().Model((*MFARecoveryCode)(nil)).Where("player_id = ?", playerID).Exec(ctx)
	return err
}

func (r *Repo) CreateRecoveryCode(ctx context.Context, code *MFARecoveryCode) error {
	_, err := r.db.NewInsert().Model(code).Exec(ctx)
	return err
}

func (r *Repo) FindUnusedRecoveryCode(ctx context.Context, playerID int64, hash string) (*MFARecoveryCode, error) {
	code := new(MFARecoveryCode)
	err := r.db.NewSelect().Model(code).Where("player_id = ?", playerID).Where("code_hash = ?", hash).Where("used_at IS NULL").Scan(ctx)
	if err != nil {
		return nil, err
	}
	return code, nil
}

func (r *Repo) MarkRecoveryCodeUsed(ctx context.Context, id int64, at time.Time) error {
	_, err := r.db.NewUpdate().Model((*MFARecoveryCode)(nil)).Set("used_at = ?", at).Where("id = ?", id).Where("used_at IS NULL").Exec(ctx)
	return err
}

func (r *Repo) CountUnusedRecoveryCodes(ctx context.Context, playerID int64) (int, error) {
	count, err := r.db.NewSelect().Model((*MFARecoveryCode)(nil)).Where("player_id = ?", playerID).Where("used_at IS NULL").Count(ctx)
	return int(count), err
}

func (r *Repo) CreateMFAChallenge(ctx context.Context, c *MFAChallenge) error {
	_, err := r.db.NewInsert().Model(c).Exec(ctx)
	return err
}

func (r *Repo) FindMFAChallenge(ctx context.Context, hash string, now time.Time) (*MFAChallenge, error) {
	c := new(MFAChallenge)
	err := r.db.NewSelect().Model(c).Where("challenge_hash = ?", hash).Where("used_at IS NULL").Where("expires_at > ?", now).Scan(ctx)
	if err != nil {
		return nil, err
	}
	return c, nil
}

func (r *Repo) MarkMFAChallengeUsed(ctx context.Context, id int64, at time.Time) error {
	_, err := r.db.NewUpdate().Model((*MFAChallenge)(nil)).Set("used_at = ?", at).Where("id = ?", id).Where("used_at IS NULL").Exec(ctx)
	return err
}

func (r *Repo) CountWebAuthnCredentials(ctx context.Context, playerID int64) (int, error) {
	count, err := r.db.NewSelect().Model((*WebAuthnCredential)(nil)).Where("player_id = ?", playerID).Count(ctx)
	return int(count), err
}

func (r *Repo) ListWebAuthnCredentials(ctx context.Context, playerID int64) ([]WebAuthnCredential, error) {
	var rows []WebAuthnCredential
	err := r.db.NewSelect().Model(&rows).Where("player_id = ?", playerID).Order("created_at ASC", "id ASC").Scan(ctx)
	return rows, err
}

func (r *Repo) GetWebAuthnCredential(ctx context.Context, id int64) (*WebAuthnCredential, error) {
	c := new(WebAuthnCredential)
	err := r.db.NewSelect().Model(c).Where("id = ?", id).Scan(ctx)
	if err != nil {
		return nil, err
	}
	return c, nil
}

func (r *Repo) CreateWebAuthnCredential(ctx context.Context, c *WebAuthnCredential) error {
	_, err := r.db.NewInsert().Model(c).Exec(ctx)
	return err
}

func (r *Repo) UpdateWebAuthnCredential(ctx context.Context, c *WebAuthnCredential) error {
	_, err := r.db.NewUpdate().Model(c).WherePK().Exec(ctx)
	return err
}

func (r *Repo) DeleteWebAuthnCredential(ctx context.Context, id int64) error {
	_, err := r.db.NewDelete().Model((*WebAuthnCredential)(nil)).Where("id = ?", id).Exec(ctx)
	return err
}

func (r *Repo) CreateWebAuthnSession(ctx context.Context, s *WebAuthnSession) error {
	_, err := r.db.NewInsert().Model(s).Exec(ctx)
	return err
}

func (r *Repo) FindWebAuthnSession(ctx context.Context, sessionHash, purpose string, now time.Time) (*WebAuthnSession, error) {
	s := new(WebAuthnSession)
	err := r.db.NewSelect().Model(s).Where("session_hash = ?", sessionHash).Where("purpose = ?", purpose).Where("used_at IS NULL").Where("expires_at > ?", now).Scan(ctx)
	if err != nil {
		return nil, err
	}
	return s, nil
}

func (r *Repo) MarkWebAuthnSessionUsed(ctx context.Context, id int64, at time.Time) error {
	_, err := r.db.NewUpdate().Model((*WebAuthnSession)(nil)).Set("used_at = ?", at).Where("id = ?", id).Where("used_at IS NULL").Exec(ctx)
	return err
}
