package httpapi

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/time/rate"

	"ceems/internal/output"
)

const goodHeader = "TESTHEAD %%# %%a %e %%b %H:%M:%S" // 32 cells as written

// goodSettings is a valid PUT body; mutate replaces a key, or removes it when nil.
func goodSettings(mutate map[string]any) string {
	m := map[string]any{
		"systemName": "TEST", "vbitConf": true, "headerTemplate": goodHeader,
		"initialTeletextPage": "1ab", "rowAdaptiveMode": true, "linesPerField": 8,
		"magazinePriority": "1,2,3,4,5,6,7,8", "statusDisplay": "STATUS",
		"indexPageEnabled": true, "indexPage": "1AC",
		"networkIdentificationCode": "00ab", "countryNetworkIdentification": "0000", "datacastLines": 8, "magazineSerial": true,
	}
	for k, v := range mutate {
		if v == nil {
			delete(m, k)
		} else {
			m[k] = v
		}
	}
	b, _ := json.Marshal(m)
	return string(b)
}

func TestSettingsAdminOnly(t *testing.T) {
	e := newEnv(t, false)
	e.addUser("admin", true)
	e.addUser("guest", false)
	guest := e.login("guest")
	_, authz := e.key(false)
	for _, rt := range []struct{ method, body string }{{"GET", ""}, {"PUT", goodSettings(nil)}} {
		if rec := e.do(rt.method, "/api/v1/settings", rt.body, guest); rec.Code != http.StatusForbidden {
			t.Errorf("%s as a non-admin: %d", rt.method, rec.Code)
		}
		if rec := e.bearer(rt.method, "/api/v1/settings", rt.body, authz); rec.Code != http.StatusForbidden {
			t.Errorf("%s as a key: %d", rt.method, rec.Code)
		}
	}
	if e.count("SELECT count(*) FROM audit WHERE action = 'settings.update'") != 0 {
		t.Error("a refused PUT wrote an audit row")
	}
}

