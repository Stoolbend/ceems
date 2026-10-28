package jobs

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestNewSheetsMalformedKeyDoesNotEchoContents(t *testing.T) {
	const marker = "UNIQUE-MARKER-9f3a1c"
	p := filepath.Join(t.TempDir(), "key.json")
	if err := os.WriteFile(p, []byte(marker), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := NewSheets(t.Context(), p)
	if err == nil {
		t.Fatal("want error")
	}
	if strings.Contains(err.Error(), marker) {
		t.Fatalf("error echoes key file contents: %v", err)
	}
}

func TestSheetsReadAgainstFake(t *testing.T) {
	f := newFake(t)
	s := f.client(t)
	ctx := t.Context()

	t.Run("formatted and raw grids", func(t *testing.T) {
		f.valuesFn.Store(valuesHandler(func(w http.ResponseWriter, r *http.Request, formatted bool) {
			if !strings.Contains(r.URL.Path, "/v4/spreadsheets/"+fakeSheetID+"/values/") {
				t.Errorf("unexpected path %q", r.URL.Path)
			}
			w.Header().Set("Content-Type", "application/json")
			if formatted {
				io.WriteString(w, `{"values":[["  Ann ","1,200"],["Bob"]]}`)
				return
			}
			if r.URL.Query().Get("dateTimeRenderOption") != "SERIAL_NUMBER" {
				t.Errorf("raw read lacks SERIAL_NUMBER: %q", r.URL.RawQuery)
			}
			io.WriteString(w, `{"values":[["Ann",1200],["Bob"]]}`)
		}))
		got, err := s.Read(ctx, fakeSheetID, "A1:B")
		if err != nil {
			t.Fatal(err)
		}
		wantF := [][]string{{"Ann", "1,200"}, {"Bob"}}
		wantR := [][]any{{"Ann", 1200.0}, {"Bob"}}
		if !reflect.DeepEqual(got.Formatted, wantF) || !reflect.DeepEqual(got.Raw, wantR) {
			t.Fatalf("got %#v", got)
		}
	})

	errCases := []struct {
		name  string
		serve valuesHandler
		want  error
	}{
		{"404", func(w http.ResponseWriter, _ *http.Request, _ bool) { http.Error(w, "nope", 404) }, ErrSheetNotFound},
		{"403", func(w http.ResponseWriter, _ *http.Request, _ bool) { http.Error(w, "nope", 403) }, ErrSheetNotFound},
		{"empty object", jsonValues(`{}`), ErrEmpty},
		{"500", func(w http.ResponseWriter, _ *http.Request, _ bool) { http.Error(w, "boom", 500) }, ErrUnreachable},
	}
	for _, tc := range errCases {
		t.Run(tc.name, func(t *testing.T) {
			f.valuesFn.Store(tc.serve)
			_, err := s.Read(ctx, fakeSheetID, "A1:B")
			if !errors.Is(err, tc.want) {
				t.Fatalf("got %v want %v", err, tc.want)
			}
		})
	}

	t.Run("500 message containing the sheet id is scrubbed", func(t *testing.T) {
		f.valuesFn.Store(valuesHandler(func(w http.ResponseWriter, _ *http.Request, _ bool) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(500)
			fmt.Fprintf(w, `{"error":{"code":500,"message":"failed on %s"}}`, fakeSheetID)
		}))
		_, err := s.Read(ctx, fakeSheetID, "A1:B")
		if !errors.Is(err, ErrUnreachable) || strings.Contains(err.Error(), fakeSheetID) {
			t.Fatalf("got %v", err)
		}
	})

	t.Run("token endpoint 400 echoing the request surfaces nothing", func(t *testing.T) {
		f.valuesFn.Store(jsonValues(`{"values":[["x"]]}`))
		f.tokenMode.Store("echo400")
		defer f.tokenMode.Store("ok")
		_, err := f.client(t).Read(ctx, fakeSheetID, "A1:B")
		if !errors.Is(err, ErrUnreachable) {
			t.Fatalf("got %v", err)
		}
		for _, bad := range []string{fakeEmail, "assertion", "grant_type", fakeSheetID, f.pemBody} {
			if strings.Contains(err.Error(), bad) {
				t.Fatalf("error contains %q: %v", bad, err)
			}
		}
	})

	t.Run("server closed", func(t *testing.T) {
		g := newFake(t)
		g.valuesFn.Store(jsonValues(`{"values":[["x"]]}`))
		c := g.client(t)
		g.srv.Close()
		_, err := c.Read(ctx, fakeSheetID, "A1:B")
		if !errors.Is(err, ErrUnreachable) || strings.Contains(err.Error(), fakeSheetID) {
			t.Fatalf("got %v", err)
		}
	})
}

