# Work in progress

The working list for this repository. `AGENTS.md` records why the project is
shaped as it is; this file records what is being built and what has not been
finished. Anything here that contradicts `AGENTS.md` is a bug in one of the two,
and `AGENTS.md` is the one that decides.

Research notes on features to consider live outside the repository, at
`build/openrouter-cli-worktrees/ideas/`. What is here is the state of the work
itself.

An item `AGENTS.md` settles is not repeated here as a section. The table under
Shipped carries the merge hashes, which is what a reader needs, and prose
restating a settled decision is where the drift this file warns about above
comes from.

## Ground rules

- Feature work happens in a git worktree under `build/openrouter-cli-worktrees`, one
  directory per piece of work, named for it, merged into `main` with `--no-ff`.
- A worktree is removed and its branch deleted once the merge lands.
- A worktree that has no changes in it is not work in progress. It is a stale
  checkout and should be removed.
- Every merge is gated on `make lint`, `make check`, and `make crossbuild`.
- A commit message carries `Co-Authored-By: Space Bunny Alpha`.

## Shipped

Merged into `main`.

| Item | Merge | Note |
| --- | --- | --- |
| Stream detail, `/verbose` | `7f1bcce` | One line per turn describing the shape of the stream. |
| Frame bounds | `4ffe81f` | Frame never exceeds the terminal on either axis. |
| Pane search, `/search` | `3a7a10a` | Filters the conversation as the query is typed. |
| Port maintainer address | `82be445` | `MAINTAINER= gjb@FreeBSD.org`. |
| Terminal resize | `ccf34c1` | Size re-read per repaint rather than cached. |
| Pty test tag fix | `cef05a9` | FreeBSD-only helper, and the crossbuild target now vets. |
| Working list | `5ed0eb1` | `IDEAS.md` at the repository root. |
| Slash command completion | `4a8cb83` | Tab completes, from one table of command names. |
| Markdown rendering | `4152bf7` | Headings, lists, and emphasis, rendered as plain text. |
| Key hint row | `ccef2b0` | Names the keys that act, on one row above the prompt. |
| Session audit | `ddfb756` | The frozen reply, and five further defects in the same files. |
| Rendering audit | `a153b45` | Widths in columns, control bytes dropped, four frame-bound defects. |
| API audit | `0cfb59b` | The credential filter on every path, the stream parser, body closure. |
| Configuration audit | `9e8cbab` | XDG honoured, a blank key read as unset, the loader covered. |
| Input audit | `d669b05` | Sequences read from their shape rather than a table of lengths. |
| Conversation audit | `aa149ac` | Four races, the cognito marker, and a summary that outlived `/new`. |
| Model filter completion | `f3b7e47` | Tab completes the filter and cycles through what it matched. |
| Split mouse report | `eb57cbb` | A wheel notch split across reads no longer ends the session. |
| Queued message | `1436167` | A line sent while a model works, and escape stopping it with the line. |
| Saved session | `a828197` | A conversation written to a `.db` of its own, and loaded back. |
| Blank row under a sent line | `e5c23ad` | What was asked is told apart from what answers it. |
| Model tools | `4272ae3` | The model reads, writes and lists files under the working directory, and runs read-only git there. |

**NEXT The tools need answering four questions before they grow.** Whether a
call is asked about first, whether the git tool should write, which models can
call tools at all, and whether a saved session carrying tool turns may be
resumed against a model that cannot. All four are listed as open in
`AGENTS.md` rather than answered by omission. The approval one is the first
that matters: the `Approval` field was removed from the status bar on the
grounds that the client had no tools, and this is the moment it would come back.

Nothing was pushed. The instruction stands that the maintainer handles pushes.

## In progress

### The instruction file search

Intended to be an `/update` command that re-reads the model instruction file.
No worktree exists. One was created and removed, since an empty checkout is a
stale one rather than work in progress.

