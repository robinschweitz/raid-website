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
	ErrInvalid         = errors.New("registration request invalid")
	ErrInvalidCode     = errors.New("registration code invalid or expired")
	ErrForbidden       = errors.New("registration action forbidden")
	ErrUsernameTaken   = errors.New("username already in use")
	ErrRequestNotFound = errors.New("registration request not found")
	ErrCodeNotFound    = errors.New("registration code not found")
	ErrProfileRequired = errors.New("rising-gods profile required")
	ErrProfileUnusable = errors.New("rising-gods profile could not be used")
)

type Service struct {
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

func (s *Service) SearchCharacters(ctx context.Context, query string) ([]ExternalCharacter, error) {
	return s.external.SearchCharacters(ctx, query)
}

func (s *Service) CreateRequest(ctx context.Context, requesterID *int64, profileID string) (*Request, error) {
	profile, raw, err := s.external.LoadCharacter(ctx, profileID)
	if err != nil {
		return nil, fmt.Errorf("load rising-gods profile: %w", ErrProfileUnusable)
	}
	if profile.ProfileID == "" || profile.Name == "" || profile.Class == "" || profile.MainSpec == "" {
		return nil, ErrProfileRequired
	}
	purpose := PurposeNewAccount
	if requesterID != nil {
		purpose = PurposeClaim
	}
	now := time.Now().UTC()
	req := &Request{RequesterPlayerID: requesterID, Status: StatusPending, Purpose: purpose, ExternalSource: ExternalSourceRisingGods, ExternalProfileID: profile.ProfileID, ExternalProfileURL: profile.ProfileURL, CharacterName: profile.Name, Class: profile.Class, MainSpec: profile.MainSpec, SecondSpec: profile.SecondSpec, Level: profile.Level, Race: profile.Race, Faction: profile.Faction, RawProfileJSON: raw, CreatedAt: now, UpdatedAt: now}
	if err := s.repo.CreateRequest(ctx, req); err != nil {
		return nil, fmt.Errorf("create registration request: %w", err)
	}
	return req, nil
}

func (s *Service) ListRequests(ctx context.Context) ([]Request, error) {
	return s.repo.ListRequests(ctx)
}

func (s *Service) ListCodesByRequest(ctx context.Context, requestID int64) ([]Code, error) {
	return s.repo.ListCodesByRequest(ctx, requestID)
}

func (s *Service) CreateCode(ctx context.Context, actorID, requestID int64) (*GeneratedCode, error) {
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
		req.Status = StatusApproved
		req.UpdatedAt = now
		if err := repo.UpdateRequest(ctx, req); err != nil {
			return err
		}
		return repo.CreateCode(ctx, code)
	})
	if err != nil {
		return nil, err
	}
	return &GeneratedCode{CodeRecord: *code, PlainCode: plain}, nil
}

func (s *Service) RevokeCode(ctx context.Context, codeID int64) (*Code, error) {
	code, err := s.repo.GetCode(ctx, codeID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrCodeNotFound
		}
		return nil, err
	}
	now := time.Now().UTC()
	code.RevokedAt = &now
	if err := s.repo.UpdateCode(ctx, code); err != nil {
		return nil, err
	}
	return code, nil
}

func (s *Service) RejectRequest(ctx context.Context, requestID int64) (*Request, error) {
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
	req.Status = StatusRejected
	req.UpdatedAt = time.Now().UTC()
	if err := s.repo.UpdateRequest(ctx, req); err != nil {
		return nil, err
	}
	return req, nil
}

func (s *Service) RedeemNewAccount(ctx context.Context, codeValue, username, password string) (string, *player.Player, error) {
	uname := strings.ToLower(strings.TrimSpace(username))
	if uname == "" || strings.TrimSpace(password) == "" {
		return "", nil, ErrInvalid
	}
	if _, err := s.players.FindByUsername(ctx, uname); err == nil {
		return "", nil, ErrUsernameTaken
	} else if !errors.Is(err, sql.ErrNoRows) {
		return "", nil, err
	}
	var created *player.Player
	err := s.consumeCode(ctx, codeValue, func(ctx context.Context, tx bun.Tx, req *Request, code *Code, now time.Time) (int64, error) {
		if req.RequesterPlayerID != nil || req.Purpose != PurposeNewAccount {
			return 0, ErrForbidden
		}
		hash, err := auth.HashPassword(password)
		if err != nil {
			return 0, err
		}
		players := s.players.WithTx(tx)
		chars := s.characters.WithTx(tx)
		p := &player.Player{Name: req.CharacterName, Username: &uname, PasswordHash: hash, Role: validation.RoleGuest, Active: true, CreatedAt: now, UpdatedAt: now}
		if err := players.Create(ctx, p); err != nil {
			return 0, err
		}
		created = p
		if err := s.attachCharacter(ctx, tx, chars, p.ID, req, &p.ID, now); err != nil {
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
		row.Code.UsedAt = &now
		row.Code.UsedByPlayerID = &usedBy
		if err := repo.UpdateCode(ctx, &row.Code); err != nil {
			return err
		}
		row.Request.Status = StatusConsumed
		row.Request.UpdatedAt = now
		return repo.UpdateRequest(ctx, row.Request)
	})
}

func (s *Service) attachCharacter(ctx context.Context, tx bun.Tx, chars *character.Repo, playerID int64, req *Request, actorID *int64, now time.Time) error {
	c, err := chars.FindByName(ctx, req.CharacterName)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	if errors.Is(err, sql.ErrNoRows) {
		c = &character.Character{PlayerID: playerID, Name: req.CharacterName, Class: req.Class, MainSpec: req.MainSpec, SecondSpec: req.SecondSpec, Active: true, CreatedAt: now, UpdatedAt: now}
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
	c.Active = true
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
