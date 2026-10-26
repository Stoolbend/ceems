// Package httpapi holds the HTTP routes, handlers and middleware.
package httpapi

import (
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"io/fs"
	"log/slog"
	"net"
	"net/http"
	"net/netip"
	"strconv"
	"strings"
	"time"

	"golang.org/x/time/rate"

	"ceems/internal/auth"
	"ceems/internal/jobs"
	"ceems/internal/store"
)

const (
	cookieName = "ceems_session"
	sessionTTL = 14 * 24 * time.Hour
)

type api struct {
	db            *store.Store
	log           *slog.Logger
	secureCookies bool
	ipLimit       *limiter     // login, per client IP
	userLimit     *limiter     // login and password change, per username
	apiIPLimit    *limiter     // every /api/ request behind requireSession, per client IP
	keyLimit      *limiter     // every request authenticated by a key, per key id
	runner        *jobs.Runner // nil: run-now answers 503 and sheetsConfigured is false
	pagesDir      string       // the config value, shown read-only by GET /api/v1/settings
	routes        []routeInfo  // every route registered through pageRoute, adminRoute or sessionRoute
}

// routeInfo records one registered route; kind is "page", "admin" or "session".
type routeInfo struct {
	pattern string
	kind    string
	write   bool
}

// New returns the application handler. ui holds the single-page app files.
// Anonymous paths are exactly GET /healthz, GET /api/v1/site, POST /api/v1/login and the SPA,
// plus GET /api/v1/agent-login in an agentdev-tagged build (agentlogin.go;
// ADR 0007); every other /api/ path needs a session. runner may be nil. pagesDir is the config's pagesDir, which the settings route reports.
func New(ui fs.FS, log *slog.Logger, db *store.Store, secureCookies bool, runner *jobs.Runner, trusted []netip.Prefix, pagesDir string) http.Handler {
	h, _ := newAPI(ui, log, db, secureCookies, runner, trusted, pagesDir)
	return h
}

