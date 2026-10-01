// Command openrouter-cli is a terminal client for the OpenRouter.AI API.
package main

import (
	"errors"
	"flag"
	"fmt"
	"os"

	"github.com/glenjbarber/openrouter-cli/internal/bootstrap"
	"github.com/glenjbarber/openrouter-cli/internal/config"
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
	// attempted.
	if opts.bootstrap != "" {
		doc, err := bootstrap.Load(opts.bootstrap)
		if err != nil {
			return err
		}
		fmt.Fprintf(os.Stderr, "bootstrap: %s (%s)\n", doc.Path, doc.Format)
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
		return err
	}
	_ = cfg
	return nil
}

// options holds the parsed command line.
type options struct {
	bootstrap   string
	showHelp    bool
	showVersion bool
}

// parseFlags reads the command line.
//
// The flag set stops at the first non-flag argument so that a subcommand name
// is never mistaken for the value of a flag.
func parseFlags(args []string) (options, error) {
	var opts options

	fs := flag.NewFlagSet("openrouter-cli", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	fs.StringVar(&opts.bootstrap, "bootstrap", "",
		"start the session from this Markdown or JSON document")
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
			return opts, fmt.Errorf("unknown command %q", rest[0])
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
                     format: .md is used as written, .json is decoded.
  -version           Print the version and exit.
  -help              Print this message and exit.

The interactive interface is not yet implemented.
`)
}
