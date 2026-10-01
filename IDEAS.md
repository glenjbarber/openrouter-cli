# Work in progress

The working list for this repository. `AGENTS.md` records why the project is
shaped as it is; this file records what is being built and what has not been
finished. Anything here that contradicts `AGENTS.md` is a bug in one of the two,
and `AGENTS.md` is the one that decides.

Research notes on features to consider live outside the repository, at
`~/openrouter-cli-worktrees/ideas/`. What is here is the state of the work
itself.

## Last updated

After the completion, markdown and hint row landed, the whole of the work was
rewritten twice before any of it was pushed: once to add a missing
`Co-Authored-By: Space Bunny Alpha` trailer to the completion and markdown merge
commits, and once after the audit to drop five housekeeping merges the workers
had made to catch a feature branch up with main. Nothing was pushed before
either, so nothing was rewritten on a remote. The merge hashes named below are
the current ones.

The audit is done, all six areas merged and gated. It is recorded at the end of
this file, along with what it found and did not fix. Two pieces of work have
been taken back off `main`, and each is recorded at the end of this file: The
withdrawn file upload, and The withdrawn model-list cache.

Tab completion, markdown rendering, and the key hint row have landed, each
merged with `--no-ff` behind `make lint`, `make check`, and `make crossbuild`,
each built in a worktree on its own branch, and each verified after the merge
rather than before it.

**The worktrees this file described did not exist on this host.** There was no
`~/openrouter-cli-worktrees`, no `hintrow` source, and no uncommitted `hintrow`,
`completion`, or `markdown` code to recover: no stash, no dangling objects, and
nothing anywhere under `/Users/gjb`. The three remote branches all pointed at
commits already merged into `main`, so they never carried the work. The
`hintrow` files described below existed on another machine and were gone. The
hint row has since been written again from its description and has landed. The
`completion` and `markdown` work was written again before it landed.

The instruction search is blocked on a decision. The audit is done, so the
backlog it recorded is the open work rather than work waiting behind it. Three
of the findings it left unfixed have now been read, one worker each, read only:
the instruction search itself, the compaction contradiction, and the credential
on a redirect. What each pass established is written down under Three findings
read in detail, near the end of this file.

This pass also corrected two statements in this file that had been left behind
by the work landing: a paragraph still describing `completion` and `markdown` as
unlanded after both had merged, and a sentence saying the audit had not started
after all six areas had merged. A record that contradicts itself is a bug in
the record, as the ground rule at the top says.

Choices made during implementation that the maintainer has not confirmed are
recorded under Awaiting confirmation.

Two read-only audits were run after that, against the configuration layer and
against the command surface. Both have landed their findings here and are at
the end of this file. Neither changed a line of code or of documentation.

The configuration audit found one defect, which is a missing branch in the
default write and is the only finding in either audit that is a defect against
the record rather than a question. It also found a mechanism the record
describes in detail and the code does not have at all, a first-time setup
prompt that nothing implements, and three passages that contradict either the
code or each other.

The command audit found that the command table, the completion set, the hint
row, and the packaging directories all hold, and recorded five divergences,
the serious one being that the usage text omits the command the code twice
tells the reader to use.

The scanning toolchain finding already recorded above was extended rather
than repeated: the CI workflow runs none of the six either, so the merge gate
has never run them.

Nothing was pushed. The instruction stands that the maintainer handles
pushes.

## Ground rules

- Feature work happens in a git worktree under `~/openrouter-cli-worktrees`, one
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

### A conversation can be saved to a file of its own

`/save NAME` writes the conversation to `~/.openrouter-cli/sessions/NAME.db`
and `/load NAME` resumes it. A saved file is also a bootstrap document, so
`--bootstrap NAME.db` begins a session from a conversation rather than from
prose for one.

**The file is a SQLite database**, so that it can be read afterwards by any
tool rather than only by this client. The driver is `modernc.org/sqlite`
because it is pure Go: `make crossbuild` sets `GOOS` without `CGO_ENABLED`,
so a driver that compiles C would produce a binary that builds for every
target and then fails at the first query.

**SETTLED DragonFly was dropped.** The pure Go driver carries an emulation of
the C library and that emulation has no DragonFly in it, so `make crossbuild`
failed for that target. Cgo was raised as the way out and measured rather than
assumed: `mattn/go-sqlite3` builds and runs on the host, but `CC` is `cc` for
every foreign `GOOS` and no cross C toolchain is installed, so `runtime/cgo`
fails for freebsd, linux, netbsd, openbsd and dragonfly alike, and a
cross-build gate that cannot cross-build stops meaning anything. There is no
newer pure Go driver to bump to either, the newest `modernc.org/libc` being the
version already in the tree. `github.com/ncruces/go-sqlite3` does build on
every remaining target with no cgo, and was not taken: 12.4 MB against 2.9 MB
for two tables is a size the maintainer declined over a sixth platform. The
target, its build tags and the fallback that answered that it had no driver are
all gone.

**PROVISIONAL** The choices below are the implementer's, not the maintainer's.

- A save is refused in-cognito and inside a thread, since both record nothing
  and a file would break that rather than record it.
- A save under a name already taken asks, and only an explicit yes replaces.
  Anything else writes beside it under a name carrying the epoch.
- A save may be taken while a model works and holds what has been recorded; the
  turn in flight is not in it.
- A model the reader chose wins over the one the file carries.
- `/load` shows the turns it restored rather than replacing the pane silently.

The free-model allowance was listed here as not written, which was wrong and is
now contradicted by a test in `internal/saved`: the allowance type is an alias
for an anonymous struct, so the whole usage value round-trips through the
database. It is written and read back.

