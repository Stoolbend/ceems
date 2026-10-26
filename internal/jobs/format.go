package jobs

// Value formatters. The date parsing and the k/m/b abbreviation are ported from the
// old high-score generator (pagegen-highscores/main.go: abbreviateNumberIfNeeded,
// formatSheetDate and parseDateText).

import (
	"errors"
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
	"time"
)

var (
	reDirectDate  = regexp.MustCompile(`^(\d{1,2})[\s/-]([A-Za-z]{3,})[\s/-](\d{2,4})$`)
	reNumericDate = regexp.MustCompile(`^(\d{1,2})[\s/-](\d{1,2})[\s/-](\d{2,4})$`)

	monthNames = []string{"Jan", "Feb", "Mar", "Apr", "May", "Jun", "Jul", "Aug", "Sep", "Oct", "Nov", "Dec"}
	monthMap   = map[string]int{"jan": 1, "feb": 2, "mar": 3, "apr": 4, "may": 5, "jun": 6,
		"jul": 7, "aug": 8, "sep": 9, "oct": 10, "nov": 11, "dec": 12}
	dateLayouts = []string{time.RFC3339, "2006-01-02", "2006/01/02", "02 Jan 2006", "2 Jan 2006",
		"Jan 2 2006", "Jan 2, 2006", "2 January 2006", "January 2, 2006"}
)

// fmtSpec is a parsed Formats value: the name, plus the unit of a number column or
// the output pattern of a date column.
type fmtSpec struct{ name, unit, pattern string }

const dateDefault = "DD Mth YY"

// parseFormat parses a binding's format value: a name and an optional argument.
// Its errors carry no column or sheet; the caller adds the prefix.
func parseFormat(spec string) (fmtSpec, error) {
	name, arg, _ := strings.Cut(strings.TrimSpace(spec), " ")
	arg = strings.TrimSpace(arg)
	f := fmtSpec{name: name}
	switch name {
	case "text", "seconds":
		if arg != "" {
			return f, fmt.Errorf("%s takes no options", name)
		}
		if name == "seconds" {
			f.name, f.unit = "number", "sec"
		}
	case "number":
		if arg != "" {
			unit, ok := strings.CutPrefix(arg, "unit=")
			unit = strings.TrimSpace(unit)
			if !ok || unit == "" || strings.ContainsFunc(unit, func(r rune) bool { return r < 0x20 || r > 0x7E }) {
				return f, errors.New("number: expected unit=<text>")
			}
			f.unit = unit
		}
	case "date":
		f.pattern = arg
		if arg == "" {
			f.pattern = dateDefault
		} else if _, n := datePattern(time.Time{}, arg); n == 0 {
			return f, errors.New("date: pattern has no date token (D, DD, M, MM, Mth, YY, YYYY)")
		}
	default:
		return f, fmt.Errorf("unknown formatter %q", name)
	}
	return f, nil
}

// formatValue applies spec (default text) to one cell. width is the slot width in
// cells, 0 for no limit.
func formatValue(spec, formatted string, raw any, width int) (string, error) {
	if spec == "" {
		spec = "text"
	}
	f, err := parseFormat(spec)
	if err != nil {
		return "", err
	}
	switch f.name {
	case "number":
		return formatNumber(formatted, f.unit, width), nil
	case "date":
		return formatDateValue(formatted, raw, f.pattern)
	}
	return formatted, nil
}

// formatNumber strips a cell to its number, abbreviates it with k/m/b to fit the slot
// less the unit, appends the unit and cuts the result to the slot from the right. A
// cell that is not a number comes back as it is, without the unit.
func formatNumber(formatted, unit string, width int) string {
	num := strings.Map(func(r rune) rune {
		if strings.ContainsRune("0123456789.,-", r) {
			return r
		}
		return -1
	}, formatted)
	f, err := strconv.ParseFloat(strings.ReplaceAll(num, ",", ""), 64)
	if err != nil {
		return formatted
	}
	budget := width
	if unit != "" {
		budget = width - len(unit) - 1
	}
	if width > 0 && len(num) > budget {
		num = abbreviate(f, budget)
	}
	if unit != "" {
		num += " " + unit
	}
	if width > 0 && len(num) > width {
		num = num[:width]
	}
	return num
}

