package jobs

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

	"ceems/internal/output"
)

// A failed run writes nothing to the database, so the page's file must keep its bytes
// and its modification time. Mutation H.
func TestFailedRunLeavesFileUntouched(t *testing.T) {
	src := &funcSource{read: okRows}
	e := newRunEnv(t, src)
	e.runNow("schedule") // renders page 4A0 with PS,8000, so it transmits

	dir := filepath.Join(t.TempDir(), "pages")
	w, err := output.Open(dir, e.db, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	if err := w.Reconcile(); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(dir, "P4A0.tti")
	bytesBefore, err := os.ReadFile(file)
	if err != nil || !bytes.Equal(bytesBefore, e.stored().Bytes()) {
		t.Fatalf("P4A0.tti missing or wrong before the failed run: %v", err)
	}
	// Age the file so a rewrite is visible even at coarse timestamp resolution.
	old := time.Date(2001, 1, 1, 0, 0, 0, 0, time.UTC)
	if err := os.Chtimes(file, old, old); err != nil {
		t.Fatal(err)
	}

	e.at = e.at.Add(time.Hour) // a successful render now would differ
	src.read = func(context.Context, string, string) (Rows, error) {
		return Rows{}, fmt.Errorf("%w: HTTP 500: backend error", ErrUnreachable)
	}
	if run := e.runNow("schedule"); run.Outcome != "failed" {
		t.Fatalf("run %+v", run)
	}
	if err := w.Reconcile(); err != nil {
		t.Fatal(err)
	}
	bytesAfter, err := os.ReadFile(file)
	if err != nil || !bytes.Equal(bytesBefore, bytesAfter) {
		t.Errorf("the failed run changed P4A0.tti: %v", err)
	}
	if fi, err := os.Stat(file); err != nil || !fi.ModTime().Equal(old) {
		t.Errorf("the failed run rewrote P4A0.tti (modtime %v, err %v)", fi.ModTime(), err)
	}
}
