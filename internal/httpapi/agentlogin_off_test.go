//go:build !agentdev

package httpapi

import (
	"net/http"
	"testing"
)

func TestAgentLoginAbsentWithoutTag(t *testing.T) {
	e := newEnv(t, false)
	e.addUser("agent", true)
	rec := e.do("GET", "/api/v1/agent-login", "", nil, "RemoteAddr", "127.0.0.1:1234")
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("got %d, want 401 from requireSession", rec.Code)
	}
	if len(rec.Result().Cookies()) != 0 {
		t.Fatal("set a cookie")
	}
}
