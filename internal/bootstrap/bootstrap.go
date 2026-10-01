// Package bootstrap loads the bootstrap document that starts a session.
//
// A bootstrap document is either Markdown or JSON, selected by file extension.
// Markdown is taken as-is, since it is already prose meant for the model. JSON
// is decoded into a structured document, which permits fields the Markdown form
// has no way to express.
package bootstrap

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"
)

// Format is the on-disk format of a bootstrap document.
type Format string

// The supported formats.
const (
	// FormatMarkdown is a Markdown document used verbatim.
	FormatMarkdown Format = "markdown"
	// FormatJSON is a JSON document decoded into a Document.
	FormatJSON Format = "json"
)

// ErrUnsupportedFormat reports an extension that names no known format.
//
// The extension is the only structural signal available, since the content of
// a file cannot be probed without guessing, and a guess that is wrong is worse
// than a refusal.
var ErrUnsupportedFormat = errors.New("unsupported bootstrap format")

// ErrEmpty reports a document that carries no instructions.
//
// An empty document is refused rather than passed on, because a session begun
// with no instructions is a silent failure the user would not notice until the
// model behaved as though it had never been told anything.
var ErrEmpty = errors.New("bootstrap document is empty")

// Document is a decoded bootstrap.
type Document struct {
	// Instructions is the text handed to the model as the opening context.
	Instructions string `json:"instructions"`
	// Name is an optional label for the document.
	Name string `json:"name,omitempty"`
	// Description is an optional human-readable summary.
	Description string `json:"description,omitempty"`

	// Path is the file the document was read from.
	Path string `json:"-"`
	// Format is the format the file was read as.
	Format Format `json:"-"`
}

// formatFor resolves a path extension to a format.
func formatFor(path string) (Format, error) {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".md", ".markdown":
		return FormatMarkdown, nil
	case ".json":
		return FormatJSON, nil
	}
	// The path is named rather than the extension, since a bare extension says
	// nothing about which file was refused.
	if filepath.Ext(path) == "" {
		return "", fmt.Errorf("%w: %s has no extension, and the extension alone "+
			"selects the format", ErrUnsupportedFormat, path)
	}
	return "", fmt.Errorf("%w: %s, which is not a known extension", ErrUnsupportedFormat, filepath.Ext(path))
}

// Load reads the bootstrap document at path.
//
// An explicit path is an assertion by the caller that the file exists and is
// readable, so both a missing file and an unreadable one are reported as
// errors. This differs from the automatic instruction search, where an absent
// file is an ordinary outcome and is not an error.
func Load(path string) (*Document, error) {
	format, err := formatFor(path)
	if err != nil {
		return nil, err
	}

	target, err := resolve(path)
	if err != nil {
		return nil, err
	}

	data, err := os.ReadFile(target)
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", path, err)
	}

	// Instructions the user believed were in effect must not be silently
	// discarded, so content that is not text is refused.
	if !utf8.Valid(data) {
		return nil, fmt.Errorf("%s is not valid UTF-8", path)
	}

	doc := &Document{Path: path, Format: format}
	if format == FormatMarkdown {
		doc.Instructions = string(data)
	} else {
		if err := json.Unmarshal(data, doc); err != nil {
			return nil, fmt.Errorf("%s is not valid JSON: %w", path, err)
		}
		doc.Path = path
		doc.Format = format
	}

	if strings.TrimSpace(doc.Instructions) == "" {
		return nil, fmt.Errorf("%w: %s", ErrEmpty, path)
	}
	return doc, nil
}
