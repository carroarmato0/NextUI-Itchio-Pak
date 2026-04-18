package roms

type Upload struct {
	Filename string
	URL      string
}

func ScoreUpload(filename string) int     { return 0 }
func DestinationDir(ext string) string    { return "" }
func SelectBest(uploads []Upload) *Upload { return nil }
