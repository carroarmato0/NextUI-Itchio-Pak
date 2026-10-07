package appupdate

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/carroarmato0/nextui-itchio-pak/internal/logger"
)

// Pending is update_pending.json: an update that has been swapped in and
// has not yet proved that it starts. launch.sh rolls back while it exists.
type Pending struct {
	From string    `json:"from"`
	To   string    `json:"to"`
	At   time.Time `json:"at"`
}

// rename is os.Rename; tests replace it to fail the second rename.
var rename = os.Rename

// StagedReady reports the tag of a complete staged update.
func StagedReady(pakDir string) (string, bool) {
	b, err := os.ReadFile(filepath.Join(StagedDir(pakDir), completeMarker))
	if err != nil {
		return "", false
	}
	tag := strings.TrimSpace(string(b))
	return tag, tag != ""
}

// DiscardIncompleteStaged removes a staged folder that never got its
// .complete marker (a crash or power cut while staging).
func DiscardIncompleteStaged(pakDir string) {
	dir := StagedDir(pakDir)
	if _, err := os.Stat(dir); err != nil {
		return
	}
	if _, ok := StagedReady(pakDir); ok {
		return
	}
	if err := os.RemoveAll(dir); err != nil {
		logger.Warn("appupdate: remove incomplete %s: %v", dir, err)
		return
	}
	logger.Info("appupdate: removed incomplete staged update %s", dir)
}

// SwapStaged puts the staged update in place of the live pak folder. The
// pending file is written first, so the new version is never in place
// without rollback protection.
func SwapStaged(pakDir, dataDir, from string) error {
	to, ok := StagedReady(pakDir)
	if !ok {
		return errors.New("no complete staged update")
	}
	prev, staged := PrevDir(pakDir), StagedDir(pakDir)
	if err := os.RemoveAll(prev); err != nil {
		logger.Error("appupdate: apply: remove old %s: %v", prev, err)
		return err
	}
	pendingPath := filepath.Join(dataDir, pendingFile)
	data, _ := json.Marshal(Pending{From: from, To: to, At: time.Now().UTC()})
	if err := os.WriteFile(pendingPath, data, 0644); err != nil {
		logger.Error("appupdate: apply: write %s: %v", pendingPath, err)
		return err
	}
	logger.Info("appupdate: apply: rename %s → %s", pakDir, prev)
	if err := rename(pakDir, prev); err != nil {
		logger.Error("appupdate: apply: rename %s: %v", pakDir, err)
		_ = os.Remove(pendingPath)
		return err
	}
	logger.Info("appupdate: apply: rename %s → %s", staged, pakDir)
	if err := rename(staged, pakDir); err != nil {
		logger.Error("appupdate: apply: rename %s: %v; restoring %s", staged, err, prev)
		if rerr := rename(prev, pakDir); rerr != nil {
			logger.Error("appupdate: apply: restore %s failed: %v", pakDir, rerr)
		}
		_ = os.Remove(pendingPath)
		return err
	}
	logger.Info("appupdate: apply: %s → %s in place, pending confirmation", from, to)
	return nil
}

// ExecLauncher replaces this process with the pak's launch.sh. Same PID and
// environment, so NextUI sees the same pak still running. Returns only on
// failure.
func ExecLauncher(pakDir string) error {
	launcher := filepath.Join(pakDir, "launch.sh")
	logger.Info("appupdate: exec %s", launcher)
	err := syscall.Exec("/bin/sh", []string{"/bin/sh", launcher}, os.Environ())
	logger.Error("appupdate: exec %s: %v", launcher, err)
	return err
}

// ConfirmStarted is called once the first frame is up and input has been
// read. It clears the pending file and removes the kept versions in the
// background; the returned WaitGroup is for tests.
func ConfirmStarted(pakDir, dataDir string) *sync.WaitGroup {
	var wg sync.WaitGroup
	path := filepath.Join(dataDir, pendingFile)
	b, err := os.ReadFile(path)
	if err == nil {
		var p Pending
		_ = json.Unmarshal(b, &p)
		if err := os.Remove(path); err != nil {
			logger.Error("appupdate: confirm: remove %s: %v", path, err)
		} else {
			logger.Info("update: %s → %s confirmed", p.From, p.To)
		}
	}
	if pakDir == "" {
		return &wg
	}
	wg.Add(1)
	go func() {
		defer wg.Done()
		for _, d := range []string{PrevDir(pakDir), FailedDir(pakDir)} {
			if _, err := os.Stat(d); err != nil {
				continue
			}
			if err := os.RemoveAll(d); err != nil {
				logger.Warn("appupdate: remove %s: %v", d, err)
				continue
			}
			logger.Info("appupdate: removed %s", d)
		}
	}()
	return &wg
}

// TakeFailed reads and deletes update_failed.json, written by launch.sh
// when it rolled an update back.
func TakeFailed(dataDir string) (*Pending, bool) {
	path := filepath.Join(dataDir, failedFile)
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, false
	}
	_ = os.Remove(path)
	var p Pending
	if err := json.Unmarshal(b, &p); err != nil || p.To == "" {
		logger.Warn("appupdate: %s unreadable: %v", path, err)
		return nil, false
	}
	logger.Warn("update: %s did not start, rolled back to %s", p.To, p.From)
	return &p, true
}

// TakeLauncherLog copies launch.sh's own log into ours and deletes it.
func TakeLauncherLog(dataDir string) {
	path := filepath.Join(dataDir, launcherLogFile)
	f, err := os.Open(path)
	if err != nil {
		return
	}
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		logger.Info("launch.sh: %s", sc.Text())
	}
	f.Close()
	if err := os.Remove(path); err != nil {
		logger.Warn("appupdate: remove %s: %v", path, err)
	}
}

// String is for log lines.
func (p Pending) String() string { return fmt.Sprintf("%s → %s", p.From, p.To) }
