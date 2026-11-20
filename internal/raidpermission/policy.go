// Package raidpermission resolves the raid staff rights described in FR-1.11–13.
package raidpermission

import "raid-website/internal/validation"

// Actions represents the four independently configurable groups of staff rights.
type Actions uint8

const (
	Roster Actions = 1 << iota
	AbsenceNotes
	LootEP
	Planning
	All = Roster | AbsenceNotes | LootEP | Planning
)

// Policy is an admin-configured ceiling. AssignedOnly uses the leaders of the
// individual raid, which already includes the inherited leaders on creation.
type Policy struct {
	Allowed      Actions `json:"allowed"`
	AssignedOnly bool    `json:"assigned_only"`
}

type Effective struct {
	Allowed Actions `json:"allowed"`
	Source  string  `json:"source"`
}

// Base returns the rights before any one-occurrence substitution is considered.
// A nil policy denotes an existing account without an admin restriction.
func Base(role validation.Role, policy *Policy, leadsOccurrence bool) Effective {
	switch role {
	case validation.RoleAdmin:
		return Effective{All, "admin"}
	case validation.RoleRaidleader, validation.RoleRaidassistent:
		if policy == nil {
			return Effective{All, "role"}
		}
		if policy.AssignedOnly && !leadsOccurrence {
			return Effective{0, "outside_scope"}
		}
		if policy.Allowed & ^All != 0 {
			return Effective{0, "invalid_policy"}
		}
		return Effective{policy.Allowed, "policy"}
	default:
		return Effective{0, "role"}
	}
}

// CanDelegate checks the grantor's authority for one occurrence, independent of
// recipient eligibility. Being named in the roster alone is not sufficient.
func CanDelegate(role validation.Role, leadsOccurrence bool, base Effective, requested Actions) bool {
	return role == validation.RoleRaidleader && leadsOccurrence && requested != 0 && requested & ^All == 0 && requested & ^base.Allowed == 0
}
