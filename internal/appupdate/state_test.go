package appupdate

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLoadState_missingIsEmpty(t *testing.T) {
	st := LoadState(filepath.Join(t.TempDir(), "update_state.json"))
	if st.Latest == nil || st.Notified == nil || st.ETag == nil {
		t.Fatal("maps must be non-nil so callers can assign into them")
	}
}

func TestLoadState_corruptIsEmpty(t *testing.T) {
	path := filepath.Join(t.TempDir(), "update_state.json")
	// What a power cut mid-write leaves behind.
	os.WriteFile(path, []byte(`{"checked_at":"2026-09-26T14:00:00Z","latest":{"stable":{"tag":"v1.`), 0644)
	st := LoadState(path)
	if !st.CheckedAt.IsZero() || len(st.Latest) != 0 {
		t.Fatalf("corrupt file must load as empty, got %+v", st)
	}
	st.Notified[RC] = "v1.1.0-rc3"
	if err := st.Save(path); err != nil {
		t.Fatal(err)
	}
	if LoadState(path).Notified[RC] != "v1.1.0-rc3" {
		t.Fatal("the next save must repair the file")
	}
}

func TestState_roundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "update_state.json")
	st := LoadState(path)
	st.CheckedAt = time.Date(2026, 9, 26, 14, 0, 0, 0, time.UTC)
	st.ETag[etagReleases] = `W/"abc"`
	st.Latest[Stable] = &Release{Tag: "v1.0.25", URL: "u", Asset: "a", Digest: "sha256:00", Size: 12678524}
	st.PakJSON = &Release{Tag: "v1.0.25", URL: "u"}
	st.Notified[Stable] = "v1.0.25"
	if err := st.Save(path); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path + ".tmp"); !os.IsNotExist(err) {
		t.Fatal("Save must not leave its temporary file behind")
	}
	got := LoadState(path)
	if !got.CheckedAt.Equal(st.CheckedAt) || got.ETag[etagReleases] != `W/"abc"` ||
		*got.Latest[Stable] != *st.Latest[Stable] || got.PakJSON.Tag != "v1.0.25" ||
		got.Notified[Stable] != "v1.0.25" {
		t.Fatalf("round trip lost data: %+v", got)
	}
}
