package v1

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"raid-website/internal/auth"
	"raid-website/internal/player"
	"raid-website/internal/validation"
)

const sessionMaxAgeSeconds = 30 * 24 * 60 * 60
const csrfCookieName = "raid_csrf"

// writePasswordBusy answers 503 when no password hashing slot became free in
// time (SC-005). The client may simply try again.
func writePasswordBusy(w http.ResponseWriter, err error) bool {
	if !errors.Is(err, auth.ErrPasswordHashBusy) {
		return false
	}
	w.Header().Set("Retry-After", "5")
	WriteError(w, http.StatusServiceUnavailable, "server_busy", "Der Server ist gerade ausgelastet. Bitte versuche es in ein paar Sekunden erneut.")
	return true
}

// passwordTooLongMessage is shown for passwords above the 1024-byte limit (SC-005).
// Multibyte characters count several bytes, which is why the hint says so.
const passwordTooLongMessage = "Das Passwort darf höchstens 1024 Bytes lang sein. Umlaute und Sonderzeichen zählen mehrfach."

type loginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type mfaVerifyRequest struct {
	Challenge string `json:"challenge"`
	Code      string `json:"code"`
}

type mfaEnableRequest struct {
	Code string `json:"code"`
}

type passwordConfirmRequest struct {
	CurrentPassword string `json:"current_password"`
}

type passwordChangeRequest struct {
	CurrentPassword string `json:"current_password"`
	NewPassword     string `json:"new_password"`
}

type deleteWebAuthnRequest struct {
	CurrentPassword string `json:"current_password"`
}

type webAuthnBeginLoginRequest struct {
	Challenge string `json:"challenge"`
}

type webAuthnFinishRequest struct {
	Session  string          `json:"session"`
	Response json.RawMessage `json:"response"`
}

type accountRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
	Role     string `json:"role"`
}

func (a *API) setSessionCookie(w http.ResponseWriter, token string, maxAge int) {
	http.SetCookie(w, &http.Cookie{
		Name:     auth.CookieName,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   a.CookieSecure,
		MaxAge:   maxAge,
	})
}

// startSession hands the new session cookie to the browser. First it removes
// push subscriptions of other players on this device, so the last player's
// opt-in never reaches the one signing in now (FR-32.3, ADR 103); if that
// fails, the browser gets no session. A browser without a device cookie gets
// one here; which subscription it holds only the browser knows, so the web
// client checks in before it shows the player as signed in.
func (a *API) startSession(w http.ResponseWriter, r *http.Request, token string, playerID int64) bool {
	hadDevice := deviceHash(r) != ""
	hash, err := a.ensureDevice(w, r)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "internal", "could not create login session")
		return false
	}
	if hadDevice {
		if err := a.Push.ClaimDevice(r.Context(), playerID, hash); err != nil {
			WriteError(w, http.StatusInternalServerError, "internal", "could not create login session")
			return false
		}
	}
	a.setSessionCookie(w, token, sessionMaxAgeSeconds)
	return true
}

func (a *API) csrfToken(w http.ResponseWriter, r *http.Request) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		WriteError(w, http.StatusInternalServerError, "internal", "could not create CSRF token")
		return
	}
	token := base64.RawURLEncoding.EncodeToString(b)
	http.SetCookie(w, &http.Cookie{Name: csrfCookieName, Value: token, Path: "/", SameSite: http.SameSiteLaxMode, Secure: a.CookieSecure, MaxAge: sessionMaxAgeSeconds})
	WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]string{"token": token}})
}

