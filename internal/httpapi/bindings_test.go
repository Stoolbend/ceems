package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"ceems/internal/jobs"
)

const testSheetID = "TEST-SHEET-ID-7c1e5b"

// stubSource is a jobs.Source whose Read is a test closure.
type stubSource struct {
	read func(ctx context.Context, sheet, rng string) (jobs.Rows, error)
}

func (s *stubSource) Read(ctx context.Context, sheet, rng string) (jobs.Rows, error) {
	return s.read(ctx, sheet, rng)
}

func okSource() *stubSource {
	return &stubSource{read: func(context.Context, string, string) (jobs.Rows, error) {
		return jobs.Rows{Formatted: [][]string{{"ALICE"}}}, nil
	}}
}

// bindingEnv is newEnv with a runner over src (none when src is nil), an admin
// logged in as c, and template 1 "scores" (twoSlotTTI: slots p1name, p1score, p4name).
func bindingEnv(t *testing.T, src jobs.Source) (*env, *http.Cookie, *jobs.Runner) {
	t.Helper()
	e := newEnv(t, false)
	log := slog.New(slog.NewTextHandler(e.log, nil))
	var r *jobs.Runner
	if src != nil {
		r = jobs.New(e.db, src, log)
	}
	e.h = New(ui, log, e.db, false, r, nil, "/data/pages")
	e.addUser("conor", true)
	c := e.login("conor")
	wantStatus(t, e.do("POST", "/api/v1/templates", templateJSONBody("scores", twoSlotTTI(), `{}`), c), http.StatusCreated, "")
	return e, c, r
}

// The source fills p1name (and updated), so p1score and p4name are the fixed slots.
const (
	testSourceJSON = `{"sheet":"` + testSheetID + `","range":"C5:E","columns":["p#name"],"rows":1}`
	testStatic     = `{"p1score":"\u0001120","p4name":""}`
)

func bindingJSON(name string, templateID int, mpp, source, static string) string {
	return fmt.Sprintf(`{"name":%q,"templateId":%d,"mpp":%q,"source":%s,"static":%s}`, name, templateID, mpp, source, static)
}

func pacman() string { return bindingJSON("pacman", 1, "4A0", testSourceJSON, testStatic) }

func TestBindingCRUD(t *testing.T) {
	e, c, _ := bindingEnv(t, okSource())
	rec := e.do("POST", "/api/v1/bindings", pacman(), c)
	wantStatus(t, rec, http.StatusCreated, `{"id":1,"name":"pacman","templateId":1,"templateName":"scores","mpp":"4A0",`+
		`"source":{"sheet":"`+testSheetID+`","range":"C5:E","columns":["p#name"],"rows":1,"format":{}},`+
		`"static":{"p1score":"\u0001120","p4name":""}}`)
	if n := e.count("SELECT count(*) FROM pages WHERE mpp = 0x4A0 AND binding_id = 1"); n != 1 {
		t.Errorf("bound page rows %d", n)
	}
	rec = e.do("GET", "/api/v1/bindings/1", "", c)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"source":{"sheet":"`+testSheetID+`"`) {
		t.Errorf("get: %d %s", rec.Code, rec.Body.String())
	}

	rec = e.do("PUT", "/api/v1/bindings/1", bindingJSON(" pacman2 ", 1, "4A0", testSourceJSON, `{"p1score":"x","p4name":"y"}`), c)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"name":"pacman2"`) || !strings.Contains(rec.Body.String(), `"static":{"p1score":"x","p4name":"y"}`) {
		t.Errorf("put: %d %s", rec.Code, rec.Body.String())
	}
	wantStatus(t, e.do("PUT", "/api/v1/bindings/9", pacman(), c), http.StatusNotFound, `{"error":{"code":"not_found","message":"no such binding"}}`)
	wantStatus(t, e.do("GET", "/api/v1/bindings", "", c), http.StatusOK,
		`{"bindings":[{"id":1,"name":"pacman2","templateId":1,"templateName":"scores","mpp":"4A0","lastRun":null}],"sheetsConfigured":true}`)
	wantStatus(t, e.do("GET", "/api/v1/bindings/1/runs", "", c), http.StatusOK, `{"runs":[]}`)

	if n := e.count("SELECT count(*) FROM audit WHERE action IN ('binding.create','binding.update') AND target = 'binding:1' AND actor LIKE 'user:%'"); n != 2 {
		t.Errorf("audit rows %d", n)
	}
	wantStatus(t, e.do("DELETE", "/api/v1/bindings/1", "", c), http.StatusNoContent, "")
	wantStatus(t, e.do("GET", "/api/v1/bindings/1", "", c), http.StatusNotFound, "")
	wantStatus(t, e.do("DELETE", "/api/v1/bindings/1", "", c), http.StatusNotFound, "")
	wantStatus(t, e.do("GET", "/api/v1/bindings/1/runs", "", c), http.StatusNotFound, "")
	wantStatus(t, e.do("GET", "/api/v1/bindings/abc", "", c), http.StatusNotFound, "")
	if n := e.count("SELECT count(*) FROM audit WHERE action = 'binding.delete' AND detail = 'pacman2'"); n != 1 {
		t.Errorf("delete audit rows %d", n)
	}
}

