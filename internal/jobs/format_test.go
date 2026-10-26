package jobs

import "testing"

// Number and date expectations were derived by hand from the old generator's main.go
// (abbreviateNumberIfNeeded, formatScoreWithUnit, formatSheetDate, parseDateText).

type fmtCase struct {
	name, formatted string
	raw             any
	want            string
}

func runFormat(t *testing.T, spec string, width int, cases []fmtCase) {
	t.Helper()
	for _, c := range cases {
		got, err := formatValue(spec, c.formatted, c.raw, width)
		if err != nil || got != c.want {
			t.Errorf("%s/%s: formatValue(%q, %q, %v, %d) = %q, %v; want %q", spec, c.name, spec, c.formatted, c.raw, width, got, err, c.want)
		}
	}
}

func TestFormatNumber(t *testing.T) {
	runFormat(t, "number", 9, []fmtCase{
		{"fits", "1,234", nil, "1,234"},
		{"nine cells fits", "12,345.67", nil, "12,345.67"},
		{"ten cells abbreviates", "123,456.78", nil, "123.46k"}, // 123.45678 rounds to 123.46
		{"millions", "12,345,670", nil, "12.35m"},               // 12.34567 -> "12.35m", 6 cells
		{"billions drop .00", "1,000,000,000", nil, "1b"},       // "1.00b" -> "1b"
		{"billions", "123,456,789,012", nil, "123.46b"},         // "123.46b" is 7 cells, so no further trim
		{"rounds up a decade", "999,999,999", nil, "1000m"},     // 999.999999 -> "1000.00m" -> "1000m"
		{"trim decimals", "123,456,780,000,000", nil, "123456b"},
		{"cut to nine", "100,000,000,000,000,000", nil, "100000000"}, // "100000000b" is 10 cells, no '.', cut
		{"not a number", "ABCDEFGHIJKL", nil, "ABCDEFGHIJKL"},
		{"raw ignored", "12,345,670", 5.0, "12.35m"},
	})
	runFormat(t, "seconds", 9, []fmtCase{
		{"plain", "12.34", nil, "12.34 sec"},
		{"own unit replaced", "12.34 s", nil, "12.34 sec"},
		{"already sec", "12.34 sec", nil, "12.34 sec"},
		{"abbreviated", "1,000,000,000", nil, "1b sec"},
		{"word only", "DNF", nil, "DNF"},
	})
}

func TestFormatNumberUnitAndWidth(t *testing.T) {
	for _, c := range []struct {
		spec  string
		in    string
		width int
		want  string
	}{
		{"number unit=sec", "12,345.67", 9, "12k sec"},
		{"number unit=points", "7,582", 9, "7k points"},
		{"number unit=pts", "7,582", 9, "7,582 pts"},
		{"number", "12.34 s", 9, "12.34"},
		{"number", "$1,234", 9, "1,234"},
		{"number", "123,456,789,012", 0, "123,456,789,012"},
		{"number", "-1,234,567", 6, "-1.23m"},
		{"number unit=seconds", "999", 7, "999 sec"},
		{"number unit=sec", "DNF", 9, "DNF"},
		{"number unit=sec", "1-2", 9, "1-2"},
	} {
		got, err := formatValue(c.spec, c.in, nil, c.width)
		if err != nil || got != c.want {
			t.Errorf("formatValue(%q, %q, %d) = %q, %v; want %q", c.spec, c.in, c.width, got, err, c.want)
		}
	}
}

func TestFormatDate(t *testing.T) {
	runFormat(t, "date", 0, []fmtCase{
		{"serial", "x", 46000.0, "09 Dec 25"}, // 45658 is 2025-01-01; +342 days is day 343, 9 Dec
		{"serial fraction floors", "x", 46000.9, "09 Dec 25"},
		{"serial epoch", "x", 1.0, "31 Dec 99"}, // 1899-12-30 + 1 day
		{"numeric day first", "12/10/2025", nil, "12 Oct 25"},
		{"numeric month first", "12/13/25", nil, "13 Dec 25"}, // second > 12 and first <= 12 swaps
		{"numeric day over 12", "13/12/25", nil, "13 Dec 25"},
		{"month name", "12 Oct 25", nil, "12 Oct 25"},
		{"long month name", "1 January 2025", nil, "01 Jan 25"},
		{"iso", "2025-10-12", nil, "12 Oct 25"},
		{"text raw falls to text", "12 Oct 25", "12 Oct 25", "12 Oct 25"},
	})
	for spec, want := range map[string]string{
		"date DD/MM/YY":   "09/12/25",
		"date D Mth YYYY": "9 Dec 2025",
		"date YYYY-MM-DD": "2025-12-09",
		"date DDth Mth":   "09th Dec",
		"date M/D":        "12/9",
	} {
		runFormat(t, spec, 0, []fmtCase{{"pattern", "x", 46000.0, want}})
	}
	for _, bad := range []string{"garbage", "31/02/2025", ""} {
		if got, err := formatValue("date", bad, nil, 0); err == nil || got != "" || err.Error() != `date: unsupported value "`+bad+`"` {
			t.Errorf("date(%q) = %q, %v; want the unsupported-value error", bad, got, err)
		}
	}
}
