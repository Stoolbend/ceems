package store

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"ceems/internal/tti"
)

// Template is one stored template page; RowRules is its JSON object "row" -> slot name.
type Template struct {
	ID       int64
	Name     string
	TTI      []byte
	RowRules string
}

// Binding renders a template into page MPP; Source and Static are JSON.
type Binding struct {
	ID         int64
	Name       string
	TemplateID int64
	MPP        int
	Source     string
	Static     string
}

var (
	ErrInUse     = errors.New("template is used by a binding")
	ErrBound     = errors.New("page is already bound") // CreateBinding on a page with binding_id set
	ErrNotBound  = errors.New("page is not bound to this binding")
	ErrNameTaken = errors.New("name is already used") // UNIQUE name of a template or binding
)

// nameErr maps a UNIQUE violation on a name column to ErrNameTaken.
func nameErr(err error) error {
	if err != nil && strings.Contains(err.Error(), "UNIQUE constraint failed") && strings.Contains(err.Error(), ".name") {
		return ErrNameTaken
	}
	return err
}

// Templates returns every template ordered by id.
func (s *Store) Templates() ([]Template, error) {
	rows, err := s.db.Query("SELECT id, name, tti, row_rules FROM templates ORDER BY id")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ts []Template
	for rows.Next() {
		var t Template
		if err := rows.Scan(&t.ID, &t.Name, &t.TTI, &t.RowRules); err != nil {
			return nil, err
		}
		ts = append(ts, t)
	}
	return ts, rows.Err()
}

// Template returns template id; ErrNotFound when absent.
func (s *Store) Template(id int64) (*Template, error) {
	t := &Template{ID: id}
	err := s.db.QueryRow("SELECT name, tti, row_rules FROM templates WHERE id = ?", id).Scan(&t.Name, &t.TTI, &t.RowRules)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return t, nil
}

// CreateTemplate inserts t and its audit row; ErrNameTaken on a duplicate name.
func (s *Store) CreateTemplate(t *Template, a Audit) (int64, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	res, err := tx.Exec("INSERT INTO templates (name, tti, row_rules) VALUES (?, ?, ?)", t.Name, t.TTI, t.RowRules)
	if err != nil {
		return 0, nameErr(err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, err
	}
	if err := auditNamed(tx, a, "template.create", "template", id, t.Name); err != nil {
		return 0, err
	}
	return id, tx.Commit()
}

// UpdateTemplate replaces template t.ID; ErrNotFound, ErrNameTaken.
func (s *Store) UpdateTemplate(t *Template, a Audit) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	res, err := tx.Exec("UPDATE templates SET name = ?, tti = ?, row_rules = ? WHERE id = ?", t.Name, t.TTI, t.RowRules, t.ID)
	if err != nil {
		return nameErr(err)
	}
	if n, err := res.RowsAffected(); err != nil {
		return err
	} else if n == 0 {
		return ErrNotFound
	}
	if err := auditNamed(tx, a, "template.update", "template", t.ID, t.Name); err != nil {
		return err
	}
	return tx.Commit()
}

// DeleteTemplate deletes template id; ErrNotFound, or ErrInUse (with the binding count)
// when a binding references it, which the foreign key enforces.
func (s *Store) DeleteTemplate(id int64, a Audit) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var name string
	err = tx.QueryRow("DELETE FROM templates WHERE id = ? RETURNING name", id).Scan(&name)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil && strings.Contains(err.Error(), "FOREIGN KEY constraint failed") {
		var n int
		if err := tx.QueryRow("SELECT count(*) FROM bindings WHERE template_id = ?", id).Scan(&n); err != nil {
			return err
		}
		return fmt.Errorf("%w (%d bindings)", ErrInUse, n)
	}
	if err != nil {
		return err
	}
	if err := auditNamed(tx, a, "template.delete", "template", id, name); err != nil {
		return err
	}
	return tx.Commit()
}

