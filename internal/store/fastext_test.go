package store

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"ceems/internal/tti"
)

// plainPage returns a page of subs subpages whose FL line is fl.
func plainPage(t *testing.T, mpp, subs int, fl string) *tti.Page {
	t.Helper()
	p := tti.NewPage(mpp)
	for i := 1; i < subs; i++ {
		p.AddSubpage()
	}
	for i := 0; i < p.Subpages(); i++ {
		p.SetField(i, "FL", fl)
	}
	return p
}

const manualFL = "111,222,333,444,5aa,666"

func addPage(t *testing.T, s *Store, mpp, subs int) {
	t.Helper()
	if err := s.CreatePage(plainPage(t, mpp, subs, manualFL), Audit{Actor: "user:1", Action: "page.create"}); err != nil {
		t.Fatal(err)
	}
}

// flOf returns the FL body of every subpage of page mpp.
func flOf(t *testing.T, s *Store, mpp int) []string {
	t.Helper()
	pg, err := s.Page(mpp)
	if err != nil {
		t.Fatal(err)
	}
	p, err := tti.Parse(pg.TTI)
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for i := 0; i < p.Subpages(); i++ {
		b, _ := p.Field(i, "FL")
		out = append(out, b)
	}
	return out
}

func wantFL(t *testing.T, s *Store, mpp int, want string) {
	t.Helper()
	got := flOf(t, s, mpp)
	if len(got) == 0 {
		t.Fatalf("page %03X has no subpages", mpp)
	}
	for i, g := range got {
		if g != want {
			t.Errorf("page %03X subpage %d FL %q, want %q", mpp, i, g, want)
		}
	}
}

func ip(v int) *int { return &v }

var rgyc = [4]string{"101", "102", "103", "104"}

func newGroup(t *testing.T, s *Store, g FastextGroup, mpps ...int) int64 {
	t.Helper()
	if g.Name == "" {
		g.Name = "g"
	}
	if g.Links == ([4]string{}) {
		g.Links = rgyc
	}
	id, err := s.CreateFastextGroup(&g, admin)
	if err != nil {
		t.Fatal(err)
	}
	if len(mpps) > 0 {
		if err := s.SetFastextGroupPages(id, mpps, admin); err != nil {
			t.Fatal(err)
		}
	}
	return id
}

func TestMigrationGroupForeignKeySetsNull(t *testing.T) {
	s := openTemp(t)
	addPage(t, s, 0x200, 1)
	res, err := s.db.Exec("INSERT INTO fastext_groups (name, paged, links) VALUES ('x', 0, '101,102,103,104')")
	if err != nil {
		t.Fatal(err)
	}
	id, _ := res.LastInsertId()
	if _, err := s.db.Exec("UPDATE pages SET fastext_group_id = ?, fastext_order = 1 WHERE mpp = 512", id); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec("UPDATE pages SET fastext_group_id = 999 WHERE mpp = 512"); err == nil {
		t.Fatal("a page was put in a group that does not exist")
	}
	if _, err := s.db.Exec("DELETE FROM fastext_groups WHERE id = ?", id); err != nil {
		t.Fatal(err)
	}
	if n := scalar(t, s, "SELECT count(*) FROM pages WHERE mpp = 512 AND fastext_group_id IS NULL"); n != 1 {
		t.Fatal("deleting the group did not null the member's group id")
	}
	set, _ := s.Settings()
	if set.IndexPageEnabled || set.IndexPage != "100" {
		t.Fatalf("index defaults %v %q", set.IndexPageEnabled, set.IndexPage)
	}
}

func TestStaticGroupRewritesEverySubpageAndKeepsSlot5(t *testing.T) {
	s := openTemp(t)
	addPage(t, s, 0x201, 3)
	addPage(t, s, 0x202, 1)
	newGroup(t, s, FastextGroup{Name: "static"}, 0x201, 0x202)
	wantFL(t, s, 0x201, "101,102,103,104,5aa,666")
	wantFL(t, s, 0x202, "101,102,103,104,5aa,666")
	if n := len(flOf(t, s, 0x201)); n != 3 {
		t.Fatalf("%d subpages", n)
	}
}

