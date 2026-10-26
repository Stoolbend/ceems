package httpapi

import (
	"errors"
	"fmt"
	"math"
	"net/http"

	"ceems/internal/output"
	"ceems/internal/store"
	"ceems/internal/tti"
)

// defaultSeconds is how long a subpage with no readable CT line is shown.
const defaultSeconds = 8

type previewSubpage struct {
	Rows    []*string `json:"rows"`
	Seconds float64   `json:"seconds"`
}

// previewDoc is everything a viewer needs to play a page as a TV shows it, and
// nothing an editor needs. It must not depend on the caller beyond the read check.
type previewDoc struct {
	MPP            string           `json:"mpp"`
	Description    string           `json:"description"`
	HeaderTemplate string           `json:"headerTemplate"`
	Subpages       []previewSubpage `json:"subpages"`
}

func (a *api) getPreview(w http.ResponseWriter, r *http.Request, mpp int) {
	p, err := a.db.Page(mpp)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not_found", "no such page")
		return
	}
	if err != nil {
		a.internalError(w, err)
		return
	}
	set, err := a.db.Settings()
	if err != nil {
		a.internalError(w, err)
		return
	}
	pages, err := a.db.TransmitPages()
	if err != nil {
		a.internalError(w, err)
		return
	}
	// A page that is off air and alone in its magazine has no estimate: n cycles are n seconds.
	cycle := 1.0
	if c, ok := output.CycleTimes(pages, set)[mpp>>8]; ok {
		cycle = c
	}
	d := previewDoc{MPP: fmt.Sprintf("%03X", mpp), Description: p.Description, HeaderTemplate: set.HeaderTemplate, Subpages: []previewSubpage{}}
	t, err := tti.Parse(p.TTI)
	if err != nil { // stored bytes were validated on write
		writeJSON(w, http.StatusOK, d)
		return
	}
	for i := 0; i < t.Subpages(); i++ {
		s := previewSubpage{Rows: make([]*string, 25), Seconds: defaultSeconds}
		if body, ok := t.Field(i, "CT"); ok {
			if sec, mode, ok := tti.ParseCT(body); ok {
				s.Seconds = float64(sec)
				if mode == 'C' {
					s.Seconds = math.Round(float64(sec)*cycle*10) / 10
				}
			}
		}
		for row := range s.Rows {
			if c, ok := t.Row(i, row); ok {
				s.Rows[row] = &c
			}
		}
		d.Subpages = append(d.Subpages, s)
	}
	writeJSON(w, http.StatusOK, d)
}
