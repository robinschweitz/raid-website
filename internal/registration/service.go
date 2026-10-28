package registration

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base32"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/uptrace/bun"

	"raid-website/internal/audit"
	"raid-website/internal/auth"
	"raid-website/internal/character"
	"raid-website/internal/player"
	"raid-website/internal/validation"
)

const codeTTL = 14 * 24 * time.Hour

var (
	ErrInvalid          = errors.New("registration request invalid")
	ErrInvalidCode      = errors.New("registration code invalid or expired")
	ErrForbidden        = errors.New("registration action forbidden")
	ErrUsernameTaken    = errors.New("username already in use")
	ErrRequestNotFound  = errors.New("registration request not found")
	ErrCodeNotFound     = errors.New("registration code not found")
	ErrProfileRequired  = errors.New("rising-gods profile required")
	ErrProfileUnusable  = errors.New("rising-gods profile could not be used")
	ErrProfileLinked    = errors.New("rising-gods profile already linked")
	ErrProfileAmbiguous = errors.New("multiple rising-gods profiles match character name")
	ErrProfileMissing   = errors.New("character has no rising-gods profile")
	ErrRequestExists    = errors.New("registration request already exists for profile")

	ErrMatchChoiceRequired = errors.New("name match needs an admin choice")
	ErrMatchChoiceInvalid  = errors.New("name match choice invalid")
)

type Service struct {
	visitors   visitorLimiter
	db         *bun.DB
	repo       *Repo
	players    *player.Repo
	characters *character.Repo
	audit      *audit.Repo
	auth       *auth.Service
	external   ExternalClient
}

func NewService(db *bun.DB, players *player.Repo, characters *character.Repo, audits *audit.Repo, authSvc *auth.Service, external ExternalClient) *Service {
	if external == nil {
		external = RisingGodsClient{}
	}
	return &Service{db: db, repo: NewRepo(db), players: players, characters: characters, audit: audits, auth: authSvc, external: external}
}

type GeneratedCode struct {
	CodeRecord Code   `json:"code_record"`
	PlainCode  string `json:"code"`
}

type LinkedCharacterInput struct {
	PlayerID  int64
	ProfileID string
	IsMain    bool
	ActorID   *int64
	Reason    string
}

func (s *Service) SearchCharacters(ctx context.Context, query string) ([]ExternalCharacter, error) {
	return s.external.SearchCharacters(ctx, query)
}

func (s *Service) LoadCharacterProfile(ctx context.Context, profileID string) (ExternalCharacter, string, error) {
	return s.loadUsableProfile(ctx, profileID)
}

