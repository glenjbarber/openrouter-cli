package bootstrap

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The extension alone selects the format, so a file whose content is JSON but
// whose name says Markdown is passed through as prose rather than decoded. A
// guess from the content is worse than a refusal.
func TestFormatIsNotGuessedFromTheContent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "MEMORY.md")
	body := `{"instructions": "Answer in the third person."}`
	write(t, path, body)

	doc, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if doc.Format != FormatMarkdown {
		t.Errorf("Format = %q, want %q", doc.Format, FormatMarkdown)
	}
	if doc.Instructions != body {
		t.Errorf("Instructions = %q, want the content verbatim", doc.Instructions)
	}
}

// A file with no extension cannot name a format, and the diagnostic names the
// file rather than reporting a bare empty extension.
func TestNoExtensionNamesTheFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "MEMORY")
	write(t, path, "Be terse.")

	_, err := Load(path)
	if !errors.Is(err, ErrUnsupportedFormat) {
		t.Fatalf("err = %v, want ErrUnsupportedFormat", err)
	}
	if !strings.Contains(err.Error(), path) {
		t.Errorf("err = %q, want it to name the file %q", err, path)
	}
}

// An unknown extension is reported as such, and the file is never read, since
// reading it would mean the content had been consulted about the format.
func TestUnknownExtensionIsNotRead(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "MEMORY.txt")
	write(t, path, "Be terse.")

	_, err := Load(path)
	if !errors.Is(err, ErrUnsupportedFormat) {
		t.Fatalf("err = %v, want ErrUnsupportedFormat", err)
	}
	if !strings.Contains(err.Error(), ".txt") {
		t.Errorf("err = %q, want it to name the extension", err)
	}
}

// A JSON document with no instructions field is refused. A session begun with
// none would fail silently until the model behaved as though it had never been
// told anything.
func TestJSONWithoutInstructionsIsRefused(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "memory.json")
	write(t, path, `{"name":"terse","description":"No instructions field."}`)

	_, err := Load(path)
	if !errors.Is(err, ErrEmpty) {
		t.Errorf("err = %v, want ErrEmpty", err)
	}
}

// An empty JSON object is refused as well.
func TestEmptyJSONObjectIsRefused(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "memory.json")
	write(t, path, `{}`)

	_, err := Load(path)
	if !errors.Is(err, ErrEmpty) {
		t.Errorf("err = %v, want ErrEmpty", err)
	}
}

// The Markdown form is passed on as written, so a document holding fenced code
// and unusual whitespace survives intact rather than being reflowed.
func TestMarkdownIsPassedVerbatim(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "MEMORY.md")
	body := "# Title\n\n\tindented\n\n```go\nfunc main() {}\n```\n\nno trailing newline"
	write(t, path, body)

	doc, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if doc.Instructions != body {
		t.Errorf("Instructions = %q, want the file verbatim", doc.Instructions)
	}
}

// The document is read whole, and a large one is not refused for its size. The
// file is named by the user, so its size is their decision.
func TestLargeDocumentIsRead(t *testing.T) {
	if testing.Short() {
		t.Skip("builds a large document")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "MEMORY.md")
	body := strings.Repeat("a", 1<<20)
	write(t, path, body)

	doc, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(doc.Instructions) != len(body) {
		t.Errorf("Instructions = %d bytes, want %d", len(doc.Instructions), len(body))
	}
}

// An invalid UTF-8 sequence is refused in the JSON form as well as the
// Markdown form, since both reach the model as text.
func TestJSONRejectsInvalidUTF8(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "memory.json")
	if err := os.WriteFile(path, []byte(`{"instructions":"`+"\xff"+`"}`), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	_, err := Load(path)
	if err == nil {
		t.Fatal("Load accepted invalid UTF-8, want an error")
	}
	if !strings.Contains(err.Error(), "not valid UTF-8") {
		t.Errorf("err = %q, want it to mention UTF-8", err)
	}
}

// A document whose instructions are only a JSON null or a number is a
// malformed document rather than an empty one, since the field is there but
// does not hold text.
func TestJSONWithNonTextInstructionsIsMalformed(t *testing.T) {
	for name, body := range map[string]string{
		"null":   `{"instructions":null}`,
		"number": `{"instructions":42}`,
		"array":  `{"instructions":["Be terse."]}`,
	} {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "memory.json")
			write(t, path, body)

			_, err := Load(path)
			if err == nil {
				t.Fatal("Load accepted a non-text instructions field, want an error")
			}
		})
	}
}

// The path reported is the one named, so a diagnostic points at the file the
// user typed rather than at a target reached by following a link.
func TestJSONReportsTheNamedPath(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "memory.json")
	write(t, path, `{"instructions":"`)

	_, err := Load(path)
	if err == nil {
		t.Fatal("Load accepted malformed JSON, want an error")
	}
	if !strings.Contains(err.Error(), path) {
		t.Errorf("err = %q, want it to name %q", err, path)
	}
}

// A two-hop chain of links is followed, and the reported path is still the one
// named. Every hop stays on the same filesystem here.
func TestTwoHopChainIsFollowed(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "real.md")
	first := filepath.Join(dir, "first.md")
	second := filepath.Join(dir, "MEMORY.md")
	write(t, target, "Be terse.")

	if err := os.Symlink("real.md", first); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if err := os.Symlink("first.md", second); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}

	doc, err := Load(second)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if doc.Instructions != "Be terse." {
		t.Errorf("Instructions = %q, want the chain followed to the target", doc.Instructions)
	}
	if doc.Path != second {
		t.Errorf("Path = %q, want the link the user named", doc.Path)
	}
}

// A link to itself is reported rather than followed.
func TestSelfReferentialLinkIsReported(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "MEMORY.md")
	if err := os.Symlink("MEMORY.md", path); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}

	_, err := Load(path)
	if err == nil {
		t.Fatal("Load followed a self-referential link, want an error")
	}
}

// A chain longer than the hop bound is refused rather than followed, so a long
// chain cannot spin. The bound is a backstop and not a policy, so the test
// accepts either refusal: the walk reporting the bound itself, or the kernel
// refusing to resolve the chain, which on most systems happens first.
func TestLongChainIsReported(t *testing.T) {
	dir := t.TempDir()
	last := filepath.Join(dir, fmt.Sprintf("hop%03d.md", maxHops+2))
	write(t, last, "Be terse.")

	prev := last
	for i := maxHops + 1; i >= 0; i-- {
		link := filepath.Join(dir, fmt.Sprintf("hop%03d.md", i))
		if err := os.Symlink(filepath.Base(prev), link); err != nil {
			t.Skipf("symlinks unavailable: %v", err)
		}
		prev = link
	}

	_, err := Load(prev)
	if err == nil {
		t.Fatal("Load followed a chain past the bound, want an error")
	}
	if !strings.Contains(err.Error(), "link chain") &&
		!strings.Contains(err.Error(), "symbolic links") {
		t.Errorf("err = %q, want the chain reported", err)
	}
}

// A missing target is reported as such rather than as an empty document.
func TestBrokenLinkIsNotAnEmptyDocument(t *testing.T) {
	dir := t.TempDir()
	link := filepath.Join(dir, "MEMORY.md")
	if err := os.Symlink("absent.md", link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}

	_, err := Load(link)
	if errors.Is(err, ErrEmpty) {
		t.Error("err = ErrEmpty, want the missing target reported")
	}
	if !errors.Is(err, os.ErrNotExist) {
		t.Errorf("err = %v, want a not-exist error", err)
	}
}
