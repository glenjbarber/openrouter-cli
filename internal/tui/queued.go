package tui

import "strings"

// queuedPrefix marks a line the reader sent while a model was working.
//
// The prefix is what tells the model that the line is a follow-up or an urgent
// interruption rather than a question standing on its own. Without it a queued
// line is joined to the request it updates by a blank line, and a blank line
// reads as the start of a new topic rather than as a correction to what came
// before.
const queuedPrefix = "[QUEUED] "

// asQueued returns a queued line as it is sent to the model.
//
// The prefix is added here rather than where the line is queued, so that the
// pane is not marked twice. The queue is already drawn with queuedMarker, and a
// row reading "queued [QUEUED] an update" says the same thing in two ways.
//
// A line that already carries the prefix is left as it is, since a reader who
// typed the marker themselves should not be shown it twice either, and a line
// queued twice by a retry would otherwise grow a second copy.
func asQueued(line string) string {
	line = strings.TrimSpace(line)
	if line == "" {
		return ""
	}
	if strings.HasPrefix(line, queuedPrefix) {
		return line
	}
	return queuedPrefix + line
}

// queuedTexts marks every line of a queue, dropping the ones that are empty.
//
// The queue is taken under the session lock and marked outside it, since the
// marking is a string operation and holding the lock across it would stop the
// twiddle turning for the length of it. A line that is nothing but whitespace
// is dropped rather than marked, since a marker alone would be a queued message
// with no message in it.
func queuedTexts(queued []string) []string {
	if len(queued) == 0 {
		return nil
	}
	out := make([]string, 0, len(queued))
	for _, line := range queued {
		if marked := asQueued(line); marked != "" {
			out = append(out, marked)
		}
	}
	return out
}
