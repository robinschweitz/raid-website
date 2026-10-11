package v1

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"

	"raid-website/internal/auth"
	"raid-website/internal/validation"
)

const sessionMaxAgeSeconds = 30 * 24 * 60 * 60
const csrfCookieName = "raid_csrf"

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
		if errors.Is(err, auth.ErrLoginRateLimited) {
			WriteError(w, http.StatusTooManyRequests, "login_rate_limited", "too many failed login attempts; please try again later")
			return
		}
		if errors.Is(err, auth.ErrInactive) {
			WriteError(w, http.StatusForbidden, "inactive", "account is inactive")
			return
		}
		if errors.Is(err, auth.ErrInvalidCredentials) {
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
	a.setSessionCookie(w, result.Token, sessionMaxAgeSeconds)
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
		if errors.Is(err, auth.ErrInvalidMFA) {
			WriteError(w, http.StatusUnauthorized, "invalid_mfa_code", "invalid MFA code")
			return
		}
		WriteError(w, http.StatusInternalServerError, "internal", "could not verify MFA")
		return
	}
	a.setSessionCookie(w, token, sessionMaxAgeSeconds)
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
		if errors.Is(err, auth.ErrInvalidCredentials) {
			WriteError(w, http.StatusUnauthorized, "invalid_credentials", "current password is invalid")
			return
		}
		WriteError(w, http.StatusInternalServerError, "internal", "could not disable TOTP")
		return
	}
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
		WriteError(w, http.StatusUnauthorized, "invalid_credentials", "current password is invalid")
		return
	}
	rpID, origin, ok := webAuthnRelyingParty(a.PublicOrigin, r, a.TrustProxyHeaders)
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
	rpID, origin, ok := webAuthnRelyingParty(a.PublicOrigin, r, a.TrustProxyHeaders)
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
	w.WriteHeader(http.StatusNoContent)
}

func (a *API) beginWebAuthnMFA(w http.ResponseWriter, r *http.Request) {
	var req webAuthnBeginLoginRequest
	if err := DecodeJSON(r, &req); err != nil {
		BadRequest(w, "malformed json body")
		return
	}
	rpID, origin, ok := webAuthnRelyingParty(a.PublicOrigin, r, a.TrustProxyHeaders)
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
	rpID, origin, ok := webAuthnRelyingParty(a.PublicOrigin, r, a.TrustProxyHeaders)
	if !ok {
		WriteError(w, http.StatusInternalServerError, "webauthn_origin_invalid", "WebAuthn origin is not configured")
		return
	}
	token, p, err := a.Auth.FinishWebAuthnMFA(r.Context(), req.Session, req.Response, rpID, origin)
	if err != nil {
		if errors.Is(err, auth.ErrInvalidMFA) {
			WriteError(w, http.StatusUnauthorized, "invalid_mfa_code", "invalid MFA challenge")
			return
		}
		WriteError(w, http.StatusUnprocessableEntity, "webauthn_invalid", "security key login failed")
		return
	}
	a.setSessionCookie(w, token, sessionMaxAgeSeconds)
	WriteJSON(w, http.StatusOK, map[string]any{"data": visiblePlayerPtr(p, p)})
}

func (a *API) logout(w http.ResponseWriter, r *http.Request) {
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
	if err := a.Auth.ChangeOwnPassword(r.Context(), actor.ID, req.CurrentPassword, req.NewPassword); err != nil {
		if errors.Is(err, auth.ErrInvalidCredentials) {
			WriteError(w, http.StatusUnauthorized, "invalid_credentials", "current password is invalid")
			return
		}
		if errors.Is(err, auth.ErrWeakPassword) {
			WriteError(w, http.StatusUnprocessableEntity, "password_too_short", "password must be at least 12 characters")
			return
		}
		WriteError(w, http.StatusInternalServerError, "internal", "could not change password")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *API) setAccount(w http.ResponseWriter, r *http.Request) {
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
	p, err := a.Auth.SetCredentials(r.Context(), id, req.Username, req.Password, role)
	if err != nil {
		if isNotFound(err) {
			WriteError(w, http.StatusNotFound, "not_found", "player not found")
			return
		}
		if errors.Is(err, auth.ErrWeakPassword) {
			WriteError(w, http.StatusUnprocessableEntity, "password_too_short", "password must be at least 12 characters")
			return
		}
		WriteError(w, http.StatusUnprocessableEntity, "invalid_request", err.Error())
		return
	}
	actor, _ := CurrentPlayer(r)
	WriteJSON(w, http.StatusOK, map[string]any{"data": visiblePlayerPtr(actor, p)})
}
