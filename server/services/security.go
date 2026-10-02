package services

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"strings"
	"time"
)

// AllowInternalURLs indicates if internal/private URLs should be allowed (e.g. for E2E testing).
func AllowInternalURLs() bool {
	v := strings.ToLower(os.Getenv("MACHDOWN_ALLOW_INTERNAL"))
	return v == "true" || v == "1" || v == "yes"
}

// isInternalIP checks if an IP is loopback, private, link-local, multicast or cloud metadata.
func isInternalIP(ip net.IP) bool {
	if ip == nil {
		return true
	}

	// Unmap IPv4-mapped IPv6 (e.g. ::ffff:127.0.0.1)
	if ip4 := ip.To4(); ip4 != nil {
		ip = ip4
	}

	if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsInterfaceLocalMulticast() || ip.IsUnspecified() {
		return true
	}

	// Cloud metadata (AWS/GCP/Azure: 169.254.169.254)
	if ip.Equal(net.ParseIP("169.254.169.254")) {
		return true
	}

	// CGNAT: 100.64.0.0/10
	_, cgnat, _ := net.ParseCIDR("100.64.0.0/10")
	if cgnat != nil && cgnat.Contains(ip) {
		return true
	}

	return false
}

// ValidateURL parses rawURL, checks the scheme and verifies that it does not resolve to internal/private IPs.
func ValidateURL(rawURL string) (*url.URL, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return nil, fmt.Errorf("invalid url: %w", err)
	}

	scheme := strings.ToLower(u.Scheme)
	if scheme != "http" && scheme != "https" {
		return nil, fmt.Errorf("unsupported scheme: %s (only http/https allowed)", u.Scheme)
	}

	hostname := u.Hostname()
	if hostname == "" {
		return nil, fmt.Errorf("empty hostname")
	}

	if AllowInternalURLs() {
		return u, nil
	}

	ips, err := net.LookupIP(hostname)
	if err != nil {
		return nil, fmt.Errorf("failed to lookup ip for host %s: %w", hostname, err)
	}

	for _, ip := range ips {
		if isInternalIP(ip) {
			return nil, fmt.Errorf("URL %s resolves to internal/private IP: %s", rawURL, ip.String())
		}
	}

	return u, nil
}

// isInternalURL maintains backwards-compatibility with existing calls and tests.
func isInternalURL(rawURL string) error {
	_, err := ValidateURL(rawURL)
	return err
}

// SafeDialContext returns a DialContext func that resolves DNS and verifies the target IP
// at connection time, preventing DNS Rebinding (TOCTOU) attacks.
func SafeDialContext(timeout time.Duration) func(ctx context.Context, network, addr string) (net.Conn, error) {
	dialer := &net.Dialer{
		Timeout:   timeout,
		KeepAlive: 30 * time.Second,
	}

	return func(ctx context.Context, network, addr string) (net.Conn, error) {
		host, port, err := net.SplitHostPort(addr)
		if err != nil {
			return nil, err
		}

		if AllowInternalURLs() {
			return dialer.DialContext(ctx, network, addr)
		}

		ips, err := net.DefaultResolver.LookupIP(ctx, "ip", host)
		if err != nil {
			return nil, fmt.Errorf("SSRF guard: failed to resolve host %s: %w", host, err)
		}

		var lastErr error = errors.New("no valid external IP found")
		for _, ip := range ips {
			if isInternalIP(ip) {
				return nil, fmt.Errorf("SSRF guard: connection blocked to internal IP: %s", ip.String())
			}

			targetAddr := net.JoinHostPort(ip.String(), port)
			conn, err := dialer.DialContext(ctx, network, targetAddr)
			if err == nil {
				return conn, nil
			}
			lastErr = err
		}

		return nil, lastErr
	}
}