func (a *API) login(w http.ResponseWriter, r *http.Request) {
	var req loginRequest
	if err := DecodeJSON(r, &req); err != nil {
		BadRequest(w, "malformed json body")
		return
	}
	if req.Username == "" || req.Password == "" {
		BadRequest(w, "username and password are required")
		return
	}
	result, err := a.Auth.StartLogin(r.Context(), req.Username, req.Password, clientAddress(r, a.TrustProxyHeaders))
	if err != nil {
		if writePasswordBusy(w, err) {
			return
		}
		if errors.Is(err, auth.ErrLoginRateLimited) {
			a.securityEvent(w, r, "login_blocked", slog.String("username", normalisedUsername(req.Username)))
			WriteError(w, http.StatusTooManyRequests, "login_rate_limited", "too many failed login attempts; please try again later")
			return
		}
		if errors.Is(err, auth.ErrInactive) {
			a.securityEvent(w, r, "login_failed", slog.String("username", normalisedUsername(req.Username)), slog.String("reason", "inactive"))
			WriteError(w, http.StatusForbidden, "inactive", "account is inactive")
			return
		}
		if errors.Is(err, auth.ErrInvalidCredentials) {
			a.securityEvent(w, r, "login_failed", slog.String("username", normalisedUsername(req.Username)), slog.String("reason", "invalid_credentials"))
			WriteError(w, http.StatusUnauthorized, "invalid_credentials", "invalid username or password")
			return
		}
		WriteError(w, http.StatusInternalServerError, "internal", "could not create login session")
		return
	}
	if result.MFAChallenge != "" {
		WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]any{"mfa_required": true, "challenge": result.MFAChallenge, "factors": result.MFAFactors}})
		return
	}
	if !a.startSession(w, r, result.Token, result.Player.ID) {
		return
	}
	WriteJSON(w, http.StatusOK, map[string]any{"data": visiblePlayerPtr(result.Player, result.Player)})
}

func (a *API) verifyMFA(w http.ResponseWriter, r *http.Request) {
	var req mfaVerifyRequest
	if err := DecodeJSON(r, &req); err != nil {
		BadRequest(w, "malformed json body")
		return
	}
	token, p, err := a.Auth.VerifyMFAChallenge(r.Context(), req.Challenge, req.Code)
	if err != nil {
		if errors.Is(err, auth.ErrMFAChallengeInvalid) {
			a.securityEvent(w, r, "mfa_challenge_invalid", playerAttrs(err)...)
			WriteError(w, http.StatusUnauthorized, "mfa_challenge_invalid", "MFA challenge is no longer valid; log in again")
			return
		}
		if errors.Is(err, auth.ErrInvalidMFA) {
			a.securityEvent(w, r, "mfa_failed", append(playerAttrs(err), slog.String("method", "code"))...)
			WriteError(w, http.StatusUnauthorized, "invalid_mfa_code", "invalid MFA code")
			return
		}
		WriteError(w, http.StatusInternalServerError, "internal", "could not verify MFA")
		return
	}
	if !a.startSession(w, r, token, p.ID) {
		return
	}
	WriteJSON(w, http.StatusOK, map[string]any{"data": visiblePlayerPtr(p, p)})
}

func (a *API) setupTOTP(w http.ResponseWriter, r *http.Request) {
	actor, ok := CurrentPlayer(r)
	if !ok {
		WriteError(w, http.StatusUnauthorized, "unauthenticated", "login required")
		return
	}
	var req passwordConfirmRequest
	if err := DecodeJSON(r, &req); err != nil {
		BadRequest(w, "malformed json body")
		return
	}
	if err := a.Auth.VerifyPassword(r.Context(), actor.ID, req.CurrentPassword); err != nil {
		if writePasswordBusy(w, err) {
			return
		}
		WriteError(w, http.StatusUnauthorized, "invalid_credentials", "current password is invalid")
		return
	}
	setup, err := a.Auth.SetupTOTP(r.Context(), actor.ID, "Bratwurst des Schicksals")
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "internal", "could not start MFA setup")
		return
	}
	WriteJSON(w, http.StatusOK, map[string]any{"data": setup})
}

func (a *API) enableTOTP(w http.ResponseWriter, r *http.Request) {
	actor, ok := CurrentPlayer(r)
	if !ok {
		WriteError(w, http.StatusUnauthorized, "unauthenticated", "login required")
		return
	}
	var req mfaEnableRequest
	if err := DecodeJSON(r, &req); err != nil {
		BadRequest(w, "malformed json body")
		return
	}
	codes, err := a.Auth.EnableTOTP(r.Context(), actor.ID, req.Code)
	if err != nil {
		if errors.Is(err, auth.ErrInvalidMFA) {
			WriteError(w, http.StatusUnauthorized, "invalid_mfa_code", "invalid MFA code")
			return
		}
		WriteError(w, http.StatusInternalServerError, "internal", "could not enable MFA")
		return
	}
	a.securityEvent(w, r, "mfa_enabled", slog.Int64("player_id", actor.ID), slog.String("method", "totp"))
	WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]any{"recovery_codes": codes}})
}

