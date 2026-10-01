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
this file, along with what it found and did not fix. A piece of work was taken
back off `main` during it and is recorded under The withdrawn file upload.

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

## In progress

### The instruction file search

Intended to be an `/update` command that re-reads the model instruction file.
No worktree exists. One was created and removed, since an empty checkout is a
stale one rather than work in progress.

**Blocked on a discrepancy.** `AGENTS.md` describes a search over
`AGENTS.md`, `OPENROUTER.md`, `RULES.md`, and `SHARED.md` in the working
directory, read before every request. No `.go` file mentions any of those
names. The only path that exists is `--bootstrap FILE`, named on the command
line.

So there are two possible pieces of work and they are not the same: build the
search the documentation describes, or correct the documentation to match the
code. An `/update` command has nothing to re-read until one of those is done.

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
a second timer for one job. It is kept rather than deleted until the withdrawn
upload branch is decided, since both were branches taken off `main` and should
be read together.

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
| Configuration | `internal/config`, `internal/bootstrap`, `cmd/openrouter-cli` | `9e8cbab` |
| Input | `line.go`, `keys.go`, `paste.go`, `mouse.go`, `hintrow.go` | `d669b05` |
| Conversation | `conversation.go`, `compact.go`, `cognito.go`, `btw.go`, `delegate.go`, `threshold.go`, `verbose.go`, `bell.go`, `spinner.go` | `aa149ac` |

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
  recomputed and the network tried again on every repaint.
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