func TestPagedGroupLinksNoWrap(t *testing.T) {
	s := openTemp(t)
	for _, m := range []int{0x201, 0x202, 0x203} {
		addPage(t, s, m, 2)
	}
	// Previous is the red key (slot 0), Next the green (slot 1); order 203, 201, 202.
	newGroup(t, s, FastextGroup{Paged: true, PrevSlot: ip(0), NextSlot: ip(1)}, 0x203, 0x201, 0x202)
	wantFL(t, s, 0x203, "8ff,201,103,104,5aa,666") // first: no previous
	wantFL(t, s, 0x201, "203,202,103,104,5aa,666")
	wantFL(t, s, 0x202, "201,8ff,103,104,5aa,666") // last: no next
}

func TestPagedGroupOfOneIsAbsentBothWays(t *testing.T) {
	s := openTemp(t)
	addPage(t, s, 0x201, 2)
	newGroup(t, s, FastextGroup{Paged: true, PrevSlot: ip(2), NextSlot: ip(3)}, 0x201)
	wantFL(t, s, 0x201, "101,102,8ff,8ff,5aa,666")
}

func TestGroupReplacesManualLinksOnSave(t *testing.T) { // mutation 1
	s := openTemp(t)
	addPage(t, s, 0x201, 1)
	newGroup(t, s, FastextGroup{}, 0x201)
	pg, _ := s.Page(0x201)
	p := plainPage(t, 0x201, 1, "7aa,7bb,7cc,7dd,7ee,7ff")
	if changed, err := s.SavePage(0x201, p, pg.ETag(), audit); err != nil || !changed {
		t.Fatalf("changed %v err %v", changed, err)
	}
	wantFL(t, s, 0x201, "101,102,103,104,7ee,7ff") // slot 5 is the editor's, slots 1-4 the group's
}

func TestDetachedPageKeepsLinksThenTakesManualOnes(t *testing.T) {
	s := openTemp(t)
	addPage(t, s, 0x201, 1)
	addPage(t, s, 0x202, 1)
	id := newGroup(t, s, FastextGroup{}, 0x201, 0x202)
	if err := s.SetFastextGroupPages(id, []int{0x202}, admin); err != nil {
		t.Fatal(err)
	}
	wantFL(t, s, 0x201, "101,102,103,104,5aa,666")
	if pg, _ := s.Page(0x201); pg.FastextGroupID != nil {
		t.Fatal("still in the group")
	}
	pg, _ := s.Page(0x201)
	if _, err := s.SavePage(0x201, plainPage(t, 0x201, 1, "7aa,7bb,7cc,7dd,7ee,7ff"), pg.ETag(), audit); err != nil {
		t.Fatal(err)
	}
	wantFL(t, s, 0x201, "7aa,7bb,7cc,7dd,7ee,7ff")
}

func TestDeleteGroupLeavesMembersWithTheirLinks(t *testing.T) {
	s := openTemp(t)
	addPage(t, s, 0x201, 1)
	id := newGroup(t, s, FastextGroup{}, 0x201)
	if err := s.DeleteFastextGroup(id, admin); err != nil {
		t.Fatal(err)
	}
	wantFL(t, s, 0x201, "101,102,103,104,5aa,666")
	pg, _ := s.Page(0x201)
	if pg.FastextGroupID != nil || pg.FastextGroupName != "" {
		t.Fatalf("group %v %q", pg.FastextGroupID, pg.FastextGroupName)
	}
	if err := s.DeleteFastextGroup(id, admin); !errors.Is(err, ErrNotFound) {
		t.Fatalf("second delete %v", err)
	}
}

func TestDeletePageRelinksNeighbours(t *testing.T) { // mutation 3
	s := openTemp(t)
	for _, m := range []int{0x201, 0x202, 0x203} {
		addPage(t, s, m, 1)
	}
	newGroup(t, s, FastextGroup{Paged: true, PrevSlot: ip(0), NextSlot: ip(1)}, 0x201, 0x202, 0x203)
	pg, _ := s.Page(0x202)
	if err := s.DeletePage(0x202, pg.ETag(), Audit{Actor: "user:1", Action: "page.delete"}); err != nil {
		t.Fatal(err)
	}
	wantFL(t, s, 0x201, "8ff,203,103,104,5aa,666")
	wantFL(t, s, 0x203, "201,8ff,103,104,5aa,666")
}

