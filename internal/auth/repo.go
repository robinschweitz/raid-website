package auth

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/uptrace/bun"

	"raid-website/internal/audit"
	"raid-website/internal/player"
)

type Repo struct {
	db         bun.IDB
	encryption *TOTPEncryption
}

func NewRepo(db bun.IDB) *Repo {
	return &Repo{db: db}
}

// UpgradePasswordHash replaces a verified hash with a new one of the same
// password. It only writes while the old hash is still stored, so a password
// change that ran meanwhile is not undone.
func (r *Repo) UpgradePasswordHash(ctx context.Context, playerID int64, oldHash, newHash string) error {
	_, err := r.db.NewUpdate().Model((*player.Player)(nil)).
		Set("password_hash = ?", newHash).
		Where("id = ? AND password_hash = ?", playerID, oldHash).
		Exec(ctx)
	if err != nil {
		return fmt.Errorf("upgrade password hash of player %d: %w", playerID, err)
	}
	return nil
}

func (r *Repo) WithTx(tx bun.Tx) *Repo {
	return &Repo{db: tx, encryption: r.encryption}
}

// InTransaction runs account changes using a transaction-bound player repository.
func (r *Repo) InTransaction(ctx context.Context, fn func(*player.Repo) error) error {
	return r.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		return fn(player.NewRepo(tx))
	})
}

// InAccountTransaction runs account changes and session deletion in one
// transaction using transaction-bound repositories.
func (r *Repo) InAccountTransaction(ctx context.Context, fn func(*player.Repo, *Repo) error) error {
	return r.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		return fn(player.NewRepo(tx), r.WithTx(tx))
	})
}

// DeleteSessionsOfPlayer deletes all sessions of a player except the one with
// keepTokenHash; an empty keepTokenHash keeps none (SC-017).
func (r *Repo) DeleteSessionsOfPlayer(ctx context.Context, playerID int64, keepTokenHash string) error {
	q := r.db.NewDelete().Model((*Session)(nil)).Where("player_id = ?", playerID)
	if keepTokenHash != "" {
		q = q.Where("token_hash <> ?", keepTokenHash)
	}
	_, err := q.Exec(ctx)
	return err
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

// ReserveLoginAttempt checks the per-username-and-address limit and the
// per-address limit and, when neither is reached, stores the attempt as
// failed, all in one transaction. With the single database connection (DR-9)
// no other login runs between the count and the insert, so parallel attempts
// are counted one by one (SC-008, SC-043). At a limit it stores nothing and
// returns ErrLoginRateLimited.
func (r *Repo) ReserveLoginAttempt(ctx context.Context, username, clientIP string, at, since time.Time, usernameLimit, addressLimit int) (int64, error) {
	var attemptID int64
	err := r.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		repo := r.WithTx(tx)
		addressCount, err := repo.CountFailedLoginAttemptsFromAddress(ctx, clientIP, since)
		if err != nil {
			return fmt.Errorf("count address login attempts: %w", err)
		}
		if addressCount >= addressLimit {
			return ErrLoginRateLimited
		}
		count, err := repo.CountFailedLoginAttempts(ctx, username, clientIP, since)
		if err != nil {
			return fmt.Errorf("count login attempts: %w", err)
		}
		if count >= usernameLimit {
			return ErrLoginRateLimited
		}
		attempt := &LoginAttempt{Username: username, ClientIP: clientIP, Success: false, CreatedAt: at}
		if err := repo.CreateLoginAttempt(ctx, attempt); err != nil {
			return fmt.Errorf("store login attempt: %w", err)
		}
		attemptID = attempt.ID
		return nil
	})
	return attemptID, err
}

// MarkLoginAttemptSucceeded records that a reserved attempt logged in.
func (r *Repo) MarkLoginAttemptSucceeded(ctx context.Context, id int64) error {
	_, err := r.db.NewUpdate().Model((*LoginAttempt)(nil)).Set("success = ?", true).Where("id = ?", id).Exec(ctx)
	return err
}

// DeleteLoginAttempt removes a reserved attempt whose password was never checked.
func (r *Repo) DeleteLoginAttempt(ctx context.Context, id int64) error {
	_, err := r.db.NewDelete().Model((*LoginAttempt)(nil)).Where("id = ?", id).Exec(ctx)
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
	if r.encryption != nil {
		s.TOTPSecret, err = r.encryption.decrypt(playerID, s.TOTPSecret)
		if err != nil {
			return nil, err
		}
	}
	return s, nil
}

