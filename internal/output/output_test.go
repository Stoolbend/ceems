package output

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"ceems/internal/store"
	"ceems/internal/tti"
)

var discard = slog.New(slog.NewTextHandler(io.Discard, nil))

// newStore opens a store in a temp dir holding the sample pages.
func newStore(t *testing.T) *store.Store {
	t.Helper()
	db, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	paths, err := filepath.Glob("../../testdata/sample/pages/*.tti")
	if err != nil || len(paths) == 0 {
		t.Fatalf("fixtures: %d files, err %v", len(paths), err)
	}
	var files []store.ImportFile
	for _, p := range paths {
		b, err := os.ReadFile(p)
		if err != nil {
			t.Fatal(err)
		}
		files = append(files, store.ImportFile{Path: p, TTI: b})
	}
	if _, err := db.ImportPages(files, "test"); err != nil {
		t.Fatal(err)
	}
	return db
}

// newWriter opens a Writer on a fresh empty directory.
func newWriter(t *testing.T, db *store.Store) *Writer {
	t.Helper()
	w, err := Open(filepath.Join(t.TempDir(), "pages"), db, discard)
	if err != nil {
		t.Fatal(err)
	}
	return w
}

func names(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, e := range entries {
		out = append(out, e.Name())
	}
	return out
}

func stored(t *testing.T, db *store.Store, mpp int) *tti.Page {
	t.Helper()
	p, err := db.Page(mpp)
	if err != nil {
		t.Fatal(err)
	}
	page, err := tti.Parse(p.TTI)
	if err != nil {
		t.Fatal(err)
	}
	return page
}

func TestOpenOwnership(t *testing.T) {
	t.Run("missing directory is created with the marker", func(t *testing.T) {
		dir := filepath.Join(t.TempDir(), "a", "b")
		if _, err := Open(dir, nil, discard); err != nil {
			t.Fatal(err)
		}
		if got := names(t, dir); !slices.Equal(got, []string{Marker}) {
			t.Fatalf("entries %v", got)
		}
	})
	t.Run("empty directory gets the marker", func(t *testing.T) {
		dir := t.TempDir()
		if _, err := Open(dir, nil, discard); err != nil {
			t.Fatal(err)
		}
		if got := names(t, dir); !slices.Equal(got, []string{Marker}) {
			t.Fatalf("entries %v", got)
		}
	})
	t.Run("marked directory with other files opens untouched", func(t *testing.T) {
		dir := t.TempDir()
		for _, n := range []string{Marker, "junk.txt"} {
			if err := os.WriteFile(filepath.Join(dir, n), nil, 0o644); err != nil {
				t.Fatal(err)
			}
		}
		if _, err := Open(dir, nil, discard); err != nil {
			t.Fatal(err)
		}
		if got := names(t, dir); !slices.Equal(got, []string{Marker, "junk.txt"}) {
			t.Fatalf("entries %v", got)
		}
	})
	t.Run("unmarked directory with a file is refused and left alone", func(t *testing.T) {
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, "P100.tti"), []byte("keep"), 0o644); err != nil {
			t.Fatal(err)
		}
		w, err := Open(dir, nil, discard)
		if err == nil || w != nil {
			t.Fatalf("Open = %v, %v; want a refusal", w, err)
		}
		if !strings.Contains(err.Error(), dir) || !strings.Contains(err.Error(), Marker) {
			t.Errorf("error %q must name the directory and the marker", err)
		}
		if got := names(t, dir); !slices.Equal(got, []string{"P100.tti"}) {
			t.Fatalf("entries %v", got)
		}
	})
	t.Run("a hidden file or subdirectory counts as an entry", func(t *testing.T) {
		dir := t.TempDir()
		if err := os.Mkdir(filepath.Join(dir, ".git"), 0o755); err != nil {
			t.Fatal(err)
		}
		if _, err := Open(dir, nil, discard); err == nil {
			t.Fatal("Open succeeded on a directory holding a subdirectory")
		}
	})
}