func (s *Service) CreateLinkedCharacter(ctx context.Context, input LinkedCharacterInput) (*character.Character, error) {
	profile, raw, err := s.loadUsableProfile(ctx, input.ProfileID)
	if err != nil {
		return nil, err
	}
	if input.PlayerID <= 0 {
		return nil, ErrInvalid
	}
	now := time.Now().UTC()
	var created *character.Character
	err = s.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		players := s.players.WithTx(tx)
		chars := s.characters.WithTx(tx)
		if _, err := players.Get(ctx, input.PlayerID); err != nil {
			return err
		}
		c, err := chars.FindByExternalProfile(ctx, ExternalSourceRisingGods, profile.ProfileID)
		if err == nil {
			if c.PlayerID == input.PlayerID {
				created = c
				return nil
			}
			return ErrProfileLinked
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		c = characterFromProfile(input.PlayerID, profile, raw, now)
		c.IsMain = input.IsMain
		if err := chars.Create(ctx, c); err != nil {
			return err
		}
		created = c
		if input.ActorID != nil {
			return audit.NewRepo(tx).Insert(ctx, &audit.Log{ActorID: input.ActorID, EntityType: "character", EntityID: fmt.Sprint(c.ID), Action: "create_linked_character", AfterJSON: mustJSON(c), Reason: input.Reason, CreatedAt: now})
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return created, nil
}

func (s *Service) RetryCharacterLink(ctx context.Context, actorID, characterID int64) (*character.Character, error) {
	actor, err := s.players.Get(ctx, actorID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, character.ErrInvalidActor
	}
	if err != nil {
		return nil, err
	}
	if !actor.Role.AtLeast(validation.RoleAdmin) {
		return nil, character.ErrForbidden
	}
	existing, err := s.characters.Get(ctx, characterID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, character.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if existing.ExternalProfileID != "" || existing.ExternalSource != "" {
		return nil, ErrProfileLinked
	}
	// Existing characters do not yet have a profile ID. Rising-Gods profile
	// slugs are deterministic, so resolve the exact profile instead of using a
	// broad list search for every retry.
	matches, err := s.external.SearchCharacters(ctx, "eu.rising-gods."+strings.ToLower(strings.TrimSpace(existing.Name)))
	if errors.Is(err, ErrExternalNotFound) {
		return nil, ErrProfileRequired
	}
	if errors.Is(err, ErrExternalInvalid) {
		return nil, ErrProfileUnusable
	}
	if err != nil {
		return nil, err
	}
	var profileID string
	for _, match := range matches {
		if !strings.EqualFold(match.Name, existing.Name) {
			continue
		}
		if profileID != "" && profileID != match.ProfileID {
			return nil, ErrProfileAmbiguous
		}
		profileID = match.ProfileID
	}
	if profileID == "" {
		return nil, ErrProfileRequired
	}
	profile, raw, err := s.loadUsableProfile(ctx, profileID)
	if err != nil {
		return nil, err
	}
	if !strings.EqualFold(profile.Name, existing.Name) {
		return nil, ErrProfileUnusable
	}
	var linked *character.Character
	err = s.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		chars := s.characters.WithTx(tx)
		c, err := chars.Get(ctx, characterID)
		if err != nil {
			return err
		}
		if c.ExternalProfileID != "" || c.ExternalSource != "" || !strings.EqualFold(c.Name, profile.Name) {
			return ErrProfileLinked
		}
		if other, err := chars.FindByExternalProfile(ctx, ExternalSourceRisingGods, profile.ProfileID); err == nil {
			if other.ID != c.ID {
				return ErrProfileLinked
			}
		} else if !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		before := mustJSON(c)
		now := time.Now().UTC()
		c.ExternalSource = ExternalSourceRisingGods
		c.ExternalProfileID = profile.ProfileID
		c.ExternalProfileURL = profile.ProfileURL
		c.ExternalProfileJSON = raw
		c.MainSpec = profile.MainSpec
		c.SecondSpec = profile.SecondSpec
		c.GearScore = profile.GearScore
		c.UpdatedAt = now
		if err := chars.Update(ctx, c); err != nil {
			return err
		}
		if err := audit.NewRepo(tx).Insert(ctx, &audit.Log{ActorID: &actorID, EntityType: "character", EntityID: fmt.Sprint(c.ID), Action: "link_rising_gods_profile", BeforeJSON: before, AfterJSON: mustJSON(c), Reason: "Manueller Rising-Gods-Verknüpfungsversuch", CreatedAt: now}); err != nil {
			return err
		}
		linked = c
		return nil
	})
	return linked, err
}

func (s *Service) ReloadLinkedCharacter(ctx context.Context, actorID, characterID int64) (*character.Character, error) {
	var updated *character.Character
	err := s.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		players := s.players.WithTx(tx)
		chars := s.characters.WithTx(tx)
		actor, err := players.Get(ctx, actorID)
		if errors.Is(err, sql.ErrNoRows) {
			return character.ErrInvalidActor
		}
		if err != nil {
			return fmt.Errorf("load reload actor: %w", err)
		}
		if !actor.Role.AtLeast(validation.RoleAdmin) {
			return character.ErrForbidden
		}
		c, err := chars.Get(ctx, characterID)
		if errors.Is(err, sql.ErrNoRows) {
			return character.ErrNotFound
		}
		if err != nil {
			return fmt.Errorf("load character for profile reload: %w", err)
		}
		if c.ExternalSource != ExternalSourceRisingGods || strings.TrimSpace(c.ExternalProfileID) == "" {
			return ErrProfileMissing
		}
		profile, raw, err := s.loadUsableProfile(ctx, c.ExternalProfileID)
		if err != nil {
			return err
		}
		before := mustJSON(c)
		now := time.Now().UTC()
		c.MainSpec = profile.MainSpec
		c.SecondSpec = profile.SecondSpec
		c.GearScore = profile.GearScore
		c.ExternalProfileURL = profile.ProfileURL
		c.ExternalProfileJSON = raw
		c.UpdatedAt = now
		if err := chars.Update(ctx, c); err != nil {
			return fmt.Errorf("update reloaded character: %w", err)
		}
		if err := audit.NewRepo(tx).Insert(ctx, &audit.Log{ActorID: &actorID, EntityType: "character", EntityID: fmt.Sprint(c.ID), Action: "reload_rising_gods_profile", BeforeJSON: before, AfterJSON: mustJSON(c), Reason: "Rising-Gods-Profil neu geladen", CreatedAt: now}); err != nil {
			return fmt.Errorf("audit profile reload: %w", err)
		}
		updated = c
		return nil
	})
	if err != nil {
		return nil, err
	}
	return updated, nil
}

