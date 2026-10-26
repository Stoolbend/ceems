package httpapi

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"ceems/internal/store"
	"ceems/internal/tti"
)

func readFixture(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile("../../testdata/sample/pages/" + name)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// editedP100 is P100 with the text of its OL,1 line changed.
func editedP100(t *testing.T) []byte {
	t.Helper()
	lines := bytes.Split(readFixture(t, "P100.tti"), []byte("\n"))
	for i, l := range lines {
		if bytes.HasPrefix(l, []byte("OL,1,")) {
			lines[i] = []byte("OL,1,EDITED\r")
			break
		}
	}
	return bytes.Join(lines, []byte("\n"))
}

func TestDownloadEveryFixtureByteIdentical(t *testing.T) {
	e := newEnv(t, false)
	importAll(e)
	e.addUser("conor", true)
	c := e.login("conor")
	paths, _ := filepath.Glob("../../testdata/sample/pages/*.tti")
	for _, p := range paths {
		name := filepath.Base(p)
		mpp := strings.TrimSuffix(strings.TrimPrefix(name, "P"), ".tti")
		rec := e.do("GET", "/api/v1/pages/"+mpp+"/tti", "", c)
		if rec.Code != http.StatusOK || !bytes.Equal(rec.Body.Bytes(), readFixture(t, name)) {
			t.Errorf("%s: status %d, identical %v", name, rec.Code, bytes.Equal(rec.Body.Bytes(), readFixture(t, name)))
		}
		if ct, cd := rec.Header().Get("Content-Type"), rec.Header().Get("Content-Disposition"); ct != "application/octet-stream" ||
			cd != fmt.Sprintf(`attachment; filename="%s"`, name) || rec.Header().Get("ETag") != "" {
			t.Errorf("%s: headers %q %q etag %q", name, ct, cd, rec.Header().Get("ETag"))
		}
	}
}

func TestUploadReplacesPage(t *testing.T) {
	e := newEnv(t, false)
	e.importPage("P100.tti")
	e.addUser("conor", true)
	c := e.login("conor")
	body := editedP100(t)
	rec := e.do("PUT", "/api/v1/pages/100/tti", string(body), c)
	sum := sha256.Sum256(body)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"etag":"`+hex.EncodeToString(sum[:])+`"`) || !strings.Contains(rec.Body.String(), `"updatedBy":"user:`) {
		t.Fatalf("got %d %s", rec.Code, rec.Body.String())
	}
	if dl := e.do("GET", "/api/v1/pages/100/tti", "", c); !bytes.Equal(dl.Body.Bytes(), body) {
		t.Error("download differs from upload")
	}
	if n := e.count("SELECT count(*) FROM audit WHERE action = 'page.upload' AND target = 'page:100' AND detail = 'sha256:" + hex.EncodeToString(sum[:]) + "'"); n != 1 {
		t.Errorf("audit rows %d", n)
	}
	if n := e.count("SELECT count(*) FROM page_versions"); n != 1 {
		t.Errorf("version rows %d", n)
	}
}

