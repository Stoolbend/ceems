package config

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"net/netip"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"ceems/internal/jobs"
)

func write(t *testing.T, body string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "ceems.json")
	if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestLoadValid(t *testing.T) {
	c, err := Load(write(t, `{"listen":"127.0.0.1:8080","dataDir":"data","pagesDir":"pages"}`))
	if err != nil {
		t.Fatal(err)
	}
	want := Config{Listen: "127.0.0.1:8080", DataDir: "data", PagesDir: "pages", LogLevel: "info"}
	if !reflect.DeepEqual(c, want) {
		t.Fatalf("got %+v want %+v", c, want)
	}
	c, err = Load(write(t, `{"listen":"127.0.0.1:8080","dataDir":"data","pagesDir":"pages","secureCookies":true}`))
	if err != nil || !c.SecureCookies {
		t.Fatalf("secureCookies: got %+v err %v", c, err)
	}
}

func TestLoadTrustedProxies(t *testing.T) {
	const ok = `"listen":"127.0.0.1:8080","dataDir":"d","pagesDir":"p"`
	c, err := Load(write(t, `{` + ok + `,"trustedProxies":["127.0.0.1","10.0.0.0/8","::1","fd00::/8"]}`))
	if err != nil {
		t.Fatal(err)
	}
	want := []netip.Prefix{
		netip.MustParsePrefix("127.0.0.1/32"), netip.MustParsePrefix("10.0.0.0/8"),
		netip.MustParsePrefix("::1/128"), netip.MustParsePrefix("fd00::/8"),
	}
	if !reflect.DeepEqual(c.TrustedProxyPrefixes, want) {
		t.Fatalf("got %v want %v", c.TrustedProxyPrefixes, want)
	}
	c, err = Load(write(t, `{` + ok + `}`))
	if err != nil || len(c.TrustedProxies) != 0 || len(c.TrustedProxyPrefixes) != 0 {
		t.Fatalf("absent key: got %+v err %v", c, err)
	}
	for _, bad := range []string{"10.0.0.x", "10.0.0.0/33", "example.com"} {
		p := write(t, `{` + ok + `,"trustedProxies":["127.0.0.1","` + bad + `"]}`)
		_, err := Load(p)
		want := `trustedProxies[1]: "` + bad + `" is not an IP address or CIDR prefix`
		if err == nil || !strings.Contains(err.Error(), p) || !strings.Contains(err.Error(), want) {
			t.Fatalf("%s: got %v, want error containing %q", bad, err, want)
		}
	}
}

func TestLoadErrors(t *testing.T) {
	const ok = `"listen":"127.0.0.1:8080","dataDir":"d","pagesDir":"p"`
	cases := []struct{ name, body, want string }{
		{"listen missing", `{"dataDir":"d","pagesDir":"p"}`, "listen: required"},
		{"dataDir missing", `{"listen":"127.0.0.1:1","pagesDir":"p"}`, "dataDir: required"},
		{"pagesDir missing", `{"listen":"127.0.0.1:1","dataDir":"d"}`, "pagesDir: required"},
		{"dataDir empty", `{"listen":"127.0.0.1:1","dataDir":"","pagesDir":"p"}`, "dataDir: required"},
		{"listen invalid", `{"listen":"nope","dataDir":"d","pagesDir":"p"}`, "listen: "},
		{"logLevel invalid", `{` + ok + `,"logLevel":"loud"}`, `logLevel: must be debug, info, warn or error, got "loud"`},
		{"unknown key", `{` + ok + `,"lisen":"x"}`, `unknown key "lisen"`},
		{"wrong type", `{"listen":5,"dataDir":"d","pagesDir":"p"}`, "listen: must be a string"},
		{"not json", `nope`, "invalid character"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := write(t, tc.body)
			_, err := Load(p)
			if err == nil {
				t.Fatal("want error")
			}
			if got := err.Error(); !strings.Contains(got, p) || !strings.Contains(got, tc.want) {
				t.Fatalf("error %q should contain path %q and %q", got, p, tc.want)
			}
		})
	}
}

func TestLoadMissingFileNamesPath(t *testing.T) {
	p := filepath.Join(t.TempDir(), "absent.json")
	_, err := Load(p)
	if err == nil || !strings.Contains(err.Error(), p) {
		t.Fatalf("got %v", err)
	}
}

func TestConfigGoogleKeyFile(t *testing.T) {
	const base = `"listen":"127.0.0.1:8080","dataDir":"d","pagesDir":"p"`
	c, err := Load(write(t, `{`+base+`}`))
	// The server runs jobs.Unconfigured when the key is absent.
	if _, uerr := (jobs.Unconfigured{}).Read(context.Background(), "s", "A1"); !errors.Is(uerr, jobs.ErrNoKey) {
		t.Fatalf("Unconfigured.Read: %v", uerr)
	}
	if err != nil || c.GoogleKeyFile != "" {
		t.Fatalf("absent key should be off: %+v %v", c, err)
	}

	missing := filepath.Join(t.TempDir(), "nokey.json")
	_, err = Load(write(t, `{`+base+`,"googleKeyFile":`+strconv.Quote(missing)+`}`))
	if err == nil || !strings.Contains(err.Error(), "googleKeyFile") {
		t.Fatalf("missing file: got %v", err)
	}

	_, err = Load(write(t, `{`+base+`,"googleKeyFile":`+strconv.Quote(t.TempDir())+`}`))
	if err == nil || !strings.Contains(err.Error(), "googleKeyFile: not a regular file") {
		t.Fatalf("directory: got %v", err)
	}

	key := filepath.Join(t.TempDir(), "key.json")
	if err := os.WriteFile(key, []byte("not read by Load"), 0o600); err != nil {
		t.Fatal(err)
	}
	c, err = Load(write(t, `{`+base+`,"googleKeyFile":`+strconv.Quote(key)+`}`))
	if err != nil || c.GoogleKeyFile != key {
		t.Fatalf("present file: %+v %v", c, err)
	}
}

// TestDeployExampleConfig checks the shipped deploy/ceems.json. Load stats
// googleKeyFile, which exists only on the Pi, so Load runs on a copy that
// points it at a temporary file.
func TestDeployExampleConfig(t *testing.T) {
	raw, err := os.ReadFile("../../deploy/ceems.json")
	if err != nil {
		t.Fatal(err)
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	var shipped Config
	if err := dec.Decode(&shipped); err != nil {
		t.Fatalf("shipped example: %v", err)
	}

	key := filepath.Join(t.TempDir(), "key.json")
	if err := os.WriteFile(key, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatal(err)
	}
	m["googleKeyFile"] = key
	body, _ := json.Marshal(m)
	c, err := Load(write(t, string(body)))
	if err != nil || c.PagesDir != "/run/ceems-pages" {
		t.Fatalf("got %+v, %v", c, err)
	}
}
