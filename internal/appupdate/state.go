package appupdate

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"strings"
	"time"

	"github.com/carroarmato0/nextui-itchio-pak/internal/logger"
)

// Release is one release as far as updates care.
type Release struct {
	Tag    string `json:"tag"`
	URL    string `json:"url"`              // the release page (html_url)
	Asset  string `json:"asset,omitempty"`  // muOS .muxapp download URL
	Digest string `json:"digest,omitempty"` // "sha256:<hex>"
	Size   int64  `json:"size,omitempty"`
	// The NextUI pak zip, installed in place (NextUI install spec §1).
	NextUIAsset  string `json:"nextui_asset,omitempty"`
	NextUIDigest string `json:"nextui_digest,omitempty"`
	NextUISize   int64  `json:"nextui_size,omitempty"`
}

// HasNextUIAsset: the release can be installed in place on NextUI.
func (r *Release) HasNextUIAsset() bool {
	return r != nil && r.NextUIAsset != "" && r.NextUISize > 0 && strings.HasPrefix(r.NextUIDigest, "sha256:")
}

// ETag keys in State.ETag, one per source.
const (
	etagReleases = "releases"
	etagPakJSON  = "pakjson"
)

// State is update_state.json. It is not a user choice, so it lives apart
// from config.json.
type State struct {
	CheckedAt time.Time            `json:"checked_at"`
	NotBefore time.Time            `json:"not_before"`
	ETag      map[string]string    `json:"etag,omitempty"`
	Latest    map[Channel]*Release `json:"latest,omitempty"`
	// PakJSON is what pak.json on main says: the Pak Store's own source.
	PakJSON  *Release           `json:"pakjson,omitempty"`
	Notified map[Channel]string `json:"notified,omitempty"`
	// FailedInstall is a version that was installed in place and did not
	// start; the notice stays quiet for it (NextUI install spec §4.3).
	FailedInstall string `json:"failed_install,omitempty"`
}

// LoadState reads path. A missing or corrupt file is an empty state: the worst
// outcome is one extra check and one notice shown again.
func LoadState(path string) *State {
	st := &State{}
	data, err := os.ReadFile(path)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		logger.Debug("appupdate: no %s yet", path)
	case err != nil:
		logger.Warn("appupdate: read %s: %v", path, err)
	default:
		if err := json.Unmarshal(data, st); err != nil {
			logger.Warn("appupdate: %s is corrupt, starting empty: %v", path, err)
			st = &State{}
		}
	}
	if st.ETag == nil {
		st.ETag = map[string]string{}
	}
	if st.Latest == nil {
		st.Latest = map[Channel]*Release{}
	}
	if st.Notified == nil {
		st.Notified = map[Channel]string{}
	}
	return st
}

// Save writes the state atomically (tmp + rename), like settings.Save.
func (s *State) Save(path string) error {
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0644); err != nil {
		logger.Error("appupdate: write %s: %v", tmp, err)
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		logger.Error("appupdate: rename %s → %s: %v", tmp, path, err)
		return err
	}
	logger.Debug("appupdate: state saved to %s", path)
	return nil
}
