package tui

import "strings"

// queuedLines returns the queue as the lines of an update to the request it was
// sent behind.
//
// A line is trimmed, and one that is nothing but whitespace is dropped rather
// than carried. The queue is taken under the session lock and trimmed outside
// it, since trimming is a string operation and holding the lock across it would
// stop the twiddle turning for the length of it. A blank line is dropped since
// amend joins the lines with one, so a line carrying nothing but whitespace
// would produce a blank line between the question and the correction that says
// nothing was said in between.
func queuedLines(queued []string) []string {
	if len(queued) == 0 {
		return nil
	}
	out := make([]string, 0, len(queued))
	for _, line := range queued {
		if trimmed := strings.TrimSpace(line); trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}