# Shell allowlist

The set of programs the shell tool will run, and the argument checks every program
on that set is held to.

Code: `internal/tools/shell.go`, `internal/tools/shellargs.go`.

## The invariant

The tool will run one of thirty-five named programs and nothing else. A program
outside the list is refused by name, before the filesystem is asked about it and
before the reader is interrupted about it. The refusal names the list, so a model
that asked for something outside it learns what it may ask for.

The check is made at the head of `call`, rather than after the directory is
resolved or after the reader is asked, because a program that would be refused
outright is not something to interrupt somebody about.

Being on the list does not mean a program runs. The list bounds what may be
proposed and nothing more; every call is still put to the reader unless a mode or a
file rule settles it.

## The list

Thirty-five names in declaration order. The order is not sorted, because the
declaration groups the build tools first and a model reading a refusal looks for
the name it wanted near what it already knows.

**Build and toolchain, eleven names.** `go`, `gofmt`, `make`, `bmake`, `git`,
`errcheck`, `gosec`, `govulncheck`, `protoc-gen-go`, `protoc-gen-go-grpc`,
`staticcheck`. `bmake` sits beside `make` rather than in place of it, since a build
in this repository may have been written for either and they are different programs
on a FreeBSD host. The scanners are tracked in `go.mod` as module tool dependencies
and run through `go tool`. The two protoc generators are named rather than reached
for by a path, so a reader approving one is approving a program rather than a
location that could be replaced underneath them.

**Readers and inspection, twenty-one names.** `ls`, `cat`, `pwd`, `echo`, `grep`,
`rg`, `find`, `wc`, `head`, `tail`, `sed`, `awk`, `stat`, `file`, `diff`, `hexdump`,
`od`, `jq`, `ps`, `sqlite3`. `diff` writes when given `-p` or an output option, and
that is recorded rather than separated out: there is no rule telling the forms that
write from those that do not, so such a diff is reached only through a question,
which is the same bound that covers `rm` and `gh`.

`notmuch` is an email indexing and search tool, and `sqlite3` is here because the saved-session format
could be opened afterwards with any SQLite tool rather than only with this client.
The client writes those files with a pure Go driver and never runs this program to
read one, so it is on the list for a model working on this repository rather than
for the client itself. It writes as well as reads, since a `drop table` drops one
and an import creates one, and that is recorded on the same terms as `diff`.

**Network, two names.** `urlview`, `urlscan`. These are the only entries that fetch
from the network, and every other name above reads the tree or writes to it, so a
model held to the rest of the list cannot send anything out of the machine. A
reader approving one of these is approving something the others cannot do, and that
is the thing to know before answering the question.

`urlview` prints what a URL holds, and `urlscan` prints that page with the patterns
a security tool looks for marked in it. Neither is on every host, both being ports
rather than base programs, so a host without one is refused at exec and reported as
a failed command rather than as a permission that is missing.

**Writers, two names.** `gh` and `rm`. They are on the list because they write. A
model repairing a tree needs to remove what a build left behind, and a model
working on a repository needs to read and act on a pull request; refusing both would
mean the reader does them at a second prompt.

## What a network reader is not given

A URL is an argument rather than a path, so `containedArgs` reads it as one and
holds it to the tree. That check answers where a call begins and reaches rather
than what it does with the argument it was given, which is already the case for
`git` and for `make`: `git --git-dir=/elsewhere/.git` reaches out of the tree
without any argument naming a path that `containedArgs` would refuse.

The one form worth naming is a `file:` URL, since it is the single scheme with no
network between the model and the filesystem. Whether it is refused is recorded in
`IDEAS.md` under the Chrome entry as undecided, and nothing here decides it. A
refusal by scheme is the tool that entry describes rather than a name on a list,
and a rule written here would be a partial one: it would stop `file:` while leaving
a model able to read the same content over `http:` from a server it controls.

## Three copies of the list, two of them derived

`shellPermitted` is the declaration. `shellPermittedMap` is built from it in a
variable initialiser that iterates it, so the two cannot drift. `shellParameters`
spells the same list out a third time in the JSON schema the model is shown, and
that copy is held honest by a test rather than by construction.

## Resolution and the bound

Every name resolves by bare name through `PATH`, so the program that runs is
whichever binary of that name the session reaches first. A name the host does not
have is refused at exec and reported as a failed command rather than as a missing
permission.

Arguments go to the program as an array and are never read by a shell, so a pipe, a
redirect and a chain are not expressible and no program on the list can reach
another program.

## The argument checks, in order

1. The program against the allowlist.
2. The path the command would run in, resolved. A path leaving the tree is refused
   before a filesystem lookup, so a refusal does not depend on whether the path
   exists.
3. The arguments against that directory. A question naming a command that would
   reach outside the tree is a question about something the reader cannot see.
4. The reader, once there is something specific to ask about.

## What the argument check actually enforces

A working directory is not a sandbox: a process given one can still open any path it
can name, so `rm -rf ../..` or `cat /etc/passwd` would run with the reader's own
privileges. Setting the directory bounds where a program starts, not where it may
go, and closing that gap is what the check is for.

- An absolute argument is held to the same bound as any other, namely that it must
  land inside the directory. The destination is refused, not the spelling.
- An argument beginning with a dash is never judged as a path.
- A pattern is judged by the directory leading to the first wildcard, so `*.go` is
  judged as the directory it expands against and `../*.go` is refused.
- A path that does not exist is resolved as far as the filesystem allows, since
  `rm` may be asked to create what it is about to remove.

Both sides are resolved through `EvalSymlinks`, since on Darwin a temporary
directory is `/var/folders/...` and resolves to `/private/var/folders/...`, and
`filepath.Rel` would report an unresolved and a resolved path as unrelated places.

## Timeout and output

A single command is bounded at two minutes, which is longer than the git tool's
thirty seconds since a build legitimately takes longer than a status. The wait
continues for 500ms after the deadline before the output pipe is closed, since a
killed program can leave a child holding the pipe.

Output is capped at 1 MiB and the cap is reported rather than cut, since a command
whose output stops mid-commit reads as though it were the whole of what was asked
for.

A program that succeeded and printed nothing says so, since an empty result reads
to a model as though the call had been dropped. A failing program prefers stderr,
but falls back to stdout, since `diff` prints what changed and exits 1 when the files
differ, which is the answer rather than a fault.

## The uncommitted work in this subsystem

`internal/tools/shellargs.go` carries a one-line change: `cutWildcard` lost its
second return value, which was a boolean always equal to the `ok` it already
returned. The change removes a redundant value. `shell_test.go` and
`shelldump_test.go` carry small fixture adjustments that go with it.