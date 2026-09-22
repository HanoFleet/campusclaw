package materials

import (
	"bytes"
	"errors"
	"path"
	"strings"
	"unicode/utf8"
)

var ErrBadFile = errors.New("invalid file")

func AllowedExt(filename string) (string, bool) {
	base := displayBase(filename)
	ext := strings.ToLower(path.Ext(base))
	if ext == ".txt" || ext == ".md" {
		return ext, true
	}
	return ext, false
}

func ValidateText(b []byte) error {
	if bytes.HasPrefix(b, []byte{0xEF, 0xBB, 0xBF}) {
		b = b[3:]
	}
	if len(b) == 0 || !utf8.Valid(b) || strings.TrimSpace(string(b)) == "" {
		return ErrBadFile
	}
	return nil
}

func StripBOM(b []byte) []byte {
	if bytes.HasPrefix(b, []byte{0xEF, 0xBB, 0xBF}) {
		return b[3:]
	}
	return b
}

func DisplayTitle(filename string) string {
	base := displayBase(filename)
	base = strings.TrimSpace(base)
	if base == "" || base == "." || base == ".." {
		base = "未命名.md"
	}
	r := []rune(base)
	if len(r) > 255 {
		base = string(r[:255])
	}
	return base
}

func displayBase(filename string) string {
	name := strings.ReplaceAll(filename, "\\", "/")
	return path.Base(name)
}
