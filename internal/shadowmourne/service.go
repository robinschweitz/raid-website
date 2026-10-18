package shadowmourne

import (
	"context"
	"database/sql"
	"errors"
	"sort"
	"time"

	"raid-website/internal/character"
)

var (
	ErrInvalidProgress    = errors.New("invalid shadowmourne progress")
	ErrInvalidPosition    = errors.New("invalid list position")
	ErrNotActive          = errors.New("entry is not active")
	ErrCharacterMissing   = errors.New("character not found")
	ErrClassNotEligible   = errors.New("character class cannot use shadowmourne")
	ErrAlreadyListed      = errors.New("character is already on the list")
	ErrProgressIncomplete = errors.New("shard progress is incomplete")
	ErrRaidMissing        = errors.New("raid not found")
	ErrRaidLocked         = errors.New("raid is cancelled or archived")
	ErrInvalidTask        = errors.New("invalid raid task")
	ErrNotInRoster        = errors.New("character is not in the raid roster")
	ErrNotTaskOption      = errors.New("character cannot do this raid task")
)

const (
	entryAuditType      = "shadowmourne_entry"
	assignmentAuditType = "shadowmourne_raid_task"
)

type Service struct {
	entries    *Repo
	characters *character.Repo
	now        func() time.Time
}

func NewService(entries *Repo, characters *character.Repo) *Service {
	return &Service{entries: entries, characters: characters, now: func() time.Time { return time.Now().UTC() }}
}

func (s *Service) List(ctx context.Context) ([]Entry, error) {
	return s.entries.List(ctx)
}

func validProgress(progress ProgressInput) bool {
	return validStage(progress.Stage) && progress.ShardsCollected >= 0 && progress.ShardTarget > 0 && progress.ShardsCollected <= progress.ShardTarget
}

func (s *Service) Create(ctx context.Context, actorID int64, input CreateInput) (*Entry, error) {
	if input.Stage == "" {
		input.Stage = StagePrequest
	}
	if input.ShardTarget == 0 {
		input.ShardTarget = DefaultShardTarget
	}
	if input.CharacterID <= 0 || !validProgress(ProgressInput{Stage: input.Stage, ShardsCollected: input.ShardsCollected, ShardTarget: input.ShardTarget}) {
		return nil, ErrInvalidProgress
	}
	if input.Position < 0 {
		return nil, ErrInvalidPosition
	}
	char, err := s.characters.Get(ctx, input.CharacterID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrCharacterMissing
	}
	if err != nil {
		return nil, err
	}
	if !eligibleClass(char.Class) {
		return nil, ErrClassNotEligible
	}
	var created *Entry
	err = s.entries.InTransaction(ctx, func(tx *Repo) error {
		listed, err := tx.HasActiveCharacter(ctx, input.CharacterID)
		if err != nil {
			return err
		}
		if listed {
			return ErrAlreadyListed
		}
		active, err := tx.ListActive(ctx)
		if err != nil {
			return err
		}
		now := s.now()
		created = &Entry{CharacterID: input.CharacterID, Priority: len(active) + 1, Stage: input.Stage, ShardsCollected: input.ShardsCollected, ShardTarget: input.ShardTarget, CreatedAt: now, UpdatedAt: now}
		if err := tx.Create(ctx, created); err != nil {
			return err
		}
		if input.Position > 0 && input.Position <= len(active) {
			if err := s.reorder(ctx, tx, append(active, *created), created.ID, input.Position, now); err != nil {
				return err
			}
		}
		created, err = tx.Get(ctx, created.ID)
		if err != nil {
			return err
		}
		return tx.Record(ctx, actorID, entryAuditType, created.ID, "create", nil, created)
	})
	return created, err
}

func (s *Service) UpdateProgress(ctx context.Context, actorID, entryID int64, input ProgressInput) (*Entry, error) {
	if !validProgress(input) {
		return nil, ErrInvalidProgress
	}
	var updated *Entry
	err := s.entries.InTransaction(ctx, func(tx *Repo) error {
		before, err := tx.Get(ctx, entryID)
		if err != nil {
			return err
		}
		if before.Status != StatusActive {
			return ErrNotActive
		}
		changed := *before
		changed.Stage = input.Stage
		changed.ShardsCollected = input.ShardsCollected
		changed.ShardTarget = input.ShardTarget
		changed.UpdatedAt = s.now()
		if err := tx.SaveProgress(ctx, &changed); err != nil {
			return err
		}
		updated, err = tx.Get(ctx, entryID)
		if err != nil {
			return err
		}
		return tx.Record(ctx, actorID, entryAuditType, entryID, "update_progress", before, updated)
	})
	return updated, err
}

