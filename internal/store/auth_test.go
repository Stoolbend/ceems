package store

import (
	"errors"
	"fmt"
	"testing"
)

func TestAuthTablesExist(t *testing.T) {
	s := openTemp(t)
	for _, table := range []string{"users", "sessions", "audit"} {
		if n := scalar(t, s, "SELECT count(*) FROM sqlite_master WHERE type = 'table' AND name = '"+table+"'"); n != 1 {
			t.Errorf("table %s missing", table)
		}
	}
}

func TestUsersAreCaseInsensitive(t *testing.T) {
	s := openTemp(t)
	if _, err := s.UserByUsername("conor"); !errors.Is(err, ErrUserNotFound) {
		t.Fatalf("unknown user: %v", err)
	}
	if ok, err := s.HasAdmin(); ok || err != nil {
		t.Fatalf("HasAdmin on empty db: %v %v", ok, err)
	}
	id, err := s.CreateUser("Conor", []byte("h"), true, Audit{Actor: "cli", Action: "user.create"})
	if err != nil {
		t.Fatal(err)
	}
	u, err := s.UserByUsername("CONOR")
	if err != nil || u.ID != id || u.Username != "conor" || !u.Admin || u.Disabled {
		t.Fatalf("lookup: found %v err %v", u != nil, err)
	}
	if _, err := s.CreateUser("CONOR", []byte("h"), true, Audit{Actor: "cli"}); !errors.Is(err, ErrUserExists) {
		t.Fatalf("duplicate: %v", err)
	}
	if n := scalar(t, s, "SELECT count(*) FROM audit"); n != 1 {
		t.Fatalf("audit rows %d", n)
	}
	if ok, err := s.HasAdmin(); !ok || err != nil {
		t.Fatalf("HasAdmin: %v %v", ok, err)
	}
	if err := s.SetPasswordHash("nobody", []byte("h"), Audit{Actor: "cli"}); !errors.Is(err, ErrUserNotFound) {
		t.Fatalf("reset unknown: %v", err)
	}
}

func TestSessionLookup(t *testing.T) {
	s := openTemp(t)
	id, err := s.CreateUser("conor", []byte("h"), false, Audit{Actor: "cli"})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.CreateSession([]byte("tok"), id, "2026-01-01T00:00:00Z", "2026-01-15T00:00:00Z"); err != nil {
		t.Fatal(err)
	}
	if u, err := s.UserBySession([]byte("tok"), "2026-01-10T00:00:00Z"); err != nil || u.ID != id {
		t.Fatalf("live session: found %v err %v", u != nil, err)
	}
	for name, c := range map[string]struct{ tok, now string }{
		"unknown token": {"other", "2026-01-10T00:00:00Z"},
		"expired":       {"tok", "2026-01-15T00:00:00Z"},
	} {
		if _, err := s.UserBySession([]byte(c.tok), c.now); !errors.Is(err, ErrUserNotFound) {
			t.Errorf("%s: %v", name, err)
		}
	}
	if err := s.DeleteSession([]byte("tok")); err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteSession([]byte("tok")); err != nil {
		t.Fatalf("deleting an absent session: %v", err)
	}
}

func TestUsersAndUserByID(t *testing.T) {
	s := openTemp(t)
	b, _ := s.CreateUser("bob", []byte("h"), false, Audit{Actor: "cli"})
	a, _ := s.CreateUser("alice", []byte("h"), true, Audit{Actor: "cli"})
	us, err := s.Users()
	if err != nil || len(us) != 2 || us[0].ID != a || us[1].ID != b {
		t.Fatalf("Users: %d users, err %v", len(us), err)
	}
	if u, err := s.UserByID(b); err != nil || u.Username != "bob" {
		t.Fatalf("UserByID: found %v err %v", u != nil, err)
	}
	if _, err := s.UserByID(99); !errors.Is(err, ErrUserNotFound) {
		t.Fatalf("unknown id: %v", err)
	}
}