func TestMovePageRelinksNeighbours(t *testing.T) {
	s, id := boundEnv(t) // 0x100 bound
	addPage(t, s, 0x201, 1)
	addPage(t, s, 0x202, 1)
	// Order 201, 100, 202: the bound page sits in the middle.
	gid := newGroup(t, s, FastextGroup{Paged: true, PrevSlot: ip(0), NextSlot: ip(1)}, 0x201, 0x100, 0x202)
	wantFL(t, s, 0x201, "8ff,100,103,104,5aa,666")
	b, _ := s.Binding(id)
	b.MPP = 0x300
	if err := s.UpdateBinding(b, admin); err != nil {
		t.Fatal(err)
	}
	wantFL(t, s, 0x201, "8ff,300,103,104,5aa,666")
	wantFL(t, s, 0x202, "300,8ff,103,104,5aa,666")
	if g, _ := s.FastextGroup(gid); len(g.Members) != 3 || g.Members[1].MPP != 0x300 {
		t.Fatalf("members %+v", g.Members)
	}
}

func TestOnlyOneDefaultGroup(t *testing.T) { // mutation 4
	s := openTemp(t)
	a := newGroup(t, s, FastextGroup{Name: "a", Default: true})
	b := newGroup(t, s, FastextGroup{Name: "b", Default: true})
	if n := scalar(t, s, "SELECT count(*) FROM fastext_groups WHERE is_default = 1"); n != 1 {
		t.Fatalf("%d defaults after create", n)
	}
	ga, _ := s.FastextGroup(a)
	ga.Default = true
	if err := s.UpdateFastextGroup(ga, admin); err != nil {
		t.Fatal(err)
	}
	gs, _ := s.FastextGroups()
	if gs[0].ID != a || !gs[0].Default || gs[1].ID != b || gs[1].Default {
		t.Fatalf("groups %+v", gs)
	}
}

func TestNewPageAndNewBindingPageJoinDefaultGroupLast(t *testing.T) {
	s := openTemp(t)
	addPage(t, s, 0x200, 1) // before any default: stays ungrouped
	id := newGroup(t, s, FastextGroup{Paged: true, PrevSlot: ip(0), NextSlot: ip(1), Default: true})
	if pg, _ := s.Page(0x200); pg.FastextGroupID != nil {
		t.Fatal("an existing page joined a new default")
	}
	addPage(t, s, 0x2f0, 1)
	tid, err := s.CreateTemplate(&Template{Name: "t", TTI: tti.NewPage(0x100).Bytes(), RowRules: "{}"}, admin)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateBinding(&Binding{Name: "b", TemplateID: tid, MPP: 0x210, Source: "{}", Static: "{}"}, admin); err != nil {
		t.Fatal(err)
	}
	addPage(t, s, 0x205, 1) // joins last although its number is lowest
	g, _ := s.FastextGroup(id)
	var got []int
	for _, m := range g.Members {
		got = append(got, m.MPP)
	}
	if fmt.Sprint(got) != "[752 528 517]" { // 2f0, 210, 205
		t.Fatalf("members %v", got)
	}
	wantFL(t, s, 0x2f0, "8ff,210,103,104,5aa,666")
	wantFL(t, s, 0x210, "2f0,205,103,104,8ff,8ff")
	wantFL(t, s, 0x205, "210,8ff,103,104,5aa,666")
	pg, _ := s.Page(0x205)
	if pg.FastextGroupName != "g" || !pg.FastextPaged {
		t.Fatalf("page group %q paged %v", pg.FastextGroupName, pg.FastextPaged)
	}
}

func enableIndex(t *testing.T, s *Store, on bool, page string) {
	t.Helper()
	set, _ := s.Settings()
	set.IndexPageEnabled, set.IndexPage = on, page
	if _, err := s.SaveSettings(set, admin); err != nil {
		t.Fatal(err)
	}
}

func TestEnablingIndexRewritesEverySlot6IncludingTheIndexPage(t *testing.T) {
	s := openTemp(t)
	addPage(t, s, 0x100, 2)
	addPage(t, s, 0x201, 1)
	enableIndex(t, s, true, "100")
	wantFL(t, s, 0x100, "111,222,333,444,5aa,100")
	wantFL(t, s, 0x201, "111,222,333,444,5aa,100")
	if n := scalar(t, s, "SELECT count(*) FROM audit WHERE action = 'page.fastext'"); n != 2 {
		t.Fatalf("%d page.fastext rows", n)
	}
	enableIndex(t, s, true, "1AB") // change the page: every page again, written lower case
	wantFL(t, s, 0x201, "111,222,333,444,5aa,1ab")
}

