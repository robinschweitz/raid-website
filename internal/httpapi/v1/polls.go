package v1

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"raid-website/internal/poll"
	"raid-website/internal/validation"
)

type createPollRequest struct {
	Title         string   `json:"title"`
	Description   string   `json:"description"`
	ClosesAt      string   `json:"closes_at"`
	PollType      string   `json:"poll_type"`
	AllowMultiple bool     `json:"allow_multiple"`
	Options       []string `json:"options"`
}

type editPollOption struct {
	ID    int64  `json:"id"`
	Label string `json:"label"`
}

type editPollRequest struct {
	Title       string           `json:"title"`
	Description string           `json:"description"`
	ClosesAt    string           `json:"closes_at"`
	Status      string           `json:"status"`
	Options     []editPollOption `json:"options"`
}

type votePollRequest struct {
	Choice  string   `json:"choice"`
	Choices []string `json:"choices"`
}

func (a *API) listPolls(w http.ResponseWriter, r *http.Request) {
	ps, err := a.Polls.ListPolls(r.Context())
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "internal", "could not list polls")
		return
	}
	writeListJSON(w, ps)
}

func (a *API) createPoll(w http.ResponseWriter, r *http.Request) {
	actor, ok := ActorID(r)
	if !ok {
		WriteError(w, http.StatusUnauthorized, "unauthenticated", "login required")
		return
	}
	var req createPollRequest
	if err := DecodeJSON(r, &req); err != nil {
		BadRequest(w, "malformed json body")
		return
	}
	if req.Title == "" {
		BadRequest(w, "title is required")
		return
	}
	pollType := strings.TrimSpace(req.PollType)
	if pollType == "" {
		pollType = "TEXT"
	}
	if pollType != "TEXT" && pollType != "CHOICE" {
		BadRequest(w, "poll_type must be TEXT or CHOICE")
		return
	}
	options := normalizePollOptions(req.Options)
	if pollType == "TEXT" {
		if req.AllowMultiple {
			BadRequest(w, "allow_multiple is only valid for choice polls")
			return
		}
		if len(options) > 0 {
			BadRequest(w, "options are only valid for choice polls")
			return
		}
	}
	if pollType == "CHOICE" && len(options) < 2 {
		BadRequest(w, "choice polls require at least two options")
		return
	}
	now := time.Now().UTC()
	p := &poll.Poll{
		Title:         req.Title,
		Description:   req.Description,
		PollType:      pollType,
		AllowMultiple: req.AllowMultiple,
		Status:        validation.PollStatusOpen,
		CreatedBy:     actor,
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	if req.ClosesAt != "" {
		t, err := time.Parse(time.RFC3339, req.ClosesAt)
		if err != nil {
			BadRequest(w, "closes_at must be RFC3339")
			return
		}
		t = t.UTC()
		p.ClosesAt = &t
	}
	if err := a.Polls.CreatePollWithOptions(r.Context(), p, options); err != nil {
		WriteError(w, http.StatusInternalServerError, "internal", "could not create poll")
		return
	}
	WriteJSON(w, http.StatusCreated, map[string]any{"data": p})
}

func (a *API) getPoll(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r, "id")
	if !ok {
		BadRequest(w, "invalid poll id")
		return
	}
	p, err := a.Polls.GetPoll(r.Context(), id)
	if err != nil {
		if isNotFound(err) {
			WriteError(w, http.StatusNotFound, "not_found", "poll not found")
			return
		}
		WriteError(w, http.StatusInternalServerError, "internal", "could not load poll")
		return
	}
	WriteJSON(w, http.StatusOK, map[string]any{"data": p})
}

func (a *API) votePoll(w http.ResponseWriter, r *http.Request) {
	actor, ok := ActorID(r)
	if !ok {
		WriteError(w, http.StatusUnauthorized, "unauthenticated", "login required")
		return
	}
	id, ok := pathID(r, "id")
	if !ok {
		BadRequest(w, "invalid poll id")
		return
	}
	var req votePollRequest
	if err := DecodeJSON(r, &req); err != nil {
		BadRequest(w, "malformed json body")
		return
	}
	p, err := a.Polls.GetPoll(r.Context(), id)
	if err != nil {
		if isNotFound(err) {
			WriteError(w, http.StatusNotFound, "not_found", "poll not found")
			return
		}
		WriteError(w, http.StatusInternalServerError, "internal", "could not load poll")
		return
	}
	now := time.Now().UTC()
	if p.Status != validation.PollStatusOpen {
		WriteError(w, http.StatusConflict, "poll_closed", "poll is not open")
		return
	}
	if p.ClosesAt != nil && now.After(*p.ClosesAt) {
		WriteError(w, http.StatusConflict, "poll_closed", "poll has passed its closing time")
		return
	}
	choices, optionIDsByLabel, ok := validatePollVoteRequest(w, p, req)
	if !ok {
		return
	}
	v := &poll.Vote{PollID: id, PlayerID: actor, Choice: strings.Join(choices, ", "), Choices: choices, VotedAt: now}
	if err := a.Polls.UpsertVoteWithChoices(r.Context(), v, optionIDsByLabel, p.Title); err != nil {
		WriteError(w, http.StatusInternalServerError, "internal", "could not record vote")
		return
	}
	saved, err := a.Polls.FindVote(r.Context(), id, actor)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "internal", "could not load vote")
		return
	}
	WriteJSON(w, http.StatusOK, map[string]any{"data": saved})
}