func (s *Service) loadUsableProfile(ctx context.Context, profileID string) (ExternalCharacter, string, error) {
	profile, _, err := s.external.LoadCharacter(ctx, profileID)
	if err != nil {
		return ExternalCharacter{}, "", fmt.Errorf("load rising-gods profile: %w: %w", ErrProfileUnusable, err)
	}
	if profile.ProfileID == "" || profile.Name == "" || profile.Class == "" || profile.MainSpec == "" {
		return ExternalCharacter{}, "", ErrProfileRequired
	}
	return profile, "", nil
}

// requestAudit is the readable audit snapshot of a registration request; the
// raw Rising-Gods profile JSON is left out (ADR 077).
type requestAudit struct {
	CharacterName string `json:"character_name"`
	Class         string `json:"class"`
	Status        string `json:"status"`
	Purpose       string `json:"purpose"`
}

func requestSnapshot(req *Request) requestAudit {
	return requestAudit{CharacterName: req.CharacterName, Class: req.Class, Status: req.Status, Purpose: req.Purpose}
}

func actorOrSystem(actorID *int64) int64 {
	if actorID == nil {
		return 0
	}
	return *actorID
}

func (s *Service) CreateRequest(ctx context.Context, requesterID *int64, profileID string) (*Request, error) {
	if existing, err := s.repo.FindBlockingRequest(ctx, ExternalSourceRisingGods, profileID); err != nil {
		return nil, err
	} else if existing != nil {
		return nil, ErrRequestExists
	}
	profile, _, err := s.loadUsableProfile(ctx, profileID)
	if err != nil {
		return nil, err
	}
	purpose := PurposeNewAccount
	if requesterID != nil {
		purpose = PurposeClaim
	}
	now := time.Now().UTC()
	req := &Request{RequesterPlayerID: requesterID, Status: StatusPending, Purpose: purpose, ExternalSource: ExternalSourceRisingGods, ExternalProfileID: profile.ProfileID, ExternalProfileURL: profile.ProfileURL, CharacterName: profile.Name, Class: profile.Class, MainSpec: profile.MainSpec, SecondSpec: profile.SecondSpec, GearScore: profile.GearScore, CreatedAt: now, UpdatedAt: now}
	err = s.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		if err := NewRepo(tx).CreateRequest(ctx, req); err != nil {
			return fmt.Errorf("create registration request: %w", err)
		}
		return audit.NewRepo(tx).Record(ctx, actorOrSystem(requesterID), "registration_request", req.ID, "create", nil, requestSnapshot(req), "")
	})
	if err != nil {
		return nil, err
	}
	return req, nil
}

