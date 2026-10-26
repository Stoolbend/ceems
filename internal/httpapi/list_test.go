package httpapi

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"testing"
)

func importAll(e *env) {
	e.t.Helper()
	paths, err := filepath.Glob("../../testdata/sample/pages/*.tti")
	if err != nil || len(paths) != 30 {
		e.t.Fatalf("fixtures: %d, err %v", len(paths), err)
	}
	for _, p := range paths {
		if _, err := os.Stat(p); err != nil {
			e.t.Fatal(err)
		}
		e.importPage(filepath.Base(p))
	}
}

type listBody struct {
	Pages []struct {
		MPP         string `json:"mpp"`
		Description string `json:"description"`
		Subpages    int    `json:"subpages"`
		Transmit    bool   `json:"transmit"`
		UpdatedAt   string `json:"updatedAt"`
		UpdatedBy   string `json:"updatedBy"`
	} `json:"pages"`
}

func TestListPagesMatchesDatabase(t *testing.T) {
	e := newEnv(t, false)
	importAll(e)
	e.addUser("conor", true)
	rec := e.do("GET", "/api/v1/pages", "", e.login("conor"))
	if rec.Code != http.StatusOK {
		t.Fatalf("got %d %s", rec.Code, rec.Body.String())
	}
	var got listBody
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	want, err := e.db.Pages()
	if err != nil || len(want) != 30 || len(got.Pages) != 30 {
		t.Fatalf("db %d, api %d, err %v", len(want), len(got.Pages), err)
	}
	for i, w := range want {
		g := got.Pages[i]
		if g.MPP != fmt.Sprintf("%03X", w.MPP) || g.Description != w.Description || g.Subpages != w.Subpages ||
			g.Transmit != w.Transmit || g.UpdatedAt != w.UpdatedAt || g.UpdatedBy != w.UpdatedBy {
			t.Errorf("row %d: api %+v, db %+v", i, g, w)
		}
	}
}

func TestListPagesNonAdminWithoutGrantsIsEmpty(t *testing.T) {
	e := newEnv(t, false)
	importAll(e)
	e.addUser("viewer", false)
	wantStatus(t, e.do("GET", "/api/v1/pages", "", e.login("viewer")), http.StatusOK, `{"pages":[]}`)
}
