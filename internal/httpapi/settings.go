package httpapi

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"

	"ceems/internal/store"
)

// settingsDoc is the document of GET and PUT /api/v1/settings. PagesDir is the config
// value, read-only; it is never part of a PUT body.
type settingsDoc struct {
	SystemName          string `json:"systemName"`
	PagesDir            string `json:"pagesDir"`
	VbitConf            bool   `json:"vbitConf"`
	HeaderTemplate      string `json:"headerTemplate"`
	InitialTeletextPage string `json:"initialTeletextPage"`
	RowAdaptiveMode     bool   `json:"rowAdaptiveMode"`
	LinesPerField       int    `json:"linesPerField"`
	MagazinePriority    string `json:"magazinePriority"`
	StatusDisplay       string `json:"statusDisplay"`
	IndexPageEnabled    bool   `json:"indexPageEnabled"`
	IndexPage           string `json:"indexPage"`
}

// settingsBody is the PUT body; a nil field was absent (or null).
type settingsBody struct {
	SystemName          *string         `json:"systemName"`
	VbitConf            *bool           `json:"vbitConf"`
	HeaderTemplate      *string         `json:"headerTemplate"`
	InitialTeletextPage *string         `json:"initialTeletextPage"`
	RowAdaptiveMode     *bool           `json:"rowAdaptiveMode"`
	LinesPerField       json.RawMessage `json:"linesPerField"` // raw, so only a JSON integer passes
	MagazinePriority    *string         `json:"magazinePriority"`
	StatusDisplay       *string         `json:"statusDisplay"`
	IndexPageEnabled    *bool           `json:"indexPageEnabled"`
	IndexPage           *string         `json:"indexPage"`
}

var (
	initialPage   = regexp.MustCompile(`^[1-8][0-9A-Fa-f]{2}(:[0-9A-Fa-f]{4})?$`)
	indexPageRe   = regexp.MustCompile(`^[1-8][0-9a-fA-F]{2}$`)
	magazinePrios = regexp.MustCompile(`^[1-9](,[1-9]){7}$`)
)

// printable reports whether s holds only characters from 0x20 to 0x7E.
func printable(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] < 0x20 || s[i] > 0x7E {
			return false
		}
	}
	return true
}

// headerError checks a header template: 32 cells of 0x00 to 0x7F, and every % starting
// one of vbit2's tokens. The 32 counts the template as written (vbit2 pads each token).
func headerError(h string) string {
	cells := 0
	for _, r := range h { // invalid UTF-8 and lone surrogates arrive as U+FFFD, which is refused
		if r > 0x7F {
			return "every cell must be from 0x00 to 0x7F"
		}
		cells++
	}
	if cells != 32 {
		return fmt.Sprintf("must be exactly 32 cells, got %d", cells)
	}
	for i := 0; i < len(h); i++ {
		if h[i] != '%' {
			continue
		}
		switch {
		case strings.HasPrefix(h[i:], "%%#"), strings.HasPrefix(h[i:], "%%a"), strings.HasPrefix(h[i:], "%%b"):
			i += 2
		case i+1 < len(h) && strings.IndexByte("demyHMS", h[i+1]) >= 0:
			i++
		default:
			return fmt.Sprintf("unsupported %% sequence at cell %d", i)
		}
	}
	return ""
}

