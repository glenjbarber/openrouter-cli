package config

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// writeConfig writes a configuration file at the required mode and returns its
// path.
func writeConfig(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), DefaultFileName)
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	return path
}

// The bell is a preference rather than a credential, so a file carrying it and
// no key must not lose it. The interface stands in a configuration built from
// the typed error, and a preference dropped there would make a file that was
// read look as though it had not been.
func TestMissingKeyCarriesBell(t *testing.T) {
	path := writeConfig(t, `{"OPENROUTER_BELL":true}`)

	_, err := parse(path)
	var noKey *ErrNoAPIKey
	if !errors.As(err, &noKey) {
		t.Fatalf("err = %v, want *ErrNoAPIKey", err)
	}
	if !noKey.Bell {
		t.Error("Bell = false, want the preference carried through with the typed error")
	}
}

// The endpoint is carried through for the same reason. A reader whose key has
// not been entered is still talking to whichever endpoint the file names, and a
// self-hosted deployment must not be redirected to the public one by the key
// being absent.
func TestMissingKeyCarriesURLBase(t *testing.T) {
	path := writeConfig(t, `{"OPENROUTER_URL_BASE":"http://localhost:3000/api/v1"}`)

	_, err := parse(path)
	var noKey *ErrNoAPIKey
	if !errors.As(err, &noKey) {
		t.Fatalf("err = %v, want *ErrNoAPIKey", err)
	}
	if noKey.URLBase != "http://localhost:3000/api/v1" {
		t.Errorf("URLBase = %q, want the endpoint from the file", noKey.URLBase)
	}
}

// A file that completed its setup without storing a credential returns a
// configuration rather than an error, and that configuration must still carry
// an endpoint. Without the default the session would be built against an empty
// base URL.
func TestSkippedSetupKeepsDefaultEndpoint(t *testing.T) {
	path := writeConfig(t, `{"setup_complete":true}`)

	cfg, err := parse(path)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if cfg.URLBase != DefaultURLBase {
		t.Errorf("URLBase = %q, want %q", cfg.URLBase, DefaultURLBase)
	}
	if cfg.APIKey != "" {
		t.Errorf("APIKey = %q, want empty", cfg.APIKey)
	}
	if !cfg.Skipped {
		t.Error("Skipped = false, want the deliberate skip reported")
	}
}

// The typed error carries a resolved endpoint as well, so a caller standing in
// a configuration has one to work with even when the file named none.
func TestMissingKeyCarriesResolvedURLBase(t *testing.T) {
	path := writeConfig(t, `{"OPENROUTER_MODEL":"a/b"}`)

	_, err := parse(path)
	var noKey *ErrNoAPIKey
	if !errors.As(err, &noKey) {
		t.Fatalf("err = %v, want *ErrNoAPIKey", err)
	}
	if noKey.URLBase != DefaultURLBase {
		t.Errorf("URLBase = %q, want %q", noKey.URLBase, DefaultURLBase)
	}
}

// The search order is fixed and is asserted literally, so that a change to a
// path is a deliberate act rather than an accident in a hand-written copy.
//
// A relative XDG_CONFIG_HOME is ignored rather than resolved against the
// working directory, since the specification requires an absolute path and a
// relative one would let a directory someone else wrote supply the credential
// file. A trailing separator is ordinary and is cleaned by Join.
func TestSearchPaths(t *testing.T) {
	home := filepath.Join(string(filepath.Separator), "home", "user")
	xdg := filepath.Join(string(filepath.Separator), "xdg")

	for _, tc := range []struct {
		xdg  string
		name string
		want []string
	}{
		{"", "unset", []string{
			filepath.Join(home, ".openrouter-cli.json"),
			filepath.Join(home, ".config", "openrouter-cli", "openrouter-cli.json"),
		}},
		{xdg, "absolute", []string{
			filepath.Join(home, ".openrouter-cli.json"),
			filepath.Join(xdg, "openrouter-cli", "openrouter-cli.json"),
		}},
		{xdg + string(filepath.Separator), "trailing separator", []string{
			filepath.Join(home, ".openrouter-cli.json"),
			filepath.Join(xdg, "openrouter-cli", "openrouter-cli.json"),
		}},
		{"relative/config", "relative ignored", []string{
			filepath.Join(home, ".openrouter-cli.json"),
			filepath.Join(home, ".config", "openrouter-cli", "openrouter-cli.json"),
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("XDG_CONFIG_HOME", tc.xdg)
			got := searchPaths(home)
			if len(got) != len(tc.want) {
				t.Fatalf("searchPaths = %q, want %q", got, tc.want)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Errorf("searchPaths[%d] = %q, want %q", i, got[i], tc.want[i])
				}
			}
		})
	}
}

// A configuration waiting at a later search path is a configuration. Writing
// the default at the first path would shadow it, since the search stops at the
// first match and does not merge, so a reader who keeps their file under
// XDG_CONFIG_HOME would lose it on the first run and see no credential at all.
func TestInstallDefaultDoesNotShadowConfigurationAtALaterPath(t *testing.T) {
	home := t.TempDir()
	xdg := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", xdg)

	dir := filepath.Join(xdg, "openrouter-cli")
	if err := os.Mkdir(dir, 0o700); err != nil {
		t.Fatalf("Mkdir: %v", err)
	}
	existing := `{"OPENROUTER_API_KEY":"sk-or-v1-in-xdg"}`
	if err := os.WriteFile(filepath.Join(dir, "openrouter-cli.json"), []byte(existing), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	written, err := InstallDefault()
	if err != nil {
		t.Fatalf("InstallDefault: %v", err)
	}
	if written {
		t.Error("written = true, want false while a configuration exists")
	}
	if _, err := os.Stat(filepath.Join(home, DefaultFileName)); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("Stat at the primary path = %v, want no default written there", err)
	}

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.APIKey != "sk-or-v1-in-xdg" {
		t.Errorf("APIKey = %q, want the file under XDG_CONFIG_HOME", cfg.APIKey)
	}
}

// A directory at the primary path is still reported rather than skipped, since
// the search alone must not swallow the case the installer names as an error.
func TestInstallDefaultStillReportsDirectory(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", "")
	if err := os.Mkdir(filepath.Join(home, DefaultFileName), 0o700); err != nil {
		t.Fatalf("Mkdir: %v", err)
	}

	if _, err := InstallDefault(); err == nil {
		t.Fatal("InstallDefault accepted a directory, want an error")
	}
}