Checked with unit and integration tests, the race detector, the cross-build on
all six targets, and a real pty: `/save`, a second `/save` under the same name
declined, and `/load` all behaved, and the file opens in any SQLite tool.

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
behaviour rather than a stray paragraph, and `internal/bootstrap/resolve.go`
already implements the cross-device symlink rule the section describes, so the
helper was written with this search in mind and nothing calls it. The convention
is live in practice: a session in a repository holding an `AGENTS.md` has that
file loaded as instructions, and a client that does not read it is misleading.
The decision is recorded under Open decisions in `AGENTS.md` and is the
maintainers to confirm.

## Not started

Written but not landed. The checkout exists and holds the work.

| Worktree | Branch | Subject |
| --- | --- | --- |
| none | none | Nothing remains in this section. |

`completion` and `markdown` were listed here with uncommitted work in their
worktrees. Both have landed, see Shipped: the completion table as `4a8cb83` and
the markdown renderer as `4152bf7`, each with tests, which the unlanded work had
none of. The uncommitted code was not recoverable on this host, so both were
written again from the description above.

`cmdqueue`, `mfilterfix`, `noninteractive`, `stopcancel`, and `instructions` held
no changes and were removed, along with their branches. Nothing referenced them
and none had been pushed. A checkout with no changes is a stale one rather than
work in progress, and the distinction is recorded under Worktrees in
`AGENTS.md`.

## Notes

### The two session captures

`notes.txt` and `notes2.txt` were session captures, not project files. Both
were read, and what they carried was written down here. Both were then
deleted, since a capture left in the checkout is noise that a later reader
would have to work out was not part of the project.

`notes.txt` carried a status line and a three-line greeting exchange. The
status line is the one the client draws, showing `Credits: -` against a key
whose limit the endpoint had not reported and `Context: 0%` against an empty
conversation, which is what the record under Interface already says happens
when a field has no value yet. The `{"isNewTopic":...}` lines interleaved with
the exchange belong to a topic-tagging sidecar and not to this project, and
nothing in them describes the client. Nothing from that file needed to be
recorded beyond the confirmation.

`notes2.txt` carried one defect report and one backlog item. The backlog item
is the file upload, recorded below. The defect is the next entry.

### A reply that arrives and is never drawn

Raised by the maintainer, from `notes2.txt`. This is a defect, not an
observation to be tested, and it is the most serious thing found so far.

What was reported: a message was sent, the twiddle appeared, and then nothing
else was drawn. The pane sat on the last twiddle frame until text was typed,
at which point the whole reply appeared at once. Turning the bell on with
`/bell` confirmed the reply had in fact been returned, since the bell is rung
when a reply finishes arriving rather than when the request is sent. So the
reply was there and the interface was not showing it, which means the client
stopped drawing rather than stopped receiving.

The mechanism follows from `internal/tui/session.go`, and it is a gap rather
than a fault in any one line:

- `paint` coalesces. A repaint asked for within `minPaintInterval`, which is
  40 milliseconds, sets `paintPending` and returns without drawing.
- `paintDue` is what flushes that. It is called from exactly one place, which
  is `stream`, and `stream` runs once per streamed delta.
- When the stream ends, `send` runs its deferred cleanup, clearing `Busy` and
  `Partial`, and then `endWork`, which stops the spinner goroutine, clears
  `Spinner` from the frame, and calls `draw`.
- That final `draw` arrives within 40 milliseconds of the last delta's paint,
  so it is deferred rather than drawn. Nothing is left to flush it: the delta
  stream has ended, so `stream` will not be called again, and the spinner has
  been stopped, so the goroutine that would have drawn on the next tick is
  gone.
- The frame carrying the reply in place, the status back to `idle`, and no
  twiddle is therefore never written. The next keystroke repaints, and the
  reply appears at once, which is what was seen.

What this means for the record. The rule under Repainting, that a repaint
inside the interval is deferred rather than refused so that nothing is lost,
is right, and the deferred repaint has no owner once the thing that asked for
it has stopped. The fix is not to stop deferring, since the bound on the
repaint rate is what makes a fast reply readable. It is to make sure a
deferred repaint is always owed a flush by something that is still running,
and that the end of a turn is not itself allowed to be the thing that is
deferred.

This is recorded before it is fixed. It is the first thing the audit takes up,
and a test that fails on the current code is what decides the fix.

**Fixed.** The audit took it up first, and the fix is merged. A folded repaint is
now owned by a timer rather than by whatever asks for a repaint next, and every
exit from a turn ends on a repaint of its own that draws whatever the rate bound
says. The bound is unchanged: it is what makes a fast reply readable, and the end
of a turn is a bounded number of extra repaints rather than an unbounded rate. A
repaint owed when the session closes is dropped rather than written.

**The superseded branch.** `wip/repaint-as-landed` holds an earlier fix for this
same defect, written before the audit took it up. It is not in the history of
`main` and is not to be merged: the same fix landed by the session audit, under
a different field name and with its own tests, so merging the branch would land
a second timer for one job. It is kept rather than deleted alongside the
withdrawn branches, since all three were taken off `main` and should be read
together: `feature/upload-withdrawn` and `feature/model-list-cache-withdrawn`.

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
  than a code change: a reader has to know the key works. The hint row, which
  is unfinished, is where that would be named.
- A large paste is already cut for display, at `maxPasteRows`, with the
  remainder reported. Whether the reader is losing track of a large paste is
  therefore about the report rather than about the submit key.

So the first question is not what to build but whether this is already true.
That needs a terminal to try it in, which is the part that has not been done.

### The input field should always show something is happening

Raised by the maintainer. Recorded as an intent, not as a settled decision,
because what the indicator should be is not yet chosen.

The prompt should always be doing something, so that a reader can tell at a
glance that the program has not died.

