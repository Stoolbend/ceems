package jobs

import (
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// SourceConfig is the bindings.source JSON. Unknown keys are an error.
type SourceConfig struct {
	Sheet   string            `json:"sheet"`   // spreadsheet id; required
	Range   string            `json:"range"`   // A1 range without a sheet name, e.g. "C5:E"; required
	Columns []string          `json:"columns"` // slot name per column with '#' for the entry index, e.g. "p#name"; required, non-empty
	Rows    int               `json:"rows"`    // entries to produce, 1-24; required
	Format  map[string]string `json:"format"`  // optional, column pattern -> "text", "number [unit=<text>]", "seconds" or "date [pattern]"
}

// ParseSourceConfig decodes and validates a bindings.source JSON. Its errors
// never include the sheet value.
func ParseSourceConfig(s string) (SourceConfig, error) {
	var c SourceConfig
	dec := json.NewDecoder(strings.NewReader(s))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&c); err != nil {
		var te *json.UnmarshalTypeError
		if errors.As(err, &te) {
			return c, fmt.Errorf("source: %s: must be a %s", te.Field, te.Type.Kind())
		}
		if key, ok := strings.CutPrefix(err.Error(), "json: unknown field "); ok {
			return c, fmt.Errorf("source: unknown key %s", key)
		}
		return c, fmt.Errorf("source: %v", err)
	}
	switch {
	case c.Sheet == "":
		return c, errors.New("source: sheet: required")
	case c.Range == "":
		return c, errors.New("source: range: required")
	case len(c.Columns) == 0:
		return c, errors.New("source: columns: required")
	case c.Rows < 1 || c.Rows > 24:
		return c, errors.New("source: rows: must be 1 to 24")
	}
	for pat, spec := range c.Format {
		if _, err := parseFormat(spec); err != nil {
			return c, fmt.Errorf("source: format %q: %v", pat, err)
		}
	}
	return c, nil
}

// Values maps a read to slot values: a row is complete when every configured
// column's formatted text is non-empty; leading incomplete rows are skipped; the
// first incomplete row after a complete one ends the entries; at most cfg.Rows
// entries. Every key cfg.Columns x 1..cfg.Rows is present, "" when there is no
// entry, so the template's blanking rule sees it. widths maps a slot name to its width in
// cells; a slot missing from it has no limit.
func Values(r Rows, cfg SourceConfig, widths map[string]int) (map[string]string, error) {
	if len(r.Formatted) == 0 {
		return nil, ErrEmpty
	}
	n := len(cfg.Columns)
	usable := false
	for _, row := range r.Formatted {
		if len(row) >= n {
			usable = true
			break
		}
	}
	if !usable {
		return nil, fmt.Errorf("%w: range %s needs %d columns", ErrMissingColumns, cfg.Range, n)
	}

	out := make(map[string]string, n*cfg.Rows)
	for i := 1; i <= cfg.Rows; i++ {
		for _, col := range cfg.Columns {
			out[strings.ReplaceAll(col, "#", strconv.Itoa(i))] = ""
		}
	}
	entry := 0
	for ri, row := range r.Formatted {
		if entry == cfg.Rows {
			break
		}
		if !complete(row, n) {
			if entry == 0 {
				continue
			}
			break
		}
		entry++
		for ci, col := range cfg.Columns {
			var raw any
			if ri < len(r.Raw) && ci < len(r.Raw[ri]) {
				raw = r.Raw[ri][ci]
			}
			slot := strings.ReplaceAll(col, "#", strconv.Itoa(entry))
			text, err := formatValue(cfg.Format[col], row[ci], raw, widths[slot])
			if err != nil {
				return nil, err
			}
			out[slot] = text
		}
	}
	return out, nil
}

func complete(row []string, n int) bool {
	if len(row) < n {
		return false
	}
	for _, c := range row[:n] {
		if c == "" {
			return false
		}
	}
	return true
}
