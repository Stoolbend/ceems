package store

import (
	"bytes"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"ceems/internal/tti"
)

func openRaw(t *testing.T, dir string) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", filepath.Join(dir, "ceems.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func pragma(t *testing.T, db *sql.DB, name string) string {
	t.Helper()
	var v string
	if err := db.QueryRow("PRAGMA " + name).Scan(&v); err != nil {
		t.Fatal(err)
	}
	return v
}

func backups(t *testing.T, dir string) []string {
	t.Helper()
	m, _ := filepath.Glob(filepath.Join(dir, "ceems-pre-*.db"))
	return m
}

func TestOpenCreatesAndReopensWithoutMigrating(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	if v := pragma(t, s.db, "user_version"); v != "10" {
		t.Fatalf("user_version %s", v)
	}
	if v := pragma(t, s.db, "journal_mode"); v != "wal" {
		t.Fatalf("journal_mode %s", v)
	}
	if v := pragma(t, s.db, "foreign_keys"); v != "1" {
		t.Fatalf("foreign_keys %s", v)
	}
	s.Close()

	s, err = Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if v := pragma(t, s.db, "user_version"); v != "10" {
		t.Fatalf("user_version %s", v)
	}
	if b := backups(t, dir); len(b) != 0 {
		t.Fatalf("unexpected backups %v", b)
	}
}

func TestSecondMigrationBacksUpAndApplies(t *testing.T) {
	dir := t.TempDir()
	db := openRaw(t, dir)
	if _, err := db.Exec("PRAGMA user_version = 1"); err != nil {
		t.Fatal(err)
	}
	fsys := fstest.MapFS{
		"0001_baseline.sql": {Data: []byte("-- x")},
		"0002_t.sql":        {Data: []byte("CREATE TABLE t (id INTEGER)")},
	}
	if err := migrate(db, fsys, dir); err != nil {
		t.Fatal(err)
	}
	if v := pragma(t, db, "user_version"); v != "2" {
		t.Fatalf("user_version %s", v)
	}
	if _, err := os.Stat(filepath.Join(dir, "ceems-pre-v2.db")); err != nil {
		t.Fatal(err)
	}
	if err := migrate(db, fsys, dir); err != nil || len(backups(t, dir)) != 1 {
		t.Fatalf("rerun err=%v backups=%v", err, backups(t, dir))
	}
}

func TestBadMigrationNames(t *testing.T) {
	cases := map[string]fstest.MapFS{
		"gap":       {"0001_a.sql": {Data: []byte("")}, "0003_c.sql": {Data: []byte("")}},
		"duplicate": {"0001_a.sql": {Data: []byte("")}, "0001_b.sql": {Data: []byte("")}},
		"bad name":  {"first.sql": {Data: []byte("")}},
	}
	for name, fsys := range cases {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			err := migrate(openRaw(t, dir), fsys, dir)
			if err == nil || !strings.Contains(err.Error(), ".sql") {
				t.Fatalf("got %v", err)
			}
		})
	}
}

func TestDatabaseNewerThanMigrationsRefused(t *testing.T) {
	dir := t.TempDir()
	db := openRaw(t, dir)
	if _, err := db.Exec("PRAGMA user_version = 5"); err != nil {
		t.Fatal(err)
	}
	err := migrate(db, fstest.MapFS{"0001_a.sql": {Data: []byte("")}}, dir)
	if err == nil || !strings.Contains(err.Error(), "5") || !strings.Contains(err.Error(), "1") {
		t.Fatalf("got %v", err)
	}
}

func TestFailingMigrationLeavesVersionUnchanged(t *testing.T) {
	dir := t.TempDir()
	db := openRaw(t, dir)
	fsys := fstest.MapFS{
		"0001_a.sql": {Data: []byte("CREATE TABLE a (id INTEGER)")},
		"0002_b.sql": {Data: []byte("CREATE TABLE b (id INTEGER); THIS IS NOT SQL")},
	}
	err := migrate(db, fsys, dir)
	if err == nil || !strings.Contains(err.Error(), "0002_b.sql") {
		t.Fatalf("got %v", err)
	}
	if v := pragma(t, db, "user_version"); v != "1" {
		t.Fatalf("user_version %s", v)
	}
	if _, err := db.Exec("SELECT * FROM b"); err == nil {
		t.Fatal("table b should have been rolled back")
	}
}

