package statistics

import (
	"context"

	"github.com/uptrace/bun"
)

// TopLimit is the maximum length of the top lists (FR-27.4).
const TopLimit = 10

// historicalRaids selects raids that count as history: completed or archived.
const historicalRaids = "(SELECT id FROM raids WHERE status = 'COMPLETED' OR archived_at IS NOT NULL)"

type Repo struct {
	db bun.IDB
}

func NewRepo(db bun.IDB) *Repo {
	return &Repo{db: db}
}

// RaidsByStatus returns the raid count per stored status, ordered count
// descending then status ascending. Statuses without raids are absent.
func (r *Repo) RaidsByStatus(ctx context.Context) ([]StatusCount, error) {
	var rows []StatusCount
	err := r.db.NewRaw(`SELECT status AS status, COUNT(*) AS count FROM raids
		GROUP BY status ORDER BY count DESC, status ASC`).Scan(ctx, &rows)
	return rows, err
}

// ClassDistribution counts active characters per class.
func (r *Repo) ClassDistribution(ctx context.Context) ([]ClassCount, error) {
	var rows []ClassCount
	err := r.db.NewRaw(`SELECT class AS class, COUNT(*) AS count FROM characters
		WHERE active = 1 GROUP BY class ORDER BY count DESC, class ASC`).Scan(ctx, &rows)
	return rows, err
}

// RoleHistory counts roster rows of historical raids per stored role.
func (r *Repo) RoleHistory(ctx context.Context) ([]RoleCount, error) {
	var rows []RoleCount
	err := r.db.NewRaw(`SELECT role AS role, COUNT(*) AS count FROM raid_attendance
		WHERE raid_id IN `+historicalRaids+` GROUP BY role ORDER BY count DESC, role ASC`).Scan(ctx, &rows)
	return rows, err
}

// LootByClass counts loot awards of historical raids that are not withdrawn,
// per class of the receiving character.
func (r *Repo) LootByClass(ctx context.Context) ([]ClassCount, error) {
	var rows []ClassCount
	err := r.db.NewRaw(`SELECT c.class AS class, COUNT(*) AS count FROM loot_awards l
		JOIN characters c ON c.id = l.awarded_to_character_id
		WHERE l.withdrawn_at IS NULL AND l.raid_id IN `+historicalRaids+`
		GROUP BY c.class ORDER BY count DESC, class ASC`).Scan(ctx, &rows)
	return rows, err
}

// TopAttendance returns the players with the most roster rows in historical raids.
func (r *Repo) TopAttendance(ctx context.Context) ([]PlayerCount, error) {
	var rows []PlayerCount
	err := r.db.NewRaw(`SELECT p.id AS player_id, p.name AS player_name, COUNT(*) AS count
		FROM raid_attendance a JOIN players p ON p.id = a.player_id
		WHERE a.raid_id IN `+historicalRaids+`
		GROUP BY p.id, p.name ORDER BY count DESC, p.name ASC, p.id ASC LIMIT ?`, TopLimit).Scan(ctx, &rows)
	return rows, err
}

// TopLoot returns the players with the most loot awards of historical raids
// that are not withdrawn.
func (r *Repo) TopLoot(ctx context.Context) ([]PlayerCount, error) {
	var rows []PlayerCount
	err := r.db.NewRaw(`SELECT p.id AS player_id, p.name AS player_name, COUNT(*) AS count
		FROM loot_awards l JOIN players p ON p.id = l.awarded_to_player_id
		WHERE l.withdrawn_at IS NULL AND l.raid_id IN `+historicalRaids+`
		GROUP BY p.id, p.name ORDER BY count DESC, p.name ASC, p.id ASC LIMIT ?`, TopLimit).Scan(ctx, &rows)
	return rows, err
}
