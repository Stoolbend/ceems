package jobs

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"ceems/internal/store"
	"ceems/internal/tti"
)

// funcSource is a Source whose Read is a test closure.
type funcSource struct {
	read func(ctx context.Context, sheet, rng string) (Rows, error)
}

func (s *funcSource) Read(ctx context.Context, sheet, rng string) (Rows, error) {
	return s.read(ctx, sheet, rng)
}

// okRows answers every read with one complete entry.
func okRows(context.Context, string, string) (Rows, error) {
	return Rows{Formatted: [][]string{{"ALICE", "120"}}, Raw: [][]any{{"ALICE", 120.0}}}, nil
}

// scoresTTI is a two-subpage template on page 100 with a fixed slot (title), two
// data slots and the run time.
func scoresTTI() []byte {
	var b strings.Builder
	b.WriteString("DE,template\r\n")
	for sub := 1; sub <= 2; sub++ {
		fmt.Fprintf(&b, "PN,100%02d\r\nSC,%04d\r\nPS,8000\r\nCT,8,T\r\n", sub, sub)
		b.WriteString("OL,1,{title..........}\r\n")
		b.WriteString("OL,3,Name {p1name.} Score {>p1score}\r\n")
		b.WriteString("OL,23,Updated {updated.....}\r\n")
	}
	return []byte(b.String())
}

const testSource = `{"sheet":"` + fakeSheetID + `","range":"C5:D","columns":["p#name","p#score"],"rows":1}`

type runEnv struct {
	t   *testing.T
	dir string
	db  *store.Store
	raw *sql.DB
	log *bytes.Buffer
	r   *Runner
	id  int64 // the binding on page 4A0
	at  time.Time
}

