package tui

import "strings"

// This file holds what the session records about each entry of the reply pane,
// beside the text and never inside it.
//
// An entry is only a string, and a string cannot say who wrote it. A line that
// starts with "[fs]" may be a tool line the client wrote or a sentence a model
// chose to begin that way, and so may a line that starts with "(error)". A
// guess made from the text would colour what a model wrote as though the client
// had written it, so the client records what it wrote at the moment it writes
// it, in Frame.Kinds, which runs parallel to Frame.Reply. Nothing is read back
// out of the text.

// replyKind says what sort of line an entry is.
type replyKind uint8

const (
	// kindPlain is a line that takes no colour: the line the user typed and
	// echoed, and a note the client wrote. It is the zero value, so an entry
	// with no record is plain.
	kindPlain replyKind = iota
	// kindReply is text a model wrote. Its markdown is coloured when colour is
	// on, and the kind is what says which entries are the model's, so that no
	// guess is made from what an entry says.
	kindReply
	// kindNotice is an ordinary line the client wrote to say something.
	kindNotice
	// kindFailure is an error or a stop, and a tool call that failed.
	kindFailure
	// kindApproval is a line that waits on the reader's approval.
	kindApproval
	// kindTool is a line reporting one tool call.
	kindTool
)

// entryKind is the record of one entry: small integers and a flag, never text.
type entryKind struct {
	kind replyKind
	// tool is the identity role of the tool a kindTool entry reports, or noRole
	// for a call the session cannot account for.
	tool role
	// ok marks a tool call that succeeded, which is a routine line.
	ok bool
}

// toolIdentity returns the role a tool label is drawn in.
func toolIdentity(label string) role {
	switch label {
	case fsToolLabel:
		return roleFilesystem
	case gitToolLabel:
		return roleGit
	case shellToolLabel:
		return roleShell
	}
	return noRole
}

// identityLabel is the bracketed label a tool identity is written with.
func identityLabel(r role) string {
	switch r {
	case roleFilesystem:
		return "[" + fsToolLabel + "]"
	case roleGit:
		return "[" + gitToolLabel + "]"
	case roleShell:
		return "[" + shellToolLabel + "]"
	}
	return ""
}

// kindAt returns the record of entry i, plain where there is none.
//
// A Kinds that is missing or short means plain, so a frame built without it, as
// a test builds one, is drawn exactly as it was before the records existed.
func (f Frame) kindAt(i int) entryKind {
	if i < 0 || i >= len(f.Kinds) {
		return entryKind{}
	}
	return f.Kinds[i]
}

// syncKinds brings Kinds to the length of Reply. The caller holds mu.
//
// A writer that assigned to Reply directly leaves Kinds longer or shorter than
// it. Padding with plain and trimming keeps a stale record from colouring a row
// it was not written for.
func (s *Session) syncKinds() {
	n := len(s.frame.Reply)
	if len(s.frame.Kinds) > n {
		s.frame.Kinds = s.frame.Kinds[:n]
	}
	for len(s.frame.Kinds) < n {
		s.frame.Kinds = append(s.frame.Kinds, entryKind{})
	}
}

// replaceReply replaces the pane with the given entries, all plain. The caller
// holds mu. Every write that replaces Reply goes through it, so the records are
// reset whenever the entries they described are gone.
func (s *Session) replaceReply(lines []string) {
	s.frame.Reply = lines
	s.frame.Kinds = nil
}

// addReplyTagged appends entries carrying one record. The caller does not hold
// mu.
func (s *Session) addReplyTagged(tag entryKind, lines ...string) {
	if len(lines) == 0 {
		return
	}
	s.mu.Lock()
	s.syncKinds()
	s.frame.Reply = append(s.frame.Reply, lines...)
	for range lines {
		s.frame.Kinds = append(s.frame.Kinds, tag)
	}
	s.mu.Unlock()
}

// addReplyKind appends one entry of a kind that needs nothing else recorded.
func (s *Session) addReplyKind(kind replyKind, text string) {
	s.addReplyTagged(entryKind{kind: kind}, text)
}

// toolTag returns the record of a tool line.
func toolTag(label string, failed bool) entryKind {
	return entryKind{kind: kindTool, tool: toolIdentity(label), ok: !failed}
}

// textSpan returns the span covering a row up to its last visible character, or
// none where the row is blank. Trailing spaces are left out, since colouring
// them writes a sequence around nothing.
func textSpan(row string, r role) []span {
	end := len(strings.TrimRight(row, " "))
	if end == 0 || r < 0 || r >= roleCount {
		return nil
	}
	return []span{{start: 0, end: end, role: r}}
}

// entrySpans returns the spans of one row of an entry.
//
// An entry may fold to several rows, and the whole entry takes the same role on
// each of them. A tool line is the exception: a routine success is dimmed whole,
// and dim wins over the identity of the tool, so it carries no second span. Any
// other tool line, which is a failure, keeps the failure role and names the tool
// in its identity colour on the label, which only the first row carries.
func entrySpans(tag entryKind, row string, first bool) []span {
	switch tag.kind {
	case kindNotice:
		return textSpan(row, roleNotice)
	case kindFailure:
		return textSpan(row, roleFailure)
	case kindApproval:
		return textSpan(row, roleApproval)
	case kindTool:
		if tag.ok {
			return textSpan(row, roleDim)
		}
		label := identityLabel(tag.tool)
		if first && label != "" && strings.HasPrefix(row, label) {
			out := []span{{start: 0, end: len(label), role: tag.tool}}
			rest := textSpan(row[len(label):], roleFailure)
			for _, sp := range rest {
				out = append(out, span{start: sp.start + len(label), end: sp.end + len(label), role: sp.role})
			}
			return out
		}
		return textSpan(row, roleFailure)
	}
	return nil
}

// clipSpans cuts spans to a row that was shortened to n bytes. Spans past the
// cut are dropped and one that crosses it is shortened.
func clipSpans(spans []span, n int) []span {
	if len(spans) == 0 {
		return spans
	}
	out := make([]span, 0, len(spans))
	for _, sp := range spans {
		if sp.start >= n {
			continue
		}
		if sp.end > n {
			sp.end = n
		}
		out = append(out, sp)
	}
	return out
}
