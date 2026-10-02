# AGENTS.md

Editing notes and settled decisions for this repository. This file is the record
of why the project is shaped as it is. It is read before any change is made.

## BE ADVISED
The code in this repository directly affects you, so do not break it.

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
- DragonFly was a supported target until the saved sessions needed a database
  driver. The driver carries an emulation of the C library and that emulation
  has no DragonFly in it, so `/save` could not work there at all. The target is
  dropped rather than half supported, and the reason is recorded rather than
  left to be discovered.
- Windows is a possible later target and is not yet committed to. It does not
  build, and the reason is recorded rather than left to be discovered: the
  device comparison in the bootstrap loader reads `syscall.Stat_t`, which does
  not exist there.

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
- `OPENROUTER_MOUSE` is optional and asks for mouse reporting, so that the
  wheel scrolls without `/mouse` on every run. It is a preference rather than a
  setting, since the decision is settled per session by where the session runs
  and by whether the reader can still select text.
- A file that cannot be read for its key is still read, so the mouse preference
  survives alongside the model. A file carrying a preference must not appear
  unread merely because it holds no credential.
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
  checksum file rather than a silently unpinned build. It was absent until the
  saved sessions took a database driver, and it is committed from then on.
- The `go` directive is a patch version, `go 1.26.0`, and a dependency decided
  it rather than a preference. `modernc.org/sqlite` declares `go 1.26.0`, and a
  dependency's directive is a floor this one cannot be set below: lowering it
  to `go 1.26` makes `go build` refuse with `updates to go.mod needed`, and
  `go mod tidy` puts it back without saying why. The earlier rule, that the
  directive should name a minor version rather than a patch, held until the
  saved sessions took a driver and no longer holds.
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
  document. A `.db` file is a conversation saved with `/save` and is resumed
  from rather than read as prose.
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
  told anything. A session is judged on what it carries rather than on its
  instructions: one with turns and a model has something to begin from, and one
  with neither is refused for the same reason.
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
  instruction, and `Approval` was removed with them because it implies a
  permission system for tool calls and the client had no tools and executed
  nothing. `Approval` came back with the shell tool, which is what gave it
  something to report. A dash that can never be filled is noise.
- The provider is a constant rather than a derived value, since the endpoint is
  the only one the client speaks to.
- The state reads `Working` while a request is in flight and `idle` otherwise,
  restored on every exit including a failure.
- The field named `Credits` carries the allowance the key endpoint reports as a
  figure against a limit. It is not the conversation context, which is the
  share of the model window a message occupies.
- The field named `Context` carries that share, as a percentage of the window.
  A percentage is shown rather than a figure because what a reader watches for
  is how close the conversation is to needing a compaction, not the raw count.
- The two are kept apart deliberately. Credits answers whether there is money
  left; Context answers whether the conversation must be compacted. A reader
  confusing them would misread one as the other.
- The context share is kept in preference to the allowance when the bar is
  narrow, since it is the more urgent of the two.
- An unknown window shows no share rather than a figure against nothing.
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
- The status bar shows Provider, Model, Status, Approval method, Context used,
  Tokens used in and out, and hostname. `Reasoning` and `Branch` were removed
  along with `Approval`, and `Approval` has since come back with the shell.
- Commands and configuration options are completed with the Tab key.
- A completion that matched nothing is reported on the input block, beside the
  prompt, and not in the pane. It is about what the reader is typing rather than
  about the conversation, and a notice written among the replies becomes a line
  of output they have to read back through the history to find.
- A notice is a separate field rather than a replacement for the composed line,
  since a reader who pressed Tab has not asked to lose what they typed.
  Overwriting the line would lose a half composed command over a keystroke that
  was only meant to help with one.
- A notice is cleared by the next keystroke and by a completion that succeeded.
  A reader who has moved on from what it said should not have to dismiss it, and
  one who has not sees it again on the next completion.
- The interface behaves correctly with a terminal and mouse combination, and
  within tmux.
- Text is always copyable with the ordinary terminal selection gesture, and a
  selection yields plain text with no escape sequences or padding.
- Mouse reporting is not enabled unconditionally, because capturing the mouse
  intercepts the drag that begins a selection. It is disabled inside tmux by
  default, disabled whenever output is not a terminal, and suspended for the
  duration of a selection gesture.
- The wheel scrolls the reply pane when reporting is on. It is the only mouse
  input that is acted on, since a drag has no meaning here and reading one would
  only add input the interface cannot use.
- Reporting is asked for with `/mouse` or with `--mouse`, and a file may set
  `OPENROUTER_MOUSE`. Inside tmux the flag and the file are both ignored unless
  the reader runs `/mouse`, because the drag that begins a selection is the
  gesture a nested terminal is most often used for.
- Only the button-event mode is enabled. The modes that also report a drag or
  every pointer movement would fill the input stream while the mouse merely
  crosses the window, and nothing here would act on what they send.
- The SGR encoding is asked for as well as the mode. The older report carries a
  coordinate as a single byte, and a terminal that is not asked sends it, so a
  pane past 223 in either direction cannot be scrolled: the terminal reports the
  last position it can express and the view stops at the edge. Terminal.app does
  this. The encoding is a separate setting from the mode, turned on before it and
  off after it, and the restore on the way out goes through the same path since a
  terminal left in the SGR form reports in it to whatever runs next.
- A copy does not depend on reporting being on or off. It travels the same path
  as the drawing, and a reader who turned reporting off did so to select text,
  which is the gesture a copy is.
- The offset is a count of lines back from the newest output, and zero is the
  bottom. A line count rather than a stored history is used so that a reply
  arriving needs no separate record of what was seen: the pane is folded at
  render time, so the line count is the only thing that is stable across a
  reflow.
- A reply arriving does not move the view. A reader part way through the history
  would have it move under them, so only scrolling down returns the view to the
  bottom. Clearing the pane does move it, since the history it was measured
  against is gone.
- The offset is clamped so that a full pane of history remains. Letting it run
  to the last line would leave one line at the top of an otherwise empty frame,
  where scrolling to the top should settle on the oldest lines and fill the pane
  with them.
- A notch moves three lines. Three is small enough that one notch does not throw
  a reader past the paragraph they are on, and large enough that the top of a
  long conversation is not a hundred notches away.
- The marker is on the title row rather than a row of its own. A row taken for
  it would change the height of the pane the moment the reader scrolled, and a
  pane that resizes under the reader is worse than no marker at all.
- Input is read in blocks rather than a byte at a time. A terminal writes a
  whole mouse report in one piece, so a block read returns the report complete
  and it is recognised before it can be read as a key. Reading a byte at a time
  would see the opening escape alone, and a lone escape is an interrupt, which
  is the failure this avoids.
- A report split across two blocks is held until it is whole. A reader is free
  to return a short read, and the tail of a partial report would otherwise be
  read as keys, with its escape ending the session.
- A lone escape is still a key and still interrupts. It is the one byte at the
  front of the buffer that could be a report, and it is deliberately not held,
  since treating it as a report would swallow the interrupt it stands for.
- An escape interrupts whether or not there is a line in hand. It used to be
  answered only on an empty line, which left the key doing nothing at all while
  a message was being composed, so a reader pressing it to abandon what they had
  typed had no way to say so. What the interrupt means is decided by the session,
  since escape abandons a line on an idle prompt and stops a model with it while
  one is working.
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
- A request runs on a goroutine of its own, so that the input loop keeps reading
  while a model works. With the request on the input goroutine nothing is
  reading the keys, so a message could not be queued and a model could not be
  stopped at all.
- A request carries its own context, taken from the session. Stopping a model
  stops that request and not the session, which is the only reason a turn can be
  stopped without leaving.
- A turn holds the conversation it was started from, rather than reading the
  session's. A turn records its answer where the question was asked, and a
  conversation switched underneath one would file the answer in the wrong place.
- A request cut short by the reader stopping the model is reported as stopped
  rather than as an error, and the text that had arrived is kept. A cancellation
  the reader asked for is not a fault, and telling them so would report a break
  they had caused.
- A turn the reader stopped records nothing, on the same terms as a turn that
  failed. Recording a partial answer as though it were a whole one would have the
  model carry it into the next request as something it had said.
- `/new` clears the conversation and keeps the bootstrap document, since losing
  it would silently change how the model behaves.