// Complete moves a finished character into the history. It requires the shard target.
func (s *Service) Complete(ctx context.Context, actorID, entryID int64) (*Entry, error) {
	var completed *Entry
	err := s.entries.InTransaction(ctx, func(tx *Repo) error {
		before, err := tx.Get(ctx, entryID)
		if err != nil {
			return err
		}
		if before.Status != StatusActive {
			return ErrNotActive
		}
		if before.ShardsCollected < before.ShardTarget {
			return ErrProgressIncomplete
		}
		now := s.now()
		if err := tx.Complete(ctx, entryID, now); err != nil {
			return err
		}
		if err := s.renumber(ctx, tx, now); err != nil {
			return err
		}
		completed, err = tx.Get(ctx, entryID)
		if err != nil {
			return err
		}
		return tx.Record(ctx, actorID, entryAuditType, entryID, "complete", before, completed)
	})
	return completed, err
}

// Remove takes a wrong or abandoned entry off the list. The row is kept and audited (ADR 007).
func (s *Service) Remove(ctx context.Context, actorID, entryID int64) error {
	return s.entries.InTransaction(ctx, func(tx *Repo) error {
		before, err := tx.Get(ctx, entryID)
		if err != nil {
			return err
		}
		if before.Status != StatusActive {
			return ErrNotActive
		}
		now := s.now()
		if err := tx.Remove(ctx, entryID, now); err != nil {
			return err
		}
		if err := s.renumber(ctx, tx, now); err != nil {
			return err
		}
		return tx.Record(ctx, actorID, entryAuditType, entryID, "remove", before, nil)
	})
}

// Move puts an active entry at the 1-based position and shifts the others.
func (s *Service) Move(ctx context.Context, actorID, entryID int64, position int) ([]Entry, error) {
	if position < 1 {
		return nil, ErrInvalidPosition
	}
	var entries []Entry
	err := s.entries.InTransaction(ctx, func(tx *Repo) error {
		before, err := tx.Get(ctx, entryID)
		if err != nil {
			return err
		}
		if before.Status != StatusActive {
			return ErrNotActive
		}
		active, err := tx.ListActive(ctx)
		if err != nil {
			return err
		}
		if position > len(active) {
			return ErrInvalidPosition
		}
		if err := s.reorder(ctx, tx, active, entryID, position, s.now()); err != nil {
			return err
		}
		after, err := tx.Get(ctx, entryID)
		if err != nil {
			return err
		}
		if err := tx.Record(ctx, actorID, entryAuditType, entryID, "move", before, after); err != nil {
			return err
		}
		entries, err = tx.List(ctx)
		return err
	})
	return entries, err
}

// reorder places entryID at position within active and stores priorities 1..n.
func (s *Service) reorder(ctx context.Context, tx *Repo, active []Entry, entryID int64, position int, at time.Time) error {
	var moved Entry
	others := make([]Entry, 0, len(active))
	for _, entry := range active {
		if entry.ID == entryID {
			moved = entry
			continue
		}
		others = append(others, entry)
	}
	ordered := make([]Entry, 0, len(active))
	ordered = append(ordered, others[:position-1]...)
	ordered = append(ordered, moved)
	ordered = append(ordered, others[position-1:]...)
	return storePriorities(ctx, tx, ordered, at)
}

// renumber closes gaps after an entry left the active list.
func (s *Service) renumber(ctx context.Context, tx *Repo, at time.Time) error {
	active, err := tx.ListActive(ctx)
	if err != nil {
		return err
	}
	return storePriorities(ctx, tx, active, at)
}

func storePriorities(ctx context.Context, tx *Repo, ordered []Entry, at time.Time) error {
	for index, entry := range ordered {
		if entry.Priority == index+1 {
			continue
		}
		if err := tx.SetPriority(ctx, entry.ID, index+1, at); err != nil {
			return err
		}
	}
	return nil
}