func fixtures(t *testing.T) []ImportFile {
	t.Helper()
	paths, err := filepath.Glob("../../testdata/sample/pages/*.tti")
	if err != nil || len(paths) != 30 {
		t.Fatalf("fixtures: %d files, err %v", len(paths), err)
	}
	var files []ImportFile
	for _, p := range paths {
		b, err := os.ReadFile(p)
		if err != nil {
			t.Fatal(err)
		}
		files = append(files, ImportFile{Path: p, TTI: b})
	}
	return files
}

func fixture(t *testing.T, name string) ImportFile {
	t.Helper()
	b, err := os.ReadFile("../../testdata/sample/pages/" + name)
	if err != nil {
		t.Fatal(err)
	}
	return ImportFile{Path: name, TTI: b}
}

func openTemp(t *testing.T) *Store {
	t.Helper()
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func scalar(t *testing.T, s *Store, query string) int {
	t.Helper()
	var n int
	if err := s.db.QueryRow(query).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestImportFixtures(t *testing.T) {
	s := openTemp(t)
	files := fixtures(t)
	if n, err := s.ImportPages(files, "cli"); err != nil || n != 30 {
		t.Fatalf("added %d err %v", n, err)
	}
	if n := scalar(t, s, "SELECT count(*) FROM pages"); n != 30 {
		t.Fatalf("count %d", n)
	}
	for _, f := range files {
		var mpp int
		name := strings.TrimSuffix(filepath.Base(f.Path), ".tti")
		if _, err := fmt.Sscanf(name, "P%X", &mpp); err != nil {
			t.Fatal(err)
		}
		p, err := s.Page(mpp)
		if err != nil {
			t.Fatalf("%s: %v", f.Path, err)
		}
		want := 1
		switch mpp {
		case 0x100:
			want = 7
		case 0x101:
			want = 5
		case 0x120:
			want = 2
		}
		if !bytes.Equal(p.TTI, f.TTI) || p.Subpages != want || !p.Transmit || p.UpdatedBy != "cli" {
			t.Errorf("%s: subpages %d transmit %v by %q equal %v", name, p.Subpages, p.Transmit, p.UpdatedBy, bytes.Equal(p.TTI, f.TTI))
		}
		if _, err := time.Parse(time.RFC3339, p.UpdatedAt); err != nil || !strings.HasSuffix(p.UpdatedAt, "Z") {
			t.Errorf("%s: updated_at %q", name, p.UpdatedAt)
		}
	}
	if p, _ := s.Page(0x810); p.Description != "" {
		t.Errorf("P810 description %q", p.Description)
	}
	if p, _ := s.Page(0x121); p.Description != "Weather" {
		t.Errorf("P121 description %q", p.Description)
	}
}

func TestSecondImportChangesNothing(t *testing.T) {
	s := openTemp(t)
	files := fixtures(t)
	if _, err := s.ImportPages(files, "cli"); err != nil {
		t.Fatal(err)
	}
	before := scalar(t, s, "SELECT total_changes()")
	if n, err := s.ImportPages(files, "someone-else"); err != nil || n != 0 {
		t.Fatalf("added %d err %v", n, err)
	}
	if after := scalar(t, s, "SELECT total_changes()"); after != before {
		t.Fatalf("total_changes %d -> %d", before, after)
	}
	if p, _ := s.Page(0x100); p.UpdatedBy != "cli" {
		t.Fatalf("updated_by %q", p.UpdatedBy)
	}
}

func TestImportDuplicateMPP(t *testing.T) {
	s := openTemp(t)
	p100 := fixture(t, "P100.tti")
	files := []ImportFile{
		{Path: "a/P200.tti", TTI: fixture(t, "P200.tti").TTI},
		{Path: "x/P100.tti", TTI: p100.TTI},
		{Path: "y/other.tti", TTI: p100.TTI},
	}
	_, err := s.ImportPages(files, "cli")
	if err == nil || !strings.Contains(err.Error(), "x/P100.tti") || !strings.Contains(err.Error(), "y/other.tti") || !strings.Contains(err.Error(), "100") {
		t.Fatalf("got %v", err)
	}
	if n := scalar(t, s, "SELECT count(*) FROM pages"); n != 0 {
		t.Fatalf("count %d", n)
	}
}

func TestImportRefusesDifferingStoredPage(t *testing.T) {
	s := openTemp(t)
	p100 := fixture(t, "P100.tti")
	if _, err := s.ImportPages([]ImportFile{p100}, "cli"); err != nil {
		t.Fatal(err)
	}
	// Flip one low bit of a character in the first OL line's body.
	changed := bytes.Clone(p100.TTI)
	changed[bytes.Index(changed, []byte("OL,28,"))+6] ^= 0x01
	_, err := s.ImportPages([]ImportFile{fixture(t, "P200.tti"), {Path: "changed/P100.tti", TTI: changed}}, "cli")
	if err == nil || !strings.Contains(err.Error(), "changed/P100.tti") || !strings.Contains(err.Error(), "page 100 is already stored with different content") {
		t.Fatalf("got %v", err)
	}
	if n := scalar(t, s, "SELECT count(*) FROM pages"); n != 1 {
		t.Fatalf("count %d", n)
	}
	if p, _ := s.Page(0x100); !bytes.Equal(p.TTI, p100.TTI) {
		t.Fatal("stored page changed")
	}
}

func TestImportParseErrorNamesPathAndLine(t *testing.T) {
	s := openTemp(t)
	bad := []byte("PN,10000\r\nDE,x\r\nOL,30,x\r\n")
	_, err := s.ImportPages([]ImportFile{fixture(t, "P200.tti"), {Path: "bad/P300.tti", TTI: bad}}, "cli")
	if err == nil || !strings.Contains(err.Error(), "bad/P300.tti") || !strings.Contains(err.Error(), "line 3") {
		t.Fatalf("got %v", err)
	}
	if n := scalar(t, s, "SELECT count(*) FROM pages"); n != 0 {
		t.Fatalf("count %d", n)
	}
}

func TestPageNotFound(t *testing.T) {
	if _, err := openTemp(t).Page(0x8FF); !errors.Is(err, ErrNotFound) {
		t.Fatalf("got %v", err)
	}
}

func TestPagesMatchesPage(t *testing.T) {
	s := openTemp(t)
	if _, err := s.ImportPages(fixtures(t), "cli"); err != nil {
		t.Fatal(err)
	}
	got, err := s.Pages()
	if err != nil || len(got) != 30 {
		t.Fatalf("got %d pages, err %v", len(got), err)
	}
	for i, p := range got {
		if i > 0 && got[i-1].MPP >= p.MPP {
			t.Fatalf("not in mpp order at %d", i)
		}
		want, err := s.Page(p.MPP)
		if err != nil {
			t.Fatal(err)
		}
		want.TTI = nil
		if p.TTI != nil || !reflect.DeepEqual(p, *want) {
			t.Errorf("page %03X: got %+v, want %+v", p.MPP, p, *want)
		}
	}
}

// savedEnv imports P100 and returns the store plus a changed copy of its bytes
// (one OL line altered), parsed.
func savedEnv(t *testing.T) (*Store, *tti.Page) {
	t.Helper()
	s := openTemp(t)
	f := fixture(t, "P100.tti")
	if _, err := s.ImportPages([]ImportFile{f}, "cli"); err != nil {
		t.Fatal(err)
	}
	return s, changedPage(t, f.TTI, "EDIT")
}

// changedPage returns f with the text of its first OL,1 line replaced.
func changedPage(t *testing.T, f []byte, text string) *tti.Page {
	t.Helper()
	lines := bytes.Split(f, []byte("\n"))
	for i, l := range lines {
		if bytes.HasPrefix(l, []byte("OL,1,")) {
			lines[i] = []byte("OL,1," + text + "\r")
			break
		}
	}
	p, err := tti.Parse(bytes.Join(lines, []byte("\n")))
	if err != nil {
		t.Fatal(err)
	}
	return p
}

var audit = Audit{Actor: "user:1", Action: "page.update", Remote: "192.0.2.1"}

func TestSavePageIdenticalIsNoop(t *testing.T) {
	s := openTemp(t)
	f := fixture(t, "P100.tti")
	if _, err := s.ImportPages([]ImportFile{f}, "cli"); err != nil {
		t.Fatal(err)
	}
	before, _ := s.Page(0x100)
	p, _ := tti.Parse(f.TTI)
	changed, err := s.SavePage(0x100, p, "", audit)
	if err != nil || changed {
		t.Fatalf("changed %v err %v", changed, err)
	}
	after, _ := s.Page(0x100)
	if after.UpdatedAt != before.UpdatedAt || after.UpdatedBy != "cli" {
		t.Errorf("row changed: %+v", after)
	}
	if n := scalar(t, s, "SELECT count(*) FROM audit"); n != 0 {
		t.Errorf("audit rows %d", n)
	}
	if n := scalar(t, s, "SELECT count(*) FROM page_versions"); n != 0 {
		t.Errorf("version rows %d", n)
	}
}

func TestSavePageArchivesAndAudits(t *testing.T) {
	s, p := savedEnv(t)
	old, _ := s.Page(0x100)
	// Age the stored row so the archived updated_at is distinguishable.
	if _, err := s.db.Exec("UPDATE pages SET updated_at = '2020-01-01T00:00:00Z'"); err != nil {
		t.Fatal(err)
	}
	changed, err := s.SavePage(0x100, p, old.ETag(), audit)
	if err != nil || !changed {
		t.Fatalf("changed %v err %v", changed, err)
	}
	var tb []byte
	var at, by string
	if err := s.db.QueryRow("SELECT tti, updated_at, updated_by FROM page_versions").Scan(&tb, &at, &by); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(tb, old.TTI) || at != "2020-01-01T00:00:00Z" || by != "cli" {
		t.Errorf("version row: equal %v at %q by %q", bytes.Equal(tb, old.TTI), at, by)
	}
	got, _ := s.Page(0x100)
	if !bytes.Equal(got.TTI, p.Bytes()) || got.UpdatedBy != "user:1" || got.UpdatedAt == "2020-01-01T00:00:00Z" ||
		got.Description != p.Description() || got.Subpages != p.Subpages() || got.Transmit != p.Transmit() {
		t.Errorf("page %+v", got)
	}
	var actor, action, target, detail, remote string
	if err := s.db.QueryRow("SELECT actor, action, target, detail, remote FROM audit").Scan(&actor, &action, &target, &detail, &remote); err != nil {
		t.Fatal(err)
	}
	if actor != "user:1" || action != "page.update" || target != "page:100" || remote != "192.0.2.1" ||
		detail != "sha256:"+hex.EncodeToString(sum(p.Bytes())) {
		t.Errorf("audit %q %q %q %q %q", actor, action, target, detail, remote)
	}
	if n := scalar(t, s, "SELECT count(*) FROM audit"); n != 1 {
		t.Errorf("audit rows %d", n)
	}
}

func sum(b []byte) []byte { h := sha256.Sum256(b); return h[:] }

func TestSavePagePrunesTo20(t *testing.T) {
	s, _ := savedEnv(t)
	f := fixture(t, "P100.tti")
	for i := 0; i < 21; i++ {
		if _, err := s.SavePage(0x100, changedPage(t, f.TTI, fmt.Sprintf("v%02d", i)), "", audit); err != nil {
			t.Fatal(err)
		}
	}
	if n := scalar(t, s, "SELECT count(*) FROM page_versions"); n != 20 {
		t.Fatalf("versions %d", n)
	}
	// 21 saves archive the import then v00..v19; the import (oldest) is pruned.
	var oldest []byte
	if err := s.db.QueryRow("SELECT tti FROM page_versions ORDER BY id LIMIT 1").Scan(&oldest); err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(oldest, f.TTI) {
		t.Error("the imported bytes survived pruning")
	}
}

func TestSavePageGeneratedRefused(t *testing.T) {
	s, p := savedEnv(t)
	if _, err := s.db.Exec("UPDATE pages SET binding_id = 1"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.SavePage(0x100, p, "", audit); !errors.Is(err, ErrGenerated) {
		t.Fatalf("got %v", err)
	}
	if scalar(t, s, "SELECT count(*) FROM page_versions")+scalar(t, s, "SELECT count(*) FROM audit") != 0 {
		t.Error("something was written")
	}
}

func TestSavePageStale(t *testing.T) {
	s, p := savedEnv(t)
	if _, err := s.SavePage(0x100, p, strings.Repeat("0", 64), audit); !errors.Is(err, ErrStale) {
		t.Fatalf("got %v", err)
	}
	if scalar(t, s, "SELECT count(*) FROM audit") != 0 {
		t.Error("audit written")
	}
}

func TestSavePageWrongMPPAndMissing(t *testing.T) {
	s, p := savedEnv(t)
	if _, err := s.SavePage(0x101, p, "", audit); err == nil || !strings.Contains(err.Error(), "100") || !strings.Contains(err.Error(), "101") {
		t.Fatalf("got %v", err)
	}
	if _, err := s.db.Exec("DELETE FROM pages"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.SavePage(0x100, p, "", audit); !errors.Is(err, ErrNotFound) {
		t.Fatalf("got %v", err)
	}
}

func TestDeletePageArchivesAndAudits(t *testing.T) {
	s, _ := savedEnv(t)
	old, _ := s.Page(0x100)
	if err := s.DeletePage(0x100, old.ETag(), Audit{Actor: "user:1", Action: "page.delete"}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Page(0x100); !errors.Is(err, ErrNotFound) {
		t.Fatalf("page still there: %v", err)
	}
	var tb []byte
	if err := s.db.QueryRow("SELECT tti FROM page_versions").Scan(&tb); err != nil || !bytes.Equal(tb, old.TTI) {
		t.Fatalf("version: %v", err)
	}
	var target, detail string
	if err := s.db.QueryRow("SELECT target, detail FROM audit").Scan(&target, &detail); err != nil ||
		target != "page:100" || detail != "sha256:"+old.ETag() {
		t.Fatalf("audit %q %q %v", target, detail, err)
	}
	if err := s.DeletePage(0x100, "", Audit{}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("second delete: %v", err)
	}
}

func TestCreatePageExists(t *testing.T) {
	s, p := savedEnv(t) // 0x100 is stored
	if err := s.CreatePage(p, audit); !errors.Is(err, ErrExists) {
		t.Fatalf("got %v", err)
	}
	p2, _ := tti.Parse(fixture(t, "P101.tti").TTI)
	if err := s.CreatePage(p2, Audit{Actor: "user:1", Action: "page.create"}); err != nil {
		t.Fatal(err)
	}
	if scalar(t, s, "SELECT count(*) FROM audit WHERE target = 'page:101'") != 1 {
		t.Error("no create audit row")
	}
}

func TestBackupInDirectoryWithQuote(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "it's data")
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	db := openRaw(t, dir)
	if _, err := db.Exec("PRAGMA user_version = 1"); err != nil {
		t.Fatal(err)
	}
	fsys := fstest.MapFS{
		"0001_baseline.sql": {Data: []byte("-- x")},
		"0002_t.sql":        {Data: []byte("CREATE TABLE t (id INTEGER)")},
	}
	if err := migrate(db, fsys, dir); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "ceems-pre-v2.db")); err != nil {
		t.Fatal(err)
	}
}
