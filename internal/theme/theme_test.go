package theme

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoad_AllFields(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "minuisettings.txt")
	content := `color1=0xFF0000
color2=0x00FF00
color3=0x0000FF
color4=0x112233
color5=0xAABBCC
color6=0xDDEEFF
color7=0x010203
`
	if err := os.WriteFile(p, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	th := Load(p)
	if th.MainText != [3]uint8{0xFF, 0x00, 0x00} {
		t.Errorf("MainText: got %v", th.MainText)
	}
	if th.Accent != [3]uint8{0x00, 0xFF, 0x00} {
		t.Errorf("Accent: got %v", th.Accent)
	}
	if th.HeaderBG != [3]uint8{0x00, 0x00, 0xFF} {
		t.Errorf("HeaderBG: got %v", th.HeaderBG)
	}
	if th.ListText != [3]uint8{0x11, 0x22, 0x33} {
		t.Errorf("ListText: got %v", th.ListText)
	}
	if th.AccentText != [3]uint8{0xAA, 0xBB, 0xCC} {
		t.Errorf("AccentText: got %v", th.AccentText)
	}
	if th.HintText != [3]uint8{0xDD, 0xEE, 0xFF} {
		t.Errorf("HintText: got %v", th.HintText)
	}
	if th.Background != [3]uint8{0x01, 0x02, 0x03} {
		t.Errorf("Background: got %v", th.Background)
	}
}

func TestLoad_SubsetOfFields(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "minuisettings.txt")
	if err := os.WriteFile(p, []byte("color2=0x9B2257\n"), 0644); err != nil {
		t.Fatal(err)
	}
	th := Load(p)
	if th.Accent != [3]uint8{0x9B, 0x22, 0x57} {
		t.Errorf("Accent: got %v", th.Accent)
	}
	if th.Background != [3]uint8{0x14, 0x14, 0x14} {
		t.Errorf("Background default: got %v", th.Background)
	}
}

func TestLoad_MissingFile(t *testing.T) {
	th := Load("/nonexistent/path/minuisettings.txt")
	def := defaults()
	if th != def {
		t.Errorf("expected defaults for missing file, got %+v", th)
	}
}

func TestLoad_MalformedLine(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "minuisettings.txt")
	content := `color2=notahex
color7=0x141414
`
	if err := os.WriteFile(p, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	th := Load(p)
	if th.Accent != defaults().Accent {
		t.Errorf("Accent should be default after bad hex, got %v", th.Accent)
	}
	if th.Background != [3]uint8{0x14, 0x14, 0x14} {
		t.Errorf("Background: got %v", th.Background)
	}
}

func TestLoad_EmptyFile(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "minuisettings.txt")
	if err := os.WriteFile(p, []byte(""), 0644); err != nil {
		t.Fatal(err)
	}
	th := Load(p)
	def := defaults()
	if th != def {
		t.Errorf("expected defaults for empty file, got %+v", th)
	}
}
