package statistics

// StatusCount is the number of raids with one status.
type StatusCount struct {
	Status string
	Count  int64
}

// ClassCount is a count grouped by character class.
type ClassCount struct {
	Class string
	Count int64
}

// RoleCount is a count grouped by roster role.
type RoleCount struct {
	Role  string
	Count int64
}

// PlayerCount is a count for one player.
type PlayerCount struct {
	PlayerID   int64
	PlayerName string
	Count      int64
}

// Statistics holds the server-side aggregates of FR-27.1. It contains no item,
// EP, note, or account data (FR-27.3).
type Statistics struct {
	RaidsByStatus []StatusCount
	ClassDistrib  []ClassCount
	RoleHistory   []RoleCount
	LootByClass   []ClassCount
	TopAttendance []PlayerCount
	TopLoot       []PlayerCount
}
