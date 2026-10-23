// Package recruitment stores the shared recruitment text template and target
// setup of a raid series (or of a single raid without a series).
package recruitment

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/uptrace/bun"

	"raid-website/internal/audit"
	"raid-website/internal/raid"
)

var (
	ErrNotFound = errors.New("raid not found")
	ErrInvalid  = errors.New("invalid recruitment template")
)

const (
	MaxTextRunes = 2000
	groups       = 5
	slotsPerGrp  = 5
	maxSlotRunes = 40
)

// Template is the shared text and 5x5 target setup.
type Template struct {
	Text      string     `json:"text"`
	Setup     [][]string `json:"setup"`
	UpdatedBy int64      `json:"updated_by"`
	UpdatedAt time.Time  `json:"updated_at"`
}

// Service reads and saves templates with an audit trail.
type Service struct {
	repo  *Repo
	raids *raid.Repo
	audit *audit.Repo
	now   func() time.Time
}

func NewService(db bun.IDB) *Service {
	return &Service{repo: NewRepo(db), raids: raid.NewRepo(db), audit: audit.NewRepo(db), now: func() time.Time { return time.Now().UTC() }}
}

func (s *Service) scope(ctx context.Context, raidID int64) (col string, id int64, err error) {
	rd, err := s.raids.Get(ctx, raidID)
	if errors.Is(err, sql.ErrNoRows) || errors.Is(err, raid.ErrNotFound) {
		return "", 0, ErrNotFound
	}
	if err != nil {
		return "", 0, err
	}
	if rd.SeriesID != nil {
		return "series_id", *rd.SeriesID, nil
	}
	return "raid_id", rd.ID, nil
}

func toTemplate(r *row) (*Template, error) {
	t := &Template{Text: r.Text, UpdatedBy: r.UpdatedBy, UpdatedAt: r.UpdatedAt}
	if err := json.Unmarshal([]byte(r.SetupJSON), &t.Setup); err != nil {
		return nil, err
	}
	return t, nil
}

// Get returns the template that applies to the raid, or nil if none was saved.
func (s *Service) Get(ctx context.Context, raidID int64) (*Template, error) {
	col, id, err := s.scope(ctx, raidID)
	if err != nil {
		return nil, err
	}
	r, err := s.repo.find(ctx, col, id)
	if err != nil {
		return nil, err
	}
	if r == nil {
		return nil, nil
	}
	return toTemplate(r)
}

func validate(text string, setup [][]string) error {
	if strings.TrimSpace(text) == "" || utf8.RuneCountInString(text) > MaxTextRunes {
		return ErrInvalid
	}
	if len(setup) != groups {
		return ErrInvalid
	}
	for _, g := range setup {
		if len(g) != slotsPerGrp {
			return ErrInvalid
		}
		for _, slot := range g {
			if strings.TrimSpace(slot) == "" || utf8.RuneCountInString(slot) > maxSlotRunes {
				return ErrInvalid
			}
		}
	}
	return nil
}

// Save stores the template for the raid's series (or the raid itself).
func (s *Service) Save(ctx context.Context, raidID, actor int64, text string, setup [][]string) (*Template, error) {
	if err := validate(text, setup); err != nil {
		return nil, err
	}
	var saved *Template
	err := s.repo.runInTx(ctx, func(ctx context.Context, tx bun.Tx) error {
		txs := &Service{repo: NewRepo(tx), raids: raid.NewRepo(tx), audit: audit.NewRepo(tx), now: s.now}
		col, id, err := txs.scope(ctx, raidID)
		if err != nil {
			return err
		}
		setupJSON, err := json.Marshal(setup)
		if err != nil {
			return err
		}
		before, err := txs.Get(ctx, raidID)
		if err != nil {
			return err
		}
		now := s.now()
		if err = txs.repo.upsert(ctx, col, id, text, string(setupJSON), actor, now); err != nil {
			return err
		}
		saved = &Template{Text: text, Setup: setup, UpdatedBy: actor, UpdatedAt: now}
		b, _ := json.Marshal(before)
		a, _ := json.Marshal(saved)
		return txs.audit.Insert(ctx, &audit.Log{ActorID: &actor, EntityType: "raid_recruitment_template", EntityID: col + ":" + strconv.FormatInt(id, 10), Action: "save", BeforeJSON: string(b), AfterJSON: string(a), CreatedAt: now})
	})
	return saved, err
}
