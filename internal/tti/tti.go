// Package tti parses vbit2 TTI files into ordered raw lines and serialises
// them back to identical bytes (ADR 0002, codec contract).
//
// Known line codes: PN, OL, PS (validated), DE (description), and SC, CT, FL,
// DS, RE, PF (passed through raw, no rule). Unknown codes and lines without a
// "XX," prefix are kept as they are. Typed fields are computed by scanning
// Lines, so they cannot go stale when lines are edited.
package tti

import (
	"bytes"
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"
)

// Line is one line of a TTI file: its bytes without the terminator, and the terminator.
type Line struct {
	Raw []byte // e.g. "OL,1,..." or "PN,10001"; the whole line when it has no "XX," prefix
	End string // "\r\n", "\n", or "" for an unterminated final line
}

// Code returns the two-letter line code ("PN", "OL", ...) or "" when the line has no "XX," prefix.
func (l Line) Code() string {
	r := l.Raw
	if len(r) >= 3 && r[0] >= 'A' && r[0] <= 'Z' && r[1] >= 'A' && r[1] <= 'Z' && r[2] == ',' {
		return string(r[:2])
	}
	return ""
}

// Body returns the bytes after "XX," (nil when Code is "").
func (l Line) Body() []byte {
	if l.Code() == "" {
		return nil
	}
	return l.Raw[3:]
}

// Page is the ordered list of lines of one TTI file.
type Page struct{ Lines []Line }

const esc = 0x1B

// Parse splits b into lines and validates the PN, OL and PS lines.
func Parse(b []byte) (*Page, error) {
	p := &Page{}
	for len(b) > 0 {
		var l Line
		if i := bytes.IndexByte(b, '\n'); i < 0 {
			l.Raw, b = b, nil
		} else {
			l.Raw, l.End, b = b[:i], "\n", b[i+1:]
			if n := len(l.Raw); n > 0 && l.Raw[n-1] == '\r' {
				l.Raw, l.End = l.Raw[:n-1], "\r\n"
			}
		}
		p.Lines = append(p.Lines, l)
	}
	firstPN := 0 // 1-based line of the first PN, 0 when none yet
	var mpp int
	for i, l := range p.Lines {
		n := i + 1
		body := string(l.Body())
		var err error
		switch l.Code() {
		case "PN":
			var m int
			if m, err = parsePN(body); err != nil {
				break
			}
			if firstPN == 0 {
				firstPN, mpp = n, m
			} else if m != mpp {
				err = fmt.Errorf("PN mpp %x differs from %x at line %d", m, mpp, firstPN)
			}
		case "OL":
			err = checkOL(body)
		case "PS":
			_, err = parsePS(body)
		}
		if err != nil {
			return nil, fmt.Errorf("line %d: %w", n, err)
		}
	}
	if firstPN == 0 {
		return nil, fmt.Errorf("no PN line")
	}
	return p, nil
}

func parsePN(s string) (int, error) {
	v, err := strconv.ParseUint(s, 16, 32)
	if len(s) != 5 || err != nil {
		return 0, fmt.Errorf("PN: want 5 hex digits, got %q", s)
	}
	mpp := int(v >> 8)
	if mpp>>8 < 1 || mpp>>8 > 8 {
		return 0, fmt.Errorf("PN: magazine must be 1-8, got %d", mpp>>8)
	}
	return mpp, nil
}

func parsePS(s string) (uint64, error) {
	v, err := strconv.ParseUint(s, 16, 16)
	if len(s) < 1 || len(s) > 4 || err != nil {
		return 0, fmt.Errorf("PS: want hex, got %q", s)
	}
	return v, nil
}

func checkOL(s string) error {
	i := 0
	for i < len(s) && s[i] >= '0' && s[i] <= '9' {
		i++
	}
	if i == 0 || i >= len(s) || s[i] != ',' {
		return fmt.Errorf(`OL: want "row,cells"`)
	}
	row, err := strconv.Atoi(s[:i])
	if err != nil || row > 29 {
		return fmt.Errorf("OL row %s outside 0-29", s[:i])
	}
	cells := 0
	for j := i + 1; j < len(s); j++ {
		if s[j] == esc {
			j++ // ESC and its following byte are one cell
		}
		cells++
	}
	if cells > 40 {
		return fmt.Errorf("OL row %d has %d cells", row, cells)
	}
	return nil
}

