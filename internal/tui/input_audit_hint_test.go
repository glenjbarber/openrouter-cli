package tui

import (
	"strings"
	"testing"
)

// The hint row names only keys that act, and names Tab only where the filter
// or the editor acts on it, since a key the reader was told about and which
// then did nothing would read as a hung client.
func TestAuditHintRowNamesNoKeyThatDoesNothing(t *testing.T) {
	for _, st := range []hintState{
		{},
		{history: true},
		{busy: true},
		{mouse: true},
		{history: true, busy: true, mouse: true},
	} {
		for _, h := range st.hints() {
			if strings.Contains(h, "Tab") && st.overlay != hintCompose {
				t.Errorf("overlay %d names Tab: %q", st.overlay, h)
			}
		}
	}

	// The filter names neither history nor the wheel, since neither acts
	// while the catalogue is open.
	st := hintState{history: true, busy: true, mouse: true, overlay: hintListing}
	for _, h := range st.hints() {
		if strings.Contains(h, "Up/Down") || strings.Contains(h, "wheel") {
			t.Errorf("the listing names a key that does not act there: %q", h)
		}
	}

	// The search names neither Tab nor the wheel. It reads a byte at a time
	// and does not complete, so naming Tab there would promise nothing.
	search := hintState{history: true, mouse: true, overlay: hintSearch}
	for _, h := range search.hints() {
		if strings.Contains(h, "Tab") || strings.Contains(h, "wheel") {
			t.Errorf("the search names a key that does not act there: %q", h)
		}
	}
}

// The row is budgeted from the rendered line rather than from the entries,
// so a terminal too narrow for even one entry gives up no row.
func TestAuditHintCostsNoRowWhenNothingFits(t *testing.T) {
	hints := hintState{}.hints()
	line := hintLine(hints, 2)
	if line != "" {
		t.Errorf("row = %q at width 2, want empty", line)
	}
	if n := hintRows(line, 10); n != 0 {
		t.Errorf("hintRows = %d for an unrenderable row, want 0", n)
	}
	if n := hintRows(hintLine(hints, 40), 0); n != 0 {
		t.Errorf("hintRows = %d with no room, want 0", n)
	}
	if n := hintRows(hintLine(hints, 40), 1); n != 1 {
		t.Errorf("hintRows = %d with a row of room, want 1", n)
	}
}
