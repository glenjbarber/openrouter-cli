package config

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInstallDefaultWritesFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), DefaultFileName)

	written, err := InstallDefaultAt(path)
	if err != nil {
		t.Fatalf("InstallDefaultAt: %v", err)
	}
	if !written {
		t.Fatal("written = false, want true")
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(data) != DefaultFile {
		t.Errorf("file = %q, want %q", string(data), DefaultFile)
	}
}

// The loader refuses any mode but 0600, so a default written at a permissive
// mode would be rejected by the client that wrote it.
func TestInstallDefaultModeIsRequired(t *testing.T) {
	path := filepath.Join(t.TempDir(), DefaultFileName)

	if _, err := InstallDefaultAt(path); err != nil {
		t.Fatalf("InstallDefaultAt: %v", err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	if got := info.Mode().Perm(); got != RequiredMode {
		t.Errorf("mode = %04o, want %04o", got, RequiredMode)
	}
}

// The default must itself be loadable, or the client writes a file it then
// rejects. A key is deliberately absent, so Load reports it as unset rather
// than as a configuration error.
func TestInstallDefaultIsLoadable(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, DefaultFileName)

	if _, err := InstallDefaultAt(path); err != nil {
		t.Fatalf("InstallDefaultAt: %v", err)
	}

	// Load validates the mode and the JSON before it looks at the key, so a
	// key error here means both of those passed.
	_, err := parse(path)
	if err == nil {
		t.Fatal("parse accepted a file with no key, want a key error")
	}
	if want := "does not contain OPENROUTER_API_KEY"; !strings.Contains(err.Error(), want) {
		t.Errorf("err = %q, want it to mention %q", err, want)
	}
}

// An existing file is never overwritten, and never merged. A file holding a
// real credential must survive untouched.
func TestInstallDefaultLeavesExistingFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), DefaultFileName)
	existing := `{"OPENROUTER_API_KEY": "sk-or-v1-secret"}`
	if err := os.WriteFile(path, []byte(existing), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	written, err := InstallDefaultAt(path)
	if err != nil {
		t.Fatalf("InstallDefaultAt: %v", err)
	}
	if written {
		t.Error("written = true, want false for an existing file")
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(data) != existing {
		t.Errorf("file = %q, want it unchanged", string(data))
	}
}

// A file that is empty, malformed, or holds an unrelated key is still left
// alone. Deciding what a merge would mean is a question for the user, not
// something to be settled by rewriting their file.
func TestInstallDefaultDoesNotRepairUnusableFile(t *testing.T) {
	for name, body := range map[string]string{
		"empty":     "",
		"malformed": "{not json",
		"no key":    `{"OPENROUTER_URL_BASE": "http://localhost:3000"}`,
	} {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), DefaultFileName)
			if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
				t.Fatalf("WriteFile: %v", err)
			}

			written, err := InstallDefaultAt(path)
			if err != nil {
				t.Fatalf("InstallDefaultAt: %v", err)
			}
			if written {
				t.Error("written = true, want false")
			}

			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("ReadFile: %v", err)
			}
			if string(data) != body {
				t.Errorf("file = %q, want it unchanged", string(data))
			}
		})
	}
}

