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
- Every commit message carries the trailer `Co-Authored-By: Space Bunny Alpha`.

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
- `OPENROUTER_MODEL` is optional and sets the model a session starts with, so
  that `/model NAME` is not needed on every run. The key is named after the
  model field that would otherwise be introduced, for the same reason as the
  others: a value is transferable between the file and the environment.
- A model taken from the file is adopted only when the session has not chosen
  one, so an explicit choice always wins over the file.
- The model survives a missing key. A file carrying a preference must not appear
  unread merely because it has no credential, and a session cannot reach a
  model without one anyway.
- A model identifier is trimmed of surrounding whitespace, since a stray space
  would otherwise be sent to the backend as part of the identifier.
- The API key is never read from the process environment. The file is the only
  source. An environment variable of the same name is ignored even when it is set,
  which removes the class of failure in which a correct file is shadowed by a
  stale value elsewhere. This matters on the maintainer's host, where
  `OPENROUTER_API_KEY` is exported in the environment and would otherwise defeat
  the file.
- `go.mod` is tracked, and was briefly ignored by explicit instruction before
  that was reversed. The reversal is recorded because the reason is the lesson:
  an ignored module file makes a fresh clone unbuildable, breaks continuous
  integration, and makes `go install` impossible, so no other user could build
  the project. A module file is part of a Go program's source, not a local
  artifact.
- `go.sum` is not ignored either, so a dependency added later yields a committed
  checksum file rather than a silently unpinned build. It is currently absent
  because the module has no external dependencies.
- The `go` directive is a minor version, `go 1.26`, not a patch pin. A patch pin
  is unusual and needlessly excludes users on a lower patch release.
- An override carrying a path is used verbatim. The `/api/v1` suffix is appended
  only to a bare scheme and host, such as `http://localhost:3000`.
- Unknown keys are ignored, so a file written for a newer version stays readable
  by an older one.
- Invalid JSON, or a missing or empty key, is a fatal error rather than a silent
  fallback to a default.
- When no file is found, or the file lacks the setup-complete key, the client
  prompts for the API key and offers a skip, which records that setup was done
  and suppresses the prompt without storing a credential.
- The configuration file is written at startup when no file exists, and again
  during first-time setup. These are the only two circumstances in which it is
  written. The rest of the time the file is treated as read-only.
- The default written at startup sets `OPENROUTER_URL_BASE` only. No key is
  written, because a placeholder key would be indistinguishable from a real one
  and would be sent to the backend.
- The default is written only when the file is absent. An existing file is left
  exactly as it is, whether complete, empty, malformed, or missing the key.
- A merge was considered and rejected. A partial merge of a credential file can
  produce a file that parses but is wrong, and a wrong credential fails later at
  the point of use rather than where it was introduced. Doing nothing is the
  reversible outcome, so doing nothing is what happens.
- The default is created at `0600`, since the loader refuses any other mode and a
  default written at a permissive mode would be rejected by the client that
  wrote it.
- The creation is exclusive, so a file that appears between the existence check
  and the write is not overwritten.
- A directory at the configuration path is reported as an error rather than
  treated as an absent file, since the path could not be written in any case and
  a silent skip would suggest the configuration was in place.

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

### Bootstrap documents

- A session is started from a bootstrap document named with `--bootstrap`, as in
  `--bootstrap MEMORY.md`.
- The extension alone selects the format. A `.md` file is passed to the model
  verbatim, since it is already prose. A `.json` file is decoded as a structured
  document.
- The JSON form carries an `instructions` field, which is required, plus
  optional `name` and `description` fields. Unknown fields are ignored, so a
  document written for a newer version stays readable by an older one. This
  matches the treatment of the configuration file.
- The content of a file is never probed to guess a format, because a guess that
  is wrong is worse than a refusal.
- A missing file, an unreadable file, and content that is not valid UTF-8 are
  each errors. This is deliberately stricter than the automatic instruction
  search, where an absent file is an ordinary outcome, because naming a file on
  the command line is an assertion that it exists and can be read.
- A document carrying no instructions is refused, since a session begun with
  none would fail silently until the model behaved as though it had never been
  told anything.
- A symlink is resolved under the same rule as an instruction file, so the
  device-identifier comparison and the cross-device refusal are shared rather
  than restated.
- The document is read once at startup rather than re-read per request.
- The bootstrap document is loaded before the configuration, so that an
  unreadable document is reported before any credential work is attempted.

### Interface

- The interface is a terminal application in the manner of Codex, ChatGPT,
  Claude, and Perplexity.
- No terminal library is used. The terminal is driven through `syscall` on
  FreeBSD, using `TCGETA` and `TCSETA` for termios, `TIOCGWINSZ` for the size,
  and escape sequences for the alternate screen and the cursor. This closes the
  open decision below, and it was taken because the copyable-text requirement
  needs direct control of mouse reporting, which the higher-level candidates
  take by default.
- The alternate screen is used, so the interface does not scroll the shell
  history and the previous contents are restored on exit.
- A terminal is detected by a termios read rather than by a stat on the mode,
  since a stat cannot tell a character device that is a terminal from one that
  is not.
- The interface is entered only when both stdout and stdin are a terminal. A
  redirected run reports why and stops, since escape sequences written into a
  pipe or a file are noise in the capture.
- Raw mode is required because keys are read as they are pressed. The prior
  terminal state is saved and restored on exit, and a signal handler restores it
  before the process leaves, so that a crash cannot leave the terminal with no
  echo and no line discipline.
- The status bar order is fixed rather than configurable, because a bar that
  reorders itself between runs cannot be read at a glance.
- A status field with no value yet is rendered as a dash rather than being
  hidden, so the layout does not shift as values arrive and a missing value is
  visible rather than ambiguous.
