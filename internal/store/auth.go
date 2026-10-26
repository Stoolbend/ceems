package store

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

// User is one stored user. No HTTP handler serialises it.
type User struct {
	ID           int64
	Username     string
	PasswordHash []byte
	Admin        bool
	Disabled     bool
	CreatedAt    string
}

// Audit is one audit row; At is set by the store.
type Audit struct{ Actor, Action, Target, Detail, Remote string }

var (
	ErrUserExists   = errors.New("user already exists")
	ErrUserNotFound = errors.New("user not found")
)

func utcNow() string { return time.Now().UTC().Format(time.RFC3339) }

// CreateUser inserts a user and its audit row in one transaction. The username
// is stored lowercased; the audit target is set to user:<id>.
func (s *Store) CreateUser(username string, hash []byte, admin bool, a Audit) (int64, error) {
	at := utcNow()
	tx, err := s.db.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	res, err := tx.Exec("INSERT INTO users (username, password_hash, admin, created_at) VALUES (?, ?, ?, ?)",
		strings.ToLower(username), hash, admin, at)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE constraint failed: users.username") {
			return 0, ErrUserExists
		}
		return 0, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, err
	}
	a.Target = fmt.Sprintf("user:%d", id)
	if err := insertAudit(tx, at, a); err != nil {
		return 0, err
	}
	return id, tx.Commit()
}

// SetPasswordHash replaces a user's hash, deletes all their sessions and
// writes the audit row, in one transaction. The audit target is set to user:<id>.
func (s *Store) SetPasswordHash(username string, hash []byte, a Audit) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var id int64
	err = tx.QueryRow("UPDATE users SET password_hash = ? WHERE username = ? RETURNING id", hash, strings.ToLower(username)).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrUserNotFound
	}
	if err != nil {
		return err
	}
	if _, err := tx.Exec("DELETE FROM sessions WHERE user_id = ?", id); err != nil {
		return err
	}
	a.Target = fmt.Sprintf("user:%d", id)
	if err := insertAudit(tx, utcNow(), a); err != nil {
		return err
	}
	return tx.Commit()
}

const userColumns = "u.id, u.username, u.password_hash, u.admin, u.disabled, u.created_at"

func scanUser(row *sql.Row) (*User, error) {
	u := &User{}
	err := row.Scan(&u.ID, &u.Username, &u.PasswordHash, &u.Admin, &u.Disabled, &u.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrUserNotFound
	}
	if err != nil {
		return nil, err
	}
	return u, nil
}

// UserByUsername looks a user up case-insensitively.
func (s *Store) UserByUsername(username string) (*User, error) {
	return scanUser(s.db.QueryRow("SELECT "+userColumns+" FROM users u WHERE u.username = ?", strings.ToLower(username)))
}

// HasAdmin reports whether an enabled admin exists.
func (s *Store) HasAdmin() (bool, error) {
	var ok bool
	err := s.db.QueryRow("SELECT EXISTS (SELECT 1 FROM users WHERE admin = 1 AND disabled = 0)").Scan(&ok)
	return ok, err
}

// CreateSession stores a session by the SHA-256 of its token. Times are RFC 3339 UTC.
func (s *Store) CreateSession(tokenHash []byte, userID int64, createdAt, expiresAt string) error {
	_, err := s.db.Exec("INSERT INTO sessions (token_hash, user_id, created_at, expires_at) VALUES (?, ?, ?, ?)",
		tokenHash, userID, createdAt, expiresAt)
	return err
}

// DeleteSession deletes a session; an absent one is not an error.
func (s *Store) DeleteSession(tokenHash []byte) error {
	_, err := s.db.Exec("DELETE FROM sessions WHERE token_hash = ?", tokenHash)
	return err
}

// UserBySession returns the enabled user owning an unexpired session, else
// ErrUserNotFound. now is RFC 3339 UTC, compared as text.
func (s *Store) UserBySession(tokenHash []byte, now string) (*User, error) {
	return scanUser(s.db.QueryRow(
		"SELECT "+userColumns+" FROM sessions s JOIN users u ON u.id = s.user_id"+
			" WHERE s.token_hash = ? AND s.expires_at > ? AND u.disabled = 0", tokenHash, now))
}

// insertAudit is the single audit writer.
func insertAudit(tx *sql.Tx, at string, a Audit) error {
	_, err := tx.Exec("INSERT INTO audit (at, actor, action, target, detail, remote) VALUES (?, ?, ?, ?, ?, ?)",
		at, a.Actor, a.Action, a.Target, a.Detail, a.Remote)
	return err
}

// Users returns every user, ordered by username.
func (s *Store) Users() ([]User, error) {
	rows, err := s.db.Query("SELECT " + userColumns + " FROM users u ORDER BY u.username")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []User
	for rows.Next() {
		var u User
		if err := rows.Scan(&u.ID, &u.Username, &u.PasswordHash, &u.Admin, &u.Disabled, &u.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, u)
	}
	return out, rows.Err()
}