func TestUnconfiguredReturnsErrNoKey(t *testing.T) {
	if _, err := (Unconfigured{}).Read(t.Context(), "s", "A1"); !errors.Is(err, ErrNoKey) {
		t.Fatalf("got %v", err)
	}
}

func TestClassifyHidesLibraryText(t *testing.T) {
	const id = "SHEET-ID-VALUE"
	cases := []struct {
		name string
		err  error
		keep string // text that must survive, if any
	}{
		{"plain error", errors.New("MARKER secret-body " + id), ""},
		{"wrapped plain error", fmt.Errorf("wrapped: %w", errors.New("MARKER "+id)), ""},
		{"url.Error with plain inner", &url.Error{Op: "Get", URL: "https://x/" + id, Err: errors.New("MARKER body")}, ""},
		{"url.Error with OpError", &url.Error{Op: "Get", URL: "https://x/" + id,
			Err: &net.OpError{Op: "dial", Net: "tcp", Err: errors.New("refused " + id)}}, "dial tcp: refused"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := classify(tc.err, id)
			if !errors.Is(err, ErrUnreachable) {
				t.Fatalf("got %v", err)
			}
			for _, bad := range []string{"MARKER", id} {
				if strings.Contains(err.Error(), bad) {
					t.Fatalf("error contains %q: %v", bad, err)
				}
			}
			if tc.keep != "" && !strings.Contains(err.Error(), tc.keep) {
				t.Fatalf("error lost %q: %v", tc.keep, err)
			}
		})
	}
}

func TestSheetsRead401Retry(t *testing.T) {
	old := retryPause
	retryPause = time.Millisecond
	t.Cleanup(func() { retryPause = old })
	f := newFake(t)
	s := f.client(t)

	// serve answers status for the first n requests of the chosen render option
	// (formatted or raw), then 200. It returns the per-option request counters.
	serve := func(status, n int32, failFormatted bool) (fmtN, rawN *atomic.Int32) {
		fmtN, rawN = new(atomic.Int32), new(atomic.Int32)
		f.valuesFn.Store(valuesHandler(func(w http.ResponseWriter, _ *http.Request, formatted bool) {
			c := rawN
			if formatted {
				c = fmtN
			}
			k := c.Add(1)
			if formatted == failFormatted && k <= n {
				http.Error(w, "nope", int(status))
				return
			}
			w.Header().Set("Content-Type", "application/json")
			io.WriteString(w, `{"values":[["x"]]}`)
		}))
		return
	}

	for _, tc := range []struct {
		name      string
		formatted bool
	}{{"formatted 401 then 200", true}, {"raw 401 then 200", false}} {
		t.Run(tc.name, func(t *testing.T) {
			fmtN, rawN := serve(401, 1, tc.formatted)
			if _, err := s.Read(t.Context(), fakeSheetID, "A1:B"); err != nil {
				t.Fatal(err)
			}
			want := [2]int32{1, 2}
			if tc.formatted {
				want = [2]int32{2, 1}
			}
			if got := [2]int32{fmtN.Load(), rawN.Load()}; got != want {
				t.Fatalf("requests (formatted, raw) = %v want %v", got, want)
			}
		})
	}

	t.Run("401 twice fails as before", func(t *testing.T) {
		fmtN, _ := serve(401, 2, true)
		_, err := s.Read(t.Context(), fakeSheetID, "A1:B")
		if !errors.Is(err, ErrUnreachable) || !strings.Contains(err.Error(), "HTTP 401") {
			t.Fatalf("got %v", err)
		}
		if fmtN.Load() != 2 {
			t.Fatalf("attempts = %d want 2", fmtN.Load())
		}
	})

	for _, code := range []int32{403, 404, 500} {
		t.Run(fmt.Sprint(code, " is not retried"), func(t *testing.T) {
			fmtN, _ := serve(code, 99, true)
			if _, err := s.Read(t.Context(), fakeSheetID, "A1:B"); err == nil {
				t.Fatal("want error")
			}
			if fmtN.Load() != 1 {
				t.Fatalf("attempts = %d want 1", fmtN.Load())
			}
		})
	}

	t.Run("cancel during the pause", func(t *testing.T) {
		retryPause = time.Minute
		defer func() { retryPause = time.Millisecond }()
		fmtN, _ := serve(401, 99, true)
		ctx, cancel := context.WithTimeout(t.Context(), 50*time.Millisecond)
		defer cancel()
		start := time.Now()
		_, err := s.Read(ctx, fakeSheetID, "A1:B")
		if !errors.Is(err, ErrUnreachable) || time.Since(start) > 5*time.Second || fmtN.Load() != 1 {
			t.Fatalf("err %v after %v, attempts %d", err, time.Since(start), fmtN.Load())
		}
	})
}