**Blocked on a discrepancy.** `AGENTS.md` describes a search over
`AGENTS.md`, `OPENROUTER.md`, `RULES.md`, and `SHARED.md` in the working
directory, read before every request, and records it under a settled heading
with no caveat. No `.go` file mentions any of those names. The only path that
exists is `--bootstrap FILE`, named on the command line.

**The two documents do not agree about how settled it is.** `README.md`
describes the same search, and opens that section with a bold
**Not implemented yet.** naming `--bootstrap FILE` as the only path that works
today and pointing at `IDEAS.md`. So `README.md` already states the position
correctly, and the correction route touches `AGENTS.md` alone rather than both
documents. It is `AGENTS.md` that presents unimplemented behaviour as settled,
which is the worse of the two, since a reader of it would believe a repository
holding an `AGENTS.md` had that file loaded.

So there are two possible pieces of work and they are not the same: build the
search, or correct `AGENTS.md` to match the code. An `/update` command has
nothing to re-read until one of those is done.

**Recommendation: build the search.** The documentation reads as designed
behaviour rather than as a stray paragraph, and `internal/bootstrap/resolve.go`
already implements the cross-device symlink rule the section describes, so the
helper was written with this search in mind and nothing calls it. The convention
is live in practice: a session in a repository holding an `AGENTS.md` has that
file loaded as instructions, and a client that does not read it is misleading.
The decision is recorded under Open decisions in `AGENTS.md` and is the
maintainers to confirm.

## Notes

### Accepting a large paste with Ctrl-J

Raised by the maintainer. Recorded as an observation to be tested, not as a
decision.

Replacing the newline in a pasted block with `Ctrl-J` may be a way to accept a
large paste.

The reasoning behind the idea is that the interface currently waits for `Enter`
after the block has landed, and a reader who has just pasted a few hundred lines
has to find a key that submits the message. Submitting with `Ctrl-J` instead
would mean the paste is accepted by the same key the terminal itself would send
for a newline, which is the character the paste already contains.

What is already in place, and matters to any implementation:

- `internal/tui/line.go` already accepts both `\r` and `\n` as the submit key,
  so a terminal sending either form submits the message.
- `internal/tui/paste.go` normalises a block to lines and drops a trailing
  break, so a paste is not submitted the instant it lands.
- A landed paste is held in `le.pasted` and joined with whatever is typed
  afterwards, returned as one message on submit.

What is not known, and is the reason this is a note rather than a task:

- Whether `Ctrl-J` is distinguishable here. A terminal in raw mode sends
  `Ctrl-J` as `0x0a`, which is the same byte as `\n`, so it is already handled
  as a submit key rather than being a separate case.
- If that holds, the idea is not new behaviour but a discovery problem rather
  than a code change: a reader has to know the key works. The hint row is where
  that would be named.
- A large paste is already cut for display, at `maxPasteRows`, with the
  remainder reported. Whether the reader is losing track of a large paste is
  therefore about the report rather than about the submit key.

So the first question is not what to build but whether this is already true.
That needs a terminal to try it in, which is the part that has not been done.

### The prompt should always show something is happening

Raised by the maintainer. Recorded as an intent, not as a settled decision,
because what the indicator should be is not yet chosen.

The reason it is wanted is that an idle interface and a hung one look identical
from the outside. Everything the client draws stops while nothing is happening,
so a crash, a wedged request, or a terminal that has stopped reading all look
the same as a session that is simply waiting for a message.

Two indicators have been proposed for the same row, and the decision is whether
they are alternatives or complements.

**The resting indicator, on an idle prompt.** What already exists and what the
work has to fit around:

- The bar above the input box reads `Working` while a request is in flight and
  `idle` otherwise. That covers a request but not an idle prompt.
- A twiddle runs while work is in progress. It is the closest thing to an
  always-on indicator that the client has.
- The hint row is state-dependent and changes with what the interface is doing.
  It is the natural place for a resting indicator, since a row that always shows
  something is the point of it.
