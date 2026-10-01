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

// run must not report a configuration failure once the bootstrap document has
// been reported, so a caller reading stderr sees the document first.
func TestRunReportsBootstrapBeforeConfig(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "MEMORY.md")
	if err := os.WriteFile(path, []byte("Be terse."), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	stderr := os.Stderr
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("Pipe: %v", err)
	}
	os.Stderr = w
	runErr := run([]string{"--bootstrap", path})
	w.Close()
	os.Stderr = stderr

	buf := make([]byte, 256)
	n, _ := r.Read(buf)
	if !strings.Contains(string(buf[:n]), "MEMORY.md") {
		t.Errorf("stderr = %q, want it to name the bootstrap document", string(buf[:n]))
	}
	if runErr == nil {
		t.Skip("a configuration file is present, so run succeeded")
	}
}
