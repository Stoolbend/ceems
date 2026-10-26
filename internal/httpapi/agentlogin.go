//go:build agentdev

package httpapi

import (
	"errors"
	"net"
	"net/http"
	"net/netip"

	"ceems/internal/store"
)

// registerAgentLogin adds the credential-free agent login (ADR 0007, D-042). It is
// registered on the outer mux, so it is outside requireSession and the route registry.
func (a *api) registerAgentLogin(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/agent-login", a.agentLogin)
}

// agentLogin gives a loopback caller a session for the enabled user "agent". The raw
// RemoteAddr is checked, never clientIP, so X-Forwarded-For cannot make a caller local.
func (a *api) agentLogin(w http.ResponseWriter, r *http.Request) {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	if ip, err := netip.ParseAddr(host); err != nil || !ip.IsLoopback() {
		writeError(w, http.StatusNotFound, "not_found", "no such API route")
		return
	}
	u, err := a.db.UserByUsername("agent")
	if errors.Is(err, store.ErrUserNotFound) || (err == nil && u.Disabled) {
		writeError(w, http.StatusNotFound, "not_found", "no such API route")
		return
	}
	if err != nil {
		a.internalError(w, err)
		return
	}
	if err := a.startSession(w, u); err != nil {
		a.internalError(w, err)
		return
	}
	a.log.Warn("agent-login", "ip", host, "username", u.Username)
	http.Redirect(w, r, "/", http.StatusSeeOther)
}