func TestUploadIdenticalWritesNothing(t *testing.T) {
	e := newEnv(t, false)
	e.importPage("P100.tti")
	e.addUser("conor", true)
	e.exec("UPDATE pages SET updated_at = '2020-01-01T00:00:00Z'")
	rec := e.do("PUT", "/api/v1/pages/100/tti", string(readFixture(t, "P100.tti")), e.login("conor"))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"updatedAt":"2020-01-01T00:00:00Z"`) {
		t.Fatalf("got %d %s", rec.Code, rec.Body.String())
	}
	if e.count("SELECT count(*) FROM audit WHERE action = 'page.upload'")+e.count("SELECT count(*) FROM page_versions") != 0 {
		t.Error("audit or version row written")
	}
}

func TestUploadMPPMismatch(t *testing.T) {
	e := newEnv(t, false)
	e.importPage("P100.tti")
	e.addUser("conor", true)
	rec := e.do("PUT", "/api/v1/pages/100/tti", string(readFixture(t, "P101.tti")), e.login("conor"))
	wantStatus(t, rec, http.StatusBadRequest, `{"error":{"code":"mpp_mismatch","message":"file is page 101 but the target is page 100"}}`)
	if e.count("SELECT count(*) FROM audit WHERE action = 'page.upload'")+e.count("SELECT count(*) FROM page_versions") != 0 {
		t.Error("audit or version row written")
	}
}

func TestUploadInvalidAndTooLarge(t *testing.T) {
	e := newEnv(t, false)
	e.importPage("P100.tti")
	e.addUser("conor", true)
	c := e.login("conor")
	rec := e.do("PUT", "/api/v1/pages/100/tti", "", c)
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), `"invalid_tti"`) {
		t.Errorf("empty: %d %s", rec.Code, rec.Body.String())
	}
	bad := bytes.Replace(readFixture(t, "P100.tti"), []byte("OL,1,"), []byte("OL,99,"), 1)
	rec = e.do("PUT", "/api/v1/pages/100/tti", string(bad), c)
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), `"invalid_tti"`) || !strings.Contains(rec.Body.String(), "line ") {
		t.Errorf("bad line: %d %s", rec.Code, rec.Body.String())
	}
	rec = e.do("PUT", "/api/v1/pages/100/tti", strings.Repeat("x", maxPageBody+1), c)
	if rec.Code != http.StatusRequestEntityTooLarge || !strings.Contains(rec.Body.String(), `"too_large"`) {
		t.Errorf("large: %d %s", rec.Code, rec.Body.String())
	}
}

func TestUploadIfMatch(t *testing.T) {
	e := newEnv(t, false)
	e.importPage("P100.tti")
	e.addUser("conor", true)
	c := e.login("conor")
	body := string(editedP100(t))
	rec := e.do("PUT", "/api/v1/pages/100/tti", body, c, "If-Match", `"`+strings.Repeat("0", 64)+`"`)
	if rec.Code != http.StatusPreconditionFailed || !strings.Contains(rec.Body.String(), `"precondition_failed"`) {
		t.Fatalf("wrong etag: %d %s", rec.Code, rec.Body.String())
	}
	if e.count("SELECT count(*) FROM audit WHERE action = 'page.upload'") != 0 {
		t.Error("write happened on 412")
	}
	sum := sha256.Sum256(readFixture(t, "P100.tti"))
	rec = e.do("PUT", "/api/v1/pages/100/tti", body, c, "If-Match", `"`+hex.EncodeToString(sum[:])+`"`)
	if rec.Code != http.StatusOK {
		t.Fatalf("right etag: %d %s", rec.Code, rec.Body.String())
	}
}

func TestUploadGeneratedPage409(t *testing.T) {
	e := newEnv(t, false)
	e.importPage("P100.tti")
	e.addUser("conor", true)
	e.exec("UPDATE pages SET binding_id = 1")
	rec := e.do("PUT", "/api/v1/pages/100/tti", string(editedP100(t)), e.login("conor"))
	wantStatus(t, rec, http.StatusConflict, `{"error":{"code":"generated_page","message":"page is generated by a binding and cannot be edited"}}`)
}

func TestUploadMissingPage404(t *testing.T) {
	e := newEnv(t, false)
	e.addUser("conor", true)
	rec := e.do("PUT", "/api/v1/pages/100/tti", string(readFixture(t, "P100.tti")), e.login("conor"))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("got %d %s", rec.Code, rec.Body.String())
	}
}

// getDoc returns the GET document of page mpp as raw JSON.
func (e *env) getDoc(mpp string, c *http.Cookie) []byte {
	e.t.Helper()
	rec := e.do("GET", "/api/v1/pages/"+mpp, "", c)
	if rec.Code != http.StatusOK {
		e.t.Fatalf("GET %s: %d %s", mpp, rec.Code, rec.Body.String())
	}
	return rec.Body.Bytes()
}

// putDoc decodes doc, lets edit change it, and PUTs it back with the document's own etag.
func (e *env) putDoc(mpp string, doc []byte, c *http.Cookie, edit func(d map[string]any)) *httptest.ResponseRecorder {
	e.t.Helper()
	var d map[string]any
	if err := json.Unmarshal(doc, &d); err != nil {
		e.t.Fatal(err)
	}
	etag := d["etag"].(string)
	if edit != nil {
		edit(d)
	}
	b, _ := json.Marshal(d)
	return e.do("PUT", "/api/v1/pages/"+mpp, string(b), c, "If-Match", `"`+etag+`"`)
}

func (e *env) storedTTI(mpp int) []byte {
	e.t.Helper()
	p, err := e.db.Page(mpp)
	if err != nil {
		e.t.Fatal(err)
	}
	return p.TTI
}

func setRow(d map[string]any, sub, row int, v any) {
	d["subpages"].([]any)[sub].(map[string]any)["rows"].([]any)[row] = v
}

func TestGetPageDocument(t *testing.T) {
	e := newEnv(t, false)
	e.importPage("P100.tti")
	e.addUser("conor", true)
	var d struct {
		Description string `json:"description"`
		Subpages    []struct {
			CycleSeconds *int      `json:"cycleSeconds"`
			CycleMode    *string   `json:"cycleMode"`
			Links        []string  `json:"links"`
			Rows         []*string `json:"rows"`
		} `json:"subpages"`
	}
	if err := json.Unmarshal(e.getDoc("100", e.login("conor")), &d); err != nil {
		t.Fatal(err)
	}
	s := d.Subpages[0]
	if len(d.Subpages) != 7 || len(s.Rows) != 25 || s.Rows[1] == nil || !strings.HasPrefix(*s.Rows[1], "\x0d") ||
		s.CycleSeconds == nil || *s.CycleSeconds != 10 || *s.CycleMode != "T" || len(s.Links) != 6 || s.Links[0] != "120" {
		t.Errorf("document %+v", d)
	}
}

func TestUpdateUnchangedWritesNothing(t *testing.T) {
	// P121 row 4 holds raw 8-bit bytes (not valid UTF-8) that decode to cells but would re-encode differently.
	for _, f := range []struct{ file, mpp string }{{"P100.tti", "100"}, {"P121.tti", "121"}} {
		e := newEnv(t, false)
		e.importPage(f.file)
		e.addUser("conor", true)
		c := e.login("conor")
		e.exec("UPDATE pages SET updated_at = '2020-01-01T00:00:00Z'")
		rec := e.putDoc(f.mpp, e.getDoc(f.mpp, c), c, nil)
		if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"updatedAt":"2020-01-01T00:00:00Z"`) {
			t.Fatalf("%s: got %d %s", f.file, rec.Code, rec.Body.String())
		}
		if e.count("SELECT count(*) FROM audit WHERE action = 'page.update'")+e.count("SELECT count(*) FROM page_versions") != 0 {
			t.Errorf("%s: audit or version row written", f.file)
		}
		mpp, _ := strconv.ParseInt(f.mpp, 16, 32)
		if !bytes.Equal(e.storedTTI(int(mpp)), readFixture(t, f.file)) {
			t.Errorf("%s: bytes changed", f.file)
		}
	}
}

