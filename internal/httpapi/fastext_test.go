package httpapi

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"golang.org/x/time/rate"
)

// fxEnv is a server with an admin logged in and the blank pages 201 to 204.
func fxEnv(t *testing.T) (*env, *http.Cookie) {
	t.Helper()
	e := newEnv(t, false)
	e.api.apiIPLimit = newLimiter(rate.Inf, 1)
	e.addUser("admin", true)
	admin := e.login("admin")
	for _, p := range []string{"201", "202", "203", "204"} {
		wantStatus(t, e.do("POST", "/api/v1/pages/"+p, "", admin), http.StatusCreated, "")
	}
	return e, admin
}

func fxBody(name string, paged bool, slots string, def bool) string {
	return fmt.Sprintf(`{"name":%q,"paged":%v,"links":["7aa","7bb","7cc","7dd"],%s"default":%v}`, name, paged, slots, def)
}

const pagedSlots = `"prevSlot":0,"nextSlot":1,`

func decode(t *testing.T, body string) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal([]byte(body), &m); err != nil {
		t.Fatal(err)
	}
	return m
}

// fxCreate makes a group and returns its path.
func fxCreate(t *testing.T, e *env, admin *http.Cookie, body string) string {
	t.Helper()
	rec := e.do("POST", "/api/v1/fastext-groups", body, admin)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create: %d %s", rec.Code, rec.Body.String())
	}
	return fmt.Sprintf("/api/v1/fastext-groups/%v", decode(t, rec.Body.String())["id"])
}

// fl returns the first subpage's FL links of page mpp, comma separated.
func fl(t *testing.T, e *env, admin *http.Cookie, mpp string) string {
	t.Helper()
	d := decode(t, e.do("GET", "/api/v1/pages/"+mpp, "", admin).Body.String())
	var links []string
	for _, l := range d["subpages"].([]any)[0].(map[string]any)["links"].([]any) {
		links = append(links, l.(string))
	}
	return strings.Join(links, ",")
}

func TestFastextRoutesAdminOnly(t *testing.T) {
	e, admin := fxEnv(t)
	path := fxCreate(t, e, admin, fxBody("g", false, "", false))
	e.addUser("guest", false)
	guest := e.login("guest")
	_, authz := e.key(false)
	for _, rt := range [][3]string{
		{"GET", "/api/v1/fastext-groups", ""}, {"POST", "/api/v1/fastext-groups", fxBody("x", false, "", false)},
		{"GET", path, ""}, {"PUT", path, fxBody("g", false, "", false)}, {"DELETE", path, ""},
		{"PUT", path + "/pages", `{"pages":[]}`},
	} {
		if rec := e.do(rt[0], rt[1], rt[2], guest); rec.Code != http.StatusForbidden {
			t.Errorf("%s %s as a non-admin: %d", rt[0], rt[1], rec.Code)
		}
		if rec := e.bearer(rt[0], rt[1], rt[2], authz); rec.Code != http.StatusForbidden {
			t.Errorf("%s %s as a key: %d", rt[0], rt[1], rec.Code)
		}
	}
	if n := e.count("SELECT count(*) FROM fastext_groups"); n != 1 {
		t.Errorf("%d groups, want 1", n)
	}
}

func TestFastextGroupCrud(t *testing.T) {
	e, admin := fxEnv(t)
	b := fxCreate(t, e, admin, fxBody("b", true, pagedSlots, false))
	fxCreate(t, e, admin, fxBody("a", false, "", false))

	list := decode(t, e.do("GET", "/api/v1/fastext-groups", "", admin).Body.String())["groups"].([]any)
	if len(list) != 2 || list[0].(map[string]any)["name"] != "a" || list[1].(map[string]any)["name"] != "b" {
		t.Errorf("groups not ordered by name: %v", list)
	}
	if a := list[0].(map[string]any); a["prevSlot"] != nil || a["nextSlot"] != nil {
		t.Errorf("static group slots are not null: %v", a)
	}
	g := decode(t, e.do("GET", b, "", admin).Body.String())
	if g["paged"] != true || g["prevSlot"] != 0.0 || g["nextSlot"] != 1.0 || fmt.Sprint(g["links"]) != "[7aa 7bb 7cc 7dd]" || len(g["pages"].([]any)) != 0 {
		t.Errorf("paged group: %v", g)
	}

	put := e.do("PUT", b, fxBody("b2", true, `"prevSlot":2,"nextSlot":3,`, false), admin)
	if g := decode(t, put.Body.String()); put.Code != http.StatusOK || g["name"] != "b2" || g["prevSlot"] != 2.0 {
		t.Errorf("PUT: %d %s", put.Code, put.Body.String())
	}
	wantStatus(t, e.do("DELETE", b, "", admin), http.StatusNoContent, "")
	wantStatus(t, e.do("GET", b, "", admin), http.StatusNotFound, `{"error":{"code":"not_found","message":"no such group or page"}}`)
	wantStatus(t, e.do("DELETE", b, "", admin), http.StatusNotFound, "")
	for _, id := range []string{"0", "-1", "x"} {
		wantStatus(t, e.do("GET", "/api/v1/fastext-groups/"+id, "", admin), http.StatusNotFound, "")
	}
	// Two creates, one update and one delete.
	if n := e.count("SELECT count(*) FROM audit WHERE action LIKE 'fastext.%' AND actor = 'user:1'"); n != 4 {
		t.Errorf("%d group audit rows with the request's actor, want 4", n)
	}
}

