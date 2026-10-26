package tmpl

import (
	"bytes"
	"os"
	"strings"
	"testing"

	"ceems/internal/tti"
)

// ttiOf joins lines into a CRLF TTI file.
func ttiOf(lines ...string) []byte { return []byte(strings.Join(lines, "\r\n") + "\r\n") }

func pad(s string) string { return s + strings.Repeat(" ", 40-len(s)) }

func mustParse(t *testing.T, b []byte, rules map[string]string) *Template {
	t.Helper()
	tp, err := Parse(b, rules)
	if err != nil {
		t.Fatal(err)
	}
	return tp
}

func mustRender(t *testing.T, tp *Template, mpp int, static, data map[string]string) *tti.Page {
	t.Helper()
	p, err := tp.Render(mpp, static, data)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func row(t *testing.T, p *tti.Page, sub, r int) string {
	t.Helper()
	c, ok := p.Row(sub, r)
	if !ok {
		t.Fatalf("subpage %d row %d absent", sub, r)
	}
	return c
}

func TestSlotWidths(t *testing.T) {
	// ADR 0002 says "{gname" + 21 dots + "}" is 32 cells; by its own rule (width = match
	// length) that token is 28. 25 dots make the 32-cell slot.
	gname := "{gname" + strings.Repeat(".", 25) + "}"
	tp := mustParse(t, ttiOf("PN,10000",
		"OL,1,"+pad("x{p1name.}y {^title..} {>p1score.}"),
		"OL,3,{1x}{b {_x} {}",
		"OL,2,"+gname+" {a}{A}",
	), nil)
	want := []Slot{
		{Name: "p1name", Row: 1, Col: 1, Width: 9},
		{Name: "title", Row: 1, Col: 12, Width: 10, Align: '^'},
		{Name: "p1score", Row: 1, Col: 23, Width: 11, Align: '>'},
		{Name: "gname", Row: 2, Col: 0, Width: 32},
		{Name: "a", Row: 2, Col: 33, Width: 3},
	}
	got := tp.Slots()
	if len(got) != len(want) {
		t.Fatalf("slots %+v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("slot %d: got %+v want %+v", i, got[i], want[i])
		}
	}
}

func TestSlotsOnlyOnRows1To24(t *testing.T) {
	tp := mustParse(t, ttiOf("PN,10000", "OL,0,{head}", "OL,25,{tail}", "OL,3,{mid}"), nil)
	if s := tp.Slots(); len(s) != 1 || s[0].Name != "mid" {
		t.Fatalf("slots %+v", s)
	}
}

func TestParseRowRules(t *testing.T) {
	b := ttiOf("PN,10000", "OL,1,{a}")
	for _, rules := range []map[string]string{{"0": "a"}, {"25": "a"}, {"01": "a"}, {"x": "a"}, {"1": "zz"}} {
		if _, err := Parse(b, rules); err == nil {
			t.Errorf("rules %v accepted", rules)
		}
	}
	if _, err := Parse(b, map[string]string{"17": "a"}); err != nil {
		t.Error(err)
	}
	if _, err := Parse([]byte("OL,1,x\r\n"), nil); err == nil {
		t.Error("TTI without PN accepted")
	}
}

func TestRenderPadClampAndNoCellMoves(t *testing.T) {
	tp := mustParse(t, ttiOf("PN,10000", "OL,1,\x1bA|{l....}|{^c...}|{>r...}|\x1bGend"), nil)
	orig := row(t, tp.page(t), 0, 1)
	cases := []struct{ l, c, r, want string }{
		{"ab", "ab", "ab", "\x01|ab     |  ab   |     ab|\x07end"},
		{"abc", "abc", "abc", "\x01|abc    |  abc  |    abc|\x07end"},
		{"abcdefghij", "abcdefghij", "abcdefghij", "\x01|abcdefg|abcdefg|abcdefg|\x07end"},
		{"", "x", "", "\x01|       |   x   |       |\x07end"},
	}
	for _, c := range cases {
		p := mustRender(t, tp, 0x100, nil, map[string]string{"l": c.l, "c": c.c, "r": c.r})
		got := row(t, p, 0, 1)
		if got != c.want {
			t.Errorf("%q %q %q: got %q want %q", c.l, c.c, c.r, got, c.want)
		}
		// every cell outside the three slots equals the template's
		inSlot := func(i int) bool {
			for _, s := range tp.Slots() {
				if i >= s.Col && i < s.Col+s.Width {
					return true
				}
			}
			return false
		}
		for i := range orig {
			if !inSlot(i) && got[i] != orig[i] {
				t.Errorf("cell %d moved: %q -> %q", i, orig[i], got[i])
			}
		}
		if len(got) != len(orig) {
			t.Errorf("row length %d, template %d", len(got), len(orig))
		}
	}
}

// page re-parses the template's bytes, for comparing against the source.
func (tp *Template) page(t *testing.T) *tti.Page {
	t.Helper()
	p, err := tti.Parse(tp.src)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func TestRenderBlanking(t *testing.T) {
	tp := mustParse(t, ttiOf("PN,10000",
		"OL,1,keep me",
		"OL,2,name {a..} score {b..}",
		"OL,3,"+pad("-----"),
		"OL,4,ruled {a..} {c..}",
		// rows 5-24 absent
	), map[string]string{"3": "c", "4": "c"})
	p := mustRender(t, tp, 0x100, nil, map[string]string{"a": "", "b": "", "c": ""})
	blank := strings.Repeat(" ", 40)
	if got := row(t, p, 0, 1); got != "keep me" {
		t.Errorf("row 1 %q (short slot-less row must be kept)", got)
	}
	for r := 2; r <= 24; r++ {
		if got := row(t, p, 0, r); got != blank {
			t.Errorf("row %d %q, want 40 spaces", r, got)
		}
	}
	if !bytes.Contains(p.Bytes(), []byte("OL,24,"+blank+"\r\n")) {
		t.Error("row 24 not written as OL,24, + 40 spaces")
	}

	// one non-empty slot keeps row 2; the rule on c alone decides rows 3 and 4
	p = mustRender(t, tp, 0x100, nil, map[string]string{"a": "x", "b": "", "c": ""})
	if got := row(t, p, 0, 2); got != "name "+"x    "+" score "+"     " {
		t.Errorf("row 2 %q", got)
	}
	if got := row(t, p, 0, 4); got != blank {
		t.Errorf("row 4 %q: the rule's empty slot must blank it although a is set", got)
	}
	p = mustRender(t, tp, 0x100, nil, map[string]string{"a": "", "b": "", "c": "y"})
	if got := row(t, p, 0, 3); got != pad("-----") {
		t.Errorf("row 3 %q", got)
	}
	if got := row(t, p, 0, 4); got != "ruled "+"     "+" "+"y    " {
		t.Errorf("row 4 %q", got)
	}
}

func TestRenderMissingKey(t *testing.T) { // mutation A
	tp := mustParse(t, ttiOf("PN,10000", "OL,1,{a..} {p4name.}"), nil)
	_, err := tp.Render(0x100, map[string]string{"a": "1"}, map[string]string{"extra": "ignored"})
	if err == nil || !strings.Contains(err.Error(), `"p4name"`) {
		t.Fatalf("got %v", err)
	}
}

func TestRenderKeyInBothMaps(t *testing.T) {
	tp := mustParse(t, ttiOf("PN,10000", "OL,1,{a..}"), nil)
	if _, err := tp.Render(0x100, map[string]string{"a": "1"}, map[string]string{"a": "2"}); err == nil || !strings.Contains(err.Error(), `"a"`) {
		t.Fatalf("got %v", err)
	}
}

func TestClean(t *testing.T) {
	for in, want := range map[string]string{
		"plain":            "plain",
		"a\x00b\x1fc\x7fd": "abcd",
		"\x1bAred":         "Ared",
		"£5":               "#5",
		"café":             "caf?",
		"bad\xffutf8":      "bad?utf8",
		"日本":               "??",
	} {
		if got := Clean(in); got != want {
			t.Errorf("Clean(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestRenderDataIsCleaned(t *testing.T) {
	tp := mustParse(t, ttiOf("PN,10000", "OL,1,{a.....}"), nil)
	p := mustRender(t, tp, 0x100, nil, map[string]string{"a": "\x01£é"})
	if got := row(t, p, 0, 1); got != "#?      " {
		t.Errorf("row %q", got)
	}
}

func TestRenderStaticControlCodes(t *testing.T) {
	// ESC form: the template has no control codes, so the codec writes ESC x.
	tp := mustParse(t, ttiOf("PN,10000", "OL,1,{t}"), nil)
	p := mustRender(t, tp, 0x100, map[string]string{"t": "\x01\x02\x03"}, nil)
	if !bytes.Contains(p.Bytes(), []byte("OL,1,\x1bA\x1bB\x1bC\r\n")) {
		t.Errorf("ESC form: %q", p.Bytes())
	}
	// 8-bit form: the template writes control codes as 0x80-0x9F.
	tp = mustParse(t, ttiOf("PN,10000", "OL,1,\x87{t}"), nil)
	p = mustRender(t, tp, 0x100, map[string]string{"t": "\x01\x02\x03"}, nil)
	if !bytes.Contains(p.Bytes(), []byte("OL,1,\x87\x81\x82\x83\r\n")) {
		t.Errorf("8-bit form: %q", p.Bytes())
	}
	if _, err := tp.Render(0x100, map[string]string{"t": "é"}, nil); err == nil {
		t.Error("static value above 127 accepted")
	}
}

func TestRenderPNAndPassThrough(t *testing.T) {
	src := ttiOf("DE,high scores", "DS,x", "PN,12300", "SC,0001", "PS,8000", "CT,8,T", "OL,0,header {notaslot}",
		"OL,1,{a..}", "OL,26,enhancement", "ZZ,unknown",
		"PN,12301", "SC,0002", "PS,8000", "FL,100,200,300,400,500,600", "OL,1,second {a..}")
	tp := mustParse(t, src, nil)
	p := mustRender(t, tp, 0x45A, nil, map[string]string{"a": "hi"})
	if p.MPP() != 0x45A || p.Subpages() != 2 {
		t.Fatalf("mpp %X subpages %d", p.MPP(), p.Subpages())
	}
	for sub, want := range []string{"45A00", "45A01"} {
		if pn, _ := p.Field(sub, "PN"); pn != want {
			t.Errorf("subpage %d PN %q want %q", sub, pn, want)
		}
		for r := 1; r <= 24; r++ {
			row(t, p, sub, r) // every row 1-24 exists on every subpage
		}
	}
	if got := row(t, p, 1, 1); got != "second hi   " {
		t.Errorf("subpage 2 row 1 %q", got)
	}
	out := string(p.Bytes())
	for _, l := range []string{"DE,high scores\r\n", "DS,x\r\n", "SC,0001\r\n", "PS,8000\r\n", "CT,8,T\r\n",
		"OL,0,header {notaslot}\r\n", "OL,26,enhancement\r\n", "ZZ,unknown\r\n", "FL,100,200,300,400,500,600\r\n"} {
		if !strings.Contains(out, l) {
			t.Errorf("line %q not passed through", l)
		}
	}
	if !p.Transmit() {
		t.Error("PS transmit bit lost")
	}
	back, err := tti.Parse(p.Bytes())
	if err != nil || !bytes.Equal(back.Bytes(), p.Bytes()) {
		t.Fatalf("round trip: %v", err)
	}
}

func TestRenderFixturesRoundTrip(t *testing.T) {
	for _, name := range []string{"P100.tti", "P121.tti", "P81A.tti"} {
		b, err := os.ReadFile("../../testdata/sample/pages/" + name)
		if err != nil {
			t.Fatal(err)
		}
		tp := mustParse(t, b, nil)
		values := map[string]string{}
		for _, s := range tp.Slots() {
			values[s.Name] = "x"
		}
		p := mustRender(t, tp, 0x8FE, nil, values)
		back, err := tti.Parse(p.Bytes())
		if err != nil || !bytes.Equal(back.Bytes(), p.Bytes()) || back.MPP() != 0x8FE {
			t.Errorf("%s: round trip err %v", name, err)
		}
		for sub := 0; sub < p.Subpages(); sub++ {
			for r := 1; r <= 24; r++ {
				row(t, p, sub, r)
			}
		}
	}
}