// taskOptions lists who staff may choose for a task: list entries on that
// stage in priority order, then eligible roster characters not on the list.
func taskOptions(roster []RaidCharacter, task Stage) []RaidCharacter {
	listed := make([]RaidCharacter, 0, len(roster))
	unlisted := make([]RaidCharacter, 0, len(roster))
	for _, character := range roster {
		switch {
		case character.Entry != nil && character.Entry.Stage == task:
			listed = append(listed, character)
		case character.Entry == nil && eligibleClass(character.CharacterClass):
			unlisted = append(unlisted, character)
		}
	}
	sort.SliceStable(listed, func(i, j int) bool { return listed[i].Entry.Priority < listed[j].Entry.Priority })
	return append(listed, unlisted...)
}

// raidRoster returns the roster characters with their active list entries.
func raidRoster(ctx context.Context, repo *Repo, raidID int64) ([]RaidCharacter, error) {
	roster, err := repo.RosterCharacters(ctx, raidID)
	if err != nil {
		return nil, err
	}
	active, err := repo.ListActive(ctx)
	if err != nil {
		return nil, err
	}
	entries := map[int64]*Entry{}
	for i := range active {
		entries[active[i].CharacterID] = &active[i]
	}
	for i := range roster {
		roster[i].Entry = entries[roster[i].CharacterID]
	}
	return roster, nil
}

// RaidPlan shows who does each single-person questline step in a raid.
func (s *Service) RaidPlan(ctx context.Context, raidID int64) (*RaidPlan, error) {
	state, err := s.entries.RaidState(ctx, raidID)
	if err != nil {
		return nil, err
	}
	roster, err := raidRoster(ctx, s.entries, raidID)
	if err != nil {
		return nil, err
	}
	assigned, err := s.entries.Assignments(ctx, raidID)
	if err != nil {
		return nil, err
	}
	plan := &RaidPlan{RaidID: raidID, Editable: !state.Cancelled && !state.Archived, Tasks: make([]RaidTask, 0, len(RaidTasks))}
	for _, task := range RaidTasks {
		item := RaidTask{Task: task, Options: taskOptions(roster, task)}
		if len(item.Options) > 0 && item.Options[0].Entry != nil {
			item.Suggested = &item.Options[0]
		}
		// A choice only counts while the character is still in the roster.
		if characterID, ok := assigned[task]; ok {
			for i := range roster {
				if roster[i].CharacterID == characterID {
					item.Assigned = &roster[i]
					break
				}
			}
		}
		plan.Tasks = append(plan.Tasks, item)
	}
	return plan, nil
}

// AssignRaidTask stores the staff choice for one step; a nil character returns to the suggestion.
func (s *Service) AssignRaidTask(ctx context.Context, actorID, raidID int64, task Stage, characterID *int64) (*RaidPlan, error) {
	if !validRaidTask(task) {
		return nil, ErrInvalidTask
	}
	err := s.entries.InTransaction(ctx, func(tx *Repo) error {
		state, err := tx.RaidState(ctx, raidID)
		if err != nil {
			return err
		}
		if state.Cancelled || state.Archived {
			return ErrRaidLocked
		}
		if characterID != nil {
			roster, err := raidRoster(ctx, tx, raidID)
			if err != nil {
				return err
			}
			inRoster, isOption := false, false
			for _, character := range roster {
				if character.CharacterID == *characterID {
					inRoster = true
				}
			}
			for _, option := range taskOptions(roster, task) {
				if option.CharacterID == *characterID {
					isOption = true
				}
			}
			if !inRoster {
				return ErrNotInRoster
			}
			if !isOption {
				return ErrNotTaskOption
			}
		}
		before, err := tx.Assignments(ctx, raidID)
		if err != nil {
			return err
		}
		if err := tx.SaveAssignment(ctx, raidID, task, characterID, s.now()); err != nil {
			return err
		}
		type snapshot struct {
			Task        Stage  `json:"task"`
			CharacterID *int64 `json:"character_id"`
		}
		var previous *int64
		if id, ok := before[task]; ok {
			previous = &id
		}
		return tx.Record(ctx, actorID, assignmentAuditType, raidID, "assign", snapshot{Task: task, CharacterID: previous}, snapshot{Task: task, CharacterID: characterID})
	})
	if err != nil {
		return nil, err
	}
	return s.RaidPlan(ctx, raidID)
}
