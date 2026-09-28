package pakstore

import (
	"encoding/binary"
	"errors"
	"fmt"
	"strings"
)

// A reader for exactly as much of the SQLite file format as Lookup needs:
// the header, table b-trees (interior and leaf pages) and records. Overflow
// pages are not followed: a row that needs them makes the database Unknown.
// Linking a SQLite library was rejected: several MB of binary to read one
// 12 KB file. Format reference: https://www.sqlite.org/fileformat.html

var (
	errOverflow = errors.New("row uses overflow pages")
	errNoTable  = errors.New("no installed_paks table")
)

type db struct {
	data     []byte
	pageSize int
	usable   int
	pages    int
}

func open(data []byte) (*db, error) {
	if len(data) < 100 || string(data[:16]) != "SQLite format 3\x00" {
		return nil, errors.New("bad header")
	}
	ps := int(binary.BigEndian.Uint16(data[16:18]))
	if ps == 1 {
		ps = 65536
	}
	if ps < 512 || ps&(ps-1) != 0 {
		return nil, fmt.Errorf("bad page size %d", ps)
	}
	if len(data)%ps != 0 {
		return nil, fmt.Errorf("truncated: %d bytes is not a whole number of %d-byte pages", len(data), ps)
	}
	usable := ps - int(data[20])
	if usable < 480 {
		return nil, fmt.Errorf("usable page size %d too small", usable)
	}
	if enc := binary.BigEndian.Uint32(data[56:60]); enc != 1 {
		return nil, fmt.Errorf("text encoding %d is not UTF-8", enc)
	}
	return &db{data: data, pageSize: ps, usable: usable, pages: len(data) / ps}, nil
}

// page returns page n (1-based) and the offset of its b-tree header, which is
// 100 on page 1 because the file header comes first.
func (d *db) page(n int) ([]byte, int, error) {
	if n < 1 || n > d.pages {
		return nil, 0, fmt.Errorf("page %d out of range 1..%d", n, d.pages)
	}
	p := d.data[(n-1)*d.pageSize : n*d.pageSize]
	if n == 1 {
		return p, 100, nil
	}
	return p, 0, nil
}

// walkTable calls fn with the payload of every row in the table b-tree rooted
// at root.
func (d *db) walkTable(root int, fn func(payload []byte) error) error {
	visited := map[int]bool{}
	var walk func(n, depth int) error
	walk = func(n, depth int) error {
		if depth > 20 || visited[n] {
			return fmt.Errorf("b-tree loop at page %d", n)
		}
		visited[n] = true
		p, h, err := d.page(n)
		if err != nil {
			return err
		}
		if h+12 > len(p) {
			return fmt.Errorf("page %d: short header", n)
		}
		typ := p[h]
		count := int(binary.BigEndian.Uint16(p[h+3:]))
		hdr := 8
		if typ == 0x05 {
			hdr = 12
		}
		if h+hdr+2*count > len(p) {
			return fmt.Errorf("page %d: cell pointers overrun", n)
		}
		for i := 0; i < count; i++ {
			off := int(binary.BigEndian.Uint16(p[h+hdr+2*i:]))
			if off >= len(p) {
				return fmt.Errorf("page %d: cell %d out of range", n, i)
			}
			switch typ {
			case 0x0D: // table leaf: payload size, rowid, payload
				size, k1 := varint(p[off:])
				_, k2 := varint(p[off+k1:])
				if k1 == 0 || k2 == 0 {
					return fmt.Errorf("page %d: bad cell %d", n, i)
				}
				if int(size) > d.usable-35 {
					return errOverflow
				}
				start := off + k1 + k2
				if start+int(size) > len(p) {
					return fmt.Errorf("page %d: cell %d overruns the page", n, i)
				}
				if err := fn(p[start : start+int(size)]); err != nil {
					return err
				}
			case 0x05: // table interior: left child, rowid
				if off+4 > len(p) {
					return fmt.Errorf("page %d: bad cell %d", n, i)
				}
				if err := walk(int(binary.BigEndian.Uint32(p[off:])), depth+1); err != nil {
					return err
				}
			default:
				return fmt.Errorf("page %d: type 0x%02x is not a table page", n, typ)
			}
		}
		if typ == 0x05 {
			return walk(int(binary.BigEndian.Uint32(p[h+8:])), depth+1)
		}
		return nil
	}
	return walk(root, 0)
}

// varint decodes a SQLite varint. n is 0 when b ends first.
func varint(b []byte) (v uint64, n int) {
	for i := 0; i < 9 && i < len(b); i++ {
		if i == 8 {
			return v<<8 | uint64(b[i]), 9
		}
		v = v<<7 | uint64(b[i]&0x7f)
		if b[i]&0x80 == 0 {
			return v, i + 1
		}
	}
	return 0, 0
}

// value is one decoded column. Only what Lookup reads is kept.
type value struct {
	isNull bool
	isInt  bool
	i      int64
	s      string // text columns
}

func decodeRecord(p []byte) ([]value, error) {
	hdrLen, n := varint(p)
	if n == 0 || int(hdrLen) > len(p) || int(hdrLen) < n {
		return nil, errors.New("bad record header")
	}
	var types []uint64
	for pos := n; pos < int(hdrLen); {
		t, k := varint(p[pos:hdrLen])
		if k == 0 {
			return nil, errors.New("bad serial type")
		}
		types = append(types, t)
		pos += k
	}
	body := int(hdrLen)
	intSize := [...]int{0, 1, 2, 3, 4, 6, 8}
	out := make([]value, 0, len(types))
	for _, t := range types {
		var size int
		v := value{}
		switch {
		case t == 0:
			v.isNull = true
		case t >= 1 && t <= 6:
			size = intSize[t]
			v.isInt = true
		case t == 7:
			size = 8
		case t == 8, t == 9:
			v.isInt, v.i = true, int64(t-8)
		case t >= 12 && t%2 == 0:
			size = int(t-12) / 2
		case t >= 13:
			size = int(t-13) / 2
		default:
			return nil, fmt.Errorf("reserved serial type %d", t)
		}
		if body+size > len(p) {
			return nil, errors.New("record overruns its payload")
		}
		field := p[body : body+size]
		switch {
		case v.isInt && size > 0:
			var x int64
			for _, c := range field {
				x = x<<8 | int64(c)
			}
			shift := 64 - 8*uint(size) // sign-extend
			v.i = x << shift >> shift
		case t >= 13 && t%2 == 1:
			v.s = string(field)
		}
		out = append(out, v)
		body += size
	}
	return out, nil
}

// columnNames reads the column names from a CREATE TABLE statement, skipping
// table constraints. Enough for the Store's plain schema.
func columnNames(sql string) []string {
	lp, rp := strings.Index(sql, "("), strings.LastIndex(sql, ")")
	if lp < 0 || rp <= lp {
		return nil
	}
	var names []string
	depth, start := 0, lp+1
	body := sql[:rp]
	for i := lp + 1; i <= len(body); i++ {
		if i < len(body) {
			switch body[i] {
			case '(':
				depth++
				continue
			case ')':
				depth--
				continue
			case ',':
				if depth > 0 {
					continue
				}
			default:
				continue
			}
		}
		fields := strings.Fields(body[start:i])
		start = i + 1
		if len(fields) == 0 {
			continue
		}
		name := strings.Trim(fields[0], "\"`[]")
		switch strings.ToLower(name) {
		case "unique", "primary", "constraint", "check", "foreign":
			continue
		}
		names = append(names, name)
	}
	return names
}
