package v1

import (
	"net/http"
	"time"

	"raid-website/internal/poll"
	"raid-website/internal/validation"
)

type createPollRequest struct {
	Title       string `json:"title"`
	Description string `json:"description"`
	ClosesAt    string `json:"closes_at"`
}

type votePollRequest struct {
	Choice string `json:"choice"`
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
		WriteError(w, http.StatusUnauthorized, "missing_actor", "X-Actor-ID header required")
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
	now := time.Now().UTC()
	p := &poll.Poll{
		Title:       req.Title,
		Description: req.Description,
		Status:      validation.PollStatusOpen,
		CreatedBy:   actor,
		CreatedAt:   now,
		UpdatedAt:   now,
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
	if err := a.Polls.CreatePoll(r.Context(), p); err != nil {
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
		WriteError(w, http.StatusUnauthorized, "missing_actor", "X-Actor-ID header required")
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
	if req.Choice == "" {
		BadRequest(w, "choice is required")
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
	v := &poll.Vote{PollID: id, PlayerID: actor, Choice: req.Choice, VotedAt: now}
	if err := a.Polls.UpsertVote(r.Context(), v); err != nil {
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
	if _, ok := ActorID(r); !ok {
		WriteError(w, http.StatusUnauthorized, "missing_actor", "X-Actor-ID header required")
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
	p.Status = validation.PollStatusClosed
	p.UpdatedAt = time.Now().UTC()
	if err := a.Polls.UpdatePoll(r.Context(), p); err != nil {
		WriteError(w, http.StatusInternalServerError, "internal", "could not close poll")
		return
	}
	WriteJSON(w, http.StatusOK, map[string]any{"data": p})
}
