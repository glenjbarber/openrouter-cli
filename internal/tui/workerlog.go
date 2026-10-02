package tui

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/glenjbarber/openrouter-cli/internal/tools"
)

// workerDirName is the directory under the saved sessions directory that the
// logs of spawned workers are kept in.
//
// It sits beside the saved conversations rather than among them, since the two
// hold different things: a saved conversation is a database a reader can load
// back, and a worker log is a record of what a background worker did. Nothing
// here is loadable, which is the reason it is Markdown and not a database.
const workerDirName = "worker"

// workerLog is the record one spawned worker leaves behind.
//
// The log is written as the calls are made rather than at the end, so a worker
// that reaches the round cap or is cut short still leaves what it did. A log
// written at the end would record nothing for exactly the turns that went
// wrong, which are the ones worth reading.
//
// Each field is held in memory and appended to the file on every call, so the
// file and what the pane shows cannot disagree. It is guarded by the mutex of
// the Session that owns it.
type workerLog struct {
	// path is the file the log is written to.
	path string
	// f is the open file, held for the life of the worker so that a log is
	// written whole rather than by repeated truncation.
	f *os.File
	// err holds the first failure, which is reported once rather than once
	// per call.
	err error
}

// newWorkerLog opens the log for a worker started at the given moment.
//
// The name carries the second rather than the minute, since two workers started
// inside one minute is ordinary and one that overwrote the other would lose the
// record of work that had run.
func newWorkerLog(started time.Time) (*workerLog, error) {
	dir, err := workerDir()
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("creating %s: %w", dir, err)
	}
	path := filepath.Join(dir, started.Format("20060102-150405")+".md")
	// The mode is 0600, the same as the configuration file and the rules
	// beside it, since a worker log names the files a model wrote in a
	// project and that is not something another account has any business
	// reading.
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return nil, fmt.Errorf("creating %s: %w", path, err)
	}
	return &workerLog{path: path, f: f}, nil
}

// workerDir returns the directory the worker logs are kept in.
func workerDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("finding the home directory: %w", err)
	}
	return filepath.Join(home, ".openrouter-cli", "sessions", workerDirName), nil
}

// header writes the opening lines of the log, naming the question the worker
// was given and the model that took it.
func (w *workerLog) header(task, model string) {
	w.write("worker: " + task + "\n")
	w.write("model: " + model + "\n")
	w.write("started: " + time.Now().Format(time.RFC3339) + "\n\n")
}

// call records one tool call and what came of it.
//
// The argument is recorded as the model wrote it rather than cut to the width
// the pane shows it at, since the log is read after the fact and an argument
// cut to forty-eight columns is not enough to see what ran.
func (w *workerLog) call(r tools.Result) {
	name := r.Call.Function.Name
	args := strings.TrimSpace(r.Call.Function.Arguments)
	line := fmt.Sprintf("- %s %s", name, args)
	if r.Err != nil {
		line += "\n  failed: " + r.Err.Error()
	} else {
		line += fmt.Sprintf("\n  ok: %s", toolSize(len(r.Text)))
	}
	w.write(line + "\n")
}

// answer records the reply the worker finished with, so the log carries what
// it concluded as well as what it did.
func (w *workerLog) answer(text string) {
	w.write("\n## answer\n\n" + strings.TrimRight(text, "\n") + "\n")
}

// write appends a line to the log, keeping the first failure.
//
// The file is the record of a worker acting on the host, so a failure to
// write it is worth carrying to the pane rather than losing silently. Only the
// first is kept, since one error means every later write is into the same
// trouble and a page of repeats helps nobody.
func (w *workerLog) write(s string) {
	if w == nil || w.f == nil || w.err != nil {
		return
	}
	if _, err := w.f.WriteString(s); err != nil {
		w.err = err
	}
}

// close finishes the log and reports the path it is at.
//
// It is closed rather than left open, since a log held open by a finished
// worker is a descriptor that outlives the work and keeps a file appearing
// incomplete to anything reading it.
func (w *workerLog) close() string {
	if w == nil || w.f == nil {
		return ""
	}
	w.write("\n")
	w.err = errors.Join(w.err, w.f.Close())
	w.f = nil
	return w.path
}

// problem reports why the log could not be written, or the empty string where
// it was written whole.
func (w *workerLog) problem() error {
	if w == nil {
		return nil
	}
	return w.err
}
