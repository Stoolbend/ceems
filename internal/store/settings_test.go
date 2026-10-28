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
		InitialTeletextPage: "100", LinesPerField: 16, MagazinePriority: "9,3,3,6,3,3,5,6", StatusDisplay: "CeeMS", IndexPage: "100",
		NetworkIdentificationCode: "0000", CountryNetworkIdentification: "0000"}
	if got != want {
		t.Fatalf("defaults %+v, want %+v", got, want)
	}
}

// A database made before 0011 gains the four vbit2 rows and keeps its own magazine_priority.
func TestMigration0011AddsTheVbit2KeysAndKeepsStoredValues(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, q := range []string{
		"DELETE FROM settings WHERE key IN ('network_identification_code','country_network_identification','datacast_lines','magazine_serial')",
		"UPDATE settings SET value = '9,1,3,3,3,9,9,9' WHERE key = 'magazine_priority'",
		"PRAGMA user_version = 10",
	} {
		if _, err := s.db.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	s.Close()
	s, err = Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	got, _ := s.Settings()
	if got.NetworkIdentificationCode != "0000" || got.CountryNetworkIdentification != "0000" || got.DatacastLines != 0 || got.MagazineSerial {
		t.Fatalf("new keys %+v", got)
	}
	if got.MagazinePriority != "9,1,3,3,3,9,9,9" {
		t.Fatalf("magazine_priority %q was not kept", got.MagazinePriority)
	}
	if n := scalar(t, s, "SELECT count(*) FROM settings"); n != 14 {
		t.Fatalf("%d settings rows, want 14", n)
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

	cur.StatusDisplay, cur.LinesPerField, cur.VbitConf, cur.MagazineSerial, cur.DatacastLines = "NEWS", 8, false, true, 2
	cur.NetworkIdentificationCode = "ABCD"
	changed, err = s.SaveSettings(cur, admin)
	if want := []string{"vbit_conf", "lines_per_field", "status_display", "network_identification_code", "datacast_lines", "magazine_serial"}; err != nil || !reflect.DeepEqual(changed, want) {
		t.Fatalf("changed %v err %v, want %v", changed, err, want)
	}
	if n := scalar(t, s, "SELECT count(*) FROM audit WHERE action = 'settings.update' AND target = 'settings' AND detail = 'vbit_conf,lines_per_field,status_display,network_identification_code,datacast_lines,magazine_serial' AND actor = 'user:1'"); n != 1 {
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
