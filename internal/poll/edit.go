package poll

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"raid-website/internal/validation"
)

// Errors of a poll edit. ErrEditInvalid wraps a message that is safe to show.
var (
	ErrEditInvalid        = errors.New("invalid poll edit")
	ErrOptionHasVotes     = errors.New("poll option has votes")
	errEditOptionNotFound = fmt.Errorf("%w: option does not belong to this poll", ErrEditInvalid)
)

// EditOption is one option of an edited choice poll. ID 0 adds a new option;
// an existing ID keeps the option, so a new label renames it with its votes.
type EditOption struct {
	ID    int64
	Label string
}

// Edit is the full new state of the editable poll fields (FR-24.7). Poll type
// and multiple selection stay as they were created.
type Edit struct {
	Title       string
	Description string
	ClosesAt    *time.Time
	Status      validation.PollStatus
	Options     []EditOption
}

// EditPlan is a validated edit: the poll after the change (new options have
// ID 0) and the existing options to change (label or order) or remove.
type EditPlan struct {
	After   Poll
	Changed []Option
	Removed []int64
}

// PlanEdit checks an edit against the current poll. Whether a removed option
// still has votes is checked by the repository inside the write transaction.
func PlanEdit(current Poll, edit Edit, now time.Time) (EditPlan, error) {
	title := strings.TrimSpace(edit.Title)
	if title == "" {
		return EditPlan{}, fmt.Errorf("%w: title is required", ErrEditInvalid)
	}
	if edit.Status != validation.PollStatusOpen && edit.Status != validation.PollStatusClosed {
		return EditPlan{}, fmt.Errorf("%w: status must be OPEN or CLOSED", ErrEditInvalid)
	}
	// An open poll whose closing time has passed takes no votes, so reopening
	// it that way would only look open.
	if edit.Status == validation.PollStatusOpen && edit.ClosesAt != nil && !edit.ClosesAt.After(now) {
		return EditPlan{}, fmt.Errorf("%w: closes_at must be in the future for an open poll", ErrEditInvalid)
	}

	after := current
	after.Title = title
	after.Description = edit.Description
	after.ClosesAt = edit.ClosesAt
	after.Status = edit.Status
	after.UpdatedAt = now

	if current.PollType != "CHOICE" {
		if len(edit.Options) > 0 {
			return EditPlan{}, fmt.Errorf("%w: options are only valid for choice polls", ErrEditInvalid)
		}
		after.Options = current.Options
		return EditPlan{After: after}, nil
	}

	if len(edit.Options) < 2 {
		return EditPlan{}, fmt.Errorf("%w: choice polls require at least two options", ErrEditInvalid)
	}
	existing := make(map[int64]Option, len(current.Options))
	for _, option := range current.Options {
		existing[option.ID] = option
	}
	plan := EditPlan{}
	kept := make(map[int64]bool, len(edit.Options))
	labels := make(map[string]bool, len(edit.Options))
	after.Options = make([]Option, 0, len(edit.Options))
	for i, row := range edit.Options {
		label := strings.TrimSpace(row.Label)
		if label == "" {
			return EditPlan{}, fmt.Errorf("%w: option label is required", ErrEditInvalid)
		}
		if labels[label] {
			return EditPlan{}, fmt.Errorf("%w: duplicate option label", ErrEditInvalid)
		}
		labels[label] = true
		option := Option{ID: row.ID, PollID: current.ID, Label: label, SortOrder: i + 1}
		if row.ID != 0 {
			old, ok := existing[row.ID]
			if !ok || kept[row.ID] {
				return EditPlan{}, errEditOptionNotFound
			}
			kept[row.ID] = true
			if old.Label != label || old.SortOrder != option.SortOrder {
				plan.Changed = append(plan.Changed, option)
			}
		}
		after.Options = append(after.Options, option)
	}
	for _, option := range current.Options {
		if !kept[option.ID] {
			plan.Removed = append(plan.Removed, option.ID)
		}
	}
	plan.After = after
	return plan, nil
}
