// Command openrouter-cli is a terminal client for the OpenRouter.AI API.
package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/glenjbarber/openrouter-cli/internal/bootstrap"
	"github.com/glenjbarber/openrouter-cli/internal/config"
	"github.com/glenjbarber/openrouter-cli/internal/tui"
)

// version is overridden at build time with -ldflags "-X main.version=...".
var version = "0.0.0-dev"

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "openrouter-cli: %v\n", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	opts, err := parseFlags(args)
	if err != nil {
		return err
	}
	if opts.showHelp {
		usage(os.Stdout)
		return nil
	}
	if opts.showVersion {
		fmt.Printf("openrouter-cli %s\n", version)
		return nil
	}

	// A named bootstrap file is loaded before the configuration, so that a
	// document which cannot be read is reported before any credential work is
	// attempted. It is loaded once here and handed to the session below, since
	// reading it a second time would let the document that was validated be
	// different from the one that is seeded.
	var doc *bootstrap.Document
	if opts.bootstrap != "" {
		loaded, err := bootstrap.Load(opts.bootstrap)
		if err != nil {
			return err
		}
		doc = loaded
	}

	// A default is written when no configuration exists, so that the path and
	// the file mode are established before the key is ever entered. An
	// existing file is left alone.
	if _, err := config.InstallDefault(); err != nil {
		return err
	}

	// Configuration is resolved on every start so that a missing or
	// misconfigured file is reported before any work is attempted.
	cfg, err := config.Load()
	if err != nil {
		// A file with no key is not a failure. The interface opens and
		// reports the absence as an ordinary message, so that the user can
		// see what is wrong and read /help. Refusing to open would leave
		// nothing on screen to explain it.
		var keyErr *config.ErrNoAPIKey
		if !errors.As(err, &keyErr) {
			return err
		}
		// The model, the endpoint, and the preferences are preserved from the
		// file even though the key is not, so that a file carrying a preference
		// is not treated as unread.
		cfg = config.EmptyMouse(keyErr.Model, keyErr.Mouse, keyErr.Bell)
		cfg.SetColor(keyErr.Color, keyErr.ColorTheme, keyErr.ColorNote)
		if keyErr.URLBase != "" {
			cfg.URLBase = keyErr.URLBase
		}
		cfg.GitHubToken = keyErr.GitHubToken
		cfg.NotionToken = keyErr.NotionToken
		cfg.GoogleDriveCredentials = keyErr.GoogleDriveCredentials
	}

	// The current directory is read only with the reader's permission. The
	// answer is remembered in the configuration file, and any doubt, including
	// input that is not a terminal, leaves the directory untrusted.
	if err := requireTrust(); err != nil {
		return err
	}

	// The interface is entered only when both ends are a terminal. A
	// redirected run reports why and stops rather than writing a frame into
	// the capture, since escape sequences in a file or a pipe are noise.
	return interface_(os.Stdout, os.Stdin, cfg, opts, doc)
}

// interface_ starts the interactive session.
//
// The name carries a trailing underscore because interface is a keyword.
// doc is the bootstrap document read once at startup, or nil when none was
// named. It travels with the options rather than being read again, so that the
// document that was validated is the document the session is seeded with.
func interface_(out, in *os.File, cfg *config.Config, opts options, doc *bootstrap.Document) error {
	session, err := tui.Start(out, in, "openrouter-cli")
	if err != nil {
		if errors.Is(err, tui.ErrNotTerminal) {
			return errors.New("the interactive interface needs a terminal: " +
				"stdout and stdin must both be a terminal")
		}
		return err
	}
	defer session.Close()

	// The credential is installed on the session rather than used here, so
	// that the interface opens even when the key is absent and reports the
	// absence as an ordinary message rather than refusing to open.
	session.Configure(cfg)
	session.SetBell(cfg.Bell)
	// Color is handed over and drawn from the next paint. A theme value that
	// was not valid has already been dropped by the loader, and the one line
	// note is shown once so that the fallback is not silent.
	session.SetColor(cfg.Color)
	session.SetColorTheme(cfg.ColorTheme)
	// /theme records the ground in the same file /color records its own, so
	// that a reader who has chosen once is not asked again on every run. The
	// path is found again at the time of the command, for the reason the
	// color saver finds it again: a file made during setup is then seen.
	session.SetTheme(func(dark bool) error {
		path, ok := config.ConfigPath()
		if !ok {
			return config.ErrNoConfigFile
		}
		return config.WriteTheme(path, dark)
	})
	// /color records its choice in the file the loader read, found again at the
	// time of the command so that a file made during setup is seen.
	session.SetColorSaver(func(on bool) error {
		path, ok := config.ConfigPath()
		if !ok {
			return config.ErrNoConfigFile
		}
		return config.WriteColor(path, on)
	})
	// /bell and /verbosity record their choice in the file the loader read,
	// on the same terms as /color: found again at the time of the command
	// so that a file made during setup is seen.
	session.SetBellSaver(func(on bool) error {
		path, ok := config.ConfigPath()
		if !ok {
			return config.ErrNoConfigFile
		}
		return config.WriteBell(path, on)
	})
	session.SetVerbositySaver(func(level int) error {
		path, ok := config.ConfigPath()
		if !ok {
			return config.ErrNoConfigFile
		}
		return config.WriteVerbosity(path, level)
	})
	if cfg.ColorNote != "" {
		session.Note("%s", cfg.ColorNote)
	}
	if cfg.ProviderNote != "" {
		session.Note("%s", cfg.ProviderNote)
	}

	// A marker left by an earlier session is adopted before anything is sent,
	// so that a session started while the mode is in force records nothing.
	// The marker is written by /cognito and was never read back, so a reader
	// who turned the mode on and left recorded every exchange of the next
	// session while the mode was still meant to be holding nothing.
	if err := session.AdoptCognito(); err != nil {
		return err
	}

	// Mouse reporting is not turned on unless it is asked for, because a
	// terminal that reports events takes the drag that begins a selection
	// away from the terminal, and text that cannot be selected is worse than
	// a wheel that does nothing.
	if mouseWanted(opts, cfg) {
		session.SetMouse(true)
	}

	if opts.bootstrap != "" {
		// The document is reported inside the frame rather than cleared, so
		// that a startup message is not lost behind the first repaint.
		session.Seed(doc)
	}

	if err := session.Run(); err != nil && !errors.Is(err, tui.ErrQuit) {
		return err
	}
	return nil
}

