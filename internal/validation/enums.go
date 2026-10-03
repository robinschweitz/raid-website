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

// Role is a player's permission role. Higher roles include the powers of lower ones.
type Role string

const (
	RoleGuest         Role = "GUEST"
	RoleMember        Role = "MEMBER"
	RoleRaidassistent Role = "RAIDASSISTENT"
	RoleRaidleader    Role = "RAIDLEADER"
	RoleAdmin         Role = "ADMIN"
)

var roleRank = map[Role]int{
	RoleGuest:         0,
	RoleMember:        1,
	RoleRaidassistent: 2,
	RoleRaidleader:    3,
	RoleAdmin:         4,
}

// Valid reports whether r is a known role.
func (r Role) Valid() bool {
	_, ok := roleRank[r]
	return ok
}

// AtLeast reports whether r grants at least the powers of min.
func (r Role) AtLeast(min Role) bool {
	rr, ok := roleRank[r]
	if !ok {
		return false
	}
	mr, ok := roleRank[min]
	if !ok {
		return false
	}
	return rr >= mr
}
