//go:build !headless

package ui

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/carroarmato0/nextui-itchio-pak/internal/partfile"
)

// failingEntry yields some bytes and then an error, like a corrupt or
// truncated archive entry, or the card being pulled mid-extraction.
type failingEntry struct{ r io.Reader }

func (f *failingEntry) Read(p []byte) (int, error) {
	n, err := f.r.Read(p)
	if err == io.EOF {
		return n, errors.New("flate: corrupt input")
	}
	return n, err
}
func (f *failingEntry) Close() error { return nil }

func opener(rc io.ReadCloser) func() (io.ReadCloser, error) {
	return func() (io.ReadCloser, error) { return rc, nil }
}

// An update that fails partway through extraction must leave the installed
// ROM exactly as it was, and no partial file behind.
func TestExtractEntry_failureKeepsInstalledFile(t *testing.T) {
	dest := filepath.Join(t.TempDir(), "game.gb")
	if err := os.WriteFile(dest, []byte("WORKING ROM"), 0644); err != nil {
		t.Fatal(err)
	}
	err := extractEntry(opener(&failingEntry{bytes.NewReader([]byte("NEW PARTIAL DATA"))}), dest)
	if err == nil {
		t.Fatal("extractEntry succeeded on an entry that failed mid-copy")
	}
	if b, _ := os.ReadFile(dest); string(b) != "WORKING ROM" {
		t.Fatalf("installed ROM changed to %q", b)
	}
	if _, err := os.Stat(partfile.PathFor(dest)); !os.IsNotExist(err) {
		t.Fatal("partial file left behind")
	}
}

func TestExtractEntry_successReplacesFile(t *testing.T) {
	dest := filepath.Join(t.TempDir(), "game.gb")
	if err := os.WriteFile(dest, []byte("OLD ROM, LONGER THAN THE NEW ONE"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := extractEntry(opener(io.NopCloser(bytes.NewReader([]byte("NEW ROM")))), dest); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(dest); string(b) != "NEW ROM" {
		t.Fatalf("dest = %q, want the new ROM", b)
	}
	if _, err := os.Stat(partfile.PathFor(dest)); !os.IsNotExist(err) {
		t.Fatal("partial file left behind after a successful extraction")
	}
}

func TestExtractEntry_openFailureKeepsInstalledFile(t *testing.T) {
	dest := filepath.Join(t.TempDir(), "game.gb")
	os.WriteFile(dest, []byte("WORKING ROM"), 0644)
	open := func() (io.ReadCloser, error) { return nil, errors.New("zip: not a valid entry") }
	if err := extractEntry(open, dest); err == nil {
		t.Fatal("expected the open error")
	}
	if b, _ := os.ReadFile(dest); string(b) != "WORKING ROM" {
		t.Fatalf("installed ROM changed to %q", b)
	}
	if _, err := os.Stat(partfile.PathFor(dest)); !os.IsNotExist(err) {
		t.Fatal("partial file left behind")
	}
}