- The input field is drawn by the line editor with echo disabled, so whatever
  appears there is chosen by the renderer rather than by the terminal.

What is not chosen:

- Whether the indicator lives in the input field itself, as a caret or a
  character that moves, or beside it as a hint-row entry. The maintainer said
  the input field, so that is the default unless there is a reason against it.
- Whether it pauses when the session is genuinely idle or when nothing has been
  typed. A reader composing a message is not waiting on the program, so an
  indicator that moves while they type is noise.
- Cost. An always-moving indicator repaints at its interval whether or not
  anything changed, and the repaint rate is bounded to keep a fast reply
  readable. An idle loop has to respect the same bound.

**A horizontally-rotating bar, in place of the braille twiddle, while work is in
progress.** The maintainer asked for this after the immediate fixes. It is
feasible, and the colour work is smaller than it looks: `twiddleTint` at
`internal/tui/twiddlecolour.go` is a pure function of the step, so a scroll is
the same ramp sampled at an offset per column, where column *i* takes
`twiddleTint(step-i)`. The ramp, the floor, the cube quantisation, and the
timing all stay as they are.

What has to change is the plumbing. `Frame.Tint` is one sequence for the whole
row and `DrawFrame` writes it before the line and resets after, so per-column
means the screen writes one sequence per cell and resets at the end of the row.
Both are in package `tui`, so the screen can call `twiddleTint` directly and the
frame need not carry a slice.

Three constraints:

- The row must leave the renderer as plain text, which is why the tint is
  carried and applied rather than written into the row. Finding the row before
  drawing is safer than matching it, since a bar of dashes makes a whole-row
  comparison fragile against a reply that happens to collide.
- Selection stays clean, on the reasoning already settled under Progress in
  `AGENTS.md`: a sequence the client writes over a row it owns is not copied
  out of it.
- Width must bound the gradient, so the ramp cannot run past the pane, and a
  frame-bound test has to cover that.

Cost is about eight bytes per column, on one row, at the existing 40 ms repaint
bound. The twiddle figure itself is unaffected. Roughly fourteen tests in
`internal/tui/twiddlecolour_test.go` assert against `Frame.Tint` as a single
sequence, so this is not a small edit, and it needs a new line in the Progress
section of `AGENTS.md`, since one sequence covering the whole row is settled
there.

**Whether the bar replaces the braille twiddle or sits beside it** is not
chosen, and the choice changes both the tests and the Progress section of
`AGENTS.md`. If it replaces it, the resting indicator has to look unlike the
in-progress one, since a twiddle that turns while idle reads as work in
progress, which is the opposite of what is wanted.

### A spinner shown on the final token

Raised in the research notes, not by the maintainer. Recorded here so the two
are not confused with each other.

The stream carries a terminating marker, which is already required rather than
assumed, so the moment between the last token and the end of the turn is
already known. Showing the twiddle for that gap reports that the reply is being
finished rather than that it has stopped.

Small, and no decision needed beyond whether the gap is ever long enough to be
worth drawing.

## Awaiting confirmation

The record settles that rendered output is plain text and that a terminal
selection yields it with no escape sequence and no padding. It does not settle
how a construct should look within that plain text. The following were chosen
during implementation and are marked `PROVISIONAL` at each site in the source.
Each is the maintainers to confirm or reverse.

Markdown, in `internal/tui/markdown.go`:

1. A heading keeps its marker as written, rather than being underlined, capped,
   or separated by a blank row. No character is changed and no row is added.
2. A closing hash run is left in the text: `## Title ##` renders as written.
3. List markers are not normalised. `-`, `*`, `+`, `1.`, and `1)` all stand, and
   the spaces after a marker collapse to one.
4. A code span keeps its backticks, which is the plain text convention.
5. Backslash escapes are passed through rather than consumed, so `\*x\*` is left
   as written. This is the one place an escape is deliberately shown rather than
   acted on, and it is the choice most worth a second opinion.
