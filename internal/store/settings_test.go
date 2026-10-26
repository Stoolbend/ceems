package store

import (
	"reflect"
	"testing"
)

func TestSettingsDefaults(t *testing.T) {
	s := openTemp(t)
	got, err := s.Settings()
	if err != nil {
		t.Fatal(err)
	}
	want := Settings{SystemName: "CeeMS", VbitConf: true, HeaderTemplate: "CeeMS   %%# %%a %e %%b %H:%M:%S",
		InitialTeletextPage: "100", LinesPerField: 16, MagazinePriority: "9,1,3,3,3,9,9,9", StatusDisplay: "CeeMS", IndexPage: "100"}
	if got != want {
		t.Fatalf("defaults %+v, want %+v", got, want)
	}
}

func TestSaveSettingsWritesOnlyChangedKeysAndOneAuditRow(t *testing.T) {
	s := openTemp(t)
	notified := 0
	s.OnPageChange(func() { notified++ })
	cur, _ := s.Settings()

	changed, err := s.SaveSettings(cur, admin)
	if err != nil || len(changed) != 0 {
		t.Fatalf("unchanged save: %v %v", changed, err)
	}
	if n := scalar(t, s, "SELECT count(*) FROM audit WHERE action = 'settings.update'"); n != 0 || notified != 0 {
		t.Fatalf("unchanged save wrote %d audit rows and notified %d times", n, notified)
	}

	cur.StatusDisplay, cur.LinesPerField, cur.VbitConf = "NEWS", 8, false
	changed, err = s.SaveSettings(cur, admin)
	if want := []string{"vbit_conf", "lines_per_field", "status_display"}; err != nil || !reflect.DeepEqual(changed, want) {
		t.Fatalf("changed %v err %v, want %v", changed, err, want)
	}
	if n := scalar(t, s, "SELECT count(*) FROM audit WHERE action = 'settings.update' AND target = 'settings' AND detail = 'vbit_conf,lines_per_field,status_display' AND actor = 'user:1'"); n != 1 {
		t.Fatalf("%d matching audit rows, want 1", n)
	}
	if n := scalar(t, s, "SELECT count(*) FROM audit WHERE detail LIKE '%NEWS%'"); n != 0 {
		t.Fatal("the audit detail holds a value")
	}
	if notified != 1 {
		t.Fatalf("notified %d times, want 1", notified)
	}
	got, _ := s.Settings()
	if got != cur {
		t.Fatalf("stored %+v, want %+v", got, cur)
	}

	// A repeat is unchanged: no second audit row.
	if _, err := s.SaveSettings(cur, admin); err != nil {
		t.Fatal(err)
	}
	if n := scalar(t, s, "SELECT count(*) FROM audit WHERE action = 'settings.update'"); n != 1 || notified != 1 {
		t.Fatalf("repeat save: %d audit rows, %d notifies", n, notified)
	}
}
