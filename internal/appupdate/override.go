package appupdate

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/carroarmato0/nextui-itchio-pak/internal/logger"
)

// overrideFile, in the data dir, points update checks at a test server
// instead of GitHub (NextUI install spec §6). Written by
// scripts/update-fixture.sh; never by the app.
const overrideFile = "update_source_override"

// OverrideBase returns the override's base URL, with a trailing slash.
func OverrideBase(dataDir string) (string, bool) {
	b, err := os.ReadFile(filepath.Join(dataDir, overrideFile))
	if err != nil {
		return "", false
	}
	line := strings.TrimSpace(strings.SplitN(string(b), "\n", 2)[0])
	if !strings.HasPrefix(line, "http://") && !strings.HasPrefix(line, "https://") {
		logger.Warn("appupdate: ignoring %s: %q is not an http(s) URL", overrideFile, line)
		return "", false
	}
	if !strings.HasSuffix(line, "/") {
		line += "/"
	}
	return line, true
}

// NewOverrideSource is NewSource against a fixture server.
func NewOverrideSource(userAgent, base string) *Source {
	s := NewSource(userAgent)
	s.ReleasesURL = base + "releases.json"
	s.PakJSONURL = base + "pak.json"
	return s
}