// auditNamed writes the audit row of a template or binding change: target kind:<id>,
// detail the name only, never source or static.
func auditNamed(tx *sql.Tx, a Audit, action, kind string, id int64, name string) error {
	a.Action, a.Target, a.Detail = action, fmt.Sprintf("%s:%d", kind, id), name
	return insertAudit(tx, utcNow(), a)
}

const bindingColumns = "id, name, template_id, mpp, source, static"

func scanBinding(sc interface{ Scan(...any) error }) (*Binding, error) {
	b := &Binding{}
	err := sc.Scan(&b.ID, &b.Name, &b.TemplateID, &b.MPP, &b.Source, &b.Static)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return b, nil
}

// Bindings returns every binding ordered by id.
func (s *Store) Bindings() ([]Binding, error) {
	rows, err := s.db.Query("SELECT " + bindingColumns + " FROM bindings ORDER BY id")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var bs []Binding
	for rows.Next() {
		b, err := scanBinding(rows)
		if err != nil {
			return nil, err
		}
		bs = append(bs, *b)
	}
	return bs, rows.Err()
}

// Binding returns binding id; ErrNotFound when absent.
func (s *Store) Binding(id int64) (*Binding, error) {
	return scanBinding(s.db.QueryRow("SELECT "+bindingColumns+" FROM bindings WHERE id = ?", id))
}

// CreateBinding inserts the row and, in the same transaction, attaches the page: if
// pages[mpp] exists and binding_id IS NULL it sets binding_id; if it is bound, ErrBound;
// if absent, it inserts tti.NewPage(mpp) with binding_id set. Audit target
// binding:<id>, detail = name.
func (s *Store) CreateBinding(b *Binding, a Audit) (int64, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	var bound sql.NullInt64
	err = tx.QueryRow("SELECT binding_id FROM pages WHERE mpp = ?", b.MPP).Scan(&bound)
	exists := err == nil
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return 0, err
	}
	if bound.Valid {
		return 0, ErrBound
	}
	res, err := tx.Exec("INSERT INTO bindings (name, template_id, mpp, source, static) VALUES (?, ?, ?, ?, ?)",
		b.Name, b.TemplateID, b.MPP, b.Source, b.Static)
	if err != nil {
		return 0, nameErr(err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, err
	}
	var gid int64
	if !exists {
		if gid, err = insertNewPage(tx, tti.NewPage(b.MPP), utcNow(), a.Actor); err != nil {
			return 0, err
		}
	}
	if _, err := tx.Exec("UPDATE pages SET binding_id = ? WHERE mpp = ?", id, b.MPP); err != nil {
		return 0, err
	}
	if err := auditNamed(tx, a, "binding.create", "binding", id, b.Name); err != nil {
		return 0, err
	}
	swept := false
	if gid != 0 {
		if swept, err = sweepGroup(tx, gid, utcNow(), a); err != nil {
			return 0, err
		}
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	if swept {
		s.notify()
	}
	return id, nil
}

// UpdateBinding changes name, template_id, source, static; a different mpp moves the
// bound page with it (ErrExists when the new number is taken).
func (s *Store) UpdateBinding(b *Binding, a Audit) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var mpp int
	err = tx.QueryRow("SELECT mpp FROM bindings WHERE id = ?", b.ID).Scan(&mpp)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	moved := b.MPP != mpp
	if moved {
		if err := movePage(tx, b.ID, mpp, b.MPP, a); err != nil {
			return err
		}
	}
	if _, err := tx.Exec("UPDATE bindings SET name = ?, template_id = ?, mpp = ?, source = ?, static = ? WHERE id = ?",
		b.Name, b.TemplateID, b.MPP, b.Source, b.Static, b.ID); err != nil {
		return nameErr(err)
	}
	if err := auditNamed(tx, a, "binding.update", "binding", b.ID, b.Name); err != nil {
		return err
	}
	if moved {
		a.Action, a.Target, a.Detail = "binding.move", fmt.Sprintf("binding:%d", b.ID), fmt.Sprintf("%03X->%03X", mpp, b.MPP)
		if err := insertAudit(tx, utcNow(), a); err != nil {
			return err
		}
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	if moved {
		s.notify()
	}
	return nil
}

// movePage renumbers the page generated by binding id from old to new: its row, its
// PN lines and its version rows. ErrExists when new holds any page.
func movePage(tx *sql.Tx, id int64, old, new int, a Audit) error {
	var n int
	if err := tx.QueryRow("SELECT count(*) FROM pages WHERE mpp = ?", new).Scan(&n); err != nil {
		return err
	}
	if n > 0 {
		return ErrExists
	}
	var raw []byte
	var bound sql.NullInt64
	err := tx.QueryRow("SELECT tti, binding_id FROM pages WHERE mpp = ?", old).Scan(&raw, &bound)
	if errors.Is(err, sql.ErrNoRows) || err == nil && (!bound.Valid || bound.Int64 != id) {
		return ErrNotBound
	}
	if err != nil {
		return err
	}
	p, err := tti.Parse(raw)
	if err != nil {
		return err
	}
	p.SetMPP(new)
	if _, err := tx.Exec("UPDATE pages SET mpp = ?, tti = ?, updated_at = ?, updated_by = ? WHERE mpp = ?",
		new, p.Bytes(), utcNow(), a.Actor, old); err != nil {
		return err
	}
	if _, err := tx.Exec("UPDATE page_versions SET mpp = ? WHERE mpp = ?", new, old); err != nil {
		return err
	}
	// The neighbours in a paged group hold the old number.
	var gid sql.NullInt64
	if err := tx.QueryRow("SELECT fastext_group_id FROM pages WHERE mpp = ?", new).Scan(&gid); err != nil || !gid.Valid {
		return err
	}
	_, err = sweepGroup(tx, gid.Int64, utcNow(), a)
	return err
}

// DeleteBinding sets pages.binding_id = NULL where it equals id, then deletes the row.
// The page keeps its last content and becomes editable.
func (s *Store) DeleteBinding(id int64, a Audit) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec("UPDATE pages SET binding_id = NULL WHERE binding_id = ?", id); err != nil {
		return err
	}
	var name string
	err = tx.QueryRow("DELETE FROM bindings WHERE id = ? RETURNING name", id).Scan(&name)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	if err := auditNamed(tx, a, "binding.delete", "binding", id, name); err != nil {
		return err
	}
	return tx.Commit()
}

