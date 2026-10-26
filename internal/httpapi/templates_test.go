package httpapi

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"ceems/internal/jobs"
	"ceems/internal/store"
	"ceems/internal/tti"
)

// twoSlotTTI is a blank page whose rows 3 and 17 carry slots.
func twoSlotTTI() []byte {
	p := tti.NewPage(0x100)
	p.SetRow(0, 3, "Name {p1name.} Score {>p1score}")
	p.SetRow(0, 17, "------------- {^p4name.} --")
	return p.Bytes()
}

func templateJSONBody(name string, b []byte, rules string) string {
	return `{"name":"` + name + `","tti":"` + base64.StdEncoding.EncodeToString(b) + `","rowRules":` + rules + `}`
}

func TestTemplateCRUD(t *testing.T) {
	e := newEnv(t, false)
	e.addUser("conor", true)
	c := e.login("conor")

	rec := e.do("POST", "/api/v1/templates", templateJSONBody("scores", twoSlotTTI(), `{"17":"p4name"}`), c)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create: %d %s", rec.Code, rec.Body.String())
	}
	var d templateDoc
	if err := json.Unmarshal(rec.Body.Bytes(), &d); err != nil {
		t.Fatal(err)
	}
	want := []slotDoc{
		{Name: "p1name", Row: 3, Col: 5, Width: 9, Align: "left"},
		{Name: "p1score", Row: 3, Col: 21, Width: 10, Align: "right"},
		{Name: "p4name", Row: 17, Col: 14, Width: 10, Align: "centre"},
	}
	if d.ID != 1 || d.Name != "scores" || d.RowRules["17"] != "p4name" || len(d.Slots) != 3 {
		t.Fatalf("document %+v", d)
	}
	for i := range want {
		if d.Slots[i] != want[i] {
			t.Errorf("slot %d %+v want %+v", i, d.Slots[i], want[i])
		}
	}
	if b, _ := base64.StdEncoding.DecodeString(d.TTI); !bytes.Equal(b, twoSlotTTI()) {
		t.Error("tti differs")
	}

	wantStatus(t, e.do("POST", "/api/v1/templates", templateJSONBody("scores", twoSlotTTI(), `{}`), c), http.StatusConflict,
		`{"error":{"code":"exists","message":"a template with that name already exists"}}`)
	rec = e.do("POST", "/api/v1/templates", templateJSONBody("bad", twoSlotTTI(), `{"17":"nosuch"}`), c)
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), `"invalid_template"`) || !strings.Contains(rec.Body.String(), "nosuch") {
		t.Errorf("bad rule: %d %s", rec.Code, rec.Body.String())
	}
	rec = e.do("POST", "/api/v1/templates", templateJSONBody("bad", []byte("OL,1,x\r\n"), `{}`), c)
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), `"invalid_template"`) {
		t.Errorf("bad tti: %d %s", rec.Code, rec.Body.String())
	}

	rec = e.do("GET", "/api/v1/templates", "", c)
	wantStatus(t, rec, http.StatusOK, `{"templates":[{"id":1,"name":"scores","slotCount":3}]}`)
	rec = e.do("GET", "/api/v1/templates/1", "", c)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"slots":[{"name":"p1name","sub":0,"row":3,"col":5,"width":9,"align":"left"}`) {
		t.Errorf("get: %d %s", rec.Code, rec.Body.String())
	}

	// PUT requires every field
	wantStatus(t, e.do("PUT", "/api/v1/templates/1", `{"name":"x","tti":"`+base64.StdEncoding.EncodeToString(twoSlotTTI())+`"}`, c), http.StatusBadRequest, "")
	rec = e.do("PUT", "/api/v1/templates/1", templateJSONBody("renamed", tti.NewPage(0x200).Bytes(), `{}`), c)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"name":"renamed"`) || !strings.Contains(rec.Body.String(), `"slots":[]`) {
		t.Errorf("put: %d %s", rec.Code, rec.Body.String())
	}
	wantStatus(t, e.do("PUT", "/api/v1/templates/9", templateJSONBody("x", twoSlotTTI(), `{}`), c), http.StatusNotFound, "")

	rec = e.do("GET", "/api/v1/templates/1/tti", "", c)
	if rec.Code != http.StatusOK || !bytes.Equal(rec.Body.Bytes(), tti.NewPage(0x200).Bytes()) ||
		rec.Header().Get("Content-Disposition") != `attachment; filename="T1.tti"` {
		t.Errorf("download: %d %q", rec.Code, rec.Header().Get("Content-Disposition"))
	}

	if n := e.count("SELECT count(*) FROM audit WHERE action IN ('template.create','template.update') AND target = 'template:1' AND actor LIKE 'user:%'"); n != 2 {
		t.Errorf("audit rows %d", n)
	}
	wantStatus(t, e.do("DELETE", "/api/v1/templates/1", "", c), http.StatusNoContent, "")
	wantStatus(t, e.do("GET", "/api/v1/templates/1", "", c), http.StatusNotFound, "")
	wantStatus(t, e.do("DELETE", "/api/v1/templates/1", "", c), http.StatusNotFound, "")
	wantStatus(t, e.do("GET", "/api/v1/templates/abc", "", c), http.StatusNotFound, "")
}