func TestSettingsValidation(t *testing.T) {
	e := newEnv(t, false)
	e.addUser("admin", true)
	admin := e.login("admin")
	e.api.apiIPLimit = newLimiter(rate.Inf, 1) // the table is longer than the per-IP burst
	dir := filepath.Join(t.TempDir(), "pages")
	w, err := output.Open(dir, e.db, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	reconcile := func() []byte {
		t.Helper()
		if err := w.Reconcile(); err != nil {
			t.Fatal(err)
		}
		b, err := os.ReadFile(filepath.Join(dir, output.VbitConf))
		if err != nil {
			t.Fatal(err)
		}
		return b
	}
	conf := reconcile()
	before := e.do("GET", "/api/v1/settings", "", admin).Body.String()

	long := func(n int) string { return strings.Repeat("A", n) }
	set := func(k string, v any) string { return goodSettings(map[string]any{k: v}) }
	rows := []struct{ name, key, body string }{
		{"not json", "body", `{`},
		{"two objects", "body", goodSettings(nil) + goodSettings(nil)},
		{"pagesDir in the body", "pagesDir", set("pagesDir", "/etc")},
		{"unknown key", "extra", set("extra", 1)},
		{"systemName missing", "systemName", set("systemName", nil)},
		{"headerTemplate missing", "headerTemplate", set("headerTemplate", nil)},
		{"initialTeletextPage missing", "initialTeletextPage", set("initialTeletextPage", nil)},
		{"linesPerField missing", "linesPerField", set("linesPerField", nil)},
		{"magazinePriority missing", "magazinePriority", set("magazinePriority", nil)},
		{"statusDisplay missing", "statusDisplay", set("statusDisplay", nil)},
		{"vbitConf missing", "vbitConf", set("vbitConf", nil)},
		{"rowAdaptiveMode missing", "rowAdaptiveMode", set("rowAdaptiveMode", nil)},
		{"indexPageEnabled missing", "indexPageEnabled", set("indexPageEnabled", nil)},
		{"indexPage missing", "indexPage", set("indexPage", nil)},
		{"networkIdentificationCode missing", "networkIdentificationCode", set("networkIdentificationCode", nil)},
		{"countryNetworkIdentification missing", "countryNetworkIdentification", set("countryNetworkIdentification", nil)},
		{"datacastLines missing", "datacastLines", set("datacastLines", nil)},
		{"magazineSerial missing", "magazineSerial", set("magazineSerial", nil)},
		{"magazineSerial not boolean", "magazineSerial", set("magazineSerial", "true")},
		{"network code 3 chars", "networkIdentificationCode", set("networkIdentificationCode", "000")},
		{"network code 5 chars", "networkIdentificationCode", set("networkIdentificationCode", "00000")},
		{"network code not hex", "networkIdentificationCode", set("networkIdentificationCode", "00G0")},
		{"country code 3 chars", "countryNetworkIdentification", set("countryNetworkIdentification", "000")},
		{"country code 5 chars", "countryNetworkIdentification", set("countryNetworkIdentification", "00000")},
		{"country code not hex", "countryNetworkIdentification", set("countryNetworkIdentification", "00G0")},
		{"datacast -1", "datacastLines", set("datacastLines", -1)},
		{"datacast above lines", "datacastLines", set("datacastLines", 9)},
		{"datacast fraction", "datacastLines", set("datacastLines", 1.5)},
		{"datacast string", "datacastLines", set("datacastLines", "1")},
		{"page 1FF", "initialTeletextPage", set("initialTeletextPage", "1FF")},
		{"page 8ff", "initialTeletextPage", set("initialTeletextPage", "8ff")},
		{"subcode above 3F7F", "initialTeletextPage", set("initialTeletextPage", "100:3F80")},
		{"subcode with bit 0x80", "initialTeletextPage", set("initialTeletextPage", "100:0080")},
		{"subcode with bit 0x4000", "initialTeletextPage", set("initialTeletextPage", "100:4000")},
		{"subcode FFFF", "initialTeletextPage", set("initialTeletextPage", "100:FFFF")},
		{"indexPageEnabled not boolean", "indexPageEnabled", set("indexPageEnabled", "true")},
		{"indexPage 8ff", "indexPage", set("indexPage", "8ff")},
		{"indexPage 8FF", "indexPage", set("indexPage", "8FF")},
		{"indexPage 900", "indexPage", set("indexPage", "900")},
		{"indexPage 0ff", "indexPage", set("indexPage", "0ff")},
		{"indexPage with subcode", "indexPage", set("indexPage", "100:0001")},
		{"vbitConf not boolean", "vbitConf", set("vbitConf", "true")},
		{"rowAdaptiveMode not boolean", "rowAdaptiveMode", set("rowAdaptiveMode", 1)},
		{"header 31 cells", "headerTemplate", set("headerTemplate", goodHeader[:31])},
		{"header 33 cells", "headerTemplate", set("headerTemplate", goodHeader+"X")},
		{"header cell above 0x7F", "headerTemplate", set("headerTemplate", long(31)+"é")},
		{"header unknown token", "headerTemplate", set("headerTemplate", long(30)+"%x")},
		{"header trailing percent", "headerTemplate", set("headerTemplate", long(31)+"%")},
		{"header double percent then z", "headerTemplate", set("headerTemplate", long(29)+"%%z")},
		{"header percent hash", "headerTemplate", set("headerTemplate", long(30)+"%#")},
		{"page 900", "initialTeletextPage", set("initialTeletextPage", "900")},
		{"page 000", "initialTeletextPage", set("initialTeletextPage", "000")},
		{"page 2 digits", "initialTeletextPage", set("initialTeletextPage", "10")},
		{"page not hex", "initialTeletextPage", set("initialTeletextPage", "1G0")},
		{"page short subcode", "initialTeletextPage", set("initialTeletextPage", "100:12")},
		{"page long subcode", "initialTeletextPage", set("initialTeletextPage", "100:12345")},
		{"lines 0", "linesPerField", set("linesPerField", 0)},
		{"lines 17", "linesPerField", set("linesPerField", 17)},
		{"lines string", "linesPerField", set("linesPerField", "8")},
		{"lines fraction", "linesPerField", set("linesPerField", 8.5)},
		{"lines exponent", "linesPerField", strings.Replace(goodSettings(nil), `"linesPerField":8`, `"linesPerField":1e1`, 1)},
		{"lines null", "linesPerField", strings.Replace(goodSettings(nil), `"linesPerField":8`, `"linesPerField":null`, 1)},
		{"priority 3 values", "magazinePriority", set("magazinePriority", "1,2,3")},
		{"priority 0", "magazinePriority", set("magazinePriority", "0,1,2,3,4,5,6,7")},
		{"priority 9 values", "magazinePriority", set("magazinePriority", "1,2,3,4,5,6,7,8,9")},
		{"priority spaces", "magazinePriority", set("magazinePriority", "1, 2,3,4,5,6,7,8")},
		{"status 21 chars", "statusDisplay", set("statusDisplay", long(21))},
		{"status non-ASCII", "statusDisplay", set("statusDisplay", "café")},
		{"status space", "statusDisplay", set("statusDisplay", "CEEMS NEWS")},
		{"status semicolon", "statusDisplay", set("statusDisplay", "A;B")},
		{"status leading space", "statusDisplay", set("statusDisplay", " CeeMS")},
		{"status newline", "statusDisplay", set("statusDisplay", "a\nb")},
		{"name empty", "systemName", set("systemName", "")},
		{"name blank", "systemName", set("systemName", "   ")},
		{"name 41 chars", "systemName", set("systemName", long(41))},
		{"name non-ASCII", "systemName", set("systemName", "café")},
		{"name tab", "systemName", set("systemName", "a\tb")},
	}
	for _, r := range rows {
		rec := e.do("PUT", "/api/v1/settings", r.body, admin)
		if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), `"code":"bad_request"`) || !strings.Contains(rec.Body.String(), r.key+":") {
			t.Errorf("%s: got %d %s, want 400 bad_request naming %s", r.name, rec.Code, rec.Body.String(), r.key)
		}
	}
	if rec := e.do("PUT", "/api/v1/settings", set("statusDisplay", long(5000)), admin); rec.Code != http.StatusRequestEntityTooLarge {
		t.Errorf("a 5000 byte body: %d, want 413", rec.Code)
	}
	if after := e.do("GET", "/api/v1/settings", "", admin).Body.String(); after != before {
		t.Errorf("refused PUTs changed the settings:\n%s\n%s", before, after)
	}
	if n := e.count("SELECT count(*) FROM audit WHERE action = 'settings.update'"); n != 0 {
		t.Errorf("refused PUTs wrote %d audit rows", n)
	}
	if got := reconcile(); string(got) != string(conf) {
		t.Errorf("refused PUTs changed vbit.conf:\n%q\n%q", conf, got)
	}
}

