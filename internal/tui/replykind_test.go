package tui

import (
	"strings"
	"testing"

	"github.com/glenjbarber/openrouter-cli/internal/config"
	"github.com/glenjbarber/openrouter-cli/internal/openrouter"
	"github.com/glenjbarber/openrouter-cli/internal/tools"
)

// kindSession returns a session with a screen that writes into a file, so the
// paths that draw can run.
func kindSession(t *testing.T) *Session {
	t.Helper()
	sc, _ := screenCapture(t)
	return &Session{conv: NewConversation(), screen: sc}
}

// replySpansOf renders a frame and returns, for each row that holds text, the
// stretches of it that carry a role, written as "role=text".
func replySpansOf(t *testing.T, f Frame, height, width int) map[string][]string {
	t.Helper()
	rows, spans, _, _ := renderStyled(f, height, width)
	out := map[string][]string{}
	for i, row := range rows {
		for _, sp := range spans[i] {
			out[strings.TrimRight(row, " ")] = append(out[strings.TrimRight(row, " ")],
				roleName(sp.role)+"="+row[sp.start:sp.end])
		}
	}
	return out
}

func roleName(r role) string {
	switch r {
	case roleFilesystem:
		return "fs"
	case roleGit:
		return "git"
	case roleShell:
		return "shell"
	case roleFailure:
		return "failure"
	case roleDim:
		return "dim"
	case roleApproval:
		return "approval"
	case roleNotice:
		return "notice"
	}
	return "other"
}

func inStep(t *testing.T, s *Session, what string) {
	t.Helper()
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.frame.Kinds) > len(s.frame.Reply) {
		t.Fatalf("%s: %d kinds for %d entries", what, len(s.frame.Kinds), len(s.frame.Reply))
	}
}

func TestKindsStayInStepThroughAddReply(t *testing.T) {
	s := kindSession(t)
	s.addReply("one", "two")
	s.addReplyKind(kindFailure, "three")
	s.appendModelText("four")
	s.appendLines("five")
	s.addReply("six")
	inStep(t, s, "after the appends")

	want := []replyKind{kindPlain, kindPlain, kindFailure, kindReply, kindNotice, kindPlain}
	for i, k := range want {
		if got := s.frame.Kinds[i].kind; got != k {
			t.Errorf("entry %d is kind %d, want %d", i, got, k)
		}
	}
}

func TestAMissingOrShortKindsMeansPlain(t *testing.T) {
	f := Frame{Reply: []string{"a", "b", "c"}, Kinds: []entryKind{{kind: kindFailure}}}
	if f.kindAt(0).kind != kindFailure {
		t.Error("the entry that has a record lost it")
	}
	for _, i := range []int{1, 2, 3, -1} {
		if f.kindAt(i).kind != kindPlain {
			t.Errorf("entry %d has no record and must be plain", i)
		}
	}
	// A direct write to Reply, and a Kinds left longer than Reply, are both put
	// right by the next append.
	s := kindSession(t)
	s.addReplyKind(kindFailure, "x")
	s.frame.Reply = []string{"p", "q", "r"}
	s.addReply("s")
	inStep(t, s, "after a direct write")
	if s.frame.Kinds[3].kind != kindPlain {
		t.Error("the appended entry was not plain")
	}
	s.frame.Reply = nil
	s.addReplyKind(kindNotice, "t")
	inStep(t, s, "after a truncation")
	if s.frame.Kinds[0].kind != kindNotice {
		t.Errorf("a stale record survived a truncation: %+v", s.frame.Kinds)
	}
}

func TestClearReplyResetsTheKinds(t *testing.T) {
	s := kindSession(t)
	s.addReplyKind(kindFailure, "x")
	s.clearReply()
	inStep(t, s, "after clearReply")
	if len(s.frame.Kinds) != 0 {
		t.Errorf("clearReply left %d records", len(s.frame.Kinds))
	}
	s.addReply("fresh")
	if s.frame.Kinds[0].kind != kindPlain {
		t.Error("the entry after a clear took the record of one before it")
	}
}

