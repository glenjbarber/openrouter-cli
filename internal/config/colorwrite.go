package config

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// colorKey is the file key holding the color switch.
const colorKey = "color"

// ErrNoConfigFile reports that there is no configuration file to write to.
var ErrNoConfigFile = errors.New("no configuration file")

// WriteColor sets the color key in the existing file at path to on.
//
// The file is edited at the byte level and nothing in it is parsed into a value
// and written back. When a color key holds a boolean, only the bytes of that
// value change. When there is none, one member is added before the closing
// brace, copying the spacing of the last member, and the previous last member
// gains a comma. Every other byte stays as it was, whether whitespace, key
// order, line endings, escapes, or the credential. The edited bytes are written
// to a file beside the original at 0600 and renamed over it, so an interrupted
// write leaves the previous file. The file is never created: a missing file is
// an error, since a file made here would hold no credential and would suppress
// first-time setup.
//
// The file is refused, and left as it is, when it cannot be read, is not a JSON
// object, has a mode other than the one the loader requires, or holds a color
// that is not a boolean. The edited bytes are also checked to be a JSON object
// holding the requested value before anything is written. No message carries
// any of the file contents.
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
	if !isJSONObject(data) {
		return fmt.Errorf("%s is not a valid JSON object", path)
	}
	obj, ok := scanObject(data)
	if !ok {
		return fmt.Errorf("%s is not a valid JSON object", path)
	}
	value := "false"
	if on {
		value = "true"
	}

	// The last matching key is the effective one, as the decoder takes it.
	last := -1
	for i, m := range obj.members {
		if strings.EqualFold(m.key, colorKey) {
			last = i
		}
	}
	var body []byte
	if last >= 0 {
		m := obj.members[last]
		if s := string(data[m.valStart:m.valEnd]); s != "true" && s != "false" {
			return fmt.Errorf("%s: %s is not a boolean", path, colorKey)
		}
		body = splice(data, m.valStart, m.valEnd, value)
	} else {
		body = insertColor(data, obj, value)
	}
	if bytes.Equal(body, data) {
		return nil
	}
	// A safety net: the edit must still be an object holding the new value.
	var check struct {
		Color bool `json:"color"`
	}
	if !isJSONObject(body) || json.Unmarshal(body, &check) != nil || check.Color != on {
		return fmt.Errorf("cannot edit %s", path)
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

// isJSONObject reports whether data is valid JSON holding one object.
func isJSONObject(data []byte) bool {
	var fields map[string]json.RawMessage
	return json.Valid(data) && json.Unmarshal(data, &fields) == nil && fields != nil
}

// splice returns data with the bytes from start to end replaced by with.
func splice(data []byte, start, end int, with string) []byte {
	out := make([]byte, 0, len(data)-(end-start)+len(with))
	out = append(out, data[:start]...)
	out = append(out, with...)
	return append(out, data[end:]...)
}

// member is one top-level key and value, as offsets into the file.
type member struct {
	key      string // the key, decoded
	lead     int    // the offset just after the brace or comma before the key
	keyStart int    // the opening quote of the key
	keyEnd   int    // just past the closing quote of the key
	valStart int
	valEnd   int
}

// object is the top-level object of a file, as offsets into the file.
type object struct {
	open    int // the offset of the opening brace
	close   int // the offset of the closing brace
	members []member
}

func isSpace(c byte) bool {
	return c == ' ' || c == '\t' || c == '\n' || c == '\r'
}

func skipSpace(data []byte, i int) int {
	for i < len(data) && isSpace(data[i]) {
		i++
	}
	return i
}

// skipString returns the offset just past the string starting at i, which is
// its opening quote, or -1 when it is not closed. A backslash takes the byte
// after it, so an escaped quote does not end the string.
func skipString(data []byte, i int) int {
	for i++; i < len(data); i++ {
		switch data[i] {
		case '\\':
			i++
		case '"':
			return i + 1
		}
	}
	return -1
}

// skipValue returns the offset just past the value starting at i, or -1. A
// string is skipped as a string, an object or array by counting its brackets
// outside strings, and a number or literal up to the next delimiter. Nothing
// inside a nested value is read, so a key there is never seen.
func skipValue(data []byte, i int) int {
	if i >= len(data) {
		return -1
	}
	switch data[i] {
	case '"':
		return skipString(data, i)
	case '{', '[':
		depth := 0
		for i < len(data) {
			switch data[i] {
			case '"':
				if i = skipString(data, i); i < 0 {
					return -1
				}
				continue
			case '{', '[':
				depth++
			case '}', ']':
				depth--
				if depth == 0 {
					return i + 1
				}
			}
			i++
		}
		return -1
	}
	start := i
	for i < len(data) && !isSpace(data[i]) && data[i] != ',' && data[i] != '}' && data[i] != ']' {
		i++
	}
	if i == start {
		return -1
	}
	return i
}

// scanObject finds the members of the top-level object in data, which is
// already known to be a JSON object. Only the keys at depth one are recorded.
func scanObject(data []byte) (object, bool) {
	var obj object
	i := skipSpace(data, 0)
	if i >= len(data) || data[i] != '{' {
		return obj, false
	}
	obj.open = i
	i++
	lead := i
	for first := true; ; first = false {
		i = skipSpace(data, i)
		if i >= len(data) {
			return obj, false
		}
		if data[i] == '}' && first {
			break
		}
		if data[i] != '"' {
			return obj, false
		}
		m := member{lead: lead, keyStart: i}
		if m.keyEnd = skipString(data, i); m.keyEnd < 0 {
			return obj, false
		}
		if json.Unmarshal(data[m.keyStart:m.keyEnd], &m.key) != nil {
			return obj, false
		}
		i = skipSpace(data, m.keyEnd)
		if i >= len(data) || data[i] != ':' {
			return obj, false
		}
		m.valStart = skipSpace(data, i+1)
		if m.valEnd = skipValue(data, m.valStart); m.valEnd < 0 {
			return obj, false
		}
		obj.members = append(obj.members, m)
		i = skipSpace(data, m.valEnd)
		if i >= len(data) {
			return obj, false
		}
		if data[i] == '}' {
			break
		}
		if data[i] != ',' {
			return obj, false
		}
		i++
		lead = i
	}
	obj.close = i
	return obj, skipSpace(data, i+1) == len(data)
}

// insertColor returns data with a color member added before the closing brace.
// The member copies the whitespace before the last member's key and between its
// key and value, so the line separator, the indentation and the colon spacing
// are the file's own, and the comma is added after the previous last member.
func insertColor(data []byte, obj object, value string) []byte {
	key := `"` + colorKey + `"`
	if n := len(obj.members); n > 0 {
		m := obj.members[n-1]
		add := "," + string(data[m.lead:m.keyStart]) + key +
			string(data[m.keyEnd:m.valStart]) + value
		return splice(data, m.valEnd, m.valEnd, add)
	}
	inner := string(data[obj.open+1 : obj.close])
	if strings.Contains(inner, "\n") {
		sep := "\n"
		if strings.Contains(inner, "\r\n") {
			sep = "\r\n"
		}
		return splice(data, obj.open+1, obj.close, sep+"\t"+key+": "+value+sep)
	}
	return splice(data, obj.open+1, obj.close, inner+key+": "+value+inner)
}
