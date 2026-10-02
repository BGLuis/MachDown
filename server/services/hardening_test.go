package services

import "testing"

func TestProgressEventIsFor(t *testing.T) {
	cases := []struct {
		name     string
		targets  string
		clientID string
		want     bool
	}{
		{"targeted client", `["pc","laptop"]`, "pc", true},
		{"other client", `["laptop"]`, "pc", false},
		{"job without targets", `null`, "pc", false},
		{"malformed targets", `not-json`, "pc", false},
		{"anonymous subscriber receives everything", `["laptop"]`, "", true},
	}
	for _, tc := range cases {
		got := ProgressEvent{TargetClients: tc.targets}.IsFor(tc.clientID)
		if got != tc.want {
			t.Errorf("%s: got %v, want %v", tc.name, got, tc.want)
		}
	}
}

func TestCategoryMatchesExtensionCaseInsensitively(t *testing.T) {
	for name, want := range map[string]string{
		"tool.AppImage": "Programs",
		"clip.MP4":      "Videos",
		"unknown.xyz":   "Others",
	} {
		if got := getCategoryFromFileName(name); got != want {
			t.Errorf("%s: got %q, want %q", name, got, want)
		}
	}
}

func TestSharedTransportIgnoresProxyEnvironment(t *testing.T) {
	t.Setenv("HTTPS_PROXY", "http://proxy.invalid:3128")
	t.Setenv("HTTP_PROXY", "http://proxy.invalid:3128")

	if GetSharedTransport().Proxy != nil {
		t.Fatal("a proxy would bypass the SSRF guard, which validates the dial target")
	}
}
