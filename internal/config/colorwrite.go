package config

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
)

// colorKey is the file key holding the color switch.
const colorKey = "color"

// ErrNoConfigFile reports that there is no configuration file to write to.
var ErrNoConfigFile = errors.New("no configuration file")

// WriteColor sets the color key in the existing file at path to on.
//
// Every other key is carried over as the raw bytes it was read as, so the
// credential and any key this version does not know are neither interpreted nor
// altered, and color_theme is left exactly as it is. The file is rewritten
// beside itself at 0600 and renamed over, so an interrupted write leaves the
// previous file. The file is never created: a missing file is an error, since a
// file made here would hold no credential and would suppress first-time setup.
//
// The file is refused, and left as it is, when it cannot be read, is not a JSON
// object, has a mode other than the one the loader requires, or holds a color
// that is not a boolean. No message carries any of the file contents.
func WriteColor(path string, on bool) error {
	if path == "" {
		return ErrNoConfigFile
	}
	// A link is replaced at its target, so the link itself survives.
	if real, err := filepath.EvalSymlinks(path); err == nil {
		path = real
	}
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("cannot examine %s", path)
	}
	if info.IsDir() {
		return fmt.Errorf("%s is a directory", path)
	}
	if mode := info.Mode().Perm(); mode != RequiredMode {
		return fmt.Errorf("%s has mode %04o, but %04o is required", path, mode, RequiredMode)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("cannot read %s", path)
	}
	var fields map[string]json.RawMessage
	if !json.Valid(data) || json.Unmarshal(data, &fields) != nil || fields == nil {
		return fmt.Errorf("%s is not a valid JSON object", path)
	}
	if raw, ok := fields[colorKey]; ok {
		if s := string(bytes.TrimSpace(raw)); s != "true" && s != "false" {
			return fmt.Errorf("%s: %s is not a boolean", path, colorKey)
		}
	}
	if on {
		fields[colorKey] = json.RawMessage("true")
	} else {
		fields[colorKey] = json.RawMessage("false")
	}
	body, err := joinFields(fields)
	if err != nil {
		return fmt.Errorf("cannot encode %s", path)
	}

	tmp := path + ".new"
	os.Remove(tmp)
	f, err := os.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_EXCL, RequiredMode)
	if err != nil {
		return fmt.Errorf("cannot create %s", tmp)
	}
	// The umask only lowers the mode given to open, so it is set explicitly.
	err = f.Chmod(RequiredMode)
	if err == nil {
		_, err = f.Write(body)
	}
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
		return fmt.Errorf("cannot write %s", path)
	}
	return nil
}

// joinFields writes the keys in sorted order, each value as the bytes it was
// read as. Marshalling the map would compact and re-escape the values, and the
// file holds a credential that is not to be touched.
func joinFields(fields map[string]json.RawMessage) ([]byte, error) {
	keys := make([]string, 0, len(fields))
	for k := range fields {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var buf bytes.Buffer
	buf.WriteString("{\n")
	for i, k := range keys {
		var name bytes.Buffer
		enc := json.NewEncoder(&name)
		enc.SetEscapeHTML(false)
		if err := enc.Encode(k); err != nil {
			return nil, err
		}
		buf.WriteByte('\t')
		buf.Write(bytes.TrimRight(name.Bytes(), "\n"))
		buf.WriteString(": ")
		buf.Write(bytes.TrimSpace(fields[k]))
		if i < len(keys)-1 {
			buf.WriteByte(',')
		}
		buf.WriteByte('\n')
	}
	buf.WriteString("}\n")
	return buf.Bytes(), nil
}
