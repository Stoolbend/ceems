package auth

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"regexp"
	"strings"
	"sync"
	"testing"

	"ceems/internal/store"
)

const testPassword = "correct horse battery"

var testHash = sync.OnceValue(func() []byte {
	h, err := HashPassword(testPassword)
	if err != nil {
		panic(err)
	}
	return h
})

func TestHashPasswordLengthAndCost(t *testing.T) {
	for _, n := range []int{11, 73} {
		if _, err := HashPassword(strings.Repeat("a", n)); err == nil {
			t.Errorf("%d bytes: want error", n)
		}
	}
	if !bytes.HasPrefix(testHash(), []byte("$2a$11$")) {
		t.Fatalf("hash prefix %q", testHash()[:7])
	}
}

func TestRandomPassword(t *testing.T) {
	re := regexp.MustCompile(`^[A-Za-z0-9]{20}$`)
	a, b := RandomPassword(), RandomPassword()
	if !re.MatchString(a) || !re.MatchString(b) || a == b {
		t.Fatal("RandomPassword must give distinct 20-character alphanumeric strings")
	}
}

func TestNewToken(t *testing.T) {
	tok, hash := NewToken()
	raw, err := base64.RawURLEncoding.DecodeString(tok)
	if err != nil || len(raw) != 32 {
		t.Fatalf("token decodes to %d bytes, err %v", len(raw), err)
	}
	want := sha256.Sum256([]byte(tok))
	if !bytes.Equal(hash, want[:]) || !bytes.Equal(HashToken(tok), want[:]) {
		t.Fatal("token hash is not sha256(token)")
	}
}

func TestLogin(t *testing.T) {
	s, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if _, err := s.CreateUser("conor", testHash(), true, store.Audit{Actor: "test"}); err != nil {
		t.Fatal(err)
	}
	if u, err := Login(s, "Conor", testPassword); err != nil || u.Username != "conor" {
		t.Fatalf("good login: user %v err %v", u != nil, err)
	}
	for name, c := range map[string][2]string{
		"wrong password": {"conor", testPassword + "x"},
		"unknown user":   {"nobody", testPassword},
		"over 72 bytes":  {"conor", strings.Repeat("a", 73)},
	} {
		if u, err := Login(s, c[0], c[1]); u != nil || !errors.Is(err, ErrInvalidCredentials) {
			t.Errorf("%s: got user %v err %v", name, u != nil, err)
		}
	}
}

func TestCan(t *testing.T) {
	read := Principal{Grants: []Grant{{Lo: 0x200, Hi: 0x2FF}}}
	write := Principal{Grants: []Grant{{Lo: 0x200, Hi: 0x2FF, Write: true}}}
	roKey := Principal{ReadOnly: true, Grants: write.Grants}
	cases := []struct {
		name  string
		p     Principal
		mpp   int
		write bool
		want  bool
	}{
		{"admin reads anything", Principal{Admin: true}, 0x800, false, true},
		{"admin writes anything", Principal{Admin: true}, 0x100, true, true},
		{"no grants reads nothing", Principal{}, 0x100, false, false},
		{"read grant lo", read, 0x200, false, true},
		{"read grant hi", read, 0x2FF, false, true},
		{"read grant lo-1", read, 0x1FF, false, false},
		{"read grant hi+1", read, 0x300, false, false},
		{"read grant cannot write", read, 0x250, true, false},
		{"write grant writes", write, 0x250, true, true},
		{"write grant reads", write, 0x250, false, true},
		{"read-only key with write grant cannot write", roKey, 0x250, true, false},
		{"read-only key with write grant reads", roKey, 0x250, false, true},
		{"second grant covers", Principal{Grants: []Grant{{0x100, 0x100, false}, {0x300, 0x3FF, true}}}, 0x310, true, true},
	}
	for _, c := range cases {
		if got := c.p.Can(c.mpp, c.write); got != c.want {
			t.Errorf("%s: Can(%03X, %v) = %v", c.name, c.mpp, c.write, got)
		}
	}
}

