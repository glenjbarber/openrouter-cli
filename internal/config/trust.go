package config

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// trustedKey is the file key holding the directories the reader has trusted.
const trustedKey = "OPENROUTER_TRUSTED"

// ErrNotTrusted reports that the current directory is not marked trusted and
// the reader did not, or could not, allow it.
var ErrNotTrusted = errors.New("directory is not trusted")

// ConfigPath returns the configuration file the loader would read, the first
// existing non-directory at a search path, or false when there is none.
func ConfigPath() (string, bool) {
	for _, path := range SearchPaths() {
		if info, err := os.Stat(path); err == nil && !info.IsDir() {
			return path, true
		}
	}
	return "", false
}

// canonicalDir resolves dir to an absolute path with symlinks followed, so that
// two spellings of one directory are one entry and a link cannot borrow the
// trust given to its target's neighbour.
func canonicalDir(dir string) (string, error) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return "", err
	}
	return filepath.EvalSymlinks(abs)
}

// readTrusted returns the trusted directories recorded in the file at path.
func readTrusted(path string) ([]string, map[string]json.RawMessage, error) {
	if err := checkMode(path); err != nil {
		return nil, nil, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, fmt.Errorf("reading %s: %w", path, err)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return nil, nil, fmt.Errorf("%s is not a valid JSON object: %w", path, err)
	}
	var dirs []string
	if raw, ok := fields[trustedKey]; ok {
		if err := json.Unmarshal(raw, &dirs); err != nil {
			return nil, nil, fmt.Errorf("%s: %s is not a list of strings: %w", path, trustedKey, err)
		}
	}
	return dirs, fields, nil
}

// IsTrusted reports whether dir is recorded as trusted in the file at path.
//
// Every failure is reported as not trusted. A file that cannot be read, has the
// wrong mode, or is malformed grants nothing, so the default on any doubt is to
// ask. The match is on the whole directory only: trusting a directory does not
// trust the directories beneath it.
func IsTrusted(path, dir string) bool {
	want, err := canonicalDir(dir)
	if err != nil {
		return false
	}
	dirs, _, err := readTrusted(path)
	if err != nil {
		return false
	}
	for _, d := range dirs {
		if d == want {
			return true
		}
	}
	return false
}

// Trust records dir as trusted in the file at path.
//
// Every other key in the file is carried over as the raw bytes it was read as,
// so the credential is neither interpreted nor altered, and no value is added
// other than the directory. The file is rewritten beside itself at 0600 and
// renamed over, so an interrupted write leaves the previous file. A file that
// does not have the required mode is refused, as the loader refuses it.
func Trust(path, dir string) error {
	want, err := canonicalDir(dir)
	if err != nil {
		return fmt.Errorf("resolving %s: %w", dir, err)
	}
	// A link is replaced at its target, so the link itself survives.
	if real, err := filepath.EvalSymlinks(path); err == nil {
		path = real
	}
	dirs, fields, err := readTrusted(path)
	if err != nil {
		return err
	}
	for _, d := range dirs {
		if d == want {
			return nil
		}
	}
	list, err := json.Marshal(append(dirs, want))
	if err != nil {
		return err
	}
	fields[trustedKey] = list
	body, err := json.MarshalIndent(fields, "", "\t")
	if err != nil {
		return err
	}
	body = append(body, '\n')

	tmp := path + ".new"
	os.Remove(tmp)
	f, err := os.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_EXCL, RequiredMode)
	if err != nil {
		return fmt.Errorf("creating %s: %w", tmp, err)
	}
	// The umask only lowers the mode given to open, so it is set explicitly.
	if err := f.Chmod(RequiredMode); err == nil {
		_, err = f.Write(body)
		if err == nil {
			err = f.Sync()
		}
		if cerr := f.Close(); err == nil {
			err = cerr
		}
		if err == nil {
			err = os.Rename(tmp, path)
		}
		if err != nil {
			os.Remove(tmp)
			return fmt.Errorf("writing %s: %w", path, err)
		}
		return nil
	}
	f.Close()
	os.Remove(tmp)
	return fmt.Errorf("setting the mode of %s", tmp)
}

// EnsureTrusted asks permission to read dir unless it is already trusted, and
// records an approval in the file at path.
//
// Only an answer of y or yes, in any case, is an approval. An empty answer, any
// other text, end of input, a read error, and a session that is not interactive
// are all refusals, and a refusal records nothing. It reports whether dir is
// trusted when it returns.
func EnsureTrusted(path, dir string, in io.Reader, out io.Writer, interactive bool) (bool, error) {
	if path == "" {
		return false, errors.New("no configuration file to record trust in")
	}
	if IsTrusted(path, dir) {
		return true, nil
	}
	if !interactive {
		return false, nil
	}
	shown := dir
	if abs, err := canonicalDir(dir); err == nil {
		shown = abs
	}
	fmt.Fprintf(out, "%s is not marked trusted.\n"+
		"Allow openrouter-cli to read this directory, and remember the choice in %s? [y/N] ",
		shown, path)
	line, _ := bufio.NewReader(in).ReadString('\n')
	switch strings.ToLower(strings.TrimSpace(line)) {
	case "y", "yes":
	default:
		return false, nil
	}
	if err := Trust(path, dir); err != nil {
		return false, err
	}
	return true, nil
}
