package services

import (
	"testing"
)

func TestIsInternalURL(t *testing.T) {
	tests := []struct {
		url      string
		expected bool
	}{
		{"http://127.0.0.1/test", true},
		{"http://localhost/test", true},
		{"http://192.168.1.1/test", true},
		{"http://169.254.169.254/latest/meta-data", true},
		{"file:///etc/passwd", true},
		{"ftp://example.com/test", true},
		{"http://[::1]/test", true},
		{"https://google.com/test", false},
	}

	for _, tt := range tests {
		t.Run(tt.url, func(t *testing.T) {
			err := isInternalURL(tt.url)
			isInternal := err != nil
			if isInternal != tt.expected {
				t.Errorf("isInternalURL(%q) = %v, expected %v", tt.url, isInternal, tt.expected)
			}
		})
	}
}