// leanStore holds one transmit page, P100, so a reconcile in a timing test is cheap.
func leanStore(t *testing.T) *store.Store {
	t.Helper()
	db, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	addPage(t, db, 0x100)
	return db
}

func TestReconcileWritesExactlyTheTransmitPages(t *testing.T) {
	db := newStore(t)
	p := stored(t, db, 0x101) // every fixture transmits; make one that does not
	p.SetTransmit(false)
	if _, err := db.SavePage(0x101, p, "", store.Audit{Actor: "t"}); err != nil {
		t.Fatal(err)
	}
	w := newWriter(t, db)
	if err := w.Reconcile(); err != nil {
		t.Fatal(err)
	}
	all, err := db.Pages()
	if err != nil {
		t.Fatal(err)
	}
	want := []string{Marker, VbitConf} // vbit_conf is on by default
	for _, p := range all {
		if !p.Transmit {
			continue
		}
		name := fmt.Sprintf("P%03X.tti", p.MPP)
		want = append(want, name)
		got, err := os.ReadFile(filepath.Join(w.dir, name))
		if err != nil {
			t.Fatal(err)
		}
		full, err := db.Page(p.MPP)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(got, full.TTI) {
			t.Errorf("%s differs from the stored page", name)
		}
	}
	if len(want) < 3 || len(want) == len(all)+2 {
		t.Fatalf("fixtures should mix transmit and non-transmit pages: %d wanted of %d", len(want)-2, len(all))
	}
	got := names(t, w.dir)
	slices.Sort(want)
	if !slices.Equal(got, want) {
		t.Fatalf("directory holds %v, want %v", got, want)
	}
}

