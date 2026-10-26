//go:build agentdev

package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"testing"
)

func sessionCookie(rec *httptest.ResponseRecorder) *http.Cookie {
	for _, c := range rec.Result().Cookies() {
		if c.Name == cookieName && c.Value != "" {
			return c
		}
	}
	return nil
}

func TestAgentLoginLoopback(t *testing.T) {
	for _, remote := range []string{"127.0.0.1:1234", "[::1]:1234"} {
		t.Run(remote, func(t *testing.T) {
			e := newEnv(t, false)
			e.addUser("agent", true)
			rec := e.do("GET", "/api/v1/agent-login", "", nil, "RemoteAddr", remote)
			if rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != "/" {
				t.Fatalf("got %d Location %q, want 303 /", rec.Code, rec.Header().Get("Location"))
			}
			c := sessionCookie(rec)
			if c == nil {
				t.Fatal("no session cookie")
			}
			me := e.do("GET", "/api/v1/me", "", c, "RemoteAddr", remote)
			var got struct{ User userJSON }
			if me.Code != http.StatusOK || json.Unmarshal(me.Body.Bytes(), &got) != nil || got.User.Username != "agent" || !got.User.Admin {
				t.Fatalf("me: %d %s", me.Code, me.Body.String())
			}
		})
	}
}

func wantAgentLoginRefused(t *testing.T, e *env, headers ...string) {
	t.Helper()
	rec := e.do("GET", "/api/v1/agent-login", "", nil, headers...)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("got %d, want 404", rec.Code)
	}
	if c := sessionCookie(rec); c != nil {
		t.Fatal("refusal set a session cookie")
	}
	if n := e.count("SELECT count(*) FROM sessions"); n != 0 {
		t.Fatalf("refusal created %d sessions", n)
	}
}

func TestAgentLoginRemoteRefused(t *testing.T) {
	e := newEnv(t, false)
	e.addUser("agent", true)
	wantAgentLoginRefused(t, e, "RemoteAddr", "10.0.0.5:1234")
}

func TestAgentLoginIgnoresForwardedFor(t *testing.T) {
	e := proxyEnv(t, []netip.Prefix{netip.MustParsePrefix("10.0.0.0/8")})
	e.addUser("agent", true)
	wantAgentLoginRefused(t, e, "RemoteAddr", "10.0.0.5:1234", "X-Forwarded-For", "127.0.0.1")
}

func TestAgentLoginNoUser(t *testing.T) {
	e := newEnv(t, false)
	wantAgentLoginRefused(t, e, "RemoteAddr", "127.0.0.1:1234")
}

func TestAgentLoginDisabledUser(t *testing.T) {
	e := newEnv(t, false)
	id := e.addUser("agent", true)
	e.exec("UPDATE users SET disabled = 1 WHERE id = ?", id)
	wantAgentLoginRefused(t, e, "RemoteAddr", "127.0.0.1:1234")
}