func TestSettingsPutThenGet(t *testing.T) {
	e := newEnv(t, false)
	e.addUser("admin", true)
	admin := e.login("admin")

	rec := e.do("GET", "/api/v1/settings", "", admin)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"pagesDir":"/data/pages"`) || !strings.Contains(rec.Body.String(), `"linesPerField":16`) {
		t.Fatalf("defaults: %d %s", rec.Code, rec.Body.String())
	}

	// A control cell is a legal header cell; the name is trimmed; the page is upper-cased.
	header := "\x03" + goodHeader[1:]
	put := e.do("PUT", "/api/v1/settings", goodSettings(map[string]any{"systemName": "  TEST ", "headerTemplate": header}), admin)
	if put.Code != http.StatusOK {
		t.Fatalf("PUT: %d %s", put.Code, put.Body.String())
	}
	want := map[string]any{
		"systemName": "TEST", "pagesDir": "/data/pages", "vbitConf": true, "headerTemplate": header,
		"initialTeletextPage": "1AB", "rowAdaptiveMode": true, "linesPerField": 8.0,
		"magazinePriority": "1,2,3,4,5,6,7,8", "statusDisplay": "STATUS",
		"indexPageEnabled": true, "indexPage": "1ac",
		"networkIdentificationCode": "00AB", "countryNetworkIdentification": "0000", "datacastLines": 8.0, "magazineSerial": true,
	}
	for _, body := range []string{put.Body.String(), e.do("GET", "/api/v1/settings", "", admin).Body.String()} {
		var got map[string]any
		if err := json.Unmarshal([]byte(body), &got); err != nil {
			t.Fatal(err)
		}
		for k, v := range want {
			if got[k] != v {
				t.Errorf("%s: got %v, want %v", k, got[k], v)
			}
		}
		if len(got) != len(want) {
			t.Errorf("the document has %d keys, want %d", len(got), len(want))
		}
	}
	if n := e.count("SELECT count(*) FROM audit WHERE action = 'settings.update' AND target = 'settings' AND actor = 'user:1'"); n != 1 {
		t.Errorf("%d audit rows, want 1", n)
	}
	e.do("PUT", "/api/v1/settings", goodSettings(map[string]any{"systemName": "TEST", "headerTemplate": header}), admin)
	if n := e.count("SELECT count(*) FROM audit WHERE action = 'settings.update'"); n != 1 {
		t.Errorf("an unchanged PUT wrote an audit row: %d rows", n)
	}
}

// A PUT that leaves out either index field is refused whole: it must never clear a stored value.
func TestSettingsPutKeepsTheIndexSettings(t *testing.T) {
	e := newEnv(t, false)
	e.addUser("admin", true)
	admin := e.login("admin")
	wantStatus(t, e.do("PUT", "/api/v1/settings", goodSettings(nil), admin), http.StatusOK, "")
	for _, k := range []string{"indexPage", "indexPageEnabled"} {
		rec := e.do("PUT", "/api/v1/settings", goodSettings(map[string]any{k: nil, "systemName": "OTHER"}), admin)
		if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), k+":") {
			t.Errorf("PUT without %s: %d %s", k, rec.Code, rec.Body.String())
		}
	}
	var got map[string]any
	json.Unmarshal(e.do("GET", "/api/v1/settings", "", admin).Body.Bytes(), &got)
	if got["indexPageEnabled"] != true || got["indexPage"] != "1ac" || got["systemName"] != "TEST" {
		t.Errorf("a refused PUT changed the settings: %v", got)
	}
	// A full PUT turns the index off and keeps the page it names.
	put := e.do("PUT", "/api/v1/settings", goodSettings(map[string]any{"indexPageEnabled": false, "indexPage": "2BB"}), admin)
	json.Unmarshal(put.Body.Bytes(), &got)
	if got["indexPageEnabled"] != false || got["indexPage"] != "2bb" {
		t.Errorf("full PUT: %v", got)
	}
}

// The edges vbit2 accepts: datacastLines equal to linesPerField, the largest subcode, lower-case codes.
func TestSettingsAcceptsVbit2Edges(t *testing.T) {
	e := newEnv(t, false)
	e.addUser("admin", true)
	admin := e.login("admin")
	for _, page := range []string{"100:3F7F", "8FE", "100:0000"} {
		wantStatus(t, e.do("PUT", "/api/v1/settings", goodSettings(map[string]any{"initialTeletextPage": page}), admin), http.StatusOK, "")
	}
	rec := e.do("PUT", "/api/v1/settings", goodSettings(map[string]any{"linesPerField": 16, "datacastLines": 16, "countryNetworkIdentification": "abcd"}), admin)
	wantStatus(t, rec, http.StatusOK, "")
	if !strings.Contains(rec.Body.String(), `"countryNetworkIdentification":"ABCD"`) || !strings.Contains(rec.Body.String(), `"datacastLines":16`) {
		t.Errorf("body %s", rec.Body.String())
	}
}

func TestMeAndLoginCarryTheSettingsForUsersOnly(t *testing.T) {
	e := newEnv(t, false)
	e.addUser("admin", true)
	rec := e.do("POST", "/api/v1/login", loginBody("admin", testPassword), nil)
	var login map[string]any
	json.Unmarshal(rec.Body.Bytes(), &login)
	if login["systemName"] != "CeeMS" || login["headerTemplate"] != "CeeMS   %%# %%a %e %%b \x03%H:%M:%S" || login["user"] == nil {
		t.Errorf("login: %s", rec.Body.String())
	}
	cookie := rec.Result().Cookies()[0]
	var me map[string]any
	json.Unmarshal(e.do("GET", "/api/v1/me", "", cookie).Body.Bytes(), &me)
	if me["systemName"] != "CeeMS" || me["headerTemplate"] != login["headerTemplate"] {
		t.Errorf("me: %v", me)
	}
	if v, ok := me["indexPage"]; !ok || v != nil || login["indexPage"] != nil {
		t.Errorf("the index is off, so indexPage is null: me %v, login %v", me["indexPage"], login["indexPage"])
	}
	e.do("PUT", "/api/v1/settings", goodSettings(nil), cookie)
	json.Unmarshal(e.do("GET", "/api/v1/me", "", cookie).Body.Bytes(), &me)
	if me["indexPage"] != "1ac" {
		t.Errorf("the index is on, so indexPage is 1ac: %v", me["indexPage"])
	}

	_, authz := e.key(true)
	var key map[string]any
	json.Unmarshal(e.bearer("GET", "/api/v1/me", "", authz).Body.Bytes(), &key)
	if _, ok := key["systemName"]; ok || key["key"] == nil || key["user"] != nil || len(key) != 2 {
		t.Errorf("the key's me document changed: %v", key)
	}
}