6. A heading or list item folding onto a second row indents that row under the
   first character of the text.
7. A marker wider than the pane takes a row of its own and the text folds below.
8. A seventh hash, a hash with no space after it, a marker run longer than three,
   a bullet with no space, and an indent deeper than three are all treated as
   prose, so a sentence mentioning one is never altered.

Completion, in `internal/complete`:

1. A Tab where completion does not apply writes a line to the pane rather than
   inserting a literal tab. This may prove noisier than intended for a reader
   composing prose.
2. Configuration option names complete only as a whole single word on an
   otherwise empty line, so a setting name is never rewritten into a message.
3. Command arguments are never completed. The record gives no argument grammar
   beyond `/model [NAME]` and `/delegate QUESTION`, so completing one would be
   guessing. `/models` remains the discovery path.
4. The five option names are written in `internal/complete` rather than exported
   from `internal/config`. Renaming a setting means editing both, and the drift
   risk is real.
5. An empty line and a caret at the head of a line both report rather than
   staying silent, so that Tab always says something.
6. The candidate listing is one multi-line pane entry rather than one entry per
   candidate, since search folds on stored entry boundaries. The heading wording
   is the implementers and is not specified anywhere.

### The scanning toolchain is described but absent

`README.md`, under Development and Test suite, states that the project ships a
scanning and test toolchain, that `staticcheck`, `errcheck`, `gosec`,
`govulncheck`, `protoc-gen-go`, and `protoc-gen-go-grpc` are tracked as module
tool dependencies pinned in `go.mod`, and that every tool is invoked through
`go tool`. The Toolchain section of `AGENTS.md` makes the same claim, and adds
that `protoc` is a build dependency and that the race detector needs a C
compiler.

The repository does not have that toolchain. `go.mod` carries no `tool`
directive and no `require` block, and there is no `go.sum`. `make lint` runs
`go vet` and `gofmt -l` and nothing else, and `make test`, `make check`, and
`make crossbuild` invoke none of the six named tools. The only two occurrences
of the string `go tool` in the repository are the two prose claims above, and
no workflow under `.github/` refers to any of the six. `go.mod` has carried no
tool directive in any commit, so the claim has not once been true of the file
it describes.

Two documents therefore describe a toolchain that the repository does not run.
There are two ways to close that, and neither has been chosen:

- The tool dependencies are added to `go.mod` as a `tool` directive, and the
  Makefile targets are wired to invoke them. `README.md` and `AGENTS.md` then
  stand as written. `go.mod`, the `test`, `lint`, and `check` targets in the
  `Makefile`, and the merge gate all change, since the scanners would run
  wherever the gate runs.
- `README.md` and `AGENTS.md` are corrected to describe what the
  repository runs, which is `go vet`, `gofmt`, and `go test -race`. Only the two
  documents change. The merge gate is left as it is.

Nothing was changed in either direction while this was recorded. Whether the
two generators are dropped is a further question inside the first resolution
rather than part of it, since neither has a schema to work against either way.
The choice alters the dependency set and the merge gate, so it is the
maintainers to make rather than the implementers.

A second audit later confirmed the above against the CI workflow rather than
only the Makefile, and spelled out the consequence. The workflow invokes none
of the six either, so the merge gate is three targets and none of them runs a
scanner beyond `go vet`. Neither a local gate nor a push has run the toolchain
either document describes.

### A command argument is discarded rather than used

Dispatch already parses the line and hands the fields after the command to its
handler. Two handlers take the argument and ignore it: `cmdSearch` and
`cmdModels` are both `func (s *Session) cmdX([]string) bool`, so `/search
compaction` opens a search with an empty filter and `/models claude` opens the
catalogue unfiltered. The text is dropped rather than misused, which is the
safer of the two failures, but it is a defect and not a decision.

Reading the remainder of the line into the filter is not one change but a
choice, and it has not been made:

