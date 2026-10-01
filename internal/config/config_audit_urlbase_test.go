package config

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

// The endpoint override is applied by the loader, so the rule is asserted
// through parse rather than against the helper alone. A file naming an endpoint
// is resolved the same way whether or not it carries a credential.
func TestURLBaseOverride(t *testing.T) {
	for _, tc := range []struct {
		raw  string
		want string
		name string
	}{
		{"", DefaultURLBase, "absent"},
		{"   ", DefaultURLBase, "whitespace only"},
		{"  https://openrouter.ai/api/v1  ", DefaultURLBase, "surrounding whitespace"},
		{DefaultURLBase, DefaultURLBase, "already the default"},
		{"http://localhost:3000", "http://localhost:3000/api/v1", "bare scheme and host"},
		{"http://localhost:3000/", "http://localhost:3000/api/v1", "trailing separator"},
		{"http://127.0.0.1:11434", "http://127.0.0.1:11434/api/v1", "loopback address"},
		{"http://localhost:3000/api/v1", "http://localhost:3000/api/v1", "already suffixed"},
		{"http://localhost:3000/api/v1/", "http://localhost:3000/api/v1/", "suffixed with a separator"},
		{"http://localhost:3000/proxy", "http://localhost:3000/proxy", "carrying a path"},
		{"https://gateway.example/openrouter/v1", "https://gateway.example/openrouter/v1", "carrying a longer path"},
		{"http://user:pw@localhost:3000", "http://user:pw@localhost:3000/api/v1", "carrying credentials"},
		{"http://localhost:3000?tenant=a", "http://localhost:3000/api/v1?tenant=a", "carrying a query"},
		{"localhost:3000", "localhost:3000", "no scheme"},
		{"openrouter.ai/api/v1", "openrouter.ai/api/v1", "no scheme with a path"},
		{"/api/v1", "/api/v1", "path only"},
		{"://bad", "://bad", "unparseable"},
		{"http://local host:3000", "http://local host:3000", "unparseable host"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), DefaultFileName)
			body := fmt.Sprintf(`{"OPENROUTER_API_KEY":"k","OPENROUTER_URL_BASE":%q}`, tc.raw)
			if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
				t.Fatalf("WriteFile: %v", err)
			}

			cfg, err := parse(path)
			if err != nil {
				t.Fatalf("parse: %v", err)
			}
			if cfg.URLBase != tc.want {
				t.Errorf("URLBase = %q, want %q", cfg.URLBase, tc.want)
			}
		})
	}
}

// A value that does not parse as a URL is passed on as written rather than
// replaced with the default, so that the backend reports it and the reader is
// not silently redirected to a different endpoint.
func TestUnparseableEndpointIsNotReplacedByTheDefault(t *testing.T) {
	path := filepath.Join(t.TempDir(), DefaultFileName)
	body := `{"OPENROUTER_API_KEY":"k","OPENROUTER_URL_BASE":"not a url"}`
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	cfg, err := parse(path)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if cfg.URLBase == DefaultURLBase {
		t.Error("URLBase = the default, want the value from the file")
	}
	if cfg.URLBase != "not a url" {
		t.Errorf("URLBase = %q, want the value passed on as written", cfg.URLBase)
	}
}

// The endpoint is resolved before the key is examined, so a file carrying only
// an endpoint and no key still yields one for a session to be built against.
func TestEndpointIsResolvedWithoutAKey(t *testing.T) {
	path := filepath.Join(t.TempDir(), DefaultFileName)
	body := `{"OPENROUTER_URL_BASE":"http://localhost:3000"}`
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	_, err := parse(path)
	if err == nil {
		t.Fatal("parse accepted a file with no key and no setup, want the typed error")
	}
	keyErr, ok := err.(*ErrNoAPIKey)
	if !ok {
		t.Fatalf("err = %T, want *ErrNoAPIKey", err)
	}
	if keyErr.URLBase != "http://localhost:3000/api/v1" {
		t.Errorf("URLBase = %q, want the bare host suffixed", keyErr.URLBase)
	}
}