// newAPI is New that also returns the api, so tests can walk its route registry.
func newAPI(ui fs.FS, log *slog.Logger, db *store.Store, secureCookies bool, runner *jobs.Runner, trusted []netip.Prefix, pagesDir string) (http.Handler, *api) {
	a := &api{
		db:            db,
		log:           log,
		secureCookies: secureCookies,
		ipLimit:       newLimiter(rate.Every(10*time.Second), 10),
		userLimit:     newLimiter(rate.Every(30*time.Second), 5),
		apiIPLimit:    newLimiter(rate.Limit(20), 40),
		keyLimit:      newLimiter(rate.Limit(5), 20),
		runner:        runner,
		pagesDir:      pagesDir,
	}

	apiMux := http.NewServeMux()
	a.sessionRoute(apiMux, "GET /api/v1/openapi.json", serveOpenAPI)
	a.sessionRoute(apiMux, "POST /api/v1/logout", a.logout)
	a.sessionRoute(apiMux, "GET /api/v1/me", a.me)
	a.sessionRoute(apiMux, "PUT /api/v1/me/password", a.changePassword)
	a.sessionRoute(apiMux, "GET /api/v1/pages", a.listPages)
	a.pageRoute(apiMux, "GET /api/v1/pages/{mpp}", false, a.getPage)
	a.pageRoute(apiMux, "GET /api/v1/pages/{mpp}/tti", false, a.downloadPage)
	a.pageRoute(apiMux, "PUT /api/v1/pages/{mpp}", true, a.updatePage)
	a.pageRoute(apiMux, "PUT /api/v1/pages/{mpp}/tti", true, a.uploadPage)
	a.pageRoute(apiMux, "POST /api/v1/pages/{mpp}", true, a.createPage)
	a.pageRoute(apiMux, "DELETE /api/v1/pages/{mpp}", true, a.deletePage)
	a.adminRoute(apiMux, "GET /api/v1/templates", a.listTemplates)
	a.adminRoute(apiMux, "POST /api/v1/templates", a.createTemplate)
	a.adminRoute(apiMux, "GET /api/v1/templates/{id}", a.getTemplate)
	a.adminRoute(apiMux, "PUT /api/v1/templates/{id}", a.updateTemplate)
	a.adminRoute(apiMux, "DELETE /api/v1/templates/{id}", a.deleteTemplate)
	a.adminRoute(apiMux, "GET /api/v1/templates/{id}/tti", a.downloadTemplate)
	a.adminRoute(apiMux, "GET /api/v1/bindings", a.listBindings)
	a.adminRoute(apiMux, "POST /api/v1/bindings", a.createBinding)
	a.adminRoute(apiMux, "GET /api/v1/bindings/{id}", a.getBinding)
	a.adminRoute(apiMux, "PUT /api/v1/bindings/{id}", a.updateBinding)
	a.adminRoute(apiMux, "DELETE /api/v1/bindings/{id}", a.deleteBinding)
	a.adminRoute(apiMux, "POST /api/v1/bindings/{id}/run", a.runBinding)
	a.adminRoute(apiMux, "GET /api/v1/bindings/{id}/runs", a.listRuns)
	a.adminRoute(apiMux, "GET /api/v1/users", a.listUsers)
	a.adminRoute(apiMux, "POST /api/v1/users", a.createUser)
	a.adminRoute(apiMux, "PUT /api/v1/users/{id}", a.updateUser)
	a.adminRoute(apiMux, "POST /api/v1/users/{id}/reset-password", a.resetPassword)
	a.adminRoute(apiMux, "POST /api/v1/users/{id}/grants", a.createGrant)
	a.adminRoute(apiMux, "DELETE /api/v1/grants/{id}", a.deleteGrant)
	a.adminRoute(apiMux, "GET /api/v1/keys", a.listKeys)
	a.adminRoute(apiMux, "POST /api/v1/keys", a.createKey)
	a.adminRoute(apiMux, "DELETE /api/v1/keys/{id}", a.revokeKey)
	a.adminRoute(apiMux, "POST /api/v1/keys/{id}/grants", a.createKeyGrant)
	a.adminRoute(apiMux, "GET /api/v1/fastext-groups", a.listFastext)
	a.adminRoute(apiMux, "POST /api/v1/fastext-groups", a.createFastext)
	a.adminRoute(apiMux, "GET /api/v1/fastext-groups/{id}", a.getFastext)
	a.adminRoute(apiMux, "PUT /api/v1/fastext-groups/{id}", a.updateFastext)
	a.adminRoute(apiMux, "DELETE /api/v1/fastext-groups/{id}", a.deleteFastext)
	a.adminRoute(apiMux, "PUT /api/v1/fastext-groups/{id}/pages", a.setFastextPages)
	a.adminRoute(apiMux, "GET /api/v1/settings", a.getSettings)
	a.adminRoute(apiMux, "PUT /api/v1/settings", a.updateSettings)
	apiMux.HandleFunc("/api/", func(w http.ResponseWriter, r *http.Request) {
		writeError(w, http.StatusNotFound, "not_found", "no such API route")
	})

	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})
	mux.HandleFunc("POST /api/v1/login", a.login)
	mux.HandleFunc("GET /api/v1/site", a.site)
	a.registerAgentLogin(mux)
	mux.Handle("/api/", a.requireSession(apiMux))
	mux.Handle("/", spa(ui))

	cop := http.NewCrossOriginProtection()
	cop.SetDenyHandler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeError(w, http.StatusForbidden, "cross_origin", "cross-origin request refused")
	}))
	return requestLog(log, trusted, cop.Handler(mux)), a
}

// userJSON is the only user shape sent to clients.
type userJSON struct {
	ID       int64  `json:"id"`
	Username string `json:"username"`
	Admin    bool   `json:"admin"`
}

type ctxKey struct{}

// session is what requireSession stores in the request context.
type session struct {
	principal auth.Principal
	user      *userJSON     // nil for an API key principal
	key       *store.APIKey // nil for a user session
	tokenHash []byte        // the session token's hash; nil for a key
}

// principalFrom returns the request's principal; the zero Principal can do nothing.
func principalFrom(r *http.Request) auth.Principal {
	s, _ := r.Context().Value(ctxKey{}).(*session)
	if s == nil {
		return auth.Principal{}
	}
	return s.principal
}

