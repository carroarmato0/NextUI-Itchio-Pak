package ui

import "testing"

// What A does on a game page, for every combination the approved mockup
// (docs/mockups/detail-paid-gating.html) distinguishes.
func TestDetailAction(t *testing.T) {
	for _, tc := range []struct {
		name string
		in   detailActionInput
		want detailAction
	}{
		{"free", detailActionInput{Free: true}, actionDownload},
		{"free, installed", detailActionInput{Free: true, Installed: true}, actionDownloadAgain},
		{"free, installed, signed out, update pending", detailActionInput{Free: true, Installed: true, UpdatePending: true}, actionDownloadAgain},
		{"browser-only wins", detailActionInput{Free: true, BrowserOnly: true}, actionBrowserOnly},
		{"browser-only paid", detailActionInput{BrowserOnly: true, SignedIn: true, Owned: true}, actionBrowserOnly},

		{"paid, signed out", detailActionInput{}, actionSignIn},
		{"paid, signed in, not owned", detailActionInput{SignedIn: true}, actionBuy},
		{"paid, signed in, still checking", detailActionInput{SignedIn: true, CheckingOwnership: true}, actionChecking},
		{"paid, signed in, owned", detailActionInput{SignedIn: true, Owned: true}, actionDownload},

		{"installed, signed out, update pending", detailActionInput{Installed: true, UpdatePending: true}, actionReauthUpdate},
		{"installed, signed out, no update", detailActionInput{Installed: true}, actionSignInAgain},
		{"installed, signed in, owned", detailActionInput{Installed: true, SignedIn: true, Owned: true}, actionDownloadAgain},
		{"installed, signed in, owned, update pending", detailActionInput{Installed: true, SignedIn: true, Owned: true, UpdatePending: true}, actionDownloadAgain},
		// A refund after install: the files stay, re-downloading needs a purchase.
		{"installed, signed in, no longer owned", detailActionInput{Installed: true, SignedIn: true}, actionBuy},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := chooseDetailAction(tc.in); got != tc.want {
				t.Errorf("chooseDetailAction(%+v) = %v, want %v", tc.in, got, tc.want)
			}
		})
	}
}

// Only some actions start a download or a sign-in when A is pressed.
func TestDetailAction_whatAStarts(t *testing.T) {
	for a, want := range map[detailAction]string{
		actionDownload: "download", actionDownloadAgain: "download",
		actionSignIn: "sign-in", actionSignInAgain: "sign-in", actionReauthUpdate: "sign-in",
		actionBuy: "", actionChecking: "", actionBrowserOnly: "browser-only",
	} {
		if got := a.onA(); got != want {
			t.Errorf("%v.onA() = %q, want %q", a, got, want)
		}
	}
}