// ListRequests returns requests; redeemed ones name the player they were used for (ADR 084).
func (s *Service) ListRequests(ctx context.Context, statuses []string) ([]Request, error) {
	rows, err := s.repo.ListRequests(ctx, statuses)
	if err != nil {
		return nil, err
	}
	redemptions, err := s.repo.ListRedemptions(ctx)
	if err != nil {
		return nil, err
	}
	byRequest := make(map[int64]Redemption, len(redemptions))
	for _, redemption := range redemptions {
		byRequest[redemption.RequestID] = redemption
	}
	players, err := s.playersByID(ctx)
	if err != nil {
		return nil, err
	}
	for i := range rows {
		if rows[i].Status == StatusPending || rows[i].Status == StatusApproved {
			match, err := s.findNameMatch(ctx, s.players, s.characters, &rows[i])
			if err != nil {
				return nil, err
			}
			rows[i].NameMatch = match
		}
		redemption, ok := byRequest[rows[i].ID]
		if !ok {
			continue
		}
		p, ok := players[redemption.UsedByPlayerID]
		if !ok {
			continue
		}
		rows[i].Linked = &LinkedPlayer{PlayerID: p.ID, PlayerName: p.Name, Username: p.Username, UsedAt: redemption.UsedAt}
	}
	return rows, nil
}

// ListAccounts returns every player with their login state, how it came about
// and their characters, so an Admin sees who has claimed which player (ADR 084).
func (s *Service) ListAccounts(ctx context.Context) ([]Account, error) {
	players, err := s.players.List(ctx)
	if err != nil {
		return nil, err
	}
	characters, err := s.characters.List(ctx)
	if err != nil {
		return nil, err
	}
	redemptions, err := s.repo.ListRedemptions(ctx)
	if err != nil {
		return nil, err
	}
	charactersByPlayer := make(map[int64][]AccountCharacter, len(players))
	for _, c := range characters {
		charactersByPlayer[c.PlayerID] = append(charactersByPlayer[c.PlayerID], AccountCharacter{ID: c.ID, Name: c.Name, Class: c.Class, IsMain: c.IsMain, Active: c.Active})
	}
	// The first redeemed new-account code is when the player got their login.
	firstRegistration := make(map[int64]Redemption, len(redemptions))
	for _, redemption := range redemptions {
		if redemption.Purpose != PurposeNewAccount {
			continue
		}
		if _, seen := firstRegistration[redemption.UsedByPlayerID]; !seen {
			firstRegistration[redemption.UsedByPlayerID] = redemption
		}
	}
	accounts := make([]Account, 0, len(players))
	for _, p := range players {
		account := Account{PlayerID: p.ID, PlayerName: p.Name, Active: p.Active, Role: string(p.Role), Username: p.Username, Origin: AccountNone, Characters: charactersByPlayer[p.ID]}
		if account.Characters == nil {
			account.Characters = []AccountCharacter{}
		}
		hasLogin := p.Username != nil && p.PasswordHash != ""
		if redemption, ok := firstRegistration[p.ID]; ok && hasLogin {
			usedAt := redemption.UsedAt
			requestID := redemption.RequestID
			account.Origin = AccountRegistered
			account.RegisteredAt = &usedAt
			account.RegisteredCharacter = redemption.CharacterName
			account.RequestID = &requestID
		} else if hasLogin {
			account.Origin = AccountByAdmin
		}
		accounts = append(accounts, account)
	}
	return accounts, nil
}

func (s *Service) playersByID(ctx context.Context) (map[int64]player.Player, error) {
	players, err := s.players.List(ctx)
	if err != nil {
		return nil, err
	}
	byID := make(map[int64]player.Player, len(players))
	for _, p := range players {
		byID[p.ID] = p
	}
	return byID, nil
}

