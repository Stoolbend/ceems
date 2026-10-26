package httpapi

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"golang.org/x/time/rate"

	"ceems/internal/auth"
	"ceems/internal/store"
)

// Tokens are only ever sent and compared; no test prints one or a body that holds one.

// keyWith stores a key directly and returns its id and Authorization header value.
func (e *env) keyWith(k store.APIKey) (string, string) {
	e.t.Helper()
	id, token, hash := auth.NewKey()
	k.ID = id
	if k.Name == "" {
		k.Name = "test key"
	}
	if k.CreatedBy == "" {
		k.CreatedBy, k.CreatedAt = "test", "2026-01-01T00:00:00Z"
	}
	if err := e.db.CreateKey(k, hash, store.Audit{Actor: "test"}); err != nil {
		e.t.Fatal(err)
	}
	return id, "Bearer " + token
}

func (e *env) key(readOnly bool) (string, string) { return e.keyWith(store.APIKey{ReadOnly: readOnly}) }

// keyGrant gives key id a grant directly in the store.
func (e *env) keyGrant(id string, lo, hi int, write bool) {
	e.t.Helper()
	if _, err := e.db.CreateGrant(store.Grant{KeyID: id, Lo: lo, Hi: hi, Write: write}, store.Audit{Actor: "test", Action: "grant.create"}); err != nil {
		e.t.Fatal(err)
	}
}

// bearer does a request with the Authorization header set and no cookie.
func (e *env) bearer(method, path, body, authz string, headers ...string) *httptest.ResponseRecorder {
	e.t.Helper()
	return e.do(method, path, body, nil, append([]string{"Authorization", authz}, headers...)...)
}

// createKeyAPI creates a key through the API as admin and returns its document and token.
func (e *env) createKeyAPI(admin *http.Cookie, body string) (keyDoc, string) {
	e.t.Helper()
	rec := e.do("POST", "/api/v1/keys", body, admin)
	if rec.Code != http.StatusCreated {
		e.t.Fatalf("create key: %d", rec.Code)
	}
	var out struct {
		Key   keyDoc
		Token string
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		e.t.Fatal(err)
	}
	return out.Key, out.Token
}

const adminOnly = `{"error":{"code":"forbidden","message":"admin only"}}`

// Mutation A
func TestKeyWrongSecret401(t *testing.T) {
	e := newEnv(t, false)
	e.importPage("P100.tti")
	id, good := e.key(false)
	e.keyGrant(id, 0x100, 0x100, false)
	wantStatus(t, e.bearer("GET", "/api/v1/pages/100", "", good), http.StatusOK, "")
	_, other := e.key(false) // a well-formed secret that belongs to another key
	wrong := "Bearer yfk_" + id + "_" + other[len("Bearer yfk_")+17:]
	cases := []string{
		wrong,
		"Bearer yfk_0123456789abcdef_" + other[len("Bearer yfk_")+17:], // unknown id
		strings.TrimPrefix(good, "Bearer "),                            // no scheme
		"Basic " + strings.TrimPrefix(good, "Bearer "),
		good[:len(good)-1], // short secret
		good + " ",
		"Bearer junk",
	}
	for i, h := range cases {
		if rec := e.bearer("GET", "/api/v1/pages/100", "", h); rec.Code != http.StatusUnauthorized || strings.TrimSpace(rec.Body.String()) != unauthorized {
			t.Errorf("case %d: got %d, want 401 unauthorized", i, rec.Code)
		}
	}
}

// Mutation B
func TestRevokedKey401(t *testing.T) {
	e := newEnv(t, false)
	e.importPage("P100.tti")
	e.addUser("conor", true)
	admin := e.login("conor")
	id, b := e.key(false)
	e.keyGrant(id, 0x100, 0x100, false)
	wantStatus(t, e.bearer("GET", "/api/v1/pages/100", "", b), http.StatusOK, "")
	wantStatus(t, e.do("DELETE", "/api/v1/keys/"+id, "", admin), http.StatusNoContent, "")
	wantStatus(t, e.bearer("GET", "/api/v1/pages/100", "", b), http.StatusUnauthorized, unauthorized)
	wantStatus(t, e.do("DELETE", "/api/v1/keys/"+id, "", admin), http.StatusNoContent, "")
	if n := e.count("SELECT count(*) FROM audit WHERE action = 'key.revoke'"); n != 1 {
		t.Fatalf("key.revoke audit rows %d, want 1", n)
	}
	if e.count("SELECT count(*) FROM api_keys WHERE revoked_at IS NOT NULL") != 1 || e.count("SELECT count(*) FROM grants WHERE key_id IS NOT NULL") != 1 {
		t.Fatal("revoke did not keep the key row and its grant")
	}
	wantStatus(t, e.do("POST", "/api/v1/keys/"+id+"/grants", `{"lo":"100","hi":"100","access":"read"}`, admin), http.StatusConflict,
		`{"error":{"code":"revoked","message":"the key is revoked"}}`)
	for _, path := range []string{"/api/v1/keys/0123456789abcdef", "/api/v1/keys/nothex"} {
		wantStatus(t, e.do("DELETE", path, "", admin), http.StatusNotFound, `{"error":{"code":"not_found","message":"no such key"}}`)
	}
}

