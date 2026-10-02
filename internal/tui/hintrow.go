package tui

// The hint row names the keys that do something in the state the interface is
// in. It sits on one row above the prompt, inside the input block, rather than
// below the prompt as a footer.
//
// The position is above the prompt for two reasons. The prompt stays the last
// row of the block, and the caret is placed on the last row, so the caret
// stays on the prompt. A footer would have to report which row the prompt is
// on, and the frame owns a single budget of rows rather than a second index
// into it. The hint also sits directly against the thing it describes, so it
// reads as a caption for the input line rather than as a note about the
// screen.

// hintSep separates one key from the next on the row.
//
// Two spaces rather than a rule or a bullet, since the row is plain text that
// a terminal selection copies out, and a decoration copied along with it is
// noise.
const hintSep = "  "

// hintOverlay names what has taken the input line.
//
// The keys that do something differ between composing a message and filtering
// a listing, so the row cannot be built from the composing case alone.
type hintOverlay int

const (
	// hintCompose is the ordinary state, where the line editor has the keys.
	hintCompose hintOverlay = iota
	// hintListing is the model catalogue being filtered.
	hintListing
	// hintSearch is the pane search being typed into.
	hintSearch
	// hintConfirm is an approval question being answered, where the keys that
	// answer it are not the keys that compose a message.
	hintConfirm
)

// hintState is what the row reads in order to decide which keys to name.
//
// A key is named only when it does something in the state being described. A
// row that promised a key which did nothing would be worse than no row, since
// a reader would press it and conclude the client had hung.
type hintState struct {
	overlay hintOverlay
	// history reports that the line editor holds something to recall, which
	// is what gives the up and down arrows an action at all. With nothing to
	// recall they do nothing, so they are not named.
	history bool
	// busy reports that a request is in flight, which is what makes Enter
	// queue a line rather than send it and escape stop the model. Neither key
	// does that on an idle prompt, so neither is named there.
	busy bool
	// mouse reports that reporting is on, which is what makes the wheel
	// scroll. Reporting is off unless the reader asked for it, so the wheel
	// is not named while it is off.
	mouse bool
}

// hints returns the keys that act in the state, in a fixed order.
//
// The order is fixed rather than by importance, since a row that reordered
// itself as the state changed could not be read at a glance. What is named
// changes; where each name sits does not.
func (st hintState) hints() []string {
	switch st.overlay {
	case hintListing:
		// Tab is named here because the filter completes on it, cycling
		// through what the filter matched. It was not named while the
		// filter took characters only, since naming a key that did
		// nothing was the one thing the row must not do.
		return []string{"Tab cycle", "Enter choose", "Esc close"}
	case hintSearch:
		return []string{"Enter jump to match", "Esc close"}
	case hintConfirm:
		// The keys are named because nothing else on screen says how to
		// answer, and a question with no way to answer it is a question the
		// reader can only escape. Escape is named as the refusal, since it is
		// what the question answers to anything else.
		return []string{"y once", "a all session", "n or Esc no"}
	}
	// Enter is named for what it does in this state rather than for what it
	// does on an idle prompt, since a line sent while a model is working is
	// held rather than refused, and escape stops the model with it.
	//
	// Nothing else is named. The completion and history keys act as well, but
	// naming them made the row wider than a reader reads at a glance, and the
	// row exists to name what a reader needs rather than to list every key the
	// client reads.
	if st.busy {
		return []string{"Enter queue", "Esc stop and send"}
	}
	// The compose state names what sends, what breaks the line, and what pages
	// the pane. Ctrl-J is named rather than left out, since it is the only key
	// that breaks a line on a terminal that sends nothing for shift with enter,
	// and a reader holding a multi-line message has no way to find it
	// otherwise. The shifted arrows are named for the same reason: a reader
	// holding a long conversation back and looking for a way to move through
	// it has no way to find them either.
	return []string{"Send [enter]", "[ctrl]+j newline", "[shift]+arrows page", "[ctrl]+b j/; window"}
}

// hintLine renders the keys that act onto one row.
//
// Whole entries are kept or dropped and none is cut. Half a phrase names a key
// and not what the key does, which is the one thing the row exists to say, and
// a cut entry is the failure the status bar avoids by dropping fields whole.
func hintLine(hints []string, width int) string {
	if len(hints) == 0 || width < 1 {
		return ""
	}
	kept := ""
	for _, h := range hints {
		candidate := h
		if kept != "" {
			candidate = kept + hintSep + h
		}
		// runeWidth stops at the bound it is given, so asking for one column
		// more than the width reports whether the candidate overflows.
		if runeWidth(candidate, width+1) > width {
			break
		}
		kept = candidate
	}
	return kept
}

// hintRows is the single row the hint takes, or none when there is no room.
//
// The row is budgeted with the rest of the input block rather than taken from
// the pane afterwards, so that the frame is never taller than the terminal. The
// prompt is what a reader needs in order to type a next message, so a row that
// only names keys yields to it rather than the other way round.
//
// The line is passed in already rendered rather than the entries it came from.
// A budget made from the entries would reserve a row for a hint that is too
// narrow to show even one of them, taking a row from the pane for a row that
// is never drawn.
func hintRows(line string, room int) int {
	if line == "" || room < 1 {
		return 0
	}
	return 1
}
