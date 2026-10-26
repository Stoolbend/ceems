package httpapi

import (
	"fmt"
	"net/http"
)

// listPages returns only the pages the principal may read (ADR 0003).
func (a *api) listPages(w http.ResponseWriter, r *http.Request) {
	all, err := a.db.Pages()
	if err != nil {
		a.internalError(w, err)
		return
	}
	p := principalFrom(r)
	out := []map[string]any{}
	for _, pg := range all {
		if !p.Can(pg.MPP, false) {
			continue
		}
		out = append(out, map[string]any{
			"mpp": fmt.Sprintf("%03X", pg.MPP), "description": pg.Description, "subpages": pg.Subpages,
			"transmit": pg.Transmit, "updatedAt": pg.UpdatedAt, "updatedBy": pg.UpdatedBy,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"pages": out})
}
