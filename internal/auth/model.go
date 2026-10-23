package auth

import (
	"time"

	"github.com/uptrace/bun"
)

type Session struct {
	bun.BaseModel `bun:"table:sessions"`

	ID         int64     `bun:"id,pk,autoincrement" json:"id"`
	TokenHash  string    `bun:"token_hash,notnull,unique" json:"-"`
	PlayerID   int64     `bun:"player_id,notnull" json:"player_id"`
	CreatedAt  time.Time `bun:"created_at,notnull" json:"created_at"`
	ExpiresAt  time.Time `bun:"expires_at,notnull" json:"expires_at"`
	LastSeenAt time.Time `bun:"last_seen_at,notnull" json:"last_seen_at"`
}

type LoginAttempt struct {
	bun.BaseModel `bun:"table:login_attempts"`

	ID        int64     `bun:"id,pk,autoincrement" json:"id"`
	Username  string    `bun:"username,notnull" json:"username"`
	ClientIP  string    `bun:"client_ip,notnull" json:"client_ip"`
	Success   bool      `bun:"success,notnull" json:"success"`
	CreatedAt time.Time `bun:"created_at,notnull" json:"created_at"`
}

type MFASettings struct {
	bun.BaseModel `bun:"table:player_mfa_settings"`

	PlayerID      int64      `bun:"player_id,pk" json:"player_id"`
	TOTPSecret    string     `bun:"totp_secret,notnull" json:"-"`
	TOTPEnabledAt *time.Time `bun:"totp_enabled_at" json:"totp_enabled_at,omitempty"`
	CreatedAt     time.Time  `bun:"created_at,notnull" json:"created_at"`
	UpdatedAt     time.Time  `bun:"updated_at,notnull" json:"updated_at"`
}

type MFARecoveryCode struct {
	bun.BaseModel `bun:"table:mfa_recovery_codes"`

	ID        int64      `bun:"id,pk,autoincrement" json:"id"`
	PlayerID  int64      `bun:"player_id,notnull" json:"player_id"`
	CodeHash  string     `bun:"code_hash,notnull,unique" json:"-"`
	UsedAt    *time.Time `bun:"used_at" json:"used_at,omitempty"`
	CreatedAt time.Time  `bun:"created_at,notnull" json:"created_at"`
}

type MFAChallenge struct {
	bun.BaseModel `bun:"table:mfa_login_challenges"`

	ID             int64      `bun:"id,pk,autoincrement" json:"id"`
	ChallengeHash  string     `bun:"challenge_hash,notnull,unique" json:"-"`
	PlayerID       int64      `bun:"player_id,notnull" json:"player_id"`
	CreatedAt      time.Time  `bun:"created_at,notnull" json:"created_at"`
	ExpiresAt      time.Time  `bun:"expires_at,notnull" json:"expires_at"`
	UsedAt         *time.Time `bun:"used_at" json:"used_at,omitempty"`
	FailedAttempts int        `bun:"failed_attempts,notnull,default:0" json:"failed_attempts"`
}

type WebAuthnCredential struct {
	bun.BaseModel `bun:"table:webauthn_credentials"`

	ID               int64      `bun:"id,pk,autoincrement" json:"id"`
	PlayerID         int64      `bun:"player_id,notnull" json:"player_id"`
	CredentialIDHash string     `bun:"credential_id_hash,notnull,unique" json:"-"`
	Name             string     `bun:"name,notnull" json:"name"`
	CredentialJSON   string     `bun:"credential_json,notnull" json:"-"`
	CreatedAt        time.Time  `bun:"created_at,notnull" json:"created_at"`
	UpdatedAt        time.Time  `bun:"updated_at,notnull" json:"updated_at"`
	LastUsedAt       *time.Time `bun:"last_used_at" json:"last_used_at,omitempty"`
}

type WebAuthnSession struct {
	bun.BaseModel `bun:"table:webauthn_sessions"`

	ID             int64      `bun:"id,pk,autoincrement" json:"id"`
	SessionHash    string     `bun:"session_hash,notnull,unique" json:"-"`
	PlayerID       int64      `bun:"player_id,notnull" json:"player_id"`
	MFAChallengeID *int64     `bun:"mfa_challenge_id" json:"mfa_challenge_id,omitempty"`
	Purpose        string     `bun:"purpose,notnull" json:"purpose"`
	SessionJSON    string     `bun:"session_json,notnull" json:"-"`
	CreatedAt      time.Time  `bun:"created_at,notnull" json:"created_at"`
	ExpiresAt      time.Time  `bun:"expires_at,notnull" json:"expires_at"`
	UsedAt         *time.Time `bun:"used_at" json:"used_at,omitempty"`
}
