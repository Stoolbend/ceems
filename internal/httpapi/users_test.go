package httpapi

import (
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"testing"

	"ceems/internal/auth"
)

var generatedPassword = regexp.MustCompile(`^[A-Za-z0-9]{20}$`)

// createUser creates name through the API as admin and returns the user id and the
// one-time password. The password is only ever compared, never printed.
func (e *env) createUser(admin *http.Cookie, name string, isAdmin bool) (int64, string) {
	e.t.Helper()
	rec := e.do("POST", "/api/v1/users", fmt.Sprintf(`{"username":%q,"admin":%v}`, name, isAdmin), admin)
	if rec.Code != http.StatusCreated {
		e.t.Fatalf("create %s: %d %s", name, rec.Code, rec.Body.String())
	}
	var body struct {
		User     userDoc
		Password string
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		e.t.Fatal(err)
	}
	if !generatedPassword.MatchString(body.Password) {
		e.t.Fatal("created user's password is not 20 alphanumerics")
	}
	return body.User.ID, body.Password
}

// loginWith logs name in with pw and returns the session cookie, or nil on a non-200.
func (e *env) loginWith(name, pw string) *http.Cookie {
	e.t.Helper()
	rec := e.do("POST", "/api/v1/login", loginBody(name, pw), nil, "RemoteAddr", "203.0.113.7:1")
	if rec.Code != http.StatusOK {
		return nil
	}
	return rec.Result().Cookies()[0]
}

