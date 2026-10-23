package v1

import (
	"database/sql"
	"errors"
	"net/http"
	"strconv"

	"github.com/uptrace/bun"

	"raid-website/internal/accountmerge"
	"raid-website/internal/attendance"
	"raid-website/internal/audit"
	"raid-website/internal/auth"
	"raid-website/internal/character"
	"raid-website/internal/enhancepoints"
	"raid-website/internal/loot"
	"raid-website/internal/player"
	"raid-website/internal/poll"
	"raid-website/internal/raid"
	"raid-website/internal/raidcompletion"
	"raid-website/internal/raidcorrection"
	"raid-website/internal/raidseries"
	"raid-website/internal/raidsignup"
	"raid-website/internal/recruitment"
	"raid-website/internal/registration"
	"raid-website/internal/roster"
	"raid-website/internal/shadowmourne"
	"raid-website/internal/validation"
	"raid-website/internal/wishlist"
)

// Deps holds the dependencies shared by the v1 HTTP handlers.
type Deps struct {
	DB                *bun.DB
	Auth              *auth.Service
	CookieSecure      bool
	TrustProxyHeaders bool
	PublicOrigin      string
	Players           *player.Repo
	Characters        *character.Repo
	Raids             *raid.Repo
	RaidSeries        *raidseries.Repo
	Attendance        *attendance.Repo
	Loot              *loot.Service
	LootRepo          *loot.Repo
	Points            *enhancepoints.Repo
	Polls             *poll.Repo
	Roster            *roster.Repo
	Audit             *audit.Repo
	Completion        *raidcompletion.Service
	Corrections       *raidcorrection.Service
	Signups           *raidsignup.Service
	Recruitment       *recruitment.Service
	Shadowmourne      *shadowmourne.Service
	Registration      *registration.Service
	AccountMerge      *accountmerge.Service
}

// API holds the dependencies shared by the v1 HTTP handlers.
type API struct {
	Deps
	pointService *enhancepoints.Service
}

func NewAPI(d Deps) *API {
	return &API{Deps: d, pointService: enhancepoints.NewService(d.Points)}
}