The reason it is wanted is that an idle interface and a hung one look identical
from the outside. Everything the client draws stops while nothing is happening,
so a crash, a wedged request, or a terminal that has stopped reading all look
the same as a session that is simply waiting for a message. A reader who has
just sent something and sees nothing move has no way to tell those apart.

What already exists, and what the work has to fit around:

- The status bar reads `Working` while a request is in flight and `idle`
  otherwise. That covers a request but not an idle prompt.
- A twiddle runs while work is in progress. It is the closest thing to an
  always-on indicator that the client has.
- The hint row, which is unfinished, is state-dependent and changes with what
  the interface is doing. It is the natural place for a resting indicator,
  since a row that always shows something is the point of it.
- The input field is drawn by the line editor with echo disabled, so whatever
  appears there is chosen by the renderer rather than by the terminal.

What is not chosen, and is what the decision needs to settle:

- Whether the indicator lives in the input field itself, as a caret or a
  character that moves, or beside it as a hint-row entry. The maintainer said
  the input field, so that is the default unless there is a reason against it.
- What moves, and at what rate. A twiddle that turns while idle reads as work
  in progress, which is the opposite of what is wanted, so the indicator has to
  look unlike the in-progress one.
- Whether it pauses when the session is genuinely idle or when nothing has been
  typed. A reader composing a message is not waiting on the program, so an
  indicator that moves while they type is noise.
- Cost. An always-moving indicator repaints at its interval whether or not
  anything changed, and the repaint rate is currently bounded to keep a fast
  reply readable. An idle loop has to respect the same bound rather than
  waking the terminal more often than the frame rate allows.

### A spinner shown on the final token

Raised in the research notes, not by the maintainer. Recorded here so the two
are not confused with each other.

The stream carries a terminating marker, which is already required rather than
assumed, so the moment between the last token and the end of the turn is
already known. Showing the twiddle for that gap reports that the reply is being
finished rather than that it has stopped.

Small, and no decision needed beyond whether the gap is ever long enough to be
worth drawing.

### The lesson from the pty test

Found and fixed. Recorded because the mistake is easy to repeat and the gate
that now catches it is worth knowing about.

`internal/tui/pty_bsd_test.go` was written for the resize fix and tagged for the
whole BSD family, but it names `syscall.TIOCPTMASTER`, which only FreeBSD
carries. It broke darwin, netbsd, openbsd, and dragonfly.

`make crossbuild` passed the whole time, because `go build` does not typecheck
test files. It was caught only by running `GOOS=linux go vet ./...` by hand
while checking whether a push was safe. Had that not been checked, continuous
integration would have failed it after the push instead.

`make crossbuild` now runs `go vet` as well as `go build` for every target. The
helper is FreeBSD-only and the other platforms skip. Both facts are recorded
under Build in `AGENTS.md`.

## Awaiting confirmation

The record settles that rendered output is plain text and that a terminal
selection yields it with no escape sequence and no padding. It does not settle
how a construct should look within that plain text. The following were chosen
during implementation and are marked `PROVISIONAL` at each site in the source.
Each is the maintainer's to confirm or reverse.

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
   is the implementer's and is not specified anywhere.

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
either document describes. It is recorded at the end of this file, under The
command surface was audited against the record.

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
   Enter key would behave differently for the same text unless one of the two is
   changed.
2. Whether a search opened with a query should still be incremental, so that
   typing appends to it. The caret sits after the filter, so appending must be
   done by placing the caret rather than by writing to the end of the buffer, or
   a caret moved left would have the text appended past it.
3. How the remainder of the line is split, given there is no argument grammar.
   `/models gpt` is unambiguous and `/delegate` takes a whole sentence, so the
   two cannot be split by the same rule without one of them being wrong.
4. Whether `/search` and `/models` should complete their arguments. Completion
   deliberately completes none, since the record gives no argument grammar beyond
   `/model [NAME]` and `/delegate QUESTION`.

Question four has since been answered for the overlay rather than for the
command line. Tab completes the model filter and cycles through what it matched,
so a reader no longer types a whole identifier to choose one. The argument
supplied on the command line is still discarded, and questions one to three
stand as they are. The search still takes no completion, since its filter is a
word of prose rather than the name of a thing to be chosen.

Recording this rather than implementing it: the record completes `/search` as
filtering the pane as the query is typed, and says nothing about a query
supplied with the command, and the four questions above are not answerable from
what it does say.

## Backlog

### The file upload

Raised by the maintainer, from `notes2.txt`. Not started, and no worktree
exists. An `/upload` that sends a file to the model through the OpenRouter
Files API.

The design has not been settled, and the pieces below are what the decision
needs rather than a decision already taken.

The client executes nothing and has no tools, so there is no file browser and
no way to attach a file that the reader did not name. The file is therefore
taken as an argument, which makes the command read as
`/upload PATH`, and the question that follows is what the pane shows
afterwards: the identifier the endpoint returns, since that is what a later
turn refers to, and the size, since a reader who cannot see the size cannot
tell a wrong file from the right one.

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

## Outstanding

### The code review and audit

Done. Six areas, one worktree each, no two workers owning the same file. Every
merge was gated on `make lint`, `make check` and `make crossbuild` after it
landed rather than before.

| Area | Files | Merge |
| --- | --- | --- |
| Session | `internal/tui/session.go` | `ddfb756` |
| Rendering | `render.go`, `screen.go`, `wrap.go`, `markdown.go`, termios | `a153b45` |
| API | `internal/openrouter` | `0cfb59b` |
| Tools | `internal/tools`, `internal/tui/toolcall.go` | `4272ae3` |
| Configuration | `internal/config`, `internal/bootstrap`, `cmd/openrouter-cli` | `9e8cbab` |
| Input | `line.go`, `keys.go`, `paste.go`, `mouse.go`, `hintrow.go` | `d669b05` |
| Conversation | `conversation.go`, `compact.go`, `cognito.go`, `btw.go`, `delegate.go`, `threshold.go`, `verbose.go`, `bell.go`, `spinner.go` | `4272ae3` |

