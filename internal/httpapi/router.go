package httpapi

import (
	"context"
	"fmt"
	"log"
	"net/http"

	"github.com/uptrace/bun"

	"raid-website/internal/accountmerge"
	"raid-website/internal/addons"
	"raid-website/internal/addonsync"
	"raid-website/internal/attendance"
	"raid-website/internal/audit"
	"raid-website/internal/auth"
	"raid-website/internal/bosskill"
	"raid-website/internal/character"
	"raid-website/internal/enhancepoints"
	v1 "raid-website/internal/httpapi/v1"
	"raid-website/internal/loot"
	"raid-website/internal/lootcatalog"
	"raid-website/internal/player"
	"raid-website/internal/poll"
	"raid-website/internal/push"
	"raid-website/internal/raid"
	"raid-website/internal/raidcompletion"
	"raid-website/internal/raidcorrection"
	"raid-website/internal/raidseries"
	"raid-website/internal/raidsignup"
	"raid-website/internal/recruitment"
	"raid-website/internal/registration"
	"raid-website/internal/roster"
	"raid-website/internal/shadowmourne"
	"raid-website/internal/statistics"
	"raid-website/internal/unclaimedloot"
	"raid-website/internal/webui"
)

// Options controls optional runtime behaviour of the HTTP layer.
type Options struct {
	TOTPEncryption        *auth.TOTPEncryption
	PrivacyContactName    string
	PrivacyContactAddress string
	CookieSecure          bool
	TrustProxyHeaders     bool
	PublicOrigin          string
	AdminUsername         string
	AdminPassword         string
	RegistrationFeed      registration.ExternalClient
	// VAPIDKey switches Web Push on (FR-32); nil leaves it off.
	VAPIDKey *push.VAPIDKey
	// PushSender replaces the HTTP push sender, for tests.
	PushSender push.Sender
}

// New constructs the HTTP handler tree with all v1 dependencies wired.
func New(db *bun.DB, opts Options) (http.Handler, error) {
	catalog := lootcatalog.NewService(lootcatalog.NewRepo(db))
	report, err := catalog.EnsureBundled(context.Background())
	if err != nil {
		return nil, fmt.Errorf("initialize bundled catalog: %w", err)
	}
	log.Printf("bundled catalog source=%s revision=%s hash=%s inserted=%d updated=%d retained=%d unchanged=%d warnings=%d (full report: loot_catalog_initializations)", report.Source.ID, report.Source.Revision, report.SourceHash, report.Inserted, report.Updated, report.Retained, report.Unchanged, len(report.Warnings))
	bundledAddons, err := addons.Load()
	if err != nil {
		return nil, fmt.Errorf("load bundled addons: %w", err)
	}
	for _, addon := range bundledAddons.List() {
		log.Printf("bundled addon %s version=%s files=%d zip_bytes=%d", addon.Name, addon.Version, len(addon.Files), addon.SizeBytes)
	}
	players := player.NewRepo(db)
	chars := character.NewRepo(db)
	raids := raid.NewRepo(db)
	att := attendance.NewRepo(db)
	lootRepo := loot.NewRepo(db)
	points := enhancepoints.NewRepo(db)
	rosterRepo := roster.NewRepo(db)
	polls := poll.NewRepo(db)
	audits := audit.NewRepo(db)

	authRepo := auth.NewRepo(db)
	if opts.TOTPEncryption != nil {
		authRepo = authRepo.WithTOTPEncryption(opts.TOTPEncryption)
		if err := authRepo.MigrateTOTPSecrets(context.Background()); err != nil {
			return nil, err
		}
	}
	authSvc := auth.NewService(players, authRepo)
	registrationFeed := opts.RegistrationFeed
	if registrationFeed == nil {
		registrationFeed = registration.RisingGodsClient{}
	}
	registrationSvc := registration.NewService(db, players, chars, audits, authSvc, registrationFeed)

	if err := authSvc.BootstrapAdmin(context.Background(), opts.AdminUsername, opts.AdminPassword); err != nil {
		return nil, fmt.Errorf("bootstrap admin: %w", err)
	}

	pushSvc := push.NewService(push.NewRepo(db), opts.VAPIDKey, log.Printf)
	if opts.PushSender != nil {
		pushSvc = pushSvc.WithSender(opts.PushSender)
	}
	corrections := raidcorrection.NewService(raidcorrection.NewRepo(db)).WithExternalClient(registrationFeed)
	api := v1.NewAPI(v1.Deps{
		DB:                db,
		Auth:              authSvc,
		CookieSecure:      opts.CookieSecure,
		TrustProxyHeaders: opts.TrustProxyHeaders,
		PublicOrigin:      opts.PublicOrigin,
		Players:           players,
		Characters:        chars,
		Raids:             raids,
		RaidSeries:        raidseries.NewRepo(db),
		Attendance:        att,
		Loot:              loot.NewService(lootRepo),
		LootRepo:          lootRepo,
		Points:            points,
		Polls:             polls,
		Roster:            rosterRepo,
		Audit:             audits,
		Completion:        raidcompletion.NewService(raidcompletion.NewRepo(db)),
		Corrections:       corrections,
		Signups:           raidsignup.NewService(raidsignup.NewRepo(db)),
		Recruitment:       recruitment.NewService(db),
		Shadowmourne:      shadowmourne.NewService(shadowmourne.NewRepo(db), chars),
		Registration:      registrationSvc,
		AccountMerge:      accountmerge.NewService(accountmerge.NewRepo(db)),
		Push:              pushSvc,
	})

	mux := http.NewServeMux()
	api.Register(mux)
	v1.CatalogHandlers{Catalog: catalog}.Register(mux)
	v1.AddonHandlers{Addons: bundledAddons}.Register(mux)
	v1.StatisticsHandlers{Statistics: statistics.NewService(statistics.NewRepo(db))}.Register(mux)
	v1.PrivacyHandlers{Contact: v1.PrivacyContact{Name: opts.PrivacyContactName, Address: opts.PrivacyContactAddress}}.Register(mux)
	unclaimed := unclaimedloot.NewService(unclaimedloot.NewRepo(db), raids, catalog)
	v1.UnclaimedHandlers{Unclaimed: unclaimed}.Register(mux)
	v1.BossKillHandlers{Kills: bosskill.NewService(bosskill.NewRepo(db)), Corrections: corrections}.Register(mux)
	v1.AddonSyncHandlers{AddonSync: addonsync.NewService(addonsync.NewRepo(db), raids, raidseries.NewRepo(db), att, chars, players, catalog, lootRepo, points, corrections, unclaimed), Roster: api}.Register(mux)
	mux.HandleFunc("GET /health", v1.Health)
	if spa, err := webui.Handler(); err == nil {
		mux.Handle("/", spa)
	}

	return requestID(securityHeaders(opts.CookieSecure)(logging(csrfProtection(opts.TrustProxyHeaders)(authenticate(authSvc)(mux))))), nil
}
