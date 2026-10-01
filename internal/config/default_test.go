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
