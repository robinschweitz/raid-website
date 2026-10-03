package enhancepoints

import (
	"errors"

	"raid-website/internal/validation"
)

type RaidState struct {
	Status validation.RaidStatus
}

var ErrAlreadyAwarded = errors.New("already awarded no-mainspec for raid")

// ShouldAwardNoMainSpec returns true if raid completed and no mainspec
func ShouldAwardNoMainSpec(rs RaidState, noMainSpec bool) bool {
	if rs.Status == validation.RaidStatusCancelled {
		return false
	}
	if rs.Status != validation.RaidStatusCompleted {
		return false
	}
	return noMainSpec
}
