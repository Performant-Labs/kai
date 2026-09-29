package doublecopy

import "testing"

// Kai's own simulated copy (execkey) marks itself before posting the key so the source can tell it
// apart from the user's. On every platform the call must exist and be safe to make.
func TestMarkOwnCopyIsSafeToCall(t *testing.T) {
	MarkOwnCopy()
}