// Bytes concatenates every line and terminator; Bytes(Parse(x)) equals x.
func (p *Page) Bytes() []byte {
	var out []byte
	for _, l := range p.Lines {
		out = append(append(out, l.Raw...), l.End...)
	}
	return out
}

// MPP returns magazine*256+page from the first PN line, e.g. 0x81A.
func (p *Page) MPP() int {
	for _, l := range p.Lines {
		if l.Code() == "PN" {
			m, _ := parsePN(string(l.Body()))
			return m
		}
	}
	return 0
}

// SetMPP rewrites the page number in every PN line, keeping each subpage suffix.
func (p *Page) SetMPP(mpp int) {
	for sub := 0; sub < p.Subpages(); sub++ {
		pn, _ := p.Field(sub, "PN")
		p.SetField(sub, "PN", fmt.Sprintf("%03X", mpp)+pn[3:])
	}
}

// Magazine returns MPP() >> 8.
func (p *Page) Magazine() int { return p.MPP() >> 8 }

// Subpages returns the number of PN lines.
func (p *Page) Subpages() int {
	n := 0
	for _, l := range p.Lines {
		if l.Code() == "PN" {
			n++
		}
	}
	return n
}

// Description returns the body of the first DE line, or "" when there is none.
func (p *Page) Description() string {
	for _, l := range p.Lines {
		if l.Code() == "DE" {
			return string(l.Body())
		}
	}
	return ""
}

// Transmit is true when any PS line, anywhere in the file, has bit 15 set.
func (p *Page) Transmit() bool {
	for _, l := range p.Lines {
		if l.Code() == "PS" {
			if v, err := parsePS(string(l.Body())); err == nil && v&0x8000 != 0 {
				return true
			}
		}
	}
	return false
}

// bounds returns the line range [start, end) of subpage sub (its PN line to the
// next PN); sub < 0 is the page-level range before the first PN. ok is false
// when sub does not exist.
func (p *Page) bounds(sub int) (start, end int, ok bool) {
	n := -1
	for i, l := range p.Lines {
		if l.Code() != "PN" {
			continue
		}
		n++
		if sub < 0 {
			return 0, i, true
		}
		if n == sub {
			start = i
		} else if n == sub+1 {
			return start, i, true
		}
	}
	return start, len(p.Lines), n >= sub
}

// olRow returns the row number and cell bytes of an OL line.
func olRow(l Line) (row int, cells []byte, ok bool) {
	if l.Code() != "OL" {
		return 0, nil, false
	}
	b := l.Body()
	i := bytes.IndexByte(b, ',')
	if i < 0 {
		return 0, nil, false
	}
	row, err := strconv.Atoi(string(b[:i]))
	return row, b[i+1:], err == nil
}

// decodeCells applies the ADR 0002 decode: ESC x gives x-0x40, a byte >= 0x80 gives b&0x7F.
func decodeCells(b []byte) string {
	out := make([]byte, 0, len(b))
	for i := 0; i < len(b); i++ {
		c := b[i]
		if c == esc && i+1 < len(b) {
			i++
			c = b[i] - 0x40
		}
		out = append(out, c&0x7F)
	}
	return string(out)
}

// Row returns subpage sub's OL row (0-29) decoded as cell values 0-127
// (ADR 0002 decode), and false when that subpage has no such line.
func (p *Page) Row(sub, row int) (cells string, ok bool) {
	start, end, found := p.bounds(sub)
	if !found {
		return "", false
	}
	for _, l := range p.Lines[start:end] {
		if r, b, isOL := olRow(l); isOL && r == row {
			return decodeCells(b), true
		}
	}
	return "", false
}

