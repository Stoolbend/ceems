// Package auth holds password hashing, session tokens, login and the
// authorisation check. It knows nothing about HTTP.
package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"math/big"
	"regexp"
	"strings"
	"sync"

	"golang.org/x/crypto/bcrypt"

	"ceems/internal/store"
)

// BcryptCost is the cost of every stored password hash.
const BcryptCost = 11

// ErrInvalidCredentials is returned by Login for any failed login.
var ErrInvalidCredentials = errors.New("invalid credentials")

// HashPassword hashes a 12-72 byte password with bcrypt at BcryptCost.
func HashPassword(pw string) ([]byte, error) {
	if len(pw) < 12 || len(pw) > 72 {
		return nil, fmt.Errorf("password must be 12-72 bytes, got %d", len(pw))
	}
	return bcrypt.GenerateFromPassword([]byte(pw), BcryptCost)
}

const alphabet = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789"

// RandomPassword returns 20 characters from [A-Za-z0-9], from crypto/rand.
func RandomPassword() string {
	b := make([]byte, 20)
	for i := range b {
		n, err := rand.Int(rand.Reader, big.NewInt(int64(len(alphabet))))
		if err != nil {
			panic(err) // crypto/rand does not fail on supported platforms
		}
		b[i] = alphabet[n.Int64()]
	}
	return string(b)
}

// NewToken returns a session token (32 random bytes, base64url) and its SHA-256.
func NewToken() (token string, hash []byte) {
	b := make([]byte, 32)
	rand.Read(b)
	token = base64.RawURLEncoding.EncodeToString(b)
	return token, HashToken(token)
}

// HashToken returns the SHA-256 of a session token, the form the store keeps.
func HashToken(token string) []byte {
	h := sha256.Sum256([]byte(token))
	return h[:]
}

// dummyHash is compared against for an unknown user so the time matches a wrong password.
var dummyHash = sync.OnceValue(func() []byte {
	h, err := bcrypt.GenerateFromPassword([]byte("ceems-dummy-password"), BcryptCost)
	if err != nil {
		panic(err)
	}
	return h
})

// Login checks username/password. Unknown user compares against a package-level
// dummy hash so the time and the error match a wrong password. A disabled user
// is refused the same way.
func Login(s *store.Store, username, password string) (*store.User, error) {
	if len(password) > 72 {
		return nil, ErrInvalidCredentials
	}
	u, err := s.UserByUsername(username)
	if errors.Is(err, store.ErrUserNotFound) {
		bcrypt.CompareHashAndPassword(dummyHash(), []byte(password))
		return nil, ErrInvalidCredentials
	}
	if err != nil {
		return nil, err
	}
	if err := bcrypt.CompareHashAndPassword(u.PasswordHash, []byte(password)); err != nil {
		return nil, ErrInvalidCredentials
	}
	if u.Disabled {
		return nil, ErrInvalidCredentials
	}
	return u, nil
}

var validUsername = regexp.MustCompile(`^[A-Za-z0-9._-]{1,64}$`)

// CheckUsername validates a username and returns its stored (lowercased) form.
func CheckUsername(name string) (string, error) {
	if !validUsername.MatchString(name) {
		return "", errors.New("username: must be 1-64 characters of letters, digits, '.', '_' or '-'")
	}
	return strings.ToLower(name), nil
}

// UserPrincipal builds the principal for an enabled user, reading grants now.
func UserPrincipal(s *store.Store, u *store.User) (Principal, error) {
	gs, err := s.GrantsForUser(u.ID)
	if err != nil {
		return Principal{}, err
	}
	p := Principal{Actor: fmt.Sprintf("user:%d", u.ID), Admin: u.Admin}
	for _, g := range gs {
		p.Grants = append(p.Grants, Grant{Lo: g.Lo, Hi: g.Hi, Write: g.Write})
	}
	return p, nil
}

// NewKey returns a key id (8 random bytes, 16 lowercase hex), the token shown once
// ("yfk_<id>_<secret>", secret 32 random bytes base64url, no padding) and SHA-256(secret).
func NewKey() (id, token string, secretHash []byte) {
	b := make([]byte, 8)
	rand.Read(b)
	id = hex.EncodeToString(b)
	secret, hash := NewToken()
	return id, "yfk_" + id + "_" + secret, hash
}

// bearerShape is "Bearer yfk_<id>_<secret>"; base64url's own '_' may appear in the secret.
var bearerShape = regexp.MustCompile(`^Bearer yfk_([0-9a-f]{16})_([A-Za-z0-9_-]{43})$`)

// ParseBearer splits "Bearer yfk_<id>_<secret>"; ok is false for any other shape.
func ParseBearer(header string) (id, secret string, ok bool) {
	m := bearerShape.FindStringSubmatch(header)
	if m == nil {
		return "", "", false
	}
	return m[1], m[2], true
}

// KeyPrincipal authenticates a bearer header: ErrInvalidCredentials for a malformed header,
// unknown id, revoked or expired key, or a secret whose SHA-256 differs. On success it
// loads the key's grants. A key is never admin (ADR 0003). now is RFC 3339 UTC.
func KeyPrincipal(s *store.Store, header, now string) (Principal, *store.APIKey, error) {
	id, secret, ok := ParseBearer(header)
	if !ok {
		return Principal{}, nil, ErrInvalidCredentials
	}
	k, stored, err := s.LiveKey(id, now)
	if errors.Is(err, store.ErrKeyNotFound) {
		return Principal{}, nil, ErrInvalidCredentials
	}
	if err != nil {
		return Principal{}, nil, err
	}
	match := subtle.ConstantTimeCompare(HashToken(secret), stored) == 1
	if !match {
		return Principal{}, nil, ErrInvalidCredentials
	}
	gs, err := s.GrantsForKey(id)
	if err != nil {
		return Principal{}, nil, err
	}
	p := Principal{Actor: "key:" + id, ReadOnly: k.ReadOnly}
	for _, g := range gs {
		p.Grants = append(p.Grants, Grant{Lo: g.Lo, Hi: g.Hi, Write: g.Write})
	}
	return p, k, nil
}

// Grant gives access to pages Lo..Hi inclusive; Write implies read.
type Grant struct {
	Lo, Hi int
	Write  bool
}

// Principal is who a request acts as.
type Principal struct {
	Actor    string // "user:3"; used in logs and audit
	Admin    bool
	ReadOnly bool
	Grants   []Grant
}

// Can reports whether p may read (or, with write, write) page mpp.
func (p Principal) Can(mpp int, write bool) bool {
	if p.Admin {
		return true
	}
	if write && p.ReadOnly {
		return false
	}
	for _, g := range p.Grants {
		if g.Lo <= mpp && mpp <= g.Hi && (!write || g.Write) {
			return true
		}
	}
	return false
}
