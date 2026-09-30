//go:build !headless

package ui

import (
	"time"

	"github.com/carroarmato0/nextui-itchio-pak/internal/appupdate"
	"github.com/carroarmato0/nextui-itchio-pak/internal/netstate"
	"github.com/carroarmato0/nextui-itchio-pak/internal/renderer"
)

// devAppUpdater is a stand-in for *appupdate.Checker so scenes can show every
// Updates state without a network.
type devAppUpdater struct {
	disabled bool
	v        appupdate.Verdict
	running  bool
	at       time.Time
	err      error
	archive  appupdate.ArchiveStatus
}

func (d *devAppUpdater) Enabled() bool                            { return !d.disabled }
func (d *devAppUpdater) Verdict() appupdate.Verdict               { return d.v }
func (d *devAppUpdater) Channel() appupdate.Channel               { return d.v.Channel }
func (d *devAppUpdater) SetChannel(ch appupdate.Channel)          { d.v.Channel = ch }
func (d *devAppUpdater) CheckNow()                                {}
func (d *devAppUpdater) IsRunning() bool                          { return d.running }
func (d *devAppUpdater) CheckedAt() time.Time                     { return d.at }
func (d *devAppUpdater) LastError() error                         { return d.err }
func (d *devAppUpdater) RateLimitedUntil() time.Time              { return time.Time{} }
func (d *devAppUpdater) PendingNotice() (appupdate.Verdict, bool) { return d.v, false }
func (d *devAppUpdater) MarkNotified(appupdate.Channel, string)   {}
func (d *devAppUpdater) StartArchiveSave()                        {}
func (d *devAppUpdater) CancelArchiveSave()                       {}
func (d *devAppUpdater) ArchiveStatus() appupdate.ArchiveStatus   { return d.archive }

func devVer(s string) appupdate.Version { v, _ := appupdate.Parse(s); return v }

func devRelease(tag string) *appupdate.Release {
	return &appupdate.Release{Tag: tag, URL: "https://github.com/carroarmato0/NextUI-Itchio-Pak/releases/tag/" + tag,
		Asset: "https://example.invalid/" + appupdate.AssetName(tag), Digest: "sha256:00", Size: 12819887}
}

// noticeScene draws a screen with the notice resting over it.
type noticeScene struct {
	Screen
	v appupdate.Verdict
}

func (s noticeScene) Draw(r *renderer.Renderer) {
	r.Overlay = func(r *renderer.Renderer) { drawNotice(r, s.v, 1) }
	defer func() { r.Overlay = nil }()
	s.Screen.Draw(r)
}

func devUpdates(d SceneDeps, u *devAppUpdater) Screen {
	netstate.SetForTest(netstate.State{Status: netstate.StatusOnline})
	return NewUpdatesScreen(d.Cfg, d.CfgPath, u, devList(d))
}

