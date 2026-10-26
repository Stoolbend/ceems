package httpapi

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"ceems/internal/jobs"
	"ceems/internal/store"
	"ceems/internal/tmpl"
)

// slotDoc is one parsed slot of a template document; sub is 0-based, align is
// "left", "centre" or "right".
type slotDoc struct {
	Name  string `json:"name"`
	Sub   int    `json:"sub"`
	Row   int    `json:"row"`
	Col   int    `json:"col"`
	Width int    `json:"width"`
	Align string `json:"align"`
}

// templateDoc is the document of GET /api/v1/templates/{id} and of a create or update.
type templateDoc struct {
	ID       int64             `json:"id"`
	Name     string            `json:"name"`
	RowRules map[string]string `json:"rowRules"`
	Slots    []slotDoc         `json:"slots"`
	TTI      string            `json:"tti"` // base64
}

// templateBody is the POST and PUT body; nil fields were absent.
type templateBody struct {
	Name     *string           `json:"name"`
	TTI      *string           `json:"tti"`
	RowRules map[string]string `json:"rowRules"`
}

var alignNames = map[byte]string{0: "left", '^': "centre", '>': "right"}

// parseTemplate parses a stored template; stored rows were validated on write.
func parseTemplate(t *store.Template) (*tmpl.Template, map[string]string, error) {
	rules := map[string]string{}
	if err := json.Unmarshal([]byte(t.RowRules), &rules); err != nil {
		return nil, nil, err
	}
	p, err := tmpl.Parse(t.TTI, rules)
	return p, rules, err
}

func templateJSON(t *store.Template) (templateDoc, error) {
	p, rules, err := parseTemplate(t)
	if err != nil {
		return templateDoc{}, err
	}
	d := templateDoc{ID: t.ID, Name: t.Name, RowRules: rules, Slots: []slotDoc{}, TTI: base64.StdEncoding.EncodeToString(t.TTI)}
	for _, s := range p.Slots() {
		d.Slots = append(d.Slots, slotDoc{Name: s.Name, Sub: s.Sub, Row: s.Row, Col: s.Col, Width: s.Width, Align: alignNames[s.Align]})
	}
	return d, nil
}

// templateID returns the {id} path value, writing a 404 itself when it is not a positive integer.
func templateID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id < 1 {
		writeError(w, http.StatusNotFound, "not_found", "no such template")
		return 0, false
	}
	return id, true
}

// readTemplate decodes and validates a template body; every field is required when all
// is true (PUT), and rowRules may be omitted otherwise (POST). It writes the 400 itself.
func readTemplate(w http.ResponseWriter, r *http.Request, all bool) (*store.Template, bool) {
	body, ok := readBody(w, r)
	if !ok {
		return nil, false
	}
	var b templateBody
	if err := json.Unmarshal(body, &b); err != nil || b.Name == nil || b.TTI == nil || (all && b.RowRules == nil) {
		writeError(w, http.StatusBadRequest, "bad_request", "body must be {name, tti, rowRules}")
		return nil, false
	}
	name := strings.TrimSpace(*b.Name)
	if name == "" || len(name) > 100 || strings.ContainsAny(name, "\r\n") {
		writeError(w, http.StatusBadRequest, "bad_request", "name must be one line of 1-100 bytes")
		return nil, false
	}
	raw, err := base64.StdEncoding.DecodeString(*b.TTI)
	if err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", "tti must be base64")
		return nil, false
	}
	if b.RowRules == nil {
		b.RowRules = map[string]string{}
	}
	if _, err := tmpl.Parse(raw, b.RowRules); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_template", err.Error())
		return nil, false
	}
	rules, _ := json.Marshal(b.RowRules)
	return &store.Template{Name: name, TTI: raw, RowRules: string(rules)}, true
}

// templateError maps a store error from a template call to its response.
func (a *api) templateError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, store.ErrNotFound):
		writeError(w, http.StatusNotFound, "not_found", "no such template")
	case errors.Is(err, store.ErrNameTaken):
		writeError(w, http.StatusConflict, "exists", "a template with that name already exists")
	case errors.Is(err, store.ErrInUse):
		writeError(w, http.StatusConflict, "in_use", err.Error())
	default:
		a.internalError(w, err)
	}
}

