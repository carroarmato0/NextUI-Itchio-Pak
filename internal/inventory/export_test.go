package inventory

import "time"

// MarkCheckedForTest sets UpdateCheckedAt without recording upstream files,
// as the 1.0.x paid-game check did.
func (inv *Inventory) MarkCheckedForTest(gameURL string) {
	inv.mu.Lock()
	defer inv.mu.Unlock()
	if e, ok := inv.Entries[gameURL]; ok {
		e.UpdateCheckedAt = time.Now()
	}
}