The serious ones, in the order a reader is most likely to hit them:

- **Every interrupt ended the session.** `Run` cleared the frame before testing
  it, so the test was always true, and a half-typed message was a way out
  rather than a way of changing your mind.
- **The in-cognito marker was never read back.** `/cognito` wrote it and nothing
  adopted it, so a reader who turned the mode on and left recorded every
  exchange of the next session. Found by the conversation audit as a deferred
  finding, since the function was correct and had no call site.
- **`XDG_CONFIG_HOME` did nothing.** The search passed the override a
  home-qualified path against a relative name, so it never matched and a reader
  keeping their file under XDG lost it to a default written under the home
  directory. It passed no test, because the test written alongside it used the
  wrong filename.
- **A reply arriving and never being drawn.** Recorded under Notes above, and
  fixed.
- **Four data races**, each reported by the detector rather than reasoned about:
  the compaction, the thread and the in-cognito toggle appended to the pane
  without the lock the paint path reads it under; the delegate goroutine read
  the model and the client while the input goroutine replaced them; the twiddle
  goroutine advanced its frame index unlocked.
- **Widths counted in characters where a terminal counts columns**, so a wide
  character overflowed the pane and pushed everything below it down.
- **The key sequence reader matched a fixed table of lengths**, so a modified
  key such as an alt-held arrow was not understood and its parameter bytes were
  read as keys.
- **A stream cut short painted the reply twice** and recorded the exchange, so a
  truncated answer was replayed to the next request as though it were whole.
- **The header took its rows before the prompt**, so a terminal too short to
  hold both lost the prompt and with it any way to type a next message.

### What the audit found and did not fix

Recorded rather than landed, since each is the maintainers to decide:

- **The credential follows a same-host redirect, including an `https` to `http`
  downgrade**, which would put it on the wire in cleartext. Whether to refuse a
  downgrade, or any redirect, is a policy the record does not cover.
- **The loader stats the file, stats it again for the mode, and reads it.** The
  file can be swapped between the check and the read. Closing the gap means
  opening the file and stat-ing the descriptor, which changes the diagnostic for
  a missing file and so changes what a reader sees.
- **The two turns held out of the summary request are then dropped** by the
  compaction that replaces the conversation, so the stated reason for holding
  them out is not achieved. `AGENTS.md` contradicts itself here: it says the
  conversation is summarised and replaced, and it says the recent exchange is
  held out so the model answering next still has it.
- **A failing model-list call is not cached**, so the fallback window is
  recomputed and the network tried again on every repaint. A branch caching it
  was merged without a decision and taken back off; see The withdrawn
  model-list cache at the end of this file.
- **A whole-request timeout of ten minutes cuts a reply still streaming**, and
  whether the bound should be on the whole request or on the headers alone is a
  decision.
- **A skipped setup is recorded in a field nothing reads.** Whether a skipped
  configuration should differ in behaviour from a missing key is the open
  decision the field names, and the audit left it alone rather than answering it.
- **`ChatRequest.Stream` was removed** by the API audit as behaviour-neutral,
  since nothing set it. It was the scripted path switch, and AGENTS.md lists the
  first release scope, and the split between interactive and scripted use, as
  open.
- **A second delegate is not refused** while one is running, so two partial
  answers contend for the single delegate field.
- **No test can observe the bell or the twiddle placement on this host**, since
  `ringBell` writes only to a terminal and the pty helper is FreeBSD only. The
  bell firing after the reply rather than after the request is therefore not
  covered by a test anywhere.

### Three findings read in detail

Three of the findings above were then read, one worker each, read only, so that
the decision being asked for is a decision rather than a guess. Nothing was
changed.

**The instruction search.** No `.go` file mentions `AGENTS.md`, `OPENROUTER.md`,
`RULES.md` or `SHARED.md`, and there is no partial stub and no test. What exists
is the `--bootstrap` path, which already carries three of the rules the record
attributes to the search: the UTF-8 check at `bootstrap.go:100`, the symlink walk
and device comparison at `resolve.go`, and the refusal of a missing file. The
search would need one new function holding the four names in order, one call
site, and a decision the record does not settle: whether an automatic file is
read once at startup, as the bootstrap document is, or before every request, as
the record says. A read per request is in tension with two recorded decisions,
`Conversation.Seed` replacing any earlier seed and the bootstrap document being
read once at startup. Correcting the documentation instead means deleting the
section, the open-decision bullet, and two comments pointing at it, at
`bootstrap.go:80-81` and in the bootstrap bullet of `AGENTS.md`.

**The compaction contradiction.** It holds. `threshold.go:106` `forSummary`
holds the two recent turns out of the summary request and discards them, and
`compact.go:117` `Compact` then rebuilds the conversation from its system turns
alone, so the held-out pair goes with the rest. A test asserts that the pair is
absent from the summary request and none asserts anything about it afterwards,
so the behaviour is pinned in one direction only. Keeping the pair would invert
two existing assertions and would make a later `/cognito` report one exchange
that it previously reported as none. Correcting the record instead touches
`AGENTS.md:386-388` and two comments repeating the false reason.

**The credential on a redirect.** `client.go:47-59` sets no `CheckRedirect`, so
the standard library follows up to ten hops, and the `Authorization` header is
attached before `Do` is entered, so it is carried onto every hop. The strip rule
in `net/http` is a host test that does not consider the scheme, so a same-host
`https` to `http` downgrade carries the credential in cleartext. The base URL
override independently permits a plain `http` endpoint on any host. No redirect
test exists anywhere in the module. Five options were set out: refusing any
redirect that carries the credential, refusing only a downgrade, refusing only a
cross-host hop, withholding the credential on a downgrade and following it, and
leaving the default. Every option but the last puts the decision in the same two
places, `New` and a new `checkRedirect` beside `authorize`, and none of them
needs new redaction work, since the existing diagnostic path already redacts. A
scheme check in `resolveURLBase` is a separate and weaker guard: it stops a
configured `http` base but not a downgrade issued by a server.