// eightBit reports whether the page writes control codes as bytes 0x80-0x9F
// (first control code found in file order); ESC form otherwise.
func (p *Page) eightBit() bool {
	for _, l := range p.Lines {
		if r, b, ok := olRow(l); ok && r <= 24 { // rows 25-29 hold enhancement data, not control codes
			for _, c := range b {
				if c == esc {
					return false
				}
				if c >= 0x80 && c <= 0x9F {
					return true
				}
			}
		}
	}
	return false
}

// SetRow makes cells the content of subpage sub's OL row. It returns an error
// when cells has more than 40 code points or one above 127. It does nothing
// when cells equals Row. Otherwise it replaces the line's Raw (keeping End), or
// inserts "OL,<row>,<encoded>" after the subpage's last OL line with a lower
// row number, else before its first OL line, else at the subpage's end.
func (p *Page) SetRow(sub, row int, cells string) error {
	if n := utf8.RuneCountInString(cells); n > 40 {
		return fmt.Errorf("%d cells (max 40)", n)
	}
	for _, r := range cells {
		if r > 127 {
			return fmt.Errorf("contains a character outside 0-127")
		}
	}
	start, end, found := p.bounds(sub)
	if !found || row < 0 || row > 29 {
		return fmt.Errorf("no such subpage or row")
	}
	if old, ok := p.Row(sub, row); ok && old == cells {
		return nil
	}
	eight := p.eightBit()
	enc := []byte(fmt.Sprintf("OL,%d,", row))
	for i := 0; i < len(cells); i++ {
		switch c := cells[i]; {
		case c >= 0x20:
			enc = append(enc, c)
		case eight:
			enc = append(enc, c|0x80)
		default:
			enc = append(enc, esc, c+0x40)
		}
	}
	firstOL, after := -1, -1
	for i := start; i < end; i++ {
		r, _, isOL := olRow(p.Lines[i])
		if !isOL {
			continue
		}
		if r == row {
			p.Lines[i].Raw = enc
			return nil
		}
		if firstOL < 0 {
			firstOL = i
		}
		if r < row {
			after = i + 1
		}
	}
	switch {
	case after >= 0:
		p.insert(after, enc)
	case firstOL >= 0:
		p.insert(firstOL, enc)
	default:
		p.insert(end, enc)
	}
	return nil
}

// insert adds a line at index i, copying the terminator of the line before it.
// An unterminated last line gets the file's first terminator so the new line can be last.
func (p *Page) insert(i int, raw []byte) {
	end := p.Lines[0].End
	if i > 0 {
		end = p.Lines[i-1].End
	}
	nl := Line{Raw: raw, End: end}
	if end == "" {
		first := p.Lines[0].End
		if first == "" {
			first = "\n"
		}
		if i == len(p.Lines) {
			p.Lines[i-1].End = first
		} else {
			nl.End = first
		}
	}
	p.Lines = append(p.Lines[:i], append([]Line{nl}, p.Lines[i:]...)...)
}

// Field returns the body of the first line with code inside subpage sub
// (after its PN, before the next PN); sub < 0 searches the page-level lines
// before the first PN.
func (p *Page) Field(sub int, code string) (body string, ok bool) {
	start, end, found := p.bounds(sub)
	if !found {
		return "", false
	}
	for _, l := range p.Lines[start:end] {
		if l.Code() == code {
			return string(l.Body()), true
		}
	}
	return "", false
}

// SetField sets that line's body (no change when equal), or inserts
// "code,body": page-level at line 1, in a subpage directly after its PN line.
func (p *Page) SetField(sub int, code, body string) {
	start, end, found := p.bounds(sub)
	if !found {
		return
	}
	for i := start; i < end; i++ {
		if p.Lines[i].Code() == code {
			if string(p.Lines[i].Body()) != body {
				p.Lines[i].Raw = []byte(code + "," + body)
			}
			return
		}
	}
	if sub < 0 {
		p.insert(0, []byte(code+","+body))
	} else {
		p.insert(start+1, []byte(code+","+body))
	}
}

