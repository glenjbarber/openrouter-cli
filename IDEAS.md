# Work in progress

The working list for this repository. `AGENTS.md` records why the project is
shaped as it is; this file records what is being built and what has not been
finished. Anything here that contradicts `AGENTS.md` is a bug in one of the two,
and `AGENTS.md` is the one that decides.

Research notes on features to consider live outside the repository, at
`~/openrouter-cli-worktrees/ideas/`. What is here is the state of the work
itself.

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
The maintainer decides which.

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

The four others hold no changes and are stale checkouts.

## Outstanding

### The code review and audit

Requested and not started. It waits on the backlog above, since reviewing code
that is about to change is wasted effort.

The instruction is to fix and land what comes up, and to land nothing that does
not have a resolution.
