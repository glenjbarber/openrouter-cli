package tui

import (
	"strings"
	"testing"
)

// searchSession returns a session holding a conversation of the given entries,
// with a screen of a known size so the fold is the one the reader would see.
func searchSession(entries []string) *Session {
	return &Session{
		conv:   NewConversation(),
		screen: &Screen{height: 30, width: 80},
		frame:  Frame{Reply: entries},
	}
}

// beginTestSearch opens the search the way the command does, without drawing.
func beginTestSearch(s *Session) {
	s.mu.Lock()
	s.searchReply = s.frame.Reply
	s.searchOpen = true
	s.search = ""
	s.searchScroll = s.scroll
	s.mu.Unlock()
	s.searchPane()
}

// searchConversation is a short exchange used by most of the tests.
func searchConversation() []string {
	return []string{
		"> what is the plan",
		"the plan is to finish the pane",
		"> go on",
		"the pane is finished and the search finds a distinctive word: kumquat",
	}
}

func TestSearchFindsMatches(t *testing.T) {
	s := searchSession(searchConversation())
	beginTestSearch(s)
	s.search = "kumquat"
	s.searchPane()

	body := strings.Join(s.frame.Reply, "\n")
	if !strings.Contains(body, "kumquat") {
		t.Errorf("frame is missing the matching line:\n%s", body)
	}
	if !strings.Contains(body, "1 matching") {
		t.Errorf("frame does not report the count:\n%s", body)
	}
}

// The query is shown, so that a reader who has lost track of what was typed
// can see it rather than having to delete back to find out.
func TestSearchShowsTheQuery(t *testing.T) {
	s := searchSession(searchConversation())
	beginTestSearch(s)
	s.search = "kumquat"
	s.searchPane()

	body := strings.Join(s.frame.Reply, "\n")
	if !strings.Contains(body, "search: kumquat_") {
		t.Errorf("frame does not show the query:\n%s", body)
	}
}

func TestSearchIsCaseInsensitive(t *testing.T) {
	s := searchSession(searchConversation())
	beginTestSearch(s)
	s.search = "KUMQUAT"
	s.searchPane()

	body := strings.Join(s.frame.Reply, "\n")
	if !strings.Contains(body, "1 matching") {
		t.Errorf("an upper-case query did not match:\n%s", body)
	}
}

// A query matching nothing must say so, rather than showing a pane that reads
// as a failed search rather than an empty one.
func TestSearchReportsNoMatch(t *testing.T) {
	s := searchSession(searchConversation())
	beginTestSearch(s)
	s.search = "nothing here"
	s.searchPane()

	body := strings.Join(s.frame.Reply, "\n")
	if !strings.Contains(body, "nothing matches") {
		t.Errorf("frame does not report the empty result:\n%s", body)
	}
}

// An empty query matches nothing but must not claim the pane is full of
// matches, and must say so rather than sitting blank.
func TestSearchEmptyQuery(t *testing.T) {
	s := searchSession(searchConversation())
	beginTestSearch(s)

	body := strings.Join(s.frame.Reply, "\n")
	if !strings.Contains(body, "nothing matches") {
		t.Errorf("an empty query did not report an empty result:\n%s", body)
	}
	if strings.Contains(body, "0 matching") {
		t.Errorf("an empty query reported a count rather than nothing:\n%s", body)
	}
}

// The count must be right when there is more than one match, since a count
// that disagrees with the lines shown is worse than no count.
func TestSearchCountsEveryMatch(t *testing.T) {
	entries := []string{"alpha one", "beta", "gamma alpha two", "alpha three"}
	s := searchSession(entries)
	beginTestSearch(s)
	s.search = "alpha"
	s.searchPane()

	body := strings.Join(s.frame.Reply, "\n")
	if !strings.Contains(body, "3 matching") {
		t.Errorf("frame does not count every match:\n%s", body)
	}
}

// The listing carries no escape sequence, since a selection taken from the
// pane must yield plain text.
func TestSearchListingIsPlainText(t *testing.T) {
	s := searchSession(searchConversation())
	beginTestSearch(s)
	s.search = "kumquat"
	s.searchPane()

	for _, line := range s.frame.Reply {
		if strings.ContainsAny(line, "\x1b\a") {
			t.Errorf("listing carries an escape sequence: %q", line)
		}
	}
}

// The matched part is made visible by the column it starts at, in the margin.
func TestSearchMarksTheColumn(t *testing.T) {
	s := searchSession([]string{"say the distinctive word kumquat here"})
	beginTestSearch(s)
	s.search = "kumquat"
	s.searchPane()

	var found string
	for _, line := range s.frame.Reply {
		if strings.Contains(line, "kumquat") && strings.Contains(line, "|") {
			found = line
		}
	}
	if found == "" {
		t.Fatal("the matching line is not shown")
	}
	// The word begins at column 25 of the text, after "say the distinctive
	// word ".
	if !strings.HasPrefix(found, "  25 | ") {
		t.Errorf("line = %q, want the match column in the margin", found)
	}
}

