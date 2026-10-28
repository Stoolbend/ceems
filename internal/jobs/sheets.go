package jobs

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/url"
	"strings"
	"time"

	"google.golang.org/api/googleapi"
	"google.golang.org/api/option"
	"google.golang.org/api/sheets/v4"
)

// Sheets is the Source backed by the Google Sheets API.
type Sheets struct{ svc *sheets.Service }

// NewSheets builds a client from a service-account key file. Extra options exist
// so tests can point the client at a fake. The returned error never carries the
// library's text, which can quote the key file.
func NewSheets(ctx context.Context, keyFile string, opts ...option.ClientOption) (*Sheets, error) {
	opts = append([]option.ClientOption{
		option.WithCredentialsFile(keyFile),
		option.WithScopes(sheets.SpreadsheetsReadonlyScope),
	}, opts...)
	svc, err := sheets.NewService(ctx, opts...)
	if err != nil {
		return nil, fmt.Errorf("cannot create the Sheets client (%T)", err)
	}
	return &Sheets{svc}, nil
}

// Read fetches rng twice, as display text and as raw values. rng carries no
// sheet name, so the API reads the first sheet.
func (s *Sheets) Read(ctx context.Context, sheetID, rng string) (Rows, error) {
	f, err := getRetry401(ctx, s.svc.Spreadsheets.Values.Get(sheetID, rng).ValueRenderOption("FORMATTED_VALUE").Context(ctx))
	if err != nil {
		return Rows{}, classify(err, sheetID)
	}
	if len(f.Values) == 0 {
		return Rows{}, ErrEmpty
	}
	r, err := getRetry401(ctx, s.svc.Spreadsheets.Values.Get(sheetID, rng).
		ValueRenderOption("UNFORMATTED_VALUE").DateTimeRenderOption("SERIAL_NUMBER").Context(ctx))
	if err != nil {
		return Rows{}, classify(err, sheetID)
	}
	out := Rows{Raw: r.Values, Formatted: make([][]string, len(f.Values))}
	for i, row := range f.Values {
		out.Formatted[i] = make([]string, len(row))
		for j, c := range row {
			if c != nil {
				out.Formatted[i][j] = strings.TrimSpace(fmt.Sprint(c))
			}
		}
	}
	return out, nil
}

// retryPause is the wait before the one retry after an HTTP 401. Tests shorten it.
var retryPause = 2 * time.Second

// getRetry401 runs the call and, on HTTP 401 only, retries it once after
// retryPause. An hourly run can land just as Google expires the cached token.
func getRetry401(ctx context.Context, call *sheets.SpreadsheetsValuesGetCall) (*sheets.ValueRange, error) {
	v, err := call.Do()
	var ge *googleapi.Error
	if !errors.As(err, &ge) || ge.Code != 401 {
		return v, err
	}
	select {
	case <-time.After(retryPause):
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	return call.Do()
}

// classify maps a library error to a fixed-shape error. Library text is passed
// on only where it cannot hold the key or a response body.
func classify(err error, sheetID string) error {
	scrub := func(s string) string { return strings.ReplaceAll(s, sheetID, "<sheet>") }
	var ge *googleapi.Error
	if errors.As(err, &ge) {
		if ge.Code == 404 || ge.Code == 403 {
			return ErrSheetNotFound
		}
		return fmt.Errorf("%w: HTTP %d: %s", ErrUnreachable, ge.Code, scrub(ge.Message))
	}
	var ue *url.Error
	if errors.As(err, &ue) {
		// A token exchange failure arrives wrapped in a url.Error and its text
		// carries the response body, so only network errors keep their text.
		var oe *net.OpError
		if errors.As(ue.Err, &oe) || errors.Is(ue.Err, context.Canceled) || errors.Is(ue.Err, context.DeadlineExceeded) {
			return fmt.Errorf("%w: %s", ErrUnreachable, scrub(ue.Err.Error()))
		}
		return fmt.Errorf("%w: %T", ErrUnreachable, ue.Err)
	}
	return fmt.Errorf("%w: %T", ErrUnreachable, err)
}
