package httpapi

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strings"

	"ceems/internal/store"
)

// fastextMember is one page of a group in a group document.
type fastextMember struct {
	MPP         string `json:"mpp"`
	Description string `json:"description"`
}

// fastextDoc is the document of the fastext group routes. PrevSlot and NextSlot are
// null for a static group.
type fastextDoc struct {
	ID       int64           `json:"id"`
	Name     string          `json:"name"`
	Paged    bool            `json:"paged"`
	Links    []string        `json:"links"`
	Texts    []string        `json:"texts"`
	PrevSlot *int            `json:"prevSlot"`
	NextSlot *int            `json:"nextSlot"`
	Default  bool            `json:"default"`
	Pages    []fastextMember `json:"pages"`
}

// fastextBody is the POST and PUT body; nil fields were absent. The slots may be absent or null.
type fastextBody struct {
	Name     *string   `json:"name"`
	Paged    *bool     `json:"paged"`
	Links    *[]string `json:"links"`
	Texts    *[]string `json:"texts"`
	PrevSlot *int      `json:"prevSlot"`
	NextSlot *int      `json:"nextSlot"`
	Default  *bool     `json:"default"`
}

func fastextJSON(g *store.FastextGroup) fastextDoc {
	d := fastextDoc{ID: g.ID, Name: g.Name, Paged: g.Paged, Links: g.Links[:], Texts: g.Texts[:], PrevSlot: g.PrevSlot, NextSlot: g.NextSlot,
		Default: g.Default, Pages: []fastextMember{}}
	for _, m := range g.Members {
		d.Pages = append(d.Pages, fastextMember{MPP: fmt.Sprintf("%03X", m.MPP), Description: m.Description})
	}
	return d
}

// readFastext decodes and checks a group body, writing the 400 itself. The store checks
// the links and slots, so its ErrInvalid reaches the caller as a 400 too.
func readFastext(w http.ResponseWriter, r *http.Request) (*store.FastextGroup, bool) {
	body, ok := readBody(w, r)
	if !ok {
		return nil, false
	}
	var b fastextBody
	dec := json.NewDecoder(bytes.NewReader(body))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&b); err != nil || b.Name == nil || b.Paged == nil || b.Links == nil || b.Default == nil {
		writeError(w, http.StatusBadRequest, "bad_request", "body must be {name, paged, links, texts (optional), prevSlot, nextSlot, default}")
		return nil, false
	}
	if len(*b.Links) != 4 {
		writeError(w, http.StatusBadRequest, "bad_request", "links: must be four pages")
		return nil, false
	}
	if b.Texts != nil && len(*b.Texts) != 4 {
		writeError(w, http.StatusBadRequest, "bad_request", "texts: must be four strings")
		return nil, false
	}
	g := &store.FastextGroup{Name: strings.TrimSpace(*b.Name), Paged: *b.Paged, PrevSlot: b.PrevSlot, NextSlot: b.NextSlot, Default: *b.Default}
	copy(g.Links[:], *b.Links)
	if b.Texts != nil {
		copy(g.Texts[:], *b.Texts)
	}
	return g, true
}

// fastextError maps a store error from a group call to its response.
func (a *api) fastextError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, store.ErrNotFound):
		writeError(w, http.StatusNotFound, "not_found", "no such group or page")
	case errors.Is(err, store.ErrInvalid):
		writeError(w, http.StatusBadRequest, "bad_request", err.Error())
	case errors.Is(err, store.ErrNameTaken):
		writeError(w, http.StatusConflict, "conflict", "a group with that name already exists")
	case errors.Is(err, store.ErrInGroup):
		writeError(w, http.StatusConflict, "in_group", err.Error())
	default:
		a.internalError(w, err)
	}
}

// writeFastext answers status with group id's document.
func (a *api) writeFastext(w http.ResponseWriter, status int, id int64) {
	g, err := a.db.FastextGroup(id)
	if err != nil {
		a.fastextError(w, err)
		return
	}
	writeJSON(w, status, fastextJSON(g))
}

func (a *api) listFastext(w http.ResponseWriter, r *http.Request) {
	gs, err := a.db.FastextGroups()
	if err != nil {
		a.internalError(w, err)
		return
	}
	sort.Slice(gs, func(i, j int) bool { return gs[i].Name < gs[j].Name })
	out := []fastextDoc{}
	for i := range gs {
		out = append(out, fastextJSON(&gs[i]))
	}
	writeJSON(w, http.StatusOK, map[string]any{"groups": out})
}

func (a *api) getFastext(w http.ResponseWriter, r *http.Request) {
	if id, ok := pathID(w, r, "group"); ok {
		a.writeFastext(w, http.StatusOK, id)
	}
}

func (a *api) createFastext(w http.ResponseWriter, r *http.Request) {
	g, ok := readFastext(w, r)
	if !ok {
		return
	}
	id, err := a.db.CreateFastextGroup(g, audit(r, ""))
	if err != nil {
		a.fastextError(w, err)
		return
	}
	a.writeFastext(w, http.StatusCreated, id)
}

func (a *api) updateFastext(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "group")
	if !ok {
		return
	}
	g, ok := readFastext(w, r)
	if !ok {
		return
	}
	g.ID = id
	if err := a.db.UpdateFastextGroup(g, audit(r, "")); err != nil {
		a.fastextError(w, err)
		return
	}
	a.writeFastext(w, http.StatusOK, id)
}

func (a *api) deleteFastext(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "group")
	if !ok {
		return
	}
	if err := a.db.DeleteFastextGroup(id, audit(r, "")); err != nil {
		a.fastextError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *api) setFastextPages(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r, "group")
	if !ok {
		return
	}
	body, ok := readBody(w, r)
	if !ok {
		return
	}
	var b struct {
		Pages *[]string `json:"pages"`
	}
	if err := json.Unmarshal(body, &b); err != nil || b.Pages == nil {
		writeError(w, http.StatusBadRequest, "bad_request", "body must be {pages}")
		return
	}
	mpps := []int{}
	for _, s := range *b.Pages {
		mpp, ok := parseMPPHex(s)
		if !ok {
			writeError(w, http.StatusBadRequest, "bad_request", "pages: each must be three hex digits from 100 to 8FF")
			return
		}
		mpps = append(mpps, mpp)
	}
	if err := a.db.SetFastextGroupPages(id, mpps, audit(r, "")); err != nil {
		a.fastextError(w, err)
		return
	}
	a.writeFastext(w, http.StatusOK, id)
}
