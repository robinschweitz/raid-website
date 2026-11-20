package raidpermission

import (
	"testing"

	"raid-website/internal/validation"
)

func TestOccurrenceBoundary(t *testing.T) {
	p := &Policy{Roster, true}
	for _, role := range []validation.Role{validation.RoleRaidassistent, validation.RoleRaidleader} {
		if Base(role, nil, false).Allowed != All || Base(role, p, false).Allowed != 0 || Base(role, p, true).Allowed != Roster {
			t.Fatalf("unexpected scope or default for %s", role)
		}
	}
	if Base(validation.RoleMember, p, true).Allowed != 0 {
		t.Fatal("leader label granted staff rights")
	}
	base := Base(validation.RoleRaidleader, p, true)
	if !CanDelegate(validation.RoleRaidleader, true, base, Roster) || CanDelegate(validation.RoleRaidleader, true, base, Roster|AbsenceNotes) || CanDelegate(validation.RoleRaidleader, false, base, Roster) || CanDelegate(validation.RoleRaidassistent, true, base, Roster) {
		t.Fatal("delegation crossed an occurrence, role or action boundary")
	}
}
