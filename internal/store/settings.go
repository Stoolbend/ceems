package store

import (
	"database/sql"
	"slices"
	"strconv"
	"strings"
)

// Settings is the system settings row set. Values are validated by the HTTP layer
// before they reach SaveSettings.
type Settings struct {
	SystemName          string
	VbitConf            bool
	HeaderTemplate      string // 32 cells, each 0x00 to 0x7F
	InitialTeletextPage string
	RowAdaptiveMode     bool
	LinesPerField       int
	MagazinePriority    string
	StatusDisplay       string
	IndexPageEnabled    bool   // every page's sixth link is IndexPage
	IndexPage           string // three hex digits

	NetworkIdentificationCode    string // four hex digits
	CountryNetworkIdentification string // four hex digits
	DatacastLines                int
	MagazineSerial               bool
}

// pairs lists each setting as its stored key and text value, in the vbit.conf order.
func (s Settings) pairs() [][2]string {
	return [][2]string{
		{"system_name", s.SystemName},
		{"vbit_conf", strconv.FormatBool(s.VbitConf)},
		{"header_template", s.HeaderTemplate},
		{"initial_teletext_page", s.InitialTeletextPage},
		{"row_adaptive_mode", strconv.FormatBool(s.RowAdaptiveMode)},
		{"lines_per_field", strconv.Itoa(s.LinesPerField)},
		{"magazine_priority", s.MagazinePriority},
		{"status_display", s.StatusDisplay},
		{"network_identification_code", s.NetworkIdentificationCode},
		{"country_network_identification", s.CountryNetworkIdentification},
		{"datacast_lines", strconv.Itoa(s.DatacastLines)},
		{"magazine_serial", strconv.FormatBool(s.MagazineSerial)},
		{"index_page_enabled", strconv.FormatBool(s.IndexPageEnabled)},
		{"index_page", s.IndexPage},
	}
}

// Settings returns the stored settings.
func (s *Store) Settings() (Settings, error) {
	return readSettings(s.db.Query)
}

func readSettings(query func(string, ...any) (*sql.Rows, error)) (Settings, error) {
	rows, err := query("SELECT key, value FROM settings")
	if err != nil {
		return Settings{}, err
	}
	defer rows.Close()
	var out Settings
	for rows.Next() {
		var k, v string
		if err := rows.Scan(&k, &v); err != nil {
			return Settings{}, err
		}
		switch k {
		case "system_name":
			out.SystemName = v
		case "vbit_conf":
			out.VbitConf = v == "true"
		case "header_template":
			out.HeaderTemplate = v
		case "initial_teletext_page":
			out.InitialTeletextPage = v
		case "row_adaptive_mode":
			out.RowAdaptiveMode = v == "true"
		case "lines_per_field":
			out.LinesPerField, _ = strconv.Atoi(v)
		case "magazine_priority":
			out.MagazinePriority = v
		case "status_display":
			out.StatusDisplay = v
		case "network_identification_code":
			out.NetworkIdentificationCode = v
		case "country_network_identification":
			out.CountryNetworkIdentification = v
		case "datacast_lines":
			out.DatacastLines, _ = strconv.Atoi(v)
		case "magazine_serial":
			out.MagazineSerial = v == "true"
		case "index_page_enabled":
			out.IndexPageEnabled = v == "true"
		case "index_page":
			out.IndexPage = v
		}
	}
	return out, rows.Err()
}

// SaveSettings writes the keys of s that differ from the stored values, in one
// transaction, and returns their stored names. When any changed it writes one
// settings.update audit row naming the keys (never the values) and calls notify
// after the commit. An unchanged save writes nothing.
func (s *Store) SaveSettings(n Settings, a Audit) (changed []string, err error) {
	tx, err := s.db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	old, err := readSettings(tx.Query)
	if err != nil {
		return nil, err
	}
	oldPairs := old.pairs()
	for i, p := range n.pairs() {
		if p == oldPairs[i] {
			continue
		}
		if _, err := tx.Exec("UPDATE settings SET value = ? WHERE key = ?", p[1], p[0]); err != nil {
			return nil, err
		}
		changed = append(changed, p[0])
	}
	if len(changed) == 0 {
		return nil, nil
	}
	now := utcNow()
	a.Action, a.Target, a.Detail = "settings.update", "settings", strings.Join(changed, ",")
	if err := insertAudit(tx, now, a); err != nil {
		return nil, err
	}
	// Turning the index off rewrites nothing: the sixth links stay as they are.
	if n.IndexPageEnabled && (slices.Contains(changed, "index_page_enabled") || slices.Contains(changed, "index_page")) {
		mpps, err := pagesWhere(tx, "SELECT mpp FROM pages ORDER BY mpp")
		if err != nil {
			return nil, err
		}
		if _, err := rewritePages(tx, mpps, now, a, false); err != nil {
			return nil, err
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	s.notify()
	return changed, nil
}