// newRunEnv makes a store with one template and one binding "pacman" on 4A0, and a
// runner over src whose clock reads e.at.
func newRunEnv(t *testing.T, src Source) *runEnv {
	t.Helper()
	dir := t.TempDir()
	db, err := store.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	raw, err := sql.Open("sqlite", filepath.Join(dir, "ceems.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { raw.Close() })
	a := store.Audit{Actor: "user:1"}
	tid, err := db.CreateTemplate(&store.Template{Name: "scores", TTI: scoresTTI(), RowRules: "{}"}, a)
	if err != nil {
		t.Fatal(err)
	}
	id, err := db.CreateBinding(&store.Binding{Name: "pacman", TemplateID: tid, MPP: 0x4A0, Source: testSource, Static: `{"title":"\u0001Scores"}`}, a)
	if err != nil {
		t.Fatal(err)
	}
	e := &runEnv{t: t, dir: dir, db: db, raw: raw, log: &bytes.Buffer{}, id: id,
		at: time.Date(2026, 10, 12, 14, 5, 0, 0, time.Local)}
	e.r = New(db, src, slog.New(slog.NewTextHandler(e.log, nil)))
	e.r.now = func() time.Time { return e.at }
	return e
}

func (e *runEnv) count(query string, args ...any) int {
	e.t.Helper()
	var n int
	if err := e.raw.QueryRow(query, args...).Scan(&n); err != nil {
		e.t.Fatal(err)
	}
	return n
}

// snapshot is everything a failed run must leave unchanged.
type snapshot struct {
	tti                    []byte
	updatedAt              string
	audit, versions, pages int
}

func (e *runEnv) snap() snapshot {
	e.t.Helper()
	var s snapshot
	if err := e.raw.QueryRow("SELECT tti, updated_at FROM pages WHERE mpp = ?", 0x4A0).Scan(&s.tti, &s.updatedAt); err != nil {
		e.t.Fatal(err)
	}
	s.audit = e.count("SELECT count(*) FROM audit")
	s.versions = e.count("SELECT count(*) FROM page_versions")
	s.pages = e.count("SELECT count(*) FROM pages")
	return s
}

func (e *runEnv) runNow(actor string) store.Run {
	e.t.Helper()
	run, err := e.r.RunNow(e.t.Context(), e.id, actor)
	if err != nil {
		e.t.Fatal(err)
	}
	return run
}

func (e *runEnv) stored() *tti.Page {
	e.t.Helper()
	p, err := e.db.Page(0x4A0)
	if err != nil {
		e.t.Fatal(err)
	}
	page, err := tti.Parse(p.TTI)
	if err != nil {
		e.t.Fatal(err)
	}
	return page
}

func TestRunWritesPage(t *testing.T) {
	e := newRunEnv(t, &funcSource{read: okRows})
	run := e.runNow("schedule")
	if run.Outcome != "ok" || run.Error != "" || run.ID == 0 || run.Actor != "schedule" || run.BindingID != e.id ||
		run.StartedAt != stamp(e.at) || run.FinishedAt == "" {
		t.Fatalf("run %+v", run)
	}
	p := e.stored()
	if p.Subpages() != 2 {
		t.Fatalf("subpages %d", p.Subpages())
	}
	for sub := 0; sub < 2; sub++ {
		if pn, _ := p.Field(sub, "PN"); pn != fmt.Sprintf("4A0%02d", sub+1) {
			t.Errorf("subpage %d PN %q", sub+1, pn)
		}
		if row, _ := p.Row(sub, 3); !strings.HasPrefix(row, "Name ALICE     Score        120") {
			t.Errorf("subpage %d row 3 %q", sub+1, row)
		}
		if row, _ := p.Row(sub, 1); !strings.HasPrefix(row, "\x01Scores ") {
			t.Errorf("subpage %d row 1 %q", sub+1, row)
		}
		if row, _ := p.Row(sub, 23); !strings.HasPrefix(row, "Updated 12 Oct 14:05 ") {
			t.Errorf("subpage %d row 23 %q", sub+1, row)
		}
	}
	if de, _ := p.Field(-1, "DE"); de != "pacman" {
		t.Errorf("DE %q", de)
	}
	if n := e.count("SELECT count(*) FROM audit WHERE action = 'page.generate' AND actor = ? AND target = 'page:4A0'", fmt.Sprintf("job:binding:%d", e.id)); n != 1 {
		t.Errorf("page.generate audit rows %d", n)
	}
	if n := e.count("SELECT count(*) FROM runs WHERE binding_id = ? AND outcome = 'ok' AND error = '' AND actor = 'schedule'", e.id); n != 1 {
		t.Errorf("ok run rows %d", n)
	}
}

func TestRunUsesBindingMPP(t *testing.T) { // mutation D
	e := newRunEnv(t, &funcSource{read: okRows})
	if run := e.runNow("schedule"); run.Outcome != "ok" {
		t.Fatalf("run %+v", run)
	}
	if p := e.stored(); p.MPP() != 0x4A0 {
		t.Fatalf("stored page carries %03X, want the binding's 4A0", p.MPP())
	}
	if n := e.count("SELECT count(*) FROM pages WHERE mpp = ?", 0x100); n != 0 {
		t.Error("the run wrote the template's own page 100")
	}
}

func TestRunNowAuditsAsJob(t *testing.T) { // mutation F
	e := newRunEnv(t, &funcSource{read: okRows})
	e.runNow("user:7")
	var actor string
	if err := e.raw.QueryRow("SELECT actor FROM audit WHERE action = 'page.generate'").Scan(&actor); err != nil {
		t.Fatal(err)
	}
	if want := fmt.Sprintf("job:binding:%d", e.id); actor != want {
		t.Errorf("audit actor %q want %q", actor, want)
	}
	if n := e.count("SELECT count(*) FROM runs WHERE actor = 'user:7'"); n != 1 {
		t.Errorf("runs with actor user:7: %d", n)
	}
}

func TestFailedRunKeepsPage(t *testing.T) { // mutation A
	src := &funcSource{read: okRows}
	e := newRunEnv(t, src)
	e.runNow("schedule")
	before := e.snap()
	e.at = e.at.Add(time.Hour) // a successful render now would differ
	src.read = func(context.Context, string, string) (Rows, error) {
		return Rows{}, fmt.Errorf("%w: HTTP 500: backend error", ErrUnreachable)
	}
	run := e.runNow("schedule")
	if run.Outcome != "failed" || run.Error != "sheet unreachable: HTTP 500: backend error" {
		t.Fatalf("run %+v", run)
	}
	if after := e.snap(); !reflect.DeepEqual(before, after) {
		t.Errorf("a failed fetch changed the page: before %d bytes %s audit %d versions %d, after %d bytes %s audit %d versions %d",
			len(before.tti), before.updatedAt, before.audit, before.versions, len(after.tti), after.updatedAt, after.audit, after.versions)
	}
	if n := e.count("SELECT count(*) FROM runs WHERE outcome = 'failed' AND error = ?", run.Error); n != 1 {
		t.Errorf("failed run rows %d", n)
	}
}

func TestRenderErrorWritesNothing(t *testing.T) { // mutation B
	e := newRunEnv(t, &funcSource{read: okRows})
	e.runNow("schedule")
	before := e.snap()
	e.at = e.at.Add(time.Hour)
	if _, err := e.raw.Exec("UPDATE bindings SET static = '{}' WHERE id = ?", e.id); err != nil {
		t.Fatal(err)
	}
	run := e.runNow("schedule")
	if run.Outcome != "failed" || !strings.Contains(run.Error, `"title"`) {
		t.Fatalf("run %+v", run)
	}
	if after := e.snap(); !reflect.DeepEqual(before, after) {
		t.Errorf("a render error changed the page: audit %d->%d versions %d->%d updated %s->%s",
			before.audit, after.audit, before.versions, after.versions, before.updatedAt, after.updatedAt)
	}
}

func TestRunNowUnconfiguredRecordsFailure(t *testing.T) { // mutation J
	e := newRunEnv(t, Unconfigured{})
	before := e.snap()
	run, err := e.r.RunNow(t.Context(), e.id, "user:1")
	if err != nil {
		t.Fatal(err)
	}
	if run.Outcome != "failed" || run.Error != "googleKeyFile is not configured" {
		t.Fatalf("run %+v", run)
	}
	if n := e.count("SELECT count(*) FROM runs WHERE outcome = 'failed' AND error = 'googleKeyFile is not configured'"); n != 1 {
		t.Errorf("failed run rows %d, want 1", n)
	}
	if after := e.snap(); !reflect.DeepEqual(before, after) {
		t.Error("an unconfigured run changed the page")
	}
}

func TestRunNowUnknownBinding(t *testing.T) {
	e := newRunEnv(t, &funcSource{read: okRows})
	if _, err := e.r.RunNow(t.Context(), 99, "user:1"); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("got %v", err)
	}
	if n := e.count("SELECT count(*) FROM runs"); n != 0 {
		t.Errorf("runs %d", n)
	}
}

func TestRunPassesDeadlineToRead(t *testing.T) {
	var left time.Duration
	e := newRunEnv(t, &funcSource{read: func(ctx context.Context, sheet, rng string) (Rows, error) {
		d, ok := ctx.Deadline()
		if !ok {
			t.Error("Read got no deadline")
		}
		left = time.Until(d)
		if sheet != fakeSheetID || rng != "C5:D" {
			t.Errorf("Read(%d-byte sheet, %q)", len(sheet), rng)
		}
		return okRows(ctx, sheet, rng)
	}})
	e.runNow("schedule")
	if left <= 55*time.Second || left > 60*time.Second {
		t.Errorf("deadline %v away, want about 60s", left)
	}
}

func TestDataKeys(t *testing.T) {
	cases := []struct {
		cfg  SourceConfig
		want []string
	}{
		// The same examples as web/src/BindingsView.test.ts.
		{SourceConfig{Columns: []string{"p#name", "p#score"}, Rows: 2}, []string{"p1name", "p2name", "p1score", "p2score", "updated"}},
		{SourceConfig{Columns: []string{"p#name"}, Rows: 1}, []string{"p1name", "updated"}},
	}
	for _, c := range cases {
		want := map[string]bool{}
		for _, k := range c.want {
			want[k] = true
		}
		if got := DataKeys(c.cfg); !reflect.DeepEqual(got, want) {
			t.Errorf("DataKeys(%v) = %v want %v", c.cfg.Columns, got, want)
		}
	}
}

func TestNextHour(t *testing.T) {
	loc := time.FixedZone("plus0530", 5*3600+1800) // a zone whose hours are not UTC hours
	cases := [][2]time.Time{
		{time.Date(2026, 10, 12, 14, 5, 7, 9, loc), time.Date(2026, 10, 12, 15, 0, 0, 0, loc)},
		{time.Date(2026, 10, 12, 14, 0, 0, 0, loc), time.Date(2026, 10, 12, 15, 0, 0, 0, loc)},
		{time.Date(2026, 12, 31, 23, 59, 59, 0, loc), time.Date(2027, 1, 1, 0, 0, 0, 0, loc)},
	}
	for _, c := range cases {
		if got := nextHour(c[0]); !got.Equal(c[1]) {
			t.Errorf("nextHour(%v) = %v want %v", c[0], got, c[1])
		}
	}
}

// startAndStop runs Start until cond holds or a second passes, then cancels it and
// waits for it. It reports whether Start had returned on its own.
func startAndStop(t *testing.T, r *Runner, cond func() bool) (returned bool) {
	t.Helper()
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan struct{})
	go func() { r.Start(ctx); close(done) }()
	defer func() { cancel(); <-done }()
	for deadline := time.Now().Add(time.Second); time.Now().Before(deadline) && !cond(); {
		select {
		case <-done:
			return true
		case <-time.After(10 * time.Millisecond):
		}
	}
	select {
	case <-done:
		return true
	default:
		return false
	}
}