func TestCheckUsername(t *testing.T) {
	if got, err := CheckUsername("Conor.M_1-x"); err != nil || got != "conor.m_1-x" {
		t.Fatalf("good name: %q %v", got, err)
	}
	for _, bad := range []string{"", "a b", "conor!", strings.Repeat("a", 65)} {
		if _, err := CheckUsername(bad); err == nil || err.Error() != "username: must be 1-64 characters of letters, digits, '.', '_' or '-'" {
			t.Errorf("%q: %v", bad, err)
		}
	}
}

func TestUserPrincipalReadsGrants(t *testing.T) {
	s, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	id, _ := s.CreateUser("conor", testHash(), false, store.Audit{Actor: "test"})
	s.CreateGrant(store.Grant{UserID: &id, Lo: 0x100, Hi: 0x1FF, Write: true}, store.Audit{Actor: "test"})
	s.CreateGrant(store.Grant{UserID: &id, Lo: 0x300, Hi: 0x300}, store.Audit{Actor: "test"})
	u, _ := s.UserByID(id)
	p, err := UserPrincipal(s, u)
	if err != nil || p.Actor != "user:1" || p.Admin || p.ReadOnly || len(p.Grants) != 2 ||
		p.Grants[0] != (Grant{0x100, 0x1FF, true}) || p.Grants[1] != (Grant{0x300, 0x300, false}) {
		t.Fatalf("principal %+v err %v", p, err)
	}
}

// Tokens are compared and shape-checked here, never printed.
func TestNewKeyAndParseBearer(t *testing.T) {
	id, token, hash := NewKey()
	id2, token2, _ := NewKey()
	if !regexp.MustCompile(`^[0-9a-f]{16}$`).MatchString(id) || id == id2 || token == token2 {
		t.Fatal("key ids are not 16 lowercase hex, or repeat")
	}
	gotID, secret, ok := ParseBearer("Bearer " + token)
	if !ok || gotID != id || len(secret) != 43 || token != "yfk_"+id+"_"+secret {
		t.Fatal("ParseBearer did not split a fresh token")
	}
	if raw, err := base64.RawURLEncoding.DecodeString(secret); err != nil || len(raw) != 32 {
		t.Fatal("secret is not 32 bytes of base64url")
	}
	if want := sha256.Sum256([]byte(secret)); !bytes.Equal(hash, want[:]) {
		t.Fatal("hash is not sha256(secret)")
	}
	for _, bad := range []string{"", token, "bearer " + token, "Bearer  " + token, "Bearer " + token + "x", "Bearer " + token[:len(token)-1],
		"Bearer yfk_" + strings.ToUpper("abcdefabcdefabcd") + "_" + secret, "Bearer yfk_abc_" + secret, "Bearer " + token + "\n"} {
		if _, _, ok := ParseBearer(bad); ok {
			t.Errorf("ParseBearer accepted a malformed header (case length %d)", len(bad))
		}
	}
}

func TestKeyPrincipal(t *testing.T) {
	s, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	id, token, hash := NewKey()
	if err := s.CreateKey(store.APIKey{ID: id, Name: "k", ReadOnly: true, CreatedBy: "user:1", CreatedAt: "2026-01-01T00:00:00Z"}, hash, store.Audit{}); err != nil {
		t.Fatal(err)
	}
	s.CreateGrant(store.Grant{KeyID: id, Lo: 0x100, Hi: 0x1FF, Write: true}, store.Audit{})
	p, k, err := KeyPrincipal(s, "Bearer "+token, "2026-01-02T00:00:00Z")
	if err != nil || k.ID != id || p.Actor != "key:"+id || p.Admin || !p.ReadOnly || len(p.Grants) != 1 || p.Grants[0] != (Grant{0x100, 0x1FF, true}) {
		t.Fatalf("principal %+v err %v", p, err)
	}
	_, other, _ := NewKey()
	for _, h := range []string{"Bearer junk", "Bearer yfk_" + id + "_" + other[len("yfk_")+17:]} {
		if _, _, err := KeyPrincipal(s, h, "2026-01-02T00:00:00Z"); !errors.Is(err, ErrInvalidCredentials) {
			t.Errorf("bad header: %v", err)
		}
	}
}