- A multi-line reply occupies one pane row per line, since a reply carrying
  embedded newlines would otherwise push the frame down the screen.
- Each pane row is cleared before it is written. Without that, a repaint shorter
  than the frame before it leaves the tail of the longer one visible, so a
  reply appears twice.
- A line that has been sent is followed by a blank row. Without it the reply
  reads as a continuation of the question that asked for it, and two exchanges
  in a row are not told apart at a glance. The row is a row of the pane rather
  than padding drawn around the reply, since a selection out of the pane has to
  yield the text with nothing in it.
- The two refusals a turn can make, a missing credential and an unselected
  model, are not followed by a blank row. The reason they give belongs with the
  line that provoked it rather than under it.

### Getting started

- The reply pane carries a hint while it is empty, naming whatever is missing: an
  absent key, an unselected model, or simply that a message can be typed. The
  hint is removed once a conversation has started, since it is then in the way.
- Without the hint a first run shows an empty pane and a prompt, which gives no
  indication that plain text is the message and that a model must be selected
  first.

### Queued messages

- A line sent while a model is working is queued rather than refused, and the
  model carries on. Enter does the queueing, and escape is what sends it.
- Escape with a line in hand stops the model and sends the line as an update to
  the request it was answering. The queue goes first and the line being composed
  second, since the queue is what was committed first. Escape with nothing in
  hand and nothing queued stops the model and sends nothing, which is a stop on
  its own.
- The update travels as part of the request it updates rather than as a question
  after it. The model was asked the first question and the update is the
  correction to it, and a blank line separates the two without marking either,
  since a marker would be read as part of what was asked.
- A queued line is sent as a question of its own once the request ahead of it has
  been answered, and one line at a time. Two queued lines are two questions, and
  sending them together would ask them as one.
- A stop claims the queue under the lock that registers a turn, so a turn that
  ends at the same moment cannot also drain it. Without that the reader would be
  sent the same line once by the stop and again by the turn finishing.
- The stop waits for the turn to settle rather than for the request to stop, since
  a turn that has not finished unwinding would still be writing to the pane after
  the update was sent.
- The queue is drawn above the prompt, one row per line and marked as queued, and
  is cut and reported rather than allowed to cost the prompt. The decision to
  stop a model cannot be made against a queue that is not shown.
- The hint row names Enter as queueing and escape as stopping while a model is
  working, since a key named for what it does on an idle prompt would be naming
  something else.
- A command that changes the conversation is refused while a model is working.
  A turn records its answer into the conversation it was asked in, so a
  conversation cleared underneath one would collect an exchange nobody asked it
  to keep. The refusal says what to do about it.
- The turn in flight is waited for on the way out, on the same terms as a
  delegate. A turn is not started once the session is closing, since it would be
  a request made against a terminal nothing is drawing on.

### Worktrees

- Feature and bug work is done in a git worktree under
  `build/openrouter-cli-worktrees` inside the checkout, one directory per piece
  of work, named for it.
  Work is merged into `main` after it is implemented and tested.
- The main checkout is left on `main` and is not edited directly for a feature.
  A worktree keeps an unfinished change from sitting on `main`, where it would
  be pushed by anything that pushes the branch.
- The worktree is created from `main` and its branch is merged back with
  `--no-ff`, so that the work is visible as a unit rather than as a row of
  commits indistinguishable from the rest.
- A worktree is removed after the merge, and the branch is deleted with it. A
  stale worktree holds a whole checkout of disk that nothing refers to.
- A worktree holding no changes is not work in progress. It is a stale
  checkout, and the distinction matters: one is a thing being built and the
  other is disk that could be reclaimed.
- `IDEAS.md` in the repository root is the working list. It records what is
  being built, what is unfinished, and what has been shipped. Research notes on
  features that are only being considered live outside the repository, at
  `build/openrouter-cli-worktrees/ideas/`.
- `IDEAS.md` is a status, not a decision. Where it disagrees with this file,
  this file decides, and the disagreement is a bug in one of them.
- Every merge is gated on `make lint`, `make check`, and `make crossbuild`. The
  cross-compile is part of the gate rather than a separate chore, since the
  terminal layer names ioctl requests that differ between platforms and nothing
  else would notice.

### Compaction

- A long conversation is summarised and replaced before the request that would
  exceed the window is sent. Checking beforehand matters, since a request past
  the window is refused outright and the turn is lost with it.
- `/compact` runs a compaction on demand, regardless of size. Asking for it is
  the point.
- The threshold is 75% of the window. Compaction costs a request of its own,
  and leaving only a small remainder would compact again immediately after the
  next exchange, so the work starts while there is still room to do it.
- The window is the context length reported by the model list, cached per model.
  Fetching it per request would add a call to every message for a value that
  does not change.
- A model the list does not carry falls back to a common window rather than to
  an unbounded one, which would disable compaction entirely.
- Conversation size is estimated at four characters per token rather than
  counted, since counting exactly needs the model tokenizer. The estimate is
  deliberately generous so that compaction starts before a request would fail.
- A conversation too short to gain from compacting is left alone. Summarising
  two turns produces something no shorter than itself, which costs a request to
  achieve nothing.
- The summary is a system turn, so it governs every later request. It carries a
  marker, and a second compaction replaces it rather than summarising a
  summary, which would compound the loss on every pass.
- The opening instructions survive a compaction. Losing them would change how
  the model behaves without anything saying so. The instructions are also held
  out of the summary request, since a summary of them would compete with the
  original rather than add to it.
- The most recent two turns are held out of the summary request, so the model
  answering next still has the immediate context rather than only a description
  of it.
- The summarisation asks for decisions, constraints, identifiers, and what is
  unresolved, rather than for prose. A summary that paraphrases the intent is
  useless for continuing the work.
- The conversation is left untouched when a summary fails, since a
  half-summarised history is worse than a long one.

### Pasted input

- Bracketed paste is enabled on the alternate screen and turned off before it
  is left, so that a paste arriving after the interface closes is not swallowed
  by a mode the terminal still believes is on.
- Without the markers a paste is indistinguishable from someone typing very
  fast, and a newline inside one would submit the line halfway through and send
  half the paste as a message. A terminal without the mode sends the text bare,
  which still works rather than being mangled.
- A pasted block is taken whole out of the input buffer before any of its bytes
  are treated as keys.
- A paste is joined with what is typed after it and returned as one message,
  since the user pasted one thing. The paste comes first, being what was in the
  buffer before the typing began.
- A carriage return and newline together are one break, since that is what a
  terminal sends for a single newline. A trailing break belongs to the paste
  rather than asking for an empty line, and is dropped.
- A landed paste is reported and shown above the prompt, since a paste cannot
  fit on one row and a prompt that silently swallowed it would read as a lost
  paste. A large paste is cut to a fixed number of rows, and the overflow is
  reported rather than pushing the status bar off the screen.

### Special keys

- A key sequence is consumed whole before any of its bytes are read as keys. Left
  to be read one byte at a time, a three-byte arrow sent its opening escape to
  the line editor, and a lone escape ends the line: pressing an arrow closed the
  session.
- A sequence is left alone when it begins like a mouse report, since a report
  arriving in pieces would otherwise be taken as an unknown sequence and the rest
  of it read as keys.
- A sequence at the end of a read is drained before the end of the input is
  handled, since acting on it afterwards would leave the key with no effect.
- Keys read in one block are queued rather than held in a single slot, since a
  block can hold several and a second would overwrite the first.
- The up and down arrows walk the input history, and do nothing at all when there
  is none. They stop at each end rather than wrapping, since wrapping makes it
  impossible to tell which end one is at.
- Walking forward past the newest resumes the line that was being composed. That
  line is captured on leaving it for the history, so that a later walk knows
  where the user is rather than treating every key as a fresh start.
- The left and right arrows are consumed and reserved for input toggles. They do
  nothing yet, and are held so that the keys they will take are already spoken
  for rather than being taken by something else later.
- Ctrl-B is a prefix, as in tmux, and the key after it moves between panes: n
  to the next and p to the previous, wrapping at both ends. The main
  conversation and the delegate pane are the two panes, and the pane set holds
  which is shown, so that the keys and `/pane` cannot disagree.
- Any other key after the prefix is consumed and does nothing. It is not typed
  and does not end the line, since a mistyped binding that sent the message
  would be worse than one that did nothing.
