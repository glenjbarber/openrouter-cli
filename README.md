# openrouter-cli

![OS](https://img.shields.io/badge/OS-FreeBSD-red.svg?logo=freebsd&logoColor=white)

![Architecture](https://img.shields.io/badge/arch-amd64-blue)
![Architecture](https://img.shields.io/badge/arch-arm64-blue)
![Architecture](https://img.shields.io/badge/arch-aarch64-blue)

[![License](https://img.shields.io/github/license/glenjbarber/openrouter-cli?color=blue)](LICENSE)

![Go Version](https://img.shields.io/github/go-mod/go-version/glenjbarber/openrouter-cli)
[![Go Reference](https://pkg.go.dev/badge/github.com/glenjbarber/openrouter-cli.svg)](https://pkg.go.dev/github.com/glenjbarber/openrouter-cli)

![Last Commit](https://img.shields.io/github/last-commit/glenjbarber/openrouter-cli)
[![Go CI](https://github.com/glenjbarber/openrouter-cli/actions/workflows/go-ci.yml/badge.svg?branch=main)](https://github.com/glenjbarber/openrouter-cli/actions/workflows/go-ci.yml)

A command-line client for the OpenRouter.AI API, written in Go.

`openrouter-cli` provides an interactive terminal interface in the style of Codex,
ChatGPT, Claude, and Perplexity, along with non-interactive commands suitable for
use in scripts and pipelines.

## Status

This project is in early development. The first release has not been scoped, and
no release has been published. The README is updated as decisions are settled.

## Requirements

Go 1.26 or later is required to build from source.

FreeBSD is the primary target platform. Best effort is made for other Unix-like
systems, and macOS is a supported target. Windows support is planned.

### Build dependencies

Build dependencies are required to compile the client and to run the scanning
tools in the test suite. They are not required to run a released binary.

| Dependency          | Purpose                                              |
| ------------------- | ---------------------------------------------------- |
| `go`                | The Go toolchain.                                    |
| `protoc`            | The protocol buffer compiler, used by the generators. |
| `gmake`             | GNU Make, used by the test entry point.              |
| `cc`                | A C compiler, required for `go test -race`.          |

On FreeBSD the corresponding packages are `go`, `protobuf`, `gmake`, and `clang`.

The generated code is committed to the repository, so `protoc` is required only
when a `.proto` file is added or changed, and not for an ordinary build.

### Runtime dependencies

A released binary is statically linked and has no runtime dependencies beyond a
supported platform and a C library on Windows. The following are required at
runtime, and are not build dependencies.

| Requirement          | Detail                                               |
| -------------------- | ---------------------------------------------------- |
| Configuration file   | A JSON file holding the API key, required at startup. |
| Network access       | Reachability of the OpenRouter.AI API.                |
| Supported terminal   | A terminal capable of the sequences the client uses.  |

## Installation

Installation instructions are pending and are added once a release is published.

## Usage

Running the client with no options opens the interactive interface:

```sh
openrouter-cli
```

The interface draws a frame on the alternate screen, so the shell history and
whatever was on the screen before are left untouched and restored on exit.

The frame opens with a header: the title, a rule, then the status bar. The bar
lives at the top rather than the foot because it sits outside the scrolled
slice, so the credits, the context share, and the token counts stay in view while
reading earlier output. At the foot they scrolled away at exactly the moment
they were wanted.

```
openrouter-cli
──────────────────────────────────────────────────────────────────────────
Provider: openrouter.ai | Model: stealth/space-bunny-alpha | Status: idle | Context: 12% | In: 1.2k | Out: 5.7k

> what does this project do?
```

`--mouse` turns on mouse reporting for the run, so the wheel scrolls the reply
pane. It is off by default, since a terminal that reports the mouse cannot also
be dragged to select text. See [Scrollback](#scrollback).

The status bar sits above the input line, carrying Provider, Model, Status,
Credits, the token counters, and the hostname:

```
Provider: openrouter.ai | Model: stealth/space-bunny-alpha | Status: idle | Credits: 0.42/5 | Context: 12% | In: 1.2k | Out: 5.7k | claude1.lab3.home.arpa
```

`Status` reads `Working` while a request is in flight. `Credits` is the
remaining allowance reported against the API key. `Context` is the share of the
model window the conversation occupies, which is what says when a compaction is
coming. The two are separate measures: one is money, the other is window. The
token counters accumulate across the session.

A field with no value yet is shown as a dash. Fields with no source in the
client are not shown at all: `Branch`, `Reasoning`, and `Approval` were removed,
since the client has no branching, no reasoning parameter, and no tool execution
to approve.

When the terminal is too narrow, fields are dropped rather than allowed to wrap.
The order is by how much is lost: the token counters go first, then the
allowance, and the hostname last, since it does not change while the session
runs.

The interface requires both stdout and stdin to be a terminal. A redirected run
reports that and stops rather than writing escape sequences into the capture. A
missing API key is not a reason to refuse to open: the interface opens and
reports the absence, so that the cause is on screen.

A message is typed as plain text and sent with Enter, with no command needed:

```
> what does this project do?
```

While the pane is empty it shows what is still missing: an absent key, or an
unselected model, or simply that a message can be typed. The hint disappears
once a conversation has started.

Commands begin with a slash, so that they do not collide with text sent to the
model:

| Command          | Effect                                                |
| ---------------- | ----------------------------------------------------- |
| `/connect`       | Test the connection and report the key.              |
| `/key`           | Report the usage against the key.                    |
| `/models`        | List the models, filtered as it is typed.           |
| `/search`        | Search the conversation, filtered as it is typed.   |
| `/freemodels`    | List the models that cost nothing to call.          |
| `/model NAME`    | Choose the model. Without an argument, report it.    |
| `/info`          | Report the model, the endpoint, and whether a key is set. |
| `/bell`          | Ring the terminal bell when a reply arrives, on or off. |
| `/cognito`       | Record nothing from now on, or record again.        |
| `/verbose`       | Report the shape of each streamed turn, on or off.  |
| `/delegate Q`    | Ask a question alongside, without recording it.     |
| `/btw`           | Start a thread branched from this conversation.      |
| `/main`          | Leave the thread and return to the conversation.    |
| `/new`           | Clear the conversation, keeping the bootstrap document. |
| `/compact`       | Summarise the conversation and carry on from the summary. |
| `/mouse`         | Turn mouse reporting on or off, for wheel scrolling. |
| `/clear`         | Clear the pane.                                      |
| `/help`          | List the commands.                                   |
| `/quit`, `/exit` | Leave the interface.                                 |

`/models` opens the catalogue and narrows it as the filter is typed, without a
separate submit. Escape leaves the listing and Enter chooses what the filter
names. `/freemodels` is the same listing narrowed to the models that cost nothing,
which are the ones whose quoted prompt and completion prices are both zero. A
model whose price is not reported is not treated as free.

`/search` filters the conversation as the query is typed, without a separate
submit. Each line carrying the word is listed with the column the match fell
at, and nothing is highlighted, so a selection copied out of the pane is plain
text. Enter moves the view to the newest match and leaves the search open, and
Escape closes it and puts the view back where it was.

`/connect` contacts the key endpoint rather than running a completion, since it
is cheap and it distinguishes a rejected key from a rejected model, which is the
first thing worth knowing when nothing works.

A reply streams into the pane a token at a time rather than appearing all at
once, so that a slow model does not look idle. A twiddle turns beside the reply
while a request, a compaction, or a connection test is in progress, and the
status field reads `Working` for the same period. A stream that fails partway keeps
the text received before the failure and reports the error beneath it.

`/new` clears the conversation but keeps the bootstrap document in force, since
losing it would silently change how the model behaves.

`/delegate QUESTION` asks a question from a copy of the conversation while the
main one stays open. The prompt remains live, so a message can be typed while
the answer is still arriving. The answer is shown in the pane and is not added to
either conversation: a delegate records nothing, which is what lets it be used
while `/cognito` is on.

`/btw` starts a thread branched from the current conversation. It begins with
the history it branched from, so the model has the context, but records nothing
that is said in it. `/main` leaves it and restores the conversation exactly as
it was. A thread is not written anywhere and is not carried into a later
session.

`/cognito` records nothing at all. Switching it off discards whatever was
recorded before it went on, and reports how many exchanges that was, so the model
does not carry work the mode was meant to drop. The opening instructions survive. It is recorded by a marker file in the home
directory holding a process identifier and nothing else, so that a marker left
behind by a crash is noticed at the next start and reported rather than
honoured silently.

The terminal bell is off unless asked for. `/bell` turns it on or off at
runtime, and `OPENROUTER_BELL` in the configuration file sets it for every
session. The bell is rung when a reply has finished arriving rather than when
the request was sent, and it is written only when the output is a terminal, so a
redirected run carries no stray control character.

`/verbose` is off unless asked for. It adds one line beneath each reply
describing how that turn arrived: the number of deltas, the character count, the
reason the model gave for stopping, and whether the token accounting was
reported. A turn that was cut short or that failed says so, and one that carried
no accounting says that too, so a line is never read as a complete exchange
when it was not. It is a display preference and changes nothing about what is
asked for or what is recorded, so it can be left on while working.

A long conversation is compacted automatically. Once it passes 75% of the
model's window, the next message triggers a summarisation before the request is
sent, since a request past the window is refused outright and the turn is lost.
The summary replaces the earlier turns as a system turn, so it governs every
later request, and the opening instructions are kept. `/compact` runs the same
work on demand.

Conversation size is estimated at four characters per token rather than counted,
since exact counting needs the model tokenizer. The estimate is deliberately
generous, so compaction begins before a request would fail rather than after.
A model whose window the endpoint does not report falls back to a common one.

### Key handling

A key sequence is consumed whole, so pressing an arrow, Home, End, or Delete
does not end the line. The up and down arrows walk the input history and do
nothing at all when there is none, stopping at each end rather than wrapping.
Walking forward past the newest resumes the line that was being composed. The
left and right arrows are reserved for input toggles and do nothing yet.

A mouse report arriving in pieces over a slow link is held until it completes,
rather than being read as a keypress. An arrow key or a lone Escape is handed
back as a key as soon as it is known not to be a report, so neither is swallowed
and neither is mistaken for the end of the input.

The frame is repainted at most twenty-five times a second. A reply arrives a
token at a time, and drawing every one of them makes a fast reply read as a
flicker rather than as text arriving.

### Pasted input

Pasting several lines at once sends one message rather than one message per
line. The terminal is put into bracketed paste mode so that a newline inside a
paste is not mistaken for pressing Enter, and the pasted lines are shown above
the prompt while they wait.

Text pasted after the prompt is joined to whatever was typed, in the order it
was entered, and sent as a single message. A paste of more than a few lines is
cut on screen with the remainder reported, so that a very large paste cannot
push the status bar off the screen.

The frame is kept inside the terminal at any size. A short window drops the
title before it drops the pane or the prompt, a large paste gives up rows
rather than the prompt, and a terminal too narrow to hold the prompt marker
shows the line being typed instead. The prompt is the last thing still on
screen, since a reader who cannot see it cannot send the next message.

Resizing the window takes effect on the next repaint rather than at the next
keystroke. A frame is drawn to the size the terminal has at the moment it is
drawn, so shrinking the window shrinks the frame instead of leaving part of it
below the bottom of the screen.

A terminal without bracketed paste support still works: the text is taken as
typed, which means a multi-line paste from such a terminal is submitted line by
line.

### Input box

The prompt sits in its own box at the foot of the screen, separated from the
conversation by a full-width rule with a blank row either side:

```
Provider: openrouter.ai | Model: stealth/space-bunny-alpha | Status: idle | Credits: 0.42/5

────────────────────────────────────────────────

> what does this project do?
```

The rule keeps a reply and the prompt from reading as one block, which they do
when a reply ends mid-sentence directly above the prompt. On a terminal too
short to hold the rule, it is dropped rather than drawn, since a frame taller
than the screen would push the status bar off it.

### Wrapping

A reply wider than the terminal is folded to fit rather than cut, so nothing is
lost past the edge. A word too wide to fold, such as a long URL, is placed on a
line of its own instead of being split.

A fenced code block is left unwrapped. Code keeps its own line breaks, since
reflowing it changes what it means. A code line too wide for the terminal is cut
with an ellipsis, so it is clear that it continues past the edge rather than
having ended there.

### Terminal behavior

The terminal is driven through `syscall` rather than through a terminal library,
so that the alternate screen, the cursor, and the input mode are under direct
control. No third-party dependency is introduced.

The BSD family and System V spell the termios ioctls differently, so those names
live in build-tagged files. `make crossbuild` compiles every supported target,
and continuous integration runs it, since a change to the terminal layer
otherwise breaks only the platforms that are not being developed on. FreeBSD,
Linux, macOS, NetBSD, and OpenBSD build. Windows does not, and
`AGENTS.md` records why.

The keys the line editor acts on are:

| Key       | Effect                                                    |
| --------- | --------------------------------------------------------- |
| `Enter`   | Submit the line.                                          |
| `Backspace`, `Delete` | Remove the character before the cursor.          |
| `Ctrl-U`  | Clear the whole line.                                     |
| `Ctrl-W`  | Clear the word before the cursor.                         |
| `Ctrl-C`  | Abandon the line, or leave the interface if it is empty.  |
| `Ctrl-D`  | Leave the interface when the line is empty.               |

A multibyte character arriving one byte at a time is held until the sequence is
complete, so a character is never committed half written.

A binary is built from a checkout with the Makefile, which is written in the
syntax common to BSD make and GNU make:

```sh
make build          # compile into build/openrouter-cli
make test           # run the test suite
make check          # run the test suite under the race detector
make lint           # static analysis and the format check
make crossbuild     # compile for every supported target
make install        # copy into /usr/local/bin, honouring PREFIX
make help           # list the targets
```

The build is stamped with the version through the linker, so `make build`
followed by `openrouter-cli -version` reports the port version.

### Bootstrap documents

A session may be started from a bootstrap document, which supplies the opening
instructions for the model:

```sh
openrouter-cli --bootstrap MEMORY.md
```

The extension selects the format. A `.md` file is passed to the model as
written, since it is already prose. A `.json` file is decoded as a structured
document:

```json
{
  "instructions": "Answer in the third person.",
  "name": "third-person",
  "description": "Short answers, no contractions."
}
```

Only `instructions` is required. The `name` and `description` fields are
optional. Unknown fields are ignored, so a document written for a newer version
stays readable by an older one.

The format is selected by extension alone, since the content of a file cannot be
probed without guessing and a guess that is wrong is worse than a refusal. A
file with any other extension is refused with a diagnostic naming the extension.

An explicitly named file is treated as an assertion that it exists and can be
read, so a missing file, an unreadable file, and content that is not valid
UTF-8 are each reported as errors. This is stricter than the automatic
instruction search described below, where an absent file is an ordinary outcome.

A symlink is followed under the same rule as an instruction file, described
under [Symlinks](#symlinks).

## Configuration

Configuration is read from a JSON file. The file is treated as read-only input
except during first-time setup, which is described below.

### Location

The configuration file is searched for in the following locations, in order.
The first file found is used, and the search stops there.

| Order | Path                                              |
| ----- | ------------------------------------------------- |
| 1     | `~/.openrouter-cli.json`                          |
| 2     | `~/.config/openrouter-cli/openrouter-cli.json`    |

The second path follows the XDG Base Directory Specification. When
`XDG_CONFIG_HOME` is set, it is used in place of `~/.config`.

The search does not merge files. When a file is found at an earlier location, the
later locations are neither read nor consulted.

### Permissions

The configuration file must have mode `0600`, meaning readable and writable by
the owner only. When the file has any other mode, the client refuses to start and
explains why.

The API key is a credential, and a group-readable or world-readable file would
expose it to other accounts on the system. The check is a hard requirement rather
than a warning, so a key cannot be leaked by an accidentally permissive mode that
is never noticed.

The check is performed on the file itself rather than on its parent directories,
since directory permissions are commonly `0755` and are not a credential
exposure on their own.

When the file is created by the client, it is created with mode `0600`.

### Format

The file is JSON, which is parsed using the Go standard library and requires no
external dependency. These keys are recognized:

| Key                   | Required | Description                                                        |
| --------------------- | -------- | ------------------------------------------------------------------ |
| `OPENROUTER_API_KEY`  | Yes      | The API key used to authenticate against OpenRouter.AI.            |
| `OPENROUTER_URL_BASE` | No       | The base URL of the backend.                                        |
| `OPENROUTER_MODEL`    | No       | The model requests are sent to, such as `stealth/space-bunny-alpha`. |
| `OPENROUTER_MOUSE`    | No       | Set to `true` to ask for mouse reporting, so the wheel scrolls.     |
| `OPENROUTER_BELL`     | No       | Set to `true` to ring the terminal bell when a reply arrives.       |

The keys are given in the same form as the equivalent environment variables,
which keeps a value transferable between the file and the environment.

`OPENROUTER_API_KEY` is never read from the process environment. The key is
accepted only from the configuration file. An environment variable of that name
is ignored, even when it is set and even when the file is absent, so a key
present in the environment cannot silently take effect. This removes an entire
class of confusion in which a correct file is shadowed by a stale value
elsewhere.

An example, with the key redacted:

```json
{
  "OPENROUTER_API_KEY": "sk-or-v1-xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx",
  "OPENROUTER_URL_BASE": "https://openrouter.ai/api/v1",
  "OPENROUTER_MODEL": "stealth/space-bunny-alpha"
}
```

`OPENROUTER_MODEL` sets the model a session starts with, so `/model NAME` is not
needed on every run. It may still be changed inside the interface, and a file
without the key is not an error: the preference is kept even when no credential
is present, since a session cannot reach a model without one anyway.

`OPENROUTER_MOUSE` asks for the wheel to scroll the reply pane. It is ignored
inside tmux, where the flag `--mouse` and the key are both ignored as well,
since a terminal that reports the mouse cannot also be dragged to select text.
Run `/mouse` inside tmux to turn reporting on anyway.

JSON is used rather than YAML because Go has no standard-library YAML parser, and
a configuration file is not a place where a dependency is worth taking. The
trade-off is that JSON does not permit comments, and that hand-editing requires
strict punctuation, where a trailing comma is an error. The client writes the
file with tab indentation so that it remains readable and produces a readable
diff when it is changed.

Unknown keys are ignored rather than treated as an error, so a file written for a
newer version of the client is still readable by an older one.

A file that is not valid JSON, or a file in which `OPENROUTER_API_KEY` is absent
or empty, is an error, and the client exits with a diagnostic rather than
falling back to a default.

### First-time setup

When no configuration file is found, or when a file is found that does not carry a
setup key indicating that configuration is complete, the client prompts for the
API key.

The prompt allows the entry to be skipped. Skipping records that setup has been
carried out, so the prompt is not shown again, while no credential is stored.

The setup key name, and whether a skipped configuration is recorded distinctly
from a completed one, are not yet settled.

On start, the client writes a default configuration file when none exists, so
that the location and the file mode are established before a key is ever
entered:

```json
{
  "OPENROUTER_URL_BASE": "https://openrouter.ai/api/v1"
}
```

The default is written only when the file is absent. An existing file is left
exactly as it is, whether it is complete, empty, malformed, or missing the key.
No merge is attempted, since a partial merge of a credential file can produce a
file that parses but is wrong, and a wrong credential fails later at the point
of use rather than where it was introduced. A file that already exists is
reported by the client as it stands, and correcting it is left to the user.

The file is created with mode `0600`, which is the only mode the loader accepts.
A default written at a more permissive mode would be rejected by the client
that wrote it.

The client also writes the file when setup is completed or skipped.

### Model instruction files

**Not implemented yet.** This section describes intended behaviour. The only
instruction path that works today is `--bootstrap FILE`, which names a file on
the command line. The search below is specified but not built, so a repository
holding an `AGENTS.md` will not have it read. See `IDEAS.md`.

Before a request is sent, the client reads instructions from a file in the
current working directory. The file is selected from the following names, tried
in alphabetical order, and the first one found is used:

| Order | Name         |
| ----- | ------------ |
| 1     | `AGENTS.md`  |
| 2     | `OPENROUTER.md` |
| 3     | `RULES.md`   |
| 4     | `SHARED.md`  |

The names are ordered alphabetically, which places `AGENTS.md` first, then
`OPENROUTER.md`, then `RULES.md`, then `SHARED.md`. The order is deterministic,
so a directory containing more than one of these files always resolves to the
same one.

The search is confined to the current working directory. It is not a recursive
walk, and no parent directory is consulted.

A file that is not present is not an error, and the request is sent without
additional instructions. A file that is present but unreadable, or that is not
valid UTF-8, is an error, since silently ignoring instructions that the user
believed were in effect would be worse than refusing.

### Symlinks

A model instruction file may be a symbolic link, which allows one set of
instructions to be shared from a single location.

A symlink is followed only when its target lies on the same filesystem as the
link itself. A link that crosses a filesystem boundary, which is what a link into
another mount appears as, is refused with a diagnostic naming the offending path.

The comparison is made on the device identifier of the link and of its target,
which is the portable way to express sameness of a filesystem. The restriction
keeps instruction loading predictable, since a link out of the working tree can
otherwise reach content the user did not intend to supply to the model.

A chain of symlinks is resolved to its final target, and the device check is
applied to the final target. A symlink loop is reported as an error rather than
followed.

### Endpoint

The default base URL is:

```
https://openrouter.ai/api/v1
```

When `OPENROUTER_URL_BASE` is supplied, it overrides the default, which allows
the client to be pointed at a proxy, a gateway, or a self-hosted deployment.

The override is used verbatim when it includes a path. The `/api/v1` suffix is
appended only when the override names a scheme and host with no path, such as
`http://localhost:3000`.

### Precedence

The configuration file is authoritative. The process environment is not consulted
for the API key or the base URL, and no environment variable overrides the file.

The precedence between the configuration file and command-line flags is not yet
settled. A flag that selects a different configuration file, for example
`--config`, is expected to be honored, and the treatment of a flag that merely
overrides an individual value is an open decision.

## Features

### Status bar

An interactive session displays a status bar showing the following fields:

- Provider
- Model
- Status
- Credits
- Context used
- Tokens used (input and output)
- Hostname

The field set and the order are fixed rather than configurable. A bar that
reorders itself between runs cannot be read at a glance. A field with no value
yet is shown as a dash, so the layout does not shift as values arrive.

`Reasoning`, `Branch`, and `Approval Method` were removed rather than shown as a
permanent dash. The client has no reasoning parameter, no branching, and no tool
execution, so there would be nothing for any of them to report. A dash that can
never be filled is noise.

### Usage and quota meter

Key usage and quota are displayed natively by the client. The meter is rendered
in-process and is not written into the tmux status bar or maintained by an
external helper or scheduled task. The data is read from the key endpoint.

### Backgrounding and ephemeral chats

A running chat may be backgrounded, which suspends it and returns the user to a
prompt without discarding the conversation. A new ephemeral chat may then be
started alongside the backgrounded one.

An ephemeral chat is not persisted. It is discarded when the session ends, and
it is not written to disk and is not listed among saved conversations. Its
context is not carried into a subsequent chat, so a later conversation begins
without the history of an earlier one.

The behavior is intended to allow one thread to be set aside while another is
worked on, without the set-aside thread being lost or contaminating the
continuation. Backgrounded chats are retained within the running session, and
the manner in which they are restored, listed, and resumed is not yet settled.

### Tab completion

Commands and configuration options are completed with the Tab key.

### Terminal and mouse

The interface is designed to behave correctly with a combination of terminal and
mouse, and within tmux. The details are under
[Terminal behavior](#terminal-behavior) and [Scrollback](#scrollback).

### Scrollback

The wheel scrolls the reply pane, so an earlier part of a conversation can be
read after the newest output has filled the window. A notch moves three lines.
While the view is scrolled back, the title carries a marker and a reply arriving
does not move it, so the history being read stays where it is. Scrolling down
returns the view to the newest output.

The wheel is only read when mouse reporting is on, and reporting is off by
default. Capturing the mouse is what makes the wheel work, and it is also what
stops a drag from selecting text, so the choice is left to the reader:

- `--mouse` turns it on for the run.
- `/mouse` turns it on and off inside a session.
- `OPENROUTER_MOUSE` in the configuration file asks for it on every run.

Inside tmux the flag and the configuration are ignored, and reporting stays off
until `/mouse` is run, since a nested selection is rarely intended. Run
`/mouse` to turn it on anyway.

### Copyable text

Text is selectable and copyable from the terminal with the same gesture used for
any other terminal output. A selection made with the mouse yields the plain text
of the region, with no escape sequences, styling, or layout padding included.

This is in deliberate tension with mouse reporting, since a program that captures
mouse events intercepts the drag that would otherwise begin a selection. The
resolution is that mouse reporting is not enabled unconditionally. It is enabled
only where it is both wanted and safe, and it is suspended for the duration of
any selection gesture so the terminal retains the selection.

Three cases are distinguished:

- Text is always copyable, whether or not the mouse is used for anything else.
- Mouse reporting is not enabled inside tmux by default, because a nested
  selection inside tmux is rarely intended and a captured drag cannot be
  recovered by the user. It is enabled by `/mouse` rather than by the flag or
  the configuration, which are ignored there.
- Mouse reporting is not enabled when output is not a terminal, such as when the
  client is piped or redirected, and the copy path is unaffected.

The status bar and any bordered region are drawn without writing to the scrollback
where that is achievable, so that copied text does not carry the frame with it.

## License

BSD 3-Clause. See [LICENSE](LICENSE).

## Development

### Test suite

The project ships a scanning and test toolchain. The tools are tracked as module
tool dependencies, so they are pinned in `go.mod` and are installed together with
the module rather than being fetched by hand.

Every tool is invoked through `go tool`, and no global installation is required:

| Tool                                | Purpose                                              |
| ----------------------------------- | ---------------------------------------------------- |
| `staticcheck`                       | Bug-oriented static analysis.                          |
| `errcheck`                          | Reports unchecked errors.                             |
| `gosec`                             | Security analysis of the source.                      |
| `govulncheck`                       | Reports known vulnerabilities in the dependency graph. |
| `protoc-gen-go`                     | Generates Go code from a protocol buffer schema.      |
| `protoc-gen-go-grpc`                | Generates gRPC bindings from a protocol buffer schema. |

The `protoc-gen-go` and `protoc-gen-go-grpc` commands are provided by the modules
`google.golang.org/protobuf` and `google.golang.org/grpc/cmd/protoc-gen-go-grpc`
respectively. The second is a module in its own right, and the parent module
`google.golang.org/grpc` does not contain the command.

The generators are listed for completeness. The client communicates with the
backend over a REST interface, so no schema exists yet and the two generators
remain idle until one is added. They can be removed from the tool set if a
REST-only design is confirmed.

`govulncheck` reports on the standard library and the dependency graph rather
than on the source, so it requires network access to the vulnerability database
when it is run.

### Layout

The repository is laid out in the manner a FreeBSD port expects.

| Path                | Contents                                              |
| ------------------- | ----------------------------------------------------- |
| `cmd/`              | One directory per executable, each holding a `main` package. |
| `internal/`         | Packages that are not importable from outside the module. |
| `doc/`              | Manual pages and additional documentation.            |
| `files/`            | Auxiliary files consumed by the port, such as the pkg-descr. |
| `test/`             | Test scripts and fixtures, including a `Vagrantfile` where one is used. |
| `.github/workflows/`| Continuous integration definitions.                    |

A port additionally expects a `Makefile` carrying the port metadata, such as
`PORTNAME`, `DISTVERSION`, `CATEGORIES`, and `MAINTAINER`, along with the
`distinfo` and `pkg-descr` entries. Those files are not yet written, and the
`PORTVERSION` field is held until the first release is defined.