func TestSearchKeepsKindsInStepAndRestoresThem(t *testing.T) {
	s := kindSession(t)
	s.addReply("> go")
	s.addReplyKind(kindFailure, "(error) kumquat failed")
	s.addReplyKind(kindNotice, "note")
	before := append([]entryKind(nil), s.frame.Kinds...)

	s.beginSearch()
	inStep(t, s, "with the search open")
	s.mu.Lock()
	s.search = "kumquat"
	s.mu.Unlock()
	s.searchPane()
	inStep(t, s, "after a query")
	for i := range s.frame.Reply {
		if k := s.frame.kindAt(i); k.kind != kindPlain {
			t.Errorf("listing row %d carries kind %d from the pane it replaced", i, k.kind)
		}
	}
	// The listing is drawn with no span on any of its rows.
	f := s.frame
	rows, spans, _, _ := renderStyled(f, 30, 80)
	for i, row := range rows {
		for _, sp := range spans[i] {
			if strings.Contains(row, "kumquat") && sp.role != roleChrome {
				t.Errorf("the search listing row %q is colored %d", row, sp.role)
			}
		}
	}

	s.endSearch()
	inStep(t, s, "after the search closed")
	if len(s.frame.Kinds) != len(before) {
		t.Fatalf("the pane came back with %d records, want %d", len(s.frame.Kinds), len(before))
	}
	for i := range before {
		if s.frame.Kinds[i] != before[i] {
			t.Errorf("entry %d came back as %+v, want %+v", i, s.frame.Kinds[i], before[i])
		}
	}
}

func TestModelListingKeepsKindsInStep(t *testing.T) {
	s := &Session{}
	s.addReplyKind(kindFailure, "(error) old")
	s.addReplyKind(kindNotice, "old note")
	s.modelList = modelsForTest()
	s.modelFilter = "openai"
	s.pane()
	inStep(t, s, "after the listing")
	for i := range s.frame.Reply {
		if k := s.frame.kindAt(i); k.kind != kindPlain {
			t.Errorf("listing row %d carries kind %d from the pane it replaced", i, k.kind)
		}
	}
	// Leaving the listing clears the pane, and the records with it.
	s.mu.Lock()
	s.modelList = nil
	s.replaceReply(nil)
	s.mu.Unlock()
	inStep(t, s, "after the listing closed")
}

func TestTheDelegatePaneDropsTheMainKinds(t *testing.T) {
	s := &Session{}
	s.addReplyKind(kindFailure, "(error) main")
	s.addDelegateLines("/delegate what", "an answer")
	s.mu.Lock()
	s.registerPanes()
	s.panes.Select(delegatePaneIndex)
	f := s.frame
	s.applyDelegatePane(&f)
	s.mu.Unlock()
	if len(f.Reply) != 2 {
		t.Fatalf("the delegate pane holds %d lines", len(f.Reply))
	}
	if len(f.Kinds) != 0 {
		t.Errorf("the delegate pane carries %d records of the main pane", len(f.Kinds))
	}
	for _, sp := range replySpansOf(t, f, 24, 80) {
		for _, v := range sp {
			if strings.HasPrefix(v, "failure=") {
				t.Errorf("a delegate line was colored as a failure: %q", v)
			}
		}
	}
	// The main pane is untouched.
	if s.frame.Kinds[0].kind != kindFailure {
		t.Error("the main pane lost its record")
	}
}

func toolFrame(tags ...any) Frame {
	var f Frame
	for i := 0; i < len(tags); i += 2 {
		f.Reply = append(f.Reply, tags[i].(string))
		f.Kinds = append(f.Kinds, tags[i+1].(entryKind))
	}
	return f
}

func TestEachToolIdentityTakesItsRoleOnAFailure(t *testing.T) {
	for _, c := range []struct {
		label, want string
	}{{"fs", "fs"}, {"git", "git"}, {"shell", "shell"}} {
		line := "[" + c.label + "] name x -> failed: nope"
		f := toolFrame(line, toolTag(c.label, true))
		got := replySpansOf(t, f, 24, 80)[line]
		if len(got) != 2 || got[0] != c.want+"=["+c.label+"]" || got[1] != "failure= name x -> failed: nope" {
			t.Errorf("%s failure drew %q", c.label, got)
		}
	}
}

func TestAnUnknownToolFailureIsAFailureAndNothingElse(t *testing.T) {
	line := "[?] mystery x -> failed: nope"
	got := replySpansOf(t, toolFrame(line, toolTag("?", true)), 24, 80)[line]
	if len(got) != 1 || got[0] != "failure="+line {
		t.Errorf("an unlabelled failure drew %q", got)
	}
}

func TestARoutineSuccessIsDimWholeAndDimWinsOverTheIdentity(t *testing.T) {
	for _, label := range []string{"fs", "git", "shell"} {
		line := "[" + label + "] name x -> 12 bytes"
		got := replySpansOf(t, toolFrame(line, toolTag(label, false)), 24, 80)[line]
		if len(got) != 1 || got[0] != "dim="+line {
			t.Errorf("%s success drew %q", label, got)
		}
	}
}