// A long line is folded before it is searched, so a match lands on the row the
// reader actually saw rather than on an entry that was never shown whole.
func TestSearchFoldsBeforeMatching(t *testing.T) {
	var b strings.Builder
	for i := 0; i < 40; i++ {
		b.WriteString("filler word ")
	}
	b.WriteString("kumquat")

	s := searchSession([]string{b.String()})
	beginTestSearch(s)
	s.search = "kumquat"
	s.searchPane()

	matches := searchMatches(s.searchReply, "kumquat", 80)
	if len(matches) != 1 {
		t.Fatalf("matches = %d, want one", len(matches))
	}
	// The word falls on a folded row well below the first.
	if matches[0].Index == 0 {
		t.Error("the match was not found on a folded row")
	}
	if len(matches[0].Text) > 80 {
		t.Errorf("the matched row is %d columns, want it folded", len(matches[0].Text))
	}
}

// A long conversation with a distinctive word deep in it must still find the
// word, and the count must survive the fold.
func TestSearchLongConversation(t *testing.T) {
	var entries []string
	for i := 0; i < 30; i++ {
		entries = append(entries, "> a question about the middle")
		entries = append(entries, "an answer that is long enough to occupy more than one row of the pane when it is folded")
	}
	entries = append(entries, "and at last a distinctive kumquat appears")

	s := searchSession(entries)
	beginTestSearch(s)
	s.search = "kumquat"
	s.searchPane()

	body := strings.Join(s.frame.Reply, "\n")
	if !strings.Contains(body, "1 matching") {
		t.Errorf("a long conversation lost the match:\n%s", body)
	}
}

// Backspace removes one character of the query, and the count follows.
func TestSearchBackspace(t *testing.T) {
	s := searchSession(searchConversation())
	beginTestSearch(s)
	s.search = "kumqat"
	s.search = trimLastRuneString(s.search)
	if s.search != "kumqa" {
		t.Errorf("search = %q, want one character removed", s.search)
	}
}

// Escape closes the search and restores the pane that was there before.
func TestSearchEscapeRestores(t *testing.T) {
	entries := searchConversation()
	s := searchSession(entries)
	beginTestSearch(s)
	s.search = "kumquat"
	s.searchPane()

	s.searchPaneKey(keyEscape)

	if s.searching() {
		t.Error("the search is still open after escape")
	}
	if len(s.frame.Reply) != len(entries) {
		t.Errorf("the pane was not restored: %d lines, want %d", len(s.frame.Reply), len(entries))
	}
	if !strings.Contains(s.frame.Reply[len(s.frame.Reply)-1], "kumquat") {
		t.Error("the restored pane is not the conversation")
	}
}

// Enter with a match moves the view, and closing returns it to where the
// reader had it, so the search does not fight the wheel.
func TestSearchEnterJumpsAndClosingResets(t *testing.T) {
	var entries []string
	for i := 0; i < 40; i++ {
		entries = append(entries, "a line of ordinary conversation")
	}
	entries = append(entries, "and at last a distinctive kumquat appears")
	// Content below the match, so the newest match is not already sitting at
	// the bottom of the pane and the jump has somewhere to move it to.
	for i := 0; i < 40; i++ {
		entries = append(entries, "more conversation after the match")
	}
	s := searchSession(entries)
	beginTestSearch(s)
	s.search = "kumquat"
	s.searchPane()

	s.searchPaneKey(keyEnter)
	if s.scroll <= 0 {
		t.Errorf("scroll = %d, want a jump to the match", s.scroll)
	}

	s.searchPaneKey(keyEscape)
	if s.scroll != 0 {
		t.Errorf("scroll = %d after closing, want the offset restored", s.scroll)
	}
}

// Enter with nothing found must not move the view, since there is nowhere to
// go and the pane should stay readable.
func TestSearchEnterWithNoMatchDoesNotMove(t *testing.T) {
	s := searchSession(searchConversation())
	beginTestSearch(s)
	s.search = "nothing here"

	s.searchPaneKey(keyEnter)
	if s.scroll != 0 {
		t.Errorf("scroll = %d, want the view left alone", s.scroll)
	}
}

// The offset the reader held is restored, not reset to zero, so a search
// opened while scrolled back does not silently return them to the bottom.
func TestSearchClosingRestoresAHeldOffset(t *testing.T) {
	s := searchSession(searchConversation())
	s.scroll = 4
	beginTestSearch(s)
	s.search = "kumquat"
	s.searchPaneKey(keyEnter)

	s.searchPaneKey(keyEscape)
	if s.scroll != 4 {
		t.Errorf("scroll = %d, want the offset held before the search", s.scroll)
	}
}

func TestPaneRows(t *testing.T) {
	if got := paneRows(30); got <= 0 {
		t.Errorf("paneRows(30) = %d, want a positive number", got)
	}
	if got := paneRows(1); got != 1 {
		t.Errorf("paneRows(1) = %d, want the pane never empty", got)
	}
}