// Mutation C
func TestExpiredKey401(t *testing.T) {
	e := newEnv(t, false)
	e.importPage("P100.tti")
	past, b := e.keyWith(store.APIKey{ExpiresAt: time.Now().UTC().Add(-time.Second).Format(time.RFC3339)})
	e.keyGrant(past, 0x100, 0x100, false)
	wantStatus(t, e.bearer("GET", "/api/v1/pages/100", "", b), http.StatusUnauthorized, unauthorized)
	future, b2 := e.keyWith(store.APIKey{ExpiresAt: time.Now().UTC().Add(time.Hour).Format(time.RFC3339)})
	e.keyGrant(future, 0x100, 0x100, false)
	wantStatus(t, e.bearer("GET", "/api/v1/pages/100", "", b2), http.StatusOK, "")
}

// Mutation D
func TestReadOnlyKeyWrite403(t *testing.T) {
	e := newEnv(t, false)
	e.importPage("P100.tti")
	id, b := e.key(true)
	e.keyGrant(id, 0x100, 0x100, true)
	wantStatus(t, e.bearer("GET", "/api/v1/pages/100", "", b), http.StatusOK, "")
	wantStatus(t, e.bearer("DELETE", "/api/v1/pages/100", "", b), http.StatusForbidden,
		`{"error":{"code":"forbidden","message":"no write access to this page"}}`)
	if e.count("SELECT count(*) FROM pages") != 1 {
		t.Fatal("a read-only key deleted a page")
	}
}

// Mutation E: every registered admin route refuses a key, even one granted every page.
func TestKeyAdminRoutes403(t *testing.T) {
	e := newEnv(t, false)
	e.addUser("conor", true)
	id, b := e.key(false)
	e.keyGrant(id, 0x100, 0x8FF, true)
	e.api.keyLimit = newLimiter(rate.Inf, 1) // this test sends more requests than one key's burst
	n := 0
	for _, rt := range e.api.routes {
		if rt.kind != "admin" {
			continue
		}
		n++
		method, path := routePath(rt.pattern, map[string]string{"id": id})
		body := `{"username":"x","admin":true,"disabled":true,"lo":"100","hi":"100","access":"write","name":"x"}`
		// Bodies are not printed: a wrongly allowed create would carry a secret.
		if rec := e.bearer(method, path, body, b); rec.Code != http.StatusForbidden || strings.TrimSpace(rec.Body.String()) != adminOnly {
			t.Errorf("%s as a key: got %d, want 403 admin only", rt.pattern, rec.Code)
		}
	}
	if n < 23 {
		t.Fatalf("only %d admin routes in the registry", n)
	}
	if e.count("SELECT count(*) FROM api_keys WHERE revoked_at IS NULL") != 1 || e.count("SELECT count(*) FROM users") != 1 {
		t.Fatal("a refused key request changed keys or users")
	}
}

// Mutation F: with Authorization present the cookie is never used, valid header or not.
func TestBearerIgnoresCookie(t *testing.T) {
	e := newEnv(t, false)
	e.importPage("P100.tti")
	e.addUser("conor", true)
	admin := e.login("conor")
	id, b := e.key(false)
	for _, h := range []string{"Bearer junk", "Bearer yfk_" + id + "_" + strings.Repeat("A", 43), "Basic eDp5"} {
		wantStatus(t, e.do("GET", "/api/v1/users", "", admin, "Authorization", h), http.StatusUnauthorized, unauthorized)
		wantStatus(t, e.do("GET", "/api/v1/pages/100", "", admin, "Authorization", h), http.StatusUnauthorized, unauthorized)
	}
	wantStatus(t, e.do("GET", "/api/v1/users", "", admin, "Authorization", b), http.StatusForbidden, adminOnly)
	wantStatus(t, e.do("GET", "/api/v1/pages/100", "", admin, "Authorization", b), http.StatusNotFound, noSuchPage)
	wantStatus(t, e.do("GET", "/api/v1/me", "", admin, "Authorization", b), http.StatusOK,
		fmt.Sprintf(`{"key":{"id":%q,"name":"test key","readOnly":false},"user":null}`, id))
}

