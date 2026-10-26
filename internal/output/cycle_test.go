package output

import (
	"fmt"
	"math"
	"strings"
	"testing"

	"ceems/internal/store"
	"ceems/internal/tti"
)

// cyclePage builds a transmitting page in the given magazine with the given row text
// on rows 1-24 (blank rows are spaces) and an FL line.
func cyclePage(mpp int, filledRows int, fl string) store.Page {
	var b strings.Builder
	fmt.Fprintf(&b, "DE,\r\nPN,%03X00\r\nSC,0000\r\nPS,8000\r\nCT,8,T\r\nFL,%s\r\n", mpp, fl)
	for r := 1; r <= 24; r++ {
		c := strings.Repeat(" ", 40)
		if r <= filledRows {
			c = "X" + c[1:]
		}
		fmt.Fprintf(&b, "OL,%d,%s\r\n", r, c)
	}
	if _, err := tti.Parse([]byte(b.String())); err != nil {
		panic(err)
	}
	return store.Page{MPP: mpp, TTI: []byte(b.String())}
}

func near(t *testing.T, name string, got, want float64) {
	t.Helper()
	if math.Abs(got-want) > 1e-9 {
		t.Errorf("%s = %v, want %v", name, got, want)
	}
}

func TestCycleTimes(t *testing.T) {
	const fl = "100,100,100,100,100,100"
	s := store.Settings{LinesPerField: 16, MagazinePriority: "9,1,3,3,3,9,9,9"}

	var full []store.Page
	for i := 0; i < 100; i++ {
		full = append(full, cyclePage(0x100+i, 24, fl)) // R = 25, floor(25/16) = 1
	}
	got := CycleTimes(full, s)
	if len(got) != 1 {
		t.Fatalf("magazines %v", got)
	}
	near(t, "100 full pages, adaptive off", got[1], 4.02) // A = (1+200)/50 beats D/C = 2601/795

	// Adaptive on: 4 filled rows + FL = 5 packets; D = 1+100*6 = 601, C = 795.
	s.RowAdaptiveMode = true
	var sparse []store.Page
	for i := 0; i < 100; i++ {
		sparse = append(sparse, cyclePage(0x100+i, 4, fl))
	}
	near(t, "adaptive", CycleTimes(sparse, s)[1], math.Max(1, 101.0/50))

	// Two magazines, one page each, 24 rows and no FL link: R = 24, A = max(1, 3/50) = 1.
	// Priorities 1 and 3 for magazines 1 and 2 (list order: 8,1,2,...): w1 = 3/4, w2 = 1/4.
	// D = 26 packets each; C = 795.
	s = store.Settings{LinesPerField: 16, MagazinePriority: "9,1,3,3,3,9,9,9"}
	two := CycleTimes([]store.Page{cyclePage(0x100, 24, "000,000,000,000,000,000"), cyclePage(0x200, 24, "000,000,000,000,000,000")}, s)
	if len(two) != 2 {
		t.Fatalf("magazines %v", two)
	}
	near(t, "magazine 1", two[1], math.Max(1, 26/(795*0.75)))
	near(t, "magazine 2", two[2], math.Max(1, 26/(795*0.25)))
	if _, ok := two[3]; ok {
		t.Error("magazine without pages present")
	}
}

func TestCycleTimesSkipsUnparseable(t *testing.T) {
	got := CycleTimes([]store.Page{{MPP: 0x100, TTI: []byte("garbage")}}, store.Settings{LinesPerField: 16, MagazinePriority: "1,1,1,1,1,1,1,1"})
	if len(got) != 0 {
		t.Errorf("got %v", got)
	}
}