func TestDisablingIndexRewritesNothing(t *testing.T) { // mutation 5
	s := openTemp(t)
	addPage(t, s, 0x201, 1)
	enableIndex(t, s, true, "100")
	rows := scalar(t, s, "SELECT count(*) FROM audit WHERE action = 'page.fastext'")
	versions := scalar(t, s, "SELECT count(*) FROM page_versions")
	enableIndex(t, s, false, "100")
	if n := scalar(t, s, "SELECT count(*) FROM audit WHERE action = 'page.fastext'"); n != rows {
		t.Fatalf("disabling wrote %d page.fastext rows", n-rows)
	}
	if n := scalar(t, s, "SELECT count(*) FROM page_versions"); n != versions {
		t.Fatalf("disabling wrote %d versions", n-versions)
	}
	wantFL(t, s, 0x201, "111,222,333,444,5aa,100") // the link is manual again
}

func TestIndexRuleAppliesToSaveAndKeepsSlot5(t *testing.T) {
	s := openTemp(t)
	addPage(t, s, 0x201, 1)
	enableIndex(t, s, true, "100")
	pg, _ := s.Page(0x201)
	if _, err := s.SavePage(0x201, plainPage(t, 0x201, 1, "7aa,7bb,7cc,7dd,7ee,7ff"), pg.ETag(), audit); err != nil {
		t.Fatal(err)
	}
	wantFL(t, s, 0x201, "7aa,7bb,7cc,7dd,7ee,100")
}

func TestFixLinksTouchesNothingWhenNoRuleOwnsASlot(t *testing.T) {
	s := openTemp(t)
	p := tti.NewPage(0x201)
	p.SetField(0, "FL", "8FF,1AA,8ff,8ff,8ff,8ff") // upper case and no rule: kept as written
	if err := s.CreatePage(p, admin); err != nil {
		t.Fatal(err)
	}
	if got := flOf(t, s, 0x201)[0]; got != "8FF,1AA,8ff,8ff,8ff,8ff" {
		t.Fatalf("FL %q", got)
	}
}

func TestMissingFLStartsFromAbsentLinks(t *testing.T) {
	s := openTemp(t)
	newGroup(t, s, FastextGroup{Name: "d", Default: true})
	p := tti.NewPage(0x201)
	p.SetField(0, "FL", "garbage")
	if err := s.CreatePage(p, admin); err != nil {
		t.Fatal(err)
	}
	wantFL(t, s, 0x201, "101,102,103,104,8ff,8ff")
}

func TestSaveGeneratedOnGroupedBoundPageStoresGroupAndIndex(t *testing.T) {
	s, id := boundEnv(t)
	newGroup(t, s, FastextGroup{}, 0x100)
	enableIndex(t, s, true, "200")
	gen := generated(t, "SCORES")
	gen.SetField(0, "FL", "7aa,7bb,7cc,7dd,7ee,7ff")
	if changed, err := s.SaveGenerated(id, gen, Audit{}); err != nil || !changed {
		t.Fatalf("changed %v err %v", changed, err)
	}
	if got := flOf(t, s, 0x100)[0]; got != "101,102,103,104,7ee,200" {
		t.Fatalf("FL %q", got)
	}
	// The sweep writes bound pages too, and a repeat writes nothing.
	if n := scalar(t, s, "SELECT count(*) FROM audit WHERE action = 'page.generate'"); n != 1 {
		t.Fatalf("%d generate rows", n)
	}
	if changed, err := s.SaveGenerated(id, gen, Audit{}); err != nil || changed {
		t.Fatalf("repeat changed %v err %v", changed, err)
	}
}

func TestImportAppliesIndexButNoGroupAndStaysIdempotent(t *testing.T) { // mutation 6
	s := openTemp(t)
	newGroup(t, s, FastextGroup{Name: "d", Default: true, Texts: [4]string{"a", "b", "c", "d"}})
	enableIndex(t, s, true, "123")
	files := fixtures(t)
	if n, err := s.ImportPages(files, "cli"); err != nil || n != 30 {
		t.Fatalf("added %d err %v", n, err)
	}
	if n := scalar(t, s, "SELECT count(*) FROM pages WHERE fastext_group_id IS NOT NULL"); n != 0 {
		t.Fatalf("%d imported pages joined a group", n)
	}
	for _, f := range files {
		p, err := tti.Parse(f.TTI)
		if err != nil {
			t.Fatal(err)
		}
		pg, _ := s.Page(p.MPP())
		stored, _ := tti.Parse(pg.TTI)
		for i := 0; i < p.Subpages(); i++ {
			was, had := p.Field(i, "FL")
			got, _ := stored.Field(i, "FL")
			links, ok := tti.ParseFL(got)
			if !ok || links[5] != "123" {
				t.Fatalf("%s subpage %d FL %q", f.Path, i, got)
			}
			if had {
				if old, ok := tti.ParseFL(was); ok && old[0] != links[0] {
					t.Fatalf("%s: slot 1 changed %q -> %q", f.Path, was, got)
				}
			}
		}
	}
	before := scalar(t, s, "SELECT total_changes()")
	if n, err := s.ImportPages(files, "again"); err != nil || n != 0 {
		t.Fatalf("second import added %d err %v", n, err)
	}
	if after := scalar(t, s, "SELECT total_changes()"); after != before {
		t.Fatalf("second import wrote: total_changes %d -> %d", before, after)
	}
}

