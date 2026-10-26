package store

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"ceems/internal/tti"
)

var admin = Audit{Actor: "user:1", Remote: "192.0.2.1"}

// boundEnv imports P100, creates a template and a binding on 0x100, and returns the
// store and the binding id.
func boundEnv(t *testing.T) (*Store, int64) {
	t.Helper()
	s, _ := savedEnv(t)
	tid, err := s.CreateTemplate(&Template{Name: "scores", TTI: tti.NewPage(0x100).Bytes(), RowRules: "{}"}, admin)
	if err != nil {
		t.Fatal(err)
	}
	id, err := s.CreateBinding(&Binding{Name: "pacman", TemplateID: tid, MPP: 0x100, Source: `{"sheet":"secret-sheet"}`, Static: `{"t":"x"}`}, admin)
	if err != nil {
		t.Fatal(err)
	}
	return s, id
}

func TestCreateBindingAttachesExistingPage(t *testing.T) {
	s, id := boundEnv(t)
	p, err := s.Page(0x100)
	if err != nil || p.BindingID == nil || *p.BindingID != id || p.BindingName != "pacman" {
		t.Fatalf("page binding %v name %q err %v", p.BindingID, p.BindingName, err)
	}
	if !bytes.Equal(p.TTI, fixture(t, "P100.tti").TTI) {
		t.Error("attaching changed the page bytes")
	}
	if b, err := s.Binding(id); err != nil || b.MPP != 0x100 || b.Static != `{"t":"x"}` {
		t.Errorf("binding %+v err %v", b, err)
	}
	if bs, err := s.Bindings(); err != nil || len(bs) != 1 || bs[0].ID != id {
		t.Errorf("bindings %+v err %v", bs, err)
	}
}

func TestCreateBindingCreatesAbsentPage(t *testing.T) {
	s, _ := boundEnv(t)
	tid := int64(1)
	id, err := s.CreateBinding(&Binding{Name: "galaxian", TemplateID: tid, MPP: 0x81A, Source: "{}", Static: "{}"}, admin)
	if err != nil {
		t.Fatal(err)
	}
	p, err := s.Page(0x81A)
	if err != nil || p.BindingID == nil || *p.BindingID != id || !bytes.Equal(p.TTI, tti.NewPage(0x81A).Bytes()) || p.UpdatedBy != "user:1" {
		t.Fatalf("page err %v", err)
	}
}

func TestCreateBindingOnBoundPage(t *testing.T) {
	s, _ := boundEnv(t)
	if _, err := s.CreateBinding(&Binding{Name: "other", TemplateID: 1, MPP: 0x100, Source: "{}", Static: "{}"}, admin); !errors.Is(err, ErrBound) {
		t.Fatalf("got %v", err)
	}
	if n := scalar(t, s, "SELECT count(*) FROM bindings"); n != 1 {
		t.Errorf("bindings %d", n)
	}
	if _, err := s.CreateBinding(&Binding{Name: "pacman", TemplateID: 1, MPP: 0x101, Source: "{}", Static: "{}"}, admin); !errors.Is(err, ErrNameTaken) {
		t.Fatalf("duplicate name: %v", err)
	}
	if _, err := s.Page(0x101); !errors.Is(err, ErrNotFound) {
		t.Error("a failed create left a page behind")
	}
}

func TestUpdateBinding(t *testing.T) {
	s, id := boundEnv(t)
	b, _ := s.Binding(id)
	b.Name, b.Static = "pacman2", `{"t":"y"}`
	if err := s.UpdateBinding(b, admin); err != nil {
		t.Fatal(err)
	}
	if got, _ := s.Binding(id); got.Name != "pacman2" || got.Static != `{"t":"y"}` {
		t.Errorf("binding %+v", got)
	}
	if err := s.UpdateBinding(&Binding{ID: 99}, admin); !errors.Is(err, ErrNotFound) {
		t.Errorf("missing: %v", err)
	}
}

// generated returns P100 with its row 1 set to text, still for mpp 0x100.
func generated(t *testing.T, text string) *tti.Page {
	return changedPage(t, fixture(t, "P100.tti").TTI, text)
}