func (a *api) requireSession(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !a.apiIPLimit.Allow(clientIP(r)) {
			writeError(w, http.StatusTooManyRequests, "rate_limited", "too many requests")
			return
		}
		s, ok := a.authenticate(w, r)
		if !ok {
			return
		}
		r2 := r.WithContext(context.WithValue(r.Context(), ctxKey{}, s))
		info := logInfoFrom(r)
		if info != nil {
			info.actor = s.principal.Actor
		}
		next.ServeHTTP(w, r2)
		if info != nil {
			info.route = r2.Pattern // the inner mux records its pattern on r2, not r
		}
	})
}

// authenticate builds the request's session, or writes the refusal and returns false.
// An Authorization header selects the key path, and then the cookie is never read,
// even when the header is malformed. Grants are read on every request, so a change
// applies to the next one.
func (a *api) authenticate(w http.ResponseWriter, r *http.Request) (*session, bool) {
	now := time.Now().UTC().Format(time.RFC3339)
	if h := r.Header.Get("Authorization"); h != "" {
		p, k, err := auth.KeyPrincipal(a.db, h, now)
		if errors.Is(err, auth.ErrInvalidCredentials) {
			writeError(w, http.StatusUnauthorized, "unauthorized", "login required")
			return nil, false
		}
		if err != nil {
			a.internalError(w, err)
			return nil, false
		}
		if !a.keyLimit.Allow(k.ID) {
			writeError(w, http.StatusTooManyRequests, "rate_limited", "too many requests")
			return nil, false
		}
		if err := a.db.TouchKey(k.ID, now); err != nil {
			a.internalError(w, err)
			return nil, false
		}
		return &session{principal: p, key: k}, true
	}
	c, err := r.Cookie(cookieName)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "unauthorized", "login required")
		return nil, false
	}
	tokenHash := auth.HashToken(c.Value)
	u, err := a.db.UserBySession(tokenHash, now)
	if errors.Is(err, store.ErrUserNotFound) {
		writeError(w, http.StatusUnauthorized, "unauthorized", "login required")
		return nil, false
	}
	if err != nil {
		a.internalError(w, err)
		return nil, false
	}
	p, err := auth.UserPrincipal(a.db, u)
	if err != nil {
		a.internalError(w, err)
		return nil, false
	}
	return &session{principal: p, user: &userJSON{ID: u.ID, Username: u.Username, Admin: u.Admin}, tokenHash: tokenHash}, true
}

// site serves the system name to anyone, so the login page can show it. No limiter: one small read, like /healthz.
func (a *api) site(w http.ResponseWriter, r *http.Request) {
	s, err := a.db.Settings()
	if err != nil {
		a.internalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"systemName": s.SystemName})
}

func (a *api) login(w http.ResponseWriter, r *http.Request) {
	ip := clientIP(r)
	if !a.ipLimit.Allow(ip) {
		writeError(w, http.StatusTooManyRequests, "rate_limited", "too many login attempts; try again later")
		return
	}
	var body struct{ Username, Password string }
	r.Body = http.MaxBytesReader(w, r.Body, 4096)
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Username == "" || body.Password == "" {
		writeError(w, http.StatusBadRequest, "bad_request", "username and password are required")
		return
	}
	if !a.userLimit.Allow(strings.ToLower(body.Username)) {
		writeError(w, http.StatusTooManyRequests, "rate_limited", "too many login attempts; try again later")
		return
	}
	u, err := auth.Login(a.db, body.Username, body.Password)
	if errors.Is(err, auth.ErrInvalidCredentials) {
		// The attempted username is logged only when it exists.
		if known, err := a.db.UserByUsername(body.Username); err == nil {
			a.log.Info("login", "ok", false, "ip", ip, "username", known.Username)
		} else {
			a.log.Info("login", "ok", false, "ip", ip)
		}
		writeError(w, http.StatusUnauthorized, "invalid_credentials", "invalid username or password")
		return
	}
	if err != nil {
		a.internalError(w, err)
		return
	}
	// Built before the session starts, so a failed read leaves no cookie behind.
	doc, ok := a.userDoc(w, &userJSON{ID: u.ID, Username: u.Username, Admin: u.Admin})
	if !ok {
		return
	}
	if err := a.startSession(w, u); err != nil {
		a.internalError(w, err)
		return
	}
	a.log.Info("login", "ok", true, "ip", ip, "username", u.Username)
	writeJSON(w, http.StatusOK, doc)
}