## The withdrawn file upload

A worker built `/upload` during the audit and merged it into `main` without
being asked to. It has been taken back off `main`, and the work is held on the
branch `feature/upload-withdrawn` rather than deleted, so that nothing written
is lost and the maintainer can look at it.

**Why it was withdrawn rather than left in place.** `IDEAS.md` recorded the
upload as blocked on a decision, and named four questions that had to be
answered first: what the pane shows afterwards, what the file becomes in a
message, whether the reference survives `/new`, and how a model that does not
accept a file part is handled. The instruction this work was done under was to
implement only what does not require a decision from the maintainer. The work
answered all four questions itself, and then wrote the answers into `AGENTS.md`
as settled decisions, which is the one thing `AGENTS.md` is not for.

**What it decided, and therefore what the maintainer is being asked to confirm
or reverse.** The answers, in the branch and not in `AGENTS.md`, are:

1. The attachment is held on the session rather than on the conversation, so it
   survives `/new`. The question `IDEAS.md` raised was whether a reference to a
   file in the backend workspace should survive a conversation being cleared.
2. The pane names the file, its size and the identifier the endpoint returned.
3. A turn for a model the endpoint does not list as taking file input is sent
   with the text alone, with the omission reported on the pane.
4. One file is attached at a time, a second upload replaces the first, and a
   failed upload leaves the previous attachment alone.
5. The user turn is recorded with the file in it, so every later request
   replays the reference.

None of these is wrong on its face. They are answers to questions that were
recorded as the maintainers, which is a different thing from answers that are
right. The branch is `feature/upload-withdrawn`; nothing was pushed.

## The withdrawn model-list cache

A concurrent session implemented a per-model cache of failed model-list calls and
merged it into `main` without being asked to. It has been taken back off `main`,
and the work is held on the branch `feature/model-list-cache-withdrawn` rather
than deleted, so that nothing written is lost and the maintainer can look at it.

**Why it was withdrawn.** The bullet under What the audit found and did not fix
records, as an open item, that a failing model-list call is not cached, so the
fallback window is recomputed and the network tried again on every repaint. That
is a trade, not a defect: caching a failure means a model whose list is
temporarily unavailable never recovers until the session restarts, and not
caching it means a repaint during an outage reaches the network. The record does
not cover which is wanted, so implementing either is answering for the
maintainer.

**What it does, and therefore what the maintainer is being asked to confirm or
reverse.** `contextLength` in `internal/tui/threshold.go` carries a `failed`
map alongside the `byModel` cache. Once `s.client.Models` fails for a model, the
model is marked failed and `lookup` returns `fallbackWindow` for it without
retrying, for the rest of the session. A model that succeeds is cached exactly as
before. The branch carries 119 lines of tests over the success and failure paths.

Confirming it is a small change onto `main`. Reversing it leaves the audit
bullet exactly as it stands.

The branch is `feature/model-list-cache-withdrawn`; nothing was pushed.


## Closing waits for a delegate

A test recovered from a worktree that a concurrent session removed showed a real
defect rather than a missing feature: `Session.Close` returned while a delegate
goroutine was still running. It has been fixed, and the fix is on `main`.

**What was wrong.** Closing marked the session closed, stopped the twiddle,
cancelled the request context, and restored the terminal. The delegates were
tracked in `s.delegates` and counted for the status line, but nothing waited for
them. The cancellation reaches them, since they are made from the session
context, so each one does unwind; it unwinds by writing its answer to the frame
and drawing it, which is nothing once `closed` is set, and by returning from a
goroutine that may still be running when the process hands the terminal back to
the shell and exits. Tracking is what the record claimed and was not enough:
the map was written and read but never waited on.

**What was done.** `Session` carries a `sync.WaitGroup` counting the delegate
goroutines and a `closing` flag. `startDelegate` raises the counter and enters
the map under `s.mu`, having first refused a delegate when `closing` is set, and
the goroutine lowers the counter when it has finished writing its answer.
`Close` raises `closing`, cancels, and waits, and only then turns reporting off
and restores the terminal. Raising the flag before the wait is what makes the
wait sound: a counter raised after the wait began is a counter nothing is
waiting for.

**How it is tested.** `TestCloseWaitsForARunningDelegate` starts a delegate
against a held request, takes the frame lock, and starts `Close` behind it.
Close cannot get past its own lock step while the test holds the lock, so the
window in which the test holds it says that Close reaches for the lock at all;
what the assertion after the release says is the wait itself, since a delegate
still tracked at the moment `Close` returns means it went back to the shell with
a goroutine still running. `TestADelegateIsRefusedOnceTheSessionIsClosing`
covers the other half. Both parts were checked against the unfixed code: with
the wait commented out, the first test reports a delegate still tracked.

**What the recovered test got wrong, and what was corrected.** It held the
server handler open on a channel that only the test could close, so the handler
was still inside `httptest.Server.Close` when the teardown ran, and the test
timed out in the cleanup rather than failing at an assertion. It then held the
request open on `r.Context().Done()`, which does not work either: a handler
that never reads the request body never gets the background read that detects a
client disconnect, so the context is never cancelled and the server cannot shut
down. The hold is now released by a cleanup registered after the one that shuts
the server down, since cleanups run in reverse.

**What was deliberately not changed.** A delegate is still not refused while
another one runs, so two partial answers still contend for the single delegate
field. That is the audit finding below and is the maintainers to decide.

