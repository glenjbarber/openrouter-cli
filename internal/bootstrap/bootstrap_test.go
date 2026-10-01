package bootstrap

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadMarkdown(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "MEMORY.md")
	want := "Answer in the third person.\n"
	write(t, path, want)

	doc, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if doc.Instructions != want {
		t.Errorf("Instructions = %q, want %q", doc.Instructions, want)
	}
	if doc.Format != FormatMarkdown {
		t.Errorf("Format = %q, want %q", doc.Format, FormatMarkdown)
	}
	if doc.Path != path {
		t.Errorf("Path = %q, want %q", doc.Path, path)
	}
}

func TestLoadJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "memory.json")
	write(t, path, `{
		"instructions": "Be terse.",
		"name": "terse",
		"description": "Short answers only."
	}`)

	doc, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if doc.Instructions != "Be terse." {
		t.Errorf("Instructions = %q, want %q", doc.Instructions, "Be terse.")
	}
	if doc.Name != "terse" {
		t.Errorf("Name = %q, want %q", doc.Name, "terse")
	}
	if doc.Description != "Short answers only." {
		t.Errorf("Description = %q, want %q", doc.Description, "Short answers only.")
	}
	if doc.Format != FormatJSON {
		t.Errorf("Format = %q, want %q", doc.Format, FormatJSON)
	}
}

// A JSON document that omits the optional fields still loads, since only the
// instructions are required.
func TestLoadJSONMinimal(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "memory.json")
	write(t, path, `{"instructions": "Be terse."}`)

	doc, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if doc.Name != "" || doc.Description != "" {
		t.Errorf("optional fields = %q/%q, want both empty", doc.Name, doc.Description)
	}
}

// Unknown keys are ignored, so a document written for a newer version stays
// readable by an older one. This mirrors the configuration file.
func TestLoadJSONIgnoresUnknownKeys(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "memory.json")
	write(t, path, `{"instructions": "Be terse.", "future_field": 42}`)

	if _, err := Load(path); err != nil {
		t.Fatalf("Load: %v", err)
	}
}

func TestLoadRejectsUnknownExtension(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "MEMORY.txt")
	write(t, path, "instructions")

	_, err := Load(path)
	if !errors.Is(err, ErrUnsupportedFormat) {
		t.Errorf("err = %v, want ErrUnsupportedFormat", err)
	}
}

// The extension is compared without regard to case, so MEMORY.MD is accepted.
func TestLoadAcceptsUppercaseExtension(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "MEMORY.MD")
	write(t, path, "Be terse.")

	if _, err := Load(path); err != nil {
		t.Fatalf("Load: %v", err)
	}
}

func TestLoadRejectsMissingFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "absent.md")

	_, err := Load(path)
	if !errors.Is(err, os.ErrNotExist) {
		t.Errorf("err = %v, want os.ErrNotExist", err)
	}
}

// Content that is not text is refused rather than passed on, since silently
// discarding instructions the user believed were active is worse.
func TestLoadRejectsInvalidUTF8(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bad.md")
	if err := os.WriteFile(path, []byte{0xff, 0xfe, 0x00}, 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	_, err := Load(path)
	if err == nil {
		t.Fatal("Load accepted invalid UTF-8, want an error")
	}
	if got := err.Error(); !strings.Contains(got, "not valid UTF-8") {
		t.Errorf("err = %q, want it to mention UTF-8", got)
	}
}

func TestLoadRejectsEmptyMarkdown(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "empty.md")
	write(t, path, "   \n\t\n")

	_, err := Load(path)
	if !errors.Is(err, ErrEmpty) {
		t.Errorf("err = %v, want ErrEmpty", err)
	}
}

func TestLoadRejectsEmptyJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "empty.json")
	write(t, path, `{"instructions": "  "}`)

	_, err := Load(path)
	if !errors.Is(err, ErrEmpty) {
		t.Errorf("err = %v, want ErrEmpty", err)
	}
}

func TestLoadRejectsMalformedJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bad.json")
	write(t, path, `{"instructions": `)

	_, err := Load(path)
	if err == nil {
		t.Fatal("Load accepted malformed JSON, want an error")
	}
	if got := err.Error(); !strings.Contains(got, "not valid JSON") {
		t.Errorf("err = %q, want it to mention JSON", got)
	}
}

func write(t *testing.T, path, body string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
}
