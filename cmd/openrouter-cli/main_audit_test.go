package main

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/glenjbarber/openrouter-cli/internal/bootstrap"
	"github.com/glenjbarber/openrouter-cli/internal/config"
)

// isolateHome points the loader at a temporary home directory and clears the
// XDG variable, so a test never reads or writes the maintainer's own
// configuration.
func isolateHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("OPENROUTER_API_KEY", "")
	return home
}

// A bad flag is reported once. The flag set used to write its own message to
// stderr and return the same error, which the caller then printed again, so one
// mistake reached the reader twice.
func TestBadFlagIsReportedOnce(t *testing.T) {
	isolateHome(t)

	_, err := parseFlags([]string{"--bogus"})
	if err == nil {
		t.Fatal("parseFlags accepted an unknown flag, want an error")
	}
	if !strings.Contains(err.Error(), "-bogus") {
		t.Errorf("err = %q, want it to name the flag", err)
	}
}

// The error carries no output attached, so printing it once cannot produce a
// doubled diagnostic. A bare wording is what lets the caller decide the format.
func TestBadFlagErrorIsWording(t *testing.T) {
	isolateHome(t)

	_, err := parseFlags([]string{"--bogus"})
	if err == nil {
		t.Fatal("parseFlags accepted an unknown flag, want an error")
	}
	if strings.Contains(err.Error(), "Usage of") {
		t.Errorf("err = %q, want the wording alone", err)
	}
}

// The flag set writes nothing itself, so the single diagnostic the caller
// prints is the only one. The flag set used to write its own message to stderr
// and return the same error, which the caller then printed again.
func TestBadFlagWritesNothingToStderr(t *testing.T) {
	isolateHome(t)

	captured := captureStderr(t, func() {
		if _, err := parseFlags([]string{"--bogus"}); err == nil {
			t.Error("parseFlags accepted an unknown flag, want an error")
		}
	})
	if captured != "" {
		t.Errorf("stderr = %q, want nothing written by the flag set", captured)
	}
}

// captureStderr collects what fn writes to os.Stderr for the duration of the
// call. The file is restored before the test ends, since the testing package
// reports through it.
func captureStderr(t *testing.T, fn func()) string {
	t.Helper()

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("Pipe: %v", err)
	}
	saved := os.Stderr
	os.Stderr = w

	done := make(chan string, 1)
	go func() {
		var b strings.Builder
		buf := make([]byte, 4096)
		for {
			n, err := r.Read(buf)
			b.Write(buf[:n])
			if err != nil {
				break
			}
		}
		done <- b.String()
	}()

	fn()

	os.Stderr = saved
	w.Close()
	out := <-done
	r.Close()
	return out
}

// An unknown command names itself and points at the help, since a bare word is
// the one mistake that carries no flag name to identify it.
func TestUnknownCommandPointsAtHelp(t *testing.T) {
	isolateHome(t)

	_, err := parseFlags([]string{"bogus"})
	if err == nil {
		t.Fatal("parseFlags accepted an unknown command, want an error")
	}
	if !strings.Contains(err.Error(), "-help") {
		t.Errorf("err = %q, want it to point at the help", err)
	}
}

// A bootstrap document is loaded before any credential work, so an unreadable
// document is reported without a configuration file having been written. The
// order is what makes the failure name the document rather than the credential.
func TestBootstrapIsLoadedBeforeTheConfiguration(t *testing.T) {
	home := isolateHome(t)
	path := filepath.Join(t.TempDir(), "MEMORY.txt")

	err := run([]string{"--bootstrap", path})
	if !errors.Is(err, bootstrap.ErrUnsupportedFormat) {
		t.Fatalf("err = %v, want the format refused", err)
	}

	if _, err := os.Stat(filepath.Join(home, config.DefaultFileName)); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("Stat = %v, want no configuration written before the document is read", err)
	}
}

// A readable document with an unusable form is still refused before the
// configuration is touched, which is the case the ordering exists for.
func TestUnreadableDocumentPrecedesConfiguration(t *testing.T) {
	home := isolateHome(t)
	path := filepath.Join(t.TempDir(), "absent.md")

	if err := run([]string{"--bootstrap", path}); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("err = %v, want a not-exist error", err)
	}
	if _, err := os.Stat(filepath.Join(home, config.DefaultFileName)); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("Stat = %v, want no configuration written", err)
	}
}