func TestApprovalNoticeAndFailureKinds(t *testing.T) {
	f := toolFrame(
		"approval needed: go build", entryKind{kind: kindApproval},
		"a notice", entryKind{kind: kindNotice},
		"(stopped)", entryKind{kind: kindFailure},
		"> typed", entryKind{},
		"model text", entryKind{kind: kindReply},
	)
	got := replySpansOf(t, f, 24, 80)
	for line, want := range map[string]string{
		"approval needed: go build": "approval=approval needed: go build",
		"a notice":                  "notice=a notice",
		"(stopped)":                 "failure=(stopped)",
	} {
		if len(got[line]) != 1 || got[line][0] != want {
			t.Errorf("%q drew %q, want %q", line, got[line], want)
		}
	}
	for _, line := range []string{"> typed", "model text"} {
		if len(got[line]) != 0 {
			t.Errorf("%q must take no span, drew %q", line, got[line])
		}
	}
}

func TestAModelReplyThatLooksLikeAToolOrAnErrorTakesNoSpan(t *testing.T) {
	s := kindSession(t)
	s.appendModelText("[fs] read_file x -> 12 bytes")
	s.appendModelText("(error) it only looks like one")
	s.addReplyKind(kindReply, "(stopped)")
	s.addReply("> [fs] typed by the user")
	f := s.frame
	rows, spans, _, _ := renderStyled(f, 24, 80)
	seen := 0
	for i, row := range rows {
		if strings.Contains(row, "[fs]") || strings.Contains(row, "(error)") || strings.Contains(row, "(stopped)") {
			seen++
			if len(spans[i]) != 0 {
				t.Errorf("row %q was colored: %+v", row, spans[i])
			}
		}
	}
	if seen != 4 {
		t.Fatalf("found %d of the 4 rows", seen)
	}
}

func TestAnEntryThatFoldsTakesItsRoleOnEveryRow(t *testing.T) {
	long := "(error) " + strings.Repeat("the connection was refused again ", 6)
	f := toolFrame(long, entryKind{kind: kindFailure})
	rows, spans, _, _ := renderStyled(f, 24, 40)
	n := 0
	for i, row := range rows {
		if strings.Contains(row, "connection") || strings.Contains(row, "(error)") {
			n++
			if len(spans[i]) != 1 || spans[i][0].role != roleFailure ||
				spans[i][0].start != 0 || spans[i][0].end != len(strings.TrimRight(row, " ")) {
				t.Errorf("row %q carries %+v", row, spans[i])
			}
		}
	}
	if n < 3 {
		t.Fatalf("the entry folded to %d rows, want several", n)
	}
}

func TestSpansFollowAScrolledAndCutPane(t *testing.T) {
	var f Frame
	for i := 0; i < 40; i++ {
		f.Reply = append(f.Reply, "row")
		f.Kinds = append(f.Kinds, entryKind{})
	}
	f.Reply = append(f.Reply, "(error) at the end", "tail")
	f.Kinds = append(f.Kinds, entryKind{kind: kindFailure}, entryKind{})
	for _, scroll := range []int{0, 1, 5} {
		f.Scroll = scroll
		rows, spans, _, _ := renderStyled(f, 24, 80)
		for i, row := range rows {
			has := len(spans[i]) > 0 && spans[i][0].role == roleFailure
			if has != strings.HasPrefix(row, "(error)") {
				t.Errorf("scroll %d: row %q has failure span %v", scroll, row, has)
			}
		}
	}
	// A cut row keeps only the part of the span that is left.
	rows, spans, _, _ := renderStyled(toolFrame("[fs] "+strings.Repeat("w", 100), toolTag("fs", true)), 24, 20)
	for i, row := range rows {
		for _, sp := range spans[i] {
			if sp.end > len(row) {
				t.Errorf("span %+v runs past the row %q", sp, row)
			}
		}
	}
}

func TestAPartialAndTheEntryItBecomesTakeTheSameSpans(t *testing.T) {
	text := "[fs] pretend (error) a model said this"
	streaming := Frame{Reply: []string{"> q"}, Kinds: []entryKind{{}}, Partial: text}
	done := Frame{Reply: []string{"> q", text}, Kinds: []entryKind{{}, {kind: kindReply}}}
	_, a, _, _ := renderStyled(streaming, 24, 80)
	_, b, _, _ := renderStyled(done, 24, 80)
	for i := range a {
		if len(a[i]) != len(b[i]) {
			t.Fatalf("row %d: %d spans while streaming, %d when finished", i, len(a[i]), len(b[i]))
		}
		for j := range a[i] {
			if a[i][j] != b[i][j] {
				t.Errorf("row %d changed color when the stream ended", i)
			}
		}
	}
	for i := range b {
		for _, sp := range b[i] {
			if sp.role != roleChrome && sp.role != roleTitle {
				t.Errorf("a reply row was colored: row %d %+v", i, sp)
			}
		}
	}
}

