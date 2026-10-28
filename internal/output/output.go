// Package output keeps the pages directory equal to the transmit pages (ADR 0002).
package output

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"ceems/internal/store"
)

// VbitConf is the file the writer renders from the settings while vbit_conf is on.
const VbitConf = "vbit.conf"

// Marker is the empty file that says ceems created the directory.
const Marker = ".ceems-owned"

// Writer owns one pages directory. One goroutine runs Reconcile; Notify only enqueues.
type Writer struct {
	dir     string
	db      *store.Store
	log     *slog.Logger
	ch      chan struct{} // capacity 1; never closed
	quiet   time.Duration // wait after the last change before reconciling
	maxWait time.Duration // longest a burst of changes can delay the reconcile
}

// Open applies the ownership rule and returns a Writer, or an error naming dir. A missing
// directory is created; an empty one is claimed; one with the marker is reused; any other
// directory (any entry at all, hidden files and subdirectories included) is refused.
func Open(dir string, db *store.Store, log *slog.Logger) (*Writer, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("pagesDir %s: %w", dir, err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("pagesDir %s: %w", dir, err)
	}
	owned := false
	for _, e := range entries {
		if e.Name() == Marker {
			owned = true
		}
	}
	switch {
	case owned:
	case len(entries) == 0:
		if err := os.WriteFile(filepath.Join(dir, Marker), nil, 0o644); err != nil {
			return nil, fmt.Errorf("pagesDir %s: %w", dir, err)
		}
	default:
		return nil, fmt.Errorf("pagesDir %s holds %d entries but no %s; ceems only writes a directory it created. "+
			"Point pagesDir at an empty directory, or create %s there to confirm.", dir, len(entries), Marker, Marker)
	}
	return &Writer{dir: dir, db: db, log: log, ch: make(chan struct{}, 1),
		quiet: 500 * time.Millisecond, maxWait: 2 * time.Second}, nil
}

// Notify records that a page changed. It never blocks.
func (w *Writer) Notify() {
	select {
	case w.ch <- struct{}{}:
	default:
	}
}

// Run reconciles once, then reconciles after each burst of Notify calls, until ctx ends.
// A burst fires quiet after its last change, and at most maxWait after its first.
func (w *Writer) Run(ctx context.Context) {
	w.reconcileLogged()
	var deadline time.Time
	timer := time.NewTimer(0)
	<-timer.C
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-w.ch:
			now := time.Now()
			if deadline.IsZero() {
				deadline = now.Add(w.maxWait)
			}
			timer.Reset(min(w.quiet, deadline.Sub(now)))
		case <-timer.C:
			deadline = time.Time{}
			w.reconcileLogged()
		}
	}
}

func (w *Writer) reconcileLogged() {
	if err := w.Reconcile(); err != nil {
		w.log.Error("output", "dir", w.dir, "err", err)
	}
}

// Reconcile makes dir hold exactly the transmit pages as P<MPP>.tti, plus vbit.conf when the
// vbit_conf setting is on, the marker and any file that is not a .tti or a leftover temp file. It returns the first error and
// carries on past it; a failed run is retried by the next trigger.
func (w *Writer) Reconcile() error {
	pages, err := w.db.TransmitPages()
	if err != nil {
		return err
	}
	wanted := make(map[string][]byte, len(pages))
	for _, p := range pages {
		wanted[fmt.Sprintf("P%03X.tti", p.MPP)] = p.TTI
	}
	var first error
	remember := func(err error) {
		if err != nil && first == nil {
			first = err
		}
	}
	// vbit.conf rides the same compare-then-temp-and-rename path as a page. With vbit_conf off
	// it is not wanted and, being neither a .tti nor a temp file, is never touched (ADR 0002).
	if s, err := w.db.Settings(); err != nil {
		remember(err)
	} else if s.VbitConf {
		wanted[VbitConf] = renderVbitConf(s)
	}
	entries, err := os.ReadDir(w.dir)
	if err != nil {
		return err
	}
	for _, e := range entries {
		name := e.Name()
		if !e.Type().IsRegular() || name == Marker {
			continue
		}
		path := filepath.Join(w.dir, name)
		if want, ok := wanted[name]; ok {
			have, err := os.ReadFile(path)
			if err == nil && bytes.Equal(have, want) {
				delete(wanted, name)
			}
			remember(err)
		} else if strings.EqualFold(filepath.Ext(name), ".tti") || (strings.HasPrefix(name, ".") && strings.HasSuffix(name, ".tmp")) {
			remember(os.Remove(path))
		}
	}
	// shortcut: no fsync, so power loss can leave a stale file; the directory is rebuilt at start.
	for name, data := range wanted {
		tmp := filepath.Join(w.dir, "."+name+".tmp")
		if err := os.WriteFile(tmp, data, 0o644); err != nil {
			remember(err)
			continue
		}
		remember(os.Rename(tmp, filepath.Join(w.dir, name)))
	}
	return first
}

// renderVbitConf is the vbit.conf text for s. The header template is written in 7-bit
// form: a cell below 0x20 becomes ESC then the cell plus 0x40, every other cell is its byte.
func renderVbitConf(s store.Settings) []byte {
	var b bytes.Buffer
	b.WriteString("; written by CeeMS; edit vbit.conf.override for other keys; the override is read after this file and wins\n")
	b.WriteString("; vbit2 reads this file when it starts\n")
	b.WriteString("header_template=")
	for i := 0; i < len(s.HeaderTemplate); i++ {
		if c := s.HeaderTemplate[i]; c < 0x20 {
			b.Write([]byte{0x1B, c + 0x40})
		} else {
			b.WriteByte(c)
		}
	}
	fmt.Fprintf(&b, "\ninitial_teletext_page=%s\nrow_adaptive_mode=%t\nlines_per_field=%d\nmagazine_priority=%s\nstatus_display=%s\nnetwork_identification_code=%s\ncountry_network_identification=%s\ndatacast_lines=%d\nmagazine_serial=%t\n",
		s.InitialTeletextPage, s.RowAdaptiveMode, s.LinesPerField, s.MagazinePriority, s.StatusDisplay,
		s.NetworkIdentificationCode, s.CountryNetworkIdentification, s.DatacastLines, s.MagazineSerial)
	return b.Bytes()
}
