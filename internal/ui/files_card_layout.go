package ui

// filesCardInline reports whether the downloaded-files card fits on the same
// line as the action label. It needs at least half the usable width; a long
// label ("Scan the QR code to buy it on itch.io", for a game that was
// downloaded free and has since become paid) would otherwise squeeze the file
// name down to a few letters, so the card moves to its own line instead.
func filesCardInline(labelEnd, rowStart, usableW int32) bool {
	return usableW-(labelEnd-rowStart)-filesCardGap >= usableW/2
}

// filesCardGap separates the action label from the card on a shared line.
const filesCardGap = int32(12)
