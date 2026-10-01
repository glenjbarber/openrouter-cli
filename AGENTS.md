# AGENTS.md

Editing notes and settled decisions for this repository. This file is the record
of why the project is shaped as it is. It is read before any change is made.

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
  checksum file rather than a silently unpinned build. It is currently absent
  because the module has no external dependencies.
- The `go` directive is a minor version, `go 1.26`, not a patch pin. A patch pin
  is unusual and needlessly excludes users on a lower patch release.
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
  document.
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
  told anything.
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
  instruction. `Approval` was removed because it implies a permission system
  for tool calls, and the client has no tools and executes nothing, so there
  would be nothing to approve. A dash that can never be filled is noise.
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
- The status bar shows Provider, Model, Reasoning, Branch, Status, Approval
  Method, Context used, Tokens used in and out, and hostname.
- Commands and configuration options are completed with the Tab key.
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
- `/new` clears the conversation and keeps the bootstrap document, since losing
  it would silently change how the model behaves.
- A multi-line reply occupies one pane row per line, since a reply carrying
  embedded newlines would otherwise push the frame down the screen.
- Each pane row is cleared before it is written. Without that, a repaint shorter
  than the frame before it leaves the tail of the longer one visible, so a
  reply appears twice.

### Getting started

- The reply pane carries a hint while it is empty, naming whatever is missing: an
  absent key, an unselected model, or simply that a message can be typed. The
  hint is removed once a conversation has started, since it is then in the way.
- Without the hint a first run shows an empty pane and a prompt, which gives no
  indication that plain text is the message and that a model must be selected
  first.

### Worktrees

- Feature and bug work is done in a git worktree under
  `~/openrouter-cli-worktrees`, one directory per piece of work, named for it.
  Work is merged into `main` after it is implemented and tested.
- The main checkout is left on `main` and is not edited directly for a feature.
  A worktree keeps an unfinished change from sitting on `main`, where it would
  be pushed by anything that pushes the branch.
- The worktree is created from `main` and its branch is merged back with
  `--no-ff`, so that the work is visible as a unit rather than as a row of
  commits indistinguishable from the rest.
- A worktree is removed after the merge, and the branch is deleted with it. A
  stale worktree holds a whole checkout of disk that nothing refers to.

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
  the failure reported over ssh.
- A held prefix is waited for with a read deadline, since a prefix at the end of
  a stream would otherwise wait for a byte that never comes. Only a file can be
  given a deadline; a reader that is not a file reports the end of its input
  rather than blocking, so a hold over one returns.
- The deadline is per read rather than for the whole hold, so a report arriving
  over several reads is assembled rather than cut short after the first gap.
- A prefix too short to recognise is still held. Requiring three bytes was what
  returned the opening escape as a key when a report arrived one byte at a time.
- Beyond the bracket the form decides. An arrow or another escape sequence is
  not held, or the key would be swallowed rather than a report. The up arrow is
  the shortest sequence that begins like a report and is not one.
- A prefix that does not complete within the deadline is handed back as keys, so
  a lone escape interrupts and an arrow reaches the key handler, rather than
  either being reported as the end of the input.
- The six-byte mouse form is not held, since it is a fixed width and is taken
  whole or not at all.

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
- The header yields before the prompt does. The title is the first row
  dropped, since the pane is what a reader is reading and the status bar
  carries the figures worth keeping on a short terminal.
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