func TestStartRunsEveryBindingOnce(t *testing.T) {
	e := newRunEnv(t, &funcSource{read: okRows})
	if _, err := e.db.CreateBinding(&store.Binding{Name: "galaxian", TemplateID: 1, MPP: 0x4A1, Source: testSource, Static: `{"title":"G"}`}, store.Audit{Actor: "user:1"}); err != nil {
		t.Fatal(err)
	}
	startAndStop(t, e.r, func() bool { return e.count("SELECT count(*) FROM runs") >= 2 })
	if n := e.count("SELECT count(*) FROM runs WHERE outcome = 'ok' AND actor = 'schedule'"); n != 2 {
		t.Fatalf("ok scheduled runs %d, want one per binding", n)
	}
	if n := e.count("SELECT count(DISTINCT binding_id) FROM runs"); n != 2 {
		t.Errorf("bindings run %d", n)
	}
	if n := e.count("SELECT count(*) FROM audit WHERE action = 'page.generate'"); n != 2 {
		t.Errorf("page.generate rows %d", n)
	}
}

func TestStartUnconfiguredRunsNothing(t *testing.T) { // mutation I
	e := newRunEnv(t, Unconfigured{})
	audit := e.count("SELECT count(*) FROM audit")
	if !startAndStop(t, e.r, func() bool { return false }) {
		t.Error("Start without a key did not return on its own")
	}
	if n := e.count("SELECT count(*) FROM runs"); n != 0 {
		t.Errorf("run rows %d, want 0", n)
	}
	if n := e.count("SELECT count(*) FROM audit"); n != audit {
		t.Errorf("audit rows %d -> %d, want unchanged", audit, n)
	}
	if !strings.Contains(e.log.String(), "scheduler off") {
		t.Errorf("log %q", e.log.String())
	}
}