- A field that has no source in this client is removed rather than shown as a
  permanent dash. `Branch` and `Reasoning` were removed on the maintainer's
  instruction. `Approval` was removed because it implies a permission system
  for tool calls, and the client has no tools and executes nothing, so there
  would be nothing to approve. A dash that can never be filled is noise.
- The provider is a constant rather than a derived value, since the endpoint is
  the only one the client speaks to.
- The state reads `Working` while a request is in flight and `idle` otherwise,
  restored on every exit including a failure.
- The field named `Credits` carries the allowance the key endpoint reports as a
  figure against a limit. It is not the conversation context, which is the
  share of the model window a message occupies. The name `Context` is left free
  for that, and is not yet filled.
- Token accounting is requested explicitly in the request body, since an
  endpoint that is not asked for it sends nothing and the counters would stay
  blank. It arrives on the final chunk, so it is a pointer: a reported zero and
  an absent value are different, and an absent value must not clear an
  accumulated total.
- The counters accumulate across the session rather than describing one
  exchange.
- When the bar is too narrow, fields are dropped rather than allowed to wrap,
  since a wrapped bar pushes the input line off the screen. The order is by how
  much a reader loses, not by position: the counters go before the allowance,
  and the host goes before either, since it does not change while the session
  runs. Dropping by position removed whichever field was last, which was the
  token count.
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

### API

- The API is spoken over REST with `net/http`. No dependency is used for it.
- A completion is `POST /chat/completions` with a bearer credential. The
  credential is sent as a header and is never written to a log or a diagnostic.
- A reply is read as a server-sent event stream. The terminating `[DONE]` marker
  is required rather than assumed, since a stream that ends without it was cut
  short and must not be presented as a complete reply. The text received before
  a failure is kept, because it is usually more useful than an error alone.
- A comment line in the stream is a keep-alive and carries no payload.
- The response body on success is the stream itself, so the status is examined
  without reading it. Reading it would consume the reply before the parser saw
  it.
- `/connect` calls the key endpoint rather than a completion, since it is cheap
  and it distinguishes a rejected key from a rejected model.
- A missing key is not a startup failure. The interface opens and reports the
  absence, because refusing to open would leave nothing on screen explaining
  why. A file that cannot be parsed remains a failure.
- A request with no credential is caught before it is sent. OpenRouter answers a
  request with no credential the same way it answers an invalid one, reporting
  the key as rejected when in truth none was sent, so the check is made locally
  rather than by reading the diagnostic.
- The bootstrap document is applied as a system turn, so it governs every request
  in the session rather than one.
- A user turn travels with the request but is not recorded until a reply arrives,
  so a failed request leaves no half exchange for the next one to replay.
- `/new` clears the conversation and keeps the bootstrap document, since losing
  it would silently change how the model behaves.
- A multi-line reply occupies one pane row per line, since a reply carrying
  embedded newlines would otherwise push the frame down the screen.
- Each pane row is cleared before it is written. Without that, a repaint shorter
  than the frame before it leaves the tail of the longer one visible, so a
  reply appears twice.

### Getting started

- The reply pane carries a hint while it is empty, naming whatever is missing: an
  absent key, an unselected model, or simply that a message can be typed. The
  hint is removed once a conversation has started, since it is then in the way.
- Without the hint a first run shows an empty pane and a prompt, which gives no
  indication that plain text is the message and that a model must be selected
  first.

### Echo

- The terminal is in raw mode with echo disabled, so the composed line is drawn
  by the interface rather than by the line discipline. The line editor reports
  the line after every keystroke and the frame is redrawn from that report.
- Without this the keystrokes are held until the line is submitted and appear to
  do nothing at all, which reads as a frozen interface rather than as a missing
  feature.
- A multibyte character is reported only once it is whole, so a character is
  never shown half formed.
- The report callback is optional. A non-interactive reader passes none, and the
  editor must not depend on it.

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
- `Makefile` carries the port metadata and the developer targets.

A port also expects `distinfo` and `pkg-descr`. The `pkg-descr` is held under
`files/`, which is empty, and the `distinfo` is not yet written.

### Build

- The `Makefile` is written in the syntax common to BSD make and GNU make,
  because the port host uses BSD make while a developer may reach for GNU make.
  No GNU-only construct is used.
- `$(shell ...)` is avoided for the same reason. The binary is therefore
  rebuilt whenever the target is requested rather than only when a dependency
  is newer, since a source list cannot be expanded into a dependency.
- The Go toolchain version is not repeated in the `Makefile`. The directive in
  `go.mod` is the source of it, and a second copy would drift.
- Build output is written to `build/`, which is ignored, so that the binary
  does not sit beside the directories a port expects.

## Open decisions

These are unsettled. Each is listed so that it is not mistaken for a decision.

- The name of the setup-complete key, and whether a skipped configuration is
  recorded distinctly from a completed one. A skip marks setup done while storing
  no credential, so without a distinct state the client believes itself
  configured and fails later at the point of use. The startup default sharpens
  this, since a freshly installed file also holds no credential, so the skipped
  case and the not-yet-entered case have to be told apart. The startup default makes this
  more visible rather than less, since a freshly installed file also holds no
  credential, so the two cases need to be told apart.
- Precedence between the configuration file and command-line flags. The
  environment is settled and is not consulted at all, so that question is
  narrowed rather than open. The bootstrap document is unaffected, since it
  carries instructions rather than configuration.
- Whether the JSON bootstrap form should carry fields beyond `instructions`,
  `name`, and `description`, such as tool permissions or a model preference.
  Three fields were written on the assumption that instructions are the only
  thing a bootstrap document needs to express.
- The first release scope, and the split between interactive and scripted use.
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
