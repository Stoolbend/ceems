package store

import (
	"fmt"
	"testing"
)

func insertRun(t *testing.T, s *Store, binding int64, outcome, errText string) int64 {
	t.Helper()
	id, err := s.InsertRun(&Run{BindingID: binding, Actor: "schedule", StartedAt: "2026-10-12T10:00:00Z",
		FinishedAt: "2026-10-12T10:00:01Z", Outcome: outcome, Error: errText})
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func TestRunsPruned(t *testing.T) { // mutation G
	s, a := boundEnv(t)
	b, err := s.CreateBinding(&Binding{Name: "other", TemplateID: 1, MPP: 0x81A, Source: "{}", Static: "{}"}, admin)
	if err != nil {
		t.Fatal(err)
	}
	other := insertRun(t, s, b, "ok", "")
	first := insertRun(t, s, a, "failed", "run 0")
	for i := 1; i <= 100; i++ {
		insertRun(t, s, a, "ok", fmt.Sprint("run ", i))
	}
	if n := scalar(t, s, fmt.Sprintf("SELECT count(*) FROM runs WHERE binding_id = %d", a)); n != 100 {
		t.Fatalf("binding %d has %d runs after 101 inserts, want 100", a, n)
	}
	if n := scalar(t, s, fmt.Sprintf("SELECT count(*) FROM runs WHERE id = %d", first)); n != 0 {
		t.Error("the oldest run was kept")
	}
	if n := scalar(t, s, fmt.Sprintf("SELECT count(*) FROM runs WHERE id = %d", other)); n != 1 {
		t.Error("pruning one binding removed another binding's run")
	}
}

func TestRunsNewestFirstAndLastRuns(t *testing.T) {
	s, a := boundEnv(t)
	b, err := s.CreateBinding(&Binding{Name: "other", TemplateID: 1, MPP: 0x81A, Source: "{}", Static: "{}"}, admin)
	if err != nil {
		t.Fatal(err)
	}
	insertRun(t, s, a, "ok", "")
	insertRun(t, s, b, "ok", "")
	last := insertRun(t, s, a, "failed", "sheet unreachable")

	rs, err := s.Runs(a)
	if err != nil || len(rs) != 2 || rs[0].ID != last || rs[0].Outcome != "failed" || rs[0].Error != "sheet unreachable" || rs[1].ID >= rs[0].ID {
		t.Fatalf("runs %+v err %v", rs, err)
	}
	if rs[0].Actor != "schedule" || rs[0].StartedAt != "2026-10-12T10:00:00Z" || rs[0].FinishedAt != "2026-10-12T10:00:01Z" || rs[0].BindingID != a {
		t.Errorf("run fields %+v", rs[0])
	}
	m, err := s.LastRuns()
	if err != nil || len(m) != 2 || m[a].ID != last || m[b].BindingID != b {
		t.Fatalf("last runs %+v err %v", m, err)
	}
	if rs, err := s.Runs(999); err != nil || len(rs) != 0 {
		t.Errorf("unknown binding: %+v %v", rs, err)
	}
}

func TestDeleteBindingDeletesRuns(t *testing.T) {
	s, a := boundEnv(t)
	insertRun(t, s, a, "ok", "")
	if err := s.DeleteBinding(a, admin); err != nil {
		t.Fatal(err)
	}
	if n := scalar(t, s, "SELECT count(*) FROM runs"); n != 0 {
		t.Errorf("runs %d after delete", n)
	}
}
