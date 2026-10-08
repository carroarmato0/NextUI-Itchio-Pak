//go:build !linux

package appupdate

// freeBytes is unknown off Linux; the download then relies on write errors.
func freeBytes(dir string) (int64, bool) { return 0, false }