- The prefix is held on the line editor rather than in the read loop, since the
  prefix and the key after it arrive in separate reads. The prefix is a plain
  control byte rather than an escape sequence, so it is read as a key and not
  queued with the sequences.

### Repainting

- The frame is repainted at most once every 40 milliseconds, and a request inside
  that interval is deferred rather than refused, so the state drawn at the end of
  it is the most recent one and nothing is lost.
- A terminal repaints far faster than the eye resolves. Drawing every token made
  a fast reply read as a flickering block rather than as text arriving, so the
  rate is bounded however fast the input comes.
- Writes to the terminal are serialised, so two requests for a repaint cannot
  interleave their output into the same row.

### Input sequences

- A mouse report arriving split across reads is held until it completes, because
  a link delivers it in pieces and returning the leading escape as a key ends the
  line and leaves the session. That is a crash rather than a misread, and it was
  the failure reported over ssh. A wheel scroll ends the session the same way, and
  was reported as a crash when scrolling a long way up.
- One notch in six is split, since a read of sixty-four bytes does not divide the
  twelve-byte report, and scrolling fast is what fills a queue that splits one.
  Scrolling slowly produces whole reports, which is why the fault appears only
  when a reader scrolls hard.
- The hold is bounded by a read that cannot block, with a short wait between
  attempts, and not by a read deadline on the file. A prefix at the end of a
  stream would otherwise wait for a byte that never comes, and Go hands the
  standard streams to a program through `os.NewFile`, which leaves the descriptor
  blocking and outside the runtime poller, so a deadline cannot be set on one. A
  hold that asked for a deadline took the failure for the end of the wait.
- Only a file can be read this way. A reader that is not a file reports the end of
  its input rather than blocking, so a hold over one returns.
- The descriptor is put back the way it was found. The terminal is given to the
  client as it was handed over, and the mode of a descriptor is not the client's to
  keep.
- `select(2)` is not used, since Go spells the descriptor set differently on every
  BSD it carries and it would take one spelling per system for the sake of a
  single bit. `poll(2)` is not used either, since on Darwin it ignores its
  timeout and waits for ever when nothing is ready, which is the one case the
  bound exists for.
- The wait is per read rather than for the whole hold, so a report arriving
  over several reads is assembled rather than cut short after the first gap.
- A prefix too short to recognise is still held. Requiring three bytes was what
  returned the opening escape as a key when a report arrived one byte at a time.
- Beyond the bracket the form decides. An arrow or another escape sequence is
  not held, or the key would be swallowed rather than a report. The up arrow is
  the shortest sequence that begins like a report and is not one.
- A prefix that does not complete within the wait is handed back as keys, so
  a lone escape interrupts and an arrow reaches the key handler, rather than
  either being reported as the end of the input.
- The six-byte mouse form is not held, since it is a fixed width and is taken
  whole or not at all.

### Saved sessions

- `/save [NAME]` writes the conversation to a file of its own and `/load NAME`
  puts it back in front of the model. The file is a SQLite database, so that it
  can be read afterwards by any tool rather than only by this client, which was
  what the extension was chosen for.
- The driver is `modernc.org/sqlite`. It is pure Go, which matters because
  `make crossbuild` sets `GOOS` without `CGO_ENABLED` and therefore leaves cgo
  off: a driver that compiles C would produce a binary that builds for every
  target and then fails at the first query.
- Files are held in `~/.openrouter-cli/sessions`, not beside the configuration
  file. The two hold different things: the configuration is written at setup and
  read thereafter, a session whenever the reader asks for one. Keeping them
  apart means removing a configuration file does not remove conversations.
- One file per save rather than one database of many, so that a file can be
  handed to someone else, queried on its own, or deleted without touching
  anything else.
- What is written is the turns, the model, the token counters, the reported
  usage, and the time. What is not written is as deliberate: not the
  credential, since the configuration file is the only source of the key, and
  not the session preferences, which belong to the configuration file.
- The free-model allowance travels with that usage. This file once recorded it
  as omitted, on the grounds that its type is unexported, and a test now proves
  the opposite: `freeAllowance` is an alias for an anonymous struct rather than
  a defined type, so the whole `Usage` value marshals and unmarshals across the
  package boundary. What a package outside `openrouter` cannot do is name the
  type, which is a different thing from being unable to carry it.
- The opening instructions are saved as the system turn they are, so that a
  resumed session behaves as the one that was saved did.
- The schema is at version 2, which added a column to the message table for the
  turns a tool call makes. A file written before the column existed is still
  read, and a file written by a later version is still refused.
- The added column is one nullable column carrying a JSON envelope, rather than
  a column per field. The format is a database so that a reader can open a
  saved conversation with any SQLite tool, and role and content staying
  readable as plain text is the whole reason for that. A column per field would
  have made the interesting part of a file unreadable without the client that
  wrote it.
- An ordinary turn stores no envelope at all rather than an empty one. A reader
  telling the two apart is what leaves room for a field added later without
  migrating the files already written.
- A turn is no longer comparable with `==`, since a turn can carry a slice of
  calls. The comparison in the round-trip test is on the JSON form, which
  compares the whole of a call rather than the fields a comparison happened to
  remember to write down.
- A save is refused in-cognito and inside a thread. Both record nothing, and a
  file holding the conversation would break that rather than record that it was
  broken. A refusal is the honest outcome; a flag saying recording was on would
  be the failure.
- `/save` under a name already taken asks before replacing, and only an explicit
  yes replaces. Anything else, an empty line included, keeps the earlier file and
  writes beside it under a name carrying the epoch, which cannot collide with
  itself and needs nothing remembered to stay unique.
- `/save` with no name is filed under the date and second, since a session saved
  twice in one minute is ordinary and one that quietly replaced the last would
  not be.
- `/load` is idle only, on the same terms as `/new`: a turn records its answer
  into the conversation it was asked in, so replacing that conversation
  underneath one would file the answer somewhere nobody asked for it.
- `/load` shows the turns it restored as well as restoring them. A load that
  replaced the conversation silently would leave the reader unable to tell a
  resumed conversation from one that had merely been going on.
- A model the reader chose wins over the one the file carries, which is the rule
  the configuration file's model is already held to.
- The ephemeral flag is not read from a file. It is what the reader asked for in
  this session, and a file that could turn recording off on load would defeat the
  mode from somewhere the reader never looked.
- A save may be taken while a model works, and holds what has been recorded. The
  turn in flight is not in it, since a turn is recorded only once its answer has
  arrived.
- A file that is some other database, or one written by a later version, is
  reported rather than read as an empty conversation.
- The driver cannot be built for DragonFly, so that platform was dropped rather
  than shipped with a command that reports it has no driver. Cgo was offered as
  a way out and was measured rather than assumed: `mattn/go-sqlite3` builds and
  runs on the host, but no cross C toolchain is installed here, so
  `CGO_ENABLED=1` fails inside `runtime/cgo` for every foreign target and a
  cross-build gate that cannot cross-build stops meaning anything. There is no
  newer pure Go release to bump to, the newest `modernc.org/libc` being the
  version already in the tree. `github.com/ncruces/go-sqlite3` does build
  everywhere without cgo, and was not adopted, since it costs a 12.4 MB binary
  against 2.9 MB to store two tables. An empty one loaded over a
  real one would look like a session the reader had.

### Tools

- The model is given tools, and a turn that calls one makes as many requests as
  the calls need. `read_file`, `write_file` and `list_dir` reach the filesystem;
  `git` runs git commands in the repository at the working directory, and the set
  permits commit, push and worktree alongside the read-only subcommands, since
  this repository needs them. Every other subcommand that writes is refused.
- The filesystem is contained by `os.Root`, opened on the working directory at
  startup. The standard library refuses `..`, a cleaned `..`, and a symlink
  pointing out of the tree, and a prefix check on a cleaned path is defeated by
  exactly that symlink. The containment is the open descriptor rather than a
  string, so nothing after it can be swapped for something else.
- The root does not move. There is no command that changes the working
  directory, so the tree the client was opened in is the tree it reaches for the
  whole session.
- A root that cannot be opened is reported rather than refused, and the session
  opens without tools, in the manner of a session with no API key. A reader
  whose working directory has gone away still gets a working chat client.
