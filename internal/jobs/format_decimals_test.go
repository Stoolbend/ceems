package jobs

import "testing"

// The owner's example game (T-083): times held as text such as "2.908 sec".
func TestValuesExampleGame(t *testing.T) {
	game := [][]string{
		{"VET", "2.908 sec", "15 Apr 2026"}, {"ALEX", "3.008 sec", "02 Sep 2026"},
		{"TLDA", "3.161 sec", "24 Apr 2026"}, {"ARI", "3.459 sec", "06 Aug 2026"},
		{"LEE", "3.517 sec", "02 Jul 2026"}, {"LMMM", "3.541 sec", "20 May 2026"},
		{"MURY", "3.733 sec", "02 Sep 2026"}, {"JACK", "3.901 sec", "03 Jun 2026"},
	}
	wantDate := []string{"15 Apr 26", "02 Sep 26", "24 Apr 26", "06 Aug 26", "02 Jul 26", "20 May 26", "02 Sep 26", "03 Jun 26"}
	cfg, err := ParseSourceConfig(`{"sheet":"S","range":"C5:E","columns":["p#name","p#scre","p#date"],"rows":8,` +
		`"format":{"p#scre":"number unit=sec decimals=3","p#date":"date"}}`)
	if err != nil {
		t.Fatal(err)
	}
	widths := map[string]int{}
	for i := 1; i <= 8; i++ {
		widths["p"+string(rune('0'+i))+"name"] = 4
		widths["p"+string(rune('0'+i))+"scre"] = 9
		widths["p"+string(rune('0'+i))+"date"] = 9
	}
	got, err := Values(Rows{Formatted: game}, cfg, widths)
	if err != nil {
		t.Fatal(err)
	}
	for i, row := range game {
		n := string(rune('1' + i))
		if got["p"+n+"name"] != row[0] || got["p"+n+"scre"] != row[1] || got["p"+n+"date"] != wantDate[i] {
			t.Errorf("row %s: got %q %q %q; want %q %q %q", n, got["p"+n+"name"], got["p"+n+"scre"], got["p"+n+"date"], row[0], row[1], wantDate[i])
		}
	}
}

func TestFormatNumberDecimals(t *testing.T) {
	for _, c := range []struct {
		name, spec, in string
		width          int
		want           string
	}{
		{"round to two", "number unit=sec decimals=2", "2.908", 9, "2.91 sec"},
		{"pad to three", "number unit=sec decimals=3", "3.0", 9, "3.000 sec"},
		{"pad from whole", "number decimals=2", "7", 9, "7.00"},
		{"zero decimals", "number decimals=0", "2.908", 9, "3"},
		{"option order", "number decimals=3 unit=sec", "2.9", 9, "2.900 sec"},
		{"commas kept", "number decimals=2", "1,234.5", 9, "1,234.50"},
		{"no commas without", "number decimals=2", "1234.5", 9, "1234.50"},
		{"negative commas", "number decimals=1", "-1,234.56", 9, "-1,234.6"},
		{"drops a decimal", "number decimals=3", "12.3456", 5, "12.35"},
		{"drops decimals before k", "number decimals=3", "1,234.5678", 6, "1,235"},
		{"then abbreviates", "number decimals=3", "1,234.5678", 4, "1k"},
		{"unit budget drops decimals", "number unit=test decimals=3", "2.908", 9, "2.91 test"},
		{"no width limit", "number decimals=3", "2.9", 0, "2.900"},
		// without decimals: keep the sheet text when it fits, else as many places as fit
		{"text kept", "number", "2.908", 9, "2.908"},
		{"under 1000 keeps places", "number unit=test", "2.908", 9, "2.91 test"},
		{"three cells", "number", "2.908", 3, "2.9"},
		{"four cells", "number", "12.3456", 4, "12.3"},
		{"no padding zeros", "number", "2.9000", 4, "2.9"},
		{"whole at last", "number", "2.908", 1, "3"},
	} {
		got, err := formatValue(c.spec, c.in, nil, c.width)
		if err != nil || got != c.want {
			t.Errorf("%s: formatValue(%q, %q, %d) = %q, %v; want %q", c.name, c.spec, c.in, c.width, got, err, c.want)
		}
	}
}
