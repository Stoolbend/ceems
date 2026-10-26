package httpapi

import (
	"encoding/json"
	"net/http"
	"testing"

	"ceems/internal/store"
	"ceems/internal/tti"
)

// importCT stores a transmitting one-subpage page whose CT line is ct ("" removes the line).
func (e *env) importCT(mpp int, ct string) {
	e.t.Helper()
	p := tti.NewPage(mpp)
	p.SetField(0, "PS", "8000")
	if ct == "" {
		for i, l := range p.Lines {
			if l.Code() == "CT" {
				p.Lines = append(p.Lines[:i], p.Lines[i+1:]...)
				break
			}
		}
	} else {
		p.SetField(0, "CT", ct)
	}
	if _, err := e.db.ImportPages([]store.ImportFile{{Path: "x.tti", TTI: p.Bytes()}}, "cli"); err != nil {
		e.t.Fatal(err)
	}
}

func (e *env) preview(mpp string, c *http.Cookie) map[string]any {
	e.t.Helper()
	rec := e.do("GET", "/api/v1/preview/"+mpp, "", c)
	if rec.Code != http.StatusOK {
		e.t.Fatalf("preview %s: %d %s", mpp, rec.Code, rec.Body.String())
	}
	var m map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &m); err != nil {
		e.t.Fatal(err)
	}
	return m
}

func seconds(d map[string]any) float64 {
	return d["subpages"].([]any)[0].(map[string]any)["seconds"].(float64)
}

func TestPreviewSeconds(t *testing.T) {
	e := newEnv(t, false)
	e.addUser("conor", true)
	c := e.login("conor")
	e.importCT(0x100, "4,T")
	e.importCT(0x101, "2,C")
	e.importCT(0x102, "")
	// Hand-computed: L = 1 gives C = 45 packets/s and one magazine gets all of it. Each page
	// has R = 24 rows + the FL line = 25. A = (1 + 3*(1+25))/50 = 1.58 s; D = 1 + 3*26 = 79
	// packets = 1.7556 s. T = 1.7556 s, so 2 cycles = 3.5 s.
	e.exec("UPDATE settings SET value = '1' WHERE key = 'lines_per_field'")
	e.exec("UPDATE settings SET value = 'false' WHERE key = 'row_adaptive_mode'")
	set, err := e.db.Settings()
	if err != nil {
		t.Fatal(err)
	}

	d := e.preview("100", c)
	if len(d) != 4 || seconds(d) != 4 {
		t.Errorf("CT,4,T: %v", d)
	}
	for k := range d["subpages"].([]any)[0].(map[string]any) {
		if k != "rows" && k != "seconds" {
			t.Errorf("subpage key %q", k)
		}
	}
	if len(d["subpages"].([]any)[0].(map[string]any)) != 2 || len(d["subpages"].([]any)[0].(map[string]any)["rows"].([]any)) != 25 {
		t.Errorf("subpage shape %v", d["subpages"])
	}
	if got := seconds(e.preview("101", c)); got != 3.5 {
		t.Errorf("CT,2,C = %v, want 3.5", got)
	}
	if got := seconds(e.preview("102", c)); got != 8 {
		t.Errorf("no CT = %v, want 8", got)
	}
	if d["mpp"] != "100" || d["headerTemplate"] != set.HeaderTemplate {
		t.Errorf("mpp/header %v %v", d["mpp"], d["headerTemplate"])
	}
}

func TestPreviewOffAirPageUsesOneSecondCycle(t *testing.T) {
	e := newEnv(t, false)
	e.addUser("conor", true)
	e.importCT(0x300, "5,C")
	e.exec("UPDATE pages SET transmit = 0")
	if got := seconds(e.preview("300", e.login("conor"))); got != 5 {
		t.Errorf("off-air 5 cycles = %v s, want 5", got)
	}
}

func TestPreviewRefusesPageOutsideGrant(t *testing.T) {
	e := newEnv(t, false)
	e.importPage("P100.tti")
	e.importPage("P101.tti")
	uid := e.addUser("viewer", false)
	e.grant(uid, 0x100, 0x100, false)
	c := e.login("viewer")
	wantStatus(t, e.do("GET", "/api/v1/preview/101", "", c), http.StatusNotFound, noSuchPage)
	wantStatus(t, e.do("GET", "/api/v1/pages/101", "", c), http.StatusNotFound, noSuchPage)
	if rec := e.do("GET", "/api/v1/preview/100", "", c); rec.Code != http.StatusOK {
		t.Errorf("granted page: %d", rec.Code)
	}
}
