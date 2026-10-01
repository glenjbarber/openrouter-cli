# Work in progress

The working list for this repository. `AGENTS.md` records why the project is
shaped as it is; this file records what is being built and what has not been
finished. Anything here that contradicts `AGENTS.md` is a bug in one of the two,
and `AGENTS.md` is the one that decides.

Research notes on features to consider live outside the repository, at
`~/openrouter-cli-worktrees/ideas/`. What is here is the state of the work
itself.

## Last updated

At `main` commit `7c511b2`, seven commits ahead of `origin/main` and not
pushed.

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

The instruction search is blocked on a decision. The audit has not started. It
waits on the backlog, since reviewing code that is about to change is wasted
effort.

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
| Slash command completion | `c453b63` | Tab completes, from one table of command names. |
| Markdown rendering | `3189907` | Headings, lists, and emphasis, rendered as plain text. |
| Key hint row | `7c511b2` | Names the keys that act, on one row above the prompt. |

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
worktrees. They have landed, see Shipped. The uncommitted code was not
recoverable on this host and was written again from the description above.

`completion` and `markdown` have uncommitted work in them. `completion` is 129
lines with no test file at all. `markdown` is 297 lines, unwired, also untested.
Both need a rebase before they can land, and both need tests written.

`cmdqueue`, `mfilterfix`, `noninteractive`, `stopcancel`, and `instructions` held
no changes and were removed, along with their branches. Nothing referenced them
and none had been pushed. A checkout with no changes is a stale one rather than
work in progress, and the distinction is recorded under Worktrees in
`AGENTS.md`.

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

Requested and not started. It waits on the backlog above, since reviewing code
that is about to change is wasted effort.

The instruction is to fix and land what comes up, and to land nothing that does
not have a resolution.
