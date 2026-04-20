package roms

import (
	"path/filepath"
	"strings"
)

type Upload struct {
	Filename      string
	URL           string
	UploadID      string // itch.io upload ID (API-based paid download)
	DownloadKeyID string // itch.io download key ID (API-based paid download)
}

func ScoreUpload(filename string) int {
	switch strings.ToLower(filepath.Ext(filename)) {
	case ".gbc":
		return 2
	case ".gb":
		return 1
	default:
		return 0
	}
}

func DestinationDir(ext string) string {
	switch strings.ToLower(ext) {
	case ".gbc":
		return "/mnt/SDCARD/Roms/Game Boy Color (GBC)/"
	case ".gb":
		return "/mnt/SDCARD/Roms/Game Boy (GB)/"
	default:
		return ""
	}
}

func SelectBest(uploads []Upload) *Upload {
	var best *Upload
	bestScore := 0
	for i := range uploads {
		s := ScoreUpload(uploads[i].Filename)
		if s > bestScore {
			bestScore = s
			best = &uploads[i]
		}
	}
	return best
}