func TestUpdateOneRowChangesOneLine(t *testing.T) {
	e := newEnv(t, false)
	e.importPage("P100.tti")
	e.addUser("conor", true)
	c := e.login("conor")
	rec := e.putDoc("100", e.getDoc("100", c), c, func(d map[string]any) {
		setRow(d, 0, 3, "\x01"+strings.Repeat("x", 39))
	})
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), strings.Repeat("x", 39)) {
		t.Fatalf("got %d %s", rec.Code, rec.Body.String())
	}
	old, now := bytes.Split(readFixture(t, "P100.tti"), []byte("\n")), bytes.Split(e.storedTTI(0x100), []byte("\n"))
	diff := 0
	for i := range old {
		if !bytes.Equal(old[i], now[i]) {
			diff++
			if !bytes.HasPrefix(now[i], []byte("OL,3,\x1bA"+strings.Repeat("x", 39))) {
				t.Errorf("changed line %q", now[i])
			}
		}
	}
	if len(old) != len(now) || diff != 1 {
		t.Errorf("lines %d vs %d, changed %d", len(old), len(now), diff)
	}
	if e.count("SELECT count(*) FROM audit WHERE action = 'page.update'") != 1 {
		t.Error("want one page.update audit row")
	}
}

func TestUpdateRejectsBadRows(t *testing.T) {
	e := newEnv(t, false)
	e.importPage("P100.tti")
	e.addUser("conor", true)
	c := e.login("conor")
	doc := e.getDoc("100", c)
	rec := e.putDoc("100", doc, c, func(d map[string]any) { setRow(d, 0, 5, strings.Repeat("a", 41)) })
	wantStatus(t, rec, http.StatusBadRequest, `{"error":{"code":"invalid_page","message":"subpage 1 row 5: 41 cells (max 40)"}}`)
	rec = e.putDoc("100", doc, c, func(d map[string]any) { setRow(d, 1, 6, "café") })
	wantStatus(t, rec, http.StatusBadRequest, `{"error":{"code":"invalid_page","message":"subpage 2 row 6: contains a character outside 0-127"}}`)
	rec = e.putDoc("100", doc, c, func(d map[string]any) {
		d["subpages"].([]any)[0].(map[string]any)["links"] = []string{"199", "200", "30", "400", "8ff", "8ff"}
	})
	wantStatus(t, rec, http.StatusBadRequest, `{"error":{"code":"invalid_page","message":"subpage 1: link 3 must be three hex digits"}}`)
	if !bytes.Equal(e.storedTTI(0x100), readFixture(t, "P100.tti")) {
		t.Error("a rejected PUT changed the page")
	}
}

