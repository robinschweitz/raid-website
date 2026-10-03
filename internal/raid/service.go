package raid

import "raid-website/internal/validation"

type Service struct {
	repo *Repo
}

func NewService(r *Repo) *Service {
	return &Service{repo: r}
}

func IsCompletedOrCancelled(s validation.RaidStatus) bool {
	return s == validation.RaidStatusCompleted || s == validation.RaidStatusCancelled
}
