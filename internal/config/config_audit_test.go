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
