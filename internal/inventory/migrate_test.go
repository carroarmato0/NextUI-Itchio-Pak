package inventory_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/carroarmato0/nextui-itchio-pak/internal/inventory"
)

func TestReadMigrateFormats_Defaults(t *testing.T) {
	f := inventory.ReadMigrateFormats("/nonexistent/path/minuisettings.txt")
	if f.SaveFormat != 0 || f.StateFormat != 0 || f.UseExtractedFileName {
		t.Errorf("defaults: got %+v", f)
	}
}

func TestReadMigrateFormats_AllFields(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "minuisettings.txt")
	content := "saveFormat=2\nstateFormat=3\nuseExtractedFileName=1\n"
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	f := inventory.ReadMigrateFormats(path)
	if f.SaveFormat != 2 {
		t.Errorf("SaveFormat = %d, want 2", f.SaveFormat)
	}
	if f.StateFormat != 3 {
		t.Errorf("StateFormat = %d, want 3", f.StateFormat)
	}
	if !f.UseExtractedFileName {
		t.Error("UseExtractedFileName should be true")
	}
}

func TestReadMigrateFormats_PartialFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "minuisettings.txt")
	if err := os.WriteFile(path, []byte("saveFormat=1\n"), 0644); err != nil {
		t.Fatal(err)
	}
	f := inventory.ReadMigrateFormats(path)
	if f.SaveFormat != 1 {
		t.Errorf("SaveFormat = %d, want 1", f.SaveFormat)
	}
	if f.StateFormat != 0 {
		t.Errorf("StateFormat = %d, want 0 (default)", f.StateFormat)
	}
}
