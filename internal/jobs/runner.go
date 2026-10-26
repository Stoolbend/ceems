package jobs

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"sync"
	"time"

	"ceems/internal/store"
	"ceems/internal/tmpl"
)

// runTimeout bounds one run, including the read of its sheet.
const runTimeout = 60 * time.Second

// Runner renders bindings into their pages: every binding at start and at every
// top of the hour, and one binding on demand. Runs never overlap.
type Runner struct {
	db  *store.Store
	src Source
	log *slog.Logger
	mu  sync.Mutex
	now func() time.Time // injected in tests
}

// New returns a runner reading from src.
func New(db *store.Store, src Source, log *slog.Logger) *Runner {
	return &Runner{db: db, src: src, log: log, now: time.Now}
}

// SheetsConfigured is false when src is Unconfigured.
func (r *Runner) SheetsConfigured() bool {
	_, off := r.src.(Unconfigured)
	return !off
}

// ErrBusy is returned by RunNow while another run or sweep holds the runner.
var ErrBusy = errors.New("a run is in progress")

// Start runs every binding once, then at every top of the hour on the wall clock,
// until ctx ends. Without a Google key it logs one line and returns (D-025).
func (r *Runner) Start(ctx context.Context) {
	if !r.SheetsConfigured() {
		r.log.Info("scheduler off", "reason", ErrNoKey.Error())
		return
	}
	for {
		r.sweep(ctx)
		wait := time.NewTimer(nextHour(r.now()).Sub(r.now()))
		select {
		case <-ctx.Done():
			wait.Stop()
			return
		case <-wait.C:
		}
	}
}

// nextHour is the next :00 after t on t's wall clock.
func nextHour(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), t.Hour()+1, 0, 0, 0, t.Location())
}

// sweep runs every binding in id order, holding mu throughout.
func (r *Runner) sweep(ctx context.Context) {
	r.mu.Lock()
	defer r.mu.Unlock()
	bs, err := r.db.Bindings()
	if err != nil {
		r.log.Error("listing bindings", "err", err)
		return
	}
	for i := range bs {
		if ctx.Err() != nil {
			return
		}
		r.run(ctx, &bs[i], "schedule")
	}
}

// RunNow runs one binding for actor; ErrBusy while a run is in progress,
// store.ErrNotFound for an unknown id. It runs without a Google key too, so the
// failure is recorded (D-025).
func (r *Runner) RunNow(ctx context.Context, bindingID int64, actor string) (store.Run, error) {
	if !r.mu.TryLock() {
		return store.Run{}, ErrBusy
	}
	defer r.mu.Unlock()
	b, err := r.db.Binding(bindingID)
	if err != nil {
		return store.Run{}, err
	}
	return r.run(ctx, b, actor), nil
}

func stamp(t time.Time) string { return t.UTC().Format(time.RFC3339) }

// run renders b, writes its page and records the run. A failed run writes no page.
func (r *Runner) run(ctx context.Context, b *store.Binding, actor string) store.Run {
	run := store.Run{BindingID: b.ID, Actor: actor, StartedAt: stamp(r.now()), Outcome: "ok"}
	err := r.render(ctx, b)
	run.FinishedAt = stamp(r.now())
	if err != nil {
		run.Outcome, run.Error = "failed", err.Error()
		r.log.Warn("run", "binding", b.Name, "outcome", run.Outcome, "error", run.Error)
	} else {
		r.log.Info("run", "binding", b.Name, "outcome", run.Outcome)
	}
	id, err := r.db.InsertRun(&run)
	if err != nil {
		r.log.Error("recording run", "binding", b.Name, "err", err)
	}
	run.ID = id
	return run
}

// render returns at the first error with nothing written. Every error it returns is
// safe to store and log: none carries the sheet id (D-021).
func (r *Runner) render(ctx context.Context, b *store.Binding) error {
	ctx, cancel := context.WithTimeout(ctx, runTimeout)
	defer cancel()
	st, err := r.db.Template(b.TemplateID)
	if err != nil {
		return fmt.Errorf("template %d: %w", b.TemplateID, err)
	}
	t, _, err := parseTemplate(st)
	if err != nil {
		return fmt.Errorf("template %s: %w", st.Name, err)
	}
	cfg, err := ParseSourceConfig(b.Source)
	if err != nil {
		return err
	}
	rows, err := r.src.Read(ctx, cfg.Sheet, cfg.Range)
	if err != nil {
		return err
	}
	widths := map[string]int{}
	for _, sl := range t.Slots() {
		if w, ok := widths[sl.Name]; !ok || sl.Width < w {
			widths[sl.Name] = sl.Width
		}
	}
	data, err := Values(rows, cfg, widths)
	if err != nil {
		return err
	}
	data["updated"] = r.now().Format("02 Jan 15:04") // host-local time (D-020)
	static := map[string]string{}
	if err := json.Unmarshal([]byte(b.Static), &static); err != nil {
		return fmt.Errorf("static: %w", err)
	}
	page, err := t.Render(b.MPP, static, data)
	if err != nil {
		return err
	}
	page.SetField(-1, "DE", b.Name)
	_, err = r.db.SaveGenerated(b.ID, page, store.Audit{})
	return err
}

// DataKeys is every slot name a source fills: each column with '#' replaced by
// 1..cfg.Rows, plus "updated". The web form's dataKeys mirrors it.
func DataKeys(cfg SourceConfig) map[string]bool {
	keys := map[string]bool{"updated": true}
	for i := 1; i <= cfg.Rows; i++ {
		for _, col := range cfg.Columns {
			keys[strings.ReplaceAll(col, "#", strconv.Itoa(i))] = true
		}
	}
	return keys
}

// parseTemplate parses a stored template; a copy of the httpapi helper, which jobs
// may not import (ADR 0005).
func parseTemplate(t *store.Template) (*tmpl.Template, map[string]string, error) {
	rules := map[string]string{}
	if err := json.Unmarshal([]byte(t.RowRules), &rules); err != nil {
		return nil, nil, err
	}
	p, err := tmpl.Parse(t.TTI, rules)
	return p, rules, err
}
