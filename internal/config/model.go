package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Chosen is what the reader selected at runtime, kept apart from the
// configuration file.
//
// The configuration file holds the credential and is read-only outside setup,
// so a choice made at the keyboard cannot be written back into it. The
// approval rules are kept beside it for the same reason and by the same
// route, and this file follows that precedent rather than inventing a second
// one.
type Chosen struct {
	// Model is the model identifier requests are sent to, as /model last set
	// it. It is empty where the reader has not chosen one, which is not the
	// same as having chosen none: an absent value leaves the configuration
	// file to decide.
	Model string `json:"OPENROUTER_MODEL"`
	// Provider is the name the status bar shows for the service.
	//
	// It is a label and nothing more. Where requests are actually sent is
	// settled by OPENROUTER_URL_BASE, which the loader has always read, so a
	// provider named here says how the service should read on screen and
	// does not redirect a single request. The two keys are deliberately
	// separate rather than merged, since merging them would make a name
	// written for the status bar silently repoint the client somewhere else.
	Provider string `json:"OPENROUTER_PROVIDER"`
}

// ModelFile is the file the runtime choices are kept in, beside the
// configuration.
func ModelFile() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("locating the home directory: %w", err)
	}
	return filepath.Join(home, ".openrouter-cli", "model.json"), nil
}

// LoadChosen reads the runtime choices.
//
// The file is consulted after the configuration rather than before it, on the
// same terms as the rules: a choice made at the keyboard is the one a reader
// most recently said, so it settles over a value typed into the configuration
// by hand. A missing file is not an error, since a reader who has chosen
// nothing is asking for the configuration and nothing else.
func LoadChosen() (Chosen, error) {
	var chosen Chosen
	path, err := ModelFile()
	if err != nil {
		return chosen, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return chosen, nil
		}
		return chosen, fmt.Errorf("reading %s: %w", path, err)
	}

	// Unknown keys are ignored rather than refused, on the same terms as the
	// configuration and the rules file: a file written for a newer version
	// stays readable by an older one.
	var written Chosen
	if err := json.Unmarshal(data, &written); err != nil {
		return chosen, fmt.Errorf("%s is not valid JSON: %w", path, err)
	}
	written.Model = strings.TrimSpace(written.Model)
	written.Provider = strings.TrimSpace(written.Provider)
	return written, nil
}

// WriteChosen saves the runtime choices.
//
// The file is written whole and renamed over, as the rules file is, so that a
// reader reading it never sees half of a value. It is written at 0600 for the
// same reason the configuration file is: it sits beside a credential and is
// named by the same program, so it is given the same protection rather than a
// weaker one for being smaller.
func WriteChosen(chosen Chosen) error {
	path, err := ModelFile()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("creating %s: %w", filepath.Dir(path), err)
	}

	body, err := json.MarshalIndent(struct {
		Model    string `json:"OPENROUTER_MODEL"`
		Provider string `json:"OPENROUTER_PROVIDER"`
	}{
		Model:    strings.TrimSpace(chosen.Model),
		Provider: strings.TrimSpace(chosen.Provider),
	}, "", "  ")
	if err != nil {
		return err
	}
	body = append(body, '\n')

	// The file is written beside itself and renamed over, so that a reader
	// reading it never sees half of one value. A write interrupted by a crash
	// leaves the previous one rather than a truncated model identifier, which
	// would send every request to a model that does not exist.
	tmp := path + ".new"
	if err := os.WriteFile(tmp, body, 0o600); err != nil {
		return fmt.Errorf("writing %s: %w", path, err)
	}
	if err := os.Rename(tmp, path); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("replacing %s: %w", path, err)
	}
	return nil
}

// ModelFileWhere names the file the runtime choices are in, for a message that
// points a reader at it.
func ModelFileWhere() string {
	path, err := ModelFile()
	if err != nil {
		return "the model file"
	}
	return path
}
