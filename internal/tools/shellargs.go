package tools

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// containedArgs checks the path-shaped arguments of a command against the
// working directory it would run in.
//
// The bound being enforced is that a command reaches nothing outside the
// directory it is run in. It is enforced on the arguments rather than on the
// working directory alone, since a working directory is not a sandbox: a
// process given one can still open any path it can name, so `rm -rf ../..` or
// `cat /etc/passwd` would run with the reader's own privileges either way.
// Setting the directory bounds where a program starts, not where it may go,
// and the difference is the whole of what this check exists to close.
//
// A refusal is the outcome rather than a question naming the resolved target.
// The reader is asked about every call already, and naming each target would
// make the question longer than the command it is about; a refusal says what
// was wrong in one word, which is what a model can act on.
//
// The arguments are not read as a shell would read them, since no shell is
// involved: there is no wildcard to expand and no variable to substitute. What
// this can judge is therefore each argument on its own, which is why a pattern
// such as *.go is passed through untouched and a path reaching out is refused.
func containedArgs(argv []string, dir string) error {
	root := resolvedRoot(dir)
	for _, arg := range argv {
		if escapesArg(arg, root) {
			return fmt.Errorf("refused: %q is outside the working directory at %s", arg, dir)
		}
	}
	return nil
}

// urlArgs checks the URL schemes carried by the arguments of a network reader.
//
// A URL is an argument rather than a path, so containedArgs holds one to the
// tree without knowing what it is. That check answers where a call begins and
// reaches rather than what it does with the argument it was given, which is
// already the case for git and for make. What it cannot answer is the question
// this one is: a file: URL is the one scheme with no network between the model
// and the filesystem, so `file:///etc/passwd` reaches the account through a
// program whose name suggests it is only fetching a page, and the reader
// approving `urlview` was approving a fetch.
//
// The rule is one scheme rather than a list of refused ones, so a scheme
// nobody thought of is refused with the rest rather than permitted by an
// omission. https is the one permitted, and http is refused beside file rather
// than being overlooked by it: an http URL carries the same content over a
// channel anyone on the path can change, so it is the same reach for less.
//
// An argument carrying no scheme is left alone. Both of these programs take a
// filename as well as a URL, and the name of a file may hold a colon without
// being a scheme at all, so an argument is read as a URL only where it is
// spelled as one. The two are told apart by requiring `://` rather than by a
// colon, since a scheme without an authority is not what a fetch is given.
func urlArgs(argv []string) error {
	for _, arg := range argv {
		scheme, ok := urlScheme(arg)
		if !ok {
			continue
		}
		if scheme != "https" {
			return fmt.Errorf("refused: %q is a %s URL and only https is permitted; "+
				"a file: URL has no network between the model and the filesystem, "+
				"and anything else is not a page", arg, scheme)
		}
	}
	return nil
}

// urlScheme reads the scheme of an argument spelled as a URL, and reports
// whether it was.
//
// The scheme is required to be followed by an authority, since `://` is what
// separates a URL from a path carrying a colon in it. The comparison is
// against a lowercased scheme, since a scheme is case insensitive and a model
// writing `HTTPS://` is naming the same scheme as one writing `https://`.
func urlScheme(arg string) (string, bool) {
	if strings.HasPrefix(arg, "-") {
		return "", false
	}
	head, _, found := strings.Cut(arg, "://")
	if !found || head == "" {
		return "", false
	}
	for i := 0; i < len(head); i++ {
		c := head[i]
		switch {
		case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z':
		case c >= '0' && c <= '9', c == '+', c == '-', c == '.':
			// A scheme may carry these after the first character, and are
			// refused by the same rule as any other, since naming them here
			// is what keeps this from reading part of a path as a scheme.
			if i == 0 {
				return "", false
			}
		default:
			return "", false
		}
	}
	return strings.ToLower(head), true
}

// resolvedRoot is the directory arguments are judged against, with its symlinks
// followed.
//
// Both sides of the comparison have to be resolved or the answer is wrong on a
// host where the directory is reached through one: on Darwin a temporary
// directory is /var/folders/... and resolves to /private/var/folders/..., and
// filepath.Rel reports an unresolved and a resolved path as two unrelated
// places. Every path inside the tree would then read as reaching out of it.
func resolvedRoot(dir string) string {
	resolved, err := filepath.EvalSymlinks(dir)
	if err != nil {
		return filepath.Clean(dir)
	}
	return resolved
}

// escapesArg reports whether an argument reaches outside root.
//
// An absolute path is not refused for being absolute. It is held to the same
// bound as any other, which is that it must land inside the directory the
// command runs in: the working directory of a test is a temporary directory,
// and an argument naming a file inside it by its full path is naming something
// this tool is meant to reach. It is the destination that is refused rather
// than the spelling, since a program given no path reads the working directory
// and one given ../.. does not.
//
// A relative argument is joined to root first and judged where it lands. A name
// carrying no separator is judged the same way rather than assumed safe, since
// it is joined to the directory by the program and lands there by that joining.
func escapesArg(arg, root string) bool {
	if arg == "" {
		return false
	}
	// An option is not a path. The dash is what separates the two, and an
	// argument carrying one is read as the flag or option it is however much
	// it resembles a path.
	if strings.HasPrefix(arg, "-") {
		return false
	}

	joined := arg
	if !filepath.IsAbs(joined) {
		joined = filepath.Join(root, joined)
	}

	// A pattern is not a path. It is passed to the program as written, since
	// no shell is read and the program is what expands it, and the directory
	// it is expanded against is inside the tree. A pattern is judged by the
	// directory leading to it, which is the part that can reach out.
	if head, ok := cutWildcard(joined); ok {
		return escapesArg(head, root)
	}

	resolved := resolveExisting(joined)

	rel, err := filepath.Rel(root, resolved)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return true
	}
	return false
}

// cutWildcard splits a path at the first name carrying a wildcard, returning
// the part before it.
func cutWildcard(path string) (string, bool) {
	for i := 0; i < len(path); i++ {
		if strings.ContainsAny(path[i:i+1], "*?[") {
			if i == 0 {
				return "", true
			}
			return filepath.Dir(path[:i]), true
		}
	}
	return "", false
}

// resolveExisting resolves joined as far as the filesystem allows, and returns
// where the names that do exist point at.
//
// A path with nothing behind it is the ordinary case rather than an edge: rm is
// asked to create what it is about to remove, and a model asks for a file it
// has not written yet. Resolving only the part that does exist is what makes
// the answer correct for those, since the symlinks a path would pass through
// are the ones on the part of it that is there.
func resolveExisting(joined string) string {
	if resolved, err := filepath.EvalSymlinks(joined); err == nil {
		return resolved
	}

	// Walk up until something does exist, then resolve that and put the names
	// that were walked past back on the end of it.
	rest := ""
	current := joined
	for {
		parent := filepath.Dir(current)
		if parent == current {
			return filepath.Clean(joined)
		}
		rest = filepath.Join(filepath.Base(current), rest)
		current = parent
		if _, err := os.Stat(current); err != nil {
			continue
		}
		resolved, err := filepath.EvalSymlinks(current)
		if err != nil {
			return filepath.Clean(joined)
		}
		return filepath.Join(resolved, rest)
	}
}