// wantRefused checks a 400-class refusal by code and message, and that nothing was stored.
func wantRefused(t *testing.T, e *env, rec *httptest.ResponseRecorder, status int, code, message string) {
	t.Helper()
	want, _ := json.Marshal(map[string]any{"error": map[string]string{"code": code, "message": message}})
	wantStatus(t, rec, status, string(want))
	if strings.Contains(rec.Body.String(), testSheetID) {
		t.Error("error body carries the sheet id")
	}
	if n := e.count("SELECT count(*) FROM bindings"); n != 0 {
		t.Errorf("bindings %d after a refused create", n)
	}
	if n := e.count("SELECT count(*) FROM pages"); n != 0 {
		t.Errorf("pages %d after a refused create", n)
	}
}

func TestBindingValidation(t *testing.T) {
	e, c, _ := bindingEnv(t, okSource())
	const body = "body must be {name, templateId, mpp, source, static}"
	cases := []struct {
		name, body, code, message string
	}{
		{"not json", `{`, "bad_request", body},
		{"no static", `{"name":"x","templateId":1,"mpp":"4A0","source":` + testSourceJSON + `}`, "bad_request", body},
		{"no source", `{"name":"x","templateId":1,"mpp":"4A0","static":{}}`, "bad_request", body},
		{"no mpp", `{"name":"x","templateId":1,"source":` + testSourceJSON + `,"static":{}}`, "bad_request", body},
		{"no template", `{"name":"x","mpp":"4A0","source":` + testSourceJSON + `,"static":{}}`, "bad_request", body},
		{"no name", `{"templateId":1,"mpp":"4A0","source":` + testSourceJSON + `,"static":{}}`, "bad_request", body},
		{"blank name", bindingJSON("  ", 1, "4A0", testSourceJSON, testStatic), "bad_request", "name must be one line of 1-100 bytes"},
		{"two-line name", bindingJSON("a\nb", 1, "4A0", testSourceJSON, testStatic), "bad_request", "name must be one line of 1-100 bytes"},
		{"long name", bindingJSON(strings.Repeat("n", 101), 1, "4A0", testSourceJSON, testStatic), "bad_request", "name must be one line of 1-100 bytes"},
		{"unknown template", bindingJSON("x", 9, "4A0", testSourceJSON, testStatic), "bad_request", "no such template"},
		{"short mpp", bindingJSON("x", 1, "4A", testSourceJSON, testStatic), "bad_request", "mpp must be three hex digits in magazines 1-8"},
		{"magazine 0", bindingJSON("x", 1, "0FF", testSourceJSON, testStatic), "bad_request", "mpp must be three hex digits in magazines 1-8"},
		{"magazine 9", bindingJSON("x", 1, "900", testSourceJSON, testStatic), "bad_request", "mpp must be three hex digits in magazines 1-8"},
		{"not hex", bindingJSON("x", 1, "4G0", testSourceJSON, testStatic), "bad_request", "mpp must be three hex digits in magazines 1-8"},
		{"source string", bindingJSON("x", 1, "4A0", `"`+testSheetID+`"`, testStatic), "invalid_source", "source must be an object"},
		{"source null", bindingJSON("x", 1, "4A0", `null`, testStatic), "invalid_source", "source must be an object"},
		{"source no rows", bindingJSON("x", 1, "4A0", `{"sheet":"`+testSheetID+`","range":"C5:E","columns":["p#name"]}`, testStatic), "invalid_source", "source: rows: must be 1 to 24"},
		{"source unknown key", bindingJSON("x", 1, "4A0", `{"sheet":"`+testSheetID+`","range":"C5:E","columns":["p#name"],"rows":1,"tab":"x"}`, testStatic), "invalid_source", `source: unknown key "tab"`},
		{"static array", bindingJSON("x", 1, "4A0", testSourceJSON, `[]`), "invalid_static", "static must be an object of string values"},
		{"static number", bindingJSON("x", 1, "4A0", testSourceJSON, `{"p1score":1,"p4name":""}`), "invalid_static", "static must be an object of string values"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			wantRefused(t, e, e.do("POST", "/api/v1/bindings", tc.body, c), http.StatusBadRequest, tc.code, tc.message)
		})
	}
}