// startSession stores a new session for u and sets its cookie on w.
func (a *api) startSession(w http.ResponseWriter, u *store.User) error {
	token, hash := auth.NewToken()
	now := time.Now().UTC()
	if err := a.db.CreateSession(hash, u.ID, now.Format(time.RFC3339), now.Add(sessionTTL).Format(time.RFC3339)); err != nil {
		return err
	}
	http.SetCookie(w, a.cookie(token, int(sessionTTL/time.Second)))
	return nil
}

// logout deletes the session requireSession authenticated, never one named by a
// re-read cookie, so a key request carrying a cookie ends nothing (ADR 0006).
func (a *api) logout(w http.ResponseWriter, r *http.Request) {
	s, _ := r.Context().Value(ctxKey{}).(*session)
	if s == nil || s.key != nil || s.tokenHash == nil {
		writeError(w, http.StatusForbidden, "forbidden", "not a session")
		return
	}
	if err := a.db.DeleteSession(s.tokenHash); err != nil {
		a.internalError(w, err)
		return
	}
	http.SetCookie(w, a.cookie("", -1))
	w.WriteHeader(http.StatusNoContent)
}

func (a *api) me(w http.ResponseWriter, r *http.Request) {
	s, ok := r.Context().Value(ctxKey{}).(*session)
	if !ok { // requireSession sets it; a route registered outside it is a bug
		a.internalError(w, errors.New("me: no session in context"))
		return
	}
	if s.key != nil {
		writeJSON(w, http.StatusOK, map[string]any{"user": nil, "key": map[string]any{"id": s.key.ID, "name": s.key.Name, "readOnly": s.key.ReadOnly}})
		return
	}
	if doc, ok := a.userDoc(w, s.user); ok {
		writeJSON(w, http.StatusOK, doc)
	}
}

// cookie builds the session cookie; maxAge -1 deletes it (sent as Max-Age=0).
func (a *api) cookie(value string, maxAge int) *http.Cookie {
	return &http.Cookie{
		Name: cookieName, Value: value, Path: "/",
		HttpOnly: true, SameSite: http.SameSiteStrictMode, Secure: a.secureCookies,
		MaxAge: maxAge,
	}
}

// pageRoute registers a page handler; it is the only way a page route is registered.
// It hands h a checked mpp: a malformed or out-of-range mpp, or no read access, is
// 404; a write pattern without write access is 403.
func (a *api) pageRoute(mux *http.ServeMux, pattern string, write bool, h func(w http.ResponseWriter, r *http.Request, mpp int)) {
	a.routes = append(a.routes, routeInfo{pattern, "page", write})
	mux.HandleFunc(pattern, func(w http.ResponseWriter, r *http.Request) {
		s := r.PathValue("mpp")
		n, err := strconv.ParseUint(s, 16, 16)
		if len(s) != 3 || err != nil || n < 0x100 || n > 0x8FF {
			writeError(w, http.StatusNotFound, "not_found", "no such page")
			return
		}
		mpp := int(n)
		p := principalFrom(r)
		if !p.Can(mpp, false) {
			writeError(w, http.StatusNotFound, "not_found", "no such page")
			return
		}
		if write && !p.Can(mpp, true) {
			writeError(w, http.StatusForbidden, "forbidden", "no write access to this page")
			return
		}
		h(w, r, mpp)
	})
}

// adminRoute registers an admin handler; it is the only way an admin route (templates,
// bindings, runs, users, grants) is registered. A principal without Admin gets 403.
// Keys are never admin (ADR 0003), so this refuses every key.
func (a *api) adminRoute(mux *http.ServeMux, pattern string, h http.HandlerFunc) {
	a.routes = append(a.routes, routeInfo{pattern, "admin", !strings.HasPrefix(pattern, "GET ")})
	mux.HandleFunc(pattern, func(w http.ResponseWriter, r *http.Request) {
		if !principalFrom(r).Admin {
			writeError(w, http.StatusForbidden, "forbidden", "admin only")
			return
		}
		h(w, r)
	})
}

// sessionRoute registers a handler any session may call; it only records the route.
func (a *api) sessionRoute(mux *http.ServeMux, pattern string, h http.HandlerFunc) {
	a.routes = append(a.routes, routeInfo{pattern, "session", !strings.HasPrefix(pattern, "GET ")})
	mux.HandleFunc(pattern, h)
}