func (a *API) mfaStatus(w http.ResponseWriter, r *http.Request) {
	actor, ok := CurrentPlayer(r)
	if !ok {
		WriteError(w, http.StatusUnauthorized, "unauthenticated", "login required")
		return
	}
	status, err := a.Auth.MFAStatus(r.Context(), actor.ID)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "internal", "could not load MFA status")
		return
	}
	WriteJSON(w, http.StatusOK, map[string]any{"data": status})
}

func (a *API) disableTOTP(w http.ResponseWriter, r *http.Request) {
	actor, ok := CurrentPlayer(r)
	if !ok {
		WriteError(w, http.StatusUnauthorized, "unauthenticated", "login required")
		return
	}
	var req passwordConfirmRequest
	if err := DecodeJSON(r, &req); err != nil {
		BadRequest(w, "malformed json body")
		return
	}
	if err := a.Auth.DisableTOTP(r.Context(), actor.ID, req.CurrentPassword); err != nil {
		if writePasswordBusy(w, err) {
			return
		}
		if errors.Is(err, auth.ErrInvalidCredentials) {
			WriteError(w, http.StatusUnauthorized, "invalid_credentials", "current password is invalid")
			return
		}
		WriteError(w, http.StatusInternalServerError, "internal", "could not disable TOTP")
		return
	}
	a.securityEvent(w, r, "mfa_disabled", slog.Int64("player_id", actor.ID), slog.String("method", "totp"))
	w.WriteHeader(http.StatusNoContent)
}

func (a *API) beginWebAuthnRegistration(w http.ResponseWriter, r *http.Request) {
	actor, ok := CurrentPlayer(r)
	if !ok {
		WriteError(w, http.StatusUnauthorized, "unauthenticated", "login required")
		return
	}
	var req passwordConfirmRequest
	if err := DecodeJSON(r, &req); err != nil {
		BadRequest(w, "malformed json body")
		return
	}
	if err := a.Auth.VerifyPassword(r.Context(), actor.ID, req.CurrentPassword); err != nil {
		if writePasswordBusy(w, err) {
			return
		}
		WriteError(w, http.StatusUnauthorized, "invalid_credentials", "current password is invalid")
		return
	}
	rpID, origin, ok := webAuthnRelyingParty(a.PublicOrigin, r)
	if !ok {
		WriteError(w, http.StatusInternalServerError, "webauthn_origin_invalid", "WebAuthn origin is not configured")
		return
	}
	result, err := a.Auth.BeginWebAuthnRegistration(r.Context(), actor.ID, rpID, origin)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "internal", "could not start security key registration")
		return
	}
	WriteJSON(w, http.StatusOK, map[string]any{"data": result})
}

func (a *API) finishWebAuthnRegistration(w http.ResponseWriter, r *http.Request) {
	actor, ok := CurrentPlayer(r)
	if !ok {
		WriteError(w, http.StatusUnauthorized, "unauthenticated", "login required")
		return
	}
	var req webAuthnFinishRequest
	if err := DecodeJSON(r, &req); err != nil {
		BadRequest(w, "malformed json body")
		return
	}
	rpID, origin, ok := webAuthnRelyingParty(a.PublicOrigin, r)
	if !ok {
		WriteError(w, http.StatusInternalServerError, "webauthn_origin_invalid", "WebAuthn origin is not configured")
		return
	}
	if err := a.Auth.FinishWebAuthnRegistration(r.Context(), actor.ID, req.Session, req.Response, rpID, origin); err != nil {
		if errors.Is(err, auth.ErrInvalidMFA) {
			WriteError(w, http.StatusUnauthorized, "invalid_mfa_code", "invalid MFA challenge")
			return
		}
		WriteError(w, http.StatusUnprocessableEntity, "webauthn_invalid", "security key registration failed")
		return
	}
	a.securityEvent(w, r, "mfa_enabled", slog.Int64("player_id", actor.ID), slog.String("method", "webauthn"))
	WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]bool{"enabled": true}})
}