func TestSaveGeneratedWrites(t *testing.T) {
	s, id := boundEnv(t)
	p := generated(t, "GENERATED")
	changed, err := s.SaveGenerated(id, p, Audit{Remote: "r"})
	if err != nil || !changed {
		t.Fatalf("changed %v err %v", changed, err)
	}
	got, _ := s.Page(0x100)
	if !bytes.Equal(got.TTI, p.Bytes()) || got.BindingID == nil || *got.BindingID != id || got.UpdatedBy != "job:binding:1" {
		t.Errorf("page: bytes equal %v, binding %v, updated_by %q", bytes.Equal(got.TTI, p.Bytes()), got.BindingID, got.UpdatedBy)
	}
	if n := scalar(t, s, "SELECT count(*) FROM page_versions"); n != 1 {
		t.Errorf("versions %d", n)
	}
	// identical bytes: nothing written
	changed, err = s.SaveGenerated(id, generated(t, "GENERATED"), Audit{})
	if err != nil || changed {
		t.Fatalf("identical: changed %v err %v", changed, err)
	}
	if n := scalar(t, s, "SELECT count(*) FROM audit WHERE action = 'page.generate'"); n != 1 {
		t.Errorf("generate audit rows %d", n)
	}
	if n := scalar(t, s, "SELECT count(*) FROM page_versions"); n != 1 {
		t.Errorf("versions %d after identical save", n)
	}
}

func TestSaveGeneratedAuditsAsJob(t *testing.T) { // mutation F
	s, id := boundEnv(t)
	p := generated(t, "GENERATED")
	if _, err := s.SaveGenerated(id, p, Audit{Actor: "user:1", Action: "page.update", Remote: "192.0.2.9"}); err != nil {
		t.Fatal(err)
	}
	var actor, action, target, detail, remote string
	if err := s.db.QueryRow("SELECT actor, action, target, detail, remote FROM audit ORDER BY id DESC LIMIT 1").Scan(&actor, &action, &target, &detail, &remote); err != nil {
		t.Fatal(err)
	}
	if actor != "job:binding:1" || action != "page.generate" || target != "page:100" || detail != "sha256:"+etagOf(p.Bytes()) || remote != "192.0.2.9" {
		t.Errorf("audit %q %q %q %q %q", actor, action, target, detail, remote)
	}
	if got, _ := s.Page(0x100); got.UpdatedBy != "job:binding:1" {
		t.Errorf("updated_by %q", got.UpdatedBy)
	}
}

// nothingWritten fails when SaveGenerated left a page change, version or page audit row.
func nothingWritten(t *testing.T, s *Store) {
	t.Helper()
	if n := scalar(t, s, "SELECT count(*) FROM page_versions") + scalar(t, s, "SELECT count(*) FROM audit WHERE target LIKE 'page:%'"); n != 0 {
		t.Errorf("%d version or page audit rows written", n)
	}
	for _, mpp := range []int{0x100, 0x101} {
		if p, err := s.Page(mpp); err == nil && p.UpdatedBy != "cli" && p.UpdatedBy != "user:1" {
			t.Errorf("page %03X rewritten by %q", mpp, p.UpdatedBy)
		}
	}
}

func TestSaveGeneratedWrongMPP(t *testing.T) { // mutation B
	s, id := boundEnv(t)
	if _, err := s.ImportPages([]ImportFile{fixture(t, "P101.tti")}, "cli"); err != nil {
		t.Fatal(err)
	}
	p := changedPage(t, fixture(t, "P101.tti").TTI, "HIJACK")
	_, err := s.SaveGenerated(id, p, Audit{})
	if err == nil || !strings.Contains(err.Error(), "101") || !strings.Contains(err.Error(), "100") {
		t.Fatalf("got %v", err)
	}
	nothingWritten(t, s)
	if got, _ := s.Page(0x101); !bytes.Equal(got.TTI, fixture(t, "P101.tti").TTI) {
		t.Error("page 101 changed")
	}
}