// SaveGenerated is the only writer of a bound page. It reads mpp from bindings[id]
// (ErrNotFound), requires p.MPP() to equal it (error naming both), requires
// pages[mpp].binding_id == id (ErrNotBound), then runs the same body as SavePage
// (identical bytes: no statement; else archive, updatePage, one audit row). It sets
// a.Actor = "job:binding:<id>" and a.Action = "page.generate" itself, ignoring the
// caller's values. Returns changed.
func (s *Store) SaveGenerated(bindingID int64, p *tti.Page, a Audit) (changed bool, err error) {
	tx, err := s.db.Begin()
	if err != nil {
		return false, err
	}
	defer tx.Rollback()
	b, err := scanBinding(tx.QueryRow("SELECT "+bindingColumns+" FROM bindings WHERE id = ?", bindingID))
	if err != nil {
		return false, err
	}
	if p.MPP() != b.MPP {
		return false, fmt.Errorf("page is %03X but binding %d targets %03X", p.MPP(), bindingID, b.MPP)
	}
	var stored []byte
	var bound sql.NullInt64
	err = tx.QueryRow("SELECT tti, binding_id FROM pages WHERE mpp = ?", b.MPP).Scan(&stored, &bound)
	if errors.Is(err, sql.ErrNoRows) {
		return false, ErrNotBound
	}
	if err != nil {
		return false, err
	}
	if !bound.Valid || bound.Int64 != bindingID {
		return false, ErrNotBound
	}
	a.Actor, a.Action = fmt.Sprintf("job:binding:%d", bindingID), "page.generate"
	if changed, err := saveLocked(tx, stored, p, utcNow(), a); err != nil || !changed {
		return false, err
	}
	if err := tx.Commit(); err != nil {
		return false, err
	}
	s.notify()
	return true, nil
}
