package appupdate

import "path/filepath"

// The update is staged, and the replaced version kept, beside the live pak
// folder on the same card, so the swap is two renames. A leading dot hides
// them from NextUI's menus (hide() in workspace/all/common/utils.c).
func sibling(pakDir, suffix string) string {
	return filepath.Join(filepath.Dir(pakDir), "."+filepath.Base(pakDir)+suffix)
}

func StagedDir(pakDir string) string { return sibling(pakDir, ".staged") }
func StagedZip(pakDir string) string { return sibling(pakDir, ".staged.zip") }
func PrevDir(pakDir string) string   { return sibling(pakDir, ".prev") }
func FailedDir(pakDir string) string { return sibling(pakDir, ".failed") }

// completeMarker is written last into the staged folder: without it the
// folder is half-made and is never applied.
const completeMarker = ".complete"

// Files in the data dir ($HOME on NextUI). launch.sh reads the first two by
// these exact names.
const (
	pendingFile     = "update_pending.json"
	failedFile      = "update_failed.json"
	orphanFile      = "update_pending.orphan.json"
	launcherLogFile = "update_launcher.log"
)
