package jobs

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"google.golang.org/api/option"
)

const (
	fakeSheetID = "FAKE-SHEET-ID-0123456789"
	fakeEmail   = "robot@fake-project.iam.example.test"
)

type valuesHandler func(w http.ResponseWriter, r *http.Request, formatted bool)

// fake is a local stand-in for the OAuth token endpoint and the Sheets values endpoint.
type fake struct {
	srv       *httptest.Server
	keyFile   string
	pemBody   string       // a fragment of the private key PEM, for leak checks
	tokenMode atomic.Value // "ok" or "echo400"
	valuesFn  atomic.Value // valuesHandler
}

func newFake(t *testing.T) *fake {
	t.Helper()
	f := &fake{}
	f.tokenMode.Store("ok")
	f.srv = httptest.NewServer(http.HandlerFunc(f.serve))
	t.Cleanup(f.srv.Close)

	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	der, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	keyPEM := string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der}))
	f.pemBody = strings.Split(keyPEM, "\n")[1]
	cred, _ := json.Marshal(map[string]string{
		"type": "service_account", "project_id": "fake", "private_key_id": "k1",
		"private_key": keyPEM, "client_email": fakeEmail, "token_uri": f.srv.URL + "/token",
	})
	f.keyFile = filepath.Join(t.TempDir(), "key.json")
	if err := os.WriteFile(f.keyFile, cred, 0o600); err != nil {
		t.Fatal(err)
	}
	return f
}

func (f *fake) serve(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == "/token" {
		if f.tokenMode.Load() == "echo400" {
			b, _ := io.ReadAll(r.Body)
			w.WriteHeader(400)
			w.Write(append([]byte(fakeEmail+" "), b...))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"access_token":"tok","token_type":"Bearer","expires_in":3600}`)
		return
	}
	fn, _ := f.valuesFn.Load().(valuesHandler)
	fn(w, r, r.URL.Query().Get("valueRenderOption") == "FORMATTED_VALUE")
}

// client returns a Sheets client with no cached token, pointed at the fake.
func (f *fake) client(t *testing.T) *Sheets {
	t.Helper()
	s, err := NewSheets(t.Context(), f.keyFile, option.WithEndpoint(f.srv.URL))
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func jsonValues(body string) valuesHandler {
	return func(w http.ResponseWriter, _ *http.Request, _ bool) {
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, body)
	}
}
