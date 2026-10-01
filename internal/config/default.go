package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// DefaultFile is the default document written when no configuration exists.
//
// The URL base is written out rather than left empty, so that the file states
// the value it is using. An absent key is not written, since a placeholder key
// would be indistinguishable from a real one and would be sent to the backend.
const DefaultFile = `{
	"OPENROUTER_URL_BASE": "https://openrouter.ai/api/v1"
}
`

// DefaultFileName is the primary search path, written when no file is present.
const DefaultFileName = ".openrouter-cli.json"

// InstallDefault writes the default configuration when no file exists.
//
// An existing file is left exactly as it is. The merge is deliberately not
// attempted: a partial merge of a credential file can produce a file that
// parses but is wrong, and a wrong credential fails later at the point of use
// rather than where it was introduced. Doing nothing is the reversible
// outcome, so doing nothing is what happens.
//
// The file is created at 0600 because the loader refuses any other mode, so a
// default written at a permissive mode would be rejected by the client that
// wrote it. The creation is exclusive, so a file that appeared between the
// check and the write is not overwritten.
//
// It reports whether the file was written.
func InstallDefault() (bool, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return false, fmt.Errorf("locating the home directory: %w", err)
	}
	return InstallDefaultAt(filepath.Join(home, DefaultFileName))
}

// InstallDefaultAt writes the default configuration at path when absent.
func InstallDefaultAt(path string) (bool, error) {
	// A missing file is the only case in which a default is written, so the
	// existence check is part of the contract rather than an optimisation.
	//
	// A directory is treated as present but unusable rather than as absent,
	// since Stat reports one as existing and the path could not be written in
	// any case. Silently reporting nothing written would leave the caller
	// believing the configuration was in place.
	if info, err := os.Stat(path); err == nil {
		if info.IsDir() {
			return false, fmt.Errorf("%s is a directory, not a configuration file", path)
		}
		return false, nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return false, fmt.Errorf("examining %s: %w", path, err)
	}

	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, RequiredMode)
	if err != nil {
		// A file created between the check and the write is left alone.
		if errors.Is(err, os.ErrExist) {
			return false, nil
		}
		return false, fmt.Errorf("creating %s: %w", path, err)
	}
	defer f.Close()

	if _, err := f.WriteString(DefaultFile); err != nil {
		f.Close()
		// A partial file would be read as a configuration and rejected for
		// holding no key, so it is removed rather than left behind.
		os.Remove(path)
		return false, fmt.Errorf("writing %s: %w", path, err)
	}
	return true, nil
}