// TestRunNothingLeaks runs a binding through the real Sheets client against the fake,
// once successfully and once with an error body quoting the sheet id, and checks the
// sheet id, key and client email reach no log line, audit row, run row, or database
// byte outside bindings.source, where the admin form keeps it.
func TestRunNothingLeaks(t *testing.T) { // mutation L (with httpapi TestBindingListHidesSource)
	f := newFake(t)
	e := newRunEnv(t, f.client(t))
	f.valuesFn.Store(jsonValues(`{"values":[["ALICE","120"]]}`))
	if run := e.runNow("schedule"); run.Outcome != "ok" {
		t.Fatalf("ok run %+v", run)
	}
	f.valuesFn.Store(valuesHandler(func(w http.ResponseWriter, _ *http.Request, _ bool) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(500)
		fmt.Fprintf(w, `{"error":{"code":500,"message":"failed on %s"}}`, fakeSheetID)
	}))
	e.at = e.at.Add(time.Hour)
	if run := e.runNow("schedule"); run.Outcome != "failed" || !strings.Contains(run.Error, "<sheet>") {
		t.Fatalf("failed run %+v", run)
	}

	needles := []string{fakeSheetID, fakeEmail, f.pemBody, "PRIVATE KEY"}
	check := func(where string, hay []byte) {
		t.Helper()
		for _, n := range needles {
			if bytes.Contains(hay, []byte(n)) {
				t.Errorf("%s contains a secret of length %d", where, len(n))
			}
		}
	}
	check("log", e.log.Bytes())
	for _, q := range []string{
		"SELECT at || actor || action || target || detail || remote FROM audit",
		"SELECT actor || started_at || finished_at || outcome || error FROM runs",
	} {
		rows, err := e.raw.Query(q)
		if err != nil {
			t.Fatal(err)
		}
		n := 0
		for rows.Next() {
			var s string
			if err := rows.Scan(&s); err != nil {
				t.Fatal(err)
			}
			n++
			check(q, []byte(s))
		}
		rows.Close()
		if n == 0 {
			t.Errorf("%s: no rows", q)
		}
	}

	// Drop the one legitimate copy, rebuild the file without free pages, and read every byte.
	if _, err := e.raw.Exec("UPDATE bindings SET source = '{}'"); err != nil {
		t.Fatal(err)
	}
	if _, err := e.raw.Exec("VACUUM"); err != nil {
		t.Fatal(err)
	}
	if _, err := e.raw.Exec("PRAGMA wal_checkpoint(TRUNCATE)"); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"ceems.db", "ceems.db-wal"} {
		if b, err := os.ReadFile(filepath.Join(e.dir, name)); err == nil {
			check(name+" bytes", b)
		} else if name == "ceems.db" {
			t.Fatal(err)
		}
	}
}
