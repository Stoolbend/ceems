package jobs

import (
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
	"testing"
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
