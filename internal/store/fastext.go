package store

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"unicode/utf8"

	"ceems/internal/tti"
)

var (
	ErrInGroup = errors.New("page is already in another fastext group")
	ErrInvalid = errors.New("invalid fastext group")
)

// FastextMember is one page of a group.
type FastextMember struct {
	MPP         int
	Description string
}

// FastextGroup is a named set of the four coloured links (red, green, yellow, cyan).
// A paged group also names the slots (0-3) that point at the previous and next member;
// a static group has PrevSlot and NextSlot nil.
type FastextGroup struct {
	ID                 int64
	Name               string
	Paged              bool
	Links              [4]string
	Texts              [4]string // decoded cells of each link's row 24 text, at most 9
	PrevSlot, NextSlot *int
	Default            bool
	Members            []FastextMember // in order; filled by FastextGroups and FastextGroup
}

var linkRe = regexp.MustCompile(`^(?:[1-8][0-9a-f]{2}|8ff)$`)

func (g *FastextGroup) validate() error {
	if n := len(g.Name); n < 1 || n > 40 {
		return fmt.Errorf("%w: name must be 1 to 40 characters", ErrInvalid)
	}
	for i, l := range g.Links {
		if !linkRe.MatchString(strings.ToLower(l)) {
			return fmt.Errorf("%w: link %d must be three hex digits with magazine 1 to 8, or 8ff", ErrInvalid, i+1)
		}
	}
	for i, t := range g.Texts {
		bad := utf8.RuneCountInString(t) > 9
		for _, r := range t { // invalid UTF-8 ranges as U+FFFD
			bad = bad || r > 0x7F
		}
		if bad {
			return fmt.Errorf("%w: text %d must be at most 9 cells, each from 0x00 to 0x7F", ErrInvalid, i+1)
		}
	}
	switch {
	case !g.Paged && (g.PrevSlot != nil || g.NextSlot != nil):
		return fmt.Errorf("%w: a static group has no previous or next slot", ErrInvalid)
	case g.Paged && (g.PrevSlot == nil || g.NextSlot == nil || *g.PrevSlot < 0 || *g.PrevSlot > 3 ||
		*g.NextSlot < 0 || *g.NextSlot > 3 || *g.PrevSlot == *g.NextSlot):
		return fmt.Errorf("%w: a paged group needs two different slots from 0 to 3", ErrInvalid)
	}
	return nil
}

func (g *FastextGroup) linksText() string {
	return strings.ToLower(strings.Join(g.Links[:], ","))
}

func (g *FastextGroup) textsJSON() string {
	b, _ := json.Marshal(g.Texts[:]) // a string slice cannot fail to marshal
	return string(b)
}

func fastextAudit(tx *sql.Tx, a Audit, action string, id int64, detail string) error {
	a.Action, a.Target, a.Detail = action, fmt.Sprintf("fastext:%d", id), detail
	return insertAudit(tx, utcNow(), a)
}

