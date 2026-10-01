# AGENTS.md

Editing notes and settled decisions for this repository. This file is the record
of why the project is shaped as it is. It is read before any change is made.

## Editing rules

These rules are carried over from `~/agents.md` and apply to every file in this
repository, including this one.

- Never use curly quotes, single or double.
- Never use em dashes.
- Never use contractions.
- Never use a curly apostrophe.
- Apostrophes indicating ownership are acceptable.
- Speak in third person, passive voice.
- Keep commit logs less than 72 characters long.

The wording rules apply to prose. Code identifiers, comments in code, commit
messages, and identifiers inherited from an external API are exempt, since
altering an API field name would break the wire format.

## Settled decisions

Each decision below was made by the maintainer. Each is recorded with the reason,
so that a later change does not silently reverse it.

### Language, license, and name

- Written in Go. Module path `github.com/glenjbarber/openrouter-cli`.
- BSD 3-Clause license, as committed in `LICENSE`.
- The executable is named `openrouter-cli`, matching the repository.

### Platforms

- FreeBSD is the primary target.
- Best effort is made for other Unix-like systems.
- macOS is a supported target.
- Windows is a possible later target and is not yet committed to.

### Configuration

- The configuration file format is JSON. JSON was chosen over YAML because Go
  has no standard-library YAML parser, and a configuration file is not a place
  where a dependency is worth taking. The accepted cost is that JSON permits no
  comments and demands strict punctuation when hand-edited.
- The file is searched at `~/.openrouter-cli.json`, then
  `~/.config/openrouter-cli/openrouter-cli.json`. The search does not merge, and
  the first match wins.
- `XDG_CONFIG_HOME` is honored in place of `~/.config`.
- A system-wide path was considered and removed. The world-readable permissions
  such a path requires would expose the API key to every local account.
- The file must be mode `0600`. Any other mode is a hard startup failure with an
  explanation, not a warning, because a permissive mode leaks the credential
  silently. The check is on the file alone, since a `0755` parent directory is
  not itself a credential exposure.
- The client creates the file at `0600`.
- `OPENROUTER_API_KEY` is required. `OPENROUTER_URL_BASE` is optional and
  overrides the default base URL of `https://openrouter.ai/api/v1`.
- The API key is never read from the process environment. The file is the only
  source. An environment variable of the same name is ignored even when it is set,
  which removes the class of failure in which a correct file is shadowed by a
  stale value elsewhere. This matters on the maintainer's host, where
  `OPENROUTER_API_KEY` is exported in the environment and would otherwise defeat
  the file.
- `go.mod` is ignored by git. The maintainer asked for this explicitly. The
  consequence is accepted and recorded: `go install github.com/glenjbarber/
  openrouter-cli@latest` cannot work without a `go.mod` on the remote, so
  installation by users is served by a FreeBSD port or a release artifact rather
  than by `go install`. If `go install` support is wanted later, the file must be
  un-ignored.
- An override carrying a path is used verbatim. The `/api/v1` suffix is appended
  only to a bare scheme and host, such as `http://localhost:3000`.
- Unknown keys are ignored, so a file written for a newer version stays readable
  by an older one.
- Invalid JSON, or a missing or empty key, is a fatal error rather than a silent
  fallback to a default.
- When no file is found, or the file lacks the setup-complete key, the client
  prompts for the API key and offers a skip, which records that setup was done
  and suppresses the prompt without storing a credential.
- Writing the configuration file occurs only during first-time setup. This is the
  single exception to the otherwise read-only treatment of the file.

### Model instruction files

- Instructions are read from a file in the current working directory before a
  request is sent.
- The candidate names are searched in alphabetical order, and the first match
  wins: `AGENTS.md`, `OPENROUTER.md`, `RULES.md`, `SHARED.md`.
- The search is not recursive and does not consult a parent directory.
- A missing file is not an error. An unreadable or non-UTF-8 file is an error,
  because silently discarding instructions the user believed were active is
  worse than refusing.
- A symlink is followed only when the target is on the same filesystem as the
  link, compared by device identifier. A cross-device link is refused with a
  diagnostic. The final target of a chain is what is compared, and a loop is
  reported rather than followed.

### Interface