1. Whether a query given on the command line should only filter, or should also
   jump to the newest match. Enter jumps, so the query on the line and the
   Enter key would behave differently for the same text unless one of the two
   is changed.
2. Whether a search opened with a query should still be incremental, so that
   typing appends to it. The caret sits after the filter, so appending must be
   done by placing the caret rather than by writing to the end of the buffer, or
   a caret moved left would have the text appended past it.
3. How the remainder of the line is split, given there is no argument grammar.
   `/models gpt` is unambiguous and `/delegate` takes a whole sentence, so the
   two cannot be split by the same rule without one of them being wrong.

Tab completes the model filter and cycles through what it matched, so a reader no
longer types a whole identifier to choose one. The argument supplied on the
command line is still discarded, and questions one to three stand as they are.
The search takes no completion, since its filter is a word of prose rather than
the name of a thing to be chosen.

Recording this rather than implementing it: the record completes `/search` as
filtering the pane as the query is typed, and says nothing about a query
supplied with the command, and the three questions above are not answerable from
what it does say.

## Backlog


### Chrome on the shell allowlist, or as a tool of its own

Raised by the maintainer. An intent rather than a decision. Not started, and
no worktree exists.

A name on `shellPermitted` would let a model launch Chrome, and nothing more.
The two are different questions, and the record does not say which is meant, so
this entry covers both.

**A list entry is three edits and buys three things.** Chrome launches with its
own window rather than inside the terminal. Headless works, and
`--headless --dump-dom URL` prints the rendered DOM as text, which is the one
of the three a model can genuinely use. Launching it so a reader can watch is
the third. None of that is driving it.

**Driving it needs a tool, not a name.** Four things stand between the
allowlist and a browser a model can navigate, and none of them is a program
already on the list:

1. A debugging port. Chrome must be launched with `--remote-debugging-port`,
which is an argument, so a model could do it after the reader approves.
2. An HTTP client to reach it. Every DevTools request is HTTP, and `curl` is not
on the list.
3. A JSON protocol. Navigating, clicking and evaluating script are DevTools
methods, not arguments, and `sed` and `awk` cannot speak them.
4. A websocket. DevTools is not plain HTTP, and nothing on the list can hold one.

So the work is a new tool of the same order as the terminal layer rather than
as a list edit. It would launch Chrome under a profile in a directory the tool
owns, bind the port to localhost, and speak DevTools over it.

**The containment point, which is not settled anywhere.** A shell program is
contained only in the directory it runs in. Its arguments are not checked. So
Chrome given `--user-data-dir=/somewhere` writes there, and `--headless --dump-dom`
writes a whole profile wherever it is told. That is already true of `git` and of
`make`, and the record does not say so in as many words. The `curl` question, in the note at build/IDEAS-tail.md,
raises the same one.

What the decision needs:

1. A list entry, a tool, or both.
2. If an entry, whether `--user-data-dir` is refused outside the working directory, on
the pattern `gitRefused` already uses.
3. Whose profile. A reader launching their own browser and a tool launching one
under a directory it owns are different things with different consequences.
4. What the reader sees. A browser window opening on its own is not something a
reader asked for.
5. Whether `file://` URLs are refused, on the maintainers instruction. A
browser a model drives can be pointed at `file:///etc/passwd` or at any other
file the account reads, and a page it loads can carry content back out. That is
the one URL scheme with no network between the model and the filesystem, so it
is the one worth refusing. The question is where: refused by name beside the
other options, or refused wherever a URL appears as an argument, which is the
check `gitRefused` already models. A browser tool would want the second, since
the URL reaches it by several routes.
### An urgent queued message bypasses the queue

Raised by the maintainer. An intent rather than a decision, and it contradicts a
settled one.