// FastextGroups returns every group by id, each with its members in order.
func (s *Store) FastextGroups() ([]FastextGroup, error) {
	rows, err := s.db.Query("SELECT id, name, paged, links, prev_slot, next_slot, is_default, texts FROM fastext_groups ORDER BY id")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var gs []FastextGroup
	for rows.Next() {
		g, err := scanGroup(rows)
		if err != nil {
			return nil, err
		}
		gs = append(gs, *g)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	mrows, err := s.db.Query(`SELECT fastext_group_id, mpp, description FROM pages
		WHERE fastext_group_id IS NOT NULL ORDER BY fastext_order, mpp`)
	if err != nil {
		return nil, err
	}
	defer mrows.Close()
	for mrows.Next() {
		var gid int64
		var m FastextMember
		if err := mrows.Scan(&gid, &m.MPP, &m.Description); err != nil {
			return nil, err
		}
		for i := range gs {
			if gs[i].ID == gid {
				gs[i].Members = append(gs[i].Members, m)
			}
		}
	}
	return gs, mrows.Err()
}

// FastextGroup returns group id with its members; ErrNotFound when absent.
func (s *Store) FastextGroup(id int64) (*FastextGroup, error) {
	gs, err := s.FastextGroups()
	if err != nil {
		return nil, err
	}
	for i := range gs {
		if gs[i].ID == id {
			return &gs[i], nil
		}
	}
	return nil, ErrNotFound
}

func scanGroup(sc interface{ Scan(...any) error }) (*FastextGroup, error) {
	g := &FastextGroup{}
	var links, texts string
	var prev, next sql.NullInt64
	if err := sc.Scan(&g.ID, &g.Name, &g.Paged, &links, &prev, &next, &g.Default, &texts); err != nil {
		return nil, err
	}
	if err := json.Unmarshal([]byte(texts), &g.Texts); err != nil {
		return nil, fmt.Errorf("group %d texts: %w", g.ID, err)
	}
	copy(g.Links[:], strings.Split(links, ","))
	if prev.Valid {
		v := int(prev.Int64)
		g.PrevSlot = &v
	}
	if next.Valid {
		v := int(next.Int64)
		g.NextSlot = &v
	}
	return g, nil
}

// CreateFastextGroup validates and inserts g (Members ignored); ErrInvalid, ErrNameTaken.
// A default group clears the previous default.
func (s *Store) CreateFastextGroup(g *FastextGroup, a Audit) (int64, error) {
	if err := g.validate(); err != nil {
		return 0, err
	}
	tx, err := s.db.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	if g.Default {
		if _, err := tx.Exec("UPDATE fastext_groups SET is_default = 0 WHERE is_default = 1"); err != nil {
			return 0, err
		}
	}
	res, err := tx.Exec("INSERT INTO fastext_groups (name, paged, links, texts, prev_slot, next_slot, is_default) VALUES (?, ?, ?, ?, ?, ?, ?)",
		g.Name, g.Paged, g.linksText(), g.textsJSON(), g.PrevSlot, g.NextSlot, g.Default)
	if err != nil {
		return 0, nameErr(err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, err
	}
	if err := fastextAudit(tx, a, "fastext.create", id, g.Name); err != nil {
		return 0, err
	}
	return id, tx.Commit()
}

// UpdateFastextGroup replaces group g.ID's fields and rewrites its members;
// ErrNotFound, ErrInvalid, ErrNameTaken.
func (s *Store) UpdateFastextGroup(g *FastextGroup, a Audit) error {
	if err := g.validate(); err != nil {
		return err
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if g.Default {
		if _, err := tx.Exec("UPDATE fastext_groups SET is_default = 0 WHERE is_default = 1 AND id <> ?", g.ID); err != nil {
			return err
		}
	}
	res, err := tx.Exec("UPDATE fastext_groups SET name = ?, paged = ?, links = ?, texts = ?, prev_slot = ?, next_slot = ?, is_default = ? WHERE id = ?",
		g.Name, g.Paged, g.linksText(), g.textsJSON(), g.PrevSlot, g.NextSlot, g.Default, g.ID)
	if err != nil {
		return nameErr(err)
	}
	if n, err := res.RowsAffected(); err != nil {
		return err
	} else if n == 0 {
		return ErrNotFound
	}
	if err := fastextAudit(tx, a, "fastext.update", g.ID, g.Name); err != nil {
		return err
	}
	return s.commitSweep(tx, g.ID, a, nil)
}

// DeleteFastextGroup deletes group id. Its members keep their last links, which are
// manual again; ErrNotFound.
func (s *Store) DeleteFastextGroup(id int64, a Audit) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	members, err := pagesWhere(tx, "SELECT mpp FROM pages WHERE fastext_group_id = ? ORDER BY mpp", id)
	if err != nil {
		return err
	}
	if _, err := tx.Exec("UPDATE pages SET fastext_order = NULL WHERE fastext_group_id = ?", id); err != nil {
		return err
	}
	var name string
	err = tx.QueryRow("DELETE FROM fastext_groups WHERE id = ? RETURNING name", id).Scan(&name)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	if err := fastextAudit(tx, a, "fastext.delete", id, name); err != nil {
		return err
	}
	return s.commitSweep(tx, id, a, members)
}

// SetFastextGroupPages makes mpps, in order, the whole membership of group id.
// Members not listed are detached and keep their last links. ErrNotFound for an
// unknown group or page, ErrInGroup for a page in another group, ErrInvalid for a
// repeated page.
func (s *Store) SetFastextGroupPages(id int64, mpps []int, a Audit) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var n int
	if err := tx.QueryRow("SELECT count(*) FROM fastext_groups WHERE id = ?", id).Scan(&n); err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	seen := map[int]bool{}
	for _, mpp := range mpps {
		if seen[mpp] {
			return fmt.Errorf("%w: page %03X is listed twice", ErrInvalid, mpp)
		}
		seen[mpp] = true
		var gid sql.NullInt64
		err := tx.QueryRow("SELECT fastext_group_id FROM pages WHERE mpp = ?", mpp).Scan(&gid)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		if gid.Valid && gid.Int64 != id {
			return ErrInGroup
		}
	}
	current, err := pagesWhere(tx, "SELECT mpp FROM pages WHERE fastext_group_id = ? ORDER BY mpp", id)
	if err != nil {
		return err
	}
	var detached []int
	for _, m := range current {
		if !seen[m] {
			detached = append(detached, m)
		}
	}
	if _, err := tx.Exec("UPDATE pages SET fastext_group_id = NULL, fastext_order = NULL WHERE fastext_group_id = ?", id); err != nil {
		return err
	}
	for i, mpp := range mpps {
		if _, err := tx.Exec("UPDATE pages SET fastext_group_id = ?, fastext_order = ? WHERE mpp = ?", id, i+1, mpp); err != nil {
			return err
		}
	}
	if err := fastextAudit(tx, a, "fastext.pages", id, fmt.Sprintf("%d pages", len(mpps))); err != nil {
		return err
	}
	return s.commitSweep(tx, id, a, detached)
}

// commitSweep empties row 24 of the pages that left group id, rewrites its members,
// commits, and notifies when any page changed.
func (s *Store) commitSweep(tx *sql.Tx, id int64, a Audit, leaving []int) error {
	now := utcNow()
	gone, err := rewritePages(tx, leaving, now, a, true)
	if err != nil {
		return err
	}
	changed, err := sweepGroup(tx, id, now, a)
	if err != nil {
		return err
	}
	changed = changed || gone
	if err := tx.Commit(); err != nil {
		return err
	}
	if changed {
		s.notify()
	}
	return nil
}

// pagesWhere returns the mpps selected by the query, in query order.
func pagesWhere(tx *sql.Tx, query string, args ...any) ([]int, error) {
	rows, err := tx.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var mpps []int
	for rows.Next() {
		var m int
		if err := rows.Scan(&m); err != nil {
			return nil, err
		}
		mpps = append(mpps, m)
	}
	return mpps, rows.Err()
}

// sweepGroup rewrites every member of group id.
func sweepGroup(tx *sql.Tx, id int64, now string, a Audit) (bool, error) {
	mpps, err := pagesWhere(tx, "SELECT mpp FROM pages WHERE fastext_group_id = ? ORDER BY mpp", id)
	if err != nil {
		return false, err
	}
	return rewritePages(tx, mpps, now, a, false)
}

// rewritePages runs each listed page, bound ones too, through saveLocked, which applies
// fixLinks: identical bytes write nothing; otherwise one version row and one page.fastext
// audit row. When leaving, the pages have just left their group and row 24 is emptied on
// every subpage first. The caller commits and notifies when changed.
func rewritePages(tx *sql.Tx, mpps []int, now string, a Audit, leaving bool) (changed bool, err error) {
	a.Action = "page.fastext"
	for _, mpp := range mpps {
		var stored []byte
		if err := tx.QueryRow("SELECT tti FROM pages WHERE mpp = ?", mpp).Scan(&stored); err != nil {
			return false, err
		}
		p, err := tti.Parse(stored)
		if err != nil {
			return false, fmt.Errorf("page %03X: %w", mpp, err)
		}
		if leaving {
			for sub := 0; sub < p.Subpages(); sub++ {
				if err := p.SetRow(sub, 24, ""); err != nil {
					return false, err
				}
			}
		}
		c, err := saveLocked(tx, stored, p, now, a)
		if err != nil {
			return false, err
		}
		changed = changed || c
	}
	return changed, nil
}

// joinDefaultGroup puts the stored page mpp last in the default group, if one exists, and
// returns its id (0 when there is none).
func joinDefaultGroup(tx *sql.Tx, mpp int) (int64, error) {
	var gid int64
	err := tx.QueryRow("SELECT id FROM fastext_groups WHERE is_default = 1").Scan(&gid)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	_, err = tx.Exec(`UPDATE pages SET fastext_group_id = ?,
		fastext_order = (SELECT COALESCE(MAX(fastext_order), 0) + 1 FROM pages WHERE fastext_group_id = ?) WHERE mpp = ?`, gid, gid, mpp)
	return gid, err
}

// insertNewPage inserts a page made in the editor or by a binding: it joins the default
// group and gets the rules applied. It returns the group id (0 for none); the caller
// sweeps the group's neighbours.
func insertNewPage(tx *sql.Tx, p *tti.Page, now, by string) (int64, error) {
	if err := insertPage(tx, p, now, by); err != nil {
		return 0, err
	}
	gid, err := joinDefaultGroup(tx, p.MPP())
	if err != nil {
		return 0, err
	}
	before := string(p.Bytes())
	if err := fixLinks(tx, p); err != nil {
		return 0, err
	}
	if string(p.Bytes()) != before {
		err = updatePage(tx, p, now, by)
	}
	return gid, err
}

const absentLink = "8ff"

// fixLinks rewrites the FL slots a rule owns on every subpage of p (D-065): slots 1-4
// from the page's group, slot 6 from the index setting. Slot 5 is never touched. A page
// no rule owns a slot of is left byte for byte alone.
func fixLinks(tx *sql.Tx, p *tti.Page) error {
	var own [6]string // "" = no rule owns the slot
	set, err := readSettings(tx.Query)
	if err != nil {
		return err
	}
	if idx := strings.ToLower(set.IndexPage); set.IndexPageEnabled && linkRe.MatchString(idx) {
		own[5] = idx
	}

	var gid int64
	var paged bool
	var links, texts string
	var prev, next sql.NullInt64
	err = tx.QueryRow(`SELECT g.id, g.paged, g.links, g.texts, g.prev_slot, g.next_slot
		FROM pages p JOIN fastext_groups g ON g.id = p.fastext_group_id WHERE p.mpp = ?`, p.MPP(),
	).Scan(&gid, &paged, &links, &texts, &prev, &next)
	var row24 string // row 24 of a grouped page; "" for an ungrouped one
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	if err == nil {
		copy(own[:4], strings.Split(links, ","))
		var t [4]string
		if err := json.Unmarshal([]byte(texts), &t); err != nil {
			return fmt.Errorf("group %d texts: %w", gid, err)
		}
		if paged {
			mpps, err := pagesWhere(tx, "SELECT mpp FROM pages WHERE fastext_group_id = ? ORDER BY fastext_order, mpp", gid)
			if err != nil {
				return err
			}
			t[prev.Int64], t[next.Int64] = " [ Prev  ", "  Next ] "
			own[prev.Int64], own[next.Int64] = absentLink, absentLink
			for i, m := range mpps {
				if m != p.MPP() {
					continue
				}
				if i > 0 {
					own[prev.Int64] = fmt.Sprintf("%03x", mpps[i-1])
				}
				if i < len(mpps)-1 {
					own[next.Int64] = fmt.Sprintf("%03x", mpps[i+1])
				}
			}
			for _, e := range []int64{prev.Int64, next.Int64} {
				if own[e] == absentLink {
					t[e] = "" // the end of the group shows nothing
				}
			}
		}
		for i, code := range []string{"\x01", "\x02", "\x03", "\x06"} {
			row24 += code + fmt.Sprintf("%-9s", t[i])
		}
	}
	if own == [6]string{} {
		return nil
	}

	for sub := 0; sub < p.Subpages(); sub++ {
		body, _ := p.Field(sub, "FL")
		cur, ok := tti.ParseFL(body)
		if !ok {
			cur = [6]string{absentLink, absentLink, absentLink, absentLink, absentLink, absentLink}
		}
		differs := !ok
		for i, l := range own {
			if l != "" && l != cur[i] {
				cur[i], differs = l, true
			}
		}
		if differs {
			p.SetField(sub, "FL", strings.Join(cur[:], ","))
		}
		if row24 != "" {
			if err := p.SetRow(sub, 24, row24); err != nil {
				return err
			}
		}
	}
	return nil
}