- A read is bounded at 1 MiB and a refusal names the limit. A model asking for a
  two gigabyte file is told so rather than being allowed to take the client
  down with it. Streaming the file instead was considered and rejected: the
  model asked for a file, not for a pipeline, and the pane has nowhere to put a
  file arriving a gigabyte at a time.
- The git tool is an allowlist, not a blocklist. Only the named subcommands run,
  and anything else is refused by name. A blocklist would be defeated by a
  subcommand nobody thought of, which is the whole reason the set is an
  allowlist.
- The git tool permits commit, push and worktree alongside the read-only
  subcommands, on the maintainer's instruction that this repository needs them.
  The remaining write subcommands, add and reset among them, are still refused,
  so the set grew rather than opening. A delegate cannot reach any of them, since
  a delegate is sent no tools, so the review and push happen in the parent
  conversation and a delegate never puts work on a remote.
- Arguments are given to git as an array, never through a shell, since a shell
  reads an argument as a command and these come from a model. A `--` precedes
  any pathspec, so an argument that looks like a flag cannot become one.
- A turn is offered tools only when its conversation is recording. That one
  condition covers `/cognito` and `/btw`, since both mark the conversation
  ephemeral, and a delegate never reaches the turn at all. A tool acts for the
  reader, and a mode that promises nothing is recorded cannot hand the model a
  hand that acts without leaving a trace.
- The model is told so when it has no tools. `/cognito` and `/btw` each say so in
  the notice they already print, since a reader who asks a model in such a mode
  to read a file and is told it cannot would conclude the client is broken.
- A turn is a loop of rounds, at most 64. A model with tools can ask for the
  same file for ever; the cap is what stops a loop that does not converge, and
  reaching it is reported in the pane rather than truncating the turn silently.
- The cap is set by what a turn has to be able to do rather than by what a loop
  is likely to do. Eight was enough to read a file and answer, and was not
  enough for the work the tools were added for, since a turn that builds, reads
  the error, edits and builds again spends a round on each step. A converging
  model does so in far fewer rounds, so the cap is not what stops a working
  turn; a reader can stop one at any time regardless.
- The turns a turn builds are committed to the conversation once, at the end,
  and only when the turn ended cleanly. Buffering them is what keeps a turn the
  reader stopped from leaving a tool result behind for the model to be told
  about as though it had asked for one and been answered.
- A call that failed still produces a turn. A request carrying no answer to a
  call is refused by most providers, and a turn producing nothing at all is a
  turn that stalls, which a reader experiences as a hang.
- The pane gets one plain line per call and never the result. A read of a large
  file would bury the conversation under the file, and the reader asked a
  question rather than for a file listing. The model is given the whole result;
  the reader is given what happened and how much of it there was. A failure is
  drawn as the reason on the same line, since a call that failed is not a call
  still running.
- The update a reader composes replaces the question rather than being added
  after it. After a round the request is the question and the answers to the
  calls, and restating those as prose would throw away what the model has
  already been told.
- The compaction check runs before every request of a turn and is given the
  turns about to be sent. A tool result is usually larger than the question
  that asked for it, and an estimate taken once at the start of the turn would
  not see one.
- The calls are reassembled by the transport and delivered as one event, rather
  than one event per delta. The transport is the one place that knows how the
  pieces were framed, and a caller reassembling is a caller that can get it
  wrong. A call whose name never arrived is not a call and is dropped.
- `Message.Content` stays a plain string even though a turn carrying calls has
  no content. A `null` decodes to the empty string, and an empty string is what
  every provider accepts there. A pointer would mean touching every caller in
  the tree to learn that a value is optional, which is the worse trade.
- `/tools` reports the tools, their argument schemas and the root. It is the
  only place a reader can find out what the client is willing to do.
- `/approve` reports or sets the mode, which is `ask`, `allow` or `refuse` and
  settles every call a file rule does not and no earlier answer has.
- The mode is a session preference and is not written to the configuration
  file. A file is somewhere a permission outlives the reading of it, and
  anything that would run every program without a question is not something to
  leave behind in a file a later run opens without being told what it holds. A
  permission the reader means to keep is a rule under `OPENROUTER_TOOLS`, which
  is asked for rather than switched on.
- A mode of allow does not widen the allowlist. A program outside it is refused
  by the tool before the mode is reached, so allowing says nothing about what
  may be proposed, only about what happens to what has been.
- A file rule settles a call whatever the mode says. A mode is a decision about
  what to ask rather than a revocation of what was permitted in advance, and a
  reader who permitted something in a file meant it.
- Changing the mode clears the answers remembered for the session, since an
  answer given while asking is an answer to one question. Without that, a
  reader moving from refusing to allowing would find every program they had
  once refused still refused.
- `/approve` is refused while a model is working, since a turn in flight is
  holding a question and changing the answer under it settles a call the reader
  never saw.
- The conversation is written on its own as well as by `/save`. It is written
  after every turn that ended cleanly, and again if the session sits idle for
  five minutes, which is the case a machine going down leaves behind.
- The autosave timer is inside the client rather than a cron entry or a shell
  loop. A scheduled job writing a session it has not read would keep writing
  the same conversation for ever, and a reader would come back to a directory of
  files that all say the same thing.
- The interval is five minutes. Seconds would write the same conversation over
  and over while a reader pauses to think, and a reader would come back to a
  directory full of copies of one exchange.
- An autosave is named for the user, the directory and the moment, with a link
  beside them pointing at the newest for that directory. The three parts are
  separated by the escape, so a reader can see where the name is split.
- A filename cannot carry a separator, a character outside ASCII or a space, so
  each is replaced rather than stripped. Stripping would give two directories
  differing only in a non-ASCII character one name, which is a save overwritten
  by another.
- A literal escape is doubled before anything else is replaced, so a path
  already carrying one cannot be read as a replacement. Non-ASCII is written as
  the escape and the code point in hex, since a rune and its bytes give
  different lengths and the bytes are what read back.
- A path longer than the name limit is shortened from the front and marked,
  since the tail of a path is the directory a reader recognises.
- An autosave is refused in cognito and in a thread, on the same grounds as
  `/save`. A mode that says nothing is recorded cannot then write the
  conversation to a disk.
- The newest link is replaced rather than written over, since a symlink written
  over another becomes a link to the link on some systems, and a link pointing
  at a file about to be replaced would resolve to nothing.
- `/permission` grants, removes and reports the programs a model may run in a
  directory without being asked. The rules are the same ones a reader can write
  by hand under `OPENROUTER_TOOLS`, and the two are read as one set.
- The rules are kept in a file beside the configuration rather than in it, since
  the configuration holds the credential and is treated as read-only outside
  setup. A reader editing rules should not have to open a file whose mode they
  have to get right, and a command rewriting one could damage the key in it. The
  file is written at 0600, since it names the directories a model may run
  programs in.
- The file is written whole and renamed over, so a reader never reads half a
  list. A crash during a write leaves the previous list rather than a truncated
  one.
- A rule for a directory is replaced rather than added to, since a rule is about
  a place and two rules for one place would have no way to say which applies. It
  is also removed whole rather than one program at a time, since a rule that
  permits three programs and refuses one has no way to say so.
- The rules are held by the session and replaced in place, so a permission
  granted now takes effect rather than at the next run.
- A rule named by `/permission add` covers the working directory unless a
  directory is named, and a directory is told apart from a program by being a
  path rather than by being the first argument. The first argument is usually a
  program, and reading it as a directory wrote a rule for a path that was never
  there and granted nothing.
- A rule is read against the directory a command would run in, not the one the
  session was opened in, so a rule written for a project covers a build run in
  a subdirectory of it. The listing is read against the same directory rather
  than the process one, since a rule marked as applying here and settling
  nothing is worse than no marking.
- A rule written for a directory outside the working directory settles nothing
  until a session is opened there, since the tools are contained to the
  directory the client was opened in. The rules are kept anyway, so a reader can
  set up a project before opening the client in it.
- A write that asks to overwrite creates the file where there is nothing yet.
  Every autosave names a file that is not there, and truncating a path with
  nothing at it fails, which refused `/save` under the same name as well.
- The model is given a shell, which runs a program in the working directory,
  such as `go build ./...`. It is the first tool that runs something on the
  host rather than reading the tree, and it is what made asking about a call
  necessary rather than optional.