func TestStaticValidation(t *testing.T) { // mutation K
	cases := []struct{ name, static, message string }{
		{"unknown slot", `{"p1score":"","p4name":"","nosuch":""}`, "slot nosuch is not a fixed slot of template scores"},
		{"missing fixed slot", `{"p1score":""}`, "slot p4name needs a value"},
		{"data key", `{"p1score":"","p4name":"","p1name":"x"}`, "slot p1name is not a fixed slot of template scores"},
		{"updated key", `{"p1score":"","p4name":"","updated":"x"}`, "slot updated is not a fixed slot of template scores"},
		{"non-ASCII", `{"p1score":"café","p4name":""}`, "slot p1score: character outside 0-127"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			e, c, _ := bindingEnv(t, okSource()) // one store per case, so an accepted case cannot mask the next
			wantRefused(t, e, e.do("POST", "/api/v1/bindings", bindingJSON("x", 1, "4A0", testSourceJSON, tc.static), c), http.StatusBadRequest, "invalid_static", tc.message)
		})
	}
	e, c, _ := bindingEnv(t, okSource())
	rec := e.do("POST", "/api/v1/bindings", bindingJSON("x", 1, "4A0", testSourceJSON, `{"p1score":"\u007f\u0000","p4name":""}`), c)
	if rec.Code != http.StatusCreated {
		t.Fatalf("0x7F and 0x00 refused: %d %s", rec.Code, rec.Body.String())
	}
	b, err := e.db.Binding(1)
	if err != nil || b.Static != "{\"p1score\":\"\x7f\\u0000\",\"p4name\":\"\"}" {
		t.Errorf("stored static %q err %v", b.Static, err)
	}
	// PUT is validated the same way.
	wantStatus(t, e.do("PUT", "/api/v1/bindings/1", bindingJSON("x", 1, "4A0", testSourceJSON, `{"p1score":""}`), c), http.StatusBadRequest,
		`{"error":{"code":"invalid_static","message":"slot p4name needs a value"}}`)
}

func TestCreateBindingBadTemplate400(t *testing.T) { // mutation M
	e, c, _ := bindingEnv(t, okSource())
	// static {} matches a template with no slots, so without the lookup the request reaches the store.
	wantRefused(t, e, e.do("POST", "/api/v1/bindings", bindingJSON("x", 2, "4A0", testSourceJSON, `{}`), c), http.StatusBadRequest, "bad_request", "no such template")
}

func TestCreateBindingDuplicateMPP409(t *testing.T) { // mutation M
	e, c, _ := bindingEnv(t, okSource())
	wantStatus(t, e.do("POST", "/api/v1/bindings", pacman(), c), http.StatusCreated, "")
	pages := e.count("SELECT count(*) FROM pages")
	wantStatus(t, e.do("POST", "/api/v1/bindings", bindingJSON("galaxian", 1, "4A0", testSourceJSON, testStatic), c), http.StatusConflict,
		`{"error":{"code":"exists","message":"page 4A0 already has a binding"}}`)
	if n := e.count("SELECT count(*) FROM bindings"); n != 1 {
		t.Errorf("bindings %d", n)
	}
	if n := e.count("SELECT count(*) FROM pages"); n != pages {
		t.Errorf("pages %d -> %d", pages, n)
	}
	wantStatus(t, e.do("POST", "/api/v1/bindings", bindingJSON("pacman", 1, "4A1", testSourceJSON, testStatic), c), http.StatusConflict,
		`{"error":{"code":"exists","message":"a binding with that name already exists"}}`)
}

