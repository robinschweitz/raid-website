package auth

import (
	"context"
	"testing"

	"raid-website/internal/player"
	"raid-website/internal/validation"
)

func sessionEndFixture(t *testing.T) (*Service, *player.Repo, *player.Player, string, string) {
	t.Helper()
	ctx := context.Background()
	bdb := reliabilityDB(t)
	players := player.NewRepo(bdb.DB)
	svc := NewService(players, NewRepo(bdb.DB))
	if err := svc.BootstrapAdmin(ctx, "owner", "initial-password"); err != nil {
		t.Fatal(err)
	}
	owner, err := players.FindByUsername(ctx, "owner")
	if err != nil {
		t.Fatal(err)
	}
	seedSecondAdmin(t, players)
	device1, _, err := svc.Login(ctx, "owner", "initial-password")
	if err != nil {
		t.Fatal(err)
	}
	device2, _, err := svc.Login(ctx, "owner", "initial-password")
	if err != nil {
		t.Fatal(err)
	}
	return svc, players, owner, device1, device2
}

func isAuthenticated(t *testing.T, svc *Service, token string) bool {
	t.Helper()
	p, err := svc.Authenticate(context.Background(), token)
	if err != nil {
		t.Fatal(err)
	}
	return p != nil
}

func TestReqAuth017_AC01(t *testing.T) {
	svc, _, owner, device1, device2 := sessionEndFixture(t)
	if err := svc.ChangeOwnPassword(context.Background(), owner.ID, device1, "", "initial-password", "replacement-password"); err != nil {
		t.Fatal(err)
	}
	if !isAuthenticated(t, svc, device1) {
		t.Fatal("current device was logged out")
	}
	if isAuthenticated(t, svc, device2) {
		t.Fatal("other device is still logged in")
	}
}

func TestReqAuth017_OwnPasswordFailureKeepsSessions(t *testing.T) {
	svc, _, owner, device1, device2 := sessionEndFixture(t)
	ctx := context.Background()
	if err := svc.ChangeOwnPassword(ctx, owner.ID, device1, "", "wrong-password-x", "replacement-password"); err == nil {
		t.Fatal("wrong current password accepted")
	}
	if err := svc.ChangeOwnPassword(ctx, owner.ID, device1, "", "initial-password", "short"); err == nil {
		t.Fatal("weak password accepted")
	}
	if !isAuthenticated(t, svc, device1) || !isAuthenticated(t, svc, device2) {
		t.Fatal("rejected change ended sessions")
	}
}

func TestReqAuth017_AC02(t *testing.T) {
	for _, change := range []struct {
		name     string
		username string
		password string
		role     validation.Role
	}{
		{"role", "owner", "", validation.RoleRaidleader},
		{"login name", "renamed", "", validation.RoleAdmin},
		{"password", "owner", "replacement-password", validation.RoleAdmin},
	} {
		t.Run(change.name, func(t *testing.T) {
			svc, _, owner, device1, device2 := sessionEndFixture(t)
			if _, err := svc.SetCredentials(context.Background(), owner.ID, change.username, change.password, change.role, 0); err != nil {
				t.Fatal(err)
			}
			if isAuthenticated(t, svc, device1) || isAuthenticated(t, svc, device2) {
				t.Fatal("a session of the changed player survived")
			}
		})
	}
}

func TestReqAuth017_AC03(t *testing.T) {
	svc, _, owner, device1, device2 := sessionEndFixture(t)
	if _, err := svc.SetCredentials(context.Background(), owner.ID, "OWNER ", "", validation.RoleAdmin, 0); err != nil {
		t.Fatal(err)
	}
	if !isAuthenticated(t, svc, device1) || !isAuthenticated(t, svc, device2) {
		t.Fatal("unchanged account save ended sessions")
	}
}

func TestReqAuth017_AC04(t *testing.T) {
	for _, scenario := range []string{"own password", "admin credentials"} {
		t.Run(scenario, func(t *testing.T) {
			svc, players, owner, device1, device2 := sessionEndFixture(t)
			ctx := context.Background()
			if _, err := svc.sessions.db.ExecContext(ctx, "CREATE TRIGGER reject_session_delete BEFORE DELETE ON sessions BEGIN SELECT RAISE(ABORT, 'injected failure'); END"); err != nil {
				t.Fatal(err)
			}
			var err error
			if scenario == "own password" {
				err = svc.ChangeOwnPassword(ctx, owner.ID, device1, "", "initial-password", "replacement-password")
			} else {
				_, err = svc.SetCredentials(ctx, owner.ID, "renamed", "replacement-password", validation.RoleMember, 0)
			}
			if err == nil {
				t.Fatal("change succeeded although the session delete failed")
			}
			after, err := players.Get(ctx, owner.ID)
			if err != nil {
				t.Fatal(err)
			}
			if after.PasswordHash != owner.PasswordHash || *after.Username != *owner.Username || after.Role != owner.Role {
				t.Fatal("failed change left partial account changes")
			}
			if !isAuthenticated(t, svc, device1) || !isAuthenticated(t, svc, device2) {
				t.Fatal("failed change ended sessions")
			}
		})
	}
}
