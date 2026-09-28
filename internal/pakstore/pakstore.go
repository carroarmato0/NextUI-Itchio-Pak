// Package pakstore tells whether the NextUI Pak Store manages this install,
// by reading its SQLite database read-only.
package pakstore

import (
	"errors"
	"fmt"
	"io/fs"
	"os"

	"github.com/carroarmato0/nextui-itchio-pak/internal/logger"
)

// Status is what the database says about Itch-io.
type Status int

const (
	// NotInstalled: no Store database for this platform, or no row for Itch-io.
	NotInstalled Status = iota
	// Managed: the Store has a row for Itch-io; Version is what it records.
	Managed
	// Unknown: the file exists but cannot be read with confidence. Callers
	// treat it as Managed: guessing wrong that way only points the user at the
	// Store; guessing wrong the other way would go around it.
	Unknown
)

func (s Status) String() string {
	switch s {
	case Managed:
		return "managed"
	case Unknown:
		return "unknown"
	default:
		return "not-installed"
	}
}

const (
	itchioPakID = "3JM2zY4UKh"
	itchioName  = "Itch-io"
)

// Result is Lookup's answer. Reason explains it in the log.
type Result struct {
	Status  Status
	Version string
	Reason  string
}

func (r Result) String() string {
	if r.Status == Managed {
		return "managed(" + r.Version + ")"
	}
	return r.Status.String() + "(" + r.Reason + ")"
}

// Lookup reads dbPath. "" means the firmware has no Pak Store.
func Lookup(dbPath string) Result {
	res := lookup(dbPath)
	logger.Info("pakstore: %s → %s", dbPath, res)
	return res
}

func lookup(dbPath string) (result Result) {
	// Safety net: the reader in sqlite.go is hand-written against an
	// adversarial input (a real device's database, but still a file this
	// process does not control). If some corner of it panics anyway, treat
	// that exactly like any other unreadable database — Unknown, which
	// callers treat as Managed — rather than taking the caller down.
	defer func() {
		if r := recover(); r != nil {
			logger.Error("pakstore: panic reading %s: %v", dbPath, r)
			result = Result{Status: Unknown, Reason: fmt.Sprintf("parser panic: %v", r)}
		}
	}()

	if dbPath == "" {
		return Result{Status: NotInstalled, Reason: "no Pak Store on this firmware"}
	}
	data, err := os.ReadFile(dbPath)
	if errors.Is(err, fs.ErrNotExist) {
		return Result{Status: NotInstalled, Reason: "no database"}
	}
	if err != nil {
		return Result{Status: Unknown, Reason: "read: " + err.Error()}
	}
	// Committed pages may still be sitting in a write-ahead log or a hot
	// rollback journal, not yet in the main file: an interrupted write. Both
	// are named after the main file, e.g. "pak-store.db-wal".
	if res, unknown := sidecarIsUnknown(dbPath+"-wal", "write-ahead log"); unknown {
		return res
	}
	if res, unknown := sidecarIsUnknown(dbPath+"-journal", "rollback journal"); unknown {
		return res
	}
	d, err := open(data)
	if err != nil {
		return Result{Status: Unknown, Reason: err.Error()}
	}
	logger.Debug("pakstore: page size %d, %d pages", d.pageSize, d.pages)
	version, found, err := findItchio(d)
	switch {
	case err != nil:
		return Result{Status: Unknown, Reason: err.Error()}
	case !found:
		return Result{Status: NotInstalled, Reason: "no row for Itch-io"}
	}
	return Result{Status: Managed, Version: version}
}

// sidecarIsUnknown reports whether a WAL or rollback-journal file next to the
// database makes it Unknown: a non-empty file means the main file may not
// reflect everything committed, and a Stat error other than "does not exist"
// means its state can't be trusted either way. An absent or empty sidecar
// changes nothing (unknown is false, and res is the zero Result).
func sidecarIsUnknown(path, label string) (res Result, unknown bool) {
	fi, err := os.Stat(path)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return Result{}, false
	case err != nil:
		return Result{Status: Unknown, Reason: label + ": " + err.Error()}, true
	case fi.Size() > 0:
		return Result{Status: Unknown, Reason: label + " is not empty"}, true
	default:
		return Result{}, false
	}
}

func findItchio(d *db) (string, bool, error) {
	root, sql, err := findTable(d, "installed_paks")
	if err != nil {
		return "", false, err
	}
	cols := columnNames(sql)
	idx := map[string]int{}
	for i, c := range cols {
		idx[c] = i
	}
	iName, ok1 := idx["name"]
	iID, ok2 := idx["pak_id"]
	iVer, ok3 := idx["version"]
	if !ok1 || !ok2 || !ok3 {
		return "", false, fmt.Errorf("unexpected columns %v", cols)
	}
	var byID, byName string
	var haveID, haveName bool
	err = d.walkTable(root, func(payload []byte) error {
		row, err := decodeRecord(payload)
		if err != nil {
			return err
		}
		if len(row) <= iVer || len(row) <= iName || len(row) <= iID {
			return errors.New("row has fewer columns than the schema")
		}
		switch {
		case row[iID].s == itchioPakID:
			byID, haveID = row[iVer].s, true
		case row[iName].s == itchioName:
			byName, haveName = row[iVer].s, true
		}
		return nil
	})
	if err != nil {
		return "", false, err
	}
	if haveID {
		return byID, true, nil
	}
	return byName, haveName, nil
}

// findTable looks name up in sqlite_schema, the table rooted at page 1.
func findTable(d *db, name string) (root int, sql string, err error) {
	err = d.walkTable(1, func(payload []byte) error {
		row, err := decodeRecord(payload)
		if err != nil {
			return err
		}
		// type, name, tbl_name, rootpage, sql
		if len(row) >= 5 && row[0].s == "table" && row[1].s == name && row[3].isInt {
			root, sql = int(row[3].i), row[4].s
		}
		return nil
	})
	if err == nil && root == 0 {
		err = errNoTable
	}
	return root, sql, err
}