func TestOnePageFastextAuditRowPerChangedPage(t *testing.T) {
	s := openTemp(t)
	addPage(t, s, 0x201, 1)
	addPage(t, s, 0x202, 1)
	addPage(t, s, 0x203, 1)
	notified := 0
	s.OnPageChange(func() { notified++ })
	gid := newGroup(t, s, FastextGroup{}, 0x201, 0x202)
	count := func() int {
		return scalar(t, s, "SELECT count(*) FROM audit WHERE action = 'page.fastext' AND actor = 'user:1' AND target IN ('page:201','page:202')")
	}
	if count() != 2 || notified != 1 {
		t.Fatalf("%d rows, %d notifies", count(), notified)
	}
	if n := scalar(t, s, "SELECT count(*) FROM audit WHERE target = 'page:203' AND action = 'page.fastext'"); n != 0 {
		t.Fatal("a page outside the group was rewritten")
	}
	// An update that changes nothing the pages hold rewrites no page.
	g, _ := s.FastextGroup(gid)
	if err := s.UpdateFastextGroup(g, admin); err != nil {
		t.Fatal(err)
	}
	if count() != 2 || notified != 1 {
		t.Fatalf("unchanged sweep: %d rows, %d notifies", count(), notified)
	}
	versions := scalar(t, s, "SELECT count(*) FROM page_versions")
	if err := s.UpdateFastextGroup(g, admin); err != nil { // row 24 included, still nothing to write
		t.Fatal(err)
	}
	if n := scalar(t, s, "SELECT count(*) FROM page_versions"); n != versions || count() != 2 {
		t.Fatalf("an unchanged member wrote: %d versions (was %d), %d audit rows", n, versions, count())
	}
	g.Links[0] = "1ff"
	if err := s.UpdateFastextGroup(g, admin); err != nil {
		t.Fatal(err)
	}
	if count() != 4 || notified != 2 {
		t.Fatalf("changed sweep: %d rows, %d notifies", count(), notified)
	}
	if n := scalar(t, s, "SELECT count(*) FROM audit WHERE action IN ('fastext.create','fastext.pages','fastext.update') AND target = 'fastext:1'"); n != 5 {
		t.Fatalf("%d fastext.* rows", n)
	}
}

func TestSetGroupPagesRefusals(t *testing.T) {
	s := openTemp(t)
	addPage(t, s, 0x201, 1)
	a := newGroup(t, s, FastextGroup{Name: "a"}, 0x201)
	b := newGroup(t, s, FastextGroup{Name: "b"})
	if err := s.SetFastextGroupPages(b, []int{0x201}, admin); !errors.Is(err, ErrInGroup) {
		t.Fatalf("other group's page: %v", err)
	}
	if err := s.SetFastextGroupPages(a, []int{0x201, 0x999}, admin); !errors.Is(err, ErrNotFound) {
		t.Fatalf("unknown page: %v", err)
	}
	if err := s.SetFastextGroupPages(99, nil, admin); !errors.Is(err, ErrNotFound) {
		t.Fatalf("unknown group: %v", err)
	}
	if err := s.SetFastextGroupPages(a, []int{0x201, 0x201}, admin); !errors.Is(err, ErrInvalid) {
		t.Fatalf("repeated page: %v", err)
	}
	if g, _ := s.FastextGroup(a); len(g.Members) != 1 {
		t.Fatalf("a refused call changed the members: %+v", g.Members)
	}
}

