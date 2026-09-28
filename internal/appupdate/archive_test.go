package appupdate

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/carroarmato0/nextui-itchio-pak/internal/partfile"
)

func buildZip(t *testing.T, names ...string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, n := range names {
		w, err := zw.Create(n)
		if err != nil {
			t.Fatal(err)
		}
		w.Write([]byte("#!/bin/sh\n"))
	}
	zw.Close()
	return buf.Bytes()
}

func goodMuxapp(t *testing.T) []byte {
	return buildZip(t, "Itch-io/", "Itch-io/mux_launch.sh", "Itch-io/itchio", "Itch-io/assets/font.ttf")
}

func releaseFor(url string, body []byte, tag string) Release {
	sum := sha256.Sum256(body)
	return Release{Tag: tag, Asset: url, Digest: "sha256:" + hex.EncodeToString(sum[:]), Size: int64(len(body))}
}

func archiveSetup(t *testing.T) string {
	t.Helper()
	partfile.SetJournal(filepath.Join(t.TempDir(), "partials.json"))
	t.Cleanup(func() { partfile.SetJournal("") })
	return filepath.Join(t.TempDir(), "ARCHIVE")
}

func serve(t *testing.T, body []byte) *httptest.Server {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write(body) }))
	t.Cleanup(srv.Close)
	return srv
}

func assertNoPart(t *testing.T, dir, tag string) {
	t.Helper()
	if _, err := os.Stat(partfile.PathFor(filepath.Join(dir, AssetName(tag)))); !os.IsNotExist(err) {
		t.Fatal("the partial file must not survive")
	}
	if _, err := os.Stat(filepath.Join(dir, AssetName(tag))); !os.IsNotExist(err) {
		t.Fatal("nothing may be saved under the final name")
	}
}

func TestSaveToArchive_ok_andCleansOthers(t *testing.T) {
	dir := archiveSetup(t)
	os.MkdirAll(dir, 0755)
	for _, n := range []string{"Itch-io.muOS.v1.0.25.muxapp", "Itch-io.muOS.v1.2.0-rc1.muxapp", "Other.muOS.v1.0.0.muxapp", "Itch-io.muOS.notes.txt"} {
		os.WriteFile(filepath.Join(dir, n), []byte("x"), 0644)
	}
	body := goodMuxapp(t)
	srv := serve(t, body)
	var last int64
	path, err := SaveToArchive(context.Background(), srv.Client(), "ua", releaseFor(srv.URL, body, "v1.1.1"), dir,
		func(done, total int64) { last = done })
	if err != nil {
		t.Fatal(err)
	}
	if path != filepath.Join(dir, "Itch-io.muOS.v1.1.1.muxapp") || last != int64(len(body)) {
		t.Fatalf("path=%q progress=%d", path, last)
	}
	entries, _ := os.ReadDir(dir)
	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
	}
	want := map[string]bool{"Itch-io.muOS.v1.1.1.muxapp": true, "Other.muOS.v1.0.0.muxapp": true, "Itch-io.muOS.notes.txt": true}
	if len(names) != len(want) {
		t.Fatalf("ARCHIVE holds %v; want exactly %v (the newer rc must go too)", names, want)
	}
	for _, n := range names {
		if !want[n] {
			t.Fatalf("unexpected %s left in ARCHIVE", n)
		}
	}
}

func TestSaveToArchive_digestMismatch(t *testing.T) {
	dir := archiveSetup(t)
	body := goodMuxapp(t)
	srv := serve(t, body)
	rel := releaseFor(srv.URL, body, "v1.1.1")
	rel.Digest = "sha256:" + hex.EncodeToString(make([]byte, 32))
	_, err := SaveToArchive(context.Background(), srv.Client(), "ua", rel, dir, nil)
	if !errors.Is(err, ErrIntegrity) {
		t.Fatalf("err = %v, want ErrIntegrity", err)
	}
	assertNoPart(t, dir, "v1.1.1")
}

