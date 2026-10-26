// Package store owns the SQLite database and its schema migrations.
package store

import (
	"bytes"
	"crypto/sha256"
	"database/sql"
	"embed"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"net/url"
	"path/filepath"
	"regexp"
	"sort"
	"time"

	"ceems/internal/tti"

	_ "modernc.org/sqlite"
)

//go:embed migrations/*.sql
var embedded embed.FS

// Store is the open application database.
type Store struct {
	db           *sql.DB
	onPageChange func()
}

// OnPageChange registers fn, called after each committed page change from SavePage,
// CreatePage, DeletePage and SaveGenerated. fn must return immediately. Set it before
// the store is shared between goroutines.
func (s *Store) OnPageChange(fn func()) { s.onPageChange = fn }

func (s *Store) notify() {
	if s.onPageChange != nil {
		s.onPageChange()
	}
}

// TransmitPages returns every page with transmit set, TTI filled, ordered by mpp.
func (s *Store) TransmitPages() ([]Page, error) {
	rows, err := s.db.Query("SELECT mpp, tti FROM pages WHERE transmit = 1 ORDER BY mpp")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var pages []Page
	for rows.Next() {
		var p Page
		if err := rows.Scan(&p.MPP, &p.TTI); err != nil {
			return nil, err
		}
		pages = append(pages, p)
	}
	return pages, rows.Err()
}

// Open opens <dataDir>/ceems.db and applies every pending migration.
func Open(dataDir string) (*Store, error) {
	migrations, err := fs.Sub(embedded, "migrations")
	if err != nil {
		return nil, err
	}
	q := url.Values{"_pragma": {
		"journal_mode(WAL)", "foreign_keys(ON)", "busy_timeout(5000)", "synchronous(NORMAL)",
	}}
	db, err := sql.Open("sqlite", "file:"+filepath.ToSlash(filepath.Join(dataDir, "ceems.db"))+"?"+q.Encode())
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	if err := migrate(db, migrations, dataDir); err != nil {
		db.Close()
		return nil, err
	}
	return &Store{db: db}, nil
}

// Close closes the database.
func (s *Store) Close() error { return s.db.Close() }

var migrationName = regexp.MustCompile(`^(\d{4})_[^/]+\.sql$`)

// migrate applies every migration in fsys newer than the database's
// user_version, each in its own transaction. Names must be NNNN_name.sql,
// numbered from 0001 without gaps.
func migrate(db *sql.DB, fsys fs.FS, dataDir string) error {
	entries, err := fs.Glob(fsys, "*")
	if err != nil {
		return err
	}
	sort.Strings(entries)
	for i, name := range entries {
		m := migrationName.FindStringSubmatch(name)
		if m == nil {
			return fmt.Errorf("migration %s: name must be NNNN_name.sql", name)
		}
		if want := fmt.Sprintf("%04d", i+1); m[1] != want {
			return fmt.Errorf("migration %s: expected number %s (gap or duplicate)", name, want)
		}
	}

	var current int
	if err := db.QueryRow("PRAGMA user_version").Scan(&current); err != nil {
		return err
	}
	if current > len(entries) {
		return fmt.Errorf("database is at version %d but this build only has migrations up to %d", current, len(entries))
	}
	if current == len(entries) {
		return nil
	}

	if current > 0 {
		backup := filepath.Join(dataDir, fmt.Sprintf("ceems-pre-v%d.db", current+1))
		if _, err := db.Exec("VACUUM INTO ?", backup); err != nil {
			return fmt.Errorf("backup before migration %d: %w", current+1, err)
		}
	}
	for i := current; i < len(entries); i++ {
		body, err := fs.ReadFile(fsys, entries[i])
		if err != nil {
			return err
		}
		if err := apply(db, i+1, string(body)); err != nil {
			return fmt.Errorf("migration %s: %w", entries[i], err)
		}
	}
	return nil
}

func apply(db *sql.DB, version int, body string) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(body); err != nil {
		return err
	}
	if _, err := tx.Exec(fmt.Sprintf("PRAGMA user_version = %d", version)); err != nil {
		return err
	}
	return tx.Commit()
}

// Page is one stored page.
type Page struct {
	MPP         int
	TTI         []byte
	Description string
	Subpages    int
	Transmit    bool
	UpdatedAt   string // RFC 3339 UTC
	UpdatedBy   string
	BindingID   *int64 // nil when no binding generates the page
	BindingName string // "" when unbound

	FastextGroupID   *int64 // nil when the page is in no group
	FastextGroupName string
	FastextPaged     bool
}

// ErrNotFound is returned by Page for an unknown mpp.
var ErrNotFound = errors.New("page not found")

