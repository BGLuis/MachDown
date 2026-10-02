package netutil

import (
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func fingerprintOf(srv *httptest.Server) string {
	sum := sha256.Sum256(srv.Certificate().Raw)
	return hex.EncodeToString(sum[:])
}

func newTLSServer(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestPinnedFingerprintAcceptsMatchingSelfSignedCert(t *testing.T) {
	srv := newTLSServer(t)

	resp, err := Client(fingerprintOf(srv)).Get(srv.URL)
	if err != nil {
		t.Fatalf("expected pinned self-signed server to be trusted: %v", err)
	}
	resp.Body.Close()
}

func TestPinnedFingerprintAcceptsColonSeparatedUppercase(t *testing.T) {
	srv := newTLSServer(t)
	fp := fingerprintOf(srv)
	var parts []string
	for i := 0; i < len(fp); i += 2 {
		parts = append(parts, strings.ToUpper(fp[i:i+2]))
	}

	resp, err := Client(strings.Join(parts, ":")).Get(srv.URL)
	if err != nil {
		t.Fatalf("expected formatted fingerprint to match: %v", err)
	}
	resp.Body.Close()
}

func TestWrongFingerprintIsRejected(t *testing.T) {
	srv := newTLSServer(t)

	resp, err := Client(strings.Repeat("ab", 32)).Get(srv.URL)
	if err == nil {
		resp.Body.Close()
		t.Fatal("a server whose certificate does not match the pin must be rejected")
	}
}

func TestWithoutFingerprintSelfSignedCertIsRejected(t *testing.T) {
	srv := newTLSServer(t)

	resp, err := Client("").Get(srv.URL)
	if err == nil {
		resp.Body.Close()
		t.Fatal("without a pin, an untrusted self-signed certificate must be rejected")
	}
}
