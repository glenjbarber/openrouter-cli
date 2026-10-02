package tui

import "strings"

// responseRule is the division drawn between one exchange and the next, so
// that two of them read as two rather than as one run of text.
//
// The rule is a box-drawing character rather than an ASCII dash, since a run of
// dashes reads as text and a rule reads as a rule, which is what ruleRune is
// for and is why the frame is already divided with the same character. It is
// drawn across the full width rather than a short run, so that a reader sees
// where the answer ends without having to count characters.
//
// The rule is plain text. A frame is rendered into plain text as well as onto
// a terminal, and a selection out of the pane is copied as whatever is on the
// screen, so a sequence written into a row would be copied out with the prose
// around it.
const responseRule = ruleRune

// responseRuleRow is the row a division takes above a reply.
//
// It is a rule on a row of its own with a blank above it, rather than a rule
// pressed against the reply, since a rule touching the reply reads as an
// underline of it rather than as a division of the screen. The reply is folded
// at draw time, so the row cannot be worked out here and is budgeted at draw
// time rather than in the renderer.
const responseRuleRow = responseRule + "\n"

// withResponseRule draws the division above a reply.
//
// The rule is put above rather than below, since the reader arrives at the
// question first and the division belongs where the answer is about to begin.
// A rule below would be read as an underline of the answer, which is the
// opposite of what a delimiter is for.
//
// A reply that is nothing but whitespace draws no rule, since a rule above a
// blank row is a rule above nothing and reads as the frame having drawn
// something for no reason. It is not a fault: a model returning nothing is
// reported in words, so the pane still says what happened.
func withResponseRule(text string) string {
	text = strings.TrimRight(text, "\n")
	if strings.TrimSpace(text) == "" {
		return text
	}
	return responseRuleRow + text
}

// responseRuleRows is what the divisions above replies take, given a pane that
// has already been filled.
//
// It is one row per reply that carries a division, which is why the caller has
// to settle the pane first: adding rows for the rules changes the height the
// pane was filled to, and a division that pushed the oldest reply off the top
// would scroll the conversation under the reader rather than divide it.
func responseRuleRows(text string) int {
	if strings.TrimSpace(strings.TrimRight(text, "\n")) == "" {
		return 0
	}
	return 1
}