func (a *API) deleteWebAuthnCredential(w http.ResponseWriter, r *http.Request) {
	actor, ok := CurrentPlayer(r)
	if !ok {
		WriteError(w, http.StatusUnauthorized, "unauthenticated", "login required")
		return
	}
	id, ok := pathID(r, "id")
	if !ok {
		BadRequest(w, "invalid credential id")
		return
	}
	var req deleteWebAuthnRequest
	if err := DecodeJSON(r, &req); err != nil {
		BadRequest(w, "malformed json body")
		return
	}
	if err := a.Auth.DeleteWebAuthnCredential(r.Context(), actor.ID, id, req.CurrentPassword); err != nil {
		if writePasswordBusy(w, err) {
			return
		}
		if errors.Is(err, auth.ErrInvalidCredentials) {
			WriteError(w, http.StatusUnauthorized, "invalid_credentials", "current password is invalid")
			return
		}
		if errors.Is(err, auth.ErrInvalidMFA) || isNotFound(err) {
			WriteError(w, http.StatusNotFound, "not_found", "security key not found")
			return
		}
		WriteError(w, http.StatusInternalServerError, "internal", "could not remove security key")
		return
	}
	a.securityEvent(w, r, "mfa_disabled", slog.Int64("player_id", actor.ID), slog.String("method", "webauthn"))
	w.WriteHeader(http.StatusNoContent)
}

func (a *API) beginWebAuthnMFA(w http.ResponseWriter, r *http.Request) {
	var req webAuthnBeginLoginRequest
	if err := DecodeJSON(r, &req); err != nil {
		BadRequest(w, "malformed json body")
		return
	}
	rpID, origin, ok := webAuthnRelyingParty(a.PublicOrigin, r)
	if !ok {
		WriteError(w, http.StatusInternalServerError, "webauthn_origin_invalid", "WebAuthn origin is not configured")
		return
	}
	result, err := a.Auth.BeginWebAuthnMFA(r.Context(), req.Challenge, rpID, origin)
	if err != nil {
		if errors.Is(err, auth.ErrInvalidMFA) {
			WriteError(w, http.StatusUnauthorized, "invalid_mfa_code", "invalid MFA challenge")
			return
		}
		WriteError(w, http.StatusInternalServerError, "internal", "could not start security key login")
		return
	}
	WriteJSON(w, http.StatusOK, map[string]any{"data": result})
}

func (a *API) finishWebAuthnMFA(w http.ResponseWriter, r *http.Request) {
	var req webAuthnFinishRequest
	if err := DecodeJSON(r, &req); err != nil {
		BadRequest(w, "malformed json body")
		return
	}
	rpID, origin, ok := webAuthnRelyingParty(a.PublicOrigin, r)
	if !ok {
		WriteError(w, http.StatusInternalServerError, "webauthn_origin_invalid", "WebAuthn origin is not configured")
		return
	}
	token, p, err := a.Auth.FinishWebAuthnMFA(r.Context(), req.Session, req.Response, rpID, origin)
	if err != nil {
		if errors.Is(err, auth.ErrInvalidMFA) {
			a.securityEvent(w, r, "mfa_failed", append(playerAttrs(err), slog.String("method", "webauthn"))...)
			WriteError(w, http.StatusUnauthorized, "invalid_mfa_code", "invalid MFA challenge")
			return
		}
		if _, known := auth.PlayerIDFromError(err); known {
			a.securityEvent(w, r, "mfa_failed", append(playerAttrs(err), slog.String("method", "webauthn"))...)
		}
		WriteError(w, http.StatusUnprocessableEntity, "webauthn_invalid", "security key login failed")
		return
	}
	if !a.startSession(w, r, token, p.ID) {
		return
	}
	WriteJSON(w, http.StatusOK, map[string]any{"data": visiblePlayerPtr(p, p)})
}

