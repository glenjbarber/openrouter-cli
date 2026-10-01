package main

import (
	"errors"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseFlagsBootstrap(t *testing.T) {
	opts, err := parseFlags([]string{"--bootstrap", "MEMORY.md"})
	if err != nil {
		t.Fatalf("parseFlags: %v", err)
	}
	if opts.bootstrap != "MEMORY.md" {
		t.Errorf("bootstrap = %q, want %q", opts.bootstrap, "MEMORY.md")
	}
}

// The flag must stop at the first non-flag word, so a bootstrap path is never
// mistaken for a subcommand.
func TestParseFlagsBootstrapThenCommand(t *testing.T) {
	opts, err := parseFlags([]string{"--bootstrap", "MEMORY.md", "version"})
	if err != nil {
		t.Fatalf("parseFlags: %v", err)
	}
	if opts.bootstrap != "MEMORY.md" {
		t.Errorf("bootstrap = %q, want %q", opts.bootstrap, "MEMORY.md")
	}
	if !opts.showVersion {
		t.Error("showVersion = false, want true")
	}
}

func TestParseFlagsSubcommands(t *testing.T) {
	for _, tc := range []struct {
		args []string
		want func(options) bool
		name string
	}{
		{[]string{"version"}, func(o options) bool { return o.showVersion }, "version"},
		{[]string{"-version"}, func(o options) bool { return o.showVersion }, "-version"},
		{[]string{"--version"}, func(o options) bool { return o.showVersion }, "--version"},
		{[]string{"help"}, func(o options) bool { return o.showHelp }, "help"},
		{[]string{"-help"}, func(o options) bool { return o.showHelp }, "-help"},
		{[]string{"--help"}, func(o options) bool { return o.showHelp }, "--help"},
	} {
		opts, err := parseFlags(tc.args)
		if err != nil {
			t.Errorf("%s: parseFlags: %v", tc.name, err)
			continue
		}
		if !tc.want(opts) {
			t.Errorf("%s: options = %+v, want the flag set", tc.name, opts)
		}
	}
}

func TestParseFlagsRejectsUnknownCommand(t *testing.T) {
	_, err := parseFlags([]string{"bogus"})
	if err == nil {
		t.Fatal("parseFlags accepted an unknown command, want an error")
	}
	if !strings.Contains(err.Error(), "bogus") {
		t.Errorf("err = %q, want it to name the command", err)
	}
}

func TestParseFlagsRejectsUnknownFlag(t *testing.T) {
	_, err := parseFlags([]string{"--bogus"})
	if !errors.Is(err, flag.ErrHelp) && err == nil {
		t.Fatal("parseFlags accepted an unknown flag, want an error")
	}
}

func TestParseFlagsNoArgs(t *testing.T) {
	opts, err := parseFlags(nil)
	if err != nil {
		t.Fatalf("parseFlags: %v", err)
	}
	if opts.bootstrap != "" || opts.showHelp || opts.showVersion {
		t.Errorf("options = %+v, want all zero", opts)
	}
}

// An unreadable bootstrap document is reported before any credential work, so
// the failure names the document rather than the configuration.
func TestRunReportsBadBootstrapFirst(t *testing.T) {
	dir := t.TempDir()
	// An extension naming no format is refused by the loader.
	path := filepath.Join(dir, "MEMORY.txt")

	err := run([]string{"--bootstrap", path})
	if err == nil {
		t.Fatal("run accepted an unsupported extension, want an error")
	}
	if !strings.Contains(err.Error(), "unsupported bootstrap format") {
		t.Errorf("err = %q, want it to name the format", err)
	}
}

// A bootstrap document that cannot be read is an error, and the failure names
// the document rather than the configuration.
func TestRunReportsMissingBootstrap(t *testing.T) {
	path := filepath.Join(t.TempDir(), "absent.md")

	err := run([]string{"--bootstrap", path})
	if err == nil {
		t.Fatal("run accepted a missing document, want an error")
	}
	if !errors.Is(err, os.ErrNotExist) {
		t.Errorf("err = %v, want a not-exist error", err)
	}
}
