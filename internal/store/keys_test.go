package store

import (
	"bytes"
	"errors"
	"testing"
)

func TestKeyLifecycle(t *testing.T) {
	s := openTemp(t)
	hash := bytes.Repeat([]byte{7}, 32)
	k := APIKey{ID: "0123456789abcdef", Name: "sync", ReadOnly: true, CreatedBy: "user:1", CreatedAt: "2026-01-01T00:00:00Z", ExpiresAt: "2026-06-01T00:00:00Z"}
	if err := s.CreateKey(k, hash, Audit{Actor: "user:1", Remote: "192.0.2.1"}); err != nil {
		t.Fatal(err)
	}
	if n := scalar(t, s, "SELECT count(*) FROM audit WHERE actor = 'user:1' AND action = 'key.create' AND target = 'key:0123456789abcdef' AND detail = 'sync'"); n != 1 {
		t.Fatalf("key.create audit rows %d", n)
	}
	ks, err := s.Keys()
	if err != nil || len(ks) != 1 || ks[0] != k {
		t.Fatalf("Keys: %+v %v", ks, err)
	}
	got, h, err := s.LiveKey(k.ID, "2026-05-31T23:59:59Z")
	if err != nil || *got != k || !bytes.Equal(h, hash) {
		t.Fatalf("LiveKey before expiry: %+v %v", got, err)
	}
	for _, now := range []string{"2026-06-01T00:00:00Z", "2027-01-01T00:00:00Z"} {
		if _, _, err := s.LiveKey(k.ID, now); !errors.Is(err, ErrKeyNotFound) {
			t.Errorf("LiveKey at %s: %v, want ErrKeyNotFound", now, err)
		}
	}
	if _, _, err := s.LiveKey("ffffffffffffffff", "2026-01-02T00:00:00Z"); !errors.Is(err, ErrKeyNotFound) {
		t.Fatalf("unknown key: %v", err)
	}
	if err := s.TouchKey(k.ID, "2026-01-02T00:00:00Z"); err != nil {
		t.Fatal(err)
	}
	if ks, _ := s.Keys(); ks[0].LastUsedAt != "2026-01-02T00:00:00Z" {
		t.Fatalf("last used %q", ks[0].LastUsedAt)
	}
	if _, err := s.CreateGrant(Grant{KeyID: k.ID, Lo: 0x100, Hi: 0x100}, Audit{}); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if err := s.RevokeKey(k.ID, Audit{Actor: "user:1"}); err != nil {
			t.Fatal(err)
		}
	}
	if n := scalar(t, s, "SELECT count(*) FROM audit WHERE action = 'key.revoke' AND target = 'key:0123456789abcdef' AND detail = 'sync'"); n != 1 {
		t.Fatalf("key.revoke audit rows %d, want 1 after two revokes", n)
	}
	if _, _, err := s.LiveKey(k.ID, "2026-01-02T00:00:00Z"); !errors.Is(err, ErrKeyNotFound) {
		t.Fatalf("LiveKey after revoke: %v", err)
	}
	if ks, _ := s.Keys(); len(ks) != 1 || ks[0].RevokedAt == "" || scalar(t, s, "SELECT count(*) FROM grants") != 1 {
		t.Fatal("revoke did not keep the row and its grant")
	}
	if err := s.RevokeKey("ffffffffffffffff", Audit{}); !errors.Is(err, ErrKeyNotFound) {
		t.Fatalf("revoke unknown: %v", err)
	}
}

func TestKeyWithoutExpiryIsLive(t *testing.T) {
	s := openTemp(t)
	if err := s.CreateKey(APIKey{ID: "aaaaaaaaaaaaaaaa", Name: "n", CreatedBy: "cli", CreatedAt: "2026-01-01T00:00:00Z"}, []byte("h"), Audit{}); err != nil {
		t.Fatal(err)
	}
	if k, _, err := s.LiveKey("aaaaaaaaaaaaaaaa", "2099-01-01T00:00:00Z"); err != nil || k.ExpiresAt != "" {
		t.Fatalf("LiveKey: %+v %v", k, err)
	}
	if n := scalar(t, s, "SELECT count(*) FROM api_keys WHERE expires_at IS NULL"); n != 1 {
		t.Fatal("empty ExpiresAt not stored as NULL")
	}
}
