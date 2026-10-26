// Package jobs reads data sources and maps them to template slot values.
package jobs

import (
	"context"
	"errors"
)

// Rows is one read of a range. Formatted[r][c] is the cell's display text, trimmed;
// Raw[r][c] is the unformatted value as the API returns it (string, float64, bool) or
// nil for an empty cell. Rows may be ragged, as the API returns them.
type Rows struct {
	Formatted [][]string
	Raw       [][]any
}

// Source reads a range from a sheet. Implementations never put sheetID in an error.
type Source interface {
	Read(ctx context.Context, sheetID, rng string) (Rows, error)
}

var (
	ErrNoKey          = errors.New("googleKeyFile is not configured")
	ErrUnreachable    = errors.New("sheet unreachable")
	ErrSheetNotFound  = errors.New("sheet not found or not shared with the service account")
	ErrEmpty          = errors.New("range has no values")
	ErrMissingColumns = errors.New("no row has every configured column")
)

// Unconfigured is the Source used when googleKeyFile is empty; Read returns ErrNoKey.
type Unconfigured struct{}

func (Unconfigured) Read(context.Context, string, string) (Rows, error) {
	return Rows{}, ErrNoKey
}