func TestGroupValidation(t *testing.T) {
	s := openTemp(t)
	ok := FastextGroup{Name: "ok", Links: rgyc}
	long := ok
	long.Name = strings.Repeat("x", 41)
	cases := map[string]func(g *FastextGroup){
		"empty name":      func(g *FastextGroup) { g.Name = "" },
		"long name":       func(g *FastextGroup) { g.Name = long.Name },
		"bad hex":         func(g *FastextGroup) { g.Links[1] = "12g" },
		"magazine 9":      func(g *FastextGroup) { g.Links[2] = "900" },
		"magazine 0":      func(g *FastextGroup) { g.Links[2] = "0aa" },
		"short link":      func(g *FastextGroup) { g.Links[3] = "12" },
		"paged no slots":  func(g *FastextGroup) { g.Paged = true },
		"paged same slot": func(g *FastextGroup) { g.Paged, g.PrevSlot, g.NextSlot = true, ip(1), ip(1) },
		"paged slot 4":    func(g *FastextGroup) { g.Paged, g.PrevSlot, g.NextSlot = true, ip(1), ip(4) },
		"static slots":    func(g *FastextGroup) { g.PrevSlot, g.NextSlot = ip(0), ip(1) },
	}
	for name, mod := range cases {
		g := ok
		mod(&g)
		if _, err := s.CreateFastextGroup(&g, admin); !errors.Is(err, ErrInvalid) {
			t.Errorf("%s: %v", name, err)
		}
	}
	good := ok
	good.Name = "good"
	good.Links = [4]string{"8FF", "1AB", "800", "8ff"}
	id, err := s.CreateFastextGroup(&good, admin)
	if err != nil {
		t.Fatal(err)
	}
	if g, _ := s.FastextGroup(id); g.Links != [4]string{"8ff", "1ab", "800", "8ff"} {
		t.Fatalf("links %v", g.Links)
	}
	if _, err := s.CreateFastextGroup(&ok, admin); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateFastextGroup(&ok, admin); !errors.Is(err, ErrNameTaken) {
		t.Fatalf("duplicate name: %v", err)
	}
}

func TestFixLinksNeverTouchesSlot5(t *testing.T) { // mutation 7
	s := openTemp(t)
	addPage(t, s, 0x201, 2)
	newGroup(t, s, FastextGroup{Paged: true, PrevSlot: ip(0), NextSlot: ip(3)}, 0x201)
	enableIndex(t, s, true, "100")
	wantFL(t, s, 0x201, "8ff,102,103,8ff,5aa,100")
}

// row24Of returns the decoded row 24 of every subpage of page mpp; "<none>" when a subpage has no line.
func row24Of(t *testing.T, s *Store, mpp int) []string {
	t.Helper()
	p, err := tti.Parse(mustPage(t, s, mpp).TTI)
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for i := 0; i < p.Subpages(); i++ {
		r, ok := p.Row(i, 24)
		if !ok {
			r = "<none>"
		}
		out = append(out, r)
	}
	return out
}

func mustPage(t *testing.T, s *Store, mpp int) *Page {
	t.Helper()
	pg, err := s.Page(mpp)
	if err != nil {
		t.Fatal(err)
	}
	return pg
}

func wantRow24(t *testing.T, s *Store, mpp int, want string) {
	t.Helper()
	got := row24Of(t, s, mpp)
	if len(got) == 0 {
		t.Fatalf("page %03X has no subpages", mpp)
	}
	for i, g := range got {
		if g != want {
			t.Errorf("page %03X subpage %d row 24 %q, want %q", mpp, i, g, want)
		}
	}
}

// field is one 10-cell field of row 24: the colour code then the text padded to 9.
func field(code byte, text string) string { return string(rune(code)) + fmt.Sprintf("%-9s", text) }

// withoutRow24 drops every OL,24 line, as a page made elsewhere may lack one.
func withoutRow24(p *tti.Page) *tti.Page {
	var keep []tti.Line
	for _, l := range p.Lines {
		if !strings.HasPrefix(string(l.Raw), "OL,24,") {
			keep = append(keep, l)
		}
	}
	p.Lines = keep
	return p
}

func TestStaticGroupWritesRow24OnEverySubpage(t *testing.T) { // criteria 1 and 13
	s := openTemp(t)
	addPage(t, s, 0x201, 3)
	g := FastextGroup{Name: "static", Links: [4]string{"101", "8ff", "103", "104"}, Texts: [4]string{"Index", "\x07Text\x00", "-", "Art index"}}
	newGroup(t, s, g, 0x201)
	want := field(1, "Index") + field(2, "\x07Text\x00") + field(3, "-") + field(6, "Art index")
	if len(want) != 40 {
		t.Fatalf("want is %d cells", len(want))
	}
	wantRow24(t, s, 0x201, want) // the 8ff link in slot 2 still shows its text
	wantFL(t, s, 0x201, "101,8ff,103,104,5aa,666")
}