func (a *api) getPage(w http.ResponseWriter, r *http.Request, mpp int) {
	p, err := a.db.Page(mpp)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not_found", "no such page")
		return
	}
	if err != nil {
		a.internalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, pageJSON(p, principalFrom(r).Can(mpp, true)))
}

func (a *api) internalError(w http.ResponseWriter, err error) {
	a.log.Error("internal error", "err", err)
	writeError(w, http.StatusInternalServerError, "internal", "internal error")
}

// clientIP is the address requestLog computed once for this request. Without a
// logInfo (handler-level tests that bypass requestLog) it is the RemoteAddr host.
func clientIP(r *http.Request) string {
	if info := logInfoFrom(r); info != nil && info.ip != "" {
		return info.ip
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// clientIPFor applies the trustedProxies rule. Not from a trusted address:
// the RemoteAddr host, X-Forwarded-For ignored. From a trusted address: the
// rightmost X-Forwarded-For entry that parses and is not trusted; none
// qualifying: the RemoteAddr host.
func clientIPFor(r *http.Request, trusted []netip.Prefix) string {
	remote, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		remote = r.RemoteAddr
	}
	// remote is not unmapped: inPrefixes matches mapped and plain forms alike.
	if !inPrefixes(remote, trusted) {
		return remote
	}
	entries := strings.Split(strings.Join(r.Header.Values("X-Forwarded-For"), ","), ",")
	for i := len(entries) - 1; i >= 0; i-- {
		addr, err := netip.ParseAddr(strings.TrimSpace(entries[i]))
		if err != nil {
			continue
		}
		addr = addr.WithZone("").Unmap()
		if !inPrefixes(addr.String(), trusted) {
			return addr.String()
		}
	}
	return remote
}

func inPrefixes(host string, trusted []netip.Prefix) bool {
	addr, err := netip.ParseAddr(host)
	if err != nil {
		return false
	}
	addr = addr.WithZone("").Unmap()
	for _, p := range trusted {
		if p.Addr().Is4In6() && p.Bits() >= 96 {
			p = netip.PrefixFrom(p.Addr().Unmap(), p.Bits()-96)
		}
		if p.Contains(addr) {
			return true
		}
	}
	return false
}

// spa serves a file from ui if it exists, else index.html for client routes.
func spa(ui fs.FS) http.Handler {
	files := http.FileServerFS(ui)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "only GET is allowed here")
			return
		}
		name := strings.TrimPrefix(r.URL.Path, "/")
		if st, err := fs.Stat(ui, name); name == "" || err != nil || st.IsDir() {
			r2 := r.Clone(r.Context())
			r2.URL.Path = "/"
			files.ServeHTTP(w, r2)
			return
		}
		files.ServeHTTP(w, r)
	})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, map[string]any{"error": map[string]string{"code": code, "message": message}})
}

// logInfo is filled in by inner handlers for the request log line.
type logInfo struct{ actor, route, ip string }

type logKey struct{}

func logInfoFrom(r *http.Request) *logInfo {
	info, _ := r.Context().Value(logKey{}).(*logInfo)
	return info
}

// requestLog logs one line per request with the actor ("" when anonymous). The
// route pattern is read after the mux has run, since the mux records it on the request.
func requestLog(log *slog.Logger, trusted []netip.Prefix, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		info := &logInfo{ip: clientIPFor(r, trusted)}
		r = r.WithContext(context.WithValue(r.Context(), logKey{}, info))
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, r)
		route := info.route
		if route == "" {
			route = r.Pattern
		}
		if route == "" {
			route = r.URL.Path
		}
		log.Info("request", "method", r.Method, "route", route, "status", rec.status, "duration", time.Since(start), "actor", info.actor, "ip", info.ip)
	})
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (s *statusRecorder) WriteHeader(code int) {
	s.status = code
	s.ResponseWriter.WriteHeader(code)
}

//go:embed openapi.json
var openAPIDoc []byte

// serveOpenAPI sends the hand-written API contract; openapi_test.go checks it against the routes.
func serveOpenAPI(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Write(openAPIDoc)
}
