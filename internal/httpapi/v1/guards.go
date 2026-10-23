package v1

import (
	"net/http"

	"raid-website/internal/validation"
)

// RequireAuth rejects unauthenticated requests with 401.
func RequireAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if _, ok := CurrentPlayer(r); !ok {
			WriteError(w, http.StatusUnauthorized, "unauthenticated", "login required")
			return
		}
		next(w, r)
	}
}

// RequireRole rejects requests whose authenticated player lacks the minimum role.
func RequireRole(min validation.Role, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		p, ok := CurrentPlayer(r)
		if !ok {
			WriteError(w, http.StatusUnauthorized, "unauthenticated", "login required")
			return
		}
		if !p.Role.AtLeast(min) {
			WriteError(w, http.StatusForbidden, "forbidden", "insufficient permissions")
			return
		}
		// Admin-only routes need a second factor (FR-3.10, SC-040). MFA setup itself is
		// behind RequireAuth, so an Admin without MFA can still set it up.
		if min == validation.RoleAdmin && !adminHasSecondFactor(r) {
			WriteError(w, http.StatusForbidden, "mfa_setup_required", "admin requires a second factor")
			return
		}
		next(w, r)
	}
}