// drawToolCall tags the line it writes with the tool and the outcome.
func TestDrawToolCallRecordsTheToolAndTheOutcome(t *testing.T) {
	s := kindSession(t)
	s.tools = &toolSet{labels: map[string]string{"read_thing": "fs", "log_thing": "git", "run_thing": "shell"}}
	call := func(name string) openrouter.ToolCall {
		return openrouter.ToolCall{Function: openrouter.ToolCallFunction{Name: name, Arguments: "{}"}}
	}
	s.drawToolCall(tools.Result{Call: call("read_thing"), Text: "abc"})
	s.drawToolCall(tools.Result{Call: call("log_thing"), Err: errString("no repo")})
	s.drawToolCall(tools.Result{Call: call("run_thing"), Err: errString("refused")})
	s.drawToolCall(tools.Result{Call: call("unknown_thing")})
	inStep(t, s, "after the calls")

	want := []entryKind{
		{kind: kindTool, tool: roleFilesystem, ok: true},
		{kind: kindTool, tool: roleGit, ok: false},
		{kind: kindTool, tool: roleShell, ok: false},
		{kind: kindTool, tool: noRole, ok: true},
	}
	for i, w := range want {
		if s.frame.Kinds[i] != w {
			t.Errorf("call %d is recorded as %+v, want %+v", i, s.frame.Kinds[i], w)
		}
	}
	for i, line := range s.frame.Reply {
		if strings.ContainsRune(line, 0x1b) {
			t.Errorf("entry %d holds an escape: %q", i, line)
		}
	}
}

type errString string

func (e errString) Error() string { return string(e) }

func TestErrorAndStopLinesAreRecordedAsFailures(t *testing.T) {
	s := kindSession(t)
	s.command("/nosuchcommand")
	for i, e := range s.frame.Reply {
		if strings.HasPrefix(e, "unknown command:") && s.frame.Kinds[i].kind != kindFailure {
			t.Errorf("%q is kind %d, want failure", e, s.frame.Kinds[i].kind)
		}
	}
	inStep(t, s, "after an unknown command")
	if len(s.frame.Reply) == 0 {
		t.Fatal("the command wrote nothing")
	}
}

// With color off the bytes are what they were before the records existed, and
// with color on the text with the sequences stripped is the same text.
func TestKindsLeaveTheColorOffBytesAndTheTextAlone(t *testing.T) {
	f := toolFrame(
		"> hi", entryKind{},
		"[fs] read_file a -> 3 bytes", toolTag("fs", false),
		"[shell] run x -> failed: no", toolTag("shell", true),
		"(error) boom", entryKind{kind: kindFailure},
		"a notice", entryKind{kind: kindNotice},
		"model", entryKind{kind: kindReply},
	)
	bare := Frame{Reply: f.Reply}
	height, width := 24, 60
	rows, spans, _, _ := renderStyled(f, height, width)
	bareRows, _, _, _ := renderStyled(bare, height, width)
	if strings.Join(rows, "\n") != strings.Join(bareRows, "\n") {
		t.Fatal("records changed the row text")
	}

	sc, read := screenCapture(t)
	sc.height, sc.width = height, width
	sc.DrawFrame(rows, framePaint{spans: spans, twiddle: -1})
	off := read()
	if want := oldFrame(rows, height); !strings.HasPrefix(off, want) {
		t.Errorf("color off is not the old bytes\n got %q\nwant %q...", off, want)
	}

	pal := newPalette(config.Theme{})
	sc2, read2 := screenCapture(t)
	sc2.height, sc2.width = height, width
	sc2.DrawFrame(rows, framePaint{spans: spans, pal: &pal, twiddle: -1})
	on := read2()
	if csi.ReplaceAllString(on, "") != csi.ReplaceAllString(off, "") {
		t.Error("the text with color stripped is not the color-off text")
	}
	if on == off {
		t.Error("color on drew nothing for tool, failure and notice lines")
	}
	for _, row := range rows {
		if strings.ContainsRune(row, 0x1b) {
			t.Errorf("a row holds an escape: %q", row)
		}
	}
}
