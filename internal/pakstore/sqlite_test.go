package pakstore

import (
	"encoding/binary"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

// varintBytes encodes v as a 9-byte SQLite varint, matching what varint (in
// sqlite.go) decodes: the first 8 bytes each carry 7 bits with the
// continuation bit set, and the 9th carries the low 8 bits verbatim. This is
// the only encoding these tests need: it can express every uint64, including
// values no real SQLite writer would ever produce, which is the point —
// these tests are standing in for a corrupt or adversarial database file.
func varintBytes(v uint64) []byte {
	var buf [9]byte
	top := v >> 8
	for i := 7; i >= 0; i-- {
		buf[i] = byte(top&0x7f) | 0x80
		top >>= 7
	}
	buf[8] = byte(v)
	return buf[:]
}

func TestVarintBytes_roundTrips(t *testing.T) {
	for _, v := range []uint64{0, 1, 127, 128, 1 << 62, 1 << 63, ^uint64(0)} {
		got, n := varint(varintBytes(v))
		if n != 9 || got != v {
			t.Fatalf("varint(varintBytes(%d)) = %d, %d bytes; want %d, 9 bytes", v, got, n, v)
		}
	}
}

// leafPage returns a minimal one-page SQLite file (page 1 is a table leaf
// b-tree page) containing a single cell whose bytes are exactly cell. Good
// enough to drive walkTable's cell-parsing directly, without needing a real
// sqlite3-authored database. It takes no *testing.T so it can be called from
// both TestXxx and FuzzXxx (whose seed-adding *testing.F is not a testing.TB).
func leafPage(pageSize int, cell []byte) ([]byte, error) {
	const cellOffset = 300
	if cellOffset+len(cell) > pageSize {
		return nil, fmt.Errorf("cell (%d bytes) does not fit in a %d-byte page", len(cell), pageSize)
	}
	data := make([]byte, pageSize)
	copy(data[0:16], "SQLite format 3\x00")
	binary.BigEndian.PutUint16(data[16:18], uint16(pageSize))
	// data[20] (page reserved space) stays 0.
	binary.BigEndian.PutUint32(data[56:60], 1) // UTF-8
	data[100] = 0x0D                           // table leaf page
	binary.BigEndian.PutUint16(data[103:105], 1)
	binary.BigEndian.PutUint16(data[108:110], uint16(cellOffset))
	copy(data[cellOffset:], cell)
	return data, nil
}

// TestWalkTable_hugePayloadSizeNoPanic is a regression test for a reachable
// panic: a table-leaf cell whose payload-size varint is astronomically
// large (here, exactly 2^63) makes int(size) come out negative on the old
// code (the sign bit lands in the wrong place), which slipped past the
// `int(size) > d.usable-35` overflow guard *because a negative int is never
// greater than a positive threshold* and then panicked slicing
// p[start:start+int(size)] with a negative-valued upper bound. The fixed
// code compares sizes in uint64 before any conversion to int.
func TestWalkTable_hugePayloadSizeNoPanic(t *testing.T) {
	cell := append(varintBytes(1<<63), varintBytes(1)...) // payload size, then rowid
	data, err := leafPage(512, cell)
	if err != nil {
		t.Fatalf("build fixture page: %v", err)
	}
	d, err := open(data)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	var sawPayload bool
	err = d.walkTable(1, func(payload []byte) error {
		sawPayload = true
		return nil
	})
	if err == nil {
		t.Fatal("walkTable accepted a cell with an impossible payload size")
	}
	if sawPayload {
		t.Fatal("walkTable called fn for a cell it should have rejected")
	}
	t.Logf("walkTable correctly rejected it: %v", err)
}

// TestDecodeRecord_hugeSerialTypeNoPanic is a regression test for a
// reachable panic: a record header declaring a serial type near 2^64 makes
// (t-12)/2 (or (t-13)/2) enormous; casting that to int before dividing, as
// the old code did, can wrap around to a negative size, which slipped past
// a `body+size > len(p)` bounds check written in int and then panicked
// slicing p[body:body+size] with a negative-valued upper bound. The fixed
// code computes and bounds-checks the size in uint64 first.
func TestDecodeRecord_hugeSerialTypeNoPanic(t *testing.T) {
	typeBytes := varintBytes(^uint64(0)) // max uint64: odd, so the t>=13 branch
	hdrLen := 1 + len(typeBytes)
	if hdrLen > 0x7f {
		t.Fatalf("test bug: header length %d does not fit a 1-byte varint", hdrLen)
	}
	payload := append([]byte{byte(hdrLen)}, typeBytes...)
	_, err := decodeRecord(payload)
	if err == nil {
		t.Fatal("decodeRecord accepted a record with an impossible serial type")
	}
	t.Logf("decodeRecord correctly rejected it: %v", err)
}

// FuzzLookup feeds Lookup arbitrary bytes as a database file, seeded from the
// committed fixtures (including the crafted regression cases above, so a
// `go test -fuzz` run without -seed still starts from real SQLite files and
// the two known-tricky shapes). It asserts nothing about the answer beyond
// "Lookup returns instead of panicking" — Result's zero value is always a
// technically-valid answer, so there is nothing else to check without
// re-implementing the reader.
func FuzzLookup(f *testing.F) {
	entries, err := os.ReadDir(fixtures)
	if err != nil {
		f.Fatalf("read fixtures dir: %v", err)
	}
	for _, e := range entries {
		if filepath.Ext(e.Name()) != ".db" {
			continue
		}
		data, err := os.ReadFile(filepath.Join(fixtures, e.Name()))
		if err != nil {
			f.Fatalf("read fixture %s: %v", e.Name(), err)
		}
		f.Add(data)
	}
	if hugeCellPage, err := leafPage(512, append(varintBytes(1<<63), varintBytes(1)...)); err == nil {
		f.Add(hugeCellPage)
	}

	f.Fuzz(func(t *testing.T, data []byte) {
		dir := t.TempDir()
		path := filepath.Join(dir, "pak-store.db")
		if err := os.WriteFile(path, data, 0644); err != nil {
			t.Fatalf("write candidate db: %v", err)
		}
		_ = Lookup(path) // must not panic; the result itself is not asserted
	})
}