Enter queues a line while a model is working, and `AGENTS.md` settles that a
queued line waits for the request ahead of it, one line at a time. The maintainer
wants the queue split into two buckets: a message marked urgent goes out at once
by interrupting the model, and a message that is not waits its turn as it does
now. The default is a timer of five seconds, at which point a message is treated
as urgent and sent.

**This is a change to the record rather than an addition to it,** so it belongs
under Open decisions in `AGENTS.md` alongside the line it contradicts, and not
only here.

What the decision needs:

1. What marks a message urgent. A key held while composing is one way, and a
   prefix typed into the line is another, and the two are not the same thing to
   a reader.
2. Whether the five seconds is counted from the message being queued or from
   the model being interrupted. A reader who typed a long message and reached
   for a key would have it interrupted on the first reading, which is a real
   hazard rather than an edge case.
3. What an urgent message does to a turn in flight. It arrives as an update to
   the request it was answering, on the terms escape already uses, so the
   question is whether it displaces a queue that has not gone out or joins it.
4. Whether urgency survives being drawn. The queue is shown above the prompt and
   marked as queued, so a reader watching two messages cannot tell which is about
   to interrupt and which is not.
5. Whether the timer is cancelled when the request ahead ends. A model that
   answers in four seconds and a timer of five means the message never becomes
   urgent and goes out as an ordinary question, which is a different result
   from one that was always going to be urgent.

### Ctrl-C cancels a queued message

Raised by the maintainer. An intent rather than a decision. Not started, and no
worktree exists. A cancel is the companion to the entry above: where that one
sends a queued message early, this one removes one without sending it at all.

The key collides with what it already does. Ctrl-C abandons a line being
composed, and on an idle prompt with nothing in hand it ends the session. A
queued message with an idle prompt and nothing in hand is that state, so Ctrl-C
there currently quits. A cancel that leaves the session is not a cancel, so the
key has to be told apart from the quit and the record does not say how.

What the decision needs:

1. Whether Ctrl-C with a queue and nothing in hand cancels rather than quits, and
   what is left that quits.
2. Which message a cancel takes: the newest, the oldest, or one named.
3. Whether an emptied queue is drawn as nothing, or as a row saying it is empty.
4. Whether a cancel says so in the pane. A queue that shrinks silently is a line
   the reader cannot account for.
5. Whether a cancel is refused while the request ahead is in flight, since the
   stop already claims the queue under the lock that registers a turn.

The stop settles who acts on the queue: `stopTurn` claims it under the session
lock so that a turn ending at the same moment cannot drain it as well. A cancel
reaching the queue has to take the same lock, or the two compete for the same
lines.

### The file upload

Raised by the maintainer, from `notes2.txt`. Not started, and no worktree
exists. An `/upload` that sends a file to the model through the OpenRouter
Files API.

The design has not been settled, and the pieces below are what the decision
needs rather than a decision already taken.

The client executes nothing and has no tools, so there is no file browser and
no way to attach a file that the reader did not name. The file is therefore
taken as an argument, which makes the command read as `/upload PATH`, and the
question that follows is what the pane shows afterwards: the identifier the
endpoint returns, since that is what a later turn refers to, and the size, since
a reader who cannot see the size cannot tell a wrong file from the right one.

What the file becomes in a message is the same question. The Files API stores
a file and returns an identifier, and a later turn refers to that identifier
rather than carrying the bytes, so an upload is a reference the backend holds
rather than conversation state. That has a consequence worth settling before
the code is written: whether the reference survives `/new`, since `/new` clears
the conversation and keeps only the bootstrap document.

The model matters as much as the command. A turn carrying a file part is
refused by a model that does not accept one, which is a different failure from
a rejected file and would read as the upload having failed.

`/upload` has no argument grammar in the record beyond the two commands the
completion section names, so completing a path would be guessing, the same
reason command arguments are not completed.

### A click copies an entire block

Raised by the maintainer. An intent rather than a decision. Not started, and no
worktree exists. A click on a block in the pane would put the whole of that
block on the clipboard: a fenced code block, a text block, a list, a heading,
whatever a block is taken to be here.

