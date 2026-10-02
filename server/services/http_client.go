package services

import (
	"fmt"
	"net/http"
	"time"
)

var sharedTransport = &http.Transport{
	Proxy:                 http.ProxyFromEnvironment,
	DialContext:           SafeDialContext(30 * time.Second),
	ForceAttemptHTTP2:     true,
	MaxIdleConns:          100,
	MaxIdleConnsPerHost:   20,
	IdleConnTimeout:       90 * time.Second,
	TLSHandshakeTimeout:   10 * time.Second,
	ExpectContinueTimeout: 1 * time.Second,
}

var sharedHTTPClient = &http.Client{
	Timeout:   60 * time.Second,
	Transport: sharedTransport,
}

// GetSharedHTTPClient returns a shallow copy of the shared HTTP client.
// This allows modifying fields like CheckRedirect without affecting other requests,
// while still reusing the underlying connection pool (Transport).
func GetSharedHTTPClient() *http.Client {
	clientCopy := *sharedHTTPClient
	return &clientCopy
}

// GetSharedTransport returns the underlying shared transport
// This is useful if a custom http.Client is needed but we still want connection pooling.
func GetSharedTransport() *http.Transport {
	return sharedTransport
}

// SafeCheckRedirect creates a CheckRedirect policy with SSRF guard and maximum redirect hops.
func SafeCheckRedirect(maxRedirects int) func(req *http.Request, via []*http.Request) error {
	if maxRedirects <= 0 {
		maxRedirects = 10
	}
	return func(req *http.Request, via []*http.Request) error {
		if len(via) >= maxRedirects {
			return http.ErrUseLastResponse
		}

		// Security: SSRF validation on redirect target
		if err := isInternalURL(req.URL.String()); err != nil {
			return fmt.Errorf("SSRF guard: redirect blocked: %w", err)
		}

		if len(via) > 0 {
			if cookie := via[0].Header.Get("Cookie"); cookie != "" {
				req.Header.Set("Cookie", cookie)
			}
			if ua := via[0].Header.Get("User-Agent"); ua != "" {
				req.Header.Set("User-Agent", ua)
			}
		}
		return nil
	}
}
