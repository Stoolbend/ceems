package tti

import (
	"bytes"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestFixtures(t *testing.T) {
	files, err := filepath.Glob("../../testdata/sample/pages/*.tti")
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 30 {
		t.Fatalf("want 30 fixtures, got %d", len(files))
	}
	subpages := map[string]int{"P100": 7, "P101": 5, "P120": 2}
	desc := map[string]string{
		"P100": "Index", "P101": "Help", "P120": "News", "P121": "Weather", "P199": "Contents",
		"P200": "Sport", "P300": "TV guide", "P400": "High scores", "P700": "Puzzles",
		"P704": "Colour bars", "P705": "Test card", "P800": "Engineering",
		"P418": "", "P573": "", "P701": "", "P702": "", "P703": "", "P810": "",
	}
	for _, f := range files {
		name := strings.TrimSuffix(filepath.Base(f), ".tti")
		t.Run(name, func(t *testing.T) {
			b, err := os.ReadFile(f)
			if err != nil {
				t.Fatal(err)
			}
			p, err := Parse(b)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(p.Bytes(), b) {
				t.Error("Bytes() differs from file")
			}
			want, _ := strconv.ParseInt(name[1:], 16, 32)
			if p.MPP() != int(want) || p.Magazine() != int(want)>>8 {
				t.Errorf("MPP %x magazine %d, want %x", p.MPP(), p.Magazine(), want)
			}
			if !p.Transmit() {
				t.Error("Transmit false")
			}
			wantSub := 1
			if n, ok := subpages[name]; ok {
				wantSub = n
			}
			if p.Subpages() != wantSub {
				t.Errorf("Subpages %d, want %d", p.Subpages(), wantSub)
			}
			wantDesc, ok := desc[name]
			if !ok {
				wantDesc = "Test page"
			}
			if p.Description() != wantDesc {
				t.Errorf("Description %q, want %q", p.Description(), wantDesc)
			}
		})
	}
}

func TestSyntheticPositive(t *testing.T) {
	cells39 := strings.Repeat("a", 38) + "\x1bA" // 38 + ESC pair = 39 cells; with "b" 40 cells in 41 bytes
	cases := map[string]string{
		"LF only":            "DE,x\nPN,10000\nOL,1,hi\n",
		"no final newline":   "PN,10000\r\nOL,1,hi",
		"bare CR inside":     "PN,10000\r\nOL,1,a\rb\r\n",
		"unknown and blank":  "PN,10000\r\nXX,foo\r\n\r\ngarbage\r\n",
		"PN at line 1":       "PN,10000\r\n",
		"lowercase PN":       "PN,81a00\r\n",
		"ESC pairs 40 cells": "PN,10000\r\nOL,2," + cells39 + "b\r\n",
		"8-bit controls":     "PN,10000\r\nOL,3," + strings.Repeat("\x81", 40) + "\r\n",
	}
	for name, in := range cases {
		t.Run(name, func(t *testing.T) {
			p, err := Parse([]byte(in))
			if err != nil {
				t.Fatal(err)
			}
			if string(p.Bytes()) != in {
				t.Errorf("round trip differs: %q", p.Bytes())
			}
		})
	}
	p, err := Parse([]byte("PS,8000\r\nPN,10000\r\n"))
	if err != nil || !p.Transmit() {
		t.Errorf("PS before PN: err %v", err)
	}
	if p, _ := Parse([]byte("PN,10000\r\nPS,0000\r\n")); p.Transmit() {
		t.Error("PS,0000 should not transmit")
	}
}

func TestMalformed(t *testing.T) {
	cases := []struct{ name, in, want string }{
		{"PN 4 digits", "DE,x\r\nPN,1000\r\n", "line 2: PN:"},
		{"PN non-hex", "DE,x\r\nPN,1000g\r\n", "line 2: PN:"},
		{"magazine 0", "DE,x\r\nPN,00100\r\n", "line 2: PN: magazine"},
		{"magazine 9", "DE,x\r\nPN,90000\r\n", "line 2: PN: magazine"},
		{"differing mpp", "DE,x\r\nPN,10000\r\nPN,10100\r\n", "line 3: PN mpp 101 differs from 100 at line 2"},
		{"no PN", "DE,x\r\nOL,1,hi\r\n", "no PN line"},
		{"empty", "", "no PN line"},
		{"row 30", "PN,10000\r\nOL,30,hi\r\n", "line 2: OL row 30 outside 0-29"},
		{"row non-numeric", "PN,10000\r\nOL,x,hi\r\n", "line 2: OL:"},
		{"no comma", "PN,10000\r\nOL,5\r\n", "line 2: OL:"},
		{"41 cells", "PN,10000\r\nOL,5," + strings.Repeat("a", 41) + "\r\n", "line 2: OL row 5 has 41 cells"},
		{"39 + ESC + 1", "PN,10000\r\nOL,5," + strings.Repeat("a", 39) + "\x1bA" + "b\r\n", "line 2: OL row 5 has 41 cells"},
		{"PS zz", "PN,10000\r\nPS,zz\r\n", "line 2: PS:"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := Parse([]byte(c.in))
			if err == nil || !strings.HasPrefix(err.Error(), c.want) {
				t.Errorf("err %v, want prefix %q", err, c.want)
			}
		})
	}
}