// auditRows returns every audit row as one string per row.
func (e *env) auditRows() []string {
	e.t.Helper()
	rows, err := e.raw.Query("SELECT actor || '|' || action || '|' || target || '|' || detail || '|' || remote FROM audit ORDER BY id")
	if err != nil {
		e.t.Fatal(err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var s string
		rows.Scan(&s)
		out = append(out, s)
	}
	return out
}

// noSecrets fails if any of the secrets is in the log, an audit row or any of the bodies.
func (e *env) noSecrets(secrets []string, bodies ...string) {
	e.t.Helper()
	all := append([]string{e.log.String(), strings.Join(e.auditRows(), "\n")}, bodies...)
	for _, s := range all {
		for _, secret := range secrets {
			if strings.Contains(s, secret) {
				e.t.Error("a password or hash appears in the log, the audit table or a later response")
			}
		}
	}
}

func TestCreateUserListAndPasswordShownOnce(t *testing.T) {
	e := newEnv(t, false)
	e.addUser("conor", true)
	admin := e.login("conor")
	id, pw := e.createUser(admin, "Viewer", false)
	if c := e.loginWith("viewer", pw); c == nil {
		t.Fatal("the returned password does not log in")
	}
	e.grant(id, 0x100, 0x1FF, true)
	list := e.do("GET", "/api/v1/users", "", admin)
	wantStatus(t, list, http.StatusOK, "")
	var got struct{ Users []userDoc }
	json.Unmarshal(list.Body.Bytes(), &got)
	if len(got.Users) != 2 || got.Users[0].Username != "conor" || got.Users[1].Username != "viewer" || got.Users[1].Admin ||
		got.Users[1].Disabled || got.Users[1].CreatedAt == "" || len(got.Users[1].Grants) != 1 ||
		got.Users[1].Grants[0].Lo != "100" || got.Users[1].Grants[0].Hi != "1FF" || got.Users[1].Grants[0].Access != "write" {
		t.Fatalf("users %+v", got.Users)
	}
	if strings.Contains(list.Body.String(), "password") || strings.Contains(list.Body.String(), "$2") {
		t.Fatal("user list carries a password field or hash")
	}
	e.noSecrets([]string{pw, "$2"}, list.Body.String())

	wantStatus(t, e.do("POST", "/api/v1/users", `{"username":"VIEWER","admin":false}`, admin), http.StatusConflict,
		`{"error":{"code":"exists","message":"a user with that name already exists"}}`)
	wantStatus(t, e.do("POST", "/api/v1/users", `{"username":"bad name","admin":false}`, admin), http.StatusBadRequest,
		`{"error":{"code":"bad_request","message":"username: must be 1-64 characters of letters, digits, '.', '_' or '-'"}}`)
	wantStatus(t, e.do("POST", "/api/v1/users", `nope`, admin), http.StatusBadRequest, "")
}

func TestResetPasswordRoute(t *testing.T) {
	e := newEnv(t, false)
	e.addUser("conor", true)
	admin := e.login("conor")
	id, old := e.createUser(admin, "viewer", false)
	session := e.loginWith("viewer", old)
	rec := e.do("POST", fmt.Sprintf("/api/v1/users/%d/reset-password", id), "", admin)
	wantStatus(t, rec, http.StatusOK, "")
	var body struct{ Password string }
	json.Unmarshal(rec.Body.Bytes(), &body)
	if !generatedPassword.MatchString(body.Password) || body.Password == old {
		t.Fatal("reset did not return a new 20-character password")
	}
	wantStatus(t, e.do("GET", "/api/v1/me", "", session), http.StatusUnauthorized, unauthorized)
	if e.loginWith("viewer", old) != nil || e.loginWith("viewer", body.Password) == nil {
		t.Fatal("after reset the old password should fail and the new one log in")
	}
	wantStatus(t, e.do("POST", "/api/v1/users/99/reset-password", "", admin), http.StatusNotFound, `{"error":{"code":"not_found","message":"no such user"}}`)
	e.noSecrets([]string{old, body.Password, "$2"})
}

func TestDisableEnableRoute(t *testing.T) {
	e := newEnv(t, false)
	e.addUser("conor", true)
	admin := e.login("conor")
	id, pw := e.createUser(admin, "viewer", false)
	session := e.loginWith("viewer", pw)
	rec := e.do("PUT", fmt.Sprintf("/api/v1/users/%d", id), `{"disabled":true}`, admin)
	wantStatus(t, rec, http.StatusOK, "")
	if !strings.Contains(rec.Body.String(), `"disabled":true`) {
		t.Fatalf("body %s", rec.Body.String())
	}
	wantStatus(t, e.do("GET", "/api/v1/me", "", session), http.StatusUnauthorized, unauthorized)
	if e.loginWith("viewer", pw) != nil {
		t.Fatal("a disabled user logged in")
	}
	wantStatus(t, e.do("PUT", fmt.Sprintf("/api/v1/users/%d", id), `{"disabled":false}`, admin), http.StatusOK, "")
	if e.loginWith("viewer", pw) == nil {
		t.Fatal("an enabled user cannot log in")
	}
	wantStatus(t, e.do("PUT", fmt.Sprintf("/api/v1/users/%d", id), `{}`, admin), http.StatusBadRequest, `{"error":{"code":"bad_request","message":"body must be {disabled}"}}`)
	wantStatus(t, e.do("PUT", "/api/v1/users/99", `{"disabled":true}`, admin), http.StatusNotFound, "")
	wantStatus(t, e.do("PUT", "/api/v1/users/x", `{"disabled":true}`, admin), http.StatusNotFound, "")
}

// Mutation H
func TestDisableSelfRefused(t *testing.T) {
	e := newEnv(t, false)
	id := e.addUser("conor", true)
	admin := e.login("conor")
	wantStatus(t, e.do("PUT", fmt.Sprintf("/api/v1/users/%d", id), `{"disabled":true}`, admin), http.StatusConflict,
		`{"error":{"code":"self","message":"you cannot disable yourself"}}`)
	wantStatus(t, e.do("GET", "/api/v1/me", "", admin), http.StatusOK, "")
	if n := e.count("SELECT count(*) FROM users WHERE disabled = 1"); n != 0 {
		t.Fatalf("disabled users %d", n)
	}
}

// Mutation I
func TestCreateGrantRejectsBadRange(t *testing.T) {
	e := newEnv(t, false)
	e.addUser("conor", true)
	admin := e.login("conor")
	uid := e.addUser("viewer", false)
	path := fmt.Sprintf("/api/v1/users/%d/grants", uid)
	for _, body := range []string{
		`{"lo":"200","hi":"1FF","access":"read"}`, // hi < lo
		`{"lo":"0FF","hi":"100","access":"read"}`,
		`{"lo":"100","hi":"900","access":"read"}`,
		`{"lo":"10","hi":"100","access":"read"}`,
		`{"lo":"100","hi":"100","access":"admin"}`,
		`{"lo":"100","hi":"100"}`,
		`nope`,
	} {
		if rec := e.do("POST", path, body, admin); rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), `"code":"bad_request"`) {
			t.Errorf("%s: got %d %s, want 400 bad_request", body, rec.Code, rec.Body.String())
		}
	}
	if n := e.count("SELECT count(*) FROM grants"); n != 0 {
		t.Fatalf("grants %d after refused creates", n)
	}
	rec := e.do("POST", path, `{"lo":"100","hi":"8ff","access":"write"}`, admin)
	if rec.Code != http.StatusCreated || !regexp.MustCompile(`^\{"id":\d+,"lo":"100","hi":"8FF","access":"write"\}$`).MatchString(strings.TrimSpace(rec.Body.String())) {
		t.Fatalf("good grant: %d %s", rec.Code, rec.Body.String())
	}
	wantStatus(t, e.do("POST", "/api/v1/users/99/grants", `{"lo":"100","hi":"100","access":"read"}`, admin), http.StatusNotFound, "")
	wantStatus(t, e.do("DELETE", "/api/v1/grants/99", "", admin), http.StatusNotFound, `{"error":{"code":"not_found","message":"no such grant"}}`)
}