// readSettings decodes and validates a PUT body in full. It writes the 400 itself and
// names the offending key.
func readSettings(w http.ResponseWriter, r *http.Request) (store.Settings, bool) {
	var s store.Settings
	bad := func(key, msg string) (store.Settings, bool) {
		writeError(w, http.StatusBadRequest, "bad_request", key+": "+msg)
		return s, false
	}
	raw, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 4096))
	var tooBig *http.MaxBytesError
	if errors.As(err, &tooBig) {
		writeError(w, http.StatusRequestEntityTooLarge, "too_large", "body is larger than 4 KiB")
		return s, false
	}
	if err != nil {
		return bad("body", "could not read the request body")
	}
	var b settingsBody
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&b); err != nil {
		if rest, ok := strings.CutPrefix(err.Error(), "json: unknown field "); ok {
			return bad(strings.Trim(rest, `"`), "unknown field")
		}
		var te *json.UnmarshalTypeError
		if errors.As(err, &te) {
			return bad(te.Field, "must be a "+te.Value+" of the right type")
		}
		return bad("body", "must be the settings document without pagesDir")
	}
	if dec.More() {
		return bad("body", "must be one JSON object")
	}
	switch {
	case b.SystemName == nil:
		return bad("systemName", "is required")
	case b.VbitConf == nil:
		return bad("vbitConf", "must be a boolean")
	case b.HeaderTemplate == nil:
		return bad("headerTemplate", "is required")
	case b.InitialTeletextPage == nil:
		return bad("initialTeletextPage", "is required")
	case b.RowAdaptiveMode == nil:
		return bad("rowAdaptiveMode", "must be a boolean")
	case b.LinesPerField == nil:
		return bad("linesPerField", "is required")
	case b.MagazinePriority == nil:
		return bad("magazinePriority", "is required")
	case b.StatusDisplay == nil:
		return bad("statusDisplay", "is required")
	case b.IndexPageEnabled == nil:
		return bad("indexPageEnabled", "must be a boolean")
	case b.IndexPage == nil:
		return bad("indexPage", "is required")
	}
	name := strings.TrimSpace(*b.SystemName)
	if n := len(name); n < 1 || n > 40 || !printable(name) {
		return bad("systemName", "must be 1 to 40 characters from 0x20 to 0x7E")
	}
	if msg := headerError(*b.HeaderTemplate); msg != "" {
		return bad("headerTemplate", msg)
	}
	if !initialPage.MatchString(*b.InitialTeletextPage) {
		return bad("initialTeletextPage", "must be a page such as 100, optionally followed by :SSSS")
	}
	// shortcut: 1 to 32 is a guess, since vbit2's real bound is not documented; tighten it once known.
	lines, err := strconv.Atoi(string(b.LinesPerField))
	if err != nil || lines < 1 || lines > 32 {
		return bad("linesPerField", "must be an integer from 1 to 32")
	}
	if !magazinePrios.MatchString(*b.MagazinePriority) {
		return bad("magazinePriority", "must be eight values from 1 to 9, comma separated")
	}
	if len(*b.StatusDisplay) > 20 || !printable(*b.StatusDisplay) || strings.ContainsAny(*b.StatusDisplay, " ;") {
		return bad("statusDisplay", "must be 0 to 20 characters from 0x21 to 0x7E, without ;")
	}
	if !indexPageRe.MatchString(*b.IndexPage) || strings.EqualFold(*b.IndexPage, "8ff") {
		return bad("indexPage", "must be a page from 100 to 8FE, three hex digits")
	}
	return store.Settings{
		SystemName: name, VbitConf: *b.VbitConf, HeaderTemplate: *b.HeaderTemplate,
		InitialTeletextPage: strings.ToUpper(*b.InitialTeletextPage), RowAdaptiveMode: *b.RowAdaptiveMode,
		LinesPerField: lines, MagazinePriority: *b.MagazinePriority, StatusDisplay: *b.StatusDisplay,
		IndexPageEnabled: *b.IndexPageEnabled, IndexPage: strings.ToLower(*b.IndexPage),
	}, true
}

// writeSettings answers with the stored settings document.
func (a *api) writeSettings(w http.ResponseWriter) {
	s, err := a.db.Settings()
	if err != nil {
		a.internalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, settingsDoc{
		SystemName: s.SystemName, PagesDir: a.pagesDir, VbitConf: s.VbitConf, HeaderTemplate: s.HeaderTemplate,
		InitialTeletextPage: s.InitialTeletextPage, RowAdaptiveMode: s.RowAdaptiveMode, LinesPerField: s.LinesPerField,
		MagazinePriority: s.MagazinePriority, StatusDisplay: s.StatusDisplay,
		IndexPageEnabled: s.IndexPageEnabled, IndexPage: s.IndexPage,
	})
}

func (a *api) getSettings(w http.ResponseWriter, r *http.Request) { a.writeSettings(w) }

// updateSettings has no etag: the last write wins.
func (a *api) updateSettings(w http.ResponseWriter, r *http.Request) {
	s, ok := readSettings(w, r)
	if !ok {
		return
	}
	if _, err := a.db.SaveSettings(s, audit(r, "settings.update")); err != nil {
		a.internalError(w, err)
		return
	}
	a.writeSettings(w)
}

// userDoc is the body of a user session's me and login: the user plus the settings
// the editor needs. It returns false after writing the 500.
func (a *api) userDoc(w http.ResponseWriter, u *userJSON) (map[string]any, bool) {
	s, err := a.db.Settings()
	if err != nil {
		a.internalError(w, err)
		return nil, false
	}
	var index any // null when the index link is off
	if s.IndexPageEnabled {
		index = s.IndexPage
	}
	return map[string]any{"user": u, "systemName": s.SystemName, "headerTemplate": s.HeaderTemplate, "indexPage": index}, true
}