func load(t *testing.T, name string) *Page {
	t.Helper()
	b, err := os.ReadFile("../../testdata/sample/pages/" + name)
	if err != nil {
		t.Fatal(err)
	}
	p, err := Parse(b)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

// changedLines counts the lines that differ between two serialisations of the same line count.
func changedLines(a, b []byte) int {
	la, lb := bytes.Split(a, []byte("\n")), bytes.Split(b, []byte("\n"))
	if len(la) != len(lb) {
		return -1
	}
	n := 0
	for i := range la {
		if !bytes.Equal(la[i], lb[i]) {
			n++
		}
	}
	return n
}

func TestRowSetRow(t *testing.T) {
	p := load(t, "P100.tti")
	orig := p.Bytes()
	cells, ok := p.Row(0, 1)
	if !ok || len(cells) != 40 || cells[0] != 0x0D {
		t.Fatalf("row 1 = %q %v", cells, ok)
	}
	if err := p.SetRow(0, 1, cells); err != nil || !bytes.Equal(p.Bytes(), orig) {
		t.Fatalf("unchanged SetRow altered the page (err %v)", err)
	}
	if err := p.SetRow(0, 1, "\x0dHELLO"); err != nil {
		t.Fatal(err)
	}
	if n := changedLines(orig, p.Bytes()); n != 1 || !bytes.Contains(p.Bytes(), []byte("OL,1,\x1bMHELLO\r\n")) {
		t.Errorf("changed lines %d, ESC form: %v", n, bytes.Contains(p.Bytes(), []byte("OL,1,\x1bMHELLO")))
	}
	if err := p.SetRow(0, 1, strings.Repeat("a", 41)); err == nil || !strings.Contains(err.Error(), "41 cells") {
		t.Errorf("41 cells: %v", err)
	}
	if err := p.SetRow(0, 1, "\x80"); err == nil || !strings.Contains(err.Error(), "outside 0-127") {
		t.Errorf("128: %v", err)
	}
}

func TestSetRowInsertPosition(t *testing.T) {
	p := load(t, "P81A.tti") // rows 2,6,7,9,... present
	if _, ok := p.Row(0, 4); ok {
		t.Fatal("row 4 should be absent")
	}
	if err := p.SetRow(0, 4, "new"); err != nil {
		t.Fatal(err)
	}
	for i, l := range p.Lines {
		if string(l.Raw) == "OL,4,new" {
			if prev := string(p.Lines[i-1].Raw); !strings.HasPrefix(prev, "OL,2,") {
				t.Errorf("inserted after %q, want OL,2", prev)
			}
			if !strings.HasPrefix(string(p.Lines[i+1].Raw), "OL,6,") {
				t.Errorf("inserted before %q, want OL,6", p.Lines[i+1].Raw)
			}
			return
		}
	}
	t.Error("inserted line not found")
}

func TestSetRowFormsAndUnterminated(t *testing.T) {
	p, _ := Parse([]byte("PN,10000\nOL,2,\x81abc\nOL,5,x"))
	if err := p.SetRow(0, 3, "\x01z"); err != nil {
		t.Fatal(err)
	}
	if err := p.SetRow(0, 9, "end"); err != nil {
		t.Fatal(err)
	}
	if got, want := string(p.Bytes()), "PN,10000\nOL,2,\x81abc\nOL,3,\x81z\nOL,5,x\nOL,9,end"; got != want {
		t.Errorf("got %q want %q", got, want)
	}
}

func TestP121Row4RoundTrips(t *testing.T) {
	p := load(t, "P121.tti")
	orig := p.Bytes()
	cells, _ := p.Row(0, 4)
	if !strings.ContainsRune(cells, 0x1F) {
		t.Fatalf("cells %q", cells)
	}
	if err := p.SetRow(0, 4, cells); err != nil || !bytes.Equal(p.Bytes(), orig) {
		t.Errorf("round trip changed bytes (err %v)", err)
	}
}

func TestFieldSetField(t *testing.T) {
	p := load(t, "P810.tti") // no DE
	if _, ok := p.Field(-1, "DE"); ok {
		t.Fatal("P810 has a DE")
	}
	p.SetField(-1, "DE", "hello")
	if string(p.Lines[0].Raw) != "DE,hello" || p.Description() != "hello" {
		t.Errorf("DE not at line 1: %q", p.Lines[0].Raw)
	}
	p.SetField(-1, "DE", "hello")
	if len(p.Lines) != len(load(t, "P810.tti").Lines)+1 {
		t.Error("equal SetField inserted again")
	}
	q := load(t, "P701.tti") // no CT
	q.SetField(0, "CT", "10,T")
	for i, l := range q.Lines {
		if l.Code() == "PN" {
			if string(q.Lines[i+1].Raw) != "CT,10,T" {
				t.Errorf("after PN: %q", q.Lines[i+1].Raw)
			}
			break
		}
	}
	if body, ok := q.Field(0, "CT"); !ok || body != "10,T" {
		t.Errorf("Field = %q %v", body, ok)
	}
}

func TestSetTransmit(t *testing.T) {
	p := load(t, "P701.tti") // page-level PS
	p.SetTransmit(false)
	if p.Transmit() || !strings.Contains(string(p.Bytes()), "PS,0000") {
		t.Errorf("off: %v", p.Transmit())
	}
	p.SetTransmit(true)
	if !p.Transmit() {
		t.Error("on failed")
	}
	n, _ := Parse([]byte("PN,10000\r\nOL,1,hi\r\n"))
	n.SetTransmit(true)
	if string(n.Bytes()) != "PN,10000\r\nPS,8000\r\nOL,1,hi\r\n" || !n.Transmit() {
		t.Errorf("no PS: %q", n.Bytes())
	}
}

func TestAddRemoveSubpage(t *testing.T) {
	p := load(t, "P100.tti")
	p.AddSubpage()
	q, err := Parse(p.Bytes())
	if err != nil || q.Subpages() != 8 {
		t.Fatalf("add: %v %d", err, q.Subpages())
	}
	if sc, _ := q.Field(7, "SC"); sc != "0008" {
		t.Errorf("SC %q", sc)
	}
	if !strings.Contains(string(q.Bytes()), "PN,10008") {
		t.Error("no PN,10008")
	}
	if err := q.RemoveSubpage(0); err != nil {
		t.Fatal(err)
	}
	if r, err := Parse(q.Bytes()); err != nil || r.Subpages() != 7 {
		t.Errorf("remove: %v", err)
	}
	one, _ := Parse([]byte("PN,10000\r\n"))
	if one.RemoveSubpage(0) == nil {
		t.Error("removed the only subpage")
	}
	u, _ := Parse([]byte("PN,10000\nSC,0001\nOL,1,x"))
	u.AddSubpage()
	if string(u.Bytes()) != "PN,10000\nSC,0001\nOL,1,x\nPN,10002\nSC,0002\nOL,1,x" {
		t.Errorf("unterminated add: %q", u.Bytes())
	}
}

func TestParseCTFL(t *testing.T) {
	if s, m, ok := ParseCT("10,T"); !ok || s != 10 || m != 'T' {
		t.Error("CT 10,T")
	}
	if _, _, ok := ParseCT("10,X"); ok {
		t.Error("CT mode X accepted")
	}
	l, ok := ParseFL("199,702,704,700,8FF,8ff  ")
	if !ok || l != [6]string{"199", "702", "704", "700", "8ff", "8ff"} {
		t.Errorf("FL %v %v", l, ok)
	}
	if _, ok := ParseFL("199,702"); ok {
		t.Error("short FL accepted")
	}
	p := load(t, "P704.tti")
	if body, _ := p.Field(0, "FL"); body == "" {
		t.Fatal("no FL")
	} else if _, ok := ParseFL(body); !ok {
		t.Errorf("fixture FL %q not parsed", body)
	}
}

func TestNewPage(t *testing.T) {
	p := NewPage(0x2A0)
	q, err := Parse(p.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	if q.MPP() != 0x2A0 || q.Subpages() != 1 || q.Transmit() || q.Description() != "" {
		t.Errorf("mpp %x subpages %d transmit %v de %q", q.MPP(), q.Subpages(), q.Transmit(), q.Description())
	}
	for r := 1; r <= 24; r++ {
		if c, ok := q.Row(0, r); !ok || c != strings.Repeat(" ", 40) {
			t.Errorf("row %d: %q %v", r, c, ok)
		}
	}
	if _, ok := q.Row(0, 0); ok {
		t.Error("blank page has a row 0")
	}
	want := "DE,\r\nPN,2A000\r\nSC,0000\r\nPS,0000\r\nCT,8,T\r\nFL,8ff,8ff,8ff,8ff,8ff,8ff\r\nOL,1,"
	if !strings.HasPrefix(string(p.Bytes()), want+strings.Repeat(" ", 40)+"\r\nOL,2,") {
		t.Errorf("header lines: %q", p.Bytes()[:90])
	}
}

func TestSetMPP(t *testing.T) {
	p, _ := Parse([]byte("PN,10000\r\nOL,1,a\r\nPN,10002\r\nOL,1,b\r\n"))
	p.SetMPP(0x8A5)
	if want := "PN,8A500\r\nOL,1,a\r\nPN,8A502\r\nOL,1,b\r\n"; string(p.Bytes()) != want {
		t.Errorf("got %q, want %q", p.Bytes(), want)
	}
	if p.MPP() != 0x8A5 {
		t.Errorf("MPP %03X", p.MPP())
	}
}

func FuzzParse(f *testing.F) {
	files, _ := filepath.Glob("../../testdata/sample/pages/*.tti")
	for _, name := range files {
		b, err := os.ReadFile(name)
		if err != nil {
			f.Fatal(err)
		}
		f.Add(b)
	}
	for _, s := range []string{"", "\r", "PN,10000", "PN,zzzzz\r\n", "PN,10000\nOL,1,x\nPS,8000\n"} {
		f.Add([]byte(s))
	}
	f.Fuzz(func(t *testing.T, b []byte) {
		p, err := Parse(b)
		if err != nil {
			return
		}
		p.MPP()
		p.Description()
		p.Subpages()
		p.Transmit()
		out := p.Bytes()
		q, err := Parse(out)
		if err != nil || !bytes.Equal(q.Bytes(), out) {
			t.Fatalf("round trip failed: err=%v", err)
		}
	})
}
