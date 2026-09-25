package ui

// What the A button does on a game page. Paid games are visible to everyone;
// only this action depends on being signed in and owning the game. Local
// files — the status card, Delete, renaming — never do.
// See docs/mockups/detail-paid-gating.html.

type detailAction int

const (
	actionDownload detailAction = iota
	actionDownloadAgain
	actionBrowserOnly
	actionSignIn       // paid, signed out
	actionSignInAgain  // installed paid game, signed out, nothing new upstream
	actionReauthUpdate // installed paid game, signed out, an update is waiting
	actionBuy          // signed in but not owned: the QR code opens the store page
	actionChecking     // signed in, asking itch.io whether it is owned
)

type detailActionInput struct {
	Free          bool // free or name-your-own-price
	BrowserOnly   bool
	Installed     bool
	UpdatePending bool
	SignedIn      bool
	Owned         bool
	// CheckingOwnership is set while a background owned-keys lookup runs for
	// a paid game the cached owned list does not include.
	CheckingOwnership bool
}

func chooseDetailAction(in detailActionInput) detailAction {
	switch {
	case in.BrowserOnly:
		return actionBrowserOnly
	case in.Free:
		if in.Installed {
			return actionDownloadAgain
		}
		return actionDownload
	case !in.SignedIn && in.Installed && in.UpdatePending:
		return actionReauthUpdate
	case !in.SignedIn && in.Installed:
		return actionSignInAgain
	case !in.SignedIn:
		return actionSignIn
	case in.Owned && in.Installed:
		return actionDownloadAgain
	case in.Owned:
		return actionDownload
	case in.CheckingOwnership:
		return actionChecking
	default:
		return actionBuy
	}
}

// Label is the text of the action row.
func (a detailAction) Label() string {
	switch a {
	case actionDownload:
		return "Download"
	case actionDownloadAgain:
		return "Download again"
	case actionBrowserOnly:
		return "Browser-only"
	case actionSignIn:
		return "Sign in to download"
	case actionSignInAgain:
		return "Sign in to download again"
	case actionReauthUpdate:
		return "Re-authenticate to update"
	case actionBuy:
		return "Scan the QR code to buy it on itch.io"
	case actionChecking:
		return "Checking your purchases"
	}
	return ""
}

// onA names what pressing A starts: "download", "sign-in", "browser-only"
// (an explanation), or "" when A does nothing.
func (a detailAction) onA() string {
	switch a {
	case actionDownload, actionDownloadAgain:
		return "download"
	case actionSignIn, actionSignInAgain, actionReauthUpdate:
		return "sign-in"
	case actionBrowserOnly:
		return "browser-only"
	}
	return ""
}

func (a detailAction) String() string { return a.Label() }