// UserByID looks a user up by id.
func (s *Store) UserByID(id int64) (*User, error) {
	return scanUser(s.db.QueryRow("SELECT "+userColumns+" FROM users u WHERE u.id = ?", id))
}

// SetDisabled disables or enables a user and writes the audit row, in one
// transaction. Disabling also deletes every session of the user. The audit
// target is set to user:<id>.
func (s *Store) SetDisabled(id int64, disabled bool, a Audit) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	res, err := tx.Exec("UPDATE users SET disabled = ? WHERE id = ?", disabled, id)
	if err != nil {
		return err
	}
	if n, err := res.RowsAffected(); err != nil {
		return err
	} else if n == 0 {
		return ErrUserNotFound
	}
	if disabled {
		if _, err := tx.Exec("DELETE FROM sessions WHERE user_id = ?", id); err != nil {
			return err
		}
	}
	a.Target = fmt.Sprintf("user:%d", id)
	if err := insertAudit(tx, utcNow(), a); err != nil {
		return err
	}
	return tx.Commit()
}

// Grant gives a user or an API key access to pages Lo..Hi inclusive. Exactly one
// of UserID and KeyID is set; KeyID is "" for a user grant.
type Grant struct {
	ID     int64
	UserID *int64
	KeyID  string
	Lo, Hi int
	Write  bool
}

var ErrGrantNotFound = errors.New("grant not found")

// owner is the grant's owner as written in audit detail: user:<id> or key:<id>.
func (g Grant) owner() string {
	if g.UserID != nil {
		return fmt.Sprintf("user:%d", *g.UserID)
	}
	return "key:" + g.KeyID
}

// auditDetail is "<owner> <lo>-<hi> <access>", e.g. "user:3 100-1FF write".
func (g Grant) auditDetail() string {
	access := "read"
	if g.Write {
		access = "write"
	}
	return fmt.Sprintf("%s %03X-%03X %s", g.owner(), g.Lo, g.Hi, access)
}

const grantColumns = "id, user_id, coalesce(key_id, ''), lo, hi, access = 'write'"

func (s *Store) grants(where string, args ...any) ([]Grant, error) {
	rows, err := s.db.Query("SELECT "+grantColumns+" FROM grants"+where+" ORDER BY lo, hi, id", args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Grant
	for rows.Next() {
		var g Grant
		if err := rows.Scan(&g.ID, &g.UserID, &g.KeyID, &g.Lo, &g.Hi, &g.Write); err != nil {
			return nil, err
		}
		out = append(out, g)
	}
	return out, rows.Err()
}

// GrantsForUser returns a user's grants.
func (s *Store) GrantsForUser(id int64) ([]Grant, error) {
	return s.grants(" WHERE user_id = ?", id)
}

// GrantsForKey returns an API key's grants.
func (s *Store) GrantsForKey(id string) ([]Grant, error) {
	return s.grants(" WHERE key_id = ?", id)
}

// AllGrants returns every grant, of users and keys.
func (s *Store) AllGrants() ([]Grant, error) {
	return s.grants("")
}

// CreateGrant inserts a grant and its audit row in one transaction. The audit
// target is set to grant:<id> and the detail to the grant, e.g. "user:3 100-1FF write".
func (s *Store) CreateGrant(g Grant, a Audit) (int64, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	access := "read"
	if g.Write {
		access = "write"
	}
	var key any
	if g.KeyID != "" {
		key = g.KeyID
	}
	res, err := tx.Exec("INSERT INTO grants (user_id, key_id, lo, hi, access) VALUES (?, ?, ?, ?, ?)", g.UserID, key, g.Lo, g.Hi, access)
	if err != nil {
		return 0, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, err
	}
	a.Target, a.Detail = fmt.Sprintf("grant:%d", id), g.auditDetail()
	if err := insertAudit(tx, utcNow(), a); err != nil {
		return 0, err
	}
	return id, tx.Commit()
}

// DeleteGrant deletes a grant and writes its audit row in one transaction, or
// returns ErrGrantNotFound. The audit target and detail are as for CreateGrant.
func (s *Store) DeleteGrant(id int64, a Audit) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var g Grant
	err = tx.QueryRow("DELETE FROM grants WHERE id = ? RETURNING "+grantColumns, id).
		Scan(&g.ID, &g.UserID, &g.KeyID, &g.Lo, &g.Hi, &g.Write)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrGrantNotFound
	}
	if err != nil {
		return err
	}
	a.Target, a.Detail = fmt.Sprintf("grant:%d", id), g.auditDetail()
	if err := insertAudit(tx, utcNow(), a); err != nil {
		return err
	}
	return tx.Commit()
}
