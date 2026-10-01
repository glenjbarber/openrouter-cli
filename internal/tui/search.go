package tui

import (
	"fmt"
	"strings"
)

// searching reports whether the pane search is open.
//
// It takes the session lock rather than reading the flag bare, since the search
// runs on the same goroutine as the keys but the pane is drawn from others.
func (s *Session) searching() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.searchOpen
}

// beginSearch opens the pane search, taking the pane over.
//
// The reply entries are held aside rather than searched where they lie, since
// the pane shows them folded and a match is only useful where it lines up with
// what the reader saw. The fold is therefore applied to the same entries here
// that the renderer folds, and the columns reported are columns of the screen.
func (s *Session) beginSearch() {
	s.mu.Lock()
	s.searchReply = s.frame.Reply
	s.searchOpen = true
	s.search = ""
	s.searchScroll = s.scroll
	s.mu.Unlock()
	s.searchPane()
	s.draw()
}

// endSearch closes the search and restores the pane and the offset.
//
// The offset is restored rather than left at the match, since the search moved
// the view only to show what it found. A view left elsewhere would put the
// reader somewhere they did not choose the moment the listing went away.
func (s *Session) endSearch() {
	s.mu.Lock()
	s.frame.Reply = s.searchReply
	s.searchReply = nil
	s.searchOpen = false
	s.search = ""
	s.scroll = s.searchScroll
	s.searchScroll = 0
	s.mu.Unlock()
	s.draw()
}

// searchKey reads one key for the search.
//
// The search takes every key while it is open, so that typing does not reach
// the line editor behind it.
func (s *Session) searchKey() {
	b, err := s.editor.ReadByte()
	if err != nil {
		s.endSearch()
		return
	}
	s.searchPaneKey(b)
}

// searchPaneKey acts on a key typed into the search.
func (s *Session) searchPaneKey(b byte) {
	switch b {
	case keyEscape:
		s.endSearch()
	case keyEnter:
		// Enter jumps to the newest match rather than closing, so that a
		// reader who has found what they were after can leave with the
		// escape they already know rather than learning a second key.
		s.jumpToMatch()
	case keyBackspace, keyDelete:
		s.mu.Lock()
		s.search = trimLastRuneString(s.search)
		s.mu.Unlock()
		s.searchPane()
		s.draw()
	default:
		if b < 0x20 {
			return
		}
		s.mu.Lock()
		s.search += string(b)
		s.mu.Unlock()
		s.searchPane()
		s.draw()
	}
}

// searchLine is one matching line of the pane, with where the match fell in it.
type searchLine struct {
	// Text is the line as the reader saw it, folded to the pane width.
	Text string
	// At is the column in Text at which the query begins.
	At int
	// Index is the position of the line in the pane, counted from the oldest.
	// The scroll offset is a count back from the newest, so this is what turns
	// a match into an offset.
	Index int
}

// searchMatches returns the folded pane lines carrying the query.
//
// The reply is folded the way the renderer folds it, since a match the reader
// cannot see is not a match. The comparison ignores case, since the reader
// recalls a word in whatever case they happen to type it. The first match on a
// line is the one reported, since the line is shown once and marking every
// occurrence would need delimiters that would be copied out with the text.
func searchMatches(entries []string, query string, width int) []searchLine {
	if query == "" {
		return nil
	}
	needle := strings.ToLower(query)

	var out []searchLine
	index := 0
	for _, entry := range entries {
		for _, line := range WrapBlock(entry, width) {
			if at := strings.Index(strings.ToLower(line), needle); at >= 0 {
				out = append(out, searchLine{Text: line, At: at, Index: index})
			}
			index++
		}
	}
	return out
}

// paneLineCount returns how many rows the reply occupies once folded.
func paneLineCount(entries []string, width int) int {
	n := 0
	for _, entry := range entries {
		n += len(WrapBlock(entry, width))
	}
	return n
}

// searchPane builds the listing of matching lines without drawing it.
//
// The listing is built separately from the drawing, so that it can be examined
// without a screen, which is what the tests do.
func (s *Session) searchPane() {
	s.mu.Lock()
	query := s.search
	entries := s.searchReply
	s.mu.Unlock()

	_, width := s.screen.Size()
	matches := searchMatches(entries, query, width)

	// The frame is replaced rather than appended, so that narrowing the query
	// does not leave the previous listing on screen above it. This is the same
	// choice the model filter makes.
	head := "search"
	if query != "" {
		head = fmt.Sprintf("%d matching %q", len(matches), query)
	}

	lines := []string{head}
	const maxShown = 20
	for i, m := range matches {
		if i >= maxShown {
			lines = append(lines, fmt.Sprintf("... and %d more", len(matches)-maxShown))
			break
		}
		lines = append(lines, markMatch(m, width))
	}
	// A query that matches nothing says so, rather than showing a pane that
	// looks like a failed search rather than an empty one.
	if len(matches) == 0 {
		lines = append(lines, "nothing matches")
	}
	lines = append(lines, "search: "+query+"_", "Enter to jump, Esc to leave")

	s.mu.Lock()
	s.frame.Reply = lines
	s.mu.Unlock()
}

// jumpToMatch moves the view so that the newest match is visible.
//
// The offset is set rather than the terminal being scrolled, so the view stays
// inside the pane and the wheel remains the one thing that otherwise moves it.
// The offset is cleared again when the search closes, since the two would
// otherwise fight over it.
func (s *Session) jumpToMatch() {
	s.mu.Lock()
	entries := s.searchReply
	query := s.search
	s.mu.Unlock()

	height, width := s.screen.Size()
	matches := searchMatches(entries, query, width)
	if len(matches) == 0 {
		// Nothing was found, so there is nowhere to jump. The listing already
		// says so, and leaving the view where it is keeps the pane readable.
		return
	}
	// The newest match is the one the reader most likely means, since the pane
	// grows downward and the last match is the one most recently written.
	newest := matches[len(matches)-1]

	s.mu.Lock()
	// The offset is a count back from the newest row, and the listing
	// occupying the pane is not counted at all, so the match is placed
	// against the entry it fell in. A pane height is held below it, so that a
	// match on the last row is not the only thing on screen.
	paneHeight := paneRows(height)
	below := paneLineCount(entries, width) - newest.Index - 1
	s.scroll = maxInt(0, below-paneHeight+1)
	s.mu.Unlock()
	s.draw()
}

// paneRows returns how many rows the reply pane holds on a terminal of the
// given height.
//
// It is stated here rather than read back from the renderer, so that a jump
// lands the match in the pane rather than under the prompt. The header and the
// rows below the pane are the ones the renderer reserves, so the two agree.
func paneRows(height int) int {
	inputRows := inputRowsBare
	if height >= minHeightForDivision {
		inputRows = inputRowsDivided
	}
	rows := height - headerRowCount - inputRows
	if rows < 1 {
		rows = 1
	}
	return rows
}

// markMatch makes the matched part of a line visible without colour.
//
// A selection is taken out of the pane as plain text, so an escape sequence
// drawn around the match would be copied along with it. The match is therefore
// shown by the column it starts at, set in the margin, and by the query in the
// heading, rather than by inverting anything. A line too narrow for the margin
// has it dropped rather than pushing the text off the edge, since the line is
// the part the reader came for.
func markMatch(m searchLine, width int) string {
	const margin = 6
	if width <= margin {
		return truncate(m.Text, width)
	}
	return fmt.Sprintf("%4d | %s", m.At, truncate(m.Text, maxInt(0, width-margin)))
}