func (a *api) listTemplates(w http.ResponseWriter, r *http.Request) {
	ts, err := a.db.Templates()
	if err != nil {
		a.internalError(w, err)
		return
	}
	type item struct {
		ID        int64  `json:"id"`
		Name      string `json:"name"`
		SlotCount int    `json:"slotCount"`
	}
	out := []item{}
	for _, t := range ts {
		p, _, err := parseTemplate(&t)
		if err != nil {
			a.internalError(w, err)
			return
		}
		out = append(out, item{ID: t.ID, Name: t.Name, SlotCount: len(p.Slots())})
	}
	writeJSON(w, http.StatusOK, map[string]any{"templates": out})
}

// writeTemplate answers status with the stored template id's document.
func (a *api) writeTemplate(w http.ResponseWriter, status int, id int64) {
	t, err := a.db.Template(id)
	if err != nil {
		a.templateError(w, err)
		return
	}
	d, err := templateJSON(t)
	if err != nil {
		a.internalError(w, err)
		return
	}
	writeJSON(w, status, d)
}

func (a *api) createTemplate(w http.ResponseWriter, r *http.Request) {
	t, ok := readTemplate(w, r, false)
	if !ok {
		return
	}
	id, err := a.db.CreateTemplate(t, audit(r, ""))
	if err != nil {
		a.templateError(w, err)
		return
	}
	a.writeTemplate(w, http.StatusCreated, id)
}

func (a *api) getTemplate(w http.ResponseWriter, r *http.Request) {
	if id, ok := templateID(w, r); ok {
		a.writeTemplate(w, http.StatusOK, id)
	}
}

// shortcut: the check and the write are not atomic, so a binding saved between them can
// be left with a slot it has no value for; the worst result is one failed run that keeps
// the previous page. Upgrade if there is ever more than one admin.
func (a *api) updateTemplate(w http.ResponseWriter, r *http.Request) {
	id, ok := templateID(w, r)
	if !ok {
		return
	}
	t, ok := readTemplate(w, r, true)
	if !ok {
		return
	}
	t.ID = id
	if msg, err := a.unfilledSlots(t); err != nil {
		a.internalError(w, err)
		return
	} else if msg != "" {
		writeError(w, http.StatusConflict, "in_use", msg)
		return
	}
	if err := a.db.UpdateTemplate(t, audit(r, "")); err != nil {
		a.templateError(w, err)
		return
	}
	a.writeTemplate(w, http.StatusOK, id)
}

// unfilledSlots checks the new template t against every binding that uses it and
// returns the refusal message, or "" when each binding still has a value for every slot.
// It never quotes a source or a sheet id.
func (a *api) unfilledSlots(t *store.Template) (string, error) {
	p, _, err := parseTemplate(t)
	if err != nil {
		return "", err
	}
	bs, err := a.db.Bindings()
	if err != nil {
		return "", err
	}
	var parts []string
	for _, b := range bs {
		if b.TemplateID != t.ID {
			continue
		}
		cfg, err := jobs.ParseSourceConfig(b.Source)
		if err != nil {
			return "", err
		}
		var static map[string]string
		if err := json.Unmarshal([]byte(b.Static), &static); err != nil {
			return "", err
		}
		if m := missingSlots(p, jobs.DataKeys(cfg), static); len(m) > 0 {
			for i := range m {
				m[i] = "slot " + m[i] + " needs a value"
			}
			parts = append(parts, fmt.Sprintf("binding %s (page %03X): %s", b.Name, b.MPP, strings.Join(m, ", ")))
		}
	}
	return strings.Join(parts, "; "), nil
}

func (a *api) deleteTemplate(w http.ResponseWriter, r *http.Request) {
	id, ok := templateID(w, r)
	if !ok {
		return
	}
	if err := a.db.DeleteTemplate(id, audit(r, "")); err != nil {
		a.templateError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *api) downloadTemplate(w http.ResponseWriter, r *http.Request) {
	id, ok := templateID(w, r)
	if !ok {
		return
	}
	t, err := a.db.Template(id)
	if err != nil {
		a.templateError(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="T%d.tti"`, id))
	w.Write(t.TTI)
}
