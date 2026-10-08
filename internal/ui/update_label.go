package ui

import (
	"strconv"

	"github.com/carroarmato0/nextui-itchio-pak/internal/inventory"
	"github.com/carroarmato0/nextui-itchio-pak/internal/roms"
)

// updateLabel names what an update consists of, for the game page: the first
// file and how many more. A changed file leads, since it replaces one the user
// has; a new file is one they do not have yet.
func updateLabel(files []inventory.UpstreamFile) string {
	if len(files) == 0 {
		return ""
	}
	first := files[0]
	for _, f := range files {
		if f.Changed {
			first = f
			break
		}
	}
	var label string
	switch {
	case first.Changed:
		label = "Updated: " + first.Filename
	case len(files) > 1:
		label = "New files: " + first.Filename
	default:
		label = "New file: " + first.Filename
	}
	if len(files) > 1 {
		label += " (+" + strconv.Itoa(len(files)-1) + " more)"
	}
	return label
}

// updateBadge marks a file in the picker that is part of an update: "NEW"
// for a new upload, "UPDATED" for one whose content changed, "" otherwise.
// A pending file is matched by upload ID when both sides have one; else by
// name, since the web flow's picker names files the way the page shows them.
func updateBadge(u roms.Upload, pending []inventory.UpstreamFile) string {
	for _, p := range pending {
		match := p.Filename == u.Filename
		if p.UploadID != "" && u.UploadID != "" {
			match = p.UploadID == u.UploadID
		}
		if !match {
			continue
		}
		if p.Changed {
			return "UPDATED"
		}
		return "NEW"
	}
	return ""
}