- The shell runs a program from an allowlist. A program outside it is refused
  by name before anything is looked up, on the same reasoning as the git
  subcommands: a blocklist is defeated by every program nobody thought of. The
  list is a bound on what may be proposed rather than on what may happen,
  since a program on it is still asked about.
- Arguments go to the program as an array and never through a shell. A pipe, a
  redirect and a chain are features of a shell rather than of a program, so
  offering them would mean running one, and a shell reads an argument as a
  command. The schema says so, since a model that asks for a pipeline should
  learn it is not available rather than have it silently split into arguments.
- A command is contained to the working directory as the filesystem tools are,
  by the same cleaned path and resolved path comparison the git tool makes.
- A call is put to the reader before the program runs.
- The question is put on the turn goroutine and answered on the input one. A
  turn runs on its own goroutine so that the input loop keeps reading while a
  model works, and the input loop owns the terminal, so a question read from the
  turn would race the line editor for every key the reader pressed and a `y`
  meant as an answer would be taken as part of a message being composed.
- The turn hands the question over and waits for the answer. The answer is
  buffered, so the input goroutine never blocks on the turn having arrived to
  receive it, which it may not have when the keys are read.
- A question nobody answers is a refusal rather than a wait. The wait ends with
  the session as well as with the answer, since a question left open at exit
  would strand the turn on a channel nobody is left to post to. A turn blocked
  there is a turn a reader reads as a hang.
- The question is drawn as a box, closed on all four sides, above the prompt on
  the input block. It is the one thing on the screen that asks the reader to do
  something rather than telling them something, and a line of prose among other
  lines of prose is read as part of the conversation. A reader who has just been
  asked whether a program may run must not be able to mistake the question for
  output.
- The box is drawn in red, since it is the one thing on the screen asking the
  reader to decide something. The colour is on the corner and stops there, so
  the text inside the box is the colour of the rest of the frame: a border
  entirely in one colour reads as a line of text that happens to be long.
- A corner is taken whole rather than by its first byte. The box-drawing runes
  are several bytes each, and cutting one in half draws half a glyph followed by
  the rest of it as text.
- The frame is drawn in one pass rather than one per thing to colour. Each pass
  clears every row before writing it, so a second pass would wipe the first and
  the frame would flicker.
- The box is drawn with the box-drawing set rather than with pipes and hyphens,
  which read as text, and in the same font as the rules the frame is divided by.
- A question is folded inside the box rather than cut. The command it asks about
  is the one part that must not be hidden, and a cut question would hide it.
- A terminal too narrow for a box gets the bare question. A box two columns wide
  with the text cut to nothing inside it is worse than no box.
- The box is budgeted with the rest of the input block and only the rows the
  budget allowed are drawn, since a box pushed off the bottom of the screen
  takes the prompt with it and a reader with no prompt cannot answer.
- The rows of the box are padded here rather than by the terminal, so that a
  selection out of it is the text and not the text with a run of spaces after it.
- The question is drawn on the input block rather than into the pane. A question
  written into the pane scrolls back into the history the moment a reply
  arrives, which is about the moment a reader answering it would need to read it
  again.
- A shifted up arrow pages the pane back and a shifted down pages it forward.
  The modifier is read from the parameters of the sequence rather than from a
  table of the forms a terminal writes, and only the shift is read: an arrow
  held with another modifier is passed through as the plain arrow, since a page
  on the strength of a guess is worse than an arrow that does what it always
  did. A sequence carrying no parameter is not a shifted one, which matters for
  the final bytes that are also digits.
- The keys are handed to the session through a callback the editor calls for a
  key it has no use for. Without one assigned the key reaches a nil and nothing
  happens, and every test of the key and of the paging passes anyway since each
  is tested apart.
- A page is the height of the pane rather than a fixed count, since a fixed
  count is a page on one terminal and a third of one on another. It is a little
  under the height, so the row a reader was reading before is still on screen
  afterwards.
- A page needs history on the frame to move at all. The renderer clamps the
  offset to what the pane can show, so a page in an empty conversation clamps to
  zero and looks as though it did nothing.
- `/copy` writes the conversation to the clipboard through the terminal, by the
  OSC 52 sequence. An external command would be a subprocess, which on this
  client means the approval path and a question about every copy, and it does
  not work over a link where the clipboard belongs to the machine the reader is
  sitting at.
- `/copy` copies the last reply rather than the conversation. A reader reaching
  for a copy is usually carrying one answer somewhere, and the whole
  conversation is what they would select with the mouse when they meant all of
  it.
- A reply is a whole exchange rather than one message. A turn that called a tool
  is several: the call, the result, and then what the model said about it. Those
  are one answer, and a reader carrying it elsewhere wants the build and the
  error with the answer rather than the answer with the evidence missing, so the
  walk goes back to the question rather than to the last assistant turn alone.
  The question itself is left out, since the reader asked it and has it.
- The pane is not what is copied. It is folded to the width of the terminal, so
  what a reader selected out of it is not the reply as the model wrote it.
- A copy the terminal refused is not known, since OSC 52 is written and nothing
  comes back. The command says so once, since a copy that silently did nothing
  is worse than one that reports that it did not.
- The frame is repainted after a copy rather than before it, since the sequence
  leaves the terminal wherever the copy ended. Nothing else on screen
  says how to answer, and a question with no way to answer it is a question the
  reader can only escape.
- A question outranks the pane search and the model filter while it is open. A
  key reaching a search behind it would be a key answering nothing.
- The question names the resolved directory rather than the one that was asked
  for. A reader approving a command in a directory they were not shown would be
  approving something other than what runs.
- Three answers are offered: once, for the session, and no. Anything else is a
  refusal, including escape, since a reader who did not mean to answer must not
  approve a program by pressing a key they pressed for another reason.
- An answer is remembered for the session, and a refusal is remembered on the
  same terms. A model retrying a denied program would otherwise be able to wear
  the reader down by asking again, and the record is what the reader answered
  rather than what the model asked.
- A later grant overrides an earlier refusal. A reader who changed their mind
  has changed it, and the question must not be answered from the older answer.
- An answer given at the keyboard is held for the session and is not written to
  the configuration file. A grant given in passing is a statement about this
  session, and a file is somewhere it would outlive it.
- The configuration file carries the rules a reader wants to settle in advance,
  under `OPENROUTER_TOOLS`, each rule naming a directory and the programs
  permitted there. A rule covers the directories beneath it, so a rule written
  for a project settles a session running anywhere inside it rather than needing
  one entry per repository.
- The nearest enclosing rule decides rather than the union of all of them. A
  union would make a rule unable to say anything, since every rule beneath the
  working directory would grant everything any other grants and no rule could
  narrow what was granted above it. The cost is that a child directory cannot
  revoke what a parent granted, which is left rather than answered with a deny
  the file has no way to express.
- The rules travel with a missing credential. A file that cannot be read for its
  key has still been read, and a rule dropped with the key would make it look
  as though it had not been.
- A session with nothing to ask through is offered no shell at all, rather than
  one that runs without asking. The approver is passed in rather than reached
  for, since the tools package keeps execution away from the interface that drew
  it.
- A directory that is not a repository keeps the shell. It costs the git tool
  alone, and a reader outside a repository would otherwise find the model unable
  to build anything.
- The `Approval` status field is in the bar again. It was removed on the
  grounds that the client had no tools and executed nothing, and the shell is
  what makes it mean something. It names the mode, `ask`, `allow`, or `partial`,
  rather than the programs, since a bar is too narrow to carry a list and
  `/tools` reports what the bar cannot.

### Delegates

- `/delegate QUESTION` asks a question from a copy of the conversation while the
  main one stays open. The prompt remains live throughout, so a user can keep
  typing rather than waiting for the answer.
- A delegate is not a thread and not a spawned worker. It is a second request
  made from a copy, whose answer is shown in the pane.
- A delegate records nothing. Its text goes to the pane and nowhere else: no
  file, no history, and nothing kept once it finishes. That is what lets it
  coexist with the guarantee that /cognito makes, since display is not
  persistence.
- A delegate is sent no tools, since it records nothing and a tool acts on the
  host. The model is told so. A model that is not told asks for one anyway when
  the question calls for it, and a provider asked for a call it was not offered
  streams the markup as text: the reader sees a tool call written out in the
  reply rather than a reply at all. The reader is told the same thing, since a
  delegate that answers short of what the question needed is otherwise a reader
  wondering whether the model misunderstood it.