// The version and the help are printed without a configuration being written,
// since neither starts a session.
func TestVersionAndHelpWriteNoConfiguration(t *testing.T) {
	for name, args := range map[string][]string{
		"version": {"-version"},
		"help":    {"-help"},
	} {
		t.Run(name, func(t *testing.T) {
			home := isolateHome(t)

			if err := run(args); err != nil {
				t.Fatalf("run: %v", err)
			}
			if _, err := os.Stat(filepath.Join(home, config.DefaultFileName)); !errors.Is(err, os.ErrNotExist) {
				t.Errorf("Stat = %v, want no configuration written for %v", err, args)
			}
		})
	}
}

// A file that cannot be read for its key still stands in a configuration, so
// the interface opens and reports the absence rather than refusing to start.
func TestMissingKeyDoesNotStopTheSession(t *testing.T) {
	home := isolateHome(t)
	path := filepath.Join(home, config.DefaultFileName)
	body := `{"OPENROUTER_MODEL":"a/b","OPENROUTER_MOUSE":true}`
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	// The interface needs a terminal, so the run stops there. What matters is
	// that it got that far rather than failing on the missing key.
	err := run(nil)
	if err == nil {
		t.Fatal("run entered the interface without a terminal, want the terminal refused")
	}
	if errors.Is(err, os.ErrNotExist) {
		t.Fatalf("err = %v, want the missing key tolerated", err)
	}
	if !strings.Contains(err.Error(), "terminal") {
		t.Errorf("err = %q, want it to name the terminal rather than the key", err)
	}
}

// Mouse reporting is asked for by either the flag or the file, and neither is
// honoured inside tmux, since the drag that begins a selection is the gesture a
// nested terminal is most often used for.
func TestMouseWanted(t *testing.T) {
	for _, tc := range []struct {
		flag, file, tmux bool
		name             string
		want             bool
	}{
		{false, false, false, "neither", false},
		{true, false, false, "flag only", true},
		{false, true, false, "file only", true},
		{true, true, false, "both", true},
		{true, true, true, "both inside tmux", false},
		{true, false, true, "flag inside tmux", false},
		{false, true, true, "file inside tmux", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if tc.tmux {
				t.Setenv("TMUX", "/tmp/tmux-1000/default,1,0")
			} else {
				t.Setenv("TMUX", "")
			}
			opts := options{mouse: tc.flag}
			cfg := &config.Config{Mouse: tc.file}

			if got := mouseWanted(opts, cfg); got != tc.want {
				t.Errorf("mouseWanted = %v, want %v", got, tc.want)
			}
		})
	}
}

// An explicit false is not the same as an absent flag, so a file asking for the
// mouse is honoured whether or not the command line mentions it.
func TestMouseFlagDefaultsDoNotOverrideTheFile(t *testing.T) {
	t.Setenv("TMUX", "")

	opts, err := parseFlags(nil)
	if err != nil {
		t.Fatalf("parseFlags: %v", err)
	}
	cfg := &config.Config{Mouse: true}

	if !mouseWanted(opts, cfg) {
		t.Error("mouseWanted = false, want the file honoured when no flag was given")
	}
}

// The bootstrap path is taken exactly as named and is not completed or
// resolved, since the extension is what selects the format and a rewritten
// path could change it.
func TestBootstrapPathIsTakenAsNamed(t *testing.T) {
	opts, err := parseFlags([]string{"--bootstrap", "./notes.MD"})
	if err != nil {
		t.Fatalf("parseFlags: %v", err)
	}
	if opts.bootstrap != "./notes.MD" {
		t.Errorf("bootstrap = %q, want the path as named", opts.bootstrap)
	}
}

// An empty bootstrap path names no document, so no document is loaded and a
// plain run starts without one.
func TestEmptyBootstrapPathNamesNoDocument(t *testing.T) {
	opts, err := parseFlags([]string{"--bootstrap", ""})
	if err != nil {
		t.Fatalf("parseFlags: %v", err)
	}
	if opts.bootstrap != "" {
		t.Errorf("bootstrap = %q, want empty", opts.bootstrap)
	}
}
