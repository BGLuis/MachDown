package api

import (
	"net/url"
	"os"
	"strings"
)

// IsAllowedOrigin reports whether a browser Origin may call the API: loopback
// hosts, the browser extension, the Wails webview, or an origin listed in
// MACHDOWN_ALLOWED_ORIGINS. Hosts are compared exactly, never by prefix, so
// "http://localhost.evil.com" is rejected.
func IsAllowedOrigin(origin string) bool {
	if origin == "" || origin == "null" {
		return true
	}

	if u, err := url.Parse(origin); err == nil {
		switch u.Scheme {
		case "chrome-extension", "wails":
			return u.Host != ""
		case "http", "https":
			if host := u.Hostname(); host == "localhost" || host == "127.0.0.1" {
				return true
			}
		}
	}

	for _, allowed := range strings.Split(os.Getenv("MACHDOWN_ALLOWED_ORIGINS"), ",") {
		if allowed = strings.TrimSpace(allowed); allowed != "" && allowed == origin {
			return true
		}
	}
	return false
}
