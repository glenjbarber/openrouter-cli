package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/glenjbarber/openrouter-cli/internal/config"
)

// The bell must not be wanted unless asked for, since a user who did not ask
// for one would find it startling.
func TestBellOffByDefault(t *testing.T) {
	s := &Session{}
	if s.bellWanted {
		t.Error("bellWanted = true on a fresh session")
	}
}

func TestToggleBell(t *testing.T) {
	s := &Session{}
	s.toggleBell()
	if !s.bellWanted {
		t.Error("toggle did not turn the bell on")
	}
	s.toggleBell()
	if s.bellWanted {
		t.Error("toggle did not turn the bell off")
	}
}

// The bell is written only to a terminal, since a redirected run has no
// terminal to ring and the byte would be noise in the capture.
func TestRingBellSkipsNonTerminal(t *testing.T) {
	path := filepath.Join(t.TempDir(), "out")
	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	defer f.Close()

	ringBell(f)

	data, _ := os.ReadFile(path)
	if strings.Contains(string(data), bellFile) {
		t.Errorf("output = %q, want no bell for a redirected run", string(data))
	}
}

func TestRingBellSkipsNil(t *testing.T) {
	ringBell(nil)
}

// loadConfigAt reads a configuration file through the loader, with the home
// directory pointed at the file so that the exported path is exercised rather
// than the internals.
func loadConfigAt(t *testing.T, path string) *config.Config {
	t.Helper()

	home := t.TempDir()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	// The loader searches by a fixed name, so the file is written under the
	// name it will look for rather than the name it was built with.
	if err := os.WriteFile(filepath.Join(home, ".openrouter-cli.json"), data, 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	t.Setenv("HOME", home)

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	return cfg
}

// The preference is read from the configuration file, so it survives a run.
func TestBellFromConfig(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "cfg.json")
	body := `{"OPENROUTER_API_KEY":"k","OPENROUTER_BELL":true}`
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	cfg := loadConfigAt(t, path)
	if !cfg.Bell {
		t.Error("Bell = false, want it read from the file")
	}
}

// A file without the key is not an error, since the bell is a preference.
func TestBellAbsentIsOff(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "cfg.json")
	if err := os.WriteFile(path, []byte(`{"OPENROUTER_API_KEY":"k"}`), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	cfg := loadConfigAt(t, path)
	if cfg.Bell {
		t.Error("Bell = true, want it off when the file does not say")
	}
}

// The preference must survive a file that carries no credential, since a file
// holding a preference is not unread for want of a key.
func TestBellSurvivesMissingKey(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "cfg.json")
	body := `{"OPENROUTER_BELL":true}`
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	cfg := config.Empty("some/model", true)
	if !cfg.Bell {
		t.Error("Bell = false, want the preference kept")
	}
}
