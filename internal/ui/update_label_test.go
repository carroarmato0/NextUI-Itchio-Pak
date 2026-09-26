package ui

import (
	"testing"

	"github.com/carroarmato0/nextui-itchio-pak/internal/inventory"
	"github.com/carroarmato0/nextui-itchio-pak/internal/roms"
)

func TestUpdateLabel(t *testing.T) {
	for _, tc := range []struct {
		files []inventory.UpstreamFile
		want  string
	}{
		{nil, ""},
		{[]inventory.UpstreamFile{{Filename: "Glory Hunters 2.1.zip"}}, "New file: Glory Hunters 2.1.zip"},
		{[]inventory.UpstreamFile{{Filename: "game.gb", Changed: true}}, "Updated: game.gb"},
		{[]inventory.UpstreamFile{{Filename: "a.gb"}, {Filename: "b.gb"}}, "New files: a.gb (+1 more)"},
		{[]inventory.UpstreamFile{{Filename: "a.gb", Changed: true}, {Filename: "b.gb"}}, "Updated: a.gb (+1 more)"},
	} {
		if got := updateLabel(tc.files); got != tc.want {
			t.Errorf("updateLabel(%v) = %q, want %q", tc.files, got, tc.want)
		}
	}
}

func TestUpdateBadge(t *testing.T) {
	pending := []inventory.UpstreamFile{
		{Filename: "Glory Hunters 2.1", UploadID: "15"},             // from the API: display name + ID
		{Filename: "Criss Cross Cove (GB Rom)"},                     // from the page: display name only
		{Filename: "game.gb", UploadID: "7", Changed: true},
	}
	for _, tc := range []struct {
		u    roms.Upload
		want string
	}{
		{roms.Upload{Filename: "Glory Hunters 2.1.zip", UploadID: "15"}, "NEW"}, // matched by ID, not name
		{roms.Upload{Filename: "Criss Cross Cove (GB Rom)"}, "NEW"},             // web flow: matched by name
		{roms.Upload{Filename: "game.gb", UploadID: "7"}, "UPDATED"},
		{roms.Upload{Filename: "Glory Hunters 2.0.zip", UploadID: "14"}, ""},
		{roms.Upload{Filename: "Glory Hunters 2.1", UploadID: "99"}, ""}, // IDs disagree: a different file
	} {
		if got := updateBadge(tc.u, pending); got != tc.want {
			t.Errorf("updateBadge(%+v) = %q, want %q", tc.u, got, tc.want)
		}
	}
}
