package tui

import (
	"strings"
	"testing"
)

// Prose is folded rather than cut, since cutting loses whatever fell past the
// edge and for prose that is most of a paragraph.
func TestWrapTextFoldsProse(t *testing.T) {
	in := "the quick brown fox jumps over the lazy dog and keeps on running well past the edge"
	lines := wrapText(in, 20)

	if len(lines) < 2 {
		t.Fatalf("len(lines) = %d, want the text folded", len(lines))
	}
	for _, l := range lines {
		if runeLen(l) > 20 {
			t.Errorf("line %q is %d wide, want at most 20", l, runeLen(l))
		}
	}
	if got := strings.Join(lines, " "); got != in {
		t.Errorf("joined = %q, want the original text", got)
	}
}

// A word wider than the pane is moved whole rather than cut, since splitting
// an identifier across two rows is neither readable nor copyable.
func TestWrapTextKeepsLongWordWhole(t *testing.T) {
	url := "https://example.invalid/a/very/long/path/that/exceeds/the/pane/width"
	lines := wrapText("see "+url+" for details", 20)

	joined := strings.Join(lines, " ")
	if !strings.Contains(joined, url) {
		t.Errorf("joined = %q, want the URL intact", joined)
	}
	for _, l := range lines {
		if l != url && runeLen(l) > 20 && !strings.Contains(l, url) {
			t.Errorf("line %q is %d wide, want at most 20", l, runeLen(l))
		}
	}
}

// A fenced block is not folded, because reflowing code changes what it means.
func TestWrapTextLeavesCodeBlockAlone(t *testing.T) {
	code := "```go\n" + strings.Repeat("x", 100) + "\n```"
	lines := WrapBlock(code, 20)

	var inBlock bool
	found := false
	for _, l := range lines {
		if isFence(l) {
			inBlock = !inBlock
			continue
		}
		if inBlock && runeLen(l) == 100 {
			found = true
		}
	}
	if !found {
		t.Error("the code line was folded, want it left as written")
	}
}

// A fence marker inside prose must not be mistaken for a block opener.
func TestWrapTextFoldsProseAroundFences(t *testing.T) {
	in := "before the block ``` and after it, with enough words to need folding here"
	lines := wrapText(in, 20)
	if len(lines) < 2 {
		t.Fatalf("len(lines) = %d, want folding", len(lines))
	}
}

// A fence indented inside a list item is still recognised.
func TestWrapTextRecognisesIndentedFence(t *testing.T) {
	code := "  ```sh\n  echo hello there and some more words to exceed the width\n  ```"
	lines := WrapBlock(code, 20)
	if len(lines) != 3 {
		t.Errorf("len(lines) = %d, want 3, so the block was not folded", len(lines))
	}
}

// A reply that was cut short by a disconnect must not lose its tail.
func TestWrapTextKeepsUnclosedBlock(t *testing.T) {
	code := "```\nstill streaming"
	lines := WrapBlock(code, 20)
	if len(lines) < 2 {
		t.Errorf("len(lines) = %d, want the unclosed block emitted", len(lines))
	}
}

func TestWrapTextEmpty(t *testing.T) {
	if got := wrapText("", 20); len(got) != 1 || got[0] != "" {
		t.Errorf("wrapText(\"\") = %q, want one empty line", got)
	}
}

func TestWrapTextBlankPreserved(t *testing.T) {
	got := wrapText("one\n\ntwo", 20)
	if len(got) != 3 || got[1] != "" {
		t.Errorf("got %q, want the blank line kept", got)
	}
}

// Folding must not change the text, only where the rows break. The pane relies
// on this, since a dropped character would corrupt a copied reply.
func TestWrapTextPreservesEveryCharacter(t *testing.T) {
	in := "alpha beta gamma delta epsilon zeta eta theta iota kappa lambda mu"
	lines := wrapText(in, 15)
	stripped := strings.Join(lines, " ")
	for _, word := range strings.Fields(in) {
		if !strings.Contains(stripped, word) {
			t.Errorf("word %q is missing from the folded text", word)
		}
	}
}

// A multibyte character must not be split by folding, since a byte-based cut
// would leave half a rune on a row.
func TestWrapTextHandlesMultibyte(t *testing.T) {
	in := strings.Repeat("héllo wörld ", 10)
	for _, l := range wrapText(in, 12) {
		for _, r := range l {
			if r == 0xFFFD {
				t.Errorf("line %q contains a replacement character", l)
			}
		}
	}
}

// The rendered frame must carry the whole reply, which is the point of folding.
func TestRenderKeepsLongReplyText(t *testing.T) {
	long := strings.Repeat("word ", 60)
	lines := Render(Frame{Reply: []string{long}}, 40, 40)

	joined := strings.Join(lines, "\n")
	if strings.Contains(joined, "...") {
		t.Error("the frame truncated the reply, want it folded instead")
	}
}

// A list item holding a fenced block is two constructs, and folding the item
// must not reflow the block. A nested construct is where the two rules meet.
func TestWrapBlockKeepsAListItemWithABlockIntact(t *testing.T) {
	reply := strings.Join([]string{
		"- run the build like this",
		"  " + fence + "sh",
		"  go build ./... && echo done",
		"  " + fence,
	}, "\n")

	wantRows(t, WrapBlock(reply, 40), []string{
		"- run the build like this",
		fence + "sh",
		"  go build ./... && echo done",
		fence,
	})
}

// Prose carrying markers is folded to what is left beside its marker, so that
// the rows fit and the words survive the render.
func TestWrapBlockFoldsProseCarryingMarkers(t *testing.T) {
	reply := "The **first** step and the *second* step both need explaining in full detail here"
	rows := WrapBlock(reply, 20)

	for i, r := range rows {
		if n := runeLen(r); n > 20 {
			t.Errorf("row %d = %q is %d wide, want at most 20", i, r, n)
		}
		if strings.Contains(r, "*") {
			t.Errorf("row %d = %q still carries a marker", i, r)
		}
	}
	folded := strings.Join(rows, " ")
	for _, word := range []string{"first", "second", "explaining", "detail"} {
		if !strings.Contains(folded, word) {
			t.Errorf("the word %q is missing from the folded reply: %q", word, folded)
		}
	}
}

// The info string belongs to the fence rather than to the block, and it is
// carried whole. A hash in it is part of what was written rather than a heading.
func TestWrapBlockCarriesTheInfoStringWhole(t *testing.T) {
	reply := strings.Join([]string{
		"Before the block.",
		fence + "sh # not a heading",
		"echo one",
		fence,
	}, "\n")

	rows := WrapBlock(reply, 40)
	found := false
	for _, r := range rows {
		if strings.Contains(r, "sh #") {
			found = true
		}
	}
	if !found {
		t.Errorf("rows = %q, want the info string carried whole", rows)
	}
}
