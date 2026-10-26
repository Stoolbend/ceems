package httpapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"

	"ceems/internal/auth"
	"ceems/internal/store"
)

// grantDoc is one grant as sent to clients; lo and hi are three uppercase hex digits.
type grantDoc struct {
	ID     int64  `json:"id"`
	Lo     string `json:"lo"`
	Hi     string `json:"hi"`
	Access string `json:"access"`
}

// userDoc is a user as the admin routes send it. It never carries the hash.
type userDoc struct {
	ID        int64      `json:"id"`
	Username  string     `json:"username"`
	Admin     bool       `json:"admin"`
	Disabled  bool       `json:"disabled"`
	CreatedAt string     `json:"createdAt"`
	Grants    []grantDoc `json:"grants"`
}

func grantJSON(g store.Grant) grantDoc {
	access := "read"
	if g.Write {
		access = "write"
	}
	return grantDoc{ID: g.ID, Lo: fmt.Sprintf("%03X", g.Lo), Hi: fmt.Sprintf("%03X", g.Hi), Access: access}
}

func userJSONFor(u *store.User, gs []store.Grant) userDoc {
	d := userDoc{ID: u.ID, Username: u.Username, Admin: u.Admin, Disabled: u.Disabled, CreatedAt: u.CreatedAt, Grants: []grantDoc{}}
	for _, g := range gs {
		d.Grants = append(d.Grants, grantJSON(g))
	}
	return d
}

// decodeSmall decodes a JSON body of at most 4096 bytes, the limit login uses.
func decodeSmall(w http.ResponseWriter, r *http.Request, v any) error {
	return json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(v)
}

// pathID returns the {id} path value, writing a 404 naming what itself when it is not a positive integer.
func pathID(w http.ResponseWriter, r *http.Request, what string) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id < 1 {
		writeError(w, http.StatusNotFound, "not_found", "no such "+what)
		return 0, false
	}
	return id, true
}

// pathUser loads the {id} user, writing the 404 or 500 itself.
func (a *api) pathUser(w http.ResponseWriter, r *http.Request) (*store.User, bool) {
	id, ok := pathID(w, r, "user")
	if !ok {
		return nil, false
	}
	u, err := a.db.UserByID(id)
	if errors.Is(err, store.ErrUserNotFound) {
		writeError(w, http.StatusNotFound, "not_found", "no such user")
		return nil, false
	}
	if err != nil {
		a.internalError(w, err)
		return nil, false
	}
	return u, true
}

// writeUser answers status with the user id's document.
func (a *api) writeUser(w http.ResponseWriter, status int, id int64, extra map[string]any) {
	u, err := a.db.UserByID(id)
	if err != nil {
		a.internalError(w, err)
		return
	}
	gs, err := a.db.GrantsForUser(id)
	if err != nil {
		a.internalError(w, err)
		return
	}
	body := map[string]any{"user": userJSONFor(u, gs)}
	for k, v := range extra {
		body[k] = v
	}
	writeJSON(w, status, body)
}

func (a *api) listUsers(w http.ResponseWriter, r *http.Request) {
	us, err := a.db.Users()
	if err != nil {
		a.internalError(w, err)
		return
	}
	gs, err := a.db.AllGrants()
	if err != nil {
		a.internalError(w, err)
		return
	}
	byUser := map[int64][]store.Grant{}
	for _, g := range gs {
		if g.UserID != nil {
			byUser[*g.UserID] = append(byUser[*g.UserID], g)
		}
	}
	out := []userDoc{}
	for i := range us {
		out = append(out, userJSONFor(&us[i], byUser[us[i].ID]))
	}
	writeJSON(w, http.StatusOK, map[string]any{"users": out})
}

// createUser makes a user with a random password, returned once in this response
// and never stored or logged; the same path as `ceems admin create`.
func (a *api) createUser(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Username string `json:"username"`
		Admin    bool   `json:"admin"`
	}
	if err := decodeSmall(w, r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", "body must be {username, admin}")
		return
	}
	name, err := auth.CheckUsername(body.Username)
	if err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", err.Error())
		return
	}
	pw := auth.RandomPassword()
	hash, err := auth.HashPassword(pw)
	if err != nil {
		a.internalError(w, err)
		return
	}
	au := audit(r, "user.create")
	au.Detail = name
	id, err := a.db.CreateUser(name, hash, body.Admin, au)
	if errors.Is(err, store.ErrUserExists) {
		writeError(w, http.StatusConflict, "exists", "a user with that name already exists")
		return
	}
	if err != nil {
		a.internalError(w, err)
		return
	}
	a.writeUser(w, http.StatusCreated, id, map[string]any{"password": pw})
}

func (a *api) updateUser(w http.ResponseWriter, r *http.Request) {
	u, ok := a.pathUser(w, r)
	if !ok {
		return
	}
	var body struct {
		Disabled *bool `json:"disabled"`
	}
	if err := decodeSmall(w, r, &body); err != nil || body.Disabled == nil {
		writeError(w, http.StatusBadRequest, "bad_request", "body must be {disabled}")
		return
	}
	if *body.Disabled && principalFrom(r).Actor == fmt.Sprintf("user:%d", u.ID) {
		writeError(w, http.StatusConflict, "self", "you cannot disable yourself")
		return
	}
	action := "user.enable"
	if *body.Disabled {
		action = "user.disable"
	}
	au := audit(r, action)
	au.Detail = u.Username
	if err := a.db.SetDisabled(u.ID, *body.Disabled, au); err != nil {
		a.internalError(w, err)
		return
	}
	a.writeUser(w, http.StatusOK, u.ID, nil)
}

