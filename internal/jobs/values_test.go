package jobs

import (
	"errors"
	"reflect"
	"strings"
	"testing"
)

func TestParseSourceConfig(t *testing.T) {
	ok := `{"sheet":"SECRET-ID","range":"C5:E","columns":["p#name","p#score"],"rows":2,"format":{"p#name":"text"}}`
	c, err := ParseSourceConfig(ok)
	if err != nil || c.Rows != 2 || c.Sheet != "SECRET-ID" {
		t.Fatalf("got %+v %v", c, err)
	}
	for _, name := range []string{"text", "number", "seconds", "date"} {
		src := `{"sheet":"SECRET-ID","range":"A","columns":["a"],"rows":1,"format":{"a":"` + name + `"}}`
		if _, err := ParseSourceConfig(src); err != nil {
			t.Errorf("formatter %s refused: %v", name, err)
		}
	}
	for _, bad := range []string{
		`{"sheet":"SECRET-ID","range":"A","columns":["a"],"rows":0}`,
		`{"sheet":"SECRET-ID","range":"A","columns":["a"],"rows":25}`,
		`{"sheet":"SECRET-ID","range":"A","columns":[],"rows":1}`,
		`{"sheet":"","range":"A","columns":["a"],"rows":1}`,
		`{"sheet":"SECRET-ID","range":"A","columns":["a"],"rows":1,"extra":1}`,
		`{"sheet":"SECRET-ID","range":"A","columns":["a"],"rows":1,"format":{"a":"nope"}}`,
		`{"sheet":"SECRET-ID","range":"A","columns":"a","rows":1}`,
	} {
		_, err := ParseSourceConfig(bad)
		if err == nil || strings.Contains(err.Error(), "SECRET-ID") {
			t.Errorf("%s: err %v", bad, err)
		}
	}
}

func TestValues(t *testing.T) {
	cfg := SourceConfig{Range: "C5:E", Columns: []string{"p#name", "p#score"}, Rows: 3}
	cases := []struct {
		name string
		rows [][]string
		want map[string]string
		err  error
	}{
		{"leading incomplete skipped, gap ends entries",
			[][]string{{"", ""}, {"Ann"}, {"Bob", "10"}, {"Cy", "9"}, {"", ""}, {"Di", "8"}},
			map[string]string{"p1name": "Bob", "p1score": "10", "p2name": "Cy", "p2score": "9", "p3name": "", "p3score": ""}, nil},
		{"fewer rows than entries are blank",
			[][]string{{"Bob", "10"}},
			map[string]string{"p1name": "Bob", "p1score": "10", "p2name": "", "p2score": "", "p3name": "", "p3score": ""}, nil},
		{"limit", [][]string{{"a", "1"}, {"b", "2"}, {"c", "3"}, {"d", "4"}},
			map[string]string{"p1name": "a", "p1score": "1", "p2name": "b", "p2score": "2", "p3name": "c", "p3score": "3"}, nil},
		{"empty", nil, nil, ErrEmpty},
		{"missing columns", [][]string{{"a"}, {"b"}}, nil, ErrMissingColumns},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := Values(Rows{Formatted: tc.rows}, cfg, nil)
			if !errors.Is(err, tc.err) {
				t.Fatalf("err %v want %v", err, tc.err)
			}
			if tc.err == nil && !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("got %v want %v", got, tc.want)
			}
		})
	}
	_, err := Values(Rows{Formatted: [][]string{{"a"}}}, cfg, nil)
	if err == nil || !strings.Contains(err.Error(), "C5:E") || !strings.Contains(err.Error(), "2") {
		t.Fatalf("missing-columns text should name range and count: %v", err)
	}
}

func TestParseSourceConfigFormats(t *testing.T) {
	src := func(spec string) string {
		return `{"sheet":"SECRET-ID","range":"A","columns":["a"],"rows":1,"format":{"a":"` + spec + `"}}`
	}
	for _, spec := range []string{"text", "number", "number unit=sec", "number unit=pts", "number decimals=3", "number decimals=0 unit=sec", "number unit=sec decimals=6", "seconds", "date", "date DD/MM/YY"} {
		if _, err := ParseSourceConfig(src(spec)); err != nil {
			t.Errorf("%q refused: %v", spec, err)
		}
	}
	for spec, want := range map[string]string{
		"number foo":        `source: format "a": number: unknown option "foo" (use unit=<text> or decimals=<0-6>)`,
		"number unit=":      `source: format "a": number: unit "" must be printable text with no space, ',' or '=' (one Formats entry per line)`,
		"number unit=sec,":  `source: format "a": number: unit "sec," must be printable text with no space, ',' or '=' (one Formats entry per line)`,
		"number unit=a=b":   `source: format "a": number: unit "a=b" must be printable text with no space, ',' or '=' (one Formats entry per line)`,
		"number unit=sec, p#date=date": `source: format "a": number: unit "sec," must be printable text with no space, ',' or '=' (one Formats entry per line)`,
		"number unit=a unit=b": `source: format "a": number: option unit is given twice`,
		"number decimals=1 decimals=2": `source: format "a": number: option decimals is given twice`,
		"number decimals=7": `source: format "a": number: decimals must be a whole number from 0 to 6, got "7"`,
		"number decimals=x": `source: format "a": number: decimals must be a whole number from 0 to 6, got "x"`,
		"number decimals=": `source: format "a": number: decimals must be a whole number from 0 to 6, got ""`,
		"number decimals=01": `source: format "a": number: decimals must be a whole number from 0 to 6, got "01"`,
		"number bogus=1":    `source: format "a": number: unknown option "bogus=1" (use unit=<text> or decimals=<0-6>)`,
		"text x":            `source: format "a": text takes no options`,
		"seconds x":         `source: format "a": seconds takes no options`,
		"date dd/mm/yy":     `source: format "a": date: pattern has no date token (D, DD, M, MM, Mth, YY, YYYY)`,
		"number unit=\u00e9": `source: format "a": number: unit "` + "\u00e9" + `" must be printable text with no space, ',' or '=' (one Formats entry per line)`,
		"nope":              `source: format "a": unknown formatter "nope"`,
	} {
		if _, err := ParseSourceConfig(src(spec)); err == nil || err.Error() != want {
			t.Errorf("%q: err %v; want %q", spec, err, want)
		}
	}
}

// The width a value is shrunk to comes from the template slot, not a constant.
func TestValuesSlotWidths(t *testing.T) {
	cfg := SourceConfig{Range: "A", Columns: []string{"s#", "t#"}, Rows: 1, Format: map[string]string{"s#": "number", "t#": "number"}}
	got, err := Values(Rows{Formatted: [][]string{{"1,234,567", "1,234,567"}}}, cfg, map[string]int{"s1": 6, "t1": 12})
	if err != nil || got["s1"] != "1.23m" || got["t1"] != "1,234,567" {
		t.Fatalf("got %v, %v", got, err)
	}
}