func TestReconcileLeavesIdenticalFilesAlone(t *testing.T) { // mutation E
	db := newStore(t)
	w := newWriter(t, db)
	if err := w.Reconcile(); err != nil {
		t.Fatal(err)
	}
	old := time.Date(2001, 1, 1, 0, 0, 0, 0, time.UTC)
	files, _ := filepath.Glob(filepath.Join(w.dir, "P*.tti"))
	for _, f := range files {
		if err := os.Chtimes(f, old, old); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.Reconcile(); err != nil {
		t.Fatal(err)
	}
	for _, f := range files {
		fi, err := os.Stat(f)
		if err != nil {
			t.Fatal(err)
		}
		if !fi.ModTime().Equal(old) {
			t.Errorf("%s was rewritten although its bytes were identical", filepath.Base(f))
		}
	}
}

func TestReconcileDeleteRule(t *testing.T) { // mutation C
	db := newStore(t)
	w := newWriter(t, db)
	write := func(name, body string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(w.dir, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("P7FF.tti", "stray")
	write("p7fe.TTI", "stray")
	write(".P100.tti.tmp", "half")
	write("P100.tti", "wrong")
	write("notes.txt", "mine")
	if err := os.Mkdir(filepath.Join(w.dir, "sub.tti"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := w.Reconcile(); err != nil {
		t.Fatal(err)
	}
	got := names(t, w.dir)
	for _, gone := range []string{"P7FF.tti", "p7fe.TTI", ".P100.tti.tmp"} {
		if slices.Contains(got, gone) {
			t.Errorf("%s survived", gone)
		}
	}
	for _, kept := range []string{Marker, "notes.txt", "sub.tti"} {
		if !slices.Contains(got, kept) {
			t.Errorf("%s was deleted", kept)
		}
	}
	p, err := db.Page(0x100)
	if err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(filepath.Join(w.dir, "P100.tti")); !bytes.Equal(b, p.TTI) {
		t.Error("P100.tti was not corrected")
	}
}

func TestTTIChangesOnlyByRename(t *testing.T) { // mutation D
	db := newStore(t)
	w := newWriter(t, db)
	target := filepath.Join(w.dir, "P100.tti")
	if err := os.WriteFile(target, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	// A non-empty directory squats on the temp name, so the temp write must fail.
	tmp := filepath.Join(w.dir, ".P100.tti.tmp")
	if err := os.MkdirAll(filepath.Join(tmp, "x"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := w.Reconcile(); err == nil {
		t.Fatal("Reconcile succeeded although P100.tti's temp file could not be written")
	}
	if b, _ := os.ReadFile(target); string(b) != "old" {
		t.Errorf("P100.tti was written without going through its temp file: %q", b)
	}
}

func TestReconcileFollowsPageChanges(t *testing.T) {
	db := newStore(t)
	w := newWriter(t, db)
	if err := w.Reconcile(); err != nil {
		t.Fatal(err)
	}
	exists := func(name string) bool {
		_, err := os.Stat(filepath.Join(w.dir, name))
		return err == nil
	}
	if !exists("P100.tti") {
		t.Fatal("P100 is not a transmit page in the fixtures")
	}
	p := stored(t, db, 0x100)
	p.SetTransmit(false)
	if _, err := db.SavePage(0x100, p, "", store.Audit{Actor: "t"}); err != nil {
		t.Fatal(err)
	}
	w.Reconcile()
	if exists("P100.tti") {
		t.Error("transmit off left the file")
	}

	n := tti.NewPage(0x7A0)
	n.SetTransmit(true)
	if err := db.CreatePage(n, store.Audit{Actor: "t"}); err != nil {
		t.Fatal(err)
	}
	w.Reconcile()
	if !exists("P7A0.tti") {
		t.Fatal("created transmit page has no file")
	}

	if err := db.DeletePage(0x7A0, "", store.Audit{Actor: "t"}); err != nil {
		t.Fatal(err)
	}
	w.Reconcile()
	if exists("P7A0.tti") {
		t.Error("deleted page left its file")
	}
}

func TestStoreWritersNotifyOnce(t *testing.T) { // mutation F
	db := newStore(t)
	var n atomic.Int32
	db.OnPageChange(func() { n.Add(1) })
	a := store.Audit{Actor: "t"}
	expect := func(what string, want int32) {
		t.Helper()
		if got := n.Swap(0); got != want {
			t.Errorf("%s: hook ran %d times, want %d", what, got, want)
		}
	}

	p := stored(t, db, 0x100)
	if _, err := db.SavePage(0x100, p, "", a); err != nil {
		t.Fatal(err)
	}
	expect("SavePage with identical bytes", 0)
	p.SetTransmit(false)
	if changed, err := db.SavePage(0x100, p, "", a); err != nil || !changed {
		t.Fatalf("SavePage %v %v", changed, err)
	}
	expect("SavePage", 1)

	if err := db.CreatePage(tti.NewPage(0x7A0), a); err != nil {
		t.Fatal(err)
	}
	expect("CreatePage", 1)
	if err := db.CreatePage(tti.NewPage(0x7A0), a); err == nil {
		t.Fatal("duplicate CreatePage succeeded")
	}
	expect("failed CreatePage", 0)

	if err := db.DeletePage(0x7A0, "", a); err != nil {
		t.Fatal(err)
	}
	expect("DeletePage", 1)

	tid, err := db.CreateTemplate(&store.Template{Name: "t", TTI: tti.NewPage(0x7B0).Bytes(), RowRules: "{}"}, a)
	if err != nil {
		t.Fatal(err)
	}
	id, err := db.CreateBinding(&store.Binding{Name: "b", TemplateID: tid, MPP: 0x7B0, Source: "{}", Static: "{}"}, a)
	if err != nil {
		t.Fatal(err)
	}
	expect("CreateTemplate and CreateBinding", 0)
	g := tti.NewPage(0x7B0)
	g.SetTransmit(true)
	if changed, err := db.SaveGenerated(id, g, store.Audit{}); err != nil || !changed {
		t.Fatalf("SaveGenerated %v %v", changed, err)
	}
	expect("SaveGenerated", 1)
}

// waitFor polls cond every millisecond for up to limit and returns the time it took.
func waitFor(t *testing.T, limit time.Duration, cond func() bool) time.Duration {
	return waitEvery(t, time.Millisecond, limit, cond)
}

// waitEvery is waitFor with a poll interval. A slow poll that opens the file keeps the window
// short in which, on Windows, an open reader makes the writer's rename fail.
func waitEvery(t *testing.T, every, limit time.Duration, cond func() bool) time.Duration {
	t.Helper()
	start := time.Now()
	for !cond() {
		if time.Since(start) > limit {
			t.Fatalf("not satisfied within %v", limit)
		}
		time.Sleep(every)
	}
	return time.Since(start)
}

// fileHolds returns a poll that is true once path holds want. It records the file's Stat now, so
// call it before the save, and polls only Stat until the modification time or size differs; it
// opens the file just then, because on Windows an open reader can make the writer's rename fail.
func fileHolds(path string, want []byte) func() bool {
	before, _ := os.Stat(path)
	return func() bool {
		after, err := os.Stat(path)
		if err != nil || (before != nil && after.ModTime().Equal(before.ModTime()) && after.Size() == before.Size()) {
			return false
		}
		b, err := os.ReadFile(path)
		return err == nil && bytes.Equal(b, want)
	}
}

// runWriter starts w.Run and returns once the initial reconcile has produced P100.tti.
func runWriter(t *testing.T, w *Writer) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { defer close(done); w.Run(ctx) }()
	t.Cleanup(func() { cancel(); <-done })
	waitFor(t, 5*time.Second, func() bool {
		_, err := os.Stat(filepath.Join(w.dir, "P100.tti"))
		return err == nil
	})
}

// addPage stores a new transmit page without notifying anyone and returns its file name.
func addPage(t *testing.T, db *store.Store, mpp int) string {
	t.Helper()
	n := tti.NewPage(mpp)
	n.SetTransmit(true)
	if err := db.CreatePage(n, store.Audit{Actor: "t"}); err != nil {
		t.Fatal(err)
	}
	return fmt.Sprintf("P%03X.tti", mpp)
}

func TestDebounceCoalescesABurst(t *testing.T) { // mutation G
	db := leanStore(t)
	w := newWriter(t, db)
	w.quiet, w.maxWait = 30*time.Millisecond, 150*time.Millisecond
	runWriter(t, w)
	file := filepath.Join(w.dir, addPage(t, db, 0x7A0))

	start := time.Now()
	for range 10 {
		w.Notify()
		time.Sleep(2 * time.Millisecond)
	}
	waitFor(t, 2*time.Second, func() bool { _, err := os.Stat(file); return err == nil })
	elapsed := time.Since(start)
	t.Logf("reconciled %v after the first Notify", elapsed)
	if elapsed < 40*time.Millisecond {
		t.Errorf("reconciled after %v: the burst was not coalesced", elapsed)
	}
	if elapsed > 250*time.Millisecond {
		t.Errorf("reconciled after %v: too slow for quiet 30 ms", elapsed)
	}
}

func TestDebounceIsBoundedByMaxWait(t *testing.T) { // mutation G
	db := leanStore(t)
	w := newWriter(t, db)
	w.quiet, w.maxWait = 30*time.Millisecond, 150*time.Millisecond
	runWriter(t, w)
	file := filepath.Join(w.dir, addPage(t, db, 0x7A0))

	stop := make(chan struct{})
	finished := make(chan struct{})
	go func() {
		defer close(finished)
		for end := time.Now().Add(400 * time.Millisecond); time.Now().Before(end); {
			w.Notify()
			select {
			case <-stop:
				return
			case <-time.After(10 * time.Millisecond):
			}
		}
	}()
	start := time.Now()
	waitFor(t, 2*time.Second, func() bool { _, err := os.Stat(file); return err == nil })
	elapsed := time.Since(start)
	close(stop)
	<-finished
	t.Logf("reconciled %v into a continuous burst", elapsed)
	if elapsed > 250*time.Millisecond {
		t.Errorf("reconciled after %v: a continuous burst must not outlast maxWait 150 ms", elapsed)
	}
}

func TestEditReachesFileWithinAMinute(t *testing.T) {
	db := newStore(t)
	w := newWriter(t, db)
	db.OnPageChange(w.Notify)
	runWriter(t, w)

	p := stored(t, db, 0x100)
	if err := p.SetRow(1, 5, strings.Repeat("Z", 40)); err != nil {
		t.Fatal(err)
	}
	holds := fileHolds(filepath.Join(w.dir, "P100.tti"), p.Bytes()) // before the save
	t0 := time.Now()
	if changed, err := db.SavePage(0x100, p, "", store.Audit{Actor: "t"}); err != nil || !changed {
		t.Fatalf("SavePage %v %v", changed, err)
	}
	waitEvery(t, 10*time.Millisecond, time.Minute, holds)
	t.Logf("file updated %v after SavePage", time.Since(t0))
}

func TestReconcileFailureIsLoggedAndRetried(t *testing.T) {
	db := newStore(t)
	var logged bytes.Buffer
	w, err := Open(filepath.Join(t.TempDir(), "pages"), db, slog.New(slog.NewTextHandler(&logged, nil)))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(w.dir); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(w.dir, []byte("not a directory"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := w.Reconcile(); err == nil {
		t.Fatal("Reconcile succeeded with a file in place of the directory")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Run reconciles once, then returns
	w.Run(ctx)
	if !strings.Contains(logged.String(), "level=ERROR") || !strings.Contains(logged.String(), "output") {
		t.Errorf("failure not logged: %q", logged.String())
	}

	if err := os.Remove(w.dir); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(w.dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := w.Reconcile(); err != nil {
		t.Fatalf("retry: %v", err)
	}
	if _, err := os.Stat(filepath.Join(w.dir, "P100.tti")); err != nil {
		t.Errorf("retry wrote no pages: %v", err)
	}
}

// movableBinding creates a transmitting page 7B0 generated by a binding.
func movableBinding(t *testing.T, db *store.Store) *store.Binding {
	t.Helper()
	a := store.Audit{Actor: "t"}
	tid, err := db.CreateTemplate(&store.Template{Name: "t", TTI: tti.NewPage(0x7B0).Bytes(), RowRules: "{}"}, a)
	if err != nil {
		t.Fatal(err)
	}
	id, err := db.CreateBinding(&store.Binding{Name: "b", TemplateID: tid, MPP: 0x7B0, Source: "{}", Static: "{}"}, a)
	if err != nil {
		t.Fatal(err)
	}
	g := tti.NewPage(0x7B0)
	g.SetTransmit(true)
	if _, err := db.SaveGenerated(id, g, store.Audit{}); err != nil {
		t.Fatal(err)
	}
	b, err := db.Binding(id)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestBindingMoveNotifiesOnlyOnAMove(t *testing.T) { // mutation E
	db := newStore(t)
	b := movableBinding(t, db)
	var n atomic.Int32
	db.OnPageChange(func() { n.Add(1) })
	b.Name = "renamed"
	if err := db.UpdateBinding(b, store.Audit{Actor: "t"}); err != nil {
		t.Fatal(err)
	}
	if got := n.Swap(0); got != 0 {
		t.Errorf("field-only update ran the hook %d times", got)
	}
	b.MPP = 0x7B1
	if err := db.UpdateBinding(b, store.Audit{Actor: "t"}); err != nil {
		t.Fatal(err)
	}
	if got := n.Swap(0); got != 1 {
		t.Errorf("move ran the hook %d times, want 1", got)
	}
}

func TestBindingMoveRenamesTheFile(t *testing.T) { // mutation C
	db := newStore(t)
	b := movableBinding(t, db)
	w := newWriter(t, db)
	if err := w.Reconcile(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(w.dir, "P7B0.tti")); err != nil {
		t.Fatalf("no P7B0.tti before the move: %v", err)
	}
	b.MPP = 0x7B1
	if err := db.UpdateBinding(b, store.Audit{Actor: "t"}); err != nil {
		t.Fatal(err)
	}
	if err := w.Reconcile(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(w.dir, "P7B0.tti")); err == nil {
		t.Error("P7B0.tti survived the move")
	}
	got, err := os.ReadFile(filepath.Join(w.dir, "P7B1.tti"))
	if err != nil {
		t.Fatalf("no P7B1.tti after the move: %v", err)
	}
	if p, err := tti.Parse(got); err != nil || p.MPP() != 0x7B1 || !p.Transmit() {
		t.Errorf("P7B1.tti: mpp %03X transmit %v err %v", p.MPP(), p.Transmit(), err)
	}
}

func vbitPath(w *Writer) string { return filepath.Join(w.dir, VbitConf) }

func saveSettings(t *testing.T, db *store.Store, change func(*store.Settings)) {
	t.Helper()
	s, err := db.Settings()
	if err != nil {
		t.Fatal(err)
	}
	change(&s)
	if _, err := db.SaveSettings(s, store.Audit{Actor: "t"}); err != nil {
		t.Fatal(err)
	}
}

func TestVbitConfDefaultsMatchTheSample(t *testing.T) {
	db := leanStore(t)
	w := newWriter(t, db)
	if err := w.Reconcile(); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(vbitPath(w))
	if err != nil {
		t.Fatal(err)
	}
	sample, err := os.ReadFile("../../testdata/sample/vbit.conf")
	if err != nil {
		t.Fatal(err)
	}
	live := map[string]string{} // the sample's live lines by key
	for _, l := range strings.Split(strings.ReplaceAll(string(sample), "\r\n", "\n"), "\n") {
		if k, _, ok := strings.Cut(l, "="); ok && !strings.HasPrefix(l, ";") {
			live[k] = l
		}
	}
	want := []string{
		"; written by CeeMS; edit vbit.conf.override for other keys",
		"; vbit2 reads this file when it starts",
		live["header_template"],
		"initial_teletext_page=100",
		live["row_adaptive_mode"],
		"lines_per_field=16",
		live["magazine_priority"],
		live["status_display"],
		"",
	}
	if g := strings.Split(string(got), "\n"); !slices.Equal(g, want) {
		t.Fatalf("vbit.conf lines\n got %q\nwant %q", g, want)
	}
	if live["header_template"] == "" || live["row_adaptive_mode"] == "" || live["magazine_priority"] == "" || live["status_display"] == "" {
		t.Fatalf("the sample lacks a live line: %v", live)
	}
}

func TestVbitConfWritesControlCellsAsEscape(t *testing.T) {
	db := leanStore(t)
	w := newWriter(t, db)
	// Cell 0x07 is alpha white in a header; 0x0A and 0x0D must not reach the file raw.
	header := "\x07\x0a\x0d" + strings.Repeat("A", 29)
	saveSettings(t, db, func(s *store.Settings) { s.HeaderTemplate = header })
	if err := w.Reconcile(); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(vbitPath(w))
	if want := "header_template=\x1b\x47\x1b\x4a\x1b\x4d" + strings.Repeat("A", 29) + "\n"; !bytes.Contains(got, []byte(want)) {
		t.Fatalf("header not in 7-bit form: %q", got)
	}
	if n := bytes.Count(got, []byte{'\n'}); n != 8 || bytes.ContainsRune(got, '\r') {
		t.Fatalf("%d line feeds (want 8, one per line) or a carriage return in %q", n, got)
	}
}

func TestVbitConfNoTempLeftAndUnchangedNotRewritten(t *testing.T) {
	db := leanStore(t)
	w := newWriter(t, db)
	if err := w.Reconcile(); err != nil {
		t.Fatal(err)
	}
	if slices.Contains(names(t, w.dir), ".vbit.conf.tmp") {
		t.Fatal(".vbit.conf.tmp remains after a reconcile")
	}
	old := time.Date(2001, 1, 1, 0, 0, 0, 0, time.UTC)
	if err := os.Chtimes(vbitPath(w), old, old); err != nil {
		t.Fatal(err)
	}
	if err := w.Reconcile(); err != nil {
		t.Fatal(err)
	}
	if fi, _ := os.Stat(vbitPath(w)); !fi.ModTime().Equal(old) {
		t.Error("an unchanged reconcile rewrote vbit.conf")
	}
	saveSettings(t, db, func(s *store.Settings) { s.StatusDisplay = "NEWS" })
	if err := w.Reconcile(); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(vbitPath(w)); !bytes.Contains(b, []byte("status_display=NEWS\n")) {
		t.Errorf("a saved setting did not reach the file: %q", b)
	}
}

func TestVbitConfOffNeverTouchesTheFile(t *testing.T) {
	db := leanStore(t)
	saveSettings(t, db, func(s *store.Settings) { s.VbitConf = false })
	w := newWriter(t, db)
	if err := w.Reconcile(); err != nil {
		t.Fatal(err)
	}
	if slices.Contains(names(t, w.dir), VbitConf) {
		t.Error("vbit.conf was created with vbit_conf off")
	}
	// A foreign file, and the override, survive a save and a reconcile byte for byte.
	foreign, override := []byte("; mine\nstatus_display=HAND\n"), []byte("subtitle_repeats=2\n")
	if err := os.WriteFile(vbitPath(w), foreign, 0o644); err != nil {
		t.Fatal(err)
	}
	overridePath := filepath.Join(w.dir, "vbit.conf.override")
	if err := os.WriteFile(overridePath, override, 0o644); err != nil {
		t.Fatal(err)
	}
	saveSettings(t, db, func(s *store.Settings) { s.StatusDisplay = "NEWS" })
	if err := w.Reconcile(); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(vbitPath(w)); !bytes.Equal(b, foreign) {
		t.Errorf("vbit.conf was changed with the option off: %q", b)
	}
	if b, _ := os.ReadFile(overridePath); !bytes.Equal(b, override) {
		t.Errorf("vbit.conf.override was changed: %q", b)
	}
}

func TestVbitConfOnLeavesTheOverrideAlone(t *testing.T) {
	db := leanStore(t)
	w := newWriter(t, db)
	overridePath := filepath.Join(w.dir, "vbit.conf.override")
	if err := os.WriteFile(overridePath, []byte("subtitle_repeats=2\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := w.Reconcile(); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(overridePath); string(b) != "subtitle_repeats=2\n" {
		t.Errorf("vbit.conf.override was changed: %q", b)
	}
}

func TestVbitConfChangesOnlyByRename(t *testing.T) {
	db := leanStore(t)
	w := newWriter(t, db)
	if err := os.WriteFile(vbitPath(w), []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	// A non-empty directory squats on the temp name, so the temp write must fail.
	if err := os.MkdirAll(filepath.Join(w.dir, ".vbit.conf.tmp", "x"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := w.Reconcile(); err == nil {
		t.Error("Reconcile succeeded although vbit.conf's temp file could not be written")
	}
	if b, _ := os.ReadFile(vbitPath(w)); string(b) != "old" {
		t.Errorf("vbit.conf was written without going through its temp file: %q", b)
	}
}

func TestSettingsSaveRewritesTheFileViaNotify(t *testing.T) {
	db := leanStore(t)
	w := newWriter(t, db)
	db.OnPageChange(w.Notify)
	runWriter(t, w)
	waitFor(t, 5*time.Second, func() bool { _, err := os.Stat(vbitPath(w)); return err == nil })
	saveSettings(t, db, func(s *store.Settings) { s.StatusDisplay = "LIVE" })
	waitEvery(t, 20*time.Millisecond, 5*time.Second, func() bool {
		b, _ := os.ReadFile(vbitPath(w))
		return bytes.Contains(b, []byte("status_display=LIVE\n"))
	})
}