func TestRow24InEightBitPage(t *testing.T) { // criterion 2
	s := openTemp(t)
	p := plainPage(t, 0x201, 2, manualFL)
	for i, l := range p.Lines {
		if strings.HasPrefix(string(l.Raw), "OL,1,") {
			p.Lines[i].Raw = []byte("OL,1,\x81Hello")
		}
	}
	if err := s.CreatePage(p, Audit{Actor: "user:1", Action: "page.create"}); err != nil {
		t.Fatal(err)
	}
	newGroup(t, s, FastextGroup{Texts: [4]string{"a", "b", "c", "d"}}, 0x201)
	stored := string(mustPage(t, s, 0x201).TTI)
	if n := strings.Count(stored, "OL,24,\x81a        \x82b        \x83c        \x86d        "); n != 2 {
		t.Fatalf("8-bit row 24 found %d times in %q", n, stored)
	}
	if strings.Contains(stored, "\x1b") {
		t.Fatal("ESC pair in an 8-bit page")
	}
}

func TestPagedGroupRow24HasFixedPrevNextAndBlankEnds(t *testing.T) { // criteria 3 and 4
	s := openTemp(t)
	for _, m := range []int{0x201, 0x202, 0x203} {
		addPage(t, s, m, 2)
	}
	texts := [4]string{"Index", "ignored", "ignored", "Art index"}
	// Prev on green (slot 1), Next on yellow (slot 2).
	newGroup(t, s, FastextGroup{Paged: true, PrevSlot: ip(1), NextSlot: ip(2), Texts: texts}, 0x201, 0x202, 0x203)
	prev, next, none := " [ Prev  ", "  Next ] ", strings.Repeat(" ", 9)
	wantRow24(t, s, 0x202, field(1, "Index")+field(2, prev)+field(3, next)+field(6, "Art index"))
	wantRow24(t, s, 0x201, field(1, "Index")+field(2, none)+field(3, next)+field(6, "Art index"))
	wantRow24(t, s, 0x203, field(1, "Index")+field(2, prev)+field(3, none)+field(6, "Art index"))
	addPage(t, s, 0x204, 1)
	newGroup(t, s, FastextGroup{Name: "one", Paged: true, PrevSlot: ip(0), NextSlot: ip(3)}, 0x204)
	wantRow24(t, s, 0x204, field(1, "")+field(2, "")+field(3, "")+field(6, ""))
}