func (s *Service) ListCodesByRequest(ctx context.Context, requestID int64) ([]Code, error) {
	codes, err := s.repo.ListCodesByRequest(ctx, requestID)
	if err != nil {
		return nil, err
	}
	for i := range codes {
		if codes[i].UsedByPlayerID == nil {
			continue
		}
		p, err := s.players.Get(ctx, *codes[i].UsedByPlayerID)
		if errors.Is(err, sql.ErrNoRows) {
			continue
		}
		if err != nil {
			return nil, err
		}
		codes[i].UsedByUsername = p.Username
	}
	return codes, nil
}

// findNameMatch returns the player without a login that owns a character with
// the requested name, when no character is linked to the request's profile.
// Only an Admin decision turns such a match into an attached login (SC-049).
func (s *Service) findNameMatch(ctx context.Context, players *player.Repo, chars *character.Repo, req *Request) (*NameMatch, error) {
	if req.Purpose != PurposeNewAccount || req.RequesterPlayerID != nil {
		return nil, nil
	}
	if _, err := chars.FindByExternalProfile(ctx, req.ExternalSource, req.ExternalProfileID); err == nil {
		return nil, nil
	} else if !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}
	candidates, err := chars.ListByName(ctx, req.CharacterName)
	if err != nil {
		return nil, err
	}
	for _, c := range candidates {
		p, err := players.Get(ctx, c.PlayerID)
		if errors.Is(err, sql.ErrNoRows) {
			continue
		}
		if err != nil {
			return nil, err
		}
		if p.Username == nil && p.PasswordHash == "" {
			return &NameMatch{PlayerID: p.ID, PlayerName: p.Name}, nil
		}
	}
	return nil, nil
}

// codeAudit is the audit snapshot of a created code: the request state and the
// Admin's name-match choice, never the code itself (SC-031).
type codeAudit struct {
	requestAudit
	MatchChoice   string `json:"match_choice"`
	MatchPlayerID *int64 `json:"match_player_id"`
}

func (s *Service) CreateCode(ctx context.Context, actorID, requestID int64, choice MatchChoiceInput) (*GeneratedCode, error) {
	req, err := s.repo.GetRequest(ctx, requestID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrRequestNotFound
		}
		return nil, err
	}
	if req.Status == StatusRejected || req.Status == StatusConsumed {
		return nil, ErrInvalid
	}
	plain, hash, err := newCode()
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	code := &Code{RequestID: requestID, CodeHash: hash, CreatedByPlayerID: actorID, CreatedAt: now, ExpiresAt: now.Add(codeTTL)}
	err = s.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		repo := NewRepo(tx)
		match, err := s.findNameMatch(ctx, s.players.WithTx(tx), s.characters.WithTx(tx), req)
		if err != nil {
			return err
		}
		switch {
		case match == nil && choice.Choice != "":
			return ErrMatchChoiceInvalid
		case match == nil:
		case choice.Choice == "":
			return ErrMatchChoiceRequired
		case choice.Choice == MatchNew:
			code.MatchChoice = MatchNew
		case choice.Choice == MatchAttach && choice.PlayerID == match.PlayerID:
			code.MatchChoice = MatchAttach
			code.MatchPlayerID = &match.PlayerID
		default:
			return ErrMatchChoiceInvalid
		}
		before := requestSnapshot(req)
		req.Status = StatusApproved
		req.UpdatedAt = now
		if err := repo.UpdateRequest(ctx, req); err != nil {
			return err
		}
		if err := repo.CreateCode(ctx, code); err != nil {
			return err
		}
		return audit.NewRepo(tx).Record(ctx, actorID, "registration_request", req.ID, "create_code", before, codeAudit{requestSnapshot(req), code.MatchChoice, code.MatchPlayerID}, "")
	})
	if err != nil {
		return nil, err
	}
	return &GeneratedCode{CodeRecord: *code, PlainCode: plain}, nil
}

