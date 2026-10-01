# Work in progress

The working list for this repository. `AGENTS.md` records why the project is
shaped as it is; this file records what is being built and what has not been
finished. Anything here that contradicts `AGENTS.md` is a bug in one of the two,
and `AGENTS.md` is the one that decides.

Research notes on features to consider live outside the repository, at
`~/openrouter-cli-worktrees/ideas/`. What is here is the state of the work
itself.

## Last updated

At `main` commit `f8ecbac`, with a clean working tree. Work in progress is
`hintrow` and `instructions`; `completion` and `markdown` hold written but
untested work; four worktrees are stale. The audit has not started.

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

## In progress

### The key hint row

Worktree `~/openrouter-cli-worktrees/hintrow`, branch `feature/hintrow`.

A footer row naming the keys that currently do something, changing with the
state. Only keys that act in the state are named, so the row never promises a
key that does nothing.

State: **written but not integrated, and does not build.** The files are
`internal/tui/hintrow.go` and `internal/tui/hintrow_test.go`, both complete, and
`session.go` is edited to fill `frame.Hints`. What is missing is the renderer
side: the `Hints` field on `Frame` and the row budget in `Render`.

Three tests fail against the current state, and there is no working version of
the renderer change to fall back on. An earlier attempt at this merge produced
an infinite loop in a row-trimming loop and was reset rather than half-landed.

The layout question that has to be settled first: the row sits below the prompt,
so it competes with the pasted rows and with the pane for the same height. The
frame bounds work landed since then and already owns that budget.

The worktree has been rebased onto `ccf34c1`, so the renderer it is merging into
is current. Only the renderer side is missing, not the base.

### The instruction file search

Worktree `~/openrouter-cli-worktrees/instructions`, branch
`feature/instructions`. Created and empty. Nothing written, not even a failing
test, so it is a placeholder rather than work.

Intended to be an `/update` command that re-reads the model instruction file.

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

Present as an empty worktree, or named but with no work in it. Remove the
checkout and recreate it when the work begins.

| Worktree | Branch | Subject |
| --- | --- | --- |
| `completion` | `feature/completion` | Slash command completion on Tab. |
| `markdown` | `feature/markdown` | Render headings, lists, and emphasis. |
| `cmdqueue` | `feature/cmdqueue` | Not recorded. |
| `mfilterfix` | `feature/mfilterfix` | Model filter, since merged. Stale. |
| `noninteractive` | `feature/noninteractive` | Scripted, non-interactive use. |
| `stopcancel` | `feature/stopcancel` | Cancelling a turn in flight. |

`instructions` is listed under In progress rather than here, because the
discrepancy it is blocked on needs an answer before it becomes work.

`completion` and `markdown` have uncommitted work in them. `completion` is 129
lines with no test file at all. `markdown` is 297 lines, unwired, also untested.
Both need a rebase before they can land, and both need tests written.

`cmdqueue`, `mfilterfix`, `noninteractive`, and `stopcancel` hold no changes and
are stale checkouts at `cd761d5`. `instructions` is empty but is listed above,
since it is blocked rather than abandoned.

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

## Outstanding

### The code review and audit

Requested and not started. It waits on the backlog above, since reviewing code
that is about to change is wasted effort.

The instruction is to fix and land what comes up, and to land nothing that does
not have a resolution.