// Page returns the stored page mpp.
func (s *Store) Page(mpp int) (*Page, error) {
	p := &Page{MPP: mpp}
	err := s.db.QueryRow(
		`SELECT p.tti, p.description, p.subpages, p.transmit, p.updated_at, p.updated_by, p.binding_id, COALESCE(b.name, ''),
		        p.fastext_group_id, COALESCE(g.name, ''), COALESCE(g.paged, 0)
		 FROM pages p LEFT JOIN bindings b ON b.id = p.binding_id
		 LEFT JOIN fastext_groups g ON g.id = p.fastext_group_id WHERE p.mpp = ?`, mpp,
	).Scan(&p.TTI, &p.Description, &p.Subpages, &p.Transmit, &p.UpdatedAt, &p.UpdatedBy, &p.BindingID, &p.BindingName,
		&p.FastextGroupID, &p.FastextGroupName, &p.FastextPaged)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return p, nil
}

// Pages returns every page ordered by mpp. TTI is left nil.
func (s *Store) Pages() ([]Page, error) {
	rows, err := s.db.Query("SELECT mpp, description, subpages, transmit, updated_at, updated_by FROM pages ORDER BY mpp")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var pages []Page
	for rows.Next() {
		var p Page
		if err := rows.Scan(&p.MPP, &p.Description, &p.Subpages, &p.Transmit, &p.UpdatedAt, &p.UpdatedBy); err != nil {
			return nil, err
		}
		pages = append(pages, p)
	}
	return pages, rows.Err()
}

// ImportFile is one file offered to ImportPages; Path is used only in errors.
type ImportFile struct {
	Path string
	TTI  []byte
}

// ImportPages adds every file not already stored, in one transaction, as updated_by by.
// A file whose bytes equal the stored page is skipped. It refuses, writing nothing:
// a file that fails tti.Parse; two files with the same mpp; a file whose mpp is
// stored with different bytes. Returns the number of pages added.
// shortcut: it does not call notify, because it runs only in the import process; an import
// while serve is running reaches the pages directory at the next page change or restart.
func (s *Store) ImportPages(files []ImportFile, by string) (added int, err error) {
	pages := make([]*tti.Page, len(files))
	for i, f := range files {
		if pages[i], err = tti.Parse(f.TTI); err != nil {
			return 0, fmt.Errorf("%s: %w", f.Path, err)
		}
	}
	seen := map[int]string{}
	for i, f := range files {
		mpp := pages[i].MPP()
		if first, dup := seen[mpp]; dup {
			return 0, fmt.Errorf("%s and %s both claim page %03X", first, f.Path, mpp)
		}
		seen[mpp] = f.Path
	}

	now := time.Now().UTC().Format(time.RFC3339)
	tx, err := s.db.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	for i, f := range files {
		mpp := pages[i].MPP()
		var stored []byte
		err := tx.QueryRow("SELECT tti FROM pages WHERE mpp = ?", mpp).Scan(&stored)
		// The rules run on the file first (no group applies to a page not yet stored), so a
		// second import of the same files finds them equal to what the first stored.
		if ferr := fixLinks(tx, pages[i]); ferr != nil {
			return 0, ferr
		}
		switch {
		case errors.Is(err, sql.ErrNoRows):
			if err := insertPage(tx, pages[i], now, by); err != nil {
				return 0, fmt.Errorf("%s: %w", f.Path, err)
			}
			added++
		case err != nil:
			return 0, err
		case !bytes.Equal(stored, pages[i].Bytes()):
			return 0, fmt.Errorf("%s: page %03X is already stored with different content", f.Path, mpp)
		}
	}
	return added, tx.Commit()
}

func insertPage(tx *sql.Tx, p *tti.Page, now, by string) error {
	_, err := tx.Exec(
		`INSERT INTO pages (mpp, tti, description, subpages, transmit, binding_id, updated_at, updated_by)
		 VALUES (?, ?, ?, ?, ?, NULL, ?, ?)`,
		p.MPP(), p.Bytes(), p.Description(), p.Subpages(), p.Transmit(), now, by)
	return err
}

var (
	ErrExists    = errors.New("page already exists")
	ErrGenerated = errors.New("page is generated by a binding") // binding_id IS NOT NULL
	ErrStale     = errors.New("page changed since it was read") // ifMatch given and differs
)

