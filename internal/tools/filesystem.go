package tools

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/glenjbarber/openrouter-cli/internal/openrouter"
)

// The names the three filesystem tools are offered under. They are read by the
// model rather than by the reader, so they are spelled as a model would spell
// them: an action and a thing.
const (
	readFileTool  = "read_file"
	writeFileTool = "write_file"
	listDirTool   = "list_dir"
)

// MaxRead is the largest file read_file returns.
//
// A limit rather than a stream because the result of a tool call is one string
// that goes into the conversation: a file of two gigabytes would be carried in
// a single message and would spend the window on its own, and the model has no
// way to ask for the part of it it wanted. A refusal naming the limit is a
// thing a model can act on by asking for a smaller file, where a truncated read
// is a thing it would read as though it were the whole file.
const MaxRead = 1 << 20

// The schemas the three tools are offered with. They are literals rather than
// values marshalled at startup so that what a model is sent can be read in the
// source, and a test unmarshals every one of them so that a bracket left out
// here is caught by the test rather than by a model.
const (
	readFileParameters = `{
	  "type": "object",
	  "properties": {
	    "path": {
	      "type": "string",
	      "description": "The file to read, as a path inside the working directory. A path that leaves the working directory is refused."
	    }
	  },
	  "required": ["path"],
	  "additionalProperties": false
	}`

	writeFileParameters = `{
	  "type": "object",
	  "properties": {
	    "path": {
	      "type": "string",
	      "description": "The file to write, as a path inside the working directory. Parent directories are created. A path that leaves the working directory is refused."
	    },
	    "content": {
	      "type": "string",
	      "description": "The whole content of the file, which replaces what is there."
	    }
	  },
	  "required": ["path", "content"],
	  "additionalProperties": false
	}`

	listDirParameters = `{
	  "type": "object",
	  "properties": {
	    "path": {
	      "type": "string",
	      "default": ".",
	      "description": "The directory to list, as a path inside the working directory. It is the working directory itself when it is not given."
	    }
	  },
	  "additionalProperties": false
	}`
)

// escapeMessage is the text os.Root reports a path leaving the tree with.
//
// The root does not export the error it returns, so it is recognised by this.
// Only the wording depends on the match: the containment is the root's, and
// the alternative would be a prefix check written here, which a symlink inside
// the tree pointing out of it defeats while a cleaned path compares equal.
const escapeMessage = "path escapes from parent"

// NewFilesystem returns the filesystem tools contained to root.
//
// A nil root yields a set holding no tools rather than tools that cannot run.
// A root is taken once at startup and held by the session, so a nil one is a
// mistake in the caller, and the mistake is worth one set with nothing in it
// rather than a process that stops at the first call.
func NewFilesystem(root *os.Root) *Set {
	s := New()
	if root == nil {
		return s
	}
	s.Add(openrouter.Tool{
		Type: openrouter.ToolTypeFunction,
		Function: openrouter.ToolFunction{
			Name:        readFileTool,
			Description: "Read a file and return its content as text.",
			Parameters:  json.RawMessage(readFileParameters),
		},
	}, readFile(root))
	s.Add(openrouter.Tool{
		Type: openrouter.ToolTypeFunction,
		Function: openrouter.ToolFunction{
			Name: writeFileTool,
			Description: "Write a file, creating the parent directories inside the " +
				"working directory, and return how many bytes were written.",
			Parameters: json.RawMessage(writeFileParameters),
		},
	}, writeFile(root))
	s.Add(openrouter.Tool{
		Type: openrouter.ToolTypeFunction,
		Function: openrouter.ToolFunction{
			Name:        listDirTool,
			Description: "List a directory, directories first and then files, one entry per line.",
			Parameters:  json.RawMessage(listDirParameters),
		},
	}, listDir(root))
	return s
}

// NewFilesystemAt opens dir as a root and returns the tools contained to it.
//
// The directory is opened rather than named, since a containment is the open
// descriptor and not the path: a name would be checked against a prefix and
// would be wrong for a symlink.
func NewFilesystemAt(dir string) (*Set, error) {
	root, err := os.OpenRoot(dir)
	if err != nil {
		return nil, fmt.Errorf("cannot work in %s: %w", dir, err)
	}
	return NewFilesystem(root), nil
}