// formatDateValue is the "date" formatter: a numeric raw value is a serial day
// counted from 1899-12-30, anything else is parsed from the formatted text.
func formatDateValue(formatted string, raw any, pattern string) (string, error) {
	if v, ok := raw.(float64); ok && !math.IsNaN(v) && !math.IsInf(v, 0) {
		days := int(math.Floor(v))
		s, _ := datePattern(time.Date(1899, 12, 30, 0, 0, 0, 0, time.UTC).AddDate(0, 0, days), pattern)
		return s, nil
	}
	parsed, ok := parseDateText(formatted)
	if !ok {
		return "", fmt.Errorf("date: unsupported value %q", formatted)
	}
	s, _ := datePattern(parsed, pattern)
	return s, nil
}

// datePattern renders d by pattern and counts the tokens found. Tokens are case
// sensitive and the longest match wins; everything else is copied.
func datePattern(d time.Time, pattern string) (string, int) {
	d = d.UTC()
	tokens := []struct{ tok, val string }{
		{"YYYY", strconv.Itoa(d.Year())}, {"YY", fmt.Sprintf("%02d", d.Year()%100)},
		{"Mth", monthNames[d.Month()-1]}, {"MM", fmt.Sprintf("%02d", int(d.Month()))}, {"M", strconv.Itoa(int(d.Month()))},
		{"DD", fmt.Sprintf("%02d", d.Day())}, {"D", strconv.Itoa(d.Day())},
	}
	var b strings.Builder
	n := 0
scan:
	for i := 0; i < len(pattern); {
		for _, t := range tokens {
			if strings.HasPrefix(pattern[i:], t.tok) {
				b.WriteString(t.val)
				i += len(t.tok)
				n++
				continue scan
			}
		}
		b.WriteByte(pattern[i])
		i++
	}
	return b.String(), n
}

// abbreviate shortens f with a k, m or b suffix and two decimals (".00" dropped); if
// that is still over maxLen the decimals go too. It never cuts.
func abbreviate(f float64, maxLen int) string {
	abs := math.Abs(f)
	var short string
	switch {
	case abs >= 1_000_000_000:
		short = fmt.Sprintf("%.2fb", f/1_000_000_000)
	case abs >= 1_000_000:
		short = fmt.Sprintf("%.2fm", f/1_000_000)
	case abs >= 1_000:
		short = fmt.Sprintf("%.2fk", f/1_000)
	default:
		short = fmt.Sprintf("%.0f", f)
	}
	for _, suffix := range []string{"k", "m", "b"} {
		if strings.HasSuffix(short, ".00"+suffix) {
			short = strings.Replace(short, ".00"+suffix, suffix, 1)
			break
		}
	}
	if len(short) > maxLen {
		if idx := strings.Index(short, "."); idx != -1 {
			short = short[:idx] + short[len(short)-1:]
		}
	}
	return short
}

func parseDateText(value string) (time.Time, bool) {
	if match := reDirectDate.FindStringSubmatch(value); len(match) == 4 {
		day, _ := strconv.Atoi(match[1])
		month, ok := monthMap[strings.ToLower(match[2][:3])]
		yearRaw, _ := strconv.Atoi(match[3])
		if ok {
			if parsed, valid := makeUTCDate(normalizeYear(yearRaw), month, day); valid {
				return parsed, true
			}
		}
	}
	if match := reNumericDate.FindStringSubmatch(value); len(match) == 4 {
		first, _ := strconv.Atoi(match[1])
		second, _ := strconv.Atoi(match[2])
		yearRaw, _ := strconv.Atoi(match[3])
		day, month := first, second
		if first <= 12 && second > 12 {
			day, month = second, first
		}
		if parsed, valid := makeUTCDate(normalizeYear(yearRaw), month, day); valid {
			return parsed, true
		}
	}
	for _, layout := range dateLayouts {
		if parsed, err := time.Parse(layout, value); err == nil {
			return time.Date(parsed.Year(), parsed.Month(), parsed.Day(), 0, 0, 0, 0, time.UTC), true
		}
	}
	return time.Time{}, false
}

func makeUTCDate(year, month, day int) (time.Time, bool) {
	if month < 1 || month > 12 || day < 1 || day > 31 {
		return time.Time{}, false
	}
	date := time.Date(year, time.Month(month), day, 0, 0, 0, 0, time.UTC)
	if date.Year() != year || int(date.Month()) != month || date.Day() != day {
		return time.Time{}, false
	}
	return date, true
}

func normalizeYear(year int) int {
	switch {
	case year >= 100:
		return year
	case year >= 70:
		return 1900 + year
	default:
		return 2000 + year
	}
}

