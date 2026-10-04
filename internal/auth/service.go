package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"

	"raid-website/internal/player"
	"raid-website/internal/validation"
)

const (
	defaultSessionTTL = 30 * 24 * time.Hour

	// CookieName is the name of the session cookie.
	CookieName = "raid_session"
)

var (
	// ErrInvalidCredentials is returned when a username/password pair does not match.
	ErrInvalidCredentials = errors.New("invalid credentials")
	// ErrInactive is returned when the account exists but is deactivated.
	ErrInactive = errors.New("account inactive")
)

type Service struct {
	players  *player.Repo
	sessions *Repo
	ttl      time.Duration
}

func NewService(players *player.Repo, sessions *Repo) *Service {
	return &Service{players: players, sessions: sessions, ttl: defaultSessionTTL}
}

// HashPassword derives a bcrypt hash suitable for storage.
func HashPassword(password string) (string, error) {
	b, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// BootstrapAdmin creates an initial account atomically without changing existing accounts.
func (s *Service) BootstrapAdmin(ctx context.Context, username, password string) error {
	if username == "" || password == "" {
		return nil
	}
	uname := strings.ToLower(strings.TrimSpace(username))
	if uname == "" {
		return errors.New("bootstrap admin: username required")
	}
	return s.sessions.InTransaction(ctx, func(players *player.Repo) error {
		if _, err := players.FindByUsername(ctx, uname); err == nil {
			return nil
		} else if !errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("find bootstrap admin: %w", err)
		}
		hash, err := HashPassword(password)
		if err != nil {
			return fmt.Errorf("hash bootstrap password: %w", err)
		}
		now := time.Now().UTC()
		p := &player.Player{
			Name: username, Username: &uname, PasswordHash: hash,
			Active: true, Role: validation.RoleAdmin, CreatedAt: now, UpdatedAt: now,
		}
		if err := players.Create(ctx, p); err != nil {
			return fmt.Errorf("create bootstrap admin: %w", err)
		}
		return nil
	})
}

// SetCredentials assigns login credentials and role to an existing player.
func (s *Service) SetCredentials(ctx context.Context, playerID int64, username, password string, role validation.Role) (*player.Player, error) {
	var updated *player.Player
	err := s.sessions.InTransaction(ctx, func(players *player.Repo) error {
		p, err := players.Get(ctx, playerID)
		if err != nil {
			return fmt.Errorf("get credential player: %w", err)
		}
		uname := strings.ToLower(strings.TrimSpace(username))
		if uname == "" {
			return errors.New("username required")
		}
		if !role.Valid() {
			return errors.New("invalid role")
		}
		hash := p.PasswordHash
		if password != "" {
			hash, err = HashPassword(password)
			if err != nil {
				return fmt.Errorf("hash credential password: %w", err)
			}
		}
		p.Username = &uname
		p.PasswordHash = hash
		p.Role = role
		p.UpdatedAt = time.Now().UTC()
		if err := players.Update(ctx, p); err != nil {
			return fmt.Errorf("update player credentials: %w", err)
		}
		updated = p
		return nil
	})
	if err != nil {
		return nil, err
	}
	return updated, nil
}

// Login verifies credentials and creates a session, returning the raw token.
func (s *Service) Login(ctx context.Context, username, password string) (string, *player.Player, error) {
	uname := strings.ToLower(strings.TrimSpace(username))
	p, err := s.players.FindByUsername(ctx, uname)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", nil, ErrInvalidCredentials
		}
		return "", nil, fmt.Errorf("find login player: %w", err)
	}
	if p.PasswordHash == "" {
		return "", nil, ErrInvalidCredentials
	}
	if err := bcrypt.CompareHashAndPassword([]byte(p.PasswordHash), []byte(password)); err != nil {
		return "", nil, ErrInvalidCredentials
	}
	if !p.Active {
		return "", nil, ErrInactive
	}
	token, err := newToken()
	if err != nil {
		return "", nil, fmt.Errorf("generate session token: %w", err)
	}
	now := time.Now().UTC()
	sess := &Session{
		TokenHash:  hashToken(token),
		PlayerID:   p.ID,
		CreatedAt:  now,
		ExpiresAt:  now.Add(s.ttl),
		LastSeenAt: now,
	}
	if err := s.sessions.CreateSession(ctx, sess); err != nil {
		return "", nil, fmt.Errorf("create login session: %w", err)
	}
	return token, p, nil
}

// Authenticate resolves a raw session token. Missing, unknown, or expired sessions return nil, nil.
func (s *Service) Authenticate(ctx context.Context, token string) (*player.Player, error) {
	if token == "" {
		return nil, nil
	}
	sess, err := s.sessions.FindSessionByHash(ctx, hashToken(token))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("find authentication session: %w", err)
	}
	now := time.Now().UTC()
	if !now.Before(sess.ExpiresAt) {
		return nil, nil
	}
	p, err := s.players.Get(ctx, sess.PlayerID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("get session player: %w", err)
	}
	if !p.Active {
		return nil, ErrInactive
	}
	if err := s.sessions.TouchSession(ctx, sess.ID, now); err != nil {
		return nil, fmt.Errorf("touch authentication session: %w", err)
	}
	return p, nil
}

// Logout deletes the session associated with a raw token.
func (s *Service) Logout(ctx context.Context, token string) error {
	if token == "" {
		return nil
	}
	if err := s.sessions.DeleteByHash(ctx, hashToken(token)); err != nil {
		return fmt.Errorf("delete logout session: %w", err)
	}
	return nil
}

func newToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

func hashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}