func TestCreateBindingOnBoundPage409(t *testing.T) {
	e, c, _ := bindingEnv(t, okSource())
	wantStatus(t, e.do("POST", "/api/v1/pages/4A0", "", c), http.StatusCreated, "")
	e.exec("UPDATE pages SET binding_id = 99 WHERE mpp = 0x4A0") // bound, but by no binding the pre-check sees
	wantStatus(t, e.do("POST", "/api/v1/bindings", pacman(), c), http.StatusConflict,
		`{"error":{"code":"page_bound","message":"page 4A0 is already generated by another binding"}}`)
}

func TestUpdateBindingMovesPage(t *testing.T) {
	e, c, _ := bindingEnv(t, okSource())
	wantStatus(t, e.do("POST", "/api/v1/bindings", pacman(), c), http.StatusCreated, "")
	rec := e.do("PUT", "/api/v1/bindings/1", bindingJSON("pacman", 1, "4A1", testSourceJSON, testStatic), c)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"mpp":"4A1"`) {
		t.Fatalf("move: %d %s", rec.Code, rec.Body)
	}
	wantStatus(t, e.do("GET", "/api/v1/pages/4A0", "", c), http.StatusNotFound, "")
	rec = e.do("GET", "/api/v1/pages/4A1", "", c)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"binding"`) {
		t.Errorf("new page: %d %s", rec.Code, rec.Body)
	}
}

func TestUpdateBindingOntoExistingPage409(t *testing.T) { // mutation A
	e, c, _ := bindingEnv(t, okSource())
	wantStatus(t, e.do("POST", "/api/v1/bindings", pacman(), c), http.StatusCreated, "")
	wantStatus(t, e.do("POST", "/api/v1/pages/4A1", "", c), http.StatusCreated, "")
	wantStatus(t, e.do("PUT", "/api/v1/bindings/1", bindingJSON("pacman", 1, "4A1", testSourceJSON, testStatic), c), http.StatusConflict,
		`{"error":{"code":"exists","message":"page 4A1 already exists; the binding was not moved"}}`)
	if b, _ := e.db.Binding(1); b.MPP != 0x4A0 {
		t.Errorf("mpp %03X", b.MPP)
	}
	if n := e.count("SELECT count(*) FROM pages WHERE mpp = 0x4A0 AND binding_id = 1"); n != 1 {
		t.Errorf("bound page rows %d", n)
	}
}

func TestUpdateBindingMPPOutOfRange(t *testing.T) { // I
	e, c, _ := bindingEnv(t, okSource())
	wantStatus(t, e.do("POST", "/api/v1/bindings", pacman(), c), http.StatusCreated, "")
	wantStatus(t, e.do("PUT", "/api/v1/bindings/1", bindingJSON("pacman", 1, "9FF", testSourceJSON, testStatic), c), http.StatusBadRequest,
		`{"error":{"code":"bad_request","message":"mpp must be three hex digits in magazines 1-8"}}`)
	if b, _ := e.db.Binding(1); b.MPP != 0x4A0 {
		t.Errorf("mpp %03X", b.MPP)
	}
}

func TestBindingListHidesSource(t *testing.T) { // mutation L
	e, c, _ := bindingEnv(t, okSource())
	wantStatus(t, e.do("POST", "/api/v1/bindings", pacman(), c), http.StatusCreated, "")
	wantStatus(t, e.do("POST", "/api/v1/bindings/1/run", "", c), http.StatusOK, "")
	rec := e.do("GET", "/api/v1/bindings", "", c)
	if rec.Code != http.StatusOK {
		t.Fatalf("list: %d", rec.Code)
	}
	var doc map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &doc); err != nil {
		t.Fatal(err)
	}
	if doc["sheetsConfigured"] != true {
		t.Errorf("sheetsConfigured %v", doc["sheetsConfigured"])
	}
	item := doc["bindings"].([]any)[0].(map[string]any)
	for _, k := range []string{"source", "static"} {
		if _, ok := item[k]; ok {
			t.Errorf("list item has key %q", k)
		}
	}
	if last, _ := item["lastRun"].(map[string]any); last == nil || last["outcome"] != "ok" {
		t.Errorf("lastRun %v", item["lastRun"])
	}
	if strings.Contains(rec.Body.String(), testSheetID) {
		t.Error("list body contains the sheet id")
	}
	if strings.Contains(e.log.String(), testSheetID) {
		t.Error("log contains the sheet id")
	}
}