func etagOf(b []byte) string {
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

// ETag is the SHA-256 of the stored bytes as 64 lowercase hex characters.
func (p *Page) ETag() string { return etagOf(p.TTI) }

// lockedRow reads the bytes and binding of page mpp inside tx and applies the checks
// shared by SavePage and DeletePage: ErrNotFound, ErrGenerated, ErrStale. The store
// has one connection, so the transaction is the lock.
func lockedRow(tx *sql.Tx, mpp int, ifMatch string) (stored []byte, err error) {
	var binding sql.NullInt64
	err = tx.QueryRow("SELECT tti, binding_id FROM pages WHERE mpp = ?", mpp).Scan(&stored, &binding)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if binding.Valid {
		return nil, ErrGenerated
	}
	if ifMatch != "" && ifMatch != etagOf(stored) {
		return nil, ErrStale
	}
	return stored, nil
}

// SavePage replaces page mpp with p in one transaction: ErrNotFound when absent;
// ErrGenerated when binding_id is set; ErrStale when ifMatch != "" and differs from the
// stored ETag; an error when p.MPP() != mpp. When p.Bytes() equals the stored bytes
// nothing is written and changed is false. Otherwise it archives the current row (keeping
// the newest 20 per page), rewrites every derived column and writes one audit row.
// The caller fills a.Actor, a.Action and a.Remote.
func (s *Store) SavePage(mpp int, p *tti.Page, ifMatch string, a Audit) (changed bool, err error) {
	if p.MPP() != mpp {
		return false, fmt.Errorf("page is %03X but the target is %03X", p.MPP(), mpp)
	}
	tx, err := s.db.Begin()
	if err != nil {
		return false, err
	}
	defer tx.Rollback()
	stored, err := lockedRow(tx, mpp, ifMatch)
	if err != nil {
		return false, err
	}
	if changed, err := saveLocked(tx, stored, p, utcNow(), a); err != nil || !changed {
		return false, err
	}
	if err := tx.Commit(); err != nil {
		return false, err
	}
	s.notify()
	return true, nil
}

// saveLocked is the body SavePage and SaveGenerated share, run inside tx once the caller
// has read stored and checked it may write page p.MPP(): identical bytes write nothing;
// otherwise it archives, rewrites every derived column and writes one audit row.
// The caller commits when changed is true.
func saveLocked(tx *sql.Tx, stored []byte, p *tti.Page, now string, a Audit) (changed bool, err error) {
	if err := fixLinks(tx, p); err != nil {
		return false, err
	}
	if bytes.Equal(stored, p.Bytes()) {
		return false, nil
	}
	if err := archive(tx, p.MPP()); err != nil {
		return false, err
	}
	if err := updatePage(tx, p, now, a.Actor); err != nil {
		return false, err
	}
	a.Target, a.Detail = fmt.Sprintf("page:%03X", p.MPP()), "sha256:"+etagOf(p.Bytes())
	if err := insertAudit(tx, now, a); err != nil {
		return false, err
	}
	return true, nil
}

// CreatePage inserts p and one audit row; ErrExists when the mpp is stored.
func (s *Store) CreatePage(p *tti.Page, a Audit) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var n int
	if err := tx.QueryRow("SELECT COUNT(*) FROM pages WHERE mpp = ?", p.MPP()).Scan(&n); err != nil {
		return err
	}
	if n > 0 {
		return ErrExists
	}
	now := utcNow()
	gid, err := insertNewPage(tx, p, now, a.Actor)
	if err != nil {
		return err
	}
	create := a
	create.Target, create.Detail = fmt.Sprintf("page:%03X", p.MPP()), "sha256:"+etagOf(p.Bytes())
	if err := insertAudit(tx, now, create); err != nil {
		return err
	}
	if gid != 0 {
		if _, err := sweepGroup(tx, gid, now, a); err != nil {
			return err
		}
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	s.notify()
	return nil
}

// DeletePage archives then deletes; ErrNotFound, ErrGenerated, ErrStale as SavePage.
func (s *Store) DeletePage(mpp int, ifMatch string, a Audit) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	stored, err := lockedRow(tx, mpp, ifMatch)
	if err != nil {
		return err
	}
	var gid sql.NullInt64
	if err := tx.QueryRow("SELECT fastext_group_id FROM pages WHERE mpp = ?", mpp).Scan(&gid); err != nil {
		return err
	}
	if err := archive(tx, mpp); err != nil {
		return err
	}
	if _, err := tx.Exec("DELETE FROM pages WHERE mpp = ?", mpp); err != nil {
		return err
	}
	now := utcNow()
	del := a
	del.Target, del.Detail = fmt.Sprintf("page:%03X", mpp), "sha256:"+etagOf(stored)
	if err := insertAudit(tx, now, del); err != nil {
		return err
	}
	if gid.Valid {
		if _, err := sweepGroup(tx, gid.Int64, now, a); err != nil {
			return err
		}
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	s.notify()
	return nil
}

// archive copies the current row of mpp to page_versions and prunes to the newest 20.
func archive(tx *sql.Tx, mpp int) error {
	if _, err := tx.Exec(`INSERT INTO page_versions (mpp, tti, updated_at, updated_by)
		SELECT mpp, tti, updated_at, updated_by FROM pages WHERE mpp = ?`, mpp); err != nil {
		return err
	}
	_, err := tx.Exec(`DELETE FROM page_versions WHERE mpp = ? AND id NOT IN
		(SELECT id FROM page_versions WHERE mpp = ? ORDER BY id DESC LIMIT 20)`, mpp, mpp)
	return err
}

// updatePage is the UPDATE twin of insertPage.
func updatePage(tx *sql.Tx, p *tti.Page, now, by string) error {
	_, err := tx.Exec(
		`UPDATE pages SET tti = ?, description = ?, subpages = ?, transmit = ?, updated_at = ?, updated_by = ? WHERE mpp = ?`,
		p.Bytes(), p.Description(), p.Subpages(), p.Transmit(), now, by, p.MPP())
	return err
}
