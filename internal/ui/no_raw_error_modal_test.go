package ui

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// A modal must never draw a raw request error: its text quotes the request
// URL (download_key_id, signed CDN tokens) and, for HTTP errors, can carry
// server HTML. Screens pass errors through problemText instead. The screens
// are SDL code and cannot run here, so this checks their source.
func TestNoRawErrorInModals(t *testing.T) {
	raw := regexp.MustCompile(`ShowModal\([^\n]*\berr\.Error\(\)`)
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		for i, line := range strings.Split(string(b), "\n") {
			if raw.MatchString(line) {
				t.Errorf("%s:%d draws a raw error in a modal: %s", f, i+1, strings.TrimSpace(line))
			}
		}
	}
}
