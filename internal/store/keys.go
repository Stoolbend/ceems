package store

import (
	"database/sql"
	"errors"
)

// APIKey is one stored API key without its secret hash. The time fields are
// RFC 3339 UTC; ExpiresAt, RevokedAt and LastUsedAt are "" when NULL.
type APIKey struct {
	ID, Name                         string
	ReadOnly                         bool
	CreatedBy, CreatedAt             string
	ExpiresAt, RevokedAt, LastUsedAt string
}

var ErrKeyNotFound = errors.New("api key not found")

const keyColumns = "id, name, read_only, created_by, created_at, coalesce(expires_at, ''), coalesce(revoked_at, ''), coalesce(last_used_at, '')"

type scanner interface{ Scan(dest ...any) error }

func scanKey(row scanner, extra ...any) (APIKey, error) {
	var k APIKey
	err := row.Scan(append([]any{&k.ID, &k.Name, &k.ReadOnly, &k.CreatedBy, &k.CreatedAt, &k.ExpiresAt, &k.RevokedAt, &k.LastUsedAt}, extra...)...)
	return k, err
}

// nullable maps "" to NULL.
func nullable(s string) any {
	if s == "" {
		return nil
	}
	return s
}

// Keys returns every key, revoked ones included, ordered by creation. It never reads secret_hash.
func (s *Store) Keys() ([]APIKey, error) {
	rows, err := s.db.Query("SELECT " + keyColumns + " FROM api_keys ORDER BY created_at, id")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []APIKey
	for rows.Next() {
		k, err := scanKey(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, k)
	}
	return out, rows.Err()
}

// CreateKey inserts k with the SHA-256 of its secret and the audit row (key.create,
// target key:<id>, detail the name) in one transaction. RevokedAt and LastUsedAt are ignored.
func (s *Store) CreateKey(k APIKey, secretHash []byte, a Audit) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec("INSERT INTO api_keys (id, secret_hash, name, read_only, created_by, created_at, expires_at) VALUES (?, ?, ?, ?, ?, ?, ?)",
		k.ID, secretHash, k.Name, k.ReadOnly, k.CreatedBy, k.CreatedAt, nullable(k.ExpiresAt)); err != nil {
		return err
	}
	a.Action, a.Target, a.Detail = "key.create", "key:"+k.ID, k.Name
	if err := insertAudit(tx, utcNow(), a); err != nil {
		return err
	}
	return tx.Commit()
}

// RevokeKey sets revoked_at and writes the audit row (key.revoke) in one transaction.
// An unknown id is ErrKeyNotFound; an already revoked key is a no-op with no audit row.
// The row and its grants are kept.
func (s *Store) RevokeKey(id string, a Audit) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	at := utcNow()
	var name string
	err = tx.QueryRow("UPDATE api_keys SET revoked_at = ? WHERE id = ? AND revoked_at IS NULL RETURNING name", at, id).Scan(&name)
	if errors.Is(err, sql.ErrNoRows) {
		var exists bool
		if err := tx.QueryRow("SELECT EXISTS (SELECT 1 FROM api_keys WHERE id = ?)", id).Scan(&exists); err != nil {
			return err
		}
		if !exists {
			return ErrKeyNotFound
		}
		return nil
	}
	if err != nil {
		return err
	}
	a.Action, a.Target, a.Detail = "key.revoke", "key:"+id, name
	if err := insertAudit(tx, at, a); err != nil {
		return err
	}
	return tx.Commit()
}

// LiveKey returns a key that is neither revoked nor expired at now (RFC 3339 UTC,
// compared as text) with its secret hash, else ErrKeyNotFound.
func (s *Store) LiveKey(id, now string) (*APIKey, []byte, error) {
	var hash []byte
	k, err := scanKey(s.db.QueryRow("SELECT "+keyColumns+", secret_hash FROM api_keys"+
		" WHERE id = ? AND revoked_at IS NULL AND (expires_at IS NULL OR expires_at > ?)", id, now), &hash)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil, ErrKeyNotFound
	}
	if err != nil {
		return nil, nil, err
	}
	return &k, hash, nil
}

// TouchKey records a successful authentication; it writes no audit row.
func (s *Store) TouchKey(id, now string) error {
	_, err := s.db.Exec("UPDATE api_keys SET last_used_at = ? WHERE id = ?", now, id)
	return err
}