func TestSaveToArchive_lengthMismatch(t *testing.T) {
	dir := archiveSetup(t)
	body := goodMuxapp(t)
	srv := serve(t, body)
	rel := releaseFor(srv.URL, body, "v1.1.1")
	rel.Size++
	if _, err := SaveToArchive(context.Background(), srv.Client(), "ua", rel, dir, nil); !errors.Is(err, ErrIntegrity) {
		t.Fatalf("err = %v, want ErrIntegrity", err)
	}
	assertNoPart(t, dir, "v1.1.1")
}

func TestSaveToArchive_unsafeZip(t *testing.T) {
	for name, entries := range map[string][]string{
		"outside the app folder": {"Itch-io/mux_launch.sh", "evil.sh"},
		"dot-dot":                {"Itch-io/mux_launch.sh", "Itch-io/../../etc/x"},
		"absolute":               {"Itch-io/mux_launch.sh", "/etc/x"},
		"backslash":              {"Itch-io/mux_launch.sh", "Itch-io\\x"},
		"no launcher":            {"Itch-io/itchio"},
	} {
		t.Run(name, func(t *testing.T) {
			dir := archiveSetup(t)
			body := buildZip(t, entries...)
			srv := serve(t, body)
			if _, err := SaveToArchive(context.Background(), srv.Client(), "ua", releaseFor(srv.URL, body, "v1.1.1"), dir, nil); err == nil {
				t.Fatal("want the zip rejected")
			}
			assertNoPart(t, dir, "v1.1.1")
		})
	}
}

func TestSaveToArchive_cancel(t *testing.T) {
	dir := archiveSetup(t)
	started := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", "1000000")
		w.Write(make([]byte, 1000))
		w.(http.Flusher).Flush()
		close(started)
		<-r.Context().Done()
	}))
	defer srv.Close()
	ctx, cancel := context.WithCancel(context.Background())
	go func() { <-started; cancel() }()
	_, err := SaveToArchive(ctx, srv.Client(), "ua", Release{Tag: "v1.1.1", Asset: srv.URL, Digest: "sha256:00", Size: 1000000}, dir, nil)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
	assertNoPart(t, dir, "v1.1.1")
}

func TestSaveToArchive_idleTimeout(t *testing.T) {
	old := idleTimeout
	idleTimeout = 100 * time.Millisecond
	t.Cleanup(func() { idleTimeout = old })
	dir := archiveSetup(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done() // never sends headers
	}))
	defer srv.Close()
	start := time.Now()
	_, err := SaveToArchive(context.Background(), srv.Client(), "ua", Release{Tag: "v1.1.1", Asset: srv.URL, Digest: "sha256:00", Size: 10}, dir, nil)
	if err == nil || time.Since(start) > 5*time.Second {
		t.Fatalf("err = %v after %v; want an idle timeout covering the header wait", err, time.Since(start))
	}
	assertNoPart(t, dir, "v1.1.1")
}

func TestSaveToArchive_notDownloadable(t *testing.T) {
	dir := archiveSetup(t)
	for _, rel := range []Release{{Tag: "v1.1.1"}, {Tag: "v1.1.1", Asset: "http://x", Size: 1}} {
		if _, err := SaveToArchive(context.Background(), http.DefaultClient, "ua", rel, dir, nil); err == nil {
			t.Fatalf("%+v: want an error without an asset and digest", rel)
		}
	}
}

func TestSweepRemovesArchiveLeftover(t *testing.T) {
	dir := archiveSetup(t)
	os.MkdirAll(dir, 0755)
	left := partfile.PathFor(filepath.Join(dir, AssetName("v1.1.0")))
	os.WriteFile(left, []byte("half a download"), 0644)
	partfile.Sweep([]string{dir})
	if _, err := os.Stat(left); !os.IsNotExist(err) {
		t.Fatalf("%s must be swept", left)
	}
}

// Run by hand against a genuine released .muxapp; skipped otherwise.
func TestValidateMuxapp_realRelease(t *testing.T) {
	path := os.Getenv("MUXAPP_REAL")
	if path == "" {
		t.Skip("MUXAPP_REAL not set")
	}
	if err := ValidateMuxapp(path); err != nil {
		t.Fatalf("real release %s: %v, want accepted", path, err)
	}
}
