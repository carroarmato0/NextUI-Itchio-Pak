//go:build !headless

package ui

import (
	"time"

	"github.com/carroarmato0/nextui-itchio-pak/internal/itchio"
)

// Sign-in scenes: a SignInScreen frozen in each state, with no network.
func init() {
	build := func(st signInState) func(SceneDeps) Screen {
		return func(d SceneDeps) Screen {
			s := &SignInScreen{client: d.Client, cfg: d.Cfg, cfgPath: d.CfgPath}
			s.login = &itchio.DeviceLogin{
				UserCode: "KX7T-4MPB",
				QRURL:    "https://itch.io/user/oauth/device?code=tQ8wLk2VnR5xZp9bYc3fHs",
				// Just under eight minutes left, as in the mockup.
				Expires: time.Now().Add(7*time.Minute + 48*time.Second + 500*time.Millisecond),
			}
			s.username, s.ownedCount = "carroarmato0", 31
			s.storeState(st)
			return s
		}
	}
	devScenes = append(devScenes,
		Scene{Name: "signin", Desc: "QR sign-in, waiting for the phone", Build: build(signInWaiting)},
		Scene{Name: "signin-success", Desc: "QR sign-in finished", Build: build(signInSuccess)},
		Scene{Name: "signin-expired", Desc: "QR sign-in code expired", Build: build(signInExpired)},
		Scene{Name: "signin-denied", Desc: "QR sign-in declined on the phone", Build: build(signInDenied)},
		Scene{Name: "signin-unavailable", Desc: "QR sign-in not enabled for this app yet", Build: build(signInUnavailable)},
		Scene{Name: "signin-offline", Desc: "QR sign-in could not reach itch.io", Build: build(signInFailed)},
		Scene{Name: "account-prompt", Desc: "First-run \"Do you have an itch.io account?\" prompt", Build: func(d SceneDeps) Screen {
			return &AccountPromptScreen{client: d.Client, cfg: d.Cfg, cfgPath: d.CfgPath}
		}},
		Scene{Name: "account-prompt-legacy", Desc: "Prompt after upgrading removed an API key", Build: func(d SceneDeps) Screen {
			return &AccountPromptScreen{client: d.Client, cfg: d.Cfg, cfgPath: d.CfgPath, legacy: true}
		}},
		Scene{Name: "settings-signed-in", Desc: "Settings with the Account row signed in", Build: func(d SceneDeps) Screen {
			d.Cfg.SetSignedIn("dev-token", "carroarmato0", time.Now())
			return devSettings(d)
		}},
	)
}