// Mutation J: one audit row per successful change, none for a failed one, no password in any.
func TestUserGrantAudit(t *testing.T) {
	e := newEnv(t, false)
	e.addUser("conor", true)
	admin := e.login("conor")
	before := len(e.auditRows())
	id, pw := e.createUser(admin, "viewer", false)
	rec := e.do("POST", fmt.Sprintf("/api/v1/users/%d/grants", id), `{"lo":"100","hi":"1FF","access":"write"}`, admin)
	var g struct{ ID int64 }
	json.Unmarshal(rec.Body.Bytes(), &g)
	e.do("PUT", fmt.Sprintf("/api/v1/users/%d", id), `{"disabled":true}`, admin)
	e.do("PUT", fmt.Sprintf("/api/v1/users/%d", id), `{"disabled":false}`, admin)
	reset := e.do("POST", fmt.Sprintf("/api/v1/users/%d/reset-password", id), "", admin)
	e.do("DELETE", fmt.Sprintf("/api/v1/grants/%d", g.ID), "", admin)
	// Failures write nothing.
	e.do("POST", fmt.Sprintf("/api/v1/users/%d/grants", id), `{"lo":"200","hi":"100","access":"read"}`, admin)
	e.do("POST", "/api/v1/users", `{"username":"viewer","admin":false}`, admin)
	e.do("POST", "/api/v1/users", `{"username":"bad name","admin":false}`, admin)
	e.do("DELETE", "/api/v1/grants/99", "", admin)
	e.do("PUT", "/api/v1/users/1", `{"disabled":true}`, admin)

	got := e.auditRows()[before:]
	u, gr := fmt.Sprintf("user:%d", id), fmt.Sprintf("grant:%d", g.ID)
	want := []string{
		"user:1|user.create|" + u + "|viewer|192.0.2.1",
		"user:1|grant.create|" + gr + "|" + u + " 100-1FF write|192.0.2.1",
		"user:1|user.disable|" + u + "|viewer|192.0.2.1",
		"user:1|user.enable|" + u + "|viewer|192.0.2.1",
		"user:1|user.reset_password|" + u + "|viewer|192.0.2.1",
		"user:1|grant.delete|" + gr + "|" + u + " 100-1FF write|192.0.2.1",
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("audit rows:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
	var newPw struct{ Password string }
	json.Unmarshal(reset.Body.Bytes(), &newPw)
	e.noSecrets([]string{pw, newPw.Password, "$2"})
}

// Mutation E: every admin route refuses a non-admin (403) and anonymous (401).
func TestUsersAdminOnly(t *testing.T) {
	e := newEnv(t, false)
	e.addUser("conor", true)
	uid := e.addUser("guest", false)
	e.grant(uid, 0x100, 0x100, true)
	guest := e.login("guest")
	var n int
	for _, rt := range e.api.routes {
		if rt.kind != "admin" {
			continue
		}
		n++
		method, path := routePath(rt.pattern, map[string]string{"id": "1"})
		body := `{"username":"x","admin":true,"disabled":true,"lo":"100","hi":"100","access":"write"}`
		// Bodies are not printed: a wrongly allowed reset or create would carry a password.
		if rec := e.do(method, path, body, guest); rec.Code != http.StatusForbidden || !strings.Contains(rec.Body.String(), `"message":"admin only"`) {
			t.Errorf("%s as a non-admin: got %d, want 403 admin only", rt.pattern, rec.Code)
		}
		// From a second address, so the two loops together stay inside the per-IP API burst.
		if rec := e.do(method, path, body, nil, "RemoteAddr", "198.51.100.200:1"); rec.Code != http.StatusUnauthorized {
			t.Errorf("%s anonymous: got %d, want 401", rt.pattern, rec.Code)
		}
	}
	for _, p := range []string{"GET /api/v1/users", "POST /api/v1/users", "PUT /api/v1/users/{id}", "POST /api/v1/users/{id}/reset-password", "POST /api/v1/users/{id}/grants", "DELETE /api/v1/grants/{id}",
		"GET /api/v1/keys", "POST /api/v1/keys", "DELETE /api/v1/keys/{id}", "POST /api/v1/keys/{id}/grants",
		"GET /api/v1/settings", "PUT /api/v1/settings"} {
		found := false
		for _, rt := range e.api.routes {
			found = found || (rt.pattern == p && rt.kind == "admin")
		}
		if !found {
			t.Errorf("%s is not registered through adminRoute", p)
		}
	}
	if n < 25 {
		t.Fatalf("only %d admin routes in the registry", n)
	}
	if e.count("SELECT count(*) FROM users") != 2 || e.count("SELECT count(*) FROM users WHERE disabled = 1") != 0 || e.count("SELECT count(*) FROM grants") != 1 ||
		e.count("SELECT count(*) FROM api_keys") != 0 {
		t.Fatal("a refused admin request changed users, grants or keys")
	}
}

const newPassword = "a brand new passphrase"

func changeBody(current, next string) string {
	b, _ := json.Marshal(map[string]string{"currentPassword": current, "newPassword": next})
	return string(b)
}

// session makes a session for uid in the store, so no login spends a rate-limit token.
func (e *env) session(uid int64) *http.Cookie {
	e.t.Helper()
	tok, hash := auth.NewToken()
	if err := e.db.CreateSession(hash, uid, "2026-01-01T00:00:00Z", "2099-01-01T00:00:00Z"); err != nil {
		e.t.Fatal(err)
	}
	return &http.Cookie{Name: "ceems_session", Value: tok}
}

func (e *env) passwordHash(uid int64) string {
	e.t.Helper()
	var h string
	if err := e.raw.QueryRow("SELECT password_hash FROM users WHERE id = ?", uid).Scan(&h); err != nil {
		e.t.Fatal(err)
	}
	return h
}

// Mutation M, and the success path.
func TestChangePasswordEndsAllSessions(t *testing.T) {
	e := newEnv(t, false)
	uid := e.addUser("viewer", false)
	c1, c2 := e.login("viewer"), e.login("viewer")
	before := len(e.auditRows())
	rec := e.do("PUT", "/api/v1/me/password", changeBody(testPassword, newPassword), c1)
	wantStatus(t, rec, http.StatusNoContent, "")
	cleared := rec.Result().Cookies()
	if len(cleared) != 1 || cleared[0].Name != "ceems_session" || cleared[0].Value != "" || cleared[0].MaxAge >= 0 {
		t.Fatalf("change did not clear the cookie: %d cookies", len(cleared))
	}
	if n := e.count("SELECT count(*) FROM sessions"); n != 0 {
		t.Fatalf("sessions left %d", n)
	}
	wantStatus(t, e.do("GET", "/api/v1/me", "", c1), http.StatusUnauthorized, unauthorized)
	wantStatus(t, e.do("GET", "/api/v1/me", "", c2), http.StatusUnauthorized, unauthorized)
	if e.loginWith("viewer", newPassword) == nil {
		t.Fatal("the new password does not log in")
	}
	wantStatus(t, e.do("POST", "/api/v1/login", loginBody("viewer", testPassword), nil), http.StatusUnauthorized, invalidCredentials)
	got := e.auditRows()[before:]
	if want := fmt.Sprintf("user:%d|user.change_password|user:%d||192.0.2.1", uid, uid); len(got) != 1 || got[0] != want {
		t.Fatalf("audit rows %q, want one %q", got, want)
	}
	e.noSecrets([]string{testPassword, newPassword, "$2"})
}

// Mutation K
func TestChangePasswordWrongCurrent(t *testing.T) {
	e := newEnv(t, false)
	uid := e.addUser("viewer", false)
	c := e.session(uid)
	hash, audits := e.passwordHash(uid), len(e.auditRows())
	wantStatus(t, e.do("PUT", "/api/v1/me/password", changeBody("not the password", newPassword), c), http.StatusBadRequest,
		`{"error":{"code":"wrong_password","message":"current password is incorrect"}}`)
	if e.passwordHash(uid) != hash || len(e.auditRows()) != audits || e.count("SELECT count(*) FROM sessions") != 1 {
		t.Fatal("a wrong current password changed the hash, the audit table or the sessions")
	}
	wantStatus(t, e.do("GET", "/api/v1/me", "", c), http.StatusOK, "")
	if log := e.log.String(); !strings.Contains(log, "msg=change_password ok=false ip=192.0.2.1 username=viewer") {
		t.Fatal("failed change was not logged with ok=false, the IP and the username")
	}
	e.noSecrets([]string{"not the password", newPassword, "$2"})
}

// Mutation L
func TestChangePasswordTooShort(t *testing.T) {
	e := newEnv(t, false)
	uid := e.addUser("viewer", false)
	c := e.session(uid)
	hash, audits := e.passwordHash(uid), len(e.auditRows())
	for _, next := range []string{"short", strings.Repeat("x", 73)} {
		rec := e.do("PUT", "/api/v1/me/password", changeBody(testPassword, next), c)
		wantStatus(t, rec, http.StatusBadRequest, fmt.Sprintf(`{"error":{"code":"bad_request","message":"password must be 12-72 bytes, got %d"}}`, len(next)))
	}
	if e.passwordHash(uid) != hash || len(e.auditRows()) != audits || e.count("SELECT count(*) FROM sessions") != 1 {
		t.Fatal("a refused new password changed the hash, the audit table or the sessions")
	}
}

func TestChangePasswordBadRequest(t *testing.T) {
	e := newEnv(t, false)
	c := e.session(e.addUser("viewer", false))
	for _, body := range []string{"nope", `{"currentPassword":"` + testPassword + `"}`, `{"newPassword":"` + newPassword + `"}`, `{"currentPassword":"` + strings.Repeat("x", 5000) + `"}`} {
		if rec := e.do("PUT", "/api/v1/me/password", body, c); rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), `"code":"bad_request"`) {
			t.Errorf("%.30q: got %d %s", body, rec.Code, rec.Body.String())
		}
	}
	wantStatus(t, e.do("PUT", "/api/v1/me/password", changeBody(testPassword, newPassword), nil), http.StatusUnauthorized, unauthorized)
}