func TestDeleteTemplateInUse409(t *testing.T) {
	e := newEnv(t, false)
	e.addUser("conor", true)
	c := e.login("conor")
	wantStatus(t, e.do("POST", "/api/v1/templates", templateJSONBody("scores", twoSlotTTI(), `{}`), c), http.StatusCreated, "")
	if _, err := e.db.CreateBinding(&store.Binding{Name: "pacman", TemplateID: 1, MPP: 0x81A, Source: "{}", Static: "{}"}, store.Audit{Actor: "test"}); err != nil {
		t.Fatal(err)
	}
	rec := e.do("DELETE", "/api/v1/templates/1", "", c)
	if rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), `"in_use"`) || !strings.Contains(rec.Body.String(), "1 binding") {
		t.Fatalf("got %d %s", rec.Code, rec.Body.String())
	}
}

// templateRoutes is every template route with a body that would succeed for an admin.
func templateRoutes() [][3]string {
	body := templateJSONBody("other", twoSlotTTI(), `{}`)
	return [][3]string{
		{"GET", "/api/v1/templates", ""},
		{"POST", "/api/v1/templates", body},
		{"GET", "/api/v1/templates/1", ""},
		{"GET", "/api/v1/templates/1/tti", ""},
		{"PUT", "/api/v1/templates/1", templateJSONBody("renamed", twoSlotTTI(), `{}`)},
		{"DELETE", "/api/v1/templates/1", ""},
	}
}

func TestTemplatesAdminOnly(t *testing.T) { // mutation D
	e := newEnv(t, false)
	e.addUser("conor", true)
	e.addUser("guest", false)
	admin, guest := e.login("conor"), e.login("guest")
	wantStatus(t, e.do("POST", "/api/v1/templates", templateJSONBody("scores", twoSlotTTI(), `{}`), admin), http.StatusCreated, "")
	for _, rt := range templateRoutes() {
		wantStatus(t, e.do(rt[0], rt[1], rt[2], guest), http.StatusForbidden, `{"error":{"code":"forbidden","message":"admin only"}}`)
		wantStatus(t, e.do(rt[0], rt[1], rt[2], nil), http.StatusUnauthorized, unauthorized)
	}
	if n := e.count("SELECT count(*) FROM templates"); n != 1 {
		t.Errorf("templates %d after refused writes", n)
	}
	wantCodes := []int{http.StatusOK, http.StatusCreated, http.StatusOK, http.StatusOK, http.StatusOK, http.StatusNoContent}
	for i, rt := range templateRoutes() {
		if rec := e.do(rt[0], rt[1], rt[2], admin); rec.Code != wantCodes[i] {
			t.Errorf("admin %s %s: %d %s", rt[0], rt[1], rec.Code, rec.Body.String())
		}
	}
}

// updTTI is a page with the given slot names, one per row from row 3.
func updTTI(names ...string) []byte {
	p := tti.NewPage(0x100)
	for i, n := range names {
		p.SetRow(0, 3+i, "{"+n+".........}")
	}
	return p.Bytes()
}