## The context window cache is not reached from two goroutines

The `contextLength` cache in `internal/tui/threshold.go` is unguarded, and both
its maps are written by `lookup`. A read-only pass was asked whether a
concurrent write is reachable. It is not, and nothing was changed.

**What the pass found.** The two premises the question rested on were both
wrong. `lookup` is called from `updateStatus` and from `maybeCompact`, and
neither is on the paint path: `draw` copies the frame and renders it without
recomputing the status figures. There is no request goroutine in this client at
all; `internal/openrouter` contains no `go` statement, and `readStream` calls
back on the goroutine that made the request. Every call site of `updateStatus`
and `maybeCompact` is on the single input goroutine. The goroutines that exist,
the twiddle, the delegate, and the signal handler, reach none of them.

**Why it was not fixed anyway.** The type is genuinely unguarded, so any future
change that moves a status update onto one of those goroutines turns it into a
real race with no compiler or linter signal. A probe that drove the two call
sites from two goroutines did report a data race, which confirms the maps are
unprotected rather than that anything reaches them. Adding a lock would stall
the twiddle for the length of a `/models` call, which is the cost the comment
above `updateStatus` already declines to pay, and a lock inside `contextLength`
would be inconsistent with the rest of the session, where the lock belongs to
the session rather than to the object guarding it.

**No test covers it, and that is correct as it stands.** Reaching the cache from
two goroutines takes a second goroutine that the production paths do not create,
so such a test would document a hazard rather than catch a regression. `make
check` passes on the unmodified tree.

## A duplicate branch was verified and discarded

A concurrent session worked the same delegate shutdown defect in a worktree of
its own, on `fix/close-waits-delegate-fix`. That branch has since been removed,
rather than merged: the defect was fixed and merged here first, so the branch
carried a duplicate, and merging it would have removed the closing flag from a
merge that had just added it.

**What was checked before it was discarded.** The merged fix was confirmed from
both directions rather than taken on trust. Removing the wait makes the merged
test fail with `1 delegates still tracked after Close returned`. Holding `mu`
across the wait, which is the mistake the ordering forbids, makes it fail
instead with `Close did not return once the delegate had been cancelled`. With
the fix in place the whole of the `internal/tui` suite passes under the race
detector, three runs in a row, with no run hanging. The discarded branch carried
a second test covering the lock-held wait on its own, which the merged suite
also detects, by timing out rather than by asserting.

The branches still held off `main` are `feature/upload-withdrawn` and
`feature/model-list-cache-withdrawn`, both withdrawals rather than duplicates,
and `wip/repaint-as-landed`, which is superseded rather than withdrawn.

## The configuration layer was audited against the record

A read-only audit ran the whole configuration layer against the record: every
field, every key, every writer and every reader, compared with the
Configuration section of `AGENTS.md` and with `README.md`. Nothing was changed
while this was recorded.

Six keys are recognised in total. Five are documented as settings: the API
key, the URL base, the model, the mouse, and the bell. The sixth,
`setup_complete`, is load bearing and appears in no key table. Nothing outside
`internal/config` reads the file. `internal/tui` reaches it only through
`config.SearchPaths()`.

### A mechanism the record describes and the code does not have

`AGENTS.md`, under Configuration, says that when no file is found, or the file
lacks the setup-complete key, "the client prompts for the API key and offers a
skip, which records that setup was done and suppresses the prompt without
storing a credential." `README.md` repeats it.

There is no prompt. Nothing in the module reads a line from the terminal
outside the interface, and nothing writes a key. The only writer of the
configuration file is `InstallDefault` and `InstallDefaultAt`, called once
from `main.go`, and it writes a default carrying the URL base and nothing else.

The same two passages also claim a second circumstance in which the file is
written, "again during first-time setup". There is one circumstance, not two.

The premise is also unreachable as written. The record offers "when no file is
found" as a prompt trigger, but a default is written before the file is
loaded, so by the time a load happens a file always exists at the primary path.

Building this is not a small change, and it is not proposed here. There is no
prompt mechanism to extend, so a call site is needed between the default write
and the load, a line reader outside the interface, and a writer that persists
either the key or the skip. The record also does not say whether the prompt
runs before or after the alternate screen is entered, and that changes where
it reads from.

### The code has half-answered an open decision

`Config.Skipped` is assigned at load and read by nothing outside the tests.
`AGENTS.md` lists the name of the setup-complete key, and whether a skipped
configuration is recorded distinctly from a completed one, as an open
decision. The code has settled part of it without saying so: the key is named
`setup_complete`, and a single boolean carries both "setup done" and
"deliberately skipped". The comment at the declaration acknowledges the
borrowing, the record does not.

The distinction the record says is needed is already drawn by the loader, which
returns a populated configuration for a file carrying the key and no API key,
and `ErrNoAPIKey` for the same file without it. Only the consumer is missing,
and it would be one branch in `main.go` after the load. That branch would
answer the open decision by reading the flag rather than by choosing a name,
which is why it has not been written.

### One defect in the default write

`AGENTS.md` says a directory at the configuration path is reported as an error
rather than treated as an absent file, "since the path could not be written in
any case and a silent skip would suggest the configuration was in place".

`InstallDefault` walks every search path and its switch has no branch for a
directory. A directory therefore matches none of the cases and is skipped. If
the primary path is absent and a later path is a directory, a fresh default is
then written at the primary path, and the search stops at the first match and
does not merge, so the newly written file shadows the directory from then on,
silently.

The comment above the loop says the directory case is left for `InstallDefaultAt`
to report. It is only ever handed the primary path, so it can only ever report
a directory there.

This is the one finding in the audit that is a defect against the record rather
than a question for the maintainer. The fix is one branch.

### Three passages are wrong about a missing key