// Mutation N: the change shares login's per-username bucket.
func TestChangePasswordRateLimited(t *testing.T) {
	e := newEnv(t, false)
	c := e.session(e.addUser("viewer", false))
	for i := range 5 {
		if rec := e.do("PUT", "/api/v1/me/password", changeBody("not the password", newPassword), c); rec.Code != http.StatusBadRequest {
			t.Fatalf("wrong attempt %d: got %d", i+1, rec.Code)
		}
	}
	wantStatus(t, e.do("PUT", "/api/v1/me/password", changeBody(testPassword, newPassword), c), http.StatusTooManyRequests,
		`{"error":{"code":"rate_limited","message":"too many login attempts; try again later"}}`)
	wantStatus(t, e.do("GET", "/api/v1/me", "", c), http.StatusOK, "")
}

// Mutation O: one audit row for a success, none for a failure.
func TestChangePasswordAudit(t *testing.T) {
	e := newEnv(t, false)
	uid := e.addUser("viewer", false)
	c := e.session(uid)
	before := len(e.auditRows())
	e.do("PUT", "/api/v1/me/password", changeBody("not the password", newPassword), c)
	if n := len(e.auditRows()) - before; n != 0 {
		t.Fatalf("a wrong current password wrote %d audit rows", n)
	}
	wantStatus(t, e.do("PUT", "/api/v1/me/password", changeBody(testPassword, newPassword), c), http.StatusNoContent, "")
	if n := e.count("SELECT count(*) FROM audit WHERE action = 'user.change_password'"); n != 1 {
		t.Fatalf("change_password audit rows %d, want 1", n)
	}
}

// A key principal cannot change a password, even carrying a user's cookie.
func TestChangePasswordKeyPrincipal(t *testing.T) {
	e := newEnv(t, false)
	uid := e.addUser("viewer", false)
	c := e.session(uid)
	_, bearer := e.key(false)
	wantStatus(t, e.do("PUT", "/api/v1/me/password", changeBody(testPassword, newPassword), c, "Authorization", bearer), http.StatusForbidden,
		`{"error":{"code":"forbidden","message":"not a session"}}`)
	if n := e.count("SELECT count(*) FROM audit WHERE action = 'user.change_password'"); n != 0 {
		t.Fatalf("change_password audit rows %d", n)
	}
	wantStatus(t, e.do("GET", "/api/v1/me", "", c), http.StatusOK, "")
}