func TestUpdateStaleIfMatch412(t *testing.T) {
	e := newEnv(t, false)
	e.importPage("P100.tti")
	e.addUser("conor", true)
	c := e.login("conor")
	doc := e.getDoc("100", c)
	if rec := e.putDoc("100", doc, c, func(d map[string]any) { setRow(d, 0, 1, strings.Repeat("a", 40)) }); rec.Code != http.StatusOK {
		t.Fatal(rec.Body.String())
	}
	rec := e.putDoc("100", doc, c, func(d map[string]any) { setRow(d, 0, 2, strings.Repeat("b", 40)) })
	if rec.Code != http.StatusPreconditionFailed {
		t.Errorf("stale etag: %d %s", rec.Code, rec.Body.String())
	}
}

func TestUpdateSubpagesMetadataAndDescription(t *testing.T) {
	e := newEnv(t, false)
	e.importPage("P100.tti")
	e.importPage("P810.tti")
	e.addUser("conor", true)
	c := e.login("conor")
	rec := e.putDoc("100", e.getDoc("100", c), c, func(d map[string]any) {
		d["subpages"] = d["subpages"].([]any)[:3]
		d["description"] = "Edited"
		d["transmit"] = false
		s := d["subpages"].([]any)[0].(map[string]any)
		s["cycleSeconds"], s["cycleMode"] = 20, "C"
		s["links"] = []string{"111", "222", "333", "444", "5AA", "666"}
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("got %d %s", rec.Code, rec.Body.String())
	}
	p, err := tti.Parse(e.storedTTI(0x100))
	if err != nil || p.Subpages() != 3 || p.Description() != "Edited" || p.Transmit() {
		t.Fatalf("stored: err %v subpages %d desc %q transmit %v", err, p.Subpages(), p.Description(), p.Transmit())
	}
	var back struct {
		Subpages []struct {
			CycleSeconds int      `json:"cycleSeconds"`
			CycleMode    string   `json:"cycleMode"`
			Links        []string `json:"links"`
		} `json:"subpages"`
	}
	json.Unmarshal(rec.Body.Bytes(), &back)
	if s := back.Subpages[0]; s.CycleSeconds != 20 || s.CycleMode != "C" || strings.Join(s.Links, ",") != "111,222,333,444,5aa,666" {
		t.Errorf("read back %+v", s)
	}
	// adding subpages grows the page and keeps it parseable
	rec = e.putDoc("100", e.getDoc("100", c), c, func(d map[string]any) {
		sp := d["subpages"].([]any)
		d["subpages"] = append(sp, sp[0], sp[0])
	})
	if p, err := tti.Parse(e.storedTTI(0x100)); rec.Code != http.StatusOK || err != nil || p.Subpages() != 5 {
		t.Errorf("add: %d err %v", rec.Code, err)
	}
	// P810 has no DE: a description inserts one
	rec = e.putDoc("810", e.getDoc("810", c), c, func(d map[string]any) { d["description"] = "New" })
	if p, _ := tti.Parse(e.storedTTI(0x810)); rec.Code != http.StatusOK || p.Description() != "New" || string(p.Lines[0].Raw) != "DE,New" {
		t.Errorf("DE insert: %d", rec.Code)
	}
}

func TestUpdateGeneratedAndMissing(t *testing.T) {
	e := newEnv(t, false)
	e.importPage("P100.tti")
	e.addUser("conor", true)
	c := e.login("conor")
	doc := e.getDoc("100", c)
	e.exec("UPDATE pages SET binding_id = 1")
	wantStatus(t, e.putDoc("100", doc, c, func(d map[string]any) { setRow(d, 0, 1, strings.Repeat("a", 40)) }), http.StatusConflict,
		`{"error":{"code":"generated_page","message":"page is generated by a binding and cannot be edited"}}`)
	if rec := e.putDoc("101", doc, c, nil); rec.Code != http.StatusNotFound {
		t.Errorf("missing: %d", rec.Code)
	}
}

// auditCount counts audit rows of one action.
func (e *env) auditCount(action string) int {
	return e.count("SELECT count(*) FROM audit WHERE action = '" + action + "'")
}

func TestCreateAudits(t *testing.T) { // mutation H
	e := newEnv(t, false)
	id := e.addUser("conor", true)
	c := e.login("conor")
	rec := e.do("POST", "/api/v1/pages/2A0", "", c)
	if rec.Code != http.StatusCreated {
		t.Fatalf("got %d %s", rec.Code, rec.Body.String())
	}
	var d pageDoc
	if err := json.Unmarshal(rec.Body.Bytes(), &d); err != nil {
		t.Fatal(err)
	}
	stored := e.storedTTI(0x2A0)
	sum := sha256.Sum256(stored)
	if d.MPP != "2A0" || d.ETag != hex.EncodeToString(sum[:]) || len(d.Subpages) != 1 || d.Transmit {
		t.Errorf("response %+v", d)
	}
	p, err := tti.Parse(stored)
	if err != nil || !bytes.Equal(stored, tti.NewPage(0x2A0).Bytes()) || p.Subpages() != 1 {
		t.Errorf("stored bytes are not the blank page: %v", err)
	}
	q := "SELECT count(*) FROM audit WHERE action = 'page.create' AND target = 'page:2A0' AND actor = 'user:" + strconv.FormatInt(id, 10) + "' AND detail = 'sha256:" + hex.EncodeToString(sum[:]) + "'"
	if n := e.count(q); n != 1 || e.auditCount("page.create") != 1 {
		t.Errorf("audit rows matching %d, page.create rows %d", n, e.auditCount("page.create"))
	}
}

func TestCreateExisting409(t *testing.T) { // mutation C
	e := newEnv(t, false)
	e.importPage("P100.tti")
	e.addUser("conor", true)
	c := e.login("conor")
	before := e.storedTTI(0x100)
	wantStatus(t, e.do("POST", "/api/v1/pages/100", "", c), http.StatusConflict, `{"error":{"code":"exists","message":"page 100 already exists"}}`)
	if !bytes.Equal(before, e.storedTTI(0x100)) || e.auditCount("page.create") != 0 {
		t.Error("a refused create changed the page or wrote an audit row")
	}
}

func TestCreateOutsideMagazines404(t *testing.T) { // mutation D
	e := newEnv(t, false)
	e.addUser("conor", true)
	c := e.login("conor")
	for _, mpp := range []string{"900", "0FF", "zzz", "1000"} {
		wantStatus(t, e.do("POST", "/api/v1/pages/"+mpp, "", c), http.StatusNotFound, `{"error":{"code":"not_found","message":"no such page"}}`)
	}
	if n := e.count("SELECT count(*) FROM pages") + e.auditCount("page.create"); n != 0 {
		t.Errorf("%d rows written", n)
	}
}

func TestCreateAndDeleteNeedLogin(t *testing.T) { // criterion J
	e := newEnv(t, false)
	wantStatus(t, e.do("POST", "/api/v1/pages/100", "", nil), http.StatusUnauthorized, unauthorized)
	wantStatus(t, e.do("DELETE", "/api/v1/pages/100", "", nil, "If-Match", "x"), http.StatusUnauthorized, unauthorized)
}

// deleteEnv is an env with P100 stored, an admin logged in, and P100's etag.
func deleteEnv(t *testing.T) (e *env, c *http.Cookie, id int64, etag string) {
	e = newEnv(t, false)
	e.importPage("P100.tti")
	id = e.addUser("conor", true)
	c = e.login("conor")
	var d pageDoc
	if err := json.Unmarshal(e.getDoc("100", c), &d); err != nil {
		t.Fatal(err)
	}
	return e, c, id, d.ETag
}

func TestDeleteWithoutIfMatch428(t *testing.T) { // mutation A
	e, c, _, _ := deleteEnv(t)
	for _, h := range [][]string{nil, {"If-Match", ""}, {"If-Match", `""`}} {
		wantStatus(t, e.do("DELETE", "/api/v1/pages/100", "", c, h...), http.StatusPreconditionRequired,
			`{"error":{"code":"confirmation_required","message":"send If-Match with the page's etag to confirm deletion"}}`)
	}
	if e.count("SELECT count(*) FROM pages WHERE mpp = 256") != 1 || e.auditCount("page.delete") != 0 || e.count("SELECT count(*) FROM page_versions") != 0 {
		t.Error("a refused delete removed the page or wrote an audit or version row")
	}
}

func TestDeleteStaleIfMatch412(t *testing.T) { // mutation B
	e, c, _, _ := deleteEnv(t)
	rec := e.do("DELETE", "/api/v1/pages/100", "", c, "If-Match", `"`+strings.Repeat("0", 64)+`"`)
	if rec.Code != http.StatusPreconditionFailed {
		t.Fatalf("got %d %s", rec.Code, rec.Body.String())
	}
	if e.count("SELECT count(*) FROM pages WHERE mpp = 256") != 1 || e.auditCount("page.delete") != 0 || e.count("SELECT count(*) FROM page_versions") != 0 {
		t.Error("a stale delete removed the page or wrote an audit or version row")
	}
}

func TestDeleteAudits(t *testing.T) { // mutation E, and the happy path
	e, c, id, etag := deleteEnv(t)
	if rec := e.do("DELETE", "/api/v1/pages/100", "", c, "If-Match", `"`+etag+`"`); rec.Code != http.StatusNoContent || rec.Body.Len() != 0 {
		t.Fatalf("got %d %q", rec.Code, rec.Body.String())
	}
	wantStatus(t, e.do("GET", "/api/v1/pages/100", "", c), http.StatusNotFound, "")
	if n := e.auditCount("page.delete"); n != 1 {
		t.Fatalf("%d page.delete rows", n)
	}
	var actor, action, target, detail string
	if err := e.raw.QueryRow("SELECT actor, action, target, detail FROM audit WHERE action = 'page.delete'").Scan(&actor, &action, &target, &detail); err != nil {
		t.Fatal(err)
	}
	if actor != "user:"+strconv.FormatInt(id, 10) || target != "page:100" || detail != "sha256:"+etag {
		t.Errorf("audit row %q %q %q %q", actor, action, target, detail)
	}
}

func TestDeleteArchives(t *testing.T) { // mutation F
	e, c, _, etag := deleteEnv(t)
	want := e.storedTTI(0x100)
	e.do("DELETE", "/api/v1/pages/100", "", c, "If-Match", etag) // unquoted is accepted too
	var n int
	var got []byte
	if err := e.raw.QueryRow("SELECT count(*), coalesce(max(tti), x'') FROM page_versions WHERE mpp = 256").Scan(&n, &got); err != nil {
		t.Fatal(err)
	}
	if n != 1 || !bytes.Equal(got, want) {
		t.Errorf("%d version rows, bytes equal %v", n, bytes.Equal(got, want))
	}
}

func TestDeleteGenerated409(t *testing.T) { // mutation G
	e, c, _, etag := deleteEnv(t)
	e.exec("UPDATE pages SET binding_id = 1")
	wantStatus(t, e.do("DELETE", "/api/v1/pages/100", "", c, "If-Match", etag), http.StatusConflict,
		`{"error":{"code":"generated_page","message":"page is generated by a binding and cannot be edited"}}`)
	if e.count("SELECT count(*) FROM pages") != 1 || e.auditCount("page.delete") != 0 {
		t.Error("a refused delete changed something")
	}
}

func TestDeleteMissing404(t *testing.T) {
	e, c, _, etag := deleteEnv(t)
	wantStatus(t, e.do("DELETE", "/api/v1/pages/101", "", c, "If-Match", etag), http.StatusNotFound, "")
}

// boundP100 imports P100 and binds it to a real binding named pacman.
func boundP100(t *testing.T) (*env, *http.Cookie) {
	t.Helper()
	e := newEnv(t, false)
	e.importPage("P100.tti")
	e.addUser("conor", true)
	tid, err := e.db.CreateTemplate(&store.Template{Name: "scores", TTI: tti.NewPage(0x100).Bytes(), RowRules: "{}"}, store.Audit{Actor: "test"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.db.CreateBinding(&store.Binding{Name: "pacman", TemplateID: tid, MPP: 0x100, Source: "{}", Static: "{}"}, store.Audit{Actor: "test"}); err != nil {
		t.Fatal(err)
	}
	return e, e.login("conor")
}

func TestPutBoundPage409(t *testing.T) { // mutation E, with TestDeleteGenerated409
	e, c := boundP100(t)
	doc := e.getDoc("100", c)
	before := e.storedTTI(0x100)
	const generated = `{"error":{"code":"generated_page","message":"page is generated by a binding and cannot be edited"}}`
	wantStatus(t, e.putDoc("100", doc, c, func(d map[string]any) { setRow(d, 0, 1, strings.Repeat("a", 40)) }), http.StatusConflict, generated)
	wantStatus(t, e.do("PUT", "/api/v1/pages/100/tti", string(editedP100(t)), c), http.StatusConflict, generated)
	var d struct{ ETag string }
	json.Unmarshal(doc, &d)
	wantStatus(t, e.do("DELETE", "/api/v1/pages/100", "", c, "If-Match", d.ETag), http.StatusConflict, generated)
	if !bytes.Equal(e.storedTTI(0x100), before) || e.count("SELECT count(*) FROM page_versions") != 0 ||
		e.count("SELECT count(*) FROM audit WHERE action LIKE 'page.%'") != 0 {
		t.Error("a refused write changed something")
	}
}

func TestPageDocumentCarriesBinding(t *testing.T) {
	e, c := boundP100(t)
	if !strings.Contains(string(e.getDoc("100", c)), `"binding":{"id":1,"name":"pacman"}`) {
		t.Errorf("bound document %s", e.getDoc("100", c))
	}
	e.importPage("P101.tti")
	if !strings.Contains(string(e.getDoc("101", c)), `"binding":null`) {
		t.Errorf("unbound document %s", e.getDoc("101", c))
	}
}

func TestPageDocumentCanWrite(t *testing.T) {
	e := newEnv(t, false)
	e.importPage("P100.tti")
	canWrite := func(body []byte) bool {
		t.Helper()
		var d struct{ CanWrite *bool }
		if err := json.Unmarshal(body, &d); err != nil || d.CanWrite == nil {
			t.Fatalf("no canWrite in %s (%v)", body, err)
		}
		return *d.CanWrite
	}
	e.addUser("admin", true)
	writer, reader := e.addUser("writer", false), e.addUser("reader", false)
	e.grant(writer, 0x100, 0x100, true)
	e.grant(reader, 0x100, 0x100, false)
	wid, wkey := e.key(false)
	e.keyGrant(wid, 0x100, 0x100, true)
	rid, rkey := e.key(true)
	e.keyGrant(rid, 0x100, 0x100, true)
	for _, c := range []struct {
		name string
		body []byte
		want bool
	}{
		{"admin", e.getDoc("100", e.login("admin")), true},
		{"write grant", e.getDoc("100", e.login("writer")), true},
		{"read grant", e.getDoc("100", e.login("reader")), false},
		{"write key", e.bearer("GET", "/api/v1/pages/100", "", wkey).Body.Bytes(), true},
		{"read-only key", e.bearer("GET", "/api/v1/pages/100", "", rkey).Body.Bytes(), false},
	} {
		if got := canWrite(c.body); got != c.want {
			t.Errorf("%s: canWrite %v, want %v", c.name, got, c.want)
		}
	}
	c := e.login("admin")
	if !canWrite(e.do("PUT", "/api/v1/pages/100/tti", string(editedP100(t)), c).Body.Bytes()) {
		t.Error("upload response canWrite false")
	}
	if !canWrite(e.putDoc("100", e.getDoc("100", c), c, func(d map[string]any) { d["description"] = "x" }).Body.Bytes()) {
		t.Error("PUT response canWrite false")
	}
	if !canWrite(e.do("POST", "/api/v1/pages/2A0", "", c).Body.Bytes()) {
		t.Error("create response canWrite false")
	}
}
