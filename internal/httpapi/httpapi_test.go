package httpapi

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"testing/fstest"
	"time"

	"golang.org/x/time/rate"

	"ceems/internal/auth"
	"ceems/internal/store"
)

var ui = fstest.MapFS{
	"index.html":    {Data: []byte("<html>shell</html>")},
	"assets/app.js": {Data: []byte("console.log(1)")},
}

const testPassword = "correct horse battery"

// testHash hashes testPassword once so bcrypt cost 11 does not run per test.
var testHash = sync.OnceValue(func() []byte {
	h, err := auth.HashPassword(testPassword)
	if err != nil {
		panic(err)
	}
	return h
})

type env struct {
	t   *testing.T
	db  *store.Store
	raw *sql.DB
	h   http.Handler
	api *api
	log *bytes.Buffer
}

func newEnv(t *testing.T, secureCookies bool) *env {
	t.Helper()
	dir := t.TempDir()
	db, err := store.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	raw, err := sql.Open("sqlite", filepath.Join(dir, "ceems.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { raw.Close() })
	e := &env{t: t, db: db, raw: raw, log: &bytes.Buffer{}}
	e.h, e.api = newAPI(ui, slog.New(slog.NewTextHandler(e.log, nil)), db, secureCookies, nil, nil, "/data/pages")
	return e
}

// do sends a request from 192.0.2.1 unless headers set "RemoteAddr".
func (e *env) do(method, path, body string, c *http.Cookie, headers ...string) *httptest.ResponseRecorder {
	e.t.Helper()
	r := httptest.NewRequest(method, path, strings.NewReader(body))
	r.RemoteAddr = "192.0.2.1:1234"
	for i := 0; i+1 < len(headers); i += 2 {
		if headers[i] == "RemoteAddr" {
			r.RemoteAddr = headers[i+1]
		} else {
			r.Header.Set(headers[i], headers[i+1])
		}
	}
	if c != nil {
		r.AddCookie(c)
	}
	rec := httptest.NewRecorder()
	e.h.ServeHTTP(rec, r)
	return rec
}

func (e *env) addUser(name string, admin bool) int64 {
	e.t.Helper()
	id, err := e.db.CreateUser(name, testHash(), admin, store.Audit{Actor: "test", Action: "user.create", Detail: name})
	if err != nil {
		e.t.Fatal(err)
	}
	return id
}

func loginBody(name, pw string) string {
	b, _ := json.Marshal(map[string]string{"username": name, "password": pw})
	return string(b)
}

// login logs name in with testPassword and returns the session cookie.
func (e *env) login(name string) *http.Cookie {
	e.t.Helper()
	rec := e.do("POST", "/api/v1/login", loginBody(name, testPassword), nil)
	if rec.Code != http.StatusOK {
		e.t.Fatalf("login %s: %d %s", name, rec.Code, rec.Body.String())
	}
	for _, c := range rec.Result().Cookies() {
		if c.Name == "ceems_session" {
			return c
		}
	}
	e.t.Fatal("login set no session cookie")
	return nil
}

func (e *env) exec(query string, args ...any) {
	e.t.Helper()
	if _, err := e.raw.Exec(query, args...); err != nil {
		e.t.Fatal(err)
	}
}

func (e *env) count(query string) int {
	e.t.Helper()
	var n int
	if err := e.raw.QueryRow(query).Scan(&n); err != nil {
		e.t.Fatal(err)
	}
	return n
}

func (e *env) importPage(name string) {
	e.t.Helper()
	b, err := os.ReadFile("../../testdata/sample/pages/" + name)
	if err != nil {
		e.t.Fatal(err)
	}
	if _, err := e.db.ImportPages([]store.ImportFile{{Path: name, TTI: b}}, "cli"); err != nil {
		e.t.Fatal(err)
	}
}

func wantStatus(t *testing.T, rec *httptest.ResponseRecorder, code int, body string) {
	t.Helper()
	if rec.Code != code || (body != "" && strings.TrimSpace(rec.Body.String()) != body) {
		t.Fatalf("got %d %q, want %d %q", rec.Code, rec.Body.String(), code, body)
	}
}

const unauthorized = `{"error":{"code":"unauthorized","message":"login required"}}`

func TestHealthz(t *testing.T) {
	wantStatus(t, newEnv(t, false).do("GET", "/healthz", "", nil), http.StatusOK, `{"status":"ok"}`)
}

func TestSiteIsPublicAndTracksTheSavedName(t *testing.T) {
	e := newEnv(t, false)
	e.addUser("admin", true)
	wantStatus(t, e.do("GET", "/api/v1/site", "", nil), http.StatusOK, `{"systemName":"CeeMS"}`)
	c := e.do("POST", "/api/v1/login", loginBody("admin", testPassword), nil).Result().Cookies()[0]
	wantStatus(t, e.do("PUT", "/api/v1/settings", goodSettings(map[string]any{"systemName": "NEWNAME"}), c), http.StatusOK, "")
	wantStatus(t, e.do("GET", "/api/v1/site", "", nil), http.StatusOK, `{"systemName":"NEWNAME"}`)
}

func TestUnknownAPIPathIs404WithErrorShape(t *testing.T) {
	e := newEnv(t, false)
	e.addUser("conor", true)
	rec := e.do("GET", "/api/v1/nope", "", e.login("conor"))
	wantStatus(t, rec, http.StatusNotFound, `{"error":{"code":"not_found","message":"no such API route"}}`)
}

func TestStaticFileServed(t *testing.T) {
	rec := newEnv(t, false).do("GET", "/assets/app.js", "", nil)
	if rec.Code != http.StatusOK || rec.Body.String() != "console.log(1)" {
		t.Fatalf("got %d %q", rec.Code, rec.Body.String())
	}
}

func TestClientRouteFallsBackToIndex(t *testing.T) {
	e := newEnv(t, false)
	for _, p := range []string{"/", "/pages/100", "/assets", "/login"} {
		rec := e.do("GET", p, "", nil)
		if rec.Code != http.StatusOK || rec.Body.String() != "<html>shell</html>" {
			t.Fatalf("%s: got %d %q", p, rec.Code, rec.Body.String())
		}
	}
}

func TestRequestLogLine(t *testing.T) {
	e := newEnv(t, false)
	e.do("GET", "/healthz", "", nil)
	line := e.log.String()
	for _, want := range []string{"method=GET", `route="GET /healthz"`, "status=200", `actor=""`} {
		if !strings.Contains(line, want) {
			t.Fatalf("log line %q missing %q", line, want)
		}
	}
	if strings.Count(line, "\n") != 1 {
		t.Fatalf("want one line, got %q", line)
	}

	id := e.addUser("conor", true)
	c := e.login("conor")
	e.log.Reset()
	e.do("GET", "/api/v1/me", "", c)
	line = e.log.String()
	for _, want := range []string{`route="GET /api/v1/me"`, "status=200", fmt.Sprintf("actor=user:%d", id)} {
		if !strings.Contains(line, want) {
			t.Fatalf("log line %q missing %q", line, want)
		}
	}
}

// A
func TestAnonymousAPIIs401(t *testing.T) {
	e := newEnv(t, false)
	for _, c := range [][2]string{{"GET", "/api/v1/me"}, {"POST", "/api/v1/logout"}, {"GET", "/api/v1/pages"}, {"GET", "/api/v1/pages/100"}, {"GET", "/api/v1/pages/100/tti"}, {"PUT", "/api/v1/pages/100/tti"}, {"GET", "/api/v1/nope"}} {
		rec := e.do(c[0], c[1], "", nil)
		if rec.Code != http.StatusUnauthorized || strings.TrimSpace(rec.Body.String()) != unauthorized || rec.Header().Get("Set-Cookie") != "" {
			t.Errorf("%s %s: got %d %q cookie %q", c[0], c[1], rec.Code, rec.Body.String(), rec.Header().Get("Set-Cookie"))
		}
	}
	for _, p := range []string{"/healthz", "/"} {
		if rec := e.do("GET", p, "", nil); rec.Code != http.StatusOK {
			t.Errorf("%s: got %d", p, rec.Code)
		}
	}
	// A cookie naming no session is anonymous too.
	if rec := e.do("GET", "/api/v1/me", "", &http.Cookie{Name: "ceems_session", Value: "made-up"}); rec.Code != http.StatusUnauthorized {
		t.Errorf("unknown token: got %d", rec.Code)
	}
}

// B
func TestPageRouteAnonymous401(t *testing.T) {
	e := newEnv(t, false)
	e.importPage("P100.tti")
	wantStatus(t, e.do("GET", "/api/v1/pages/100", "", nil), http.StatusUnauthorized, unauthorized)
}

func TestPageRouteWithSession200(t *testing.T) {
	e := newEnv(t, false)
	e.importPage("P100.tti")
	e.importPage("P81A.tti")
	e.addUser("conor", true)
	c := e.login("conor")
	rec := e.do("GET", "/api/v1/pages/100", "", c)
	if rec.Code != http.StatusOK {
		t.Fatalf("got %d %s", rec.Code, rec.Body.String())
	}
	var got map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got["mpp"] != "100" || len(got["subpages"].([]any)) != 7 || got["transmit"] != true || got["updatedBy"] != "cli" || len(got) != 10 || got["binding"] != nil || got["fastextGroup"] != nil {
		t.Fatalf("page body %v", got)
	}
	if _, err := time.Parse(time.RFC3339, got["updatedAt"].(string)); err != nil {
		t.Fatalf("updatedAt %v", got["updatedAt"])
	}
	if rec := e.do("GET", "/api/v1/pages/81a", "", c); rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"mpp":"81A"`) {
		t.Fatalf("lowercase mpp: %d %s", rec.Code, rec.Body.String())
	}
	for _, p := range []string{"0FF", "900", "8FF", "1000", "10", "zzz", "+10", "1_0"} {
		wantStatus(t, e.do("GET", "/api/v1/pages/"+p, "", c), http.StatusNotFound, `{"error":{"code":"not_found","message":"no such page"}}`)
	}
}

// C
func TestPageRouteDeniesWithoutGrant(t *testing.T) {
	e := newEnv(t, false)
	e.importPage("P100.tti")
	e.addUser("viewer", false)
	wantStatus(t, e.do("GET", "/api/v1/pages/100", "", e.login("viewer")), http.StatusNotFound, `{"error":{"code":"not_found","message":"no such page"}}`)
}

func TestPageRouteReadOnlyWrite403(t *testing.T) {
	mux := http.NewServeMux()
	ok := func(w http.ResponseWriter, r *http.Request, mpp int) { w.WriteHeader(http.StatusOK) }
	(&api{}).pageRoute(mux, "POST /x/{mpp}", true, ok)
	(&api{}).pageRoute(mux, "DELETE /x/{mpp}", true, ok)
	(&api{}).pageRoute(mux, "GET /x/{mpp}", false, ok)
	s := &session{principal: auth.Principal{Actor: "user:9", Grants: []auth.Grant{{Lo: 0x100, Hi: 0x100}}}}
	for _, c := range []struct {
		method, mpp string
		want        int
	}{
		{"POST", "100", http.StatusForbidden},
		{"DELETE", "100", http.StatusForbidden},
		{"GET", "100", http.StatusOK},
		{"POST", "0FF", http.StatusNotFound}, // lo-1
		{"GET", "101", http.StatusNotFound},  // hi+1
		{"POST", "101", http.StatusNotFound},
	} {
		r := httptest.NewRequest(c.method, "/x/"+c.mpp, nil)
		r = r.WithContext(context.WithValue(r.Context(), ctxKey{}, s))
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, r)
		if rec.Code != c.want {
			t.Errorf("%s %s: got %d want %d", c.method, c.mpp, rec.Code, c.want)
		}
		if c.want == http.StatusForbidden && strings.TrimSpace(rec.Body.String()) != `{"error":{"code":"forbidden","message":"no write access to this page"}}` {
			t.Errorf("403 body %q", rec.Body.String())
		}
	}
}

// D
const invalidCredentials = `{"error":{"code":"invalid_credentials","message":"invalid username or password"}}`

func TestLoginWrongPassword401(t *testing.T) {
	e := newEnv(t, false)
	e.addUser("conor", true)
	rec := e.do("POST", "/api/v1/login", loginBody("conor", testPassword+"x"), nil)
	wantStatus(t, rec, http.StatusUnauthorized, invalidCredentials)
	if rec.Header().Get("Set-Cookie") != "" || e.count("SELECT count(*) FROM sessions") != 0 {
		t.Fatal("failed login created a session")
	}
	if !strings.Contains(e.log.String(), "username=conor") {
		t.Fatalf("failed login for a known user should log the username: %q", e.log.String())
	}
}

func TestLoginUnknownUserSameBody(t *testing.T) {
	e := newEnv(t, false)
	e.addUser("conor", true)
	wrong := e.do("POST", "/api/v1/login", loginBody("conor", testPassword+"x"), nil)
	e.log.Reset()
	unknown := e.do("POST", "/api/v1/login", loginBody("nobody", testPassword), nil)
	if unknown.Code != http.StatusUnauthorized || unknown.Body.String() != wrong.Body.String() {
		t.Fatalf("unknown %d %q vs wrong password %d %q", unknown.Code, unknown.Body.String(), wrong.Code, wrong.Body.String())
	}
	if strings.Contains(e.log.String(), "nobody") {
		t.Fatalf("unknown username was logged: %q", e.log.String())
	}
}

func TestLoginBadRequest(t *testing.T) {
	e := newEnv(t, false)
	for _, body := range []string{"nope", `{"username":"conor"}`, `{"password":"x"}`, `{"username":"a","password":"` + strings.Repeat("x", 5000) + `"}`} {
		if rec := e.do("POST", "/api/v1/login", body, nil); rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), `"code":"bad_request"`) {
			t.Errorf("%.30q: got %d %s", body, rec.Code, rec.Body.String())
		}
	}
}

// E
func TestExpiredSession401(t *testing.T) {
	e := newEnv(t, false)
	e.addUser("conor", true)
	c := e.login("conor")
	e.exec("UPDATE sessions SET expires_at = '2000-01-01T00:00:00Z'")
	wantStatus(t, e.do("GET", "/api/v1/me", "", c), http.StatusUnauthorized, unauthorized)
}

// F
func TestDisabledUserSession401(t *testing.T) {
	e := newEnv(t, false)
	e.addUser("conor", true)
	c := e.login("conor")
	e.exec("UPDATE users SET disabled = 1")
	wantStatus(t, e.do("GET", "/api/v1/me", "", c), http.StatusUnauthorized, unauthorized)
	wantStatus(t, e.do("POST", "/api/v1/login", loginBody("conor", testPassword), nil), http.StatusUnauthorized, invalidCredentials)
}

// G
func TestResetPasswordDeletesSessions(t *testing.T) {
	e := newEnv(t, false)
	e.addUser("conor", true)
	c := e.login("conor")
	if err := e.db.SetPasswordHash("conor", testHash(), store.Audit{Actor: "cli", Action: "user.reset_password"}); err != nil {
		t.Fatal(err)
	}
	wantStatus(t, e.do("GET", "/api/v1/me", "", c), http.StatusUnauthorized, unauthorized)
	if n := e.count("SELECT count(*) FROM sessions"); n != 0 {
		t.Fatalf("sessions left %d", n)
	}
}

// H
func TestCrossOriginPost403(t *testing.T) {
	e := newEnv(t, false)
	e.addUser("conor", true)
	c := e.login("conor")
	const refused = `{"error":{"code":"cross_origin","message":"cross-origin request refused"}}`
	wantStatus(t, e.do("POST", "/api/v1/logout", "", c, "Sec-Fetch-Site", "cross-site"), http.StatusForbidden, refused)
	wantStatus(t, e.do("POST", "/api/v1/logout", "", c, "Origin", "http://evil.example"), http.StatusForbidden, refused)
	if e.count("SELECT count(*) FROM sessions") != 1 {
		t.Fatal("refused request changed the sessions table")
	}
	wantStatus(t, e.do("GET", "/api/v1/me", "", c), http.StatusOK, "")
	wantStatus(t, e.do("POST", "/api/v1/logout", "", c, "Sec-Fetch-Site", "same-origin"), http.StatusNoContent, "")
}

// I
func TestLoginRateLimitedPerIP(t *testing.T) {
	e := newEnv(t, false)
	for i := range 10 {
		if rec := e.do("POST", "/api/v1/login", loginBody(fmt.Sprint("user", i), testPassword), nil); rec.Code == http.StatusTooManyRequests {
			t.Fatalf("attempt %d limited", i+1)
		}
	}
	wantStatus(t, e.do("POST", "/api/v1/login", loginBody("userz", testPassword), nil), http.StatusTooManyRequests,
		`{"error":{"code":"rate_limited","message":"too many login attempts; try again later"}}`)
}

// J
func TestLoginRateLimitedPerUsername(t *testing.T) {
	e := newEnv(t, false)
	e.addUser("conor", true)
	for i, name := range []string{"conor", "CONOR", "Conor", "cOnor", "coNor"} {
		rec := e.do("POST", "/api/v1/login", loginBody(name, "wrong password!"), nil, "RemoteAddr", fmt.Sprintf("198.51.100.%d:1", i+1))
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("attempt %d: got %d", i+1, rec.Code)
		}
	}
	// The right password is refused too once the username is exhausted.
	rec := e.do("POST", "/api/v1/login", loginBody("conoR", testPassword), nil, "RemoteAddr", "198.51.100.9:1")
	wantStatus(t, rec, http.StatusTooManyRequests, `{"error":{"code":"rate_limited","message":"too many login attempts; try again later"}}`)
}

func TestLimiterEvictsFullBuckets(t *testing.T) {
	l := newLimiter(rate.Every(time.Millisecond), 1)
	for i := range 1001 {
		l.Allow(fmt.Sprint(i))
	}
	time.Sleep(5 * time.Millisecond)
	l.Allow("new")
	if len(l.m) != 1 {
		t.Fatalf("after eviction %d entries", len(l.m))
	}
}

// K
func TestLogoutInvalidatesSession(t *testing.T) {
	e := newEnv(t, false)
	e.addUser("conor", true)
	c := e.login("conor")
	rec := e.do("POST", "/api/v1/logout", "", c)
	wantStatus(t, rec, http.StatusNoContent, "")
	cleared := rec.Result().Cookies()
	if len(cleared) != 1 || cleared[0].Name != "ceems_session" || cleared[0].Value != "" || cleared[0].MaxAge >= 0 ||
		!cleared[0].HttpOnly || cleared[0].SameSite != http.SameSiteStrictMode || cleared[0].Path != "/" {
		t.Fatalf("logout cookie %+v", cleared)
	}
	wantStatus(t, e.do("GET", "/api/v1/me", "", c), http.StatusUnauthorized, unauthorized)
}

// L
func TestNoSecretInResponsesOrStore(t *testing.T) {
	e := newEnv(t, false)
	e.importPage("P100.tti")
	e.addUser("conor", true)
	login := e.do("POST", "/api/v1/login", loginBody("conor", testPassword), nil)
	c := login.Result().Cookies()[0]
	me := e.do("GET", "/api/v1/me", "", c)
	page := e.do("GET", "/api/v1/pages/100", "", c)
	wantStatus(t, me, http.StatusOK, `{"headerTemplate":"CeeMS   %%# %%a %e %%b \u0003%H:%M:%S","indexPage":null,"systemName":"CeeMS","user":{"id":1,"username":"conor","admin":true}}`)
	if strings.TrimSpace(login.Body.String()) != strings.TrimSpace(me.Body.String()) {
		t.Fatalf("login body %q", login.Body.String())
	}
	for name, s := range map[string]string{"login": login.Body.String(), "me": me.Body.String(), "page": page.Body.String(), "log": e.log.String()} {
		for _, secret := range []string{"$2", testPassword, c.Value} {
			if strings.Contains(s, secret) {
				t.Errorf("%s contains a secret", name)
			}
		}
	}
	var hash, tokenHash []byte
	if err := e.raw.QueryRow("SELECT password_hash FROM users").Scan(&hash); err != nil || !bytes.HasPrefix(hash, []byte("$2a$11$")) {
		t.Fatalf("password_hash prefix wrong, err %v", err)
	}
	if err := e.raw.QueryRow("SELECT token_hash FROM sessions").Scan(&tokenHash); err != nil {
		t.Fatal(err)
	}
	if want := sha256.Sum256([]byte(c.Value)); !bytes.Equal(tokenHash, want[:]) {
		t.Fatal("token_hash is not sha256(token)")
	}

	// An API key token is in its create response only; the store keeps SHA-256(secret).
	create := e.do("POST", "/api/v1/keys", `{"name":"script","readOnly":false}`, c)
	var created struct {
		Key   keyDoc
		Token string
	}
	if err := json.Unmarshal(create.Body.Bytes(), &created); err != nil || create.Code != http.StatusCreated || !strings.HasPrefix(created.Token, "yfk_"+created.Key.ID+"_") {
		t.Fatalf("create key: %d", create.Code)
	}
	secret := strings.TrimPrefix(created.Token, "yfk_"+created.Key.ID+"_")
	wantStatus(t, e.do("POST", "/api/v1/keys/"+created.Key.ID+"/grants", `{"lo":"100","hi":"100","access":"read"}`, c), http.StatusCreated, "")
	keyMe := e.bearer("GET", "/api/v1/me", "", "Bearer "+created.Token)
	keyPage := e.bearer("GET", "/api/v1/pages/100", "", "Bearer "+created.Token)
	wantStatus(t, keyPage, http.StatusOK, "")
	list := e.do("GET", "/api/v1/keys", "", c)
	for name, s := range map[string]string{"key me": keyMe.Body.String(), "key page": keyPage.Body.String(), "key list": list.Body.String(), "log": e.log.String()} {
		if strings.Contains(s, secret) {
			t.Errorf("%s contains the key secret", name)
		}
	}
	var secretHash []byte
	if err := e.raw.QueryRow("SELECT secret_hash FROM api_keys").Scan(&secretHash); err != nil {
		t.Fatal(err)
	}
	if want := sha256.Sum256([]byte(secret)); !bytes.Equal(secretHash, want[:]) {
		t.Fatal("secret_hash is not sha256(secret)")
	}

	rows, err := e.raw.Query("SELECT actor || action || target || detail || remote FROM audit")
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	for rows.Next() {
		var s string
		rows.Scan(&s)
		if strings.Contains(s, testPassword) || strings.Contains(s, "$2") || strings.Contains(s, c.Value) || strings.Contains(s, secret) {
			t.Error("audit row contains a secret")
		}
	}
}

// O
func TestLoginCookieAttributes(t *testing.T) {
	for _, secure := range []bool{false, true} {
		e := newEnv(t, secure)
		e.addUser("conor", true)
		c := e.login("conor")
		if !c.HttpOnly || c.SameSite != http.SameSiteStrictMode || c.Path != "/" || c.MaxAge != 1209600 || c.Secure != secure || c.Value == "" {
			t.Errorf("secureCookies %v: cookie HttpOnly=%v SameSite=%v Path=%q MaxAge=%d Secure=%v",
				secure, c.HttpOnly, c.SameSite, c.Path, c.MaxAge, c.Secure)
		}
	}
}

// grant gives user uid a grant directly in the store.
func (e *env) grant(uid int64, lo, hi int, write bool) int64 {
	e.t.Helper()
	id, err := e.db.CreateGrant(store.Grant{UserID: &uid, Lo: lo, Hi: hi, Write: write}, store.Audit{Actor: "test", Action: "grant.create"})
	if err != nil {
		e.t.Fatal(err)
	}
	return id
}

// routePath splits a registry pattern into its method and a path with each {name} replaced by values[name].
func routePath(pattern string, values map[string]string) (string, string) {
	method, path, _ := strings.Cut(pattern, " ")
	for k, v := range values {
		path = strings.ReplaceAll(path, "{"+k+"}", v)
	}
	return method, path
}

const noSuchPage = `{"error":{"code":"not_found","message":"no such page"}}`

// Mutations A and B: every registered page route checks the grant.
func TestEveryPageRouteChecksGrant(t *testing.T) {
	e := newEnv(t, false)
	e.importPage("P100.tti")
	e.importPage("P101.tti") // exists, so a 404 on 101 can only come from the grant check
	uid := e.addUser("viewer", false)
	e.grant(uid, 0x100, 0x100, false)
	c := e.login("viewer")
	var reads, writes int
	for _, rt := range e.api.routes {
		if rt.kind != "page" {
			continue
		}
		if rt.write {
			writes++
		} else {
			reads++
		}
		method, path := routePath(rt.pattern, map[string]string{"mpp": "100"})
		rec := e.do(method, path, "", c)
		if rt.write && (rec.Code != http.StatusForbidden || strings.TrimSpace(rec.Body.String()) != `{"error":{"code":"forbidden","message":"no write access to this page"}}`) {
			t.Errorf("%s with only a read grant: got %d %s, want 403 forbidden", rt.pattern, rec.Code, rec.Body.String())
		}
		if !rt.write && (rec.Code == http.StatusForbidden || rec.Code == http.StatusNotFound) {
			t.Errorf("%s with a read grant: got %d %s, want access", rt.pattern, rec.Code, rec.Body.String())
		}
		for _, mpp := range []string{"0FF", "101"} {
			method, path := routePath(rt.pattern, map[string]string{"mpp": mpp})
			if rec := e.do(method, path, "", c); rec.Code != http.StatusNotFound || strings.TrimSpace(rec.Body.String()) != noSuchPage {
				t.Errorf("%s at %s outside the grant: got %d %s, want 404", rt.pattern, mpp, rec.Code, rec.Body.String())
			}
		}
	}
	if reads == 0 || writes == 0 {
		t.Fatalf("registry has %d read and %d write page routes", reads, writes)
	}
	if n := e.count("SELECT count(*) FROM pages"); n != 2 {
		t.Fatalf("pages %d after refused writes", n)
	}
}

// Mutation C
func TestListOnlyReadable(t *testing.T) {
	e := newEnv(t, false)
	importAll(e)
	uid := e.addUser("viewer", false)
	e.grant(uid, 0x101, 0x199, false)
	e.grant(uid, 0x81B, 0x81C, true)
	rec := e.do("GET", "/api/v1/pages", "", e.login("viewer"))
	var got listBody
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil || rec.Code != http.StatusOK {
		t.Fatalf("got %d %s", rec.Code, rec.Body.String())
	}
	var mpps []string
	for _, p := range got.Pages {
		mpps = append(mpps, p.MPP)
	}
	if strings.Join(mpps, ",") != "101,120,121,199,81B,81C" {
		t.Fatalf("listed %v", mpps)
	}
}

// Mutation D: a grant change applies to the next request on the same session.
func TestGrantAppliesNextRequest(t *testing.T) {
	e := newEnv(t, false)
	e.importPage("P100.tti")
	uid := e.addUser("viewer", false)
	e.addUser("conor", true)
	viewer, admin := e.login("viewer"), e.login("conor")
	wantStatus(t, e.do("GET", "/api/v1/pages/100", "", viewer), http.StatusNotFound, noSuchPage)
	rec := e.do("POST", fmt.Sprintf("/api/v1/users/%d/grants", uid), `{"lo":"100","hi":"100","access":"read"}`, admin)
	wantStatus(t, rec, http.StatusCreated, "")
	var g struct{ ID int64 }
	json.Unmarshal(rec.Body.Bytes(), &g)
	wantStatus(t, e.do("GET", "/api/v1/pages/100", "", viewer), http.StatusOK, "")
	wantStatus(t, e.do("DELETE", fmt.Sprintf("/api/v1/grants/%d", g.ID), "", admin), http.StatusNoContent, "")
	wantStatus(t, e.do("GET", "/api/v1/pages/100", "", viewer), http.StatusNotFound, noSuchPage)
}

var proxyNet = []netip.Prefix{netip.MustParsePrefix("192.0.2.0/24")}

// proxyEnv is newEnv with trusted proxies. do sends from 192.0.2.1, inside proxyNet.
func proxyEnv(t *testing.T, trusted []netip.Prefix) *env {
	t.Helper()
	e := newEnv(t, false)
	e.h, e.api = newAPI(ui, slog.New(slog.NewTextHandler(e.log, nil)), e.db, false, nil, trusted, "/data/pages")
	return e
}

func TestClientIPTable(t *testing.T) {
	ten := []netip.Prefix{netip.MustParsePrefix("10.0.0.0/8")}
	mapped := []netip.Prefix{netip.MustParsePrefix("::ffff:10.0.0.0/104")}
	one := []netip.Prefix{netip.MustParsePrefix("::ffff:10.0.0.3/128")}
	cases := []struct {
		name, remote string
		xff          []string
		trusted      []netip.Prefix
		want         string
	}{
		{"untrusted remote ignores header", "203.0.113.5:99", []string{"1.2.3.4"}, ten, "203.0.113.5"},
		{"rightmost untrusted wins", "10.0.0.2:99", []string{"1.1.1.1, 2.2.2.2, 10.0.0.3"}, ten, "2.2.2.2"},
		{"all entries trusted", "10.0.0.2:99", []string{"10.0.0.4, 10.0.0.3"}, ten, "10.0.0.2"},
		{"header absent", "10.0.0.2:99", nil, ten, "10.0.0.2"},
		{"garbage at the right skipped", "10.0.0.2:99", []string{"1.1.1.1, foo"}, ten, "1.1.1.1"},
		{"two header lines are one list", "10.0.0.2:99", []string{"1.1.1.1", "2.2.2.2"}, ten, "2.2.2.2"},
		{"zone stripped", "10.0.0.2:99", []string{"fe80::1%eth0"}, ten, "fe80::1"},
		{"empty list ignores header", "10.0.0.2:99", []string{"1.2.3.4"}, nil, "10.0.0.2"},
		{"remote without port", "10.0.0.2", []string{"1.2.3.4"}, ten, "1.2.3.4"},
		{"mapped proxy entry is trusted", "10.0.0.2:99", []string{"1.1.1.1, ::ffff:10.0.0.3"}, ten, "1.1.1.1"},
		{"mapped client reported as IPv4", "10.0.0.2:99", []string{"::ffff:1.1.1.1"}, ten, "1.1.1.1"},
		{"mapped remote is trusted", "[::ffff:10.0.0.2]:99", []string{"1.1.1.1"}, ten, "1.1.1.1"},
		{"mapped-form prefix matches plain IPv4", "10.0.0.2:99", []string{"1.1.1.1, 10.0.0.3"}, mapped, "1.1.1.1"},
		{"mapped host prefix (config workaround) still matches", "[::ffff:10.0.0.3]:99", []string{"1.1.1.1, ::ffff:10.0.0.3"}, one, "1.1.1.1"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest("GET", "/", nil)
			r.RemoteAddr = tc.remote
			for _, v := range tc.xff {
				r.Header.Add("X-Forwarded-For", v)
			}
			if got := clientIPFor(r, tc.trusted); got != tc.want {
				t.Fatalf("got %q want %q", got, tc.want)
			}
		})
	}
}

// loginsFrom sends n logins for different usernames, with forwarded(i) as X-Forwarded-For
// and remote as RemoteAddr, and returns the status of the last one.
func (e *env) loginsFrom(n int, remote string, forwarded func(i int) string) int {
	e.t.Helper()
	var code int
	for i := range n {
		code = e.do("POST", "/api/v1/login", loginBody(fmt.Sprint("nobody", i), "wrong password!"), nil,
			"RemoteAddr", remote, "X-Forwarded-For", forwarded(i)).Code
		if i < n-1 && code == http.StatusTooManyRequests {
			e.t.Fatalf("attempt %d limited", i+1)
		}
	}
	return code
}

func TestLoginLimitKeyedByForwardedIP(t *testing.T) {
	distinct := func(i int) string { return fmt.Sprintf("198.51.100.%d", i+1) }
	same := func(int) string { return "198.51.100.99" }

	e := proxyEnv(t, proxyNet)
	if got := e.loginsFrom(11, "192.0.2.10:1", distinct); got != http.StatusUnauthorized {
		t.Fatalf("distinct forwarded IPs: got %d, want 401", got)
	}
	if got := e.loginsFrom(11, "192.0.2.10:1", same); got != http.StatusTooManyRequests {
		t.Fatalf("repeated forwarded IP: got %d, want 429", got)
	}

	e = proxyEnv(t, proxyNet)
	if got := e.loginsFrom(11, "203.0.113.7:1", distinct); got != http.StatusTooManyRequests {
		t.Fatalf("untrusted remote, varying headers: got %d, want 429", got)
	}
}

func TestAPILimitKeyedByForwardedIP(t *testing.T) {
	send := func(e *env, n int, remote string, forwarded func(i int) string) int {
		var code int
		for i := range n {
			code = e.do("GET", "/api/v1/pages", "", nil, "RemoteAddr", remote, "X-Forwarded-For", forwarded(i)).Code
			if i < n-1 && code == http.StatusTooManyRequests {
				t.Fatalf("request %d limited", i+1)
			}
		}
		return code
	}
	distinct := func(i int) string { return fmt.Sprintf("198.51.100.%d", i+1) }
	same := func(int) string { return "198.51.100.200" }

	e := proxyEnv(t, proxyNet)
	if got := send(e, 41, "192.0.2.10:1", distinct); got != http.StatusUnauthorized {
		t.Fatalf("distinct forwarded IPs: got %d, want 401", got)
	}
	if got := send(e, 41, "192.0.2.10:1", same); got != http.StatusTooManyRequests {
		t.Fatalf("repeated forwarded IP: got %d, want 429", got)
	}

	e = proxyEnv(t, proxyNet)
	if got := send(e, 41, "203.0.113.7:1", distinct); got != http.StatusTooManyRequests {
		t.Fatalf("untrusted remote, varying headers: got %d, want 429", got)
	}
}

func TestAuditRemoteIsClientIP(t *testing.T) {
	for _, tc := range []struct{ name, remote, want string }{
		{"trusted proxy", "192.0.2.10:1", "198.51.100.7"},
		{"untrusted address", "203.0.113.7:1", "203.0.113.7"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := proxyEnv(t, proxyNet)
			e.importPage("P100.tti")
			e.addUser("conor", true)
			c := e.login("conor")
			wantStatus(t, e.do("PUT", "/api/v1/pages/100/tti", string(editedP100(t)), c,
				"RemoteAddr", tc.remote, "X-Forwarded-For", "198.51.100.7"), http.StatusOK, "")
			var got string
			if err := e.raw.QueryRow("SELECT remote FROM audit WHERE action = 'page.upload'").Scan(&got); err != nil {
				t.Fatal(err)
			}
			if got != tc.want {
				t.Fatalf("audit remote %q, want %q", got, tc.want)
			}
		})
	}
}

func TestRequestLogHasIP(t *testing.T) {
	for _, tc := range []struct{ name, remote, want string }{
		{"trusted proxy", "192.0.2.10:1", "ip=198.51.100.7"},
		{"untrusted address", "203.0.113.7:1", "ip=203.0.113.7"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := proxyEnv(t, proxyNet)
			wantStatus(t, e.do("GET", "/healthz", "", nil, "RemoteAddr", tc.remote, "X-Forwarded-For", "198.51.100.7"), http.StatusOK, "")
			if !strings.Contains(e.log.String(), tc.want) {
				t.Fatalf("log %q lacks %q", e.log.String(), tc.want)
			}
		})
	}
}
