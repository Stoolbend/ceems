package main

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"ceems/internal/auth"
	"ceems/internal/store"
)

func writeConfig(t *testing.T) (cfgPath, dataDir string) {
	t.Helper()
	dir := t.TempDir()
	dataDir = filepath.Join(dir, "data")
	b, _ := json.Marshal(map[string]string{"listen": "127.0.0.1:0", "dataDir": dataDir, "pagesDir": "pages", "logLevel": "info"})
	cfgPath = filepath.Join(dir, "ceems.json")
	if err := os.WriteFile(cfgPath, b, 0o644); err != nil {
		t.Fatal(err)
	}
	return cfgPath, dataDir
}

func TestImportDirTwice(t *testing.T) {
	cfg, dataDir := writeConfig(t)
	for _, want := range []string{"import: 30 added, 0 unchanged\n", "import: 0 added, 30 unchanged\n"} {
		var buf bytes.Buffer
		if err := importDir(cfg, "../../testdata/sample/pages", &buf); err != nil {
			t.Fatal(err)
		}
		if buf.String() != want {
			t.Fatalf("got %q want %q", buf.String(), want)
		}
	}
	db, err := store.Open(dataDir)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if p, err := db.Page(0x100); err != nil || p.Subpages != 7 {
		t.Fatalf("page %v err %v", p, err)
	}
}

func TestImportDirEmpty(t *testing.T) {
	cfg, _ := writeConfig(t)
	err := importDir(cfg, t.TempDir(), &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "no .tti files") {
		t.Fatalf("got %v", err)
	}
}

var passwordLine = regexp.MustCompile(`^password: [A-Za-z0-9]{20}\n$`)

func openData(t *testing.T, dataDir string) (*store.Store, *sql.DB) {
	t.Helper()
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		t.Fatal(err)
	}
	db, err := store.Open(dataDir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	raw, err := sql.Open("sqlite", filepath.Join(dataDir, "ceems.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { raw.Close() })
	return db, raw
}

func TestAdminCreate(t *testing.T) {
	cfg, dataDir := writeConfig(t)
	var out bytes.Buffer
	if err := adminCreate(cfg, "Conor", &out); err != nil {
		t.Fatal(err)
	}
	first, rest, _ := strings.Cut(out.String(), "\n")
	if first != "created admin conor" || !passwordLine.MatchString(rest) {
		t.Fatalf("output shape wrong: first line %q, second line matches %v", first, passwordLine.MatchString(rest))
	}
	pw := strings.TrimSuffix(strings.TrimPrefix(rest, "password: "), "\n")
	db, _ := openData(t, dataDir)
	if ok, err := db.HasAdmin(); !ok || err != nil {
		t.Fatalf("HasAdmin %v %v", ok, err)
	}
	if _, err := auth.Login(db, "conor", pw); err != nil {
		t.Fatalf("printed password does not log in: %v", err)
	}
	for _, name := range []string{"conor", "CONOR"} {
		out.Reset()
		if err := adminCreate(cfg, name, &out); err == nil || err.Error() != `user "conor" already exists` || out.Len() != 0 {
			t.Fatalf("duplicate %s: err %v, printed %d bytes", name, err, out.Len())
		}
	}
	for _, bad := range []string{"", "has space", "x/y", strings.Repeat("a", 65)} {
		if err := adminCreate(cfg, bad, &out); err == nil || err.Error() != "username: must be 1-64 characters of letters, digits, '.', '_' or '-'" {
			t.Fatalf("username %q: %v", bad, err)
		}
	}
}

func TestAdminResetPassword(t *testing.T) {
	cfg, dataDir := writeConfig(t)
	if err := adminResetPassword(cfg, "nobody", &bytes.Buffer{}); err == nil || err.Error() != `user "nobody" not found` {
		t.Fatalf("unknown: %v", err)
	}
	if err := adminCreate(cfg, "conor", &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := adminResetPassword(cfg, "CONOR", &out); err != nil {
		t.Fatal(err)
	}
	first, rest, _ := strings.Cut(out.String(), "\n")
	if first != "password for conor reset" || !passwordLine.MatchString(rest) {
		t.Fatalf("output shape wrong: first line %q", first)
	}
	db, _ := openData(t, dataDir)
	if _, err := auth.Login(db, "conor", strings.TrimSuffix(strings.TrimPrefix(rest, "password: "), "\n")); err != nil {
		t.Fatalf("new password does not log in: %v", err)
	}
}

// M
func TestAdminCreateAudits(t *testing.T) {
	cfg, dataDir := writeConfig(t)
	var out bytes.Buffer
	if err := adminCreate(cfg, "conor", &out); err != nil {
		t.Fatal(err)
	}
	pw := strings.TrimSuffix(strings.SplitN(out.String(), "password: ", 2)[1], "\n")
	_, raw := openData(t, dataDir)
	count := func() int {
		var n int
		if err := raw.QueryRow("SELECT count(*) FROM audit").Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}
	var actor, action, target, detail string
	if err := raw.QueryRow("SELECT actor, action, target, detail FROM audit").Scan(&actor, &action, &target, &detail); err != nil {
		t.Fatalf("no audit row after create: %v", err)
	}
	if n := count(); n != 1 || actor != "cli" || action != "user.create" || target != "user:1" || detail != "conor" {
		t.Fatalf("audit count %d row %q %q %q %q", n, actor, action, target, detail)
	}
	if adminCreate(cfg, "conor", &bytes.Buffer{}) == nil || count() != 1 {
		t.Fatalf("duplicate create: audit count %d", count())
	}
	if err := adminResetPassword(cfg, "conor", &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	if n := count(); n != 2 {
		t.Fatalf("after reset: audit count %d", n)
	}
	if err := raw.QueryRow("SELECT action, target FROM audit WHERE id = 2").Scan(&action, &target); err != nil || action != "user.reset_password" || target != "user:1" {
		t.Fatalf("reset row %q %q %v", action, target, err)
	}
	var all string
	raw.QueryRow("SELECT group_concat(actor || action || target || detail || remote) FROM audit").Scan(&all)
	if strings.Contains(all, pw) || strings.Contains(all, "$2") {
		t.Fatal("audit rows contain a password or hash")
	}
}

func TestWarnIfNoAdmin(t *testing.T) {
	cfg, dataDir := writeConfig(t)
	db, _ := openData(t, dataDir)
	var buf bytes.Buffer
	log := slog.New(slog.NewTextHandler(&buf, nil))
	warnIfNoAdmin(db, log, cfg)
	if want := "run=" + strconv.Quote("ceems admin create -config "+cfg+" <username>"); !strings.Contains(buf.String(), "no admin user exists") || !strings.Contains(buf.String(), want) {
		t.Fatalf("log %q missing %q", buf.String(), want)
	}
	if err := adminCreate(cfg, "conor", &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	buf.Reset()
	warnIfNoAdmin(db, log, cfg)
	if buf.Len() != 0 {
		t.Fatalf("logged with an admin present: %q", buf.String())
	}
}