// G: the stored hash, presented in a secret's place, is not a credential.
func TestStoredHashNotABearer(t *testing.T) {
	e := newEnv(t, false)
	e.importPage("P100.tti")
	id, b := e.key(false)
	e.keyGrant(id, 0x100, 0x100, false)
	var hash []byte
	if err := e.raw.QueryRow("SELECT secret_hash FROM api_keys WHERE id = ?", id).Scan(&hash); err != nil {
		t.Fatal(err)
	}
	for _, s := range []string{base64.RawURLEncoding.EncodeToString(hash), fmt.Sprintf("%x", hash)} {
		wantStatus(t, e.bearer("GET", "/api/v1/pages/100", "", "Bearer yfk_"+id+"_"+s), http.StatusUnauthorized, unauthorized)
	}
	wantStatus(t, e.bearer("GET", "/api/v1/pages/100", "", b), http.StatusOK, "")
}

// Mutation H: a key sees only its own grants, not its creator's or any other user's.
func TestKeyGrantsScoped(t *testing.T) {
	e := newEnv(t, false)
	importAll(e)
	uid := e.addUser("viewer", false)
	e.grant(uid, 0x100, 0x100, true)
	id, b := e.keyWith(store.APIKey{CreatedBy: fmt.Sprintf("user:%d", uid), CreatedAt: "2026-01-01T00:00:00Z"})
	e.keyGrant(id, 0x101, 0x101, false)
	wantStatus(t, e.bearer("GET", "/api/v1/pages/100", "", b), http.StatusNotFound, noSuchPage)
	wantStatus(t, e.bearer("GET", "/api/v1/pages/101", "", b), http.StatusOK, "")
	rec := e.bearer("GET", "/api/v1/pages", "", b)
	var got listBody
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil || rec.Code != http.StatusOK || len(got.Pages) != 1 || got.Pages[0].MPP != "101" {
		t.Fatalf("key list: %d %s", rec.Code, rec.Body.String())
	}
}

const tooMany = `{"error":{"code":"rate_limited","message":"too many requests"}}`

// Mutation I
func TestKeyRateLimited429(t *testing.T) {
	e := newEnv(t, false)
	_, b := e.key(false)
	_, b2 := e.key(false)
	for i := range 20 {
		if rec := e.bearer("GET", "/api/v1/me", "", b, "RemoteAddr", fmt.Sprintf("198.51.100.%d:1", i)); rec.Code != http.StatusOK {
			t.Fatalf("request %d: got %d", i+1, rec.Code)
		}
	}
	wantStatus(t, e.bearer("GET", "/api/v1/me", "", b, "RemoteAddr", "198.51.100.99:1"), http.StatusTooManyRequests, tooMany)
	wantStatus(t, e.bearer("GET", "/api/v1/me", "", b2), http.StatusOK, "")
}

// Mutation J: the per-IP limit covers every request behind requireSession, anonymous ones too.
func TestAPIRateLimitedPerIP(t *testing.T) {
	e := newEnv(t, false)
	for i := range 40 {
		if rec := e.do("GET", "/api/v1/pages", "", nil); rec.Code != http.StatusUnauthorized {
			t.Fatalf("request %d: got %d", i+1, rec.Code)
		}
	}
	wantStatus(t, e.do("GET", "/api/v1/pages", "", nil), http.StatusTooManyRequests, tooMany)
	wantStatus(t, e.do("GET", "/api/v1/pages", "", nil, "RemoteAddr", "198.51.100.1:1"), http.StatusUnauthorized, unauthorized)
}

// Mutation L: a key's writes are audited and attributed to the key, and a bearer
// write without Origin (curl, a script) passes CrossOriginProtection (ADR 0006).
func TestKeyWriteAudited(t *testing.T) {
	e := newEnv(t, false)
	e.importPage("P100.tti")
	id, b := e.key(false)
	e.keyGrant(id, 0x100, 0x200, true)
	wantStatus(t, e.bearer("PUT", "/api/v1/pages/100/tti", string(editedP100(t)), b), http.StatusOK, "")
	wantStatus(t, e.bearer("POST", "/api/v1/pages/200", "", b), http.StatusCreated, "")
	if n := e.count(fmt.Sprintf("SELECT count(*) FROM audit WHERE actor = 'key:%s' AND action IN ('page.upload', 'page.create')", id)); n != 2 {
		t.Fatalf("audit rows as the key %d, want 2: %v", n, e.auditRows())
	}
	wantStatus(t, e.bearer("POST", "/api/v1/pages/201", "", b, "Origin", "http://evil.example"), http.StatusForbidden,
		`{"error":{"code":"cross_origin","message":"cross-origin request refused"}}`)
}

