package jobs

import (
	"bytes"
	"database/sql"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"ceems/internal/store"

	_ "modernc.org/sqlite"
)

// TestNothingLeaks runs every error path of the fake end to end and checks that
// the key, the client email and the sheet id reach no log, audit row or database byte.
func TestNothingLeaks(t *testing.T) {
	f := newFake(t)
	var logBuf bytes.Buffer
	log := slog.New(slog.NewTextHandler(&logBuf, nil))
	dir := t.TempDir()
	db, err := store.Open(dir)
	if err != nil {
		t.Fatal(err)
	}

	var texts []string
	// record logs the error and stores it as an audit detail, the way the scheduler will.
	record := func(err error) {
		if err == nil {
			t.Fatal("want error")
		}
		texts = append(texts, err.Error())
		log.Error("read failed", "binding", "demo", "error", err)
		_, cerr := db.CreateUser(fmt.Sprintf("u%d", len(texts)), []byte("h"), false,
			store.Audit{Actor: "job", Action: "read", Target: "demo", Detail: err.Error()})
		if cerr != nil {
			t.Fatal(cerr)
		}
	}

	f.valuesFn.Store(valuesHandler(func(w http.ResponseWriter, _ *http.Request, _ bool) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(500)
		fmt.Fprintf(w, `{"error":{"code":500,"message":"failed on %s"}}`, fakeSheetID)
	}))
	_, err = f.client(t).Read(t.Context(), fakeSheetID, "A1:B")
	record(err)

	f.valuesFn.Store(valuesHandler(func(w http.ResponseWriter, _ *http.Request, _ bool) { http.Error(w, fakeSheetID, 404) }))
	_, err = f.client(t).Read(t.Context(), fakeSheetID, "A1:B")
	record(err)

	f.tokenMode.Store("echo400")
	_, err = f.client(t).Read(t.Context(), fakeSheetID, "A1:B")
	record(err)

	_, err = NewSheets(t.Context(), filepath.Join(t.TempDir(), "missing.json"))
	record(err)

	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	needles := []string{fakeSheetID, fakeEmail, f.pemBody, "PRIVATE KEY"}
	check := func(where string, hay []byte) {
		for _, n := range needles {
			if bytes.Contains(hay, []byte(n)) {
				t.Errorf("%s contains a secret of length %d", where, len(n))
			}
		}
	}
	check("log buffer", logBuf.Bytes())
	check("error texts", []byte(strings.Join(texts, "\n")))

	raw, err := os.ReadFile(filepath.Join(dir, "ceems.db"))
	if err != nil {
		t.Fatal(err)
	}
	check("ceems.db bytes", raw)
	if wal, err := os.ReadFile(filepath.Join(dir, "ceems.db-wal")); err == nil {
		check("ceems.db-wal bytes", wal)
	}

	sdb, err := sql.Open("sqlite", filepath.Join(dir, "ceems.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer sdb.Close()
	rows, err := sdb.Query("SELECT detail FROM audit WHERE action = 'read'")
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	n := 0
	for rows.Next() {
		var d string
		if err := rows.Scan(&d); err != nil {
			t.Fatal(err)
		}
		n++
		check("audit detail", []byte(d))
	}
	if n != len(texts) {
		t.Fatalf("audit rows %d want %d", n, len(texts))
	}
}
