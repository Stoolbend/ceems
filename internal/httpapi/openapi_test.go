package httpapi

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"math"
	"net/http"
	"net/http/httptest"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"

	"golang.org/x/time/rate"

	"ceems/internal/jobs"
)

// The contract test holds openapi.json to the routes and to the running server,
// both ways. Request and response bodies are never printed: several routes return
// a password or a token.

// openAPIOps returns the document's operations as "METHOD /api/v1<path>"; /healthz is
// the one path served outside the prefix.
func openAPIOps(t *testing.T) (map[string]any, map[string]map[string]any) {
	t.Helper()
	var doc map[string]any
	if err := json.Unmarshal(openAPIDoc, &doc); err != nil {
		t.Fatal(err)
	}
	ops := map[string]map[string]any{}
	paths, _ := doc["paths"].(map[string]any)
	for path, item := range paths {
		for method, op := range item.(map[string]any) {
			if m := strings.ToUpper(method); m == "GET" || m == "PUT" || m == "POST" || m == "DELETE" {
				ops[m+" "+path] = op.(map[string]any)
			}
		}
	}
	return doc, ops
}

func fullPattern(op string) string {
	method, path, _ := strings.Cut(op, " ")
	if path == "/healthz" {
		return op
	}
	return method + " /api/v1" + path
}

// Mutation A
func TestOpenAPICoversRoutes(t *testing.T) {
	_, ops := openAPIOps(t)
	documented := map[string]bool{}
	for op := range ops {
		documented[fullPattern(op)] = true
	}
	// The fixed exceptions: not registered through the route helpers.
	registered := map[string]bool{"GET /healthz": true, "POST /api/v1/login": true, "GET /api/v1/site": true}
	for _, rt := range newEnv(t, false).api.routes {
		registered[rt.pattern] = true
	}
	for p := range registered {
		if !documented[p] {
			t.Errorf("route %q is not in openapi.json", p)
		}
	}
	for p := range documented {
		if !registered[p] {
			t.Errorf("openapi.json documents %q, which is not a route", p)
		}
	}
}

