package statistics

import (
	"context"
	"fmt"

	"raid-website/internal/validation"
)

type Service struct {
	repo *Repo
}

func NewService(repo *Repo) *Service {
	return &Service{repo: repo}
}

// Get computes the public aggregates. Raid statuses are always listed in the
// order of FR-10.3, also with count 0 (FR-27.4).
func (s *Service) Get(ctx context.Context) (Statistics, error) {
	var out Statistics
	stored, err := s.repo.RaidsByStatus(ctx)
	if err != nil {
		return out, fmt.Errorf("count raids by status: %w", err)
	}
	counts := make(map[string]int64, len(stored))
	for _, row := range stored {
		counts[row.Status] = row.Count
	}
	for _, status := range []validation.RaidStatus{
		validation.RaidStatusPlanned,
		validation.RaidStatusOpen,
		validation.RaidStatusCompleted,
		validation.RaidStatusCancelled,
	} {
		out.RaidsByStatus = append(out.RaidsByStatus, StatusCount{Status: string(status), Count: counts[string(status)]})
	}
	if out.ClassDistrib, err = s.repo.ClassDistribution(ctx); err != nil {
		return out, fmt.Errorf("class distribution: %w", err)
	}
	if out.RoleHistory, err = s.repo.RoleHistory(ctx); err != nil {
		return out, fmt.Errorf("role history: %w", err)
	}
	if out.LootByClass, err = s.repo.LootByClass(ctx); err != nil {
		return out, fmt.Errorf("loot by class: %w", err)
	}
	if out.TopAttendance, err = s.repo.TopAttendance(ctx); err != nil {
		return out, fmt.Errorf("top attendance: %w", err)
	}
	if out.TopLoot, err = s.repo.TopLoot(ctx); err != nil {
		return out, fmt.Errorf("top loot: %w", err)
	}
	return out, nil
}
