//go:build !headless

package ui

import (
	"sync"
	"time"

	"github.com/carroarmato0/nextui-itchio-pak/internal/appupdate"
)

// AppUpdater is satisfied by *appupdate.Checker; an interface so offscreen
// scenes and tests can use a stand-in.
type AppUpdater interface {
	Verdict() appupdate.Verdict
	Channel() appupdate.Channel
	SetChannel(appupdate.Channel)
	CheckNow()
	IsRunning() bool
	CheckedAt() time.Time
	LastError() error
	RateLimitedUntil() time.Time
	PendingNotice() (appupdate.Verdict, bool)
	MarkNotified(ch appupdate.Channel, tag string)
	StartArchiveSave()
	CancelArchiveSave()
	ArchiveStatus() appupdate.ArchiveStatus
}

var (
	appUpdMu sync.RWMutex
	appUpd   AppUpdater
)

// SetAppUpdater installs the app-update checker. main_sdl.go calls it once at
// startup. Unset (the host, tests) hides Settings → Updates and the notice.
func SetAppUpdater(u AppUpdater) {
	appUpdMu.Lock()
	appUpd = u
	appUpdMu.Unlock()
}

func appUpdater() AppUpdater {
	appUpdMu.RLock()
	defer appUpdMu.RUnlock()
	return appUpd
}
