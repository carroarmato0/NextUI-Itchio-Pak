package pakstore

import (
	"os"
	"path/filepath"
	"testing"
)

const fixtures = "../../testdata/pakstore/"

func TestLookup(t *testing.T) {
	cases := []struct {
		file    string
		status  Status
		version string
	}{
		{"single.db", Managed, "v1.0.23"},
		{"byname.db", Managed, "v1.1.0-rc2"},
		{"multipage.db", Managed, "v1.0.25"},
		{"empty.db", NotInstalled, ""},
		{"notable.db", Unknown, ""},
		{"overflow.db", Unknown, ""},
		{"truncated.db", Unknown, ""},
		{"badheader.db", Unknown, ""},
		{"does-not-exist.db", NotInstalled, ""},
	}
	for _, c := range cases {
		t.Run(c.file, func(t *testing.T) {
			got := Lookup(fixtures + c.file)
			if got.Status != c.status || got.Version != c.version {
				t.Fatalf("Lookup = %+v, want %s %q", got, c.status, c.version)
			}
			if got.Status == Unknown && got.Reason == "" {
				t.Fatal("Unknown must say why, for the log")
			}
		})
	}
}

func TestLookup_emptyPathIsNotInstalled(t *testing.T) {
	if got := Lookup(""); got.Status != NotInstalled {
		t.Fatalf("Lookup(\"\") = %+v", got)
	}
}

func TestLookup_nonEmptyWALIsUnknown(t *testing.T) {
	dir := t.TempDir()
	data, err := os.ReadFile(fixtures + "single.db")
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	db := filepath.Join(dir, "pak-store.db")
	if err := os.WriteFile(db, data, 0644); err != nil {
		t.Fatalf("write db: %v", err)
	}
	if err := os.WriteFile(db+"-wal", []byte("uncommitted pages"), 0644); err != nil {
		t.Fatalf("write -wal: %v", err)
	}
	if got := Lookup(db); got.Status != Unknown {
		t.Fatalf("with a non-empty -wal: %+v, want Unknown", got)
	}
	if err := os.WriteFile(db+"-wal", nil, 0644); err != nil {
		t.Fatalf("empty -wal: %v", err)
	}
	if got := Lookup(db); got.Status != Managed {
		t.Fatalf("with an empty -wal: %+v, want Managed", got)
	}
}

func TestLookup_nonEmptyJournalIsUnknown(t *testing.T) {
	dir := t.TempDir()
	data, err := os.ReadFile(fixtures + "single.db")
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	db := filepath.Join(dir, "pak-store.db")
	if err := os.WriteFile(db, data, 0644); err != nil {
		t.Fatalf("write db: %v", err)
	}
	if err := os.WriteFile(db+"-journal", []byte("hot rollback journal"), 0644); err != nil {
		t.Fatalf("write -journal: %v", err)
	}
	if got := Lookup(db); got.Status != Unknown {
		t.Fatalf("with a non-empty -journal: %+v, want Unknown", got)
	}
	if err := os.WriteFile(db+"-journal", nil, 0644); err != nil {
		t.Fatalf("empty -journal: %v", err)
	}
	if got := Lookup(db); got.Status != Managed {
		t.Fatalf("with an empty -journal: %+v, want Managed", got)
	}
}

func TestColumnNames(t *testing.T) {
	sql := "CREATE TABLE installed_paks\n(\n name text not null,\n pak_id text,\n version text not null,\n unique (name)\n)"
	got := columnNames(sql)
	want := []string{"name", "pak_id", "version"}
	if len(got) != len(want) {
		t.Fatalf("columnNames = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("columnNames = %v, want %v", got, want)
		}
	}
}

// Run by hand against a database pulled from a device; skipped otherwise.
func TestLookup_realDevice(t *testing.T) {
	path := os.Getenv("PAKSTORE_REAL_DB")
	if path == "" {
		t.Skip("PAKSTORE_REAL_DB not set")
	}
	got := Lookup(path)
	t.Logf("real device: %+v", got)
	if got.Status != Managed {
		t.Fatalf("real Store database: %+v, want Managed", got)
	}
}