func (a *API) listPollVotes(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r, "id")
	if !ok {
		BadRequest(w, "invalid poll id")
		return
	}
	vs, err := a.Polls.ListVotes(r.Context(), id)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "internal", "could not list votes")
		return
	}
	writeListJSON(w, vs)
}

func (a *API) closePoll(w http.ResponseWriter, r *http.Request) {
	actor, ok := ActorID(r)
	if !ok {
		WriteError(w, http.StatusUnauthorized, "unauthenticated", "login required")
		return
	}
	id, ok := pathID(r, "id")
	if !ok {
		BadRequest(w, "invalid poll id")
		return
	}
	p, err := a.Polls.GetPoll(r.Context(), id)
	if err != nil {
		if isNotFound(err) {
			WriteError(w, http.StatusNotFound, "not_found", "poll not found")
			return
		}
		WriteError(w, http.StatusInternalServerError, "internal", "could not load poll")
		return
	}
	if p.Status == validation.PollStatusClosed {
		WriteError(w, http.StatusConflict, "poll_closed", "poll is already closed")
		return
	}
	before := *p
	p.Status = validation.PollStatusClosed
	p.UpdatedAt = time.Now().UTC()
	if err := a.Polls.ClosePoll(r.Context(), p, before, actor); err != nil {
		WriteError(w, http.StatusInternalServerError, "internal", "could not close poll")
		return
	}
	WriteJSON(w, http.StatusOK, map[string]any{"data": p})
}

func (a *API) editPoll(w http.ResponseWriter, r *http.Request) {
	actor, ok := ActorID(r)
	if !ok {
		WriteError(w, http.StatusUnauthorized, "unauthenticated", "login required")
		return
	}
	id, ok := pathID(r, "id")
	if !ok {
		BadRequest(w, "invalid poll id")
		return
	}
	var req editPollRequest
	if err := DecodeJSON(r, &req); err != nil {
		BadRequest(w, "malformed json body")
		return
	}
	current, err := a.Polls.GetPoll(r.Context(), id)
	if err != nil {
		if isNotFound(err) {
			WriteError(w, http.StatusNotFound, "not_found", "poll not found")
			return
		}
		WriteError(w, http.StatusInternalServerError, "internal", "could not load poll")
		return
	}
	edit := poll.Edit{Title: req.Title, Description: req.Description, Status: validation.PollStatus(req.Status)}
	if req.ClosesAt != "" {
		t, err := time.Parse(time.RFC3339, req.ClosesAt)
		if err != nil {
			BadRequest(w, "closes_at must be RFC3339")
			return
		}
		t = t.UTC()
		edit.ClosesAt = &t
	}
	for _, option := range req.Options {
		edit.Options = append(edit.Options, poll.EditOption{ID: option.ID, Label: option.Label})
	}
	plan, err := poll.PlanEdit(*current, edit, time.Now().UTC())
	if err != nil {
		BadRequest(w, strings.TrimPrefix(err.Error(), poll.ErrEditInvalid.Error()+": "))
		return
	}
	if err := a.Polls.SaveEdit(r.Context(), *current, &plan, actor); err != nil {
		if errors.Is(err, poll.ErrOptionHasVotes) {
			WriteError(w, http.StatusConflict, "poll_option_has_votes", "an option with votes cannot be removed")
			return
		}
		WriteError(w, http.StatusInternalServerError, "internal", "could not update poll")
		return
	}
	saved, err := a.Polls.GetPoll(r.Context(), id)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "internal", "could not load poll")
		return
	}
	WriteJSON(w, http.StatusOK, map[string]any{"data": saved})
}

func normalizePollOptions(raw []string) []string {
	seen := make(map[string]bool, len(raw))
	options := make([]string, 0, len(raw))
	for _, value := range raw {
		label := strings.TrimSpace(value)
		if label == "" || seen[label] {
			continue
		}
		seen[label] = true
		options = append(options, label)
	}
	return options
}

func validatePollVoteRequest(w http.ResponseWriter, p *poll.Poll, req votePollRequest) ([]string, map[string]int64, bool) {
	if p.PollType == "" || p.PollType == "TEXT" {
		choice := strings.TrimSpace(req.Choice)
		if choice == "" {
			BadRequest(w, "choice is required")
			return nil, nil, false
		}
		if len(req.Choices) > 0 {
			BadRequest(w, "choices are only valid for choice polls")
			return nil, nil, false
		}
		return []string{choice}, nil, true
	}

	choices := req.Choices
	if len(choices) == 0 && strings.TrimSpace(req.Choice) != "" {
		choices = []string{req.Choice}
	}
	if len(choices) == 0 {
		BadRequest(w, "at least one choice is required")
		return nil, nil, false
	}
	if !p.AllowMultiple && len(choices) != 1 {
		BadRequest(w, "this poll allows only one choice")
		return nil, nil, false
	}
	optionIDsByLabel := make(map[string]int64, len(p.Options))
	for _, opt := range p.Options {
		optionIDsByLabel[opt.Label] = opt.ID
	}
	seen := make(map[string]bool, len(choices))
	normalized := make([]string, 0, len(choices))
	for _, value := range choices {
		label := strings.TrimSpace(value)
		if label == "" {
			BadRequest(w, "choice is required")
			return nil, nil, false
		}
		if seen[label] {
			BadRequest(w, "duplicate choice")
			return nil, nil, false
		}
		if optionIDsByLabel[label] == 0 {
			BadRequest(w, "choice must match a poll option")
			return nil, nil, false
		}
		seen[label] = true
		normalized = append(normalized, label)
	}
	return normalized, optionIDsByLabel, true
}