func (a *API) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/health", Health)
	mux.HandleFunc("GET /api/v1/auth/csrf", a.csrfToken)
	mux.HandleFunc("POST /api/v1/auth/login", a.login)
	mux.HandleFunc("POST /api/v1/auth/mfa/verify", a.verifyMFA)
	mux.HandleFunc("GET /api/v1/auth/mfa", RequireAuth(a.mfaStatus))
	mux.HandleFunc("POST /api/v1/auth/mfa/totp/setup", RequireAuth(a.setupTOTP))
	mux.HandleFunc("POST /api/v1/auth/mfa/totp/enable", RequireAuth(a.enableTOTP))
	mux.HandleFunc("POST /api/v1/auth/mfa/totp/disable", RequireAuth(a.disableTOTP))
	mux.HandleFunc("POST /api/v1/auth/mfa/webauthn/register/begin", RequireAuth(a.beginWebAuthnRegistration))
	mux.HandleFunc("POST /api/v1/auth/mfa/webauthn/register/finish", RequireAuth(a.finishWebAuthnRegistration))
	mux.HandleFunc("POST /api/v1/auth/mfa/webauthn/credentials/{id}/delete", RequireAuth(a.deleteWebAuthnCredential))
	mux.HandleFunc("POST /api/v1/auth/mfa/webauthn/login/begin", a.beginWebAuthnMFA)
	mux.HandleFunc("POST /api/v1/auth/mfa/webauthn/login/finish", a.finishWebAuthnMFA)
	mux.HandleFunc("POST /api/v1/auth/logout", RequireAuth(a.logout))
	mux.HandleFunc("GET /api/v1/auth/me", RequireAuth(a.me))
	mux.HandleFunc("POST /api/v1/auth/password", RequireAuth(a.changeOwnPassword))
	mux.HandleFunc("GET /api/v1/external/rising-gods/characters", a.searchRisingGodsCharacters)
	mux.HandleFunc("POST /api/v1/auth/registration-requests", a.createRegistrationRequest)
	mux.HandleFunc("POST /api/v1/auth/register-with-code", a.registerWithCode)
	mux.HandleFunc("GET /api/v1/registration-requests", RequireRole(validation.RoleAdmin, a.listRegistrationRequests))
	mux.HandleFunc("GET /api/v1/registration-requests/{id}/codes", RequireRole(validation.RoleAdmin, a.listRegistrationCodes))
	mux.HandleFunc("POST /api/v1/registration-requests/{id}/code", RequireRole(validation.RoleAdmin, a.createRegistrationCode))
	mux.HandleFunc("POST /api/v1/registration-requests/{id}/reject", RequireRole(validation.RoleAdmin, a.rejectRegistrationRequest))
	mux.HandleFunc("POST /api/v1/registration-codes/{id}/sent", RequireRole(validation.RoleAdmin, a.markRegistrationCodeSent))
	mux.HandleFunc("POST /api/v1/registration-codes/{id}/revoke", RequireRole(validation.RoleAdmin, a.revokeRegistrationCode))
	mux.HandleFunc("GET /api/v1/audit-log", RequireRole(validation.RoleAdmin, a.listAuditLog))
	mux.HandleFunc("GET /api/v1/shadowmourne", RequireAuth(a.listShadowmourne))
	mux.HandleFunc("POST /api/v1/shadowmourne", RequireRole(validation.RoleRaidassistent, a.createShadowmourneEntry))
	mux.HandleFunc("PATCH /api/v1/shadowmourne/{id}", RequireRole(validation.RoleRaidassistent, a.updateShadowmourneProgress))
	mux.HandleFunc("POST /api/v1/shadowmourne/{id}/complete", RequireRole(validation.RoleRaidassistent, a.completeShadowmourneEntry))
	mux.HandleFunc("POST /api/v1/shadowmourne/{id}/remove", RequireRole(validation.RoleRaidassistent, a.removeShadowmourneEntry))
	mux.HandleFunc("POST /api/v1/shadowmourne/{id}/move", RequireRole(validation.RoleRaidassistent, a.moveShadowmourneEntry))
	mux.HandleFunc("GET /api/v1/raids/{id}/shadowmourne", RequireAuth(a.getRaidShadowmourne))
	mux.HandleFunc("PUT /api/v1/raids/{id}/shadowmourne/{task}", RequireRole(validation.RoleRaidassistent, a.assignRaidShadowmourneTask))
	mux.HandleFunc("GET /api/v1/player-accounts", RequireRole(validation.RoleAdmin, a.listPlayerAccounts))
	mux.HandleFunc("POST /api/v1/characters/claim-requests", RequireAuth(a.createCharacterClaimRequest))
	mux.HandleFunc("POST /api/v1/characters/claim-with-code", RequireAuth(a.claimCharacterWithCode))

	mux.HandleFunc("GET /api/v1/players", a.listPlayers)
	mux.HandleFunc("POST /api/v1/players", RequireRole(validation.RoleAdmin, a.createPlayer))
	mux.HandleFunc("GET /api/v1/players/{id}", RequireAuth(a.getPlayer))
	mux.HandleFunc("PUT /api/v1/players/{id}/account", RequireRole(validation.RoleAdmin, a.setAccount))
	mux.HandleFunc("POST /api/v1/players/{id}/merge", RequireRole(validation.RoleAdmin, a.mergePlayer))
	mux.HandleFunc("GET /api/v1/players/{id}/characters", RequireAuth(a.listPlayerCharacters))
	mux.HandleFunc("POST /api/v1/players/{id}/characters", RequireRole(validation.RoleAdmin, a.createCharacter))

	mux.HandleFunc("GET /api/v1/characters/{id}", RequireAuth(a.getCharacter))
	mux.HandleFunc("GET /api/v1/characters", RequireAuth(a.listCharacters))
	mux.HandleFunc("GET /api/v1/character-options", RequireAuth(a.characterOptions))
	mux.HandleFunc("POST /api/v1/raids/{id}/quick-add", RequireRole(validation.RoleRaidassistent, a.quickAddCharacter))
	mux.HandleFunc("PATCH /api/v1/characters/{id}", RequireRole(validation.RoleAdmin, a.updateCharacter))
	mux.HandleFunc("POST /api/v1/characters/{id}/transfer", RequireRole(validation.RoleAdmin, a.transferCharacter))
	mux.HandleFunc("POST /api/v1/characters/{id}/merge", RequireRole(validation.RoleAdmin, a.mergeCharacter))
	mux.HandleFunc("POST /api/v1/characters/{id}/retry-link", RequireRole(validation.RoleAdmin, a.retryCharacterLink))
	mux.HandleFunc("POST /api/v1/characters/{id}/reload-profile", RequireRole(validation.RoleAdmin, a.reloadCharacterProfile))
	mux.HandleFunc("GET /api/v1/characters/{id}/name-history", RequireAuth(a.listCharacterNameHistory))

	mux.HandleFunc("GET /api/v1/raids", a.listRaids)
	mux.HandleFunc("POST /api/v1/raids", RequireRole(validation.RoleRaidassistent, a.createRaid))
	mux.HandleFunc("GET /api/v1/raids/current", RequireAuth(a.currentRaid))
	mux.HandleFunc("GET /api/v1/raids/{id}", RequireAuth(a.getRaid))
	mux.HandleFunc("PATCH /api/v1/raids/{id}", RequireRole(validation.RoleRaidassistent, a.configureRaidSchedule))
	mux.HandleFunc("POST /api/v1/raids/{id}/cancel", RequireRole(validation.RoleRaidassistent, a.cancelRaid))
	mux.HandleFunc("GET /api/v1/raids/{id}/archive-preview", RequireRole(validation.RoleRaidassistent, a.archivePreview))
	mux.HandleFunc("POST /api/v1/raids/{id}/archive", RequireRole(validation.RoleRaidassistent, a.archiveRaid))
	mux.HandleFunc("GET /api/v1/raid-series", RequireAuth(a.listRaidSeries))
	mux.HandleFunc("POST /api/v1/raid-series", RequireRole(validation.RoleAdmin, a.createRaidSeries))
	mux.HandleFunc("PATCH /api/v1/raid-series/{id}", RequireRole(validation.RoleAdmin, a.updateRaidSeries))
	mux.HandleFunc("POST /api/v1/raid-series/{id}/plan", RequireRole(validation.RoleRaidassistent, a.planRaidSeries))

	mux.HandleFunc("GET /api/v1/raids/{id}/attendance", a.listAttendance)
	mux.HandleFunc("GET /api/v1/raids/{id}/recruitment-template", RequireRole(validation.RoleRaidassistent, a.getRecruitmentTemplate))
	mux.HandleFunc("PUT /api/v1/raids/{id}/recruitment-template", RequireRole(validation.RoleRaidassistent, a.saveRecruitmentTemplate))
	mux.HandleFunc("GET /api/v1/raids/{id}/signups", RequireAuth(a.getSignups))
	mux.HandleFunc("PUT /api/v1/raids/{id}/signups/availability", RequireRole(validation.RoleRaidassistent, a.setSignupAvailability))
	mux.HandleFunc("PUT /api/v1/raids/{id}/signups", RequireRole(validation.RoleMember, a.saveSignups))
	mux.HandleFunc("POST /api/v1/raids/{id}/signups/{signupID}/withdraw", RequireRole(validation.RoleMember, a.withdrawSignup))
	mux.HandleFunc("PUT /api/v1/raids/{id}/absences/{playerID}", RequireRole(validation.RoleMember, a.declineAbsence))
	mux.HandleFunc("DELETE /api/v1/raids/{id}/absences/{playerID}", RequireRole(validation.RoleMember, a.revokeAbsence))
	mux.HandleFunc("POST /api/v1/raids/{id}/signups/{signupID}/accept", RequireRole(validation.RoleRaidassistent, a.acceptSignup))
	mux.HandleFunc("POST /api/v1/raids/{id}/attendance", RequireRole(validation.RoleRaidassistent, a.createAttendance))
	mux.HandleFunc("PATCH /api/v1/raids/{id}/attendance/{recordID}", RequireRole(validation.RoleRaidassistent, a.correctAttendance))
	mux.HandleFunc("DELETE /api/v1/raids/{id}/attendance/{recordID}", RequireRole(validation.RoleRaidassistent, a.removeAttendance))

	mux.HandleFunc("GET /api/v1/raids/{id}/loot", RequireAuth(a.listLoot))
	mux.HandleFunc("POST /api/v1/raids/{id}/loot", RequireRole(validation.RoleRaidassistent, a.createLoot))
	mux.HandleFunc("PATCH /api/v1/raids/{id}/loot/{recordID}", RequireRole(validation.RoleRaidassistent, a.correctLoot))
	mux.HandleFunc("POST /api/v1/raids/{id}/loot/{recordID}/withdraw", RequireRole(validation.RoleRaidassistent, a.withdrawLoot))

	mux.HandleFunc("GET /api/v1/players/{id}/points", RequireAuth(a.getPoints))
	mux.HandleFunc("GET /api/v1/players/{id}/loot", RequireAuth(a.listPlayerLoot))

	mux.HandleFunc("GET /api/v1/players/{id}/locks", RequireAuth(a.listLocks))
	mux.HandleFunc("POST /api/v1/players/{id}/locks", RequireRole(validation.RoleMember, a.createLock))
	mux.HandleFunc("POST /api/v1/players/{id}/locks/{lockID}/cancel", RequireRole(validation.RoleMember, a.cancelLock))

	mux.HandleFunc("GET /api/v1/guild-ranks", RequireAuth(a.listGuildRanks))
	mux.HandleFunc("POST /api/v1/guild-ranks", RequireRole(validation.RoleAdmin, a.createGuildRank))
	mux.HandleFunc("PATCH /api/v1/guild-ranks/{id}", RequireRole(validation.RoleAdmin, a.updateGuildRank))

	mux.HandleFunc("GET /api/v1/raid-groups", RequireAuth(a.listRaidGroups))
	mux.HandleFunc("POST /api/v1/raid-groups", RequireRole(validation.RoleAdmin, a.createRaidGroup))
	mux.HandleFunc("PATCH /api/v1/raid-groups/{id}", RequireRole(validation.RoleAdmin, a.updateRaidGroup))

	mux.HandleFunc("GET /api/v1/polls", a.listPolls)
	mux.HandleFunc("POST /api/v1/polls", requirePollRole(validation.RoleRaidassistent, a.createPoll))
	mux.HandleFunc("GET /api/v1/polls/{id}", RequireAuth(a.getPoll))
	mux.HandleFunc("POST /api/v1/polls/{id}/vote", requirePollRole(validation.RoleMember, a.votePoll))
	mux.HandleFunc("GET /api/v1/polls/{id}/votes", RequireAuth(a.listPollVotes))
	mux.HandleFunc("POST /api/v1/polls/{id}/close", requirePollRole(validation.RoleRaidassistent, a.closePoll))

	WishlistHandlers{Wishlist: wishlist.NewService(a.DB)}.Register(mux)

}

func pathID(r *http.Request, name string) (int64, bool) {
	raw := r.PathValue(name)
	if raw == "" {
		return 0, false
	}
	id, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || id <= 0 {
		return 0, false
	}
	return id, true
}

func isNotFound(err error) bool {
	return errors.Is(err, sql.ErrNoRows)
}
