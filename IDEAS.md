# Work in progress

The working list for this repository. `AGENTS.md` records why the project is
shaped as it is; this file records what is being built and what has not been
finished. Anything here that contradicts `AGENTS.md` is a bug in one of the two,
and `AGENTS.md` is the one that decides.

Research notes on features to consider live outside the repository, at
`build/openrouter-cli-worktrees/ideas/`. What is here is the state of the work
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
`build/openrouter-cli-worktrees`, no `hintrow` source, and no uncommitted `hintrow`,
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
by the work landing: a paragraph still describing `completion` and `markdown`
as unlanded after both had merged, and a sentence saying the audit had not
started after all six areas had merged. A record that contradicts itself is a bug
in the record, as the ground rule at the top says.

Choices made during implementation that the maintainer has not confirmed are
recorded under Awaiting confirmation.

Two read-only audits were run after that, against the configuration layer and
against the command surface. Both have landed their findings here and are at the
end of this file. Neither changed a line of code or of documentation.

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

**Fixed.** The audit took it up first, and the fix is merged. A folded repaint
is now owned by a timer rather than by whatever asks for a repaint next, and every
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
   Enter key would behave differently for the same text unless one of the two
   is changed.
2. Whether a search opened with a query should still be incremental, so that
   typing appends to it. The caret sits after the filter, so appending must
   be done by placing the caret rather than by writing to the end of the buffer, or
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

### Ctrl-C cancels a queued message

Raised by the maintainer. An intent rather than a decision.

Enter queues a line while a model is working, and escape stops the model and
sends the queued line and the composed line as an update to the request it was
answering. A third key is wanted: one that removes a queued line without sending
it.

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