// mouseWanted reports whether mouse reporting is asked for at startup.
//
// Neither the flag nor the file is honoured inside tmux. The drag that begins a
// selection is the gesture a nested terminal is most often used for, so a
// session inside tmux turns reporting on with /mouse and not before. Outside
// tmux either request is enough.
func mouseWanted(opts options, cfg *config.Config) bool {
	if tui.InTmux() {
		return false
	}
	return opts.mouse || cfg.Mouse
}

// options holds the parsed command line.
type options struct {
	bootstrap   string
	mouse       bool
	showHelp    bool
	showVersion bool
}

// parseFlags reads the command line.
//
// The flag set stops at the first non-flag argument so that a subcommand name
// is never mistaken for the value of a flag.
//
// The flag set writes nothing itself. It would otherwise report a bad flag on
// stderr and return the same error, which the caller then reports again, so one
// mistake reached the reader twice. The error is returned instead and named
// once.
func parseFlags(args []string) (options, error) {
	var opts options

	fs := flag.NewFlagSet("openrouter-cli", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.StringVar(&opts.bootstrap, "bootstrap", "",
		"start the session from this Markdown or JSON document")
	fs.BoolVar(&opts.mouse, "mouse", false,
		"turn on mouse reporting, so the wheel scrolls the reply pane")
	fs.BoolVar(&opts.showVersion, "version", false, "print the version and exit")
	fs.BoolVar(&opts.showHelp, "help", false, "print this message and exit")

	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			opts.showHelp = true
			return opts, nil
		}
		return opts, err
	}

	// A bare word is a subcommand rather than a flag value. These are kept
	// from the first release, so a script written against them still runs.
	if rest := fs.Args(); len(rest) > 0 {
		switch rest[0] {
		case "version":
			opts.showVersion = true
		case "help":
			opts.showHelp = true
		default:
			return opts, fmt.Errorf("unknown command %q: run openrouter-cli -help "+
				"for the commands and options", rest[0])
		}
	}
	return opts, nil
}

func usage(w *os.File) {
	fmt.Fprint(w, `openrouter-cli - a terminal client for the OpenRouter.AI API

Usage:
  openrouter-cli [option]

Options:
  --bootstrap FILE   Start the session from FILE. The extension selects the
                     format: .md is used as written, .json is decoded, .db
                     is a conversation saved with /save and is resumed from.
  --mouse            Turn on mouse reporting, so the wheel scrolls the reply
                     pane. It is off by default, and off inside tmux even when
                     the configuration asks for it, since a terminal that
                     reports the mouse cannot also be dragged to select text.
  -version           Print the version and exit.
  -help              Print this message and exit.

Commands, typed inside the interface:
  /connect           Test the connection and report the key.
  /key               Report the usage against the key.
  /models            List the models the endpoint offers.
  /model NAME        Choose the model to send to.
  /info              Report the session settings.
  /bell              Ring the terminal bell when a reply arrives.
  /cognito           Record nothing, on or off.
  /delegate Q        Ask a question alongside, without recording it.
  /btw               Start a thread branched from this conversation.
  /main              Leave the thread and return to the conversation.
  /new               Clear the conversation.
  /compact           Summarise the conversation and carry on.
  /save NAME         Write the conversation to a file of its own.
  /load NAME         Resume a conversation saved with /save.
  /tools            Report the tools the model is given, and where they reach.
  /mouse             Turn mouse reporting on or off, for wheel scrolling.
  /clear             Clear the pane.
  /quit, /exit       Leave the interface.

Ctrl-C abandons the line being composed, or leaves the interface when the line
is empty. Ctrl-D leaves when the line is empty.

The model is given tools: it can read, write and list files under the working
directory the client was started in, and can run git there. The git tool is
read-only. /tools reports exactly what is on offer. Nothing else is reachable,
and a thread or a cognito session offers the model nothing at all, since those
record nothing.
`)
}

// requireTrust refuses to continue unless the current directory is trusted.
func requireTrust() error {
	dir, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("locating the current directory: %w", err)
	}
	path, _ := config.ConfigPath()
	interactive := false
	if info, err := os.Stdin.Stat(); err == nil {
		interactive = info.Mode()&os.ModeCharDevice != 0
	}
	ok, err := config.EnsureTrusted(path, dir, os.Stdin, os.Stderr, interactive)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("%w: %s", config.ErrNotTrusted, dir)
	}
	return nil
}
