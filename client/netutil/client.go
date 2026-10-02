package netutil

import (
	"crypto/sha256"
	"crypto/subtle"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"errors"
	"net/http"
	"strings"
	"sync"
)

var (
	cacheMu sync.Mutex
	cache   = map[string]*http.Client{}
)

// NormalizeFingerprint accepts a SHA-256 fingerprint in hex, with or without
// colons or spaces, in any case.
func NormalizeFingerprint(fp string) string {
	fp = strings.ToLower(strings.TrimSpace(fp))
	return strings.NewReplacer(":", "", " ", "").Replace(fp)
}

// Client returns an HTTP client for the MachDown server. With a fingerprint, the
// server is trusted only if its leaf certificate matches it (needed for the
// self-signed certificate); without one, standard certificate verification applies.
// Clients are cached per fingerprint so connections are reused.
func Client(fingerprint string) *http.Client {
	fp := NormalizeFingerprint(fingerprint)

	cacheMu.Lock()
	defer cacheMu.Unlock()
	if c, ok := cache[fp]; ok {
		return c
	}

	tlsCfg := &tls.Config{MinVersion: tls.VersionTLS12}
	if fp != "" {
		// Chain validation is replaced by the pin below.
		tlsCfg.InsecureSkipVerify = true
		tlsCfg.VerifyPeerCertificate = func(rawCerts [][]byte, _ [][]*x509.Certificate) error {
			if len(rawCerts) == 0 {
				return errors.New("server presented no certificate")
			}
			sum := sha256.Sum256(rawCerts[0])
			if subtle.ConstantTimeCompare([]byte(hex.EncodeToString(sum[:])), []byte(fp)) != 1 {
				return errors.New("server certificate fingerprint mismatch")
			}
			return nil
		}
	}

	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.TLSClientConfig = tlsCfg
	c := &http.Client{Transport: transport}
	cache[fp] = c
	return c
}