// Mutation M: logout by a key is refused and ends no session, even with a cookie attached.
func TestLogoutByKey403(t *testing.T) {
	e := newEnv(t, false)
	e.addUser("conor", true)
	admin := e.login("conor")
	_, b := e.key(false)
	rec := e.do("POST", "/api/v1/logout", "", admin, "Authorization", b)
	wantStatus(t, rec, http.StatusForbidden, `{"error":{"code":"forbidden","message":"not a session"}}`)
	if len(rec.Result().Cookies()) != 0 || e.count("SELECT count(*) FROM sessions") != 1 {
		t.Fatal("logout by key touched the session")
	}
	wantStatus(t, e.do("GET", "/api/v1/me", "", admin), http.StatusOK, "")
}

func TestKeyCreateListAndValidation(t *testing.T) {
	e := newEnv(t, false)
	e.importPage("P100.tti")
	e.addUser("conor", true)
	admin := e.login("conor")
	future := time.Now().UTC().Add(48 * time.Hour).Truncate(time.Second)
	k, token := e.createKeyAPI(admin, fmt.Sprintf(`{"name":"  sheet sync  ","readOnly":true,"expiresAt":%q}`, future.In(time.FixedZone("x", 3600)).Format(time.RFC3339)))
	if !strings.HasPrefix(token, "yfk_"+k.ID+"_") || k.Name != "sheet sync" || !k.ReadOnly || k.CreatedBy != "user:1" ||
		k.ExpiresAt == nil || *k.ExpiresAt != future.Format(time.RFC3339) || k.RevokedAt != nil || k.LastUsedAt != nil || len(k.Grants) != 0 {
		t.Fatalf("created key %+v", k)
	}
	rec := e.do("POST", "/api/v1/keys/"+k.ID+"/grants", `{"lo":"100","hi":"1FF","access":"write"}`, admin)
	wantStatus(t, rec, http.StatusCreated, "")
	wantStatus(t, e.bearer("GET", "/api/v1/pages/100", "", "Bearer "+token), http.StatusOK, "")

	rec = e.do("GET", "/api/v1/keys", "", admin)
	var list struct{ Keys []map[string]any }
	if err := json.Unmarshal(rec.Body.Bytes(), &list); err != nil || rec.Code != http.StatusOK || len(list.Keys) != 1 {
		t.Fatalf("list: %d", rec.Code)
	}
	got := list.Keys[0]
	var fields []string
	for f := range got {
		fields = append(fields, f)
	}
	if len(fields) != 9 || got["lastUsedAt"] == nil || got["revokedAt"] != nil {
		t.Fatalf("list fields %v, lastUsedAt %v, revokedAt %v", fields, got["lastUsedAt"], got["revokedAt"])
	}
	if gs := got["grants"].([]any); len(gs) != 1 || gs[0].(map[string]any)["lo"] != "100" || gs[0].(map[string]any)["access"] != "write" {
		t.Fatalf("list grants %v", gs)
	}
	if n := e.count("SELECT count(*) FROM audit WHERE action = 'key.create' AND target = 'key:" + k.ID + "' AND detail = 'sheet sync'"); n != 1 {
		t.Fatalf("key.create audit rows %d", n)
	}

	for _, body := range []string{
		`{"name":""}`, `{"name":"   "}`, `{"name":"two\nlines"}`, fmt.Sprintf(`{"name":%q}`, strings.Repeat("x", 101)),
		`{"name":"k","expiresAt":"tomorrow"}`, `{"name":"k","expiresAt":"2020-01-01T00:00:00Z"}`, `not json`,
	} {
		if rec := e.do("POST", "/api/v1/keys", body, admin); rec.Code != http.StatusBadRequest {
			t.Errorf("create %s: got %d, want 400", body, rec.Code)
		}
	}
	if rec := e.do("POST", "/api/v1/keys", fmt.Sprintf(`{"name":%q}`, strings.Repeat("x", 100)), admin); rec.Code != http.StatusCreated {
		t.Errorf("100-byte name: got %d", rec.Code)
	}
	wantStatus(t, e.do("POST", "/api/v1/keys/"+k.ID+"/grants", `{"lo":"200","hi":"100","access":"read"}`, admin), http.StatusBadRequest, "")
	wantStatus(t, e.do("POST", "/api/v1/keys/0123456789abcdef/grants", `{"lo":"100","hi":"100","access":"read"}`, admin), http.StatusNotFound, "")
	if n := e.count("SELECT count(*) FROM api_keys"); n != 2 {
		t.Fatalf("keys %d after refused creates", n)
	}
}
