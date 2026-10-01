package config

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A mode other than 0600 is a hard failure rather than a warning, and the
// message says which mode was found and what to run.
func TestModeOtherThanRequiredIsHardFailure(t *testing.T) {
	for _, mode := range []os.FileMode{0o644, 0o666, 0o640, 0o604, 0o400, 0o700} {
		t.Run(mode.String(), func(t *testing.T) {
			path := filepath.Join(t.TempDir(), DefaultFileName)
			writeAt(t, path, `{"OPENROUTER_API_KEY":"k"}`, mode)

			cfg, err := parse(path)
			if err == nil {
				t.Fatalf("parse accepted mode %04o, want a failure", mode)
			}
			if cfg != nil {
				t.Errorf("cfg = %+v, want nil alongside the failure", cfg)
			}
			if !strings.Contains(err.Error(), "chmod 0600") {
				t.Errorf("err = %q, want it to name the remedy", err)
			}
			if !strings.Contains(err.Error(), path) {
				t.Errorf("err = %q, want it to name the file %q", err, path)
			}
		})
	}
}

// The mode is checked before the file is parsed, so a permissive file is
// refused on its mode and not on its contents.
func TestModeIsCheckedBeforeTheContents(t *testing.T) {
	path := filepath.Join(t.TempDir(), DefaultFileName)
	writeAt(t, path, `{not json at all`, 0o644)

	_, err := parse(path)
	if err == nil {
		t.Fatal("parse accepted a permissive malformed file, want the mode reported")
	}
	if !strings.Contains(err.Error(), "chmod 0600") {
		t.Errorf("err = %q, want the mode reported first", err)
	}
}

// A permissive parent directory is ordinary and is not itself a credential
// exposure, so the check is on the file alone.
func TestPermissiveParentDirectoryIsAccepted(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "parent")
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatalf("Mkdir: %v", err)
	}
	path := filepath.Join(dir, DefaultFileName)
	writeAt(t, path, `{"OPENROUTER_API_KEY":"k"}`, 0o600)

	if _, err := parse(path); err != nil {
		t.Fatalf("parse refused a file under a 0755 parent: %v", err)
	}
}

// The mode check follows a link to its target, since the target is the file
// holding the credential. A link is not a file that can hold one itself.
func TestModeCheckFollowsTheLinkToItsTarget(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "target.json")
	link := filepath.Join(dir, DefaultFileName)
	writeAt(t, target, `{"OPENROUTER_API_KEY":"k"}`, 0o644)

	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}

	// The link itself carries the mode of a link, which is not 0600, so a
	// check on the link would fail on every platform and refuse every aliased
	// configuration. The target is what is examined.
	_, err := parse(link)
	if err == nil {
		t.Fatal("parse accepted a permissive target behind a link, want a failure")
	}
	if !strings.Contains(err.Error(), "chmod 0600") {
		t.Errorf("err = %q, want the mode of the target reported", err)
	}

	writeAt(t, target, `{"OPENROUTER_API_KEY":"k"}`, 0o600)
	cfg, err := parse(link)
	if err != nil {
		t.Fatalf("parse refused a 0600 target behind a link: %v", err)
	}
	if cfg.APIKey != "k" {
		t.Errorf("APIKey = %q, want the target contents", cfg.APIKey)
	}
}

// A file that cannot be read is a fault, and the diagnostic names the file
// rather than the reason alone. The mode check runs first and passes, so the
// refusal comes from the read: the parent is made unsearchable, since a file
// whose own mode is wrong would be refused on its mode instead.
func TestUnreadableFileIsReported(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("running as root, where the mode does not prevent a read")
	}
	dir := filepath.Join(t.TempDir(), "parent")
	if err := os.Mkdir(dir, 0o700); err != nil {
		t.Fatalf("Mkdir: %v", err)
	}
	path := filepath.Join(dir, DefaultFileName)
	writeAt(t, path, `{"OPENROUTER_API_KEY":"k"}`, 0o600)
	// The parent is made unsearchable only once the file is in place, since it
	// could not otherwise be created. It is restored for the cleanup, which
	// would otherwise fail to remove a directory it cannot enter.
	if err := os.Chmod(dir, 0o600); err != nil {
		t.Fatalf("Chmod: %v", err)
	}
	t.Cleanup(func() { os.Chmod(dir, 0o700) })

	_, err := parse(path)
	if !errors.Is(err, os.ErrPermission) {
		t.Fatalf("err = %v, want a permission error", err)
	}
	if !strings.Contains(err.Error(), path) {
		t.Errorf("err = %q, want it to name the file %q", err, path)
	}
}

// A directory at the path is reported rather than treated as an absent file,
// since the path could not be written in any case.
func TestDirectoryAtTheConfigurationPathIsReported(t *testing.T) {
	home := isolateHome(t)
	if err := os.Mkdir(filepath.Join(home, DefaultFileName), 0o700); err != nil {
		t.Fatalf("Mkdir: %v", err)
	}

	_, err := InstallDefault()
	if err == nil {
		t.Fatal("InstallDefault accepted a directory, want an error")
	}
	if !strings.Contains(err.Error(), "is a directory") {
		t.Errorf("err = %q, want it to name the directory", err)
	}
}