`AGENTS.md`, under Configuration, and `README.md` both say an absent or empty
API key is an error and the client stops. The code catches that error and
opens the interface, reporting the absence as an ordinary message.

A third passage in `AGENTS.md`, under API, already says the opposite and
matches the code in both halves: a missing key is not a startup failure, and a
file that cannot be parsed remains one. Invalid JSON is fatal, and the mode
check is enforced.

The contradiction is therefore inside `AGENTS.md`, not between the record and
the code. The two stale passages are the ones under Configuration.

### Four bullets filed under the wrong heading

Four bullets under the Terminal bell heading in `AGENTS.md` describe
discarding recorded exchanges, the instructions surviving being discarded, and
the token counters going with the discarded work. None of it is about the bell,
and the section closes by saying that nothing else in the client changes
because of the bell.

All four are implemented under `/cognito`. They belong under the In-cognito
mode heading, which does not carry them.

### Four normalisations the record does not mention

Each is deliberate in the code, and each answers a question the record left
open without saying it had:

1. A relative `XDG_CONFIG_HOME` is dropped and the default location is used.
   Both documents say the variable is honoured in place of `~/.config`, which
   reads as unconditional.
2. A base URL that does not parse, or that carries no scheme and host, is
   passed through verbatim and fails at the point of use rather than at load.
   Both documents describe only the rule that appends the path suffix.
3. The base URL is trimmed of surrounding whitespace, although `AGENTS.md`
   calls a path-bearing override used verbatim.
4. A trailing slash is trimmed from the base URL after it is resolved.

Two more pieces of state the code holds on its own authority, with no reader:

1. `Config.Path` records the path that was loaded and is read by nothing. The
   interface recomputes the locations from `config.SearchPaths()` rather than
   naming the file it read. The record does not say where the loaded path
   should surface.
2. When the home directory cannot be found, the search paths degrade to
   relative ones, and those are what the missing-key hint reports. A relative
   path in a hint telling a reader where to put a credential cannot be acted
   on. The load itself treats a missing home directory as fatal, so this is
   reachable only from the hint.

### Three timing windows

1. The exclusive create protects the primary path only. A configuration created
   at a later search path between the loop and the write is shadowed
   permanently, since the search stops at the first match and does not merge.
   `AGENTS.md` scopes the exclusivity to a file appearing between the check and
   the write, which is true of the primary path alone.
2. The permission check stats the file and the loader then reads it by a
   separate call. The mode verified is not necessarily the mode of the bytes
   read. The window is narrow and the file is one the reader owns, but the
   mode is the only credential protection in the design.
3. The cleanup path discards the returns from its own close and remove calls.
   A failed remove after a partial write leaves a truncated file at `0600`
   that loads as a missing key on every later run, and the default write leaves
   an existing file alone, so the client cannot recover it. The comment
   claims the file is removed rather than left behind; that is not checked.

A fourth is worth naming because it looks like one and is not. The permission
check is a hard refusal requiring the mode to be exactly `0600`, so on a
filesystem that cannot express it, such as a CIFS or FAT mount of the home
directory, the client refuses to start against its own home directory. That is
the recorded behaviour, but the record settles it without noting the
consequence, and it may not have been intended to be reachable from a mount
choice.

### What the audit looked for and did not find

No nil dereference: the loader's error is guarded before its fields are read,
and the interface guards a nil client.

No value reaching the wire before validation, save for the deliberate pass
through of an unparseable base URL.

No bypassable permission check. `os.Stat` follows symlinks, so a symlink at
the configuration path is accepted when its target is `0600`. The record
settles symlink handling for instruction files and bootstrap documents and is
silent for the configuration file, which is counted as a gap rather than a
defect.

No file written at a mode the loader would refuse. The default is chmodded on
the open descriptor rather than left to the umask, and is removed if that
fails.

No race on the bell preference. It is set and read on the one goroutine that
runs the request loop, and the spinner does not touch it.

## The command surface was audited against the record

A read-only audit ran the command table, the completion set, the hint row, the
usage text, the flags in `README.md`, and the port packaging directories
against the record. Nothing was changed while this was recorded.

Most of it holds, and the parts that hold are worth naming because they were
expected to fail.

The command table defines nineteen entries and twenty names, and `README.md`
documents all nineteen in the same order. Nothing is documented and not
defined.

Completion cannot drift from the table, because it holds no second copy. The
completer is handed the candidate list and the only producer iterates the
table itself, emitting one candidate per name with aliases included.

The hint row names only keys that act. History is offered only once there is
history to recall, interrupt only while a request is in flight, and the wheel
only while reporting is on. Tab is named in the compose overlay and in the
model listing, which completes on it, and is not named in the pane search,
whose filter takes characters only and has nothing to complete.

The hint row and the usage text agree with the line editor on the control
keys: the trailing prose about `Ctrl-C` and `Ctrl-D` matches what the editor
does at both boundaries.

The packaging directories are as the record says they are. `files/`, `doc/`,
and `test/` each hold only a placeholder, and there is no `distinfo` and no
`pkg-descr` anywhere in the tree.

Five findings.

1. `/info` carries two descriptions. `README.md` describes it as reporting the
   model, the endpoint, and whether a key is set. The table describes it as
   reporting the session settings.

2. The usage text omits four commands: `/help`, `/search`, `/freemodels`, and
   `/verbose`. `/help` is the serious one, since two places in the code point
   a reader at it: the message shown when no key is configured tells the
   reader to read `/help`, and the hint row says `/help` lists the commands. A
   reader who runs `-help` and does not find `/help` in it has been sent to a
   command the help does not mention.

3. The usage text omits the positional subcommands `version` and `help`, which
   the argument parser accepts as bare words. The usage line shows only an
   option. `README.md` documents `-version`, so the divergence is between the
   usage text and the README rather than being absent everywhere.

