package api

import "testing"

func TestIsAllowedOrigin(t *testing.T) {
	t.Setenv("MACHDOWN_ALLOWED_ORIGINS", "https://panel.example.com, https://other.example.com")

	cases := map[string]bool{
		"http://localhost":                   true,
		"https://localhost:5173":             true,
		"http://127.0.0.1:8888":              true,
		"chrome-extension://abcdefgh":        true,
		"wails://wails":                      true,
		"https://panel.example.com":          true,
		"https://other.example.com":          true,
		"http://localhost.evil.com":          false,
		"https://localhost@evil.com":         false,
		"http://127.0.0.1.evil.com":          false,
		"https://evil.com":                   false,
		"https://panel.example.com.evil.com": false,
		"chrome-extension://":                false,
		"ftp://localhost":                    false,
	}
	for origin, want := range cases {
		if got := IsAllowedOrigin(origin); got != want {
			t.Errorf("IsAllowedOrigin(%q) = %v, want %v", origin, got, want)
		}
	}
}