- The answer joins the pane as ordinary text rather than as a turn, so it is
  never replayed to the model as though the user had asked it.
- The partial answer is kept apart from the reply while it arrives, so a line in
  the pane is not mistaken for the answer to the last question. It replaces the
  previous partial rather than appending, so a growing answer does not fill the
  pane with copies of itself.
- A running delegate is tracked, so that leaving does not leave one writing to a
  frame nobody is drawing on.
- The delegate takes the model and the instructions of the conversation it
  branched from, so that it answers about the work in hand.

### Threads and retention

- `/btw` starts an ephemeral thread branched from the current conversation, and
  `/main` leaves it. A thread is not persisted and is not carried into a later
  session.
- A branch is a copy, not a shared slice. Sharing would let an exchange in one
  appear in the other, which defeats branching.
- The main conversation is held on the session throughout, so leaving a thread
  restores it exactly and nothing from the thread is carried over.
- A thread holds the turns it branched from, so the model has the context, but
  records nothing that is sent in it. Keeping those turns would leave the work
  reachable and would make the token counters report a total that is then
  discarded.
- Starting a second thread is refused, since nesting would leave the outer
  thread unreachable.
- `/cognito` records nothing, in memory as well as on disk.
- Retention is decided in one place, on the conversation, rather than at each
  call site that sends a message. Two places could disagree, and the
  disagreement would be a session that appeared to record nothing while
  keeping everything.
- A compaction summary is a system turn, and the instructions are another. A
  thread branched from a compacted conversation must not take the summary as
  its instructions, so a summary is recognised by its marker.
- The instructions are placed in front of the copied turns rather than seeded
  over them. A seed replaces the history, and an empty seed returns without
  doing anything at all, so seeding over a copy would discard it whenever the
  conversation carried no instructions.

### Model listing

- `/models` opens the catalogue and filters it as it is typed, rather than
  listing it and leaving the reader to scan. A catalogue is long enough that
  narrowing it by hand beats reading it.
- `/freemodels` is the same listing narrowed to the models that cost nothing,
  so the two are one code with a predicate rather than two listings.
- A model is free when both quoted prices are zero. A price that is absent or
  cannot be read is not treated as free, since the endpoint omits the field for
  a model it does not price and assuming otherwise would list a paid model as
  free.
- The filter is matched without regard to case, since a model identifier is typed
  in whatever case the user happens to use.
- A filter matching nothing says so rather than showing an empty pane.
- The listing is cut with the remainder reported rather than silently dropped.
- Tab completes the filter to a model identifier. Each Tab after it advances
  through what the filter matched, wrapping at the end, since a catalogue is
  easier to walk than to retype.
- The candidates are generated from the filter as it was typed rather than from
  the filter as it has been completed. A whole identifier matches only itself, so
  completing against the completed filter would make the cycle one Tab long.
- A cycle ends as soon as the filter is changed for any other reason, so a Tab
  after a keystroke begins a new set from what is now typed rather than
  resuming a set the reader has moved on from.
- The cycle covers the models the pane shows rather than the whole catalogue.
  A candidate that was never on the screen is one the reader cannot tell from
  another.
- The place in the cycle is stated in the heading rather than marked against a
  row. The filter already holds the identifier that is selected, so a marker
  would repeat it, while the heading says which of the set it is.
- Tab on a filter matching nothing changes nothing. The listing already reports
  that nothing matches, so there is nothing to complete and nothing to say.
- Tab reaches the filter rather than the line editor behind it, since the filter
  takes every key while it is open.
- Escape closes the listing and restores the pane. Enter chooses what the filter
  names, which saves typing an identifier that is already on screen.

### Pane search

- `/search` takes the pane over and filters it as the query is typed, in the
  manner of the model filter, since a conversation is long enough that narrowing
  it beats scrolling it.
- The search is folded the way the renderer folds it before it is matched. A
  match on a line the reader cannot see is not a match.
- The columns reported are columns of the screen rather than of the stored
  entry, so the column shown is the one the match fell at on screen.
- The comparison ignores case, since a word is recalled in whatever case it
  happens to be typed.
- The first match on a line is the one reported. The line is shown once, and
  marking every occurrence would need delimiters that would be copied out along
  with the text.
- The match is shown by the column it starts at, set in the margin, and by the
  query in the heading. Nothing is inverted or coloured, since a selection is
  taken out of the pane as plain text and an escape sequence drawn around the
  match would be copied along with it.
- The margin is dropped rather than pushing the text off the edge on a pane too
  narrow for it, since the line is the part the reader came for.
- The search records nothing. The listing is pane content, not a turn.
- Enter jumps to the newest match rather than closing, so that a reader who has
  found what they were after leaves with the escape they already know rather
  than learning a second key to leave with.
- The jump moves the offset rather than scrolling the terminal, so the view
  stays inside the pane and the wheel remains the one thing that otherwise moves
  it.
- The newest match is the one chosen, since the pane grows downward and the
  last match is the most recently written.
- A jump leaves a pane height below the match, so that a match on the last row
  is not the only thing on screen.
- The offset is restored when the search closes. The search moved the view only
  to show what it found, and a view left elsewhere would put the reader
  somewhere they did not choose.
- The pane as it stood is held aside rather than rebuilt from the
  conversation, since the conversation is folded at render time and so cannot
  be turned back into the lines that were on screen.
- A pane height is stated once, as a constant the renderer and the search both
  read. A jump that placed a match under the prompt would be worse than not
  jumping at all.

### In-cognito mode

- The mode is recorded by a marker file in the home directory rather than by a
  key in the configuration file, since the configuration file holds the
  credential and is treated as read-only outside setup.
- The marker records the process that wrote it, so a marker left behind by a
  crash is distinguishable from one held by a running session.
- A marker left by a crash is reported at startup rather than honoured
  silently. A user who believes nothing is being recorded, and is, would lose
  work with nothing said.
- A marker that cannot be read is an error rather than an absence. Treating it
  as absent would record work that was meant to be discarded.
- The marker is written at mode `0600` and holds a process identifier only. No
  conversation text is written anywhere.

### Stream detail

- `/verbose` reports the shape of each streamed turn, on or off. It is off
  unless asked for, since a reader who did not ask for it would read the extra
  line as output from the model.
- The mode is a display preference rather than a recording one. Nothing about
  the request, the reply, or the conversation changes because of it, so it
  coexists with `/cognito` and with a thread.
- The events are counted and the shape is summarised rather than one row per
  event. A reply commonly arrives as hundreds of deltas, so a line each would
  bury the reply under the detail that was meant to explain it.
- The summary is one line, however long the turn was. A detail that grows with
  the turn is the noise the summary exists to avoid.
- What is reported is the count of deltas, the character count, the finish
  reason, and whether the accounting arrived. The finish reason is the one
  thing about a stream a reader cannot infer from the text.
- A missing terminator is reported, and so is a turn that failed, since a
  summary that read like a complete one would misdescribe the exchange.
- Absent accounting is stated rather than left out. A reader watching the token
  counters would otherwise wait for figures that are never coming.
- The error text is not repeated. It is already shown in the pane, and a second
  copy would be noise.
- The report is gathered only when the mode is on, so a session without it does
  no work for it.
- The mode is read once per turn rather than per event. The request goroutine
  owns the turn, and a mode changed midway would otherwise show half a summary
  under a reply that did not produce it.
- Every stream event carries a kind, naming what it held, since a caller cannot
  otherwise tell a delta from the accounting that arrives on the same chunk.
- The kind labels the event rather than replacing it. The fields the interface
  already relied on are untouched, so the label adds no coupling to the fields
  it describes.
- The count is in deltas and characters rather than tokens, since an exact
  token count needs the model tokenizer, and the accounting carries the token
  figures where the endpoint reported them.

### Wrapping

- A reply longer than the pane is folded rather than cut. Cutting loses
  whatever fell past the edge, which for prose is most of a paragraph.
- A word wider than the pane is moved whole onto a line of its own, since a URL
  or a path split across two rows is neither readable nor copyable.
- A fenced code block is not folded. Reflowing code changes what it means, so a
  block keeps its own line breaks and a line inside one that is overlong is cut
  with an ellipsis, which shows the reader it continues rather than letting it
  wrap and push the frame down.