What is already in place, and what the work has to fit around:

- Copying today is a terminal selection and nothing else. The record settles it
  under Interface: text is always copyable with the ordinary terminal selection
  gesture, and a selection yields plain text with no escape sequence and no
  padding. Nothing in the client reads a clipboard.
- A click is already received and thrown away. `parseMouse` in
  `internal/tui/mouse.go` reports `mouseNone` for anything that is not a wheel
  notch, and only the button-event mode is enabled, so a press and a release both
  arrive and neither is acted on. The parsing a click needs is written; the
  acting on it is not.
- Reporting is off unless it is asked for, and off inside tmux unless `/mouse`
  is run, since capturing the mouse is what stops a drag from selecting text. A
  key acting on a click therefore does nothing at all in the session that has
  not asked for the mouse, which is the ordinary one.
- The record resolves the tension between the two in favour of the selection,
  and deliberately: reporting is suspended for the duration of a selection
  gesture so that a drag keeps working. Click to copy sits on the other side of
  that decision rather than outside it.
- The pane holds no blocks. `Frame.Reply` is a list of strings, a reply is stored
  whole and folded at draw time by `WrapBlock`, and the only block the code knows
  is a fence pair in `internal/tui/wrap.go`. Nothing records what a row is, which
  reply it came from, or which fold produced it.
- Folding means the screen is not what was written. Prose is folded to the width,
  and a code line wider than the pane is cut with an ellipsis. A copy taken from
  the drawn rows carries the fold and the cut, and a copy taken from the store
  carries the reply as the model sent it. Those are different things, and only
  one of them is what a reader reaching for a code block wants.

What is not chosen, and is what the decision needs:

1. What a block is. Nothing settles it. A fenced code block is the case named and
   a text block is the other end of it, but a reply is stored as a single entry
   whatever is inside it, so a list, a table, a heading, and a paragraph are all
   inside one string. Either a boundary is recognised as the reply arrives or the
   renderer is asked to report it, and `internal/tui/markdown.go` already holds
   the constructs it recognises.
2. Which form is copied: what was drawn, folded and with the ellipses in it, or
   what was written. The first is what the reader can see and is wrong for a code
   line the pane cut. The second is right and is not what a reader who scrolled
   back and clicked was looking at.
3. How a click is told from a selection. A press and a release with no movement
   between them is a click, and a press that becomes a drag is a selection, so
   the two can be told apart where both are reported. A middle click and a shift
   click are the other answers, and terminals differ on whether either arrives.
4. How the text reaches the clipboard. There is no dependency beyond the SQLite
   driver and no program to hand it to. The routes are the OSC 52 sequence
   written to the terminal, which is inside the existing design since the client
   already writes escape sequences for the alternate screen and the cursor, and a
   subprocess such as a clipboard tool, which is a shell out to something that has
   to be found and named per platform. OSC 52 is refused by some terminals and
   needs tmux passthrough turned on, and what is reported when a terminal refuses
   is not settled either.
5. Whether the copy is recorded. It belongs with the things the record already
   declines to record: a delegate records nothing, `/search` records nothing, and
   the queue is pane content rather than a turn. A copy is display, and that
   should be said rather than left to be assumed.
6. Whether a copy says so in the pane. A key that acts silently is a key a reader
   presses twice, and the pane is where the client says what it has done.
7. Whether the write is serialised with the repaint. Writes to the terminal are
   serialised, and a sequence written outside that lock can land in the middle of
   a row that was being drawn.

What is worth saying before the design is chosen is that this is not only a
clipboard question. A block is something the pane does not currently have, so the
idea is two pieces of work at once: deciding where a block begins and ends, and
deciding what puts it on the clipboard. The second is the smaller of the two.

### An alias for a common command

Raised by the maintainer, and asked for as a way of speeding up a test run. An
`/alias NAME ...` command that stands in for a longer line, so a task repeated
across many turns is typed once.