func TestSheetsConfiguredFlag(t *testing.T) {
	for _, tc := range []struct {
		name string
		src  jobs.Source
		want string
	}{{"no runner", nil, "false"}, {"no key", jobs.Unconfigured{}, "false"}, {"key", okSource(), "true"}} {
		t.Run(tc.name, func(t *testing.T) {
			e, c, _ := bindingEnv(t, tc.src)
			wantStatus(t, e.do("GET", "/api/v1/bindings", "", c), http.StatusOK, `{"bindings":[],"sheetsConfigured":`+tc.want+`}`)
		})
	}
}

func TestRunNowRoute(t *testing.T) {
	e, c, _ := bindingEnv(t, okSource())
	wantStatus(t, e.do("POST", "/api/v1/bindings", pacman(), c), http.StatusCreated, "")
	rec := e.do("POST", "/api/v1/bindings/1/run", "", c)
	var run runDoc
	if rec.Code != http.StatusOK || json.Unmarshal(rec.Body.Bytes(), &run) != nil || run.ID != 1 || run.Outcome != "ok" || run.Actor != "user:1" || run.StartedAt == "" || run.FinishedAt == "" {
		t.Fatalf("run: %d %s", rec.Code, rec.Body.String())
	}
	if n := e.count("SELECT count(*) FROM audit WHERE action = 'page.generate' AND actor = 'job:binding:1'"); n != 1 {
		t.Errorf("page.generate audit rows %d", n)
	}
	wantStatus(t, e.do("POST", "/api/v1/bindings/2/run", "", c), http.StatusNotFound, `{"error":{"code":"not_found","message":"no such binding"}}`)
	e.do("POST", "/api/v1/bindings/1/run", "", c)
	rec = e.do("GET", "/api/v1/bindings/1/runs", "", c)
	var runs struct{ Runs []runDoc }
	if rec.Code != http.StatusOK || json.Unmarshal(rec.Body.Bytes(), &runs) != nil || len(runs.Runs) != 2 || runs.Runs[0].ID != 2 || runs.Runs[1].ID != 1 {
		t.Errorf("runs: %d %s", rec.Code, rec.Body.String())
	}
}