func (r *Repo) UpsertMFASettings(ctx context.Context, s *MFASettings) error {
	stored := *s
	if r.encryption != nil {
		var err error
		stored.TOTPSecret, err = r.encryption.encrypt(s.PlayerID, s.TOTPSecret)
		if err != nil {
			return err
		}
	}
	s = &stored
	_, err := r.db.NewInsert().Model(s).
		On("CONFLICT(player_id) DO UPDATE").
		Set("totp_secret = EXCLUDED.totp_secret").
		Set("totp_enabled_at = EXCLUDED.totp_enabled_at").
		Set("updated_at = EXCLUDED.updated_at").
		Exec(ctx)
	return err
}

// ClaimTOTPStep records step as the last accepted TOTP step when it is later
// than the stored one. It reports false when the step was already used, so of
// two requests with the same code exactly one succeeds (SC-012).
func (r *Repo) ClaimTOTPStep(ctx context.Context, playerID, step int64) (bool, error) {
	result, err := r.db.NewUpdate().Model((*MFASettings)(nil)).
		Set("totp_last_step = ?", step).
		Where("player_id = ?", playerID).
		Where("totp_last_step IS NULL OR totp_last_step < ?", step).
		Exec(ctx)
	if err != nil {
		return false, err
	}
	affected, err := result.RowsAffected()
	return affected == 1, err
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

// RecordMFAChallengeFailure counts one wrong code on a challenge and returns
// the new number of wrong codes (SC-011).
func (r *Repo) RecordMFAChallengeFailure(ctx context.Context, id int64) (int, error) {
	var failedAttempts int
	err := r.db.NewUpdate().Model((*MFAChallenge)(nil)).
		Set("failed_attempts = failed_attempts + 1").
		Where("id = ?", id).
		Returning("failed_attempts").
		Scan(ctx, &failedAttempts)
	return failedAttempts, err
}

func (r *Repo) DeleteMFAChallenge(ctx context.Context, id int64) error {
	_, err := r.db.NewDelete().Model((*MFAChallenge)(nil)).Where("id = ?", id).Exec(ctx)
	return err
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

// WithTOTPEncryption configures encryption for this repository and its transactions.
func (r *Repo) WithTOTPEncryption(e *TOTPEncryption) *Repo { return &Repo{db: r.db, encryption: e} }

// MigrateTOTPSecrets encrypts existing plaintext before serving requests. Any
// failure rolls back every row and exposes only the number of failures.
func (r *Repo) MigrateTOTPSecrets(ctx context.Context) error {
	if r.encryption == nil {
		return errTOTPEncryption
	}
	return r.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		var rows []MFASettings
		if err := tx.NewSelect().Model(&rows).Scan(ctx); err != nil {
			return fmt.Errorf("TOTP migration failed for %d row(s)", 1)
		}
		failures := 0
		for _, row := range rows {
			if row.TOTPSecret == "" {
				continue
			}
			plain := row.TOTPSecret
			if strings.HasPrefix(plain, "v1:") {
				if _, err := r.encryption.decrypt(row.PlayerID, plain); err != nil {
					failures++
				}
				continue
			}
			stored, err := r.encryption.encrypt(row.PlayerID, plain)
			if err != nil {
				failures++
				continue
			}
			if _, err := tx.NewUpdate().Model((*MFASettings)(nil)).Set("totp_secret = ?", stored).Where("player_id = ?", row.PlayerID).Exec(ctx); err != nil {
				failures++
			}
		}
		if failures > 0 {
			return fmt.Errorf("TOTP migration failed for %d row(s)", failures)
		}
		// Check persisted values too: a trigger or unexpected write cannot leave plaintext behind.
		rows = nil
		if err := tx.NewSelect().Model(&rows).Scan(ctx); err != nil {
			return fmt.Errorf("TOTP migration failed for %d row(s)", 1)
		}
		for _, row := range rows {
			if _, err := r.encryption.decrypt(row.PlayerID, row.TOTPSecret); err != nil {
				failures++
			}
		}
		if failures > 0 {
			return fmt.Errorf("TOTP migration failed for %d row(s)", failures)
		}
		return nil
	})
}