func (s *Service) RevokeCode(ctx context.Context, actorID, codeID int64) (*Code, error) {
	code, err := s.repo.GetCode(ctx, codeID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrCodeNotFound
		}
		return nil, err
	}
	before := *code
	now := time.Now().UTC()
	code.RevokedAt = &now
	err = s.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		if err := NewRepo(tx).UpdateCode(ctx, code); err != nil {
			return err
		}
		return audit.NewRepo(tx).Record(ctx, actorID, "registration_code", code.ID, "revoke", before, code, "")
	})
	if err != nil {
		return nil, err
	}
	return code, nil
}

func (s *Service) MarkCodeSent(ctx context.Context, actorID, codeID int64) (*Code, error) {
	code, err := s.repo.GetCode(ctx, codeID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrCodeNotFound
		}
		return nil, err
	}
	if code.SentAt == nil {
		before := *code
		now := time.Now().UTC()
		code.SentAt = &now
		err = s.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
			if err := NewRepo(tx).UpdateCode(ctx, code); err != nil {
				return err
			}
			return audit.NewRepo(tx).Record(ctx, actorID, "registration_code", code.ID, "mark_sent", before, code, "")
		})
		if err != nil {
			return nil, err
		}
	}
	return code, nil
}

func (s *Service) RejectRequest(ctx context.Context, actorID, requestID int64) (*Request, error) {
	req, err := s.repo.GetRequest(ctx, requestID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrRequestNotFound
		}
		return nil, err
	}
	if req.Status == StatusConsumed {
		return nil, ErrInvalid
	}
	before := requestSnapshot(req)
	req.Status = StatusRejected
	req.UpdatedAt = time.Now().UTC()
	err = s.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		if err := NewRepo(tx).UpdateRequest(ctx, req); err != nil {
			return err
		}
		return audit.NewRepo(tx).Record(ctx, actorID, "registration_request", req.ID, "reject", before, requestSnapshot(req), "")
	})
	if err != nil {
		return nil, err
	}
	return req, nil
}