// A permissive mode on an existing file is reported rather than corrected,
// since changing the mode of a file the user hand-edited is not this
// function's business. The loader reports it on the next run instead.
func TestInstallDefaultLeavesPermissiveMode(t *testing.T) {
	path := filepath.Join(t.TempDir(), DefaultFileName)
	if err := os.WriteFile(path, []byte(`{"OPENROUTER_API_KEY": "k"}`), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	if _, err := InstallDefaultAt(path); err != nil {
		t.Fatalf("InstallDefaultAt: %v", err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	if got := info.Mode().Perm(); got != 0o644 {
		t.Errorf("mode = %04o, want it left at 0644", got)
	}
}

// A directory in place of the file is an error rather than a silent skip,
// since the path cannot be used for the configuration.
func TestInstallDefaultRejectsDirectory(t *testing.T) {
	path := filepath.Join(t.TempDir(), DefaultFileName)
	if err := os.Mkdir(path, 0o700); err != nil {
		t.Fatalf("Mkdir: %v", err)
	}

	if _, err := InstallDefaultAt(path); err == nil {
		t.Fatal("InstallDefaultAt accepted a directory, want an error")
	}
}

// A parent directory that does not exist is reported, since the default is not
// created at a path the user did not ask for.
func TestInstallDefaultReportsMissingParent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "absent", DefaultFileName)

	_, err := InstallDefaultAt(path)
	if !errors.Is(err, os.ErrNotExist) {
		t.Errorf("err = %v, want a not-exist error", err)
	}
}

// A file holding no key is reported through the typed error, so the interface
// may open and explain rather than refusing to start.
func TestParseReportsMissingKeyAsTypedError(t *testing.T) {
	path := filepath.Join(t.TempDir(), DefaultFileName)
	if err := os.WriteFile(path, []byte(`{"OPENROUTER_URL_BASE":"x"}`), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	_, err := parse(path)
	var noKey *ErrNoAPIKey
	if !errors.As(err, &noKey) {
		t.Fatalf("err = %v, want *ErrNoAPIKey", err)
	}
	if noKey.Path != path {
		t.Errorf("Path = %q, want %q", noKey.Path, path)
	}
}

// Malformed JSON is a fault rather than an ordinary state, so it must not be
// reported as a missing key.
func TestParseDoesNotReportMalformedAsMissingKey(t *testing.T) {
	path := filepath.Join(t.TempDir(), DefaultFileName)
	if err := os.WriteFile(path, []byte(`{nope`), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	_, err := parse(path)
	var noKey *ErrNoAPIKey
	if errors.As(err, &noKey) {
		t.Errorf("err = %v, want a parse error rather than a missing key", err)
	}
}

// Empty returns a usable configuration so the interface has an endpoint even
// with no credential.
func TestEmptyHasDefaultEndpoint(t *testing.T) {
	cfg := Empty("", false)
	if cfg.URLBase != DefaultURLBase {
		t.Errorf("URLBase = %q, want %q", cfg.URLBase, DefaultURLBase)
	}
	if cfg.APIKey != "" {
		t.Errorf("APIKey = %q, want empty", cfg.APIKey)
	}
}

// The model survives a missing key, so a file carrying a preference is not
// treated as though it had not been read.
func TestEmptyKeepsModel(t *testing.T) {
	cfg := Empty("stealth/space-bunny-alpha", false)
	if cfg.Model != "stealth/space-bunny-alpha" {
		t.Errorf("Model = %q, want it preserved", cfg.Model)
	}
}

// The model is read from the file and trimmed, since a stray space would be
// sent to the backend as part of the identifier.
func TestParseReadsModel(t *testing.T) {
	path := filepath.Join(t.TempDir(), DefaultFileName)
	body := `{"OPENROUTER_API_KEY":"k","OPENROUTER_MODEL":"  stealth/space-bunny-alpha  "}`
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	cfg, err := parse(path)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if cfg.Model != "stealth/space-bunny-alpha" {
		t.Errorf("Model = %q, want it trimmed", cfg.Model)
	}
}

// A file with no model is not an error, since the model may be chosen at
// runtime instead.
func TestParseWithoutModel(t *testing.T) {
	path := filepath.Join(t.TempDir(), DefaultFileName)
	if err := os.WriteFile(path, []byte(`{"OPENROUTER_API_KEY":"k"}`), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	cfg, err := parse(path)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if cfg.Model != "" {
		t.Errorf("Model = %q, want empty", cfg.Model)
	}
}

// A missing key carries the model through, so a caller standing in a
// configuration does not lose the preference.
func TestMissingKeyCarriesModel(t *testing.T) {
	path := filepath.Join(t.TempDir(), DefaultFileName)
	body := `{"OPENROUTER_MODEL":"stealth/space-bunny-alpha"}`
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	_, err := parse(path)
	var noKey *ErrNoAPIKey
	if !errors.As(err, &noKey) {
		t.Fatalf("err = %v, want *ErrNoAPIKey", err)
	}
	if noKey.Model != "stealth/space-bunny-alpha" {
		t.Errorf("Model = %q, want it carried through", noKey.Model)
	}
}

// The mouse preference is read from the file, so that a reader who wants the
// wheel does not run /mouse on every start.
func TestParseReadsMousePreference(t *testing.T) {
	path := filepath.Join(t.TempDir(), DefaultFileName)
	body := `{"OPENROUTER_API_KEY":"k","OPENROUTER_MOUSE":true}`
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	cfg, err := parse(path)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if !cfg.Mouse {
		t.Error("Mouse = false, want the preference read from the file")
	}
}

// A file that does not ask for it leaves the preference off, since capturing
// the mouse is a choice rather than a default.
func TestParseDefaultsMouseOff(t *testing.T) {
	path := filepath.Join(t.TempDir(), DefaultFileName)
	if err := os.WriteFile(path, []byte(`{"OPENROUTER_API_KEY":"k"}`), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	cfg, err := parse(path)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if cfg.Mouse {
		t.Error("Mouse = true, want it off when the file does not ask")
	}
}

// A file that cannot be read for its key is still read, so the mouse
// preference survives alongside the model.
func TestMouseSurvivesAMissingKey(t *testing.T) {
	path := filepath.Join(t.TempDir(), DefaultFileName)
	body := `{"OPENROUTER_MOUSE":true,"setup_complete":true}`
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	cfg, err := parse(path)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if !cfg.Mouse {
		t.Error("Mouse = false, want the preference kept without a key")
	}
}
