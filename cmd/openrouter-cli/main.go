// Command openrouter-cli is a terminal client for the OpenRouter.AI API.
package main

import (
	"fmt"
	"os"

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
	if len(args) > 0 {
		switch args[0] {
		case "version", "--version", "-v":
			fmt.Printf("openrouter-cli %s\n", version)
			return nil
		case "help", "--help", "-h":
			usage(os.Stdout)
			return nil
		}
	}
	usage(os.Stdout)

	// Configuration is resolved on every start so that a missing or
	// misconfigured file is reported before any work is attempted.
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	_ = cfg
	return nil
}

func usage(w *os.File) {
	fmt.Fprint(w, `openrouter-cli - a terminal client for the OpenRouter.AI API

Usage:
  openrouter-cli [command]

Commands:
  version   Print the version and exit.
  help      Print this message and exit.

The interactive interface is not yet implemented.
`)
}