func TestSaveGeneratedUnbound(t *testing.T) { // mutation C
	s, id := boundEnv(t)
	if _, err := s.db.Exec("UPDATE pages SET binding_id = NULL"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.SaveGenerated(id, generated(t, "GENERATED"), Audit{}); !errors.Is(err, ErrNotBound) {
		t.Fatalf("unbound: got %v", err)
	}
	nothingWritten(t, s)
	// bound to another binding
	if _, err := s.db.Exec("UPDATE pages SET binding_id = 7"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.SaveGenerated(id, generated(t, "GENERATED"), Audit{}); !errors.Is(err, ErrNotBound) {
		t.Fatalf("other binding: got %v", err)
	}
	nothingWritten(t, s)
	if _, err := s.SaveGenerated(99, generated(t, "GENERATED"), Audit{}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("no binding: got %v", err)
	}
}

func TestBoundPageRefusesHumanWrites(t *testing.T) {
	s, _ := boundEnv(t)
	if _, err := s.SavePage(0x100, generated(t, "HUMAN"), "", audit); !errors.Is(err, ErrGenerated) {
		t.Fatalf("save: %v", err)
	}
	if err := s.DeletePage(0x100, "", audit); !errors.Is(err, ErrGenerated) {
		t.Fatalf("delete: %v", err)
	}
}

func TestDeleteBindingDetaches(t *testing.T) {
	s, id := boundEnv(t)
	if _, err := s.SaveGenerated(id, generated(t, "LAST"), Audit{}); err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteBinding(id, admin); err != nil {
		t.Fatal(err)
	}
	p, _ := s.Page(0x100)
	if p.BindingID != nil || p.BindingName != "" || !bytes.Equal(p.TTI, generated(t, "LAST").Bytes()) {
		t.Fatalf("page binding %v name %q", p.BindingID, p.BindingName)
	}
	if changed, err := s.SavePage(0x100, generated(t, "HUMAN"), "", audit); err != nil || !changed {
		t.Fatalf("SavePage after detach: changed %v err %v", changed, err)
	}
	if err := s.DeleteBinding(id, admin); !errors.Is(err, ErrNotFound) {
		t.Errorf("second delete: %v", err)
	}
}

func TestTemplateCRUD(t *testing.T) {
	s := openTemp(t)
	b := tti.NewPage(0x100).Bytes()
	id, err := s.CreateTemplate(&Template{Name: "a", TTI: b, RowRules: "{}"}, admin)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateTemplate(&Template{Name: "a", TTI: b, RowRules: "{}"}, admin); !errors.Is(err, ErrNameTaken) {
		t.Fatalf("duplicate: %v", err)
	}
	if err := s.UpdateTemplate(&Template{ID: id, Name: "b", TTI: b, RowRules: `{"1":"x"}`}, admin); err != nil {
		t.Fatal(err)
	}
	got, err := s.Template(id)
	if err != nil || got.Name != "b" || got.RowRules != `{"1":"x"}` || !bytes.Equal(got.TTI, b) {
		t.Fatalf("template %+v err %v", got, err)
	}
	if err := s.UpdateTemplate(&Template{ID: 99, Name: "c", TTI: b, RowRules: "{}"}, admin); !errors.Is(err, ErrNotFound) {
		t.Errorf("update missing: %v", err)
	}
	if ts, err := s.Templates(); err != nil || len(ts) != 1 {
		t.Errorf("templates %v err %v", ts, err)
	}
	if err := s.DeleteTemplate(id, admin); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Template(id); !errors.Is(err, ErrNotFound) {
		t.Errorf("after delete: %v", err)
	}
	if err := s.DeleteTemplate(id, admin); !errors.Is(err, ErrNotFound) {
		t.Errorf("second delete: %v", err)
	}
}

func TestDeleteTemplateInUse(t *testing.T) {
	s, _ := boundEnv(t)
	err := s.DeleteTemplate(1, admin)
	if !errors.Is(err, ErrInUse) || !strings.Contains(err.Error(), "1 binding") {
		t.Fatalf("got %v", err)
	}
	if _, err := s.Template(1); err != nil {
		t.Errorf("template gone: %v", err)
	}
	if n := scalar(t, s, "SELECT count(*) FROM audit WHERE action = 'template.delete'"); n != 0 {
		t.Errorf("audit rows %d", n)
	}
}

func TestTemplateAndBindingAuditDetailIsNameOnly(t *testing.T) {
	s, id := boundEnv(t)
	b, _ := s.Binding(id)
	if err := s.UpdateBinding(b, admin); err != nil {
		t.Fatal(err)
	}
	tpl, _ := s.Template(1)
	if err := s.UpdateTemplate(tpl, admin); err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteBinding(id, admin); err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteTemplate(1, admin); err != nil {
		t.Fatal(err)
	}
	rows, err := s.db.Query("SELECT action, target, detail FROM audit WHERE action LIKE 'template.%' OR action LIKE 'binding.%' ORDER BY id")
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var got []string
	for rows.Next() {
		var action, target, detail string
		rows.Scan(&action, &target, &detail)
		got = append(got, action+" "+target+" "+detail)
	}
	want := []string{
		"template.create template:1 scores", "binding.create binding:1 pacman",
		"binding.update binding:1 pacman", "template.update template:1 scores",
		"binding.delete binding:1 pacman", "template.delete template:1 scores",
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("audit rows:\n%s", strings.Join(got, "\n"))
	}
	if n := scalar(t, s, "SELECT count(*) FROM audit WHERE detail LIKE '%secret%' OR detail LIKE '%{%'"); n != 0 {
		t.Errorf("%d audit rows hold source or static", n)
	}
}

// counts snapshots the row counts a refused move must leave alone.
func counts(t *testing.T, s *Store) [4]int {
	t.Helper()
	return [4]int{
		scalar(t, s, "SELECT count(*) FROM pages"), scalar(t, s, "SELECT count(*) FROM page_versions"),
		scalar(t, s, "SELECT count(*) FROM bindings"), scalar(t, s, "SELECT count(*) FROM audit"),
	}
}

// moveEnv is boundEnv with two archived versions of the bound page 0x100.
func moveEnv(t *testing.T) (*Store, int64) {
	t.Helper()
	s, id := boundEnv(t)
	for _, text := range []string{"ONE", "TWO"} {
		if _, err := s.SaveGenerated(id, generated(t, text), Audit{}); err != nil {
			t.Fatal(err)
		}
	}
	return s, id
}

func TestMoveBinding(t *testing.T) { // mutations B, C, F
	s, id := moveEnv(t)
	before, _ := s.Page(0x100)
	b, _ := s.Binding(id)
	b.MPP = 0x101
	if err := s.UpdateBinding(b, admin); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Page(0x100); !errors.Is(err, ErrNotFound) {
		t.Errorf("old page still there: %v", err)
	}
	got, err := s.Page(0x101)
	if err != nil {
		t.Fatal(err)
	}
	p, err := tti.Parse(got.TTI)
	if err != nil || p.MPP() != 0x101 || p.Subpages() != before.Subpages || p.Subpages() < 2 {
		t.Fatalf("moved page: mpp %03X subpages %d err %v", p.MPP(), p.Subpages(), err)
	}
	if want := bytes.ReplaceAll(before.TTI, []byte("PN,100"), []byte("PN,101")); !bytes.Equal(got.TTI, want) {
		t.Error("bytes differ from the old page apart from PN")
	}
	if got.Description != before.Description || got.Transmit != before.Transmit || got.BindingID == nil || *got.BindingID != id {
		t.Errorf("moved page %+v", got)
	}
	if got.UpdatedBy != admin.Actor {
		t.Errorf("updated_by %q", got.UpdatedBy)
	}
	if n, o := scalar(t, s, "SELECT count(*) FROM page_versions WHERE mpp = 0x101"), scalar(t, s, "SELECT count(*) FROM page_versions WHERE mpp = 0x100"); n != 2 || o != 0 {
		t.Errorf("versions: %d at new, %d at old", n, o)
	}
	if b, _ := s.Binding(id); b.MPP != 0x101 {
		t.Errorf("binding mpp %03X", b.MPP)
	}
	if n := scalar(t, s, "SELECT count(*) FROM audit WHERE action = 'binding.move' AND target = 'binding:1' AND detail = '100->101'"); n != 1 {
		t.Errorf("%d binding.move rows", n)
	}
	b.Name = "renamed" // a plain update writes no move row
	if err := s.UpdateBinding(b, admin); err != nil {
		t.Fatal(err)
	}
	if n := scalar(t, s, "SELECT count(*) FROM audit WHERE action = 'binding.move'"); n != 1 {
		t.Errorf("%d binding.move rows after a plain update", n)
	}
}

func TestMoveBindingOntoExistingPageRefused(t *testing.T) { // mutation A
	s, id := moveEnv(t)
	if _, err := s.ImportPages([]ImportFile{fixture(t, "P101.tti")}, "cli"); err != nil { // unbound
		t.Fatal(err)
	}
	if _, err := s.CreateBinding(&Binding{Name: "other", TemplateID: 1, MPP: 0x102, Source: "{}", Static: "{}"}, admin); err != nil { // bound
		t.Fatal(err)
	}
	b, _ := s.Binding(id)
	old, _ := s.Page(0x100)
	for _, target := range []int{0x101, 0x102} {
		before := counts(t, s)
		taken, _ := s.Page(target)
		b.MPP = target
		if err := s.UpdateBinding(b, admin); !errors.Is(err, ErrExists) {
			t.Fatalf("move to %03X: %v", target, err)
		}
		if counts(t, s) != before {
			t.Errorf("move to %03X changed row counts", target)
		}
		if got, _ := s.Page(target); !bytes.Equal(got.TTI, taken.TTI) || got.UpdatedAt != taken.UpdatedAt {
			t.Errorf("page %03X was overwritten", target)
		}
		if got, _ := s.Page(0x100); !bytes.Equal(got.TTI, old.TTI) {
			t.Error("the bound page changed")
		}
		if got, _ := s.Binding(id); got.MPP != 0x100 {
			t.Errorf("binding mpp %03X", got.MPP)
		}
	}
}

func TestMoveBindingIsAllOrNothing(t *testing.T) { // mutation D
	s, id := moveEnv(t)
	if _, err := s.CreateBinding(&Binding{Name: "other", TemplateID: 1, MPP: 0x102, Source: "{}", Static: "{}"}, admin); err != nil {
		t.Fatal(err)
	}
	old, _ := s.Page(0x100)
	before := counts(t, s)
	b, _ := s.Binding(id)
	b.MPP, b.Name = 0x101, "other"
	if err := s.UpdateBinding(b, admin); !errors.Is(err, ErrNameTaken) {
		t.Fatalf("got %v", err)
	}
	if got, err := s.Page(0x100); err != nil || !bytes.Equal(got.TTI, old.TTI) {
		t.Errorf("page 100 after a refused move: %v", err)
	}
	if _, err := s.Page(0x101); !errors.Is(err, ErrNotFound) {
		t.Error("page 101 appeared")
	}
	if n := scalar(t, s, "SELECT count(*) FROM page_versions WHERE mpp = 0x100"); n != 2 {
		t.Errorf("%d versions left at 100", n)
	}
	if counts(t, s) != before {
		t.Error("row counts changed")
	}
}

func TestSaveGeneratedAfterMoveFailsCleanly(t *testing.T) { // mutation H
	s, id := moveEnv(t)
	b, _ := s.Binding(id)
	b.MPP = 0x101
	if err := s.UpdateBinding(b, admin); err != nil {
		t.Fatal(err)
	}
	moved, _ := s.Page(0x101)
	before := counts(t, s)
	_, err := s.SaveGenerated(id, generated(t, "STALE"), Audit{}) // a run that rendered for 100
	if err == nil || !strings.Contains(err.Error(), "100") || !strings.Contains(err.Error(), "101") {
		t.Fatalf("got %v", err)
	}
	if got, _ := s.Page(0x101); !bytes.Equal(got.TTI, moved.TTI) || got.UpdatedAt != moved.UpdatedAt {
		t.Error("page 101 changed")
	}
	if counts(t, s) != before {
		t.Error("a version or audit row was written")
	}
}
