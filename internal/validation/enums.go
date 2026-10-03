package validation

type RaidStatus string

const (
	RaidStatusPlanned   RaidStatus = "PLANNED"
	RaidStatusOpen      RaidStatus = "OPEN"
	RaidStatusCompleted RaidStatus = "COMPLETED"
	RaidStatusCancelled RaidStatus = "CANCELLED"
)

type AttendanceRole string

const (
	RoleTank AttendanceRole = "TANK"
	RoleHeal AttendanceRole = "HEAL"
	RoleDD   AttendanceRole = "DD"
)

type PollStatus string

const (
	PollStatusOpen     PollStatus = "OPEN"
	PollStatusClosed   PollStatus = "CLOSED"
	PollStatusArchived PollStatus = "ARCHIVED"
)

type LockStatus string

const (
	LockStatusActive    LockStatus = "ACTIVE"
	LockStatusUsed      LockStatus = "USED"
	LockStatusExpired   LockStatus = "EXPIRED"
	LockStatusCancelled LockStatus = "CANCELLED"
)