// F
func TestSetDisabledDeletesSessions(t *testing.T) {
	s := openTemp(t)
	id, _ := s.CreateUser("conor", []byte("h"), false, Audit{Actor: "cli"})
	other, _ := s.CreateUser("other", []byte("h"), false, Audit{Actor: "cli"})
	for i, uid := range []int64{id, id, other} {
		if err := s.CreateSession([]byte{byte(i)}, uid, "2026-01-01T00:00:00Z", "2099-01-01T00:00:00Z"); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.SetDisabled(id, true, Audit{Actor: "user:9", Action: "user.disable"}); err != nil {
		t.Fatal(err)
	}
	if n := scalar(t, s, "SELECT count(*) FROM sessions"); n != 1 {
		t.Fatalf("sessions left %d, want only the other user's", n)
	}
	if u, _ := s.UserByID(id); !u.Disabled {
		t.Fatal("user not disabled")
	}
	if n := scalar(t, s, "SELECT count(*) FROM audit WHERE action = 'user.disable' AND target = 'user:1' AND actor = 'user:9'"); n != 1 {
		t.Fatalf("disable audit rows %d", n)
	}
	if err := s.SetDisabled(id, false, Audit{Actor: "user:9", Action: "user.enable"}); err != nil {
		t.Fatal(err)
	}
	if u, _ := s.UserByID(id); u.Disabled {
		t.Fatal("user not enabled")
	}
	if err := s.SetDisabled(99, true, Audit{Actor: "user:9"}); !errors.Is(err, ErrUserNotFound) {
		t.Fatalf("unknown user: %v", err)
	}
	if n := scalar(t, s, "SELECT count(*) FROM audit"); n != 4 { // two creates, disable, enable
		t.Fatalf("audit rows %d", n)
	}
}

func TestGrants(t *testing.T) {
	s := openTemp(t)
	uid, _ := s.CreateUser("conor", []byte("h"), false, Audit{Actor: "cli"})
	id, err := s.CreateGrant(Grant{UserID: &uid, Lo: 0x100, Hi: 0x1FF, Write: true}, Audit{Actor: "user:9", Action: "grant.create"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateGrant(Grant{UserID: &uid, Lo: 0x300, Hi: 0x300}, Audit{Actor: "user:9", Action: "grant.create"}); err != nil {
		t.Fatal(err)
	}
	gs, err := s.GrantsForUser(uid)
	if err != nil || len(gs) != 2 || gs[0].ID != id || *gs[0].UserID != uid || gs[0].KeyID != "" || gs[0].Lo != 0x100 || gs[0].Hi != 0x1FF || !gs[0].Write || gs[1].Write {
		t.Fatalf("GrantsForUser %+v err %v", gs, err)
	}
	if all, err := s.AllGrants(); err != nil || len(all) != 2 {
		t.Fatalf("AllGrants %d err %v", len(all), err)
	}
	if ks, err := s.GrantsForKey("0123456789abcdef"); err != nil || len(ks) != 0 {
		t.Fatalf("GrantsForKey %d err %v", len(ks), err)
	}
	var detail string
	s.db.QueryRow("SELECT target || ' ' || detail FROM audit WHERE action = 'grant.create' ORDER BY id LIMIT 1").Scan(&detail)
	if want := fmt.Sprintf("grant:%d user:%d 100-1FF write", id, uid); detail != want {
		t.Fatalf("create audit %q want %q", detail, want)
	}
	if err := s.DeleteGrant(id, Audit{Actor: "user:9", Action: "grant.delete"}); err != nil {
		t.Fatal(err)
	}
	s.db.QueryRow("SELECT target || ' ' || detail FROM audit WHERE action = 'grant.delete'").Scan(&detail)
	if want := fmt.Sprintf("grant:%d user:%d 100-1FF write", id, uid); detail != want {
		t.Fatalf("delete audit %q want %q", detail, want)
	}
	if err := s.DeleteGrant(id, Audit{Actor: "user:9", Action: "grant.delete"}); !errors.Is(err, ErrGrantNotFound) {
		t.Fatalf("second delete: %v", err)
	}
	if n := scalar(t, s, "SELECT count(*) FROM audit WHERE action LIKE 'grant.%'"); n != 3 {
		t.Fatalf("grant audit rows %d", n)
	}
}

// The SQL CHECKs are the backstop behind the handler's validation.
func TestGrantChecks(t *testing.T) {
	s := openTemp(t)
	uid, _ := s.CreateUser("conor", []byte("h"), false, Audit{Actor: "cli"})
	for name, g := range map[string]Grant{
		"hi < lo":   {UserID: &uid, Lo: 0x200, Hi: 0x1FF},
		"lo 0FF":    {UserID: &uid, Lo: 0xFF, Hi: 0x100},
		"hi 900":    {UserID: &uid, Lo: 0x100, Hi: 0x900},
		"no owner":  {Lo: 0x100, Hi: 0x100},
		"two owner": {UserID: &uid, KeyID: "k", Lo: 0x100, Hi: 0x100},
	} {
		if _, err := s.CreateGrant(g, Audit{Actor: "test"}); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
	if n := scalar(t, s, "SELECT count(*) FROM grants") + scalar(t, s, "SELECT count(*) FROM audit WHERE actor = 'test'"); n != 0 {
		t.Fatalf("refused grants left %d rows", n)
	}
	if _, err := s.CreateGrant(Grant{UserID: &uid, Lo: 0x100, Hi: 0x100}, Audit{Actor: "test"}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec("DELETE FROM users"); err != nil {
		t.Fatal(err)
	}
	if n := scalar(t, s, "SELECT count(*) FROM grants"); n != 0 {
		t.Fatalf("grants not cascaded on user delete: %d", n)
	}
}