// checkSchema is a coverage check, not a validator: type (including the 3.1 array
// form), required, properties with additionalProperties false, items, enum and $ref
// are checked; every other keyword is ignored. Errors name paths, never values.
func checkSchema(doc, schema map[string]any, v any, at string) error {
	if ref, ok := schema["$ref"].(string); ok {
		return checkSchema(doc, resolveRef(doc, ref), v, at)
	}
	if t, ok := schema["type"]; ok && !typeMatches(t, v) {
		return fmt.Errorf("%s: a %s is not of type %v", at, jsonType(v), t)
	}
	if enum, ok := schema["enum"].([]any); ok {
		found := false
		for _, e := range enum {
			found = found || e == v
		}
		if !found {
			return fmt.Errorf("%s: value is not in the enum", at)
		}
	}
	switch x := v.(type) {
	case map[string]any:
		props, _ := schema["properties"].(map[string]any)
		req, _ := schema["required"].([]any)
		for _, r := range req {
			if _, ok := x[r.(string)]; !ok {
				return fmt.Errorf("%s: missing required %q", at, r)
			}
		}
		for k, val := range x {
			ps, ok := props[k].(map[string]any)
			if !ok {
				if schema["additionalProperties"] == false {
					return fmt.Errorf("%s: undocumented property %q", at, k)
				}
				continue
			}
			if err := checkSchema(doc, ps, val, at+"."+k); err != nil {
				return err
			}
		}
	case []any:
		if items, ok := schema["items"].(map[string]any); ok {
			for i, e := range x {
				if err := checkSchema(doc, items, e, fmt.Sprintf("%s[%d]", at, i)); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func jsonType(v any) string {
	switch x := v.(type) {
	case nil:
		return "null"
	case bool:
		return "boolean"
	case float64:
		if x == math.Trunc(x) {
			return "integer"
		}
		return "number"
	case string:
		return "string"
	case []any:
		return "array"
	}
	return "object"
}

func typeMatches(t any, v any) bool {
	names, ok := t.([]any)
	if !ok {
		names = []any{t}
	}
	for _, n := range names {
		if got := jsonType(v); n == got || (n == "number" && got == "integer") {
			return true
		}
	}
	return false
}

func resolveRef(doc map[string]any, ref string) map[string]any {
	cur := doc
	for _, part := range strings.Split(strings.TrimPrefix(ref, "#/"), "/") {
		cur, _ = cur[part].(map[string]any)
	}
	return cur
}

func TestCheckSchemaCatchesDrift(t *testing.T) {
	doc := map[string]any{"components": map[string]any{"schemas": map[string]any{}}}
	var schema map[string]any
	json.Unmarshal([]byte(`{"type":"object","required":["a"],"additionalProperties":false,"properties":{"a":{"type":["string","null"]},"b":{"type":"array","items":{"enum":["x"]}}}}`), &schema)
	for body, ok := range map[string]bool{
		`{"a":"s"}`: true, `{"a":null,"b":["x"]}`: true,
		`{}`: false, `{"a":1}`: false, `{"a":"s","c":1}`: false, `{"a":"s","b":["y"]}`: false,
	} {
		var v any
		json.Unmarshal([]byte(body), &v)
		if err := checkSchema(doc, schema, v, "$"); (err == nil) != ok {
			t.Errorf("%s: err %v, want ok=%v", body, err, ok)
		}
	}
}

// syncBuf is a log sink the server goroutines write and the test reads.
type syncBuf struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (s *syncBuf) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.Write(p)
}

func (s *syncBuf) lines() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return strings.Split(strings.TrimSpace(s.b.String()), "\n")
}

// contract is a real server over a temp database with the §3 fixtures.
type contract struct {
	t       *testing.T
	e       *env
	srv     *httptest.Server
	doc     map[string]any
	ops     map[string]map[string]any
	logs    *syncBuf
	creds   map[string]func(*http.Request) // who -> how that caller authenticates
	roID    string
	rwID    string
	covered map[string]int
}

func newContract(t *testing.T) *contract {
	t.Helper()
	e := newEnv(t, false)
	e.importPage("P100.tti")
	e.importPage("P81A.tti")
	c := &contract{t: t, e: e, logs: &syncBuf{}, creds: map[string]func(*http.Request){}, covered: map[string]int{}}
	c.doc, c.ops = openAPIOps(t)

	log := slog.New(slog.NewTextHandler(c.logs, nil))
	h, a := newAPI(ui, log, e.db, false, jobs.New(e.db, okSource(), log), nil, "/data/pages")
	// One address sends every request, so lift the per-IP and per-key limits in the test only.
	a.apiIPLimit, a.keyLimit = newLimiter(rate.Inf, 1), newLimiter(rate.Inf, 1)
	c.srv = httptest.NewServer(h)
	t.Cleanup(c.srv.Close)

	e.addUser("admin", true)
	writer := e.addUser("writer", false)
	e.grant(writer, 0x100, 0x1FF, true)
	cookie := func(name string) func(*http.Request) {
		ck := e.login(name)
		return func(r *http.Request) { r.AddCookie(ck) }
	}
	c.creds["admin"], c.creds["writer"], c.creds["anon"] = cookie("admin"), cookie("writer"), func(*http.Request) {}
	var roAuth, rwAuth string
	c.roID, roAuth = e.key(true) // read-only, with a write grant, so only ReadOnly stops it
	e.keyGrant(c.roID, 0x100, 0x100, true)
	c.rwID, rwAuth = e.key(false)
	e.keyGrant(c.rwID, 0x100, 0x100, true)
	c.creds["ro"] = func(r *http.Request) { r.Header.Set("Authorization", roAuth) }
	c.creds["rw"] = func(r *http.Request) { r.Header.Set("Authorization", rwAuth) }
	return c
}

type reply struct {
	status int
	header http.Header
	body   []byte
	obj    any // the decoded JSON body, nil when there is none
}

// at walks nested objects by key; it returns nil on any miss.
func (r *reply) at(keys ...string) any {
	v := r.obj
	for _, k := range keys {
		m, _ := v.(map[string]any)
		v = m[k]
	}
	return v
}

func (r *reply) str(keys ...string) string { s, _ := r.at(keys...).(string); return s }
func (r *reply) num(keys ...string) int    { f, _ := r.at(keys...).(float64); return int(f) }

// send does a request as who without any contract checking.
func (c *contract) send(who, method, path, body string, hdr ...string) *reply {
	c.t.Helper()
	req, err := http.NewRequest(method, c.srv.URL+path, strings.NewReader(body))
	if err != nil {
		c.t.Fatal(err)
	}
	c.creds[who](req)
	for i := 0; i+1 < len(hdr); i += 2 {
		req.Header.Set(hdr[i], hdr[i+1])
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		c.t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	r := &reply{status: resp.StatusCode, header: resp.Header, body: b}
	if strings.HasPrefix(resp.Header.Get("Content-Type"), "application/json") {
		json.Unmarshal(b, &r.obj)
	}
	return r
}

// call is a scenario: op is the document's "METHOD /path" it exercises, path the real
// path under /api/v1. The status must be want and be documented for op, and a JSON body
// must match the documented schema.
func (c *contract) call(name, op, who, path, body string, want int, hdr ...string) *reply {
	c.t.Helper()
	var r *reply
	c.t.Run(name, func(t *testing.T) {
		method, docPath, _ := strings.Cut(op, " ")
		full := "/api/v1" + path
		if docPath == "/healthz" {
			full = path
		}
		r = c.send(who, method, full, body, hdr...)
		c.covered[op]++
		if r.status != want {
			t.Errorf("%s: got %d, want %d", op, r.status, want)
		}
		responses, _ := c.ops[op]["responses"].(map[string]any)
		resp, ok := responses[strconv.Itoa(r.status)].(map[string]any)
		if !ok {
			t.Errorf("%s: status %d is not documented", op, r.status)
			return
		}
		if ref, ok := resp["$ref"].(string); ok {
			resp = resolveRef(c.doc, ref)
		}
		content, _ := resp["content"].(map[string]any)
		if s, ok := content["application/json"].(map[string]any); ok && r.obj != nil {
			if err := checkSchema(c.doc, s["schema"].(map[string]any), r.obj, "$"); err != nil {
				t.Errorf("%s %d: %v", op, r.status, err)
			}
		} else if _, ok := content["application/octet-stream"]; ok {
			if ct := r.header.Get("Content-Type"); ct != "application/octet-stream" {
				t.Errorf("%s: Content-Type %q", op, ct)
			}
		}
	})
	return r
}

// Mutation F
func TestOpenAPIResponses(t *testing.T) {
	c := newContract(t)
	page := func(mpp string) string { return "/pages/" + mpp }

	// Session routes.
	c.call("login bad body", "POST /login", "anon", "/login", `{}`, 400)
	c.call("login wrong password", "POST /login", "anon", "/login", loginBody("admin", "wrong wrong wrong"), 401)
	c.call("login ok", "POST /login", "anon", "/login", loginBody("admin", testPassword), 200)
	c.call("me user", "GET /me", "admin", "/me", "", 200)
	c.call("me key", "GET /me", "ro", "/me", "", 200)
	c.call("me anonymous", "GET /me", "anon", "/me", "", 401)
	c.call("site", "GET /site", "anon", "/site", "", 200)
	c.call("healthz", "GET /healthz", "anon", "/healthz", "", 200)
	doc := c.call("openapi", "GET /openapi.json", "admin", "/openapi.json", "", 200)
	if !bytes.Equal(doc.body, openAPIDoc) {
		t.Error("GET /openapi.json does not serve the embedded document")
	}

	// Templates and bindings come first so page 4A0 is generated.
	tplBody := templateJSONBody("scores", twoSlotTTI(), `{}`)
	tpl := c.call("template create", "POST /templates", "admin", "/templates", tplBody, 201)
	tid := tpl.num("id")
	c.call("template duplicate", "POST /templates", "admin", "/templates", tplBody, 409)
	c.call("template bad body", "POST /templates", "admin", "/templates", `{}`, 400)
	c.call("template too large", "POST /templates", "admin", "/templates", strings.Repeat("x", 300<<10), 413)
	c.call("template key 403", "GET /templates", "rw", "/templates", "", 403)
	c.call("template list", "GET /templates", "admin", "/templates", "", 200)
	c.call("template get", "GET /templates/{id}", "admin", "/templates/"+strconv.Itoa(tid), "", 200)
	c.call("template get missing", "GET /templates/{id}", "admin", "/templates/99", "", 404)
	c.call("template put", "PUT /templates/{id}", "admin", "/templates/"+strconv.Itoa(tid), tplBody, 200)
	c.call("template put bad", "PUT /templates/{id}", "admin", "/templates/"+strconv.Itoa(tid), `{}`, 400)
	c.call("template tti", "GET /templates/{id}/tti", "admin", "/templates/"+strconv.Itoa(tid)+"/tti", "", 200)
	spare := c.call("template spare", "POST /templates", "admin", "/templates", templateJSONBody("spare", twoSlotTTI(), `{}`), 201)

	bind := c.call("binding create", "POST /bindings", "admin", "/bindings", bindingJSON("pacman", tid, "4A0", testSourceJSON, testStatic), 201)
	bid := strconv.Itoa(bind.num("id"))
	c.call("binding duplicate", "POST /bindings", "admin", "/bindings", bindingJSON("pacman", tid, "4A0", testSourceJSON, testStatic), 409)
	c.call("binding bad body", "POST /bindings", "admin", "/bindings", `{}`, 400)
	c.call("binding list", "GET /bindings", "admin", "/bindings", "", 200)
	c.call("binding get", "GET /bindings/{id}", "admin", "/bindings/"+bid, "", 200)
	c.call("binding get missing", "GET /bindings/{id}", "admin", "/bindings/99", "", 404)
	c.call("binding put", "PUT /bindings/{id}", "admin", "/bindings/"+bid, bindingJSON("pacman2", tid, "4A0", testSourceJSON, testStatic), 200)
	if m := c.call("binding move", "PUT /bindings/{id}", "admin", "/bindings/"+bid, bindingJSON("pacman2", tid, "4A1", testSourceJSON, testStatic), 200); m.str("mpp") != "4A1" {
		t.Errorf("moved binding mpp %q", m.str("mpp"))
	}
	c.call("binding move target", "POST /pages/{mpp}", "admin", page("4A2"), "", 201)
	c.call("binding move taken", "PUT /bindings/{id}", "admin", "/bindings/"+bid, bindingJSON("pacman2", tid, "4A2", testSourceJSON, testStatic), 409)
	c.call("binding move back", "PUT /bindings/{id}", "admin", "/bindings/"+bid, bindingJSON("pacman2", tid, "4A0", testSourceJSON, testStatic), 200) // later cases need 4A0 generated
	c.call("binding put bad", "PUT /bindings/{id}", "admin", "/bindings/"+bid, `{}`, 400)
	c.call("binding run", "POST /bindings/{id}/run", "admin", "/bindings/"+bid+"/run", "", 200)
	c.call("binding run missing", "POST /bindings/{id}/run", "admin", "/bindings/99/run", "", 404)
	c.call("binding runs", "GET /bindings/{id}/runs", "admin", "/bindings/"+bid+"/runs", "", 200)
	c.call("binding runs missing", "GET /bindings/{id}/runs", "admin", "/bindings/99/runs", "", 404)
	c.call("template in use", "DELETE /templates/{id}", "admin", "/templates/"+strconv.Itoa(tid), "", 409)
	c.call("template delete spare", "DELETE /templates/{id}", "admin", "/templates/"+strconv.Itoa(spare.num("id")), "", 204)
	c.call("template delete missing", "DELETE /templates/{id}", "admin", "/templates/99", "", 404)

	// Pages.
	c.call("pages list admin", "GET /pages", "admin", "/pages", "", 200)
	c.call("pages list anonymous", "GET /pages", "anon", "/pages", "", 401)
	list := c.call("key list", "GET /pages", "ro", "/pages", "", 200)
	if ps, _ := list.at("pages").([]any); len(ps) != 1 || ps[0].(map[string]any)["mpp"] != "100" {
		t.Errorf("a key scoped to 100 lists %d pages", len(ps))
	}
	p100 := c.call("page get", "GET /pages/{mpp}", "admin", page("100"), "", 200)
	etag := p100.str("etag")
	c.call("key get 100", "GET /pages/{mpp}", "ro", page("100"), "", 200)
	c.call("key get 81A 404", "GET /pages/{mpp}", "ro", page("81A"), "", 404)
	c.call("page get out of range", "GET /pages/{mpp}", "admin", page("0FF"), "", 404)
	c.call("page tti", "GET /pages/{mpp}/tti", "admin", page("100")+"/tti", "", 200)
	c.call("key tti 81A 404", "GET /pages/{mpp}/tti", "ro", page("81A")+"/tti", "", 404)

	edited, _ := json.Marshal(mutate(p100.obj, "description", "contract test"))
	c.call("readonly-key put 403", "PUT /pages/{mpp}", "ro", page("100"), string(edited), 403)
	c.call("key put 81A 404", "PUT /pages/{mpp}", "rw", page("81A"), string(edited), 404)
	c.call("page put stale", "PUT /pages/{mpp}", "rw", page("100"), string(edited), 412, "If-Match", `"0000"`)
	c.call("page put bad body", "PUT /pages/{mpp}", "rw", page("100"), `not json`, 400)
	c.call("page put too large", "PUT /pages/{mpp}", "rw", page("100"), strings.Repeat("x", 300<<10), 413)
	c.call("page put generated", "PUT /pages/{mpp}", "admin", page("4A0"), string(edited), 409)
	put := c.call("page put", "PUT /pages/{mpp}", "rw", page("100"), string(edited), 200, "If-Match", `"`+etag+`"`)
	if put.str("etag") == etag || put.str("description") != "contract test" {
		t.Error("PUT /pages/100 did not change the page")
	}

	c.call("readonly-key put tti 403", "PUT /pages/{mpp}/tti", "ro", page("100")+"/tti", string(editedP100(t)), 403)
	c.call("page put tti bad", "PUT /pages/{mpp}/tti", "writer", page("100")+"/tti", "junk", 400)
	c.call("page put tti stale", "PUT /pages/{mpp}/tti", "writer", page("100")+"/tti", string(editedP100(t)), 412, "If-Match", `"0000"`)
	c.call("page put tti too large", "PUT /pages/{mpp}/tti", "writer", page("100")+"/tti", strings.Repeat("x", 300<<10), 413)
	c.call("page put tti", "PUT /pages/{mpp}/tti", "writer", page("100")+"/tti", string(editedP100(t)), 200)

	c.call("readonly-key post 403", "POST /pages/{mpp}", "ro", page("100"), "", 403)
	c.call("key post out of range", "POST /pages/{mpp}", "rw", page("101"), "", 404)
	c.call("page post", "POST /pages/{mpp}", "writer", page("101"), "", 201)
	c.call("page post exists", "POST /pages/{mpp}", "writer", page("101"), "", 409)
	c.call("readonly-key delete 403", "DELETE /pages/{mpp}", "ro", page("100"), "", 403)
	c.call("page delete unconfirmed", "DELETE /pages/{mpp}", "writer", page("101"), "", 428)
	c.call("page delete stale", "DELETE /pages/{mpp}", "writer", page("101"), "", 412, "If-Match", `"0000"`)
	e101 := c.call("page get 101", "GET /pages/{mpp}", "writer", page("101"), "", 200).str("etag")
	c.call("page delete", "DELETE /pages/{mpp}", "writer", page("101"), "", 204, "If-Match", e101)

	c.call("binding delete", "DELETE /bindings/{id}", "admin", "/bindings/"+bid, "", 204)
	c.call("binding delete missing", "DELETE /bindings/{id}", "admin", "/bindings/"+bid, "", 404)

	// Users, grants, passwords, logout.
	c.call("settings key 403", "GET /settings", "rw", "/settings", "", 403)
	c.call("settings get", "GET /settings", "admin", "/settings", "", 200)
	c.call("settings put", "PUT /settings", "admin", "/settings", goodSettings(nil), 200)
	c.call("settings put bad", "PUT /settings", "admin", "/settings", `{}`, 400)
	c.call("settings put too large", "PUT /settings", "admin", "/settings", strings.Repeat("x", 5000), 413)
	// Fastext groups.
	fx := func(name string, paged bool, links string, slots string) string {
		return fmt.Sprintf(`{"name":%q,"paged":%v,"links":%s,%s"default":false}`, name, paged, links, slots)
	}
	const fxLinks = `["101","102","103","104"]`
	c.call("fastext key 403", "GET /fastext-groups", "rw", "/fastext-groups", "", 403)
	c.call("fastext non-admin 403", "GET /fastext-groups", "writer", "/fastext-groups", "", 403)
	c.call("fastext bad body", "POST /fastext-groups", "admin", "/fastext-groups", `{}`, 400)
	c.call("fastext static with slots", "POST /fastext-groups", "admin", "/fastext-groups", fx("bad", false, fxLinks, `"prevSlot":0,"nextSlot":1,`), 400)
	c.call("fastext too large", "POST /fastext-groups", "admin", "/fastext-groups", strings.Repeat("x", 300<<10), 413)
	fg := c.call("fastext create", "POST /fastext-groups", "admin", "/fastext-groups", fx("menu", true, fxLinks, `"prevSlot":0,"nextSlot":1,`), 201)
	fid := "/fastext-groups/" + strconv.Itoa(fg.num("id"))
	c.call("fastext duplicate", "POST /fastext-groups", "admin", "/fastext-groups", fx("menu", false, fxLinks, ""), 409)
	other := c.call("fastext second", "POST /fastext-groups", "admin", "/fastext-groups", fx("other", false, fxLinks, ""), 201)
	c.call("fastext list", "GET /fastext-groups", "admin", "/fastext-groups", "", 200)
	c.call("fastext get", "GET /fastext-groups/{id}", "admin", fid, "", 200)
	c.call("fastext get missing", "GET /fastext-groups/{id}", "admin", "/fastext-groups/99", "", 404)
	c.call("fastext put", "PUT /fastext-groups/{id}", "admin", fid, fx("menu", true, fxLinks, `"prevSlot":2,"nextSlot":3,`), 200)
	c.call("fastext put bad", "PUT /fastext-groups/{id}", "admin", fid, fx("menu", true, fxLinks, `"prevSlot":2,"nextSlot":2,`), 400)
	c.call("fastext put missing", "PUT /fastext-groups/{id}", "admin", "/fastext-groups/99", fx("menu", false, fxLinks, ""), 404)
	c.call("fastext put duplicate", "PUT /fastext-groups/{id}", "admin", fid, fx("other", true, fxLinks, `"prevSlot":2,"nextSlot":3,`), 409)
	c.call("fastext pages", "PUT /fastext-groups/{id}/pages", "admin", fid+"/pages", `{"pages":["100","81A"]}`, 200)
	if g := c.call("page in a group", "GET /pages/{mpp}", "admin", page("100"), "", 200); g.str("fastextGroup", "name") != "menu" {
		t.Error("a member page does not name its group")
	}
	c.call("fastext pages bad", "PUT /fastext-groups/{id}/pages", "admin", fid+"/pages", `{"pages":["10"]}`, 400)
	c.call("fastext pages unknown", "PUT /fastext-groups/{id}/pages", "admin", fid+"/pages", `{"pages":["7FF"]}`, 404)
	c.call("fastext pages in other group", "PUT /fastext-groups/{id}/pages", "admin", "/fastext-groups/"+strconv.Itoa(other.num("id"))+"/pages", `{"pages":["100"]}`, 409)
	c.call("fastext pages missing group", "PUT /fastext-groups/{id}/pages", "admin", "/fastext-groups/99/pages", `{"pages":[]}`, 404)
	c.call("fastext delete", "DELETE /fastext-groups/{id}", "admin", fid, "", 204)
	c.call("fastext delete missing", "DELETE /fastext-groups/{id}", "admin", fid, "", 404)

	c.call("users non-admin 403", "GET /users", "writer", "/users", "", 403)
	c.call("users list", "GET /users", "admin", "/users", "", 200)
	c.call("user bad body", "POST /users", "admin", "/users", `{"username":"!"}`, 400)
	temp := c.call("user create", "POST /users", "admin", "/users", `{"username":"temp"}`, 201)
	c.call("user duplicate", "POST /users", "admin", "/users", `{"username":"temp"}`, 409)
	uid := strconv.Itoa(temp.num("user", "id"))
	c.call("user disable", "PUT /users/{id}", "admin", "/users/"+uid, `{"disabled":true}`, 200)
	c.call("user enable", "PUT /users/{id}", "admin", "/users/"+uid, `{"disabled":false}`, 200)
	c.call("user bad update", "PUT /users/{id}", "admin", "/users/"+uid, `{}`, 400)
	c.call("user missing", "PUT /users/{id}", "admin", "/users/999", `{"disabled":true}`, 404)
	c.call("user disable self", "PUT /users/{id}", "admin", "/users/1", `{"disabled":true}`, 409)
	grant := c.call("user grant", "POST /users/{id}/grants", "admin", "/users/"+uid+"/grants", `{"lo":"100","hi":"1FF","access":"read"}`, 201)
	c.call("user grant bad", "POST /users/{id}/grants", "admin", "/users/"+uid+"/grants", `{"lo":"200","hi":"100","access":"read"}`, 400)
	c.call("user grant missing user", "POST /users/{id}/grants", "admin", "/users/999/grants", `{"lo":"100","hi":"1FF","access":"read"}`, 404)
	c.call("grant delete", "DELETE /grants/{id}", "admin", "/grants/"+strconv.Itoa(grant.num("id")), "", 204)
	c.call("grant delete missing", "DELETE /grants/{id}", "admin", "/grants/999", "", 404)

	// The new user's own password change and logout, on the server's real cookie path.
	tempLogin := func(pw string) {
		r := c.send("anon", "POST", "/api/v1/login", loginBody("temp", pw))
		if r.status != 200 {
			t.Fatalf("login as temp: %d", r.status)
		}
		cookie := (&http.Response{Header: r.header}).Cookies()[0]
		c.creds["temp"] = func(req *http.Request) { req.AddCookie(cookie) }
	}
	const newPassword = "a different long password"
	tempLogin(temp.str("password"))
	c.call("password key 403", "PUT /me/password", "rw", "/me/password", `{"currentPassword":"x","newPassword":"y"}`, 403)
	c.call("password bad body", "PUT /me/password", "temp", "/me/password", `{}`, 400)
	c.call("password wrong", "PUT /me/password", "temp", "/me/password", fmt.Sprintf(`{"currentPassword":"nope nope nope","newPassword":%q}`, newPassword), 400)
	c.call("password change", "PUT /me/password", "temp", "/me/password", fmt.Sprintf(`{"currentPassword":%q,"newPassword":%q}`, temp.str("password"), newPassword), 204)
	c.call("session ended by password change", "GET /me", "temp", "/me", "", 401)
	tempLogin(newPassword)
	c.call("logout key 403", "POST /logout", "rw", "/logout", "", 403)
	c.call("logout anonymous", "POST /logout", "anon", "/logout", "", 401)
	c.call("logout", "POST /logout", "temp", "/logout", "", 204)
	reset := c.call("user reset password", "POST /users/{id}/reset-password", "admin", "/users/"+uid+"/reset-password", "", 200)
	if reset.str("password") == "" {
		t.Error("reset returned no password")
	}
	c.call("user reset missing", "POST /users/{id}/reset-password", "admin", "/users/999/reset-password", "", 404)

	// Keys.
	c.call("key bad name", "POST /keys", "admin", "/keys", `{"name":""}`, 400)
	k := c.call("key create", "POST /keys", "admin", "/keys", `{"name":"sheet sync","readOnly":true}`, 201)
	kid := k.str("key", "id")
	c.call("keys list", "GET /keys", "admin", "/keys", "", 200)
	c.call("keys as key 403", "GET /keys", "rw", "/keys", "", 403)
	c.call("key grant", "POST /keys/{id}/grants", "admin", "/keys/"+kid+"/grants", `{"lo":"100","hi":"100","access":"read"}`, 201)
	c.call("key grant bad", "POST /keys/{id}/grants", "admin", "/keys/"+kid+"/grants", `{}`, 400)
	c.call("key grant missing key", "POST /keys/{id}/grants", "admin", "/keys/0123456789abcdef/grants", `{"lo":"100","hi":"100","access":"read"}`, 404)
	c.call("key revoke", "DELETE /keys/{id}", "admin", "/keys/"+kid, "", 204)
	c.call("key revoke missing", "DELETE /keys/{id}", "admin", "/keys/0123456789abcdef", "", 404)
	c.call("key grant revoked", "POST /keys/{id}/grants", "admin", "/keys/"+kid+"/grants", `{"lo":"100","hi":"100","access":"read"}`, 409)

	var missing []string
	for op := range c.ops {
		if c.covered[op] == 0 {
			missing = append(missing, op)
		}
	}
	sort.Strings(missing)
	if len(missing) > 0 {
		t.Errorf("operations with no scenario: %s", strings.Join(missing, ", "))
	}
}

// mutate returns a copy of the JSON object v with key set.
func mutate(v any, key string, val any) map[string]any {
	out := map[string]any{}
	for k, x := range v.(map[string]any) {
		out[k] = x
	}
	out[key] = val
	return out
}

// Mutation D
func TestOutOfRangeWriteLogged(t *testing.T) {
	c := newContract(t)
	doc := c.send("rw", "GET", "/api/v1/pages/100", "")
	body, _ := json.Marshal(mutate(doc.obj, "description", "in range"))
	audits := c.e.count("SELECT count(*) FROM audit")
	byKey := fmt.Sprintf("SELECT count(*) FROM audit WHERE actor = 'key:%s'", c.rwID)

	if r := c.send("rw", "PUT", "/api/v1/pages/81A", string(body)); r.status != 404 {
		t.Fatalf("out-of-range PUT: got %d, want 404", r.status)
	}
	var lines []string
	for _, l := range c.logs.lines() {
		if strings.Contains(l, `route="PUT /api/v1/pages/{mpp}"`) {
			lines = append(lines, l)
		}
	}
	if len(lines) != 1 || !strings.Contains(lines[0], "status=404") || !strings.Contains(lines[0], "actor=key:"+c.rwID) {
		t.Fatalf("want one PUT request line with status=404 and actor=key:%s, got %d: %v", c.rwID, len(lines), lines)
	}
	if n := c.e.count("SELECT count(*) FROM audit"); n != audits {
		t.Fatalf("a refused write added %d audit rows", n-audits)
	}

	if r := c.send("rw", "PUT", "/api/v1/pages/100", string(body)); r.status != 200 {
		t.Fatalf("in-range PUT: got %d, want 200", r.status)
	}
	if n := c.e.count(byKey + " AND action = 'page.update'"); n != 1 {
		t.Fatalf("audit rows naming the key %d, want 1", n)
	}
}
