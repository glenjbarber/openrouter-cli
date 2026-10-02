package tui

// The work indicators are drawn in color that deepens with the depth of the
// work, so that a reader watching a turn sees it get louder rather than having
// to count what it is doing.
//
// Color is applied by Draw, around the row rather than into it, since the row
// text is what a terminal selection copies out. A sequence written into a row
// would be copied along with the prose and would act on whatever the reader
// pasted it into, which is the reason the frame is plain text. Draw already
// writes sequences around the frame for the cursor and the clear line, so
// nothing new is introduced by styling a row the same way.

// style is what one pane row is drawn in.
//
// A zero style is the ordinary one, which is plain text in the default
// attribute. A row carrying a style is drawn with it and reset afterwards, so
// that a styled row cannot color the rows beneath it, which are prose.
type style uint8

const (
	// stylePlain is the ordinary attribute, used for prose and for anything
	// the frame does not name a style for.
	stylePlain style = iota
	// styleDepth0 is the first round of a turn that called tools. It is the
	// quietest of the ramp rather than plain, so that a reader can tell a
	// working turn from a finished one at a glance without reading the words.
	styleDepth0
	// styleDepth1 is the second and third rounds.
	styleDepth1
	// styleDepth2 is the fourth and fifth, which is past the middle of the
	// limit and is where a turn worth stopping becomes worth noticing.
	styleDepth2
	// styleDepth3 is the sixth and seventh, which is a turn close to being cut
	// off.
	styleDepth3
	// styleDepth4 is the last round, where the turn is about to be stopped for
	// reaching its limit. It is the loudest state, since a reader who is about
	// to lose the turn is the one who most needs to see it coming.
	styleDepth4
	// styleError marks a failure, such as a turn that ended in an error. It is
	// not one of the depths, since red on the ramp means the depth of the work
	// and red here would mean the work failed, which are different things.
	styleError
	// styleStopped marks a turn the reader stopped. It is distinct from the
	// error style, since a stop the reader asked for is not a fault and drawing
	// it as one would tell them that something broke when they had not.
	styleStopped
)

// styleSequence returns the sequence that sets a style, and the one that puts
// the terminal back.
//
// The reset is written after every styled row rather than once at the end of
// the frame, so that a styled row cannot color the prose drawn under it.
//
// A plain row is written with no sequence at all, neither the set nor the
// reset. The frame already resets the attribute before each row, so a plain row
// needs nothing, and writing a reset for it would double the output for every
// row of a conversation that is mostly prose.
func styleSequence(st style) string {
	switch st {
	case styleDepth0:
		return "\x1b[90m"
	case styleDepth1:
		return "\x1b[36m"
	case styleDepth2:
		return "\x1b[33m"
	case styleDepth3:
		return "\x1b[31m"
	case styleDepth4:
		return "\x1b[1;31m"
	case styleError:
		return "\x1b[1;31m"
	case styleStopped:
		return "\x1b[35m"
	default:
		return ""
	}
}

// styleDepth returns the style for a round of a turn.
//
// The round is counted from one, since a reader watching the work counts the
// first request as the first thing the model did rather than as round zero.
// A round past the limit takes the loudest style rather than the plain one, so
// that a turn stopped at its limit reads as loud rather than as a turn that
// simply stopped working.
func styleDepth(round int) style {
	switch {
	case round <= 1:
		return styleDepth0
	case round <= 3:
		return styleDepth1
	case round <= 5:
		return styleDepth2
	case round <= 7:
		return styleDepth3
	default:
		return styleDepth4
	}
}

// styleMark is one entry of the pane and the style it was added with.
//
// The text is kept beside the style rather than matched for at paint time,
// since a reply is folded before it is drawn and a line may occupy more than
// one row, so a fold of a styled line cannot be found by looking for its text.
// The entry records which row of the pane the style belongs to by its position
// among the entries, which is what the paint path counts.
type styleMark struct {
	// at is the index of the entry in the pane, counted from the oldest.
	at int
	// st is the style the entry is drawn in.
	st style
}

// styleMarkFor returns the style for the entry at the given index, and whether
// one was marked.
//
// The list is walked rather than indexed, since entries are added as work
// happens and the index a row was drawn at is not the index it was added at
// once the pane has been cleared and refilled. A pane cleared and refilled
// leaves marks pointing at rows that are no longer the ones they were added
// for, and the caller drops the marks whose index is past the end of the pane.
func styleMarkFor(marks []styleMark, index int) (style, bool) {
	for _, m := range marks {
		if m.at == index {
			return m.st, true
		}
	}
	return stylePlain, false
}
