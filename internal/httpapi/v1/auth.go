package v1

import (
	"errors"
	"net/http"

	"raid-website/internal/auth"
	"raid-website/internal/validation"
)

const sessionMaxAgeSeconds = 30 * 24 * 60 * 60

type loginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
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
	token, p, err := a.Auth.Login(r.Context(), req.Username, req.Password)
	if err != nil {
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
		WriteError(w, http.StatusUnprocessableEntity, "invalid_request", err.Error())
		return
	}
	actor, _ := CurrentPlayer(r)
	WriteJSON(w, http.StatusOK, map[string]any{"data": visiblePlayerPtr(actor, p)})
}