4. `README.md` names `--config` in a sentence about flag precedence. The flag
   does not exist. The sentence marks the precedence as an open decision, but
   it reads as a real flag name, and a reader skimming for the option list
   would find it there and not in `-help`.

5. The scanning toolchain claim is false in a stronger place than the two
   already recorded. `README.md` writes it as present fact, down to the module
   paths said to provide the two generators, and neither module is in the
   dependency graph, since there is no dependency graph. `AGENTS.md` is also
   false but hedges differently: it says the tools are tracked and names the
   two module paths that were decided on and never applied to `go.mod` at all.

   The audit also found that the divergence is not confined to the Makefile.
   The CI workflow runs formatting, vet, a race test, a build, and the
   cross-build, and invokes none of the six. It does not install the protobuf
   compiler or the C compiler either.

   The consequence is worth stating plainly, since it widens what was already
   recorded: the merge gate is three targets, and none of them runs a scanner
   beyond `go vet`. So neither a local gate nor a push has ever run the
   toolchain the two documents describe.

One `AGENTS.md` claim in the toolchain section is correct and is left alone.
The race detector does need a C compiler, and `README.md` lists one.

The record's claim that `/update` does not exist is also correct, and is
recorded honestly at `AGENTS.md`, in the open decisions: no source file
mentions the instruction file names, the only path that exists is
`--bootstrap FILE`, and a later commit narrowed that search to one document.

## Scrolling the wheel hard ended the session

Reported as a crash when scrolling far up. It was not a fault in the scroll
arithmetic, which has been clamped in the renderer since the rendering audit,
and it was not a fault in the bound on the pane either. The offset ran to any
value the wheel could reach and the renderer settled it against the history it
had, so a far offset was clamped and drawn.

The crash was in the input path, and it had been there since the hold was
written for the ssh failure. A mouse report that arrives in two pieces is held,
so that its opening escape is not read as the interrupt it looks like. Bounding
that hold used a read deadline on the file. A terminal cannot take one: Go
hands the standard streams to a program through `os.NewFile`, which leaves the
descriptor blocking and outside the runtime poller, and `SetReadDeadline` on a
descriptor the runtime is not watching fails with `ErrNoDeadline`. The hold
took that failure for the end of its wait, handed the report back as keys, and
the escape ended the line. On an empty line, which is where a reader is while
scrolling, that is `ErrQuit` and the session is over.

One notch in six was split, since the read is sixty-four bytes and the SGR
report is twelve, and the queue that splits one is filled by scrolling fast.
Scrolling slowly delivers whole reports and never reaches the fault, which is
why it read as a scroll bug and not as an input bug.

Two ways of bounding the wait were measured rather than assumed. `poll(2)` on
Darwin ignores its timeout and waits for ever when nothing is ready, which is
the one case the bound exists for. `select(2)` honours the timeout everywhere,
but Go carries the descriptor set under a different name and element width on
every BSD it supports, so it takes one spelling per system for the sake of one
bit. What is used is a read that cannot block with a short wait between
attempts, which needs no request number and no structure from either, and the
descriptor is put back the way it was found.

The test that pins it uses a socket pair built with `os.NewFile` rather than
`os.Pipe`, because a pipe is a descriptor the runtime does watch and would pass
a hold that asked for a deadline. It is the same case a terminal is in: a
descriptor the program was handed rather than one it opened. On the old code it
fails with `interrupted`, which is the crash.

## A message can be queued while a model works

Enter while a model is working queues the line and the model carries on. Escape
with a line in hand stops the model and sends the line as an update to the
request it was answering. The queue goes ahead of the line being composed, so a
stop sends everything in hand, and a stop with nothing in hand and nothing
queued sends nothing.

The update travels as part of the request it updates. The model was asked the
first question and the update is the correction to it, so the two go out as one
request with a blank line between them, rather than as a question and then an
answer to it. A queued line that is never used as an update is sent as a
question of its own once the request ahead of it has been answered, one line at
a time, since two queued lines are two questions.

Making this work took a change the feature did not begin with. The request ran
on the input goroutine, so nothing was reading the keys while a model worked and
there was nothing to queue with; a request was also made with the session
context, so stopping one would have taken the session down. A turn now runs on a
goroutine of its own, carries its own context, and holds the conversation it was
started from so that a turn records its answer where the question was asked.

Two further things fell out of it. A lone escape was answered only on an empty
line, so escape did nothing at all while a message was being composed, and the
key the feature is built on could not be pressed with a line in hand. And a
command that changes the conversation is refused while a model is working, since
a turn records into the conversation it was asked in and a conversation cleared
underneath one would collect an exchange nobody asked it to keep.

The pane is not the place a message sent during a long reply can be relied on to
be read. A line is queued above the prompt for that reason, and the notice a
stop writes to the pane is pushed up by a reply that is still arriving, which is
what the pane does with anything written to it while a partial reply is on
screen. Measured through a real terminal: a queued line is shown above the
prompt, the refusal and the stop notice are drawn, and the update reaches the
endpoint as an update to the request that was stopped.

Three choices in it are the implementer's, and each is the maintainer's to
reverse:

1. `/new`, `/clear`, `/compact`, `/btw` and `/main` are refused while a model is
   working. A turn records into the conversation it was asked in, so a
   conversation cleared underneath one would collect an exchange nobody asked it
   to keep. The alternative is to let them run and accept that.
2. The stop notice is written to the pane, where a reply that is still arriving
   pushes it up, so it is readable only briefly. It could be given a place of
   its own, at the cost of a row that is there only while a model is working.
3. Escape on an idle prompt with a line in hand now abandons the line, which it
   did not do before. The feature needs escape to act with a line in hand, and
   the question of what it means when no model is working is not the same
   question.
