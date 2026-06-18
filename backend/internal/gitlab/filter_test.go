package gitlab

import "testing"

func TestFilePolicyRejectsBinaryLargeSecretsAndDependencies(t *testing.T) {
	policy := FilePolicy{
		MaxBytes:          100,
		ExcludedDirs:      []string{"vendor", "dist", "node_modules"},
		ExcludedFiles:     []string{".env", "*.pem"},
		AllowedExtensions: []string{".go", ".md"},
	}
	tests := []struct {
		path   string
		size   int64
		sample []byte
		want   bool
	}{
		{"cmd/main.go", 20, []byte("package main"), true},
		{"vendor/x/a.go", 20, []byte("package x"), false},
		{"README.md", 101, []byte("text"), false},
		{".env", 20, []byte("TOKEN=x"), false},
		{"cert.pem", 20, []byte("secret"), false},
		{"image.go", 20, []byte{0, 1, 2}, false},
		{"archive.zip", 20, []byte("text"), false},
	}
	for _, tt := range tests {
		if got := policy.Allow(tt.path, tt.size, tt.sample); got != tt.want {
			t.Errorf("Allow(%q)=%v want %v", tt.path, got, tt.want)
		}
	}
}