func (s *Service) RedeemNewAccount(ctx context.Context, codeValue, username, password string) (string, *player.Player, error) {
	uname := strings.ToLower(strings.TrimSpace(username))
	if uname == "" || strings.TrimSpace(password) == "" {
		return "", nil, ErrInvalid
	}
	if err := auth.ValidatePassword(password); err != nil {
		return "", nil, err
	}
	var created *player.Player
	err := s.consumeCode(ctx, codeValue, func(ctx context.Context, tx bun.Tx, req *Request, code *Code, now time.Time) (int64, error) {
		if req.RequesterPlayerID != nil || req.Purpose != PurposeNewAccount {
			return 0, ErrForbidden
		}
		hash, err := auth.HashPassword(ctx, password)
		if err != nil {
			return 0, err
		}
		players := s.players.WithTx(tx)
		chars := s.characters.WithTx(tx)
		// SC-046: the username is checked only after the code is valid, and inside
		// the code transaction so a concurrent redeem cannot slip in between.
		if _, err := players.FindByUsername(ctx, uname); err == nil {
			return 0, ErrUsernameTaken
		} else if !errors.Is(err, sql.ErrNoRows) {
			return 0, err
		}
		if existing, err := chars.FindByExternalProfile(ctx, req.ExternalSource, req.ExternalProfileID); err == nil {
			p, err := players.Get(ctx, existing.PlayerID)
			if err != nil {
				return 0, err
			}
			if p.Username != nil || p.PasswordHash != "" {
				return 0, ErrProfileLinked
			}
			p.Username = &uname
			p.PasswordHash = hash
			p.Role = memberAtLeast(p.Role)
			p.UpdatedAt = now
			if err := players.Update(ctx, p); err != nil {
				return 0, err
			}
			created = p
			return p.ID, nil
		} else if !errors.Is(err, sql.ErrNoRows) {
			return 0, err
		}
		// SC-049: a character name alone never attaches a login. Only the player
		// the Admin confirmed when creating the code is used.
		if code.MatchChoice == MatchAttach && code.MatchPlayerID != nil {
			p, err := players.Get(ctx, *code.MatchPlayerID)
			if errors.Is(err, sql.ErrNoRows) {
				return 0, ErrInvalid
			}
			if err != nil {
				return 0, err
			}
			if p.Username != nil || p.PasswordHash != "" {
				return 0, ErrProfileLinked
			}
			p.Username = &uname
			p.PasswordHash = hash
			p.Role = memberAtLeast(p.Role)
			p.UpdatedAt = now
			if err := players.Update(ctx, p); err != nil {
				return 0, err
			}
			created = p
			return p.ID, nil
		}
		// FR-4.5: an account from a registration code is a MEMBER, also when the
		// login is attached to an existing player above (ADR 103). Existing GUEST
		// accounts with a login keep their role (FR-4.10); only an Admin promotes them.
		p := &player.Player{Name: req.CharacterName, Username: &uname, PasswordHash: hash, Role: validation.RoleMember, Active: true, CreatedAt: now, UpdatedAt: now}
		if err := players.Create(ctx, p); err != nil {
			return 0, err
		}
		created = p
		// The new player gets a character of its own; an existing character of the
		// same name stays with its owner (SC-049).
		if err := chars.Create(ctx, characterFromRequest(p.ID, req, now)); err != nil {
			return 0, err
		}
		return p.ID, nil
	})
	if err != nil {
		return "", nil, err
	}
	token, p, err := s.auth.Login(ctx, uname, password)
	if err != nil {
		return "", nil, err
	}
	if p == nil {
		p = created
	}
	return token, p, nil
}

func (s *Service) RedeemClaim(ctx context.Context, actorID int64, codeValue string) (*character.Character, error) {
	var claimed *character.Character
	err := s.consumeCode(ctx, codeValue, func(ctx context.Context, tx bun.Tx, req *Request, code *Code, now time.Time) (int64, error) {
		if req.Purpose != PurposeClaim || req.RequesterPlayerID == nil || *req.RequesterPlayerID != actorID {
			return 0, ErrForbidden
		}
		chars := s.characters.WithTx(tx)
		if err := s.attachCharacter(ctx, tx, chars, actorID, req, &actorID, now); err != nil {
			return 0, err
		}
		c, err := chars.FindByName(ctx, req.CharacterName)
		if err != nil {
			return 0, err
		}
		claimed = c
		return actorID, nil
	})
	if err != nil {
		return nil, err
	}
	return claimed, nil
}

func (s *Service) consumeCode(ctx context.Context, codeValue string, apply func(context.Context, bun.Tx, *Request, *Code, time.Time) (int64, error)) error {
	hash := hashCode(codeValue)
	if hash == "" {
		return ErrInvalidCode
	}
	now := time.Now().UTC()
	return s.db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		repo := NewRepo(tx)
		row, err := repo.FindActiveCodeByHash(ctx, hash, now)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return ErrInvalidCode
			}
			return err
		}
		if row.Request == nil || row.Request.Status == StatusRejected || row.Request.Status == StatusConsumed {
			return ErrInvalidCode
		}
		usedBy, err := apply(ctx, tx, row.Request, &row.Code, now)
		if err != nil {
			return err
		}
		row.UsedAt = &now
		row.UsedByPlayerID = &usedBy
		if err := repo.UpdateCode(ctx, &row.Code); err != nil {
			return err
		}
		before := requestSnapshot(row.Request)
		row.Request.Status = StatusConsumed
		row.Request.UpdatedAt = now
		if err := repo.UpdateRequest(ctx, row.Request); err != nil {
			return err
		}
		return audit.NewRepo(tx).Record(ctx, usedBy, "registration_request", row.Request.ID, "redeem", before, requestSnapshot(row.Request), "")
	})
}

