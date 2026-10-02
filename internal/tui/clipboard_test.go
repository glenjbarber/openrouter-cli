package tui

import (
	"encoding/base64"
	"os"
	"strings"
	"testing"

	"github.com/glenjbarber/openrouter-cli/internal/openrouter"
)

// clipboardSession builds a session writing into a file, so the bytes written
// can be read back without a terminal.
func clipboardSession(t *testing.T) (*Session, func() string) {
	t.Helper()
	out, err := os.CreateTemp(t.TempDir(), "clip")
	if err != nil {
		t.Fatalf("creating the capture: %v", err)
	}
	conv := NewConversation()
	return &Session{
			conv:      conv,
			mainConv:  conv,
			screen:    &Screen{out: out, in: out, height: 24, width: 80},
			spinner:   NewSpinner(),
			windows:   newContextLength(),
			approvals: newApprovalState(),
			tools:     &toolSet{dir: t.TempDir()},
		}, func() string {
			data, err := os.ReadFile(out.Name())
			if err != nil {
				t.Fatalf("reading the capture: %v", err)
			}
			return string(data)
		}
}

// The sequence is the one a terminal reads as a copy, and the text in it has to
// decode back to what was asked for.
func TestTheSequenceCarriesTheText(t *testing.T) {
	s, read := clipboardSession(t)
	s.conv.Record("what is the weather", "it is raining")

	s.screen.copyToClipboard("it is raining")
	got := read()

	if !strings.Contains(got, seqCopyPrefix) {
		t.Fatalf("no copy sequence was written: %q", got)
	}
	body := got[len(seqCopyPrefix):]
	if i := strings.Index(body, seqCopySuffix); i >= 0 {
		body = body[:i]
	}
	// The frame may follow, so only the encoded run is decoded.
	if i := strings.IndexAny(body, "\x1b"); i >= 0 {
		body = body[:i]
	}
	decoded, err := base64.StdEncoding.DecodeString(strings.TrimSpace(body))
	if err != nil {
		t.Fatalf("the payload is not base64: %v", err)
	}
	if string(decoded) != "it is raining" {
		t.Errorf("the terminal was sent %q, want the text", decoded)
	}
}

// The clipboard rather than the primary selection, which on a Unix-like system
// is what a middle click pastes.
func TestTheClipboardIsTheOneWritten(t *testing.T) {
	s, read := clipboardSession(t)

	s.screen.copyToClipboard("x")
	got := read()

	if !strings.Contains(got, "\x1b]52;c;") {
		t.Errorf("the selector is not the clipboard: %q", got)
	}
}

func TestTheConversationIsWhatIsCopied(t *testing.T) {
	s, _ := clipboardSession(t)
	s.conv.Record("a question", "an answer")

	text := s.copyText()

	if !strings.Contains(text, "a question") {
		t.Errorf("the question is missing:\n%s", text)
	}
	if !strings.Contains(text, "an answer") {
		t.Errorf("the answer is missing:\n%s", text)
	}
	// The turn is told apart from what answers it, or a reader pasting it
	// elsewhere cannot see who said what.
	if !strings.Contains(text, "> a question") {
		t.Errorf("the question is not marked as asked:\n%s", text)
	}
}

// A turn where the model called a tool carries the call, since that is part of
// what the reader asked for and what came back.
func TestAToolCallIsCopied(t *testing.T) {
	s, _ := clipboardSession(t)
	s.conv.RecordMessages([]openrouter.Message{
		{Role: openrouter.RoleUser, Content: "build it"},
		{
			Role: openrouter.RoleAssistant,
			ToolCalls: []openrouter.ToolCall{{
				Function: openrouter.ToolCallFunction{
					Name:      "shell",
					Arguments: `{"command":"go","args":["build","./..."]}`,
				},
			}},
		},
		{Role: openrouter.RoleTool, Content: "go build ./... ran and printed nothing."},
		{Role: openrouter.RoleAssistant, Content: "it builds."},
	})

	text := s.copyText()

	for _, want := range []string{"build it", "shell", "go build ./...", "it builds."} {
		if !strings.Contains(text, want) {
			t.Errorf("%q is missing from what was copied:\n%s", want, text)
		}
	}
}

// A multi-line result is folded, or it reads as a run of separate exchanges.
func TestAMultiLineResultIsFolded(t *testing.T) {
	s, _ := clipboardSession(t)
	s.conv.RecordMessages([]openrouter.Message{
		{Role: openrouter.RoleUser, Content: "build it"},
		{Role: openrouter.RoleTool, Content: "line one\nline two\nline three"},
	})

	text := s.copyText()

	if strings.Contains(text, "\nline two") {
		t.Errorf("a result was not folded onto one line:\n%s", text)
	}
}

func TestNothingToCopySaysSo(t *testing.T) {
	s, _ := clipboardSession(t)

	if s.cmdCopy(nil) {
		t.Error("the command asked the session to end")
	}
	// Nothing was written to the terminal, so there is no copy sequence.
}

func TestTheCopyIsReported(t *testing.T) {
	s, read := clipboardSession(t)
	s.conv.Record("a question", "an answer")

	s.cmdCopy(nil)

	if !strings.Contains(read(), seqCopyPrefix) {
		t.Error("no copy was written to the terminal")
	}
}

// A copy over the limit is refused rather than sent, since a reply of two
// gigabytes would be carried through the same pipe as the drawing.
func TestAnOverlongConversationIsRefused(t *testing.T) {
	s, read := clipboardSession(t)
	s.conv.Record("a question", strings.Repeat("x", clipboardLimit+1))

	s.cmdCopy(nil)

	if strings.Contains(read(), seqCopyPrefix) {
		t.Error("an overlong copy was written to the terminal")
	}
}
