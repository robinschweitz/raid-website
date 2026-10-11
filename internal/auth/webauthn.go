package auth

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/go-webauthn/webauthn/protocol"
	wan "github.com/go-webauthn/webauthn/webauthn"

	"raid-website/internal/player"
)

const (
	webauthnSessionTTL          = 5 * time.Minute
	webauthnPurposeRegistration = "REGISTRATION"
	webauthnPurposeLogin        = "LOGIN"
)

type WebAuthnBeginRegistration struct {
	Session string                       `json:"session"`
	Options *protocol.CredentialCreation `json:"options"`
}

type WebAuthnBeginLogin struct {
	Session string                        `json:"session"`
	Options *protocol.CredentialAssertion `json:"options"`
}

type webAuthnUser struct {
	player      *player.Player
	credentials []wan.Credential
}

func (u webAuthnUser) WebAuthnID() []byte { return []byte(strconv.FormatInt(u.player.ID, 10)) }

func (u webAuthnUser) WebAuthnName() string {
	if u.player.Username != nil && *u.player.Username != "" {
		return *u.player.Username
	}
	return u.player.Name
}

func (u webAuthnUser) WebAuthnDisplayName() string { return u.player.Name }

func (u webAuthnUser) WebAuthnCredentials() []wan.Credential { return u.credentials }

func (s *Service) BeginWebAuthnRegistration(ctx context.Context, playerID int64, rpID, origin string) (*WebAuthnBeginRegistration, error) {
	user, err := s.webAuthnUser(ctx, playerID)
	if err != nil {
		return nil, err
	}
	w, err := newWebAuthn(rpID, origin)
	if err != nil {
		return nil, err
	}
	options, session, err := w.BeginRegistration(user, wan.WithExclusions(wan.Credentials(user.credentials).CredentialDescriptors()), wan.WithRegistrationOrigin(origin))
	if err != nil {
		return nil, fmt.Errorf("begin webauthn registration: %w", err)
	}
	token, err := s.storeWebAuthnSession(ctx, playerID, nil, webauthnPurposeRegistration, session)
	if err != nil {
		return nil, err
	}
	return &WebAuthnBeginRegistration{Session: token, Options: options}, nil
}

func (s *Service) FinishWebAuthnRegistration(ctx context.Context, playerID int64, sessionToken string, response []byte, rpID, origin string) error {
	now := time.Now().UTC()
	stored, session, err := s.loadWebAuthnSession(ctx, sessionToken, webauthnPurposeRegistration, now)
	if err != nil {
		return err
	}
	if stored.PlayerID != playerID {
		return ErrInvalidMFA
	}
	user, err := s.webAuthnUser(ctx, playerID)
	if err != nil {
		return err
	}
	w, err := newWebAuthn(rpID, origin)
	if err != nil {
		return err
	}
	credential, err := w.FinishRegistration(user, *session, requestWithBody(response))
	if err != nil {
		return fmt.Errorf("finish webauthn registration: %w", err)
	}
	credentialJSON, err := json.Marshal(credential)
	if err != nil {
		return fmt.Errorf("marshal webauthn credential: %w", err)
	}
	if err := s.sessions.CreateWebAuthnCredential(ctx, &WebAuthnCredential{PlayerID: playerID, CredentialIDHash: hashBytes(credential.ID), Name: "YubiKey", CredentialJSON: string(credentialJSON), CreatedAt: now, UpdatedAt: now}); err != nil {
		return fmt.Errorf("create webauthn credential: %w", err)
	}
	return s.sessions.MarkWebAuthnSessionUsed(ctx, stored.ID, now)
}

func (s *Service) BeginWebAuthnMFA(ctx context.Context, mfaChallenge, rpID, origin string) (*WebAuthnBeginLogin, error) {
	now := time.Now().UTC()
	challenge, err := s.sessions.FindMFAChallenge(ctx, hashToken(mfaChallenge), now)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrInvalidMFA
		}
		return nil, fmt.Errorf("find mfa challenge: %w", err)
	}
	user, err := s.webAuthnUser(ctx, challenge.PlayerID)
	if err != nil {
		return nil, err
	}
	if len(user.credentials) == 0 {
		return nil, ErrInvalidMFA
	}
	w, err := newWebAuthn(rpID, origin)
	if err != nil {
		return nil, err
	}
	options, session, err := w.BeginLogin(user, wan.WithLoginOrigin(origin))
	if err != nil {
		return nil, fmt.Errorf("begin webauthn login: %w", err)
	}
	token, err := s.storeWebAuthnSession(ctx, challenge.PlayerID, &challenge.ID, webauthnPurposeLogin, session)
	if err != nil {
		return nil, err
	}
	return &WebAuthnBeginLogin{Session: token, Options: options}, nil
}