func (a *API) logout(w http.ResponseWriter, r *http.Request) {
	// Signing out ends this player's notifications on this device (FR-32.3).
	if actor, ok := ActorID(r); ok {
		if hash := deviceHash(r); hash != "" {
			if err := a.Push.ForgetDevice(r.Context(), actor, hash); err != nil {
				WriteError(w, http.StatusInternalServerError, "internal", "could not end session")
				return
			}
		}
	}
	if c, err := r.Cookie(auth.CookieName); err == nil && c.Value != "" {
		if err := a.Auth.Logout(r.Context(), c.Value); err != nil {
			WriteError(w, http.StatusInternalServerError, "internal", "could not end session")
			return
		}
	}
	a.setSessionCookie(w, "", -1)
	w.WriteHeader(http.StatusNoContent)
}

func (a *API) me(w http.ResponseWriter, r *http.Request) {
	p, ok := CurrentPlayer(r)
	if !ok {
		WriteError(w, http.StatusUnauthorized, "unauthenticated", "login required")
		return
	}
	WriteJSON(w, http.StatusOK, map[string]any{"data": visiblePlayerPtr(p, p)})
}

func (a *API) changeOwnPassword(w http.ResponseWriter, r *http.Request) {
	actor, ok := CurrentPlayer(r)
	if !ok {
		WriteError(w, http.StatusUnauthorized, "unauthenticated", "login required")
		return
	}
	var req passwordChangeRequest
	if err := DecodeJSON(r, &req); err != nil {
		BadRequest(w, "malformed json body")
		return
	}
	currentToken := ""
	if c, err := r.Cookie(auth.CookieName); err == nil {
		currentToken = c.Value
	}
	if err := a.Auth.ChangeOwnPassword(r.Context(), actor.ID, currentToken, deviceHash(r), req.CurrentPassword, req.NewPassword); err != nil {
		if writePasswordBusy(w, err) {
			return
		}
		if errors.Is(err, auth.ErrInvalidCredentials) {
			WriteError(w, http.StatusUnauthorized, "invalid_credentials", "current password is invalid")
			return
		}
		if errors.Is(err, auth.ErrWeakPassword) {
			WriteError(w, http.StatusUnprocessableEntity, "password_too_short", "password must be at least 12 characters")
			return
		}
		if errors.Is(err, auth.ErrPasswordTooLong) {
			WriteError(w, http.StatusUnprocessableEntity, "password_too_long", passwordTooLongMessage)
			return
		}
		WriteError(w, http.StatusInternalServerError, "internal", "could not change password")
		return
	}
	a.securityEvent(w, r, "password_changed", slog.Int64("player_id", actor.ID))
	w.WriteHeader(http.StatusNoContent)
}

func (a *API) setAccount(w http.ResponseWriter, r *http.Request) {
	actorID, _ := ActorID(r)
	id, ok := pathID(r, "id")
	if !ok {
		BadRequest(w, "invalid player id")
		return
	}
	var req accountRequest
	if err := DecodeJSON(r, &req); err != nil {
		BadRequest(w, "malformed json body")
		return
	}
	role := validation.Role(req.Role)
	if req.Username == "" {
		BadRequest(w, "username is required")
		return
	}
	if !role.Valid() {
		BadRequest(w, "invalid role")
		return
	}
	p, err := a.Auth.SetCredentials(r.Context(), id, req.Username, req.Password, role, actorID)
	if err != nil {
		if writePasswordBusy(w, err) {
			return
		}
		if isNotFound(err) {
			WriteError(w, http.StatusNotFound, "not_found", "player not found")
			return
		}
		if errors.Is(err, player.ErrLastAdmin) {
			WriteError(w, http.StatusConflict, "last_admin", lastAdminMessage)
			return
		}
		if errors.Is(err, auth.ErrWeakPassword) {
			WriteError(w, http.StatusUnprocessableEntity, "password_too_short", "password must be at least 12 characters")
			return
		}
		if errors.Is(err, auth.ErrPasswordTooLong) {
			WriteError(w, http.StatusUnprocessableEntity, "password_too_long", passwordTooLongMessage)
			return
		}
		WriteError(w, http.StatusUnprocessableEntity, "invalid_request", err.Error())
		return
	}
	a.securityEvent(w, r, "account_changed", slog.Int64("player_id", id), slog.Int64("actor_id", actorID),
		slog.Bool("password_set", req.Password != ""), slog.String("role", string(role)))
	WriteJSON(w, http.StatusOK, map[string]any{"data": visiblePlayerPtr(accountViewer(r), p)})
}