- A fence marker is recognised anywhere on a line, not only at the start,
  because a model writes prose and an opening marker on the same line often
  enough that requiring it to lead would leave the block unfenced and its code
  folded. Text before the marker is folded as prose, since it is prose.
- The info string after an opening marker belongs to the fence, so it is taken
  with it rather than becoming the first line inside the block.
- Indentation in front of a marker is layout rather than content, and is
  trimmed, so that laying a block out does not add a blank row.
- A reply is stored whole and folded at render time, rather than being split
  into lines as it arrives. Splitting first leaves each fence marker on its own,
  so the renderer sees no block and folds code that must not be folded.
- A reply that is still arriving is folded the same way a finished one is, even
  though it is not yet known to contain a fence.

### Input box

- The composed line is drawn across three quarters of the terminal rather than
  all of it. A line reaching the edge carries the eye off the end of the screen,
  and the reader has to find the end of it again to see what they last typed. A
  prompt spanning the whole width also gives the conversation above it no visible
  edge, so the two run into each other.
- A terminal too narrow for the fraction to mean anything keeps the whole width,
  since three quarters of nothing shows nothing of what is being typed.
- The rest of the frame is still drawn across the whole terminal. The width is
  the composed line's alone and the conversation is not narrowed by it.
- A long line keeps its tail rather than being cut at the head: the reader is
  looking at the end of what they are writing, and that is the only part they
  cannot read from memory.
- The caret sits one column past the last character, which is where the next one
  is written, and is bounded by the prompt width rather than the terminal.

- The prompt is separated from the conversation by a blank row, a rule, and
  another blank row. Without them the prompt sits directly under the last line of
  a reply and the two are read as one block.
- The rule is a box-drawing character rather than a run of dashes, since dashes
  read as text and a rule reads as a rule.
- The blank row below the rule matters as much as the one above it. A rule
  touching the prompt reads as a border of the prompt rather than a division of
  the screen.
- The status bar sits in a header at the top: the title, a rule, then the bar.
  It is drawn outside the scrolled slice, so it stays put while the reader scrolls
  back through earlier output. At the foot it scrolled away at exactly the moment
  the figures in it were wanted.
- The header costs three rows, so the pane is given what is left. A frame taller
  than the terminal pushes rows off the screen, so the pane shrinks rather than
  the frame growing.
- The scroll is clamped so that a full pane of history remains, or all of it when
  there is less than that. Clamping to the pane height alone stops short of the
  oldest lines, and clamping to the whole history leaves a single line at the top
  of an empty pane.
- A pane shorter than its history is padded with blank rows, so a blank row at
  the foot of the pane is expected rather than a gap. The history itself must be
  contiguous from the oldest line.
- The row below the prompt is added only when the frame has not already filled
  the height, so a short terminal is not pushed one row over.
- A test that finds a region of the frame must do so by content rather than by
  an offset, since the header grows and the division comes and goes with the
  terminal height.
- On a terminal too short to hold the division it is dropped rather than drawn,
  for the same reason.
- A test that checks the frame fits must count columns rather than bytes, since
  the rule is a multibyte character and a byte count reports it as three times
  too wide.
- A test that finds the status bar must do so by content rather than by
  position, since the division now sits between it and the prompt.

### Hint row

- A row names the keys that do something in the state the interface is in. It
  is the only place the client says what a key is for, so a key the reader does
  not know about is one they will conclude does nothing.
- Only a key that acts is named. A row that promised a key which did nothing
  would be worse than no row, since a reader would press it and conclude the
  client had hung. The arrows are named only once there is history to recall,
  escape only while a request is in flight, and the wheel only while reporting
  is on, which is off unless the reader asked for it.
- The row sits above the prompt, inside the input block, rather than below it
  as a footer. The caret is placed on the last row, so a footer would move the
  caret off the prompt, and the frame carries one budget of rows rather than a
  second index into it. An earlier design put the row below the prompt and was
  not carried out.
- Against the prompt it reads as a caption for the input line, which is what
  it describes.
- The row is budgeted with the rest of the input block, after the paste, so
  that a landed paste is still reported. A paste the reader cannot see reads as
  a lost paste, where a missing hint costs nothing beyond its own row.
- The row is dropped whole before the prompt is. A session with no way to type
  a next message is not one the reader can carry on in.
- Entries are dropped whole rather than cut when the row is too narrow. Half a
  phrase names a key and not what the key does, which is the one thing the row
  exists to say, and the status bar avoids the same thing by dropping fields
  whole.
- The row is budgeted from the rendered line rather than from the entries, so a
  terminal too narrow for even one entry does not give up a pane row for a row
  that is never drawn. A test comparing the frame with and without hints at
  every size is what caught it.
- The state is carried on the session under its lock rather than read out of
  the line editor at paint time, since the editor belongs to the input goroutine
  and the row is also drawn from the spinner goroutine. The busy bit is read
  from the frame, which the request loop writes under the same lock, so the two
  cannot disagree.
- The overlay is derived at paint time from flags that are already guarded,
  rather than recorded at each site that opens or closes one, since six call
  sites would be six chances to leave one of them out.
- Tab is named in the model filter, since the filter completes on it, and is not
  named in the pane search, which takes characters only. A key is named where it
  acts and nowhere else, and the search has no completion to name.
- `PROVISIONAL` The wording of the entries, and the two-space separator between
  them, are the implementer's. Nothing in the record specifies them.

### Terminal bell

- The bell is a byte in the output stream rather than a terminal feature, so it
  needs no terminal support beyond an output that is a terminal.
- It is rung when a reply has finished arriving rather than when the request was
  sent, since the point of it is to say the answer is ready.
- It is off unless asked for. A user who did not ask for a bell would find one
  startling, so the default is silence.
- The preference is read from `OPENROUTER_BELL` in the configuration file and
  can be changed at runtime with `/bell`, since a user reaching for a bell
  mid-session wants it now and would not want to restart to get it.
- It is written only when the output is a terminal. A redirected run has no
  terminal to ring and the byte would be noise in the capture.
- The preference survives a file that carries no credential, since a file
  holding a preference is not unread for want of a key.
- Switching the mode off discards the exchanges recorded before it went on, and
  reports how many. Restoring recording while keeping that history would leave
  the model still carrying the work, which is the thing the mode prevents.
- The opening instructions survive being discarded, since losing them would
  change how the model behaves without anything saying so. A compaction summary
  does not, since it is recorded work rather than an instruction.
- The token counters go with the discarded work, since they counted it.
- The count reported is exchanges rather than turns, since an exchange is stored
  as two turns and counting turns would report twice the work done.
- Nothing else in the client changes because of the bell. It is a preference,
  not a mode.

### Progress

- A twiddle is shown while work is in progress: a request, a compaction, or a
  connection test. It uses braille figures rather than dots, at one column each,
  so that the frame does not jitter as it turns.
- The first frame is drawn before the goroutine starts, so something is on
  screen the moment work begins rather than after the first interval.
- The twiddle runs on its own goroutine, so the frame is guarded by a mutex
  shared with the request loop. The lock is released before the terminal is
  written, since holding it across the write would serialise the twiddle
  against the work it is reporting on.
- Stopping waits for the goroutine to finish. Returning early would let a late
  frame paint over the interface after the work had ended.
- Starting an already-running twiddle does nothing, so a caller need not track
  whether one is running.
- The twiddle row is the one part of the frame drawn in colour, and the colour
  scrolls through the six primaries while work is in progress. Every other row
  is prose: a reply is written by a model and copied out by a reader, and a
  sequence written into prose is copied out with it. The primaries are used
  since they are the colours every terminal can be relied on to show.
- The colour is interpolated between the primaries rather than stepped between
  them. Six colours turning is not a scroll, and a hard change is the thing a
  scroll is wanted in place of.
- A circuit takes four seconds, long enough that the movement reads as movement
  rather than as a flicker. The ramp is divided by the repaint interval rather
  than counted in steps of its own, so the colour moves at the rate the figure
  does whatever the interval is later changed to.
- A component reaching zero on the way down is floored rather than drawn at the
  bottom of the cube. A blue at zero is the black that red at zero would be, and
  a dark background is the common case.
- The colour is written by the screen and not by the renderer. Every row leaves
  the renderer as plain text with the bytes a terminal would act on removed, and
  a sequence inserted before that would be stripped along with the ones a model
  sent. The screen is the one place that writes bytes rather than text.