func (s *Service) attachCharacter(ctx context.Context, tx bun.Tx, chars *character.Repo, playerID int64, req *Request, actorID *int64, now time.Time) error {
	if existing, err := chars.FindByExternalProfile(ctx, req.ExternalSource, req.ExternalProfileID); err == nil {
		if existing.PlayerID != playerID {
			return ErrProfileLinked
		}
		return nil
	} else if !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	c, err := chars.FindByName(ctx, req.CharacterName)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	if errors.Is(err, sql.ErrNoRows) {
		c = characterFromRequest(playerID, req, now)
		return chars.Create(ctx, c)
	}
	if c.PlayerID == playerID {
		return nil
	}
	before, _ := json.Marshal(c)
	c.PlayerID = playerID
	c.Class = req.Class
	c.MainSpec = req.MainSpec
	c.SecondSpec = req.SecondSpec
	c.GearScore = req.GearScore
	c.Active = true
	c.ExternalSource = req.ExternalSource
	c.ExternalProfileID = req.ExternalProfileID
	c.ExternalProfileURL = req.ExternalProfileURL
	c.ExternalProfileJSON = req.RawProfileJSON
	c.UpdatedAt = now
	if err := chars.Update(ctx, c); err != nil {
		return err
	}
	if actorID != nil {
		after, _ := json.Marshal(c)
		return audit.NewRepo(tx).Insert(ctx, &audit.Log{ActorID: actorID, EntityType: "character", EntityID: fmt.Sprint(c.ID), Action: "claim_character", BeforeJSON: string(before), AfterJSON: string(after), Reason: "registration code character claim", CreatedAt: now})
	}
	return nil
}

func characterFromProfile(playerID int64, profile ExternalCharacter, raw string, now time.Time) *character.Character {
	return &character.Character{PlayerID: playerID, Name: profile.Name, Class: profile.Class, MainSpec: profile.MainSpec, SecondSpec: profile.SecondSpec, GearScore: profile.GearScore, Active: true, ExternalSource: ExternalSourceRisingGods, ExternalProfileID: profile.ProfileID, ExternalProfileURL: profile.ProfileURL, ExternalProfileJSON: raw, CreatedAt: now, UpdatedAt: now}
}

func characterFromRequest(playerID int64, req *Request, now time.Time) *character.Character {
	return &character.Character{PlayerID: playerID, Name: req.CharacterName, Class: req.Class, MainSpec: req.MainSpec, SecondSpec: req.SecondSpec, GearScore: req.GearScore, Active: true, ExternalSource: req.ExternalSource, ExternalProfileID: req.ExternalProfileID, ExternalProfileURL: req.ExternalProfileURL, ExternalProfileJSON: req.RawProfileJSON, CreatedAt: now, UpdatedAt: now}
}

func mustJSON(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}

func newCode() (string, string, error) {
	raw := make([]byte, 10)
	if _, err := rand.Read(raw); err != nil {
		return "", "", err
	}
	encoded := base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(raw)
	plain := "BD-" + encoded[:4] + "-" + encoded[4:8] + "-" + encoded[8:12]
	return plain, hashCode(plain), nil
}

func hashCode(code string) string {
	normalized := strings.ToUpper(strings.ReplaceAll(strings.TrimSpace(code), " ", ""))
	if normalized == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(normalized))
	return hex.EncodeToString(sum[:])
}

// memberAtLeast returns MEMBER for a player without a login who gets one from a
// registration code (FR-4.5, ADR 103); a higher role stays as it is.
func memberAtLeast(role validation.Role) validation.Role {
	if role.AtLeast(validation.RoleMember) {
		return role
	}
	return validation.RoleMember
}