func TestMemberSavedWithOtherRow24StoresTheGroups(t *testing.T) { // criterion 5
	s := openTemp(t)
	addPage(t, s, 0x201, 1)
	newGroup(t, s, FastextGroup{Texts: [4]string{"a", "b", "c", "d"}}, 0x201)
	p := plainPage(t, 0x201, 1, manualFL)
	if err := p.SetRow(0, 24, "my own row"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.SavePage(0x201, p, mustPage(t, s, 0x201).ETag(), audit); err != nil {
		t.Fatal(err)
	}
	wantRow24(t, s, 0x201, field(1, "a")+field(2, "b")+field(3, "c")+field(6, "d"))
}

func TestDetachBlanksRow24OnEverySubpage(t *testing.T) { // criterion 7
	s := openTemp(t)
	addPage(t, s, 0x201, 2)
	addPage(t, s, 0x202, 1)
	id := newGroup(t, s, FastextGroup{Texts: [4]string{"a", "b", "c", "d"}}, 0x201, 0x202)
	// Subpage 1 of page 201 has no row 24 line before the detach; subpage 0 keeps the group's row.
	p, _ := tti.Parse(mustPage(t, s, 0x201).TTI)
	var keep []tti.Line
	sub := -1
	for _, l := range p.Lines {
		if l.Code() == "PN" {
			sub++
		}
		if !(sub == 1 && strings.HasPrefix(string(l.Raw), "OL,24,")) {
			keep = append(keep, l)
		}
	}
	p.Lines = keep
	if _, err := s.db.Exec("UPDATE pages SET tti = ? WHERE mpp = ?", p.Bytes(), 0x201); err != nil {
		t.Fatal(err)
	}
	if got := row24Of(t, s, 0x201); got[0] == "" || got[0] == "<none>" || got[1] != "<none>" {
		t.Fatalf("setup: %q", got)
	}
	remaining := string(mustPage(t, s, 0x202).TTI)
	if err := s.SetFastextGroupPages(id, []int{0x202}, admin); err != nil {
		t.Fatal(err)
	}
	wantRow24(t, s, 0x201, "")
	wantFL(t, s, 0x201, "101,102,103,104,5aa,666")
	if string(mustPage(t, s, 0x202).TTI) != remaining {
		t.Fatal("a remaining member changed")
	}
}

func TestDeleteGroupBlanksRow24OfEveryMember(t *testing.T) { // criterion 8
	s := openTemp(t)
	addPage(t, s, 0x201, 2)
	addPage(t, s, 0x202, 1)
	id := newGroup(t, s, FastextGroup{Texts: [4]string{"a", "b", "c", "d"}}, 0x201, 0x202)
	before := scalar(t, s, "SELECT count(*) FROM audit WHERE action = 'page.fastext'")
	if err := s.DeleteFastextGroup(id, admin); err != nil {
		t.Fatal(err)
	}
	wantRow24(t, s, 0x201, "")
	wantRow24(t, s, 0x202, "")
	wantFL(t, s, 0x201, "101,102,103,104,5aa,666")
	if n := scalar(t, s, "SELECT count(*) FROM audit WHERE action = 'page.fastext'") - before; n != 2 {
		t.Fatalf("%d page.fastext rows from the delete, want 2", n)
	}
}

func TestGroupTextValidation(t *testing.T) { // criterion 9
	s := openTemp(t)
	ok := FastextGroup{Name: "ok", Links: rgyc, Texts: [4]string{"\x00\x01\x02\x03\x1b\x1f\x7f12", "", "", "123456789"}}
	if _, err := s.CreateFastextGroup(&ok, admin); err != nil {
		t.Fatalf("9 cells with control codes: %v", err)
	}
	for name, text := range map[string]string{"ten cells": "0123456789", "above 0x7F": "café", "invalid UTF-8": "a\xffb"} {
		g := FastextGroup{Name: name, Links: rgyc}
		g.Texts[2] = text
		_, err := s.CreateFastextGroup(&g, admin)
		if !errors.Is(err, ErrInvalid) || !strings.Contains(err.Error(), "text 3 must be at most 9 cells") {
			t.Errorf("%s: %v", name, err)
		}
		if n := scalar(t, s, fmt.Sprintf("SELECT count(*) FROM fastext_groups WHERE name = '%s'", name)); n != 0 {
			t.Errorf("%s: a refused create left a row", name)
		}
	}
}

func TestTextsMigrationAndRoundTrip(t *testing.T) { // criterion 10
	s := openTemp(t)
	if v := pragma(t, s.db, "user_version"); v != "11" {
		t.Fatalf("user_version %s", v)
	}
	if _, err := s.db.Exec("INSERT INTO fastext_groups (name, paged, links) VALUES ('old', 0, '101,102,103,104')"); err != nil {
		t.Fatal(err)
	}
	gs, err := s.FastextGroups()
	if err != nil || len(gs) != 1 || gs[0].Texts != ([4]string{}) {
		t.Fatalf("old shape: %+v %v", gs, err)
	}
	id := newGroup(t, s, FastextGroup{Name: "rt", Texts: [4]string{"\x00\x1b", "b", "", "d"}})
	if g, _ := s.FastextGroup(id); g.Texts != [4]string{"\x00\x1b", "b", "", "d"} {
		t.Fatalf("round trip %q", g.Texts)
	}
}

func TestSaveGeneratedOnGroupedBoundPageStoresRow24(t *testing.T) { // criterion 12
	s, id := boundEnv(t)
	newGroup(t, s, FastextGroup{Texts: [4]string{"a", "b", "c", "d"}}, 0x100)
	gen := generated(t, "SCORES")
	if err := gen.SetRow(0, 24, "generated row"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.SaveGenerated(id, gen, Audit{}); err != nil {
		t.Fatal(err)
	}
	wantRow24(t, s, 0x100, field(1, "a")+field(2, "b")+field(3, "c")+field(6, "d"))
}

func TestSetRowWithEmptyCellsKeepsAnOL24Line(t *testing.T) { // the packet's stop condition
	p := withoutRow24(plainPage(t, 0x201, 1, manualFL))
	if err := p.SetRow(0, 24, ""); err != nil {
		t.Fatal(err)
	}
	if r, ok := p.Row(0, 24); !ok || r != "" {
		t.Fatalf("Row %q %v", r, ok)
	}
	if !strings.Contains(string(p.Bytes()), "OL,24,\r\n") {
		t.Fatalf("no OL,24, line: %q", p.Bytes())
	}
}