- The sequence covers the whole row, the twiddle and the word beside it alike,
  and is reset before the next row is drawn. The row is an indicator rather than
  prose: it is written by the client rather than by a model, it is on screen only
  while work is in progress, and the next frame replaces it. A selection of the
  pane carries the characters rather than the colour either way, since the
  sequence is written by the screen and is not part of the row.
- The twiddle row is found in the finished frame rather than tracked through the
  trims the pane applies. The pane drops lines from the front and adds blanks at
  the back, so an index moved by hand through both is a second thing to keep
  correct, while matching the line once against a frame that has stopped moving
  is one comparison. A row that has scrolled off reports none, since a tint
  naming a row that has taken its place would colour a line of history.
- The tint carries the twiddle as text rather than as a count of columns. The
  figures are braille and several bytes each, so a count of bytes cuts one in
  half and the terminal draws half a glyph followed by the rest of it as text.
- The tint is cleared with the twiddle. A tint left behind with no twiddle would
  colour whichever row it was pointed at.
- The row carries how long the work has been running, since that is the question
  a reader watching a slow turn is asking. The figure follows the word rather than
  leading it, so the left of the row still reads as the twiddle and the word.
- The figure is seconds until a minute and minutes and seconds after. It is not
  padded, so it does change width as it grows, which is safe only because it is
  appended to the row rather than leading it: the twiddle and the word sit at
  the same column whatever the figure says.
- The figure is cleared with the twiddle, and a clock that has been set back
  reads zero rather than counting backwards, since a negative figure is a thing
  that cannot have happened.
- The twiddle leads the line rather than trailing it, since a trailing one
  would shift the text sideways on every step.

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

- The module carries one runtime dependency, `modernc.org/sqlite`, for saved
  sessions. It is the one departure from the no-dependency rule the rest of the
  client is built on, and it was taken so that a saved file could be opened and
  read by any SQLite tool rather than only by this client.
- The tools added nothing to that count. The filesystem is reached through
  `os.Root` and git is run as a subprocess, both of which are the standard
  library and a program already on the reader's machine. A dependency here
  would have been a third thing to audit for a sandbox the standard library
  already provides.
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

### Frame bounds

- The frame is never taller than the terminal, and no row is ever wider than
  it. A frame that runs past the bottom pushes the prompt off the screen and
  leaves the reader no way to type a next message, and a row past the right
  edge wraps and pushes everything below it down.
- The frame is cut to the height rather than padded up to a minimum. Writing
  rows the terminal does not have is what causes the overflow, so the answer
  is fewer rows rather than a frame insisting on its own.
- The frame is padded with blanks where the budget over-reserves, so that it is
  the height of the terminal on every terminal. A frame left short draws the
  bottom of the screen in whatever the terminal had there, which after a resize
  is not blank.
- The frame is divided by rules: one on the first row, one under the title, one
  under the status bar, one above the prompt, and one at the very bottom. Each
  is separated from what it borders by a blank, except the title, which sits
  against the rule above it so that the rule reads as the edge of the frame
  rather than as an underline of the title.
- The rule on the first row is kept rather than spent on decoration. It marks
  where the frame begins, so a pane scrolled back is visibly still inside a
  window, and without it the oldest line of a conversation runs into the edge of
  the terminal with nothing saying the conversation continues above it.
- The prompt is found by content rather than by being the last row with anything
  on it, since the closing rule is drawn below it. The caret therefore sits on
  a blank rather than against a rule.
- The header yields before the prompt does, and gives up its rows in the order a
  reader loses least by: the rules first, since a rule is decoration and a
  half-drawn one is not a rule, then the blanks, then the title, and the status
  bar is held to the end since it carries the figures. A kept row still has
  everything below it.
- A pasted block takes only what is left once the prompt has been accounted
  for, and is cut and reported rather than allowed to cost the prompt.
- Width is counted in columns rather than in bytes. The rule is drawn from a
  box-drawing character, which is three bytes and one column, and a rule a
  byte count reports as three times too wide wraps and breaks the layout.
- A row narrower than its own markers shows the body rather than the marker.
  On a terminal of one or two columns the marker is all that could otherwise
  be shown, which tells a reader nothing about what they typed.
- A body that does not fit keeps its tail, since the marker leads the row and
  the end of the line is the part still being composed.
- An empty frame leaves the caret where it is. There is no last row to place
  it against, and guessing would put it somewhere the reader cannot see it.
- Both bounds are checked together across every width, height, scroll offset
  and paste length, rather than one axis at a time. The rows written are
  bounded on both axes at once, so a check of one alone would pass a frame
  that is correct in width and too tall.
- The terminal size is re-read on every repaint rather than being cached from
  startup. A window resized while the client runs was drawn to the size it had
  when it opened, which left the prompt below the bottom of the screen.
- A size that cannot be read keeps the last one rather than reporting nothing.
  A frame drawn to a stale size is better than no frame at all.
- A size that reads as zero is not adopted, since a frame drawn to it is empty.
- The resize is tested against a pseudo-terminal rather than a regular file. A
  file reports a fixed size, so a test against one passes whether the value is
  cached or re-read, which is the distinction the test exists to make.

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
- `make crossbuild` compiles every supported target. The terminal layer names
  ioctl requests that differ between the BSD family and System V, so a change
  there breaks a platform that is not the one being developed on, and nothing
  else would notice. Continuous integration runs it.
- `make crossbuild` runs the vet pass as well as the build, because a build
  does not typecheck test files. A test naming an ioctl that one platform does
  not carry compiles everywhere and fails only where the constant is missing,
  which is how a FreeBSD-only pty helper reached four other BSD platforms
  without the build noticing.
- The resize tests open a real pseudo-terminal, since a regular file reports a
  fixed size and would pass whether the size were cached or re-read. The ioctl
  numbers are spelled for FreeBSD only, and the other platforms skip rather
  than fail. Nothing in the client is conditioned on the helper.
- The termios ioctl names live in build-tagged files: the BSD spelling carries a
  trailing A where System V omits it. The window-size query is spelled the same
  everywhere, so only the termios constants are split.
- A platform matching neither family compiles but cannot drive the terminal, and
  the build is made to say so rather than failing at run time.
- The Go toolchain version is not repeated in the `Makefile`. The directive in
  `go.mod` is the source of it, and a second copy would drift.
- Build output is written to `build/`, which is ignored, so that the binary
  does not sit beside the directories a port expects.

## Open decisions

These are unsettled. Each is listed so that it is not mistaken for a decision.

- Which models can call tools. A model that cannot is not told, so it simply
  answers in prose and the reader concludes the client is broken. The catalogue
  is already fetched and cached per model, so a capability field fits it, and
  nothing reads one yet.
- Whether a saved session whose history contains tool turns may be resumed
  against a model that cannot call tools. It is resumed as it stands, since the
  turns are ordinary history and the model is not asked to have called anything.
- The model instruction search under Interface is built or the section is
  corrected.
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
- Whether the model instruction search under Interface is built or the section
  is corrected. The section describes reading `AGENTS.md`, `OPENROUTER.md`,
  `RULES.md`, or `SHARED.md` from the working directory before a request, and
  no source file mentions any of those names. The only path that exists is
  `--bootstrap FILE`. Until this is answered there is nothing for an `/update`
  command to re-read.

## Working notes

- The local git identity is unset. The clone did not carry the maintainer's
  identity, and a commit will not be made until it is set. It is set with
  `--local` so the global configuration is untouched.
- The maintainer is `Glen Barber`, reached at `gjb@FreeBSD.org`. That address is
  the one recorded in the `MAINTAINER` field of the `Makefile`, which is where a
  port expects to find it. The git identity is separate from it and is not
  changed with it.
- The first draft of the API client, written before the design discussion, was
  discarded at the maintainer's instruction and is not in the repository.
- `AGENTS.md` is tracked in this repository. The file at `~/agents.md` is a
  separate, personal set of notes, and the two are not kept in sync.
- A C compiler and `gmake` are confirmed present on the host, along with `protoc`
  29.6, installed as the FreeBSD `protobuf` package. The package database does
  not list the binary path, but the binary is present and reports its version, so
  the earlier failure to find it reflected an incomplete install at the time.
