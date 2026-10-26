// Package tmpl parses template pages with slot tokens and renders them into
// ordinary pages (ADR 0002 slot syntax, ADR 0005 rendering rules).
package tmpl

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"ceems/internal/tti"
)

// Slot is one token found on rows 1-24 of a subpage.
type Slot struct {
	Name                 string
	Sub, Row, Col, Width int
	Align                byte // 0 left, '^' centre, '>' right
}

// Template is a parsed template page: its bytes, its slots and its row rules.
type Template struct {
	src      []byte
	slots    []Slot
	rowRules map[int]string
}

// slotRE matches a slot token; the match length is the slot's width (at least 3: "{a}").
var slotRE = regexp.MustCompile(`\{([\^>]?)([a-z][a-z0-9_-]*)(\.*)\}`)

const spaces = "                                        " // 40

// Parse validates the TTI with tti.Parse, scans the decoded cells of OL rows 1-24 of
// every subpage for slot tokens, and checks rowRules keys are "1".."24" and values are
// slot names.
func Parse(ttiBytes []byte, rowRules map[string]string) (*Template, error) {
	p, err := tti.Parse(ttiBytes)
	if err != nil {
		return nil, err
	}
	t := &Template{src: append([]byte(nil), ttiBytes...), rowRules: map[int]string{}}
	names := map[string]bool{}
	for sub := 0; sub < p.Subpages(); sub++ {
		for row := 1; row <= 24; row++ {
			cells, ok := p.Row(sub, row)
			if !ok {
				continue
			}
			for _, m := range slotRE.FindAllStringSubmatchIndex(cells, -1) {
				s := Slot{Name: cells[m[4]:m[5]], Sub: sub, Row: row, Col: m[0], Width: m[1] - m[0]}
				if m[3] > m[2] {
					s.Align = cells[m[2]]
				}
				t.slots = append(t.slots, s)
				names[s.Name] = true
			}
		}
	}
	for k, v := range rowRules {
		row, err := strconv.Atoi(k)
		if err != nil || row < 1 || row > 24 || strconv.Itoa(row) != k {
			return nil, fmt.Errorf("row rule %q: row must be 1-24", k)
		}
		if !names[v] {
			return nil, fmt.Errorf("row rule %s: %q is not a slot of this template", k, v)
		}
		t.rowRules[row] = v
	}
	return t, nil
}

// Slots returns the slots in subpage, row, column order.
func (t *Template) Slots() []Slot { return append([]Slot(nil), t.slots...) }

// Clean makes an untrusted value safe: drops cells below 0x20 and 0x7F, maps U+00A3 to
// 0x23, any other non-ASCII rune (including invalid UTF-8) to '?'.
func Clean(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case r < 0x20 || r == 0x7F:
		case r == '£':
			b.WriteByte(0x23)
		case r > 0x7F:
			b.WriteByte('?')
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

// fit clamps v to w cells, then pads it with spaces by alignment; centring puts the
// odd space on the right.
func fit(v string, w int, align byte) string {
	if len(v) > w {
		v = v[:w]
	}
	pad := w - len(v)
	switch align {
	case '>':
		return spaces[:pad] + v
	case '^':
		return spaces[:pad/2] + v + spaces[:pad-pad/2]
	default:
		return v + spaces[:pad]
	}
}

// Render returns a new page for mpp. static may hold control codes (error if any rune
// > 127); data is passed through Clean. A key in both maps is an error; a slot with no
// key in either is an error; extra keys are ignored.
func (t *Template) Render(mpp int, static, data map[string]string) (*tti.Page, error) {
	values := make(map[string]string, len(static)+len(data))
	for _, k := range sortedKeys(static) {
		v := static[k]
		for _, r := range v {
			if r > 127 {
				return nil, fmt.Errorf("static value %q contains a character outside 0-127", k)
			}
		}
		values[k] = v
	}
	for _, k := range sortedKeys(data) {
		if _, dup := static[k]; dup {
			return nil, fmt.Errorf("value %q is both static and data", k)
		}
		values[k] = Clean(data[k])
	}
	for _, s := range t.slots {
		if _, ok := values[s.Name]; !ok {
			return nil, fmt.Errorf("no value for slot %q (subpage %d row %d)", s.Name, s.Sub+1, s.Row)
		}
	}

	p, err := tti.Parse(t.src)
	if err != nil {
		return nil, err // Parse validated these bytes, so this cannot happen
	}
	p.SetMPP(mpp)
	for sub := 0; sub < p.Subpages(); sub++ {
		for row := 1; row <= 24; row++ {
			if err := p.SetRow(sub, row, t.renderRow(p, sub, row, values)); err != nil {
				return nil, fmt.Errorf("subpage %d row %d: %w", sub+1, row, err)
			}
		}
	}
	return p, nil
}

// renderRow returns the cells row 1-24 of sub should hold. Cells and values are
// 0-127, one byte per cell, so byte offsets are cell offsets.
func (t *Template) renderRow(p *tti.Page, sub, row int, values map[string]string) string {
	cells, ok := p.Row(sub, row)
	if !ok {
		return spaces
	}
	out := []byte(cells)
	hasSlots, allEmpty := false, true
	for _, s := range t.slots {
		if s.Sub != sub || s.Row != row {
			continue
		}
		v := values[s.Name]
		hasSlots, allEmpty = true, allEmpty && v == ""
		copy(out[s.Col:s.Col+s.Width], fit(v, s.Width, s.Align))
	}
	if rule, ok := t.rowRules[row]; ok {
		if values[rule] == "" {
			return spaces
		}
	} else if hasSlots && allEmpty {
		return spaces
	}
	return string(out)
}

func sortedKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
