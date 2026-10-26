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

// fmtSpec is a parsed Formats value: the name, plus the unit and decimals of a number
// column or the output pattern of a date column. decimals is -1 when not set.
type fmtSpec struct {
	name, unit, pattern string
	decimals            int
}

const dateDefault = "DD Mth YY"

// parseFormat parses a binding's format value: a name and an optional argument.
// Its errors carry no column or sheet; the caller adds the prefix.
func parseFormat(spec string) (fmtSpec, error) {
	name, arg, _ := strings.Cut(strings.TrimSpace(spec), " ")
	arg = strings.TrimSpace(arg)
	f := fmtSpec{name: name, decimals: -1}
	switch name {
	case "text", "seconds":
		if arg != "" {
			return f, fmt.Errorf("%s takes no options", name)
		}
		if name == "seconds" {
			f.name, f.unit = "number", "sec"
		}
	case "number":
		seen := map[string]bool{}
		for _, word := range strings.Fields(arg) {
			key, val, hasEq := strings.Cut(word, "=")
			if (key != "unit" && key != "decimals") || !hasEq {
				return f, fmt.Errorf("number: unknown option %q (use unit=<text> or decimals=<0-6>)", word)
			}
			if seen[key] {
				return f, fmt.Errorf("number: option %s is given twice", key)
			}
			seen[key] = true
			if key == "decimals" {
				d, err := strconv.Atoi(val)
				if err != nil || d < 0 || d > 6 || val != strconv.Itoa(d) {
					return f, fmt.Errorf("number: decimals must be a whole number from 0 to 6, got %q", val)
				}
				f.decimals = d
				continue
			}
			if val == "" || strings.ContainsFunc(val, func(r rune) bool { return r < 0x21 || r > 0x7E || r == ',' || r == '=' }) {
				return f, fmt.Errorf("number: unit %q must be printable text with no space, ',' or '=' (one Formats entry per line)", val)
			}
			f.unit = val
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
		return formatNumber(formatted, f.unit, f.decimals, width), nil
	case "date":
		return formatDateValue(formatted, raw, f.pattern)
	}
	return formatted, nil
}

// formatNumber strips a cell to its number, fits it to the slot less the unit, appends
// the unit and cuts the result to the slot from the right. With decimals >= 0 the
// number is first rounded to that many places; without, the sheet text is kept when it
// fits. A number that does not fit loses places (fitNumber), then shrinks with k/m/b.
// A cell that is not a number comes back as it is, without the unit.
func formatNumber(formatted, unit string, decimals, width int) string {
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
	commas := strings.Contains(num, ",")
	if decimals >= 0 {
		num = fixedDecimals(f, decimals, commas)
	}
	if width > 0 && len(num) > budget {
		num = fitNumber(f, decimals, commas, budget)
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

// fixedDecimals renders f with exactly d places, with thousands commas if asked.
func fixedDecimals(f float64, d int, commas bool) string {
	s := strconv.FormatFloat(f, 'f', d, 64)
	if !commas {
		return s
	}
	s, neg := strings.CutPrefix(s, "-")
	intPart, frac, hasFrac := strings.Cut(s, ".")
	for i := len(intPart) - 3; i > 0; i -= 3 {
		intPart = intPart[:i] + "," + intPart[i:]
	}
	if hasFrac {
		intPart += "." + frac
	}
	if neg {
		intPart = "-" + intPart
	}
	return intPart
}

// fitNumber shortens f to maxLen. With decimals set it drops one place at a time down
// to none. Without, a value under 1000 keeps as many places as fit, minus trailing
// zeros. Anything still too long is abbreviated with k, m or b.
func fitNumber(f float64, decimals int, commas bool, maxLen int) string {
	trim := decimals < 0 && math.Abs(f) < 1000
	if trim {
		decimals = 6
	}
	for d := decimals; d >= 0; d-- {
		s := fixedDecimals(f, d, commas)
		if trim && d > 0 {
			s = strings.TrimRight(strings.TrimRight(s, "0"), ".")
		}
		if len(s) <= maxLen {
			return s
		}
	}
	return abbreviate(f, maxLen)
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