func TestFastextValidation(t *testing.T) {
	e, admin := fxEnv(t)
	fxCreate(t, e, admin, fxBody("taken", false, "", false))
	rows := []struct {
		name, body string
		code       int
		errCode    string
	}{
		{"static with slots", fxBody("s", false, pagedSlots, false), 400, "bad_request"},
		{"paged without slots", fxBody("p", true, "", false), 400, "bad_request"},
		{"paged same slot", fxBody("p", true, `"prevSlot":1,"nextSlot":1,`, false), 400, "bad_request"},
		{"paged slot 4", fxBody("p", true, `"prevSlot":0,"nextSlot":4,`, false), 400, "bad_request"},
		{"paged one null slot", fxBody("p", true, `"prevSlot":0,"nextSlot":null,`, false), 400, "bad_request"},
		{"empty name", fxBody("", false, "", false), 400, "bad_request"},
		{"bad link", strings.Replace(fxBody("l", false, "", false), "7aa", "9aa", 1), 400, "bad_request"},
		{"three links", strings.Replace(fxBody("l", false, "", false), `,"7dd"`, "", 1), 400, "bad_request"},
		{"missing paged", `{"name":"x","links":["7aa","7bb","7cc","7dd"],"default":false}`, 400, "bad_request"},
		{"missing default", `{"name":"x","paged":false,"links":["7aa","7bb","7cc","7dd"]}`, 400, "bad_request"},
		{"unknown field", strings.Replace(fxBody("x", false, "", false), `"name"`, `"extra":1,"name"`, 1), 400, "bad_request"},
		{"duplicate name", fxBody("taken", false, "", false), 409, "conflict"},
	}
	for _, r := range rows {
		rec := e.do("POST", "/api/v1/fastext-groups", r.body, admin)
		if rec.Code != r.code || !strings.Contains(rec.Body.String(), `"code":"`+r.errCode+`"`) {
			t.Errorf("%s: %d %s", r.name, rec.Code, rec.Body.String())
		}
	}
	if n := e.count("SELECT count(*) FROM fastext_groups"); n != 1 {
		t.Errorf("a refused create left %d groups", n)
	}
	wantStatus(t, e.do("POST", "/api/v1/fastext-groups", strings.Repeat("x", 300<<10), admin), http.StatusRequestEntityTooLarge, "")
}