**Where the aliases are stored is not settled, and the obvious place is the wrong
one.** The maintainer asked for `~/.openrouter-cli.json`, which is the
configuration file. `AGENTS.md` settles that the configuration holds the
credential, is mode `0600`, and is treated as read-only outside setup, and it
records that the approval rules were deliberately kept in a sibling file rather
than in it: a reader editing rules should not have to open a file whose mode they
have to get right, and a command rewriting one could damage the key in it. The
same reasoning applies to an alias command that writes.

So the shape of the decision:

1. A sibling file beside the configuration, following the pattern
   `config.LoadRules` already establishes, or a table inside the configuration
   itself. The first keeps the credential file out of the path of a command that
   writes.
2. Whether an alias expands to text or is a name the dispatcher resolves. Text
   is simpler and shows the reader what will be sent; a resolved name is one more
   entry in a table that already has to stay in step with the command list.
3. Whether an alias can name another alias, and what stops a cycle.
4. Whether an alias survives a name that stops existing, since a command can be
   renamed or removed while an alias naming it is still in the file.
5. Whether an alias may be a queue or a stop rather than a question, since an
   alias standing in for a keypress is a different thing from one standing in for
   a line.

### `gh` on the shell allowlist

Raised by the maintainer. An intent rather than a decision. The allowlist is
`shellPermitted` in `internal/tools/shell.go`, and the program list is spelled
out in prose in the tool schema in the same file, so adding one is three edits
and a test rather than one.

**The question is whether a program that writes belongs on it.** Every current
entry is read-only or a build tool: `git log`, `grep`, `find`, `go build`. `gh`
is not, since `gh pr create` and `gh issue close` change things, and `AGENTS.md`
records the git tool as an allowlist precisely because a blocklist is defeated
by every subcommand nobody thought of. The same argument cuts both ways: a
program on the list is not thereby permitted to run, since `/approve` already
settles that a call is asked about, and a mode of `allow` is a reader saying so
deliberately.

What the decision needs:

1. Whether `gh` is on the list whole, or whether the list grows a way to permit
   a subcommand rather than a program. A subcommand list is a second allowlist
   and the drift argument against it applies with more force, since `gh` has far
   more subcommands than git.
2. Whether a mode of `allow` should reach it. A reader allowing the build tools
   is not the same as allowing something that pushes, and `AGENTS.md` settles
   that a mode of allow does not widen the allowlist, so the question is whether
   `gh` should be excluded from that mode rather than covered by it.
3. What the pane shows. A call line names the program and its arguments, and
   `gh pr create --title ... --body ...` is a long argument to draw on one row.

### A running total of cost for each model

Raised by the maintainer during the status bar move.
Recorded as an intent, not as a settled decision, because the storage is not yet chosen.

The status bar move shows the cost of the current session in the top bar.
This entry is the other half of the request: the total spend on a model across all sessions, kept per model.
The figure would follow the reader as the model is switched, so that the top bar shows what the selected model has cost to date beside what the session has cost.

What the work has to fit around:

- The session cost resets with the session, so it needs no storage.
  A total across sessions does, and it has to survive a crash as well as a clean exit.
- The client already has a sqlite store for saved conversations.
  The leading proposal is to keep the totals there, in a table of its own, rather than in the config file.
  The config file is read by hand and edited in place by `/color`, and a figure that changes on every reply does not belong in a file the reader maintains.
- The cost of a response has to come from somewhere.
  The session cost needs the same source, so the choice is made once for both: the cost the API reports for each response, or the tokens multiplied by the catalogue pricing and marked as an estimate.

What is not chosen:

- Whether the total is per model name or per model and provider, since the same model can be served by providers at different prices.
- Whether the reader can reset a total, and if so with which command.
- Whether a request that failed after tokens were spent is counted.
- How a model that cost nothing is shown, as a zero or as a dash.
