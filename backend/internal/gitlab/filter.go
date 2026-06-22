package gitlab

import (
	"bytes"
	"path"
	"path/filepath"
	"strings"
	"unicode/utf8"
)

type FilePolicy struct {
	MaxBytes          int64
	ExcludedDirs      []string
	ExcludedFiles     []string
	AllowedExtensions []string
}

func (p FilePolicy) Allow(filePath string, size int64, sample []byte) bool {
	clean := strings.TrimPrefix(path.Clean(filePath), "/")
	if clean == "." || size > p.MaxBytes && p.MaxBytes > 0 {
		return false
	}
	for _, part := range strings.Split(clean, "/") {
		for _, excluded := range p.ExcludedDirs {
			if strings.EqualFold(part, strings.TrimSpace(excluded)) {
				return false
			}
		}
	}
	base := path.Base(clean)
	for _, pattern := range p.ExcludedFiles {
		matched, _ := path.Match(strings.TrimSpace(pattern), base)
		if matched {
			return false
		}
	}
	ext := strings.ToLower(filepath.Ext(base))
	if strings.EqualFold(base, "Dockerfile") {
		ext = ".dockerfile"
	}
	if len(p.AllowedExtensions) > 0 {
		ok := false
		for _, allowed := range p.AllowedExtensions {
			if ext == strings.ToLower(strings.TrimSpace(allowed)) {
				ok = true
				break
			}
		}
		if !ok {
			return false
		}
	}
	return !IsBinary(sample)
}

func IsBinary(b []byte) bool {
	if len(b) == 0 {
		return false
	}
	if len(b) > 8192 {
		b = b[:8192]
	}
	return bytes.IndexByte(b, 0) >= 0 || !utf8.Valid(b)
}