// resetPassword gives the user a new random password, returned once; SetPasswordHash
// deletes every session of the user.
func (a *api) resetPassword(w http.ResponseWriter, r *http.Request) {
	u, ok := a.pathUser(w, r)
	if !ok {
		return
	}
	pw := auth.RandomPassword()
	hash, err := auth.HashPassword(pw)
	if err != nil {
		a.internalError(w, err)
		return
	}
	au := audit(r, "user.reset_password")
	au.Detail = u.Username
	if err := a.db.SetPasswordHash(u.Username, hash, au); err != nil {
		a.internalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"password": pw})
}

// parseMPPHex parses three hex digits in 0x100-0x8FF.
func parseMPPHex(s string) (int, bool) {
	n, err := strconv.ParseUint(s, 16, 16)
	if len(s) != 3 || err != nil || n < 0x100 || n > 0x8FF {
		return 0, false
	}
	return int(n), true
}

// grantBody decodes and checks a {lo, hi, access} body, writing the 400 itself.
// The grant's owner is left for the caller to set.
func grantBody(w http.ResponseWriter, r *http.Request) (store.Grant, bool) {
	var body struct{ Lo, Hi, Access string }
	if err := decodeSmall(w, r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", "body must be {lo, hi, access}")
		return store.Grant{}, false
	}
	lo, okLo := parseMPPHex(body.Lo)
	hi, okHi := parseMPPHex(body.Hi)
	if !okLo || !okHi || hi < lo {
		writeError(w, http.StatusBadRequest, "bad_request", "lo and hi must be pages 100-8FF with lo <= hi")
		return store.Grant{}, false
	}
	if body.Access != "read" && body.Access != "write" {
		writeError(w, http.StatusBadRequest, "bad_request", `access must be "read" or "write"`)
		return store.Grant{}, false
	}
	return store.Grant{Lo: lo, Hi: hi, Write: body.Access == "write"}, true
}

func (a *api) createGrant(w http.ResponseWriter, r *http.Request) {
	u, ok := a.pathUser(w, r)
	if !ok {
		return
	}
	g, ok := grantBody(w, r)
	if !ok {
		return
	}
	g.UserID = &u.ID
	a.insertGrant(w, r, g)
}

// insertGrant stores g with its audit row and answers 201 with the grant.
func (a *api) insertGrant(w http.ResponseWriter, r *http.Request, g store.Grant) {
	id, err := a.db.CreateGrant(g, audit(r, "grant.create"))
	if err != nil {
		a.internalError(w, err)
		return
	}
	g.ID = id
	writeJSON(w, http.StatusCreated, grantJSON(g))
}

func (a *api) deleteGrant(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "grant")
	if !ok {
		return
	}
	err := a.db.DeleteGrant(id, audit(r, "grant.delete"))
	if errors.Is(err, store.ErrGrantNotFound) {
		writeError(w, http.StatusNotFound, "not_found", "no such grant")
		return
	}
	if err != nil {
		a.internalError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// changePassword is the self-service change (D-023). It needs the current password,
// takes a token from login's per-username bucket, and on success ends every session
// of the user, this one included, so the user logs in again.
func (a *api) changePassword(w http.ResponseWriter, r *http.Request) {
	s, _ := r.Context().Value(ctxKey{}).(*session)
	if s == nil || s.user == nil || s.key != nil {
		writeError(w, http.StatusForbidden, "forbidden", "not a session")
		return
	}
	var body struct {
		CurrentPassword string `json:"currentPassword"`
		NewPassword     string `json:"newPassword"`
	}
	if err := decodeSmall(w, r, &body); err != nil || body.CurrentPassword == "" || body.NewPassword == "" {
		writeError(w, http.StatusBadRequest, "bad_request", "currentPassword and newPassword are required")
		return
	}
	if !a.userLimit.Allow(s.user.Username) {
		writeError(w, http.StatusTooManyRequests, "rate_limited", "too many login attempts; try again later")
		return
	}
	ip := clientIP(r)
	_, err := auth.Login(a.db, s.user.Username, body.CurrentPassword)
	if errors.Is(err, auth.ErrInvalidCredentials) {
		a.log.Info("change_password", "ok", false, "ip", ip, "username", s.user.Username)
		writeError(w, http.StatusBadRequest, "wrong_password", "current password is incorrect")
		return
	}
	if err != nil {
		a.internalError(w, err)
		return
	}
	hash, err := auth.HashPassword(body.NewPassword)
	if err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", err.Error())
		return
	}
	if err := a.db.SetPasswordHash(s.user.Username, hash, store.Audit{Actor: s.principal.Actor, Action: "user.change_password", Remote: ip}); err != nil {
		a.internalError(w, err)
		return
	}
	a.log.Info("change_password", "ok", true, "ip", ip, "username", s.user.Username)
	http.SetCookie(w, a.cookie("", -1))
	w.WriteHeader(http.StatusNoContent)
}
