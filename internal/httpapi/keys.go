package httpapi

import (
	"errors"
	"net/http"
	"regexp"
	"strings"
	"time"

	"ceems/internal/auth"
	"ceems/internal/store"
)

// keyDoc is an API key as the admin routes send it. It never carries the secret or its hash.
type keyDoc struct {
	ID         string     `json:"id"`
	Name       string     `json:"name"`
	ReadOnly   bool       `json:"readOnly"`
	CreatedBy  string     `json:"createdBy"`
	CreatedAt  string     `json:"createdAt"`
	ExpiresAt  *string    `json:"expiresAt"`
	RevokedAt  *string    `json:"revokedAt"`
	LastUsedAt *string    `json:"lastUsedAt"`
	Grants     []grantDoc `json:"grants"`
}

// orNull is nil for "", so the field is sent as null.
func orNull(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func keyJSON(k store.APIKey, gs []store.Grant) keyDoc {
	d := keyDoc{ID: k.ID, Name: k.Name, ReadOnly: k.ReadOnly, CreatedBy: k.CreatedBy, CreatedAt: k.CreatedAt,
		ExpiresAt: orNull(k.ExpiresAt), RevokedAt: orNull(k.RevokedAt), LastUsedAt: orNull(k.LastUsedAt), Grants: []grantDoc{}}
	for _, g := range gs {
		d.Grants = append(d.Grants, grantJSON(g))
	}
	return d
}

var keyID = regexp.MustCompile(`^[0-9a-f]{16}$`)

// pathKey loads the {id} key, revoked or not, writing the 404 or 500 itself.
func (a *api) pathKey(w http.ResponseWriter, r *http.Request) (*store.APIKey, bool) {
	id := r.PathValue("id")
	if keyID.MatchString(id) {
		// shortcut: scans every key, fine for a hobby install's handful; add Store.Key if keys grow.
		ks, err := a.db.Keys()
		if err != nil {
			a.internalError(w, err)
			return nil, false
		}
		for i := range ks {
			if ks[i].ID == id {
				return &ks[i], true
			}
		}
	}
	writeError(w, http.StatusNotFound, "not_found", "no such key")
	return nil, false
}

func (a *api) listKeys(w http.ResponseWriter, r *http.Request) {
	ks, err := a.db.Keys()
	if err != nil {
		a.internalError(w, err)
		return
	}
	gs, err := a.db.AllGrants()
	if err != nil {
		a.internalError(w, err)
		return
	}
	byKey := map[string][]store.Grant{}
	for _, g := range gs {
		if g.KeyID != "" {
			byKey[g.KeyID] = append(byKey[g.KeyID], g)
		}
	}
	out := []keyDoc{}
	for _, k := range ks {
		out = append(out, keyJSON(k, byKey[k.ID]))
	}
	writeJSON(w, http.StatusOK, map[string]any{"keys": out})
}

// createKey makes a key; its token is returned once in this response and only
// the SHA-256 of its secret is stored.
func (a *api) createKey(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name      string  `json:"name"`
		ReadOnly  bool    `json:"readOnly"`
		ExpiresAt *string `json:"expiresAt"`
	}
	if err := decodeSmall(w, r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", "body must be {name, readOnly, expiresAt?}")
		return
	}
	name := strings.TrimSpace(body.Name)
	if name == "" || len(name) > 100 || strings.ContainsAny(name, "\r\n") {
		writeError(w, http.StatusBadRequest, "bad_request", "name must be one line of 1-100 bytes")
		return
	}
	now := time.Now().UTC()
	k := store.APIKey{Name: name, ReadOnly: body.ReadOnly, CreatedBy: principalFrom(r).Actor, CreatedAt: now.Format(time.RFC3339)}
	if body.ExpiresAt != nil {
		t, err := time.Parse(time.RFC3339, *body.ExpiresAt)
		if err != nil || !t.After(now) {
			writeError(w, http.StatusBadRequest, "bad_request", "expiresAt must be an RFC 3339 time in the future")
			return
		}
		k.ExpiresAt = t.UTC().Format(time.RFC3339) // UTC, so LiveKey's text comparison holds
	}
	id, token, hash := auth.NewKey()
	k.ID = id
	if err := a.db.CreateKey(k, hash, audit(r, "key.create")); err != nil {
		a.internalError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"key": keyJSON(k, nil), "token": token})
}

// revokeKey revokes; revoking again is a 204 with nothing written.
func (a *api) revokeKey(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !keyID.MatchString(id) {
		writeError(w, http.StatusNotFound, "not_found", "no such key")
		return
	}
	err := a.db.RevokeKey(id, audit(r, "key.revoke"))
	if errors.Is(err, store.ErrKeyNotFound) {
		writeError(w, http.StatusNotFound, "not_found", "no such key")
		return
	}
	if err != nil {
		a.internalError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *api) createKeyGrant(w http.ResponseWriter, r *http.Request) {
	k, ok := a.pathKey(w, r)
	if !ok {
		return
	}
	if k.RevokedAt != "" {
		writeError(w, http.StatusConflict, "revoked", "the key is revoked")
		return
	}
	g, ok := grantBody(w, r)
	if !ok {
		return
	}
	g.KeyID = k.ID
	a.insertGrant(w, r, g)
}