func TestFastextMembersLinkPagesAndShowInTheDocument(t *testing.T) {
	e, admin := fxEnv(t)
	g := fxCreate(t, e, admin, fxBody("menu", true, pagedSlots, false))
	rec := e.do("PUT", g+"/pages", `{"pages":["201","202","203"]}`, admin)
	if rec.Code != http.StatusOK {
		t.Fatalf("set pages: %d %s", rec.Code, rec.Body.String())
	}
	if pages := decode(t, rec.Body.String())["pages"].([]any); len(pages) != 3 || pages[1].(map[string]any)["mpp"] != "202" {
		t.Errorf("members: %v", pages)
	}
	if got := fl(t, e, admin, "202"); got != "201,203,7cc,7dd,8ff,8ff" {
		t.Errorf("202 links %q", got)
	}
	if got := fl(t, e, admin, "201"); got != "8ff,202,7cc,7dd,8ff,8ff" {
		t.Errorf("201 links %q", got)
	}
	doc := decode(t, e.do("GET", "/api/v1/pages/202", "", admin).Body.String())
	if fg, _ := doc["fastextGroup"].(map[string]any); fg == nil || fg["name"] != "menu" || fg["paged"] != true || fg["id"] == nil || len(fg) != 3 {
		t.Errorf("fastextGroup: %v", doc["fastextGroup"])
	}
	page204 := e.do("GET", "/api/v1/pages/204", "", admin).Body.String()
	if decode(t, page204)["fastextGroup"] != nil {
		t.Errorf("a page in no group has a fastextGroup")
	}
	// The group document is ignored on a page PUT.
	forged := strings.Replace(page204, `"fastextGroup":null`, `"fastextGroup":{"id":1,"name":"x","paged":true}`, 1)
	put := e.do("PUT", "/api/v1/pages/204", forged, admin)
	if put.Code != http.StatusOK || decode(t, put.Body.String())["fastextGroup"] != nil {
		t.Errorf("page PUT with a fastextGroup: %d %s", put.Code, put.Body.String())
	}

	other := fxCreate(t, e, admin, fxBody("other", false, "", false))
	rec = e.do("PUT", other+"/pages", `{"pages":["203"]}`, admin)
	if rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), `"code":"in_group"`) {
		t.Errorf("page in another group: %d %s", rec.Code, rec.Body.String())
	}
	for _, body := range []string{`{}`, `{"pages":["20"]}`, `{"pages":["900"]}`, `{"pages":[1]}`, `{"pages":["201","201"]}`} {
		if rec := e.do("PUT", g+"/pages", body, admin); rec.Code != http.StatusBadRequest {
			t.Errorf("pages %s: %d", body, rec.Code)
		}
	}
	wantStatus(t, e.do("PUT", g+"/pages", `{"pages":["7FF"]}`, admin), http.StatusNotFound, "")
	wantStatus(t, e.do("PUT", "/api/v1/fastext-groups/99/pages", `{"pages":[]}`, admin), http.StatusNotFound, "")
	if got := fl(t, e, admin, "202"); got != "201,203,7cc,7dd,8ff,8ff" {
		t.Errorf("refused requests changed 202: %q", got)
	}
}

func TestFastextOnlyOneDefault(t *testing.T) {
	e, admin := fxEnv(t)
	first := fxCreate(t, e, admin, fxBody("first", false, "", true))
	second := fxCreate(t, e, admin, fxBody("second", false, "", true))
	if d := decode(t, e.do("GET", first, "", admin).Body.String())["default"]; d != false {
		t.Errorf("first is still the default: %v", d)
	}
	if d := decode(t, e.do("GET", second, "", admin).Body.String())["default"]; d != true {
		t.Errorf("second is not the default: %v", d)
	}
}

func TestFastextTexts(t *testing.T) { // criteria 14 and 15
	e, admin := fxEnv(t)
	texts := func(rec interface{ String() string }) string { return fmt.Sprint(decode(t, rec.String())["texts"]) }
	// Absent texts read back as four empty strings.
	rec := e.do("POST", "/api/v1/fastext-groups", fxBody("none", false, "", false), admin)
	if rec.Code != http.StatusCreated || texts(rec.Body) != "[   ]" {
		t.Fatalf("absent texts: %d %s", rec.Code, rec.Body.String())
	}
	with := func(name, arr string) string {
		return strings.Replace(fxBody(name, false, "", false), `"default"`, `"texts":`+arr+`,"default"`, 1)
	}
	path := fxCreate(t, e, admin, with("some", `["Index","","\u0001a","Art index"]`))
	if got := decode(t, e.do("GET", path, "", admin).Body.String())["texts"].([]any); fmt.Sprintf("%q", got) != `["Index" "" "\x01a" "Art index"]` {
		t.Errorf("texts %q", got)
	}
	// PUT replaces: null and absent both clear.
	for _, body := range []string{with("some", "null"), fxBody("some", false, "", false)} {
		if rec := e.do("PUT", path, body, admin); rec.Code != http.StatusOK || texts(rec.Body) != "[   ]" {
			t.Errorf("PUT %s: %d %s", body, rec.Code, rec.Body.String())
		}
	}
	for name, body := range map[string]string{
		"three texts": with("x", `["a","b","c"]`),
		"five texts":  with("x", `["a","b","c","d","e"]`),
		"ten cells":   with("x", `["","","","0123456789"]`),
		"non-ASCII":   with("x", `["é","","",""]`),
	} {
		if rec := e.do("POST", "/api/v1/fastext-groups", body, admin); rec.Code != 400 || !strings.Contains(rec.Body.String(), `"code":"bad_request"`) {
			t.Errorf("%s: %d %s", name, rec.Code, rec.Body.String())
		}
	}
	if n := e.count("SELECT count(*) FROM fastext_groups"); n != 2 {
		t.Errorf("%d groups, want 2", n)
	}
}