// SetTransmit sets or clears bit 15 on every PS line in the file (D-011); when
// on and no PS line exists it inserts PS,8000 after the first PN.
func (p *Page) SetTransmit(on bool) {
	seen := false
	for i, l := range p.Lines {
		if l.Code() != "PS" {
			continue
		}
		seen = true
		v, _ := parsePS(string(l.Body()))
		if (v&0x8000 != 0) == on {
			continue
		}
		if on {
			v |= 0x8000
		} else {
			v &^= 0x8000
		}
		p.Lines[i].Raw = []byte(fmt.Sprintf("PS,%04X", v))
	}
	if on && !seen {
		for i, l := range p.Lines {
			if l.Code() == "PN" {
				p.insert(i+1, []byte("PS,8000"))
				return
			}
		}
	}
}

// AddSubpage appends a copy of the last subpage's lines, with the PN's last two
// digits and the SC body set to the new 1-based index (%02X and %04X).
func (p *Page) AddSubpage() {
	n := p.Subpages() + 1
	start, _, _ := p.bounds(n - 2)
	oldLast := len(p.Lines) - 1
	for _, l := range p.Lines[start:] {
		c := Line{Raw: append([]byte(nil), l.Raw...), End: l.End}
		switch l.Code() {
		case "PN":
			c.Raw = append(c.Raw[:len(c.Raw)-2], fmt.Sprintf("%02X", n)...)
		case "SC":
			c.Raw = []byte(fmt.Sprintf("SC,%04X", n))
		}
		p.Lines = append(p.Lines, c)
	}
	// an unterminated final line needs a terminator now that lines follow it
	if last := &p.Lines[oldLast]; last.End == "" {
		if last.End = p.Lines[0].End; last.End == "" {
			last.End = "\n"
		}
	}
}

// RemoveSubpage deletes subpage sub's lines; error when it is the only one or absent.
func (p *Page) RemoveSubpage(sub int) error {
	start, end, ok := p.bounds(sub)
	if sub < 0 || !ok {
		return fmt.Errorf("no subpage %d", sub)
	}
	if p.Subpages() == 1 {
		return fmt.Errorf("cannot remove the only subpage")
	}
	p.Lines = append(p.Lines[:start], p.Lines[end:]...)
	return nil
}

// ParseCT parses a CT body such as "10,T" (mode 'T' or 'C').
func ParseCT(body string) (seconds int, mode byte, ok bool) {
	a, b, found := strings.Cut(strings.TrimSpace(body), ",")
	b = strings.TrimSpace(b)
	n, err := strconv.Atoi(strings.TrimSpace(a))
	if !found || err != nil || n < 0 || len(b) != 1 || (b[0] != 'T' && b[0] != 'C') {
		return 0, 0, false
	}
	return n, b[0], true
}

// ParseFL parses six comma-separated 3-hex-digit links, trimmed and lowercased.
func ParseFL(body string) (links [6]string, ok bool) {
	f := strings.Split(body, ",")
	if len(f) != 6 {
		return links, false
	}
	for i, s := range f {
		s = strings.ToLower(strings.TrimSpace(s))
		if _, err := strconv.ParseUint(s, 16, 12); err != nil || len(s) != 3 {
			return links, false
		}
		links[i] = s
	}
	return links, true
}

// NewPage is the blank page of D-016: one subpage of 24 rows of 40 spaces, not transmitted
// (PS bit 15 clear, so it is not broadcast until an editor turns Transmit on), with no row 0.
func NewPage(mpp int) *Page {
	var b strings.Builder
	fmt.Fprintf(&b, "DE,\r\nPN,%03X00\r\nSC,0000\r\nPS,0000\r\nCT,8,T\r\nFL,8ff,8ff,8ff,8ff,8ff,8ff\r\n", mpp)
	for r := 1; r <= 24; r++ {
		fmt.Fprintf(&b, "OL,%d,%s\r\n", r, strings.Repeat(" ", 40))
	}
	p, err := Parse([]byte(b.String()))
	if err != nil {
		panic(err) // the template above is valid for any mpp in magazines 1-8
	}
	return p
}
