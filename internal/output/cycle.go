package output

import (
	"math"
	"strconv"
	"strings"

	"ceems/internal/store"
	"ceems/internal/tti"
)

// CycleTimes estimates, per magazine (1 to 8) that has transmitting pages, how many
// seconds vbit2 takes to send each of its pages once (Q-027). A page that does not
// parse is skipped.
//
// shortcut: a magazine held at its minimum cycle gives spare capacity back to the
// others, and vbit2's force round favours low-priority magazines; both are ignored.
// Upgrade only if the estimate proves too far off on hardware.
func CycleTimes(pages []store.Page, s store.Settings) map[int]float64 {
	lines := float64(max(s.LinesPerField, 1))
	prio := strings.Split(s.MagazinePriority, ",") // magazine 8 first, then 1 to 7
	type mag struct{ sumA, sumD float64 }
	mags := map[int]*mag{}
	for _, p := range pages {
		t, err := tti.Parse(p.TTI)
		if err != nil || t.Subpages() == 0 {
			continue
		}
		m := t.Magazine()
		if m < 1 || m > 8 {
			continue
		}
		r := 0.0
		for sub := 0; sub < t.Subpages(); sub++ {
			r += float64(subpagePackets(t, sub, s.RowAdaptiveMode))
		}
		r /= float64(t.Subpages())
		g := mags[m]
		if g == nil {
			g = &mag{}
			mags[m] = g
		}
		g.sumA += 1 + math.Floor(r/lines)
		g.sumD += 1 + r
	}
	weight := func(m int) float64 {
		i := m
		if m == 8 {
			i = 0
		}
		if i < len(prio) {
			if v, err := strconv.Atoi(strings.TrimSpace(prio[i])); err == nil && v >= 1 {
				return 1 / float64(v)
			}
		}
		return 1
	}
	var total float64
	for m := range mags {
		total += weight(m)
	}
	capacity := 50*lines - 5 // packets per second
	out := make(map[int]float64, len(mags))
	for m, g := range mags {
		a := math.Max(1, (1+g.sumA)/50)
		d := (1 + g.sumD) / (capacity * weight(m) / total)
		out[m] = math.Max(a, d)
	}
	return out
}

// subpagePackets counts the packets sent after the subpage's header.
func subpagePackets(t *tti.Page, sub int, adaptive bool) int {
	n := 0
	for row := 1; row <= 24; row++ {
		c, ok := t.Row(sub, row)
		if !adaptive || (ok && strings.Trim(c, " ") != "") {
			n++
		}
	}
	if body, ok := t.Field(sub, "FL"); ok {
		if l, ok := tti.ParseFL(body); ok {
			for _, x := range l {
				if x != "000" {
					n++
					break
				}
			}
		}
	}
	for _, row := range []int{26, 27, 28} {
		if _, ok := t.Row(sub, row); ok {
			n++
		}
	}
	return n
}
