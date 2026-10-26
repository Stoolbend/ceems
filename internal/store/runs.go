package store

// Run is one run of a binding. Error is the run's error text, "" when Outcome is "ok".
type Run struct {
	ID, BindingID                                int64
	Actor, StartedAt, FinishedAt, Outcome, Error string
}

// keepRuns is how many runs per binding the log keeps (D-020).
const keepRuns = 100

// InsertRun inserts r, then prunes that binding's runs to the newest keepRuns by id.
func (s *Store) InsertRun(r *Run) (int64, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	res, err := tx.Exec("INSERT INTO runs (binding_id, actor, started_at, finished_at, outcome, error) VALUES (?, ?, ?, ?, ?, ?)",
		r.BindingID, r.Actor, r.StartedAt, r.FinishedAt, r.Outcome, r.Error)
	if err != nil {
		return 0, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, err
	}
	if _, err := tx.Exec(`DELETE FROM runs WHERE binding_id = ? AND id NOT IN
		(SELECT id FROM runs WHERE binding_id = ? ORDER BY id DESC LIMIT ?)`, r.BindingID, r.BindingID, keepRuns); err != nil {
		return 0, err
	}
	return id, tx.Commit()
}

const runColumns = "id, binding_id, actor, started_at, finished_at, outcome, error"

func (s *Store) queryRuns(query string, args ...any) ([]Run, error) {
	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Run
	for rows.Next() {
		var r Run
		if err := rows.Scan(&r.ID, &r.BindingID, &r.Actor, &r.StartedAt, &r.FinishedAt, &r.Outcome, &r.Error); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// Runs returns the runs of bindingID, newest first.
func (s *Store) Runs(bindingID int64) ([]Run, error) {
	return s.queryRuns("SELECT "+runColumns+" FROM runs WHERE binding_id = ? ORDER BY id DESC", bindingID)
}

// LastRuns returns the newest run of every binding that has one, keyed by binding id.
func (s *Store) LastRuns() (map[int64]Run, error) {
	rs, err := s.queryRuns("SELECT " + runColumns + " FROM runs WHERE id IN (SELECT max(id) FROM runs GROUP BY binding_id)")
	if err != nil {
		return nil, err
	}
	out := make(map[int64]Run, len(rs))
	for _, r := range rs {
		out[r.BindingID] = r
	}
	return out, nil
}