// readFile returns the body of the read tool.
//
// The size is settled by reading through a limit rather than by asking the
// directory first, so that a file growing between the two cannot turn a bounded
// read into an unbounded one. A directory is reported as one rather than read,
// since the read of one fails with a message naming the system call.
func readFile(root *os.Root) toolFn {
	return func(raw json.RawMessage) (string, error) {
		var args struct {
			Path *string `json:"path"`
		}
		if err := decode(raw, &args); err != nil {
			return "", err
		}
		if args.Path == nil {
			return "", missingArg("path")
		}
		path := *args.Path

		f, err := root.Open(path)
		if err != nil {
			return "", pathError("cannot read", path, err)
		}
		defer f.Close()

		info, err := f.Stat()
		if err != nil {
			return "", pathError("cannot read", path, err)
		}
		if info.IsDir() {
			return "", fmt.Errorf("%q is a directory, and %s lists the entries of one",
				path, listDirTool)
		}

		data, err := io.ReadAll(io.LimitReader(f, MaxRead+1))
		if err != nil {
			return "", pathError("cannot read", path, err)
		}
		if len(data) > MaxRead {
			return "", fmt.Errorf("%q is larger than the %d byte limit a read returns "+
				"whole, and is not returned", path, int64(MaxRead))
		}
		return string(data), nil
	}
}

// writeFile returns the body of the write tool.
//
// The parent directories are created through the root rather than through the
// joined path, so that a path climbing out of the tree is refused before
// anything is created. The content needs no limit of its own: it arrives inside
// the request, so it is already bounded by what a model can emit.
func writeFile(root *os.Root) toolFn {
	return func(raw json.RawMessage) (string, error) {
		var args struct {
			Path    *string `json:"path"`
			Content *string `json:"content"`
		}
		if err := decode(raw, &args); err != nil {
			return "", err
		}
		if args.Path == nil {
			return "", missingArg("path")
		}
		if args.Content == nil {
			return "", missingArg("content")
		}
		path, content := *args.Path, *args.Content

		if parent := filepath.Dir(path); parent != "." {
			if err := root.MkdirAll(parent, 0o755); err != nil {
				return "", pathError("cannot write", path, err)
			}
		}
		if err := root.WriteFile(path, []byte(content), 0o644); err != nil {
			return "", pathError("cannot write", path, err)
		}
		return fmt.Sprintf("wrote %d bytes to %s", len(content), path), nil
	}
}

// listDir returns the body of the list tool.
//
// Directories are reported before files because a model reading the listing
// picks a directory to go into more often than it picks a file, and the sort
// is on the name so that two runs of the same command answer the same way.
func listDir(root *os.Root) toolFn {
	return func(raw json.RawMessage) (string, error) {
		var args struct {
			Path *string `json:"path"`
		}
		if err := decode(raw, &args); err != nil {
			return "", err
		}
		path := "."
		if args.Path != nil && *args.Path != "" {
			path = *args.Path
		}

		dir, err := root.Open(path)
		if err != nil {
			return "", pathError("cannot list", path, err)
		}
		defer dir.Close()

		entries, err := dir.ReadDir(-1)
		if err != nil {
			return "", pathError("cannot list", path, err)
		}
		dirs := make([]string, 0, len(entries))
		files := make([]string, 0, len(entries))
		for _, e := range entries {
			if e.IsDir() {
				dirs = append(dirs, e.Name()+"/")
				continue
			}
			files = append(files, e.Name())
		}
		sort.Strings(dirs)
		sort.Strings(files)
		return strings.Join(append(dirs, files...), "\n"), nil
	}
}

// pathError reports a failure from the root, naming the refusal where the path
// left the tree.
//
// A refusal reported as the system error beneath it reads as a permission
// problem on the file rather than as a path the model chose wrongly, and the
// second is the one it can do something about.
func pathError(op, path string, err error) error {
	if strings.Contains(err.Error(), escapeMessage) {
		return fmt.Errorf("%s %q: refused, the path is outside the root", op, path)
	}
	return fmt.Errorf("%s %q: %w", op, path, err)
}