func init() {
	rcAvail := appupdate.Verdict{Kind: appupdate.Available, Channel: appupdate.Stable,
		Running: devVer("v1.1.0-rc4"), Latest: devRelease("v1.1.0"), Via: appupdate.ViaReleasePage}
	storeAvail := appupdate.Verdict{Kind: appupdate.Available, Channel: appupdate.Stable,
		Running: devVer("v1.0.25"), Latest: devRelease("v1.0.26"), Via: appupdate.ViaPakStore}
	archive := appupdate.Verdict{Kind: appupdate.Available, Channel: appupdate.RC,
		Running: devVer("v1.1.0-rc2"), Latest: devRelease("v1.1.0-rc3"), Via: appupdate.ViaArchive}

	devScenes = append(devScenes,
		// Over the game list, whose top-right header holds the sort, platform
		// and Offline pills: the audit checks the notice against them.
		Scene{Name: "update-toast", Desc: "Update notice resting top-right over the game list", Build: func(d SceneDeps) Screen {
			netstate.SetForTest(netstate.State{Status: netstate.StatusOnline})
			return noticeScene{devList(d), rcAvail}
		}},
		Scene{Name: "update-toast-pakstore", Desc: "Update notice pointing at the Pak Store", Build: func(d SceneDeps) Screen {
			netstate.SetForTest(netstate.State{Status: netstate.StatusOnline})
			return noticeScene{devList(d), storeAvail}
		}},
		Scene{Name: "settings-updates-selected", Desc: "Settings, cursor on Updates, version available (annotation on the Accent pill)", Build: func(d SceneDeps) Screen {
			netstate.SetForTest(netstate.State{Status: netstate.StatusOnline})
			SetAppUpdater(&devAppUpdater{v: rcAvail})
			defer SetAppUpdater(nil)
			s := devSettings(d)
			s.cursor = sItemUpdates
			return s
		}},
		Scene{Name: "updates-latest", Desc: "Updates: up to date", Build: func(d SceneDeps) Screen {
			return devUpdates(d, &devAppUpdater{at: time.Now().Add(-5 * time.Minute), v: appupdate.Verdict{
				Kind: appupdate.UpToDate, Channel: appupdate.RC, Running: devVer("v1.1.0-rc3"), Latest: devRelease("v1.1.0-rc3")}})
		}},
		Scene{Name: "updates-available", Desc: "Updates: available, release-page QR", Build: func(d SceneDeps) Screen {
			return devUpdates(d, &devAppUpdater{at: time.Now(), v: rcAvail})
		}},
		Scene{Name: "updates-pakstore", Desc: "Updates: available through the Pak Store", Build: func(d SceneDeps) Screen {
			return devUpdates(d, &devAppUpdater{at: time.Now(), v: storeAvail})
		}},
		Scene{Name: "updates-ahead", Desc: "Updates: rc ahead of Stable, with the Pak Store downgrade warning", Build: func(d SceneDeps) Screen {
			return devUpdates(d, &devAppUpdater{at: time.Now(), v: appupdate.Verdict{Kind: appupdate.UpToDate, Ahead: true,
				Channel: appupdate.Stable, Running: devVer("v1.1.0-rc3"), Latest: devRelease("v1.0.25"), StoreOffers: "v1.0.25"}})
		}},
		Scene{Name: "updates-offline", Desc: "Updates: offline, never checked", Build: func(d SceneDeps) Screen {
			s := devUpdates(d, &devAppUpdater{v: appupdate.Verdict{Channel: appupdate.Stable}})
			netstate.SetForTest(netstate.State{Status: netstate.StatusOffline, Reason: netstate.ReasonDNS})
			return s
		}},
		Scene{Name: "updates-dev-build", Desc: "Updates: a dev build, which never checks", Build: func(d SceneDeps) Screen {
			return devUpdates(d, &devAppUpdater{disabled: true, v: appupdate.Verdict{Channel: appupdate.Stable}})
		}},
		Scene{Name: "updates-archive-progress", Desc: "Updates (muOS): Save to ARCHIVE in progress", Build: func(d SceneDeps) Screen {
			return devUpdates(d, &devAppUpdater{at: time.Now(), v: archive,
				archive: appupdate.ArchiveStatus{State: appupdate.ArchiveRunning, Tag: "v1.1.0-rc3", Done: 5 << 20, Total: 12819887}})
		}},
		Scene{Name: "updates-archive-done", Desc: "Updates (muOS): saved to ARCHIVE", Build: func(d SceneDeps) Screen {
			return devUpdates(d, &devAppUpdater{at: time.Now(), v: archive,
				archive: appupdate.ArchiveStatus{State: appupdate.ArchiveDone, Tag: "v1.1.0-rc3", Path: "/mnt/mmc/ARCHIVE/" + appupdate.AssetName("v1.1.0-rc3")}})
		}},
		Scene{Name: "updates-archive-failed", Desc: "Updates (muOS): integrity check failed", Build: func(d SceneDeps) Screen {
			return devUpdates(d, &devAppUpdater{at: time.Now(), v: archive,
				archive: appupdate.ArchiveStatus{State: appupdate.ArchiveFailed, Tag: "v1.1.0-rc3", Err: appupdate.ErrIntegrity}})
		}},
	)
}