// updEnv has template 2 "upd" (slots p1name, title) bound by "scores-2" (page 2F0) and,
// when two is set, "scores-3" (page 2F1). Each source fills p1name and p2name and each
// static fills title.
func updEnv(t *testing.T, two bool) (*env, *http.Cookie, *jobs.Runner) {
	e, c, r := bindingEnv(t, okSource())
	wantStatus(t, e.do("POST", "/api/v1/templates", templateJSONBody("upd", updTTI("p1name", "title"), `{}`), c), http.StatusCreated, "")
	src := `{"sheet":"` + testSheetID + `","range":"C5:E","columns":["p#name"],"rows":2}`
	wantStatus(t, e.do("POST", "/api/v1/bindings", bindingJSON("scores-2", 2, "2F0", src, `{"title":"T"}`), c), http.StatusCreated, "")
	if two {
		wantStatus(t, e.do("POST", "/api/v1/bindings", bindingJSON("scores-3", 2, "2F1", src, `{"title":"T"}`), c), http.StatusCreated, "")
	}
	return e, c, r
}

func slotNames(t *testing.T, e *env, c *http.Cookie) []string {
	t.Helper()
	var d templateDoc
	if err := json.Unmarshal(e.do("GET", "/api/v1/templates/2", "", c).Body.Bytes(), &d); err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, s := range d.Slots {
		out = append(out, s.Name)
	}
	return out
}

func TestUpdateTemplateSlotRule(t *testing.T) {
	e, c, r := updEnv(t, false)
	put := func(rules string, names ...string) *httptest.ResponseRecorder {
		return e.do("PUT", "/api/v1/templates/2", templateJSONBody("upd", updTTI(names...), rules), c)
	}

	// 3: a slot the source fills is accepted
	if rec := put(`{}`, "p1name", "title", "p2name"); rec.Code != http.StatusOK {
		t.Fatalf("add filled slot: %d %s", rec.Code, rec.Body.String())
	}
	// 1: dropping the fixed slot is accepted and the binding still renders
	if rec := put(`{}`, "p1name"); rec.Code != http.StatusOK {
		t.Fatalf("drop title: %d %s", rec.Code, rec.Body.String())
	}
	if run, err := r.RunNow(context.Background(), 1, "user:1"); err != nil || run.Outcome != "ok" {
		t.Errorf("render after drop: %+v %v", run, err)
	}

	// 2: an unfilled slot is refused and nothing is written
	before := e.do("GET", "/api/v1/templates/2", "", c).Body.String()
	rec := put(`{}`, "p1name", "p9name")
	if rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), `"in_use"`) ||
		!strings.Contains(rec.Body.String(), "binding scores-2 (page 2F0): slot p9name needs a value") ||
		strings.Contains(rec.Body.String(), testSheetID) || strings.Contains(rec.Body.String(), "columns") {
		t.Errorf("unfilled slot: %d %s", rec.Code, rec.Body.String())
	}
	if after := e.do("GET", "/api/v1/templates/2", "", c).Body.String(); after != before {
		t.Errorf("old slots unchanged after 409: got %s want %s", after, before)
	}

	// 6: a row rule naming a slot absent from the new file is a 400
	rec = put(`{"3":"nosuch"}`, "p1name")
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), `"invalid_template"`) {
		t.Errorf("bad rule: %d %s", rec.Code, rec.Body.String())
	}
}

func TestUpdateTemplateNamesEveryBinding(t *testing.T) { // 4
	e, c, _ := updEnv(t, true)
	rec := e.do("PUT", "/api/v1/templates/2", templateJSONBody("upd", updTTI("p1name", "p9name"), `{}`), c)
	want := "binding scores-2 (page 2F0): slot p9name needs a value; binding scores-3 (page 2F1): slot p9name needs a value"
	if rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), want) {
		t.Errorf("two bindings: %d %s", rec.Code, rec.Body.String())
	}
}

func TestUpdateTemplateUnboundAnything(t *testing.T) { // 5: the TestTemplateCRUD PUT covers a swap to a blank page; add a new slot too
	e, c, _ := updEnv(t, false)
	wantStatus(t, e.do("POST", "/api/v1/templates", templateJSONBody("free", updTTI("p1name"), `{}`), c), http.StatusCreated, "")
	if rec := e.do("PUT", "/api/v1/templates/3", templateJSONBody("free", updTTI("p1name", "p9name"), `{}`), c); rec.Code != http.StatusOK {
		t.Errorf("unbound: %d %s", rec.Code, rec.Body.String())
	}
}
