package store

import (
	"bytes"
	"testing"

	"ceems/internal/tti"
)

func TestTransmitPages(t *testing.T) {
	s := openTemp(t)
	if _, err := s.ImportPages(fixtures(t), "test"); err != nil {
		t.Fatal(err)
	}
	p, err := s.Page(0x101) // every fixture transmits; make one that does not
	if err != nil {
		t.Fatal(err)
	}
	off, err := tti.Parse(p.TTI)
	if err != nil {
		t.Fatal(err)
	}
	off.SetTransmit(false)
	if _, err := s.SavePage(0x101, off, "", Audit{Actor: "t"}); err != nil {
		t.Fatal(err)
	}
	all, err := s.Pages()
	if err != nil {
		t.Fatal(err)
	}
	got, err := s.TransmitPages()
	if err != nil {
		t.Fatal(err)
	}
	i := 0
	for _, p := range all {
		if !p.Transmit {
			continue
		}
		full, err := s.Page(p.MPP)
		if err != nil {
			t.Fatal(err)
		}
		if i >= len(got) || got[i].MPP != p.MPP || !bytes.Equal(got[i].TTI, full.TTI) {
			t.Fatalf("TransmitPages[%d] is not page %03X with its bytes", i, p.MPP)
		}
		i++
	}
	if i == 0 || i != len(got) || len(got) == len(all) {
		t.Fatalf("matched %d of %d returned pages (want at least one, and no extras)", i, len(got))
	}
}