func (s *Service) FinishWebAuthnMFA(ctx context.Context, webAuthnSessionToken string, response []byte, rpID, origin string) (string, *player.Player, error) {
	now := time.Now().UTC()
	stored, session, err := s.loadWebAuthnSession(ctx, webAuthnSessionToken, webauthnPurposeLogin, now)
	if err != nil {
		return "", nil, err
	}
	if stored.MFAChallengeID == nil {
		return "", nil, ErrInvalidMFA
	}
	user, err := s.webAuthnUser(ctx, stored.PlayerID)
	if err != nil {
		return "", nil, err
	}
	w, err := newWebAuthn(rpID, origin)
	if err != nil {
		return "", nil, err
	}
	credential, err := w.FinishLogin(user, *session, requestWithBody(response))
	if err != nil {
		return "", nil, fmt.Errorf("finish webauthn login: %w", err)
	}
	if err := s.updateWebAuthnCredential(ctx, stored.PlayerID, credential, now); err != nil {
		return "", nil, err
	}
	if err := s.sessions.MarkMFAChallengeUsed(ctx, *stored.MFAChallengeID, now); err != nil {
		return "", nil, fmt.Errorf("mark mfa challenge used: %w", err)
	}
	if err := s.sessions.MarkWebAuthnSessionUsed(ctx, stored.ID, now); err != nil {
		return "", nil, fmt.Errorf("mark webauthn session used: %w", err)
	}
	token, err := s.createSession(ctx, stored.PlayerID, now)
	if err != nil {
		return "", nil, err
	}
	return token, user.player, nil
}

func (s *Service) webAuthnUser(ctx context.Context, playerID int64) (webAuthnUser, error) {
	p, err := s.players.Get(ctx, playerID)
	if err != nil {
		return webAuthnUser{}, fmt.Errorf("get webauthn player: %w", err)
	}
	rows, err := s.sessions.ListWebAuthnCredentials(ctx, playerID)
	if err != nil {
		return webAuthnUser{}, fmt.Errorf("list webauthn credentials: %w", err)
	}
	credentials := make([]wan.Credential, 0, len(rows))
	for _, row := range rows {
		var credential wan.Credential
		if err := json.Unmarshal([]byte(row.CredentialJSON), &credential); err != nil {
			return webAuthnUser{}, fmt.Errorf("decode webauthn credential: %w", err)
		}
		credentials = append(credentials, credential)
	}
	return webAuthnUser{player: p, credentials: credentials}, nil
}

func (s *Service) storeWebAuthnSession(ctx context.Context, playerID int64, mfaChallengeID *int64, purpose string, session *wan.SessionData) (string, error) {
	token, err := newToken()
	if err != nil {
		return "", fmt.Errorf("generate webauthn session token: %w", err)
	}
	encoded, err := json.Marshal(session)
	if err != nil {
		return "", fmt.Errorf("marshal webauthn session: %w", err)
	}
	now := time.Now().UTC()
	if err := s.sessions.CreateWebAuthnSession(ctx, &WebAuthnSession{SessionHash: hashToken(token), PlayerID: playerID, MFAChallengeID: mfaChallengeID, Purpose: purpose, SessionJSON: string(encoded), CreatedAt: now, ExpiresAt: now.Add(webauthnSessionTTL)}); err != nil {
		return "", fmt.Errorf("create webauthn session: %w", err)
	}
	return token, nil
}

func (s *Service) loadWebAuthnSession(ctx context.Context, token, purpose string, now time.Time) (*WebAuthnSession, *wan.SessionData, error) {
	stored, err := s.sessions.FindWebAuthnSession(ctx, hashToken(token), purpose, now)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil, ErrInvalidMFA
		}
		return nil, nil, fmt.Errorf("find webauthn session: %w", err)
	}
	var session wan.SessionData
	if err := json.Unmarshal([]byte(stored.SessionJSON), &session); err != nil {
		return nil, nil, fmt.Errorf("decode webauthn session: %w", err)
	}
	return stored, &session, nil
}

func (s *Service) updateWebAuthnCredential(ctx context.Context, playerID int64, credential *wan.Credential, now time.Time) error {
	rows, err := s.sessions.ListWebAuthnCredentials(ctx, playerID)
	if err != nil {
		return fmt.Errorf("list webauthn credentials: %w", err)
	}
	credentialJSON, err := json.Marshal(credential)
	if err != nil {
		return fmt.Errorf("marshal webauthn credential: %w", err)
	}
	hash := hashBytes(credential.ID)
	for _, row := range rows {
		if row.CredentialIDHash == hash {
			row.CredentialJSON = string(credentialJSON)
			row.UpdatedAt = now
			row.LastUsedAt = &now
			if err := s.sessions.UpdateWebAuthnCredential(ctx, &row); err != nil {
				return fmt.Errorf("update webauthn credential: %w", err)
			}
			return nil
		}
	}
	return ErrInvalidMFA
}

func newWebAuthn(rpID, origin string) (*wan.WebAuthn, error) {
	return wan.New(&wan.Config{RPDisplayName: "Bratwurst des Schicksals", RPID: rpID, RPOrigins: []string{origin}, AuthenticatorSelection: protocol.AuthenticatorSelection{UserVerification: protocol.VerificationPreferred}})
}

func requestWithBody(body []byte) *http.Request {
	return &http.Request{Body: io.NopCloser(bytes.NewReader(body)), Header: http.Header{"Content-Type": []string{"application/json"}}}
}

func hashBytes(value []byte) string {
	sum := sha256.Sum256(value)
	return hex.EncodeToString(sum[:])
}