func TestRunNowRouteUnconfigured(t *testing.T) {
	e, c, _ := bindingEnv(t, jobs.Unconfigured{})
	wantStatus(t, e.do("POST", "/api/v1/bindings", pacman(), c), http.StatusCreated, "")
	rec := e.do("POST", "/api/v1/bindings/1/run", "", c)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"outcome":"failed","error":"googleKeyFile is not configured"`) {
		t.Fatalf("run: %d %s", rec.Code, rec.Body.String())
	}
}

func TestRunNowNoRunner503(t *testing.T) {
	e, c, _ := bindingEnv(t, nil)
	wantStatus(t, e.do("POST", "/api/v1/bindings", pacman(), c), http.StatusCreated, "")
	wantStatus(t, e.do("POST", "/api/v1/bindings/1/run", "", c), http.StatusServiceUnavailable,
		`{"error":{"code":"scheduler_off","message":"the scheduler is not running"}}`)
}

func TestRunNowWhileRunning409(t *testing.T) { // mutation C
	entered, release := make(chan struct{}, 1), make(chan struct{})
	src := &stubSource{read: func(context.Context, string, string) (jobs.Rows, error) {
		select {
		case entered <- struct{}{}:
		default:
		}
		<-release
		return jobs.Rows{Formatted: [][]string{{"ALICE"}}}, nil
	}}
	e, c, r := bindingEnv(t, src)
	wantStatus(t, e.do("POST", "/api/v1/bindings", pacman(), c), http.StatusCreated, "")

	first := make(chan error, 1)
	go func() {
		run, err := r.RunNow(context.Background(), 1, "user:1")
		if err == nil && run.Outcome != "ok" {
			err = fmt.Errorf("first run %+v", run)
		}
		first <- err
	}()
	<-entered

	second := make(chan error, 1)
	var wg sync.WaitGroup // the goroutines below; the mutation leaves them blocked until release
	wg.Go(func() { _, err := r.RunNow(context.Background(), 1, "user:1"); second <- err })
	select {
	case err := <-second:
		if !errors.Is(err, jobs.ErrBusy) {
			t.Errorf("second RunNow: %v, want ErrBusy", err)
		}
	case <-time.After(time.Second):
		t.Error("second RunNow waited for the first instead of returning ErrBusy")
	}
	route := make(chan *httptest.ResponseRecorder, 1)
	wg.Go(func() { route <- e.do("POST", "/api/v1/bindings/1/run", "", c) })
	select {
	case rec := <-route:
		if rec.Code != http.StatusConflict || strings.TrimSpace(rec.Body.String()) != `{"error":{"code":"run_in_progress","message":"a run is in progress; try again shortly"}}` {
			t.Errorf("route during a run: %d %s", rec.Code, rec.Body.String())
		}
	case <-time.After(time.Second):
		t.Error("run route waited for the first run instead of answering 409")
	}

	close(release)
	if err := <-first; err != nil {
		t.Errorf("first run: %v", err)
	}
	wg.Wait() // before the store closes
}

// bindingRoutes is every binding route with a body that would succeed for an admin.
func bindingRoutes() [][3]string {
	return [][3]string{
		{"GET", "/api/v1/bindings", ""},
		{"POST", "/api/v1/bindings", bindingJSON("galaxian", 1, "4A1", testSourceJSON, testStatic)},
		{"GET", "/api/v1/bindings/1", ""},
		{"PUT", "/api/v1/bindings/1", bindingJSON("renamed", 1, "4A0", testSourceJSON, testStatic)},
		{"POST", "/api/v1/bindings/1/run", ""},
		{"GET", "/api/v1/bindings/1/runs", ""},
		{"DELETE", "/api/v1/bindings/1", ""},
	}
}

func TestBindingsAdminOnly(t *testing.T) { // mutation E
	e, admin, _ := bindingEnv(t, okSource())
	e.addUser("guest", false)
	guest := e.login("guest")
	wantStatus(t, e.do("POST", "/api/v1/bindings", pacman(), admin), http.StatusCreated, "")
	audit := e.count("SELECT count(*) FROM audit")
	for _, rt := range bindingRoutes() {
		wantStatus(t, e.do(rt[0], rt[1], rt[2], guest), http.StatusForbidden, `{"error":{"code":"forbidden","message":"admin only"}}`)
		wantStatus(t, e.do(rt[0], rt[1], rt[2], nil), http.StatusUnauthorized, unauthorized)
	}
	if n := e.count("SELECT count(*) FROM bindings"); n != 1 {
		t.Errorf("bindings %d after refused writes", n)
	}
	if n := e.count("SELECT count(*) FROM runs"); n != 0 {
		t.Errorf("runs %d after refused runs", n)
	}
	if n := e.count("SELECT count(*) FROM audit"); n != audit {
		t.Errorf("audit %d -> %d after refused requests", audit, n)
	}
	wantCodes := []int{http.StatusOK, http.StatusCreated, http.StatusOK, http.StatusOK, http.StatusOK, http.StatusOK, http.StatusNoContent}
	for i, rt := range bindingRoutes() {
		if rec := e.do(rt[0], rt[1], rt[2], admin); rec.Code != wantCodes[i] {
			t.Errorf("admin %s %s: %d %s", rt[0], rt[1], rec.Code, rec.Body.String())
		}
	}
}

func TestDeleteBindingRouteDetaches(t *testing.T) { // mutation H
	e, c, _ := bindingEnv(t, okSource())
	wantStatus(t, e.do("POST", "/api/v1/bindings", pacman(), c), http.StatusCreated, "")
	wantStatus(t, e.do("POST", "/api/v1/bindings/1/run", "", c), http.StatusOK, "")
	generated := e.storedTTI(0x4A0)
	wantStatus(t, e.do("DELETE", "/api/v1/bindings/1", "", c), http.StatusNoContent, "")
	if n := e.count("SELECT count(*) FROM pages WHERE mpp = 0x4A0 AND binding_id IS NULL"); n != 1 {
		t.Fatal("page still bound after its binding was deleted")
	}
	if string(e.storedTTI(0x4A0)) != string(generated) {
		t.Error("deleting the binding changed the page bytes")
	}
	rec := e.putDoc("4A0", e.getDoc("4A0", c), c, func(d map[string]any) { setRow(d, 0, 5, "edited by hand") })
	if rec.Code != http.StatusOK {
		t.Errorf("PUT a detached page: %d %s", rec.Code, rec.Body.String())
	}
}