- The interface is a terminal application in the manner of Codex, ChatGPT,
  Claude, and Perplexity.
- The status bar shows Provider, Model, Reasoning, Branch, Status, Approval
  Method, Context used, Tokens used in and out, and hostname.
- Commands and configuration options are completed with the Tab key.
- The interface behaves correctly with a terminal and mouse combination, and
  within tmux.
- Text is always copyable with the ordinary terminal selection gesture, and a
  selection yields plain text with no escape sequences or padding.
- Mouse reporting is not enabled unconditionally, because capturing the mouse
  intercepts the drag that begins a selection. It is disabled inside tmux by
  default, disabled whenever output is not a terminal, and suspended for the
  duration of a selection gesture.
- A chat may be backgrounded without losing its conversation, and a new ephemeral
  chat started alongside it.
- An ephemeral chat is not persisted and is not carried into a later chat.
- Backgrounded chats are retained within the running session.

### Usage meter

- The usage and quota meter is native to the application and is rendered
  in-process.
- It is not written into the tmux status bar, and no external helper script or
  scheduled task maintains it. The scripts and the crontab entry that served this
  purpose on the host are outside this repository and are left untouched.
- Data is read from the key endpoint.

### Toolchain

- These tools are tracked as module tool dependencies in `go.mod` and are run
  through `go tool`: `staticcheck`, `errcheck`, `gosec`, `govulncheck`,
  `protoc-gen-go`, and `protoc-gen-go-grpc`.
- `protoc` is a build dependency, never a runtime dependency. Generated code is
  committed, so it is needed only when a `.proto` file changes.
- The two generators are currently idle, since the client speaks REST and no
  schema exists. They are candidates for removal if REST-only is confirmed.
- `staticcheck` is tracked as `honnef.co/go/tools`. The path
  `github.com/golangci/golangci-lint/v2/cmd/staticcheck` does not exist, and
  using it fails.
- `protoc-gen-go-grpc` is tracked as `google.golang.org/grpc/cmd/protoc-gen-go-grpc`,
  which is a module in its own right. The parent module `google.golang.org/grpc`
  does not contain that command, so tracking the parent is not sufficient.
- The race detector needs a C compiler, so `cc` is a build dependency.

### Layout

The tree follows what a FreeBSD port expects.

- `cmd/openrouter-cli` holds the `main` package.
- `internal/` holds packages that are not importable from outside the module.
- `doc/` holds manual pages.
- `files/` holds port auxiliary files, including the pkg-descr.
- `test/` holds test scripts and fixtures.
- `.github/workflows/` holds CI definitions.

A port also expects a `Makefile` with port metadata, plus `distinfo` and
`pkg-descr`. Those are not yet written.

## Open decisions

These are unsettled. Each is listed so that it is not mistaken for a decision.

- The name of the setup-complete key, and whether a skipped configuration is
  recorded distinctly from a completed one. A skip marks setup done while storing
  no credential, so without a distinct state the client believes itself
  configured and fails later at the point of use.
- Precedence between the configuration file and command-line flags. The
  environment is settled and is not consulted at all, so that question is
  narrowed rather than open.
- The final status bar field set, its order, and whether the order is
  configurable.
- The first release scope, and the split between interactive and scripted use.
- The terminal library, which is the largest remaining dependency decision and
  governs mouse, tmux, and alternate-screen handling.
- The minimum supported Go version, currently stated as 1.26 in the README and
  taken from the host toolchain rather than decided.

## Working notes

- The local git identity is unset. The clone did not carry the maintainer's
  identity, and a commit will not be made until it is set. The value to use is
  `Glen Barber <glen.j.barber@gmail.com>`, matching the initial commit, set with
  `--local` so the global configuration is untouched.
- The first draft of the API client, written before the design discussion, was
  discarded at the maintainer's instruction and is not in the repository.
- `AGENTS.md` is tracked in this repository. The file at `~/agents.md` is a
  separate, personal set of notes, and the two are not kept in sync.
- A C compiler and `gmake` are confirmed present on the host, along with `protoc`
  29.6, installed as the FreeBSD `protobuf` package. The package database does
  not list the binary path, but the binary is present and reports its version, so
  the earlier failure to find it reflected an incomplete install at the time.
