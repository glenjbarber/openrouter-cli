package tui

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
)

// processAlive reports whether a process identifier is still running.
//
// It is used only to tell a marker left by a crash from one held by a running
// session. An error is treated as not alive, since the marker is then a stale
// one and the conservative reading is the right one.
func processAlive(pid int) bool {
	proc, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	return proc.Signal(syscall.Signal(0)) == nil
}

// cognitoMarker names the file recording that in-cognito mode is in force.
//
// A marker file is used rather than a flag in the configuration file, since
// the configuration file is the credential and is treated as read-only outside
// setup. Writing a separate small file keeps the credential untouched.
const cognitoMarker = ".openrouter-cli-cognito"

// cognitoState reports whether in-cognito mode is in force and why.
//
// The reason matters at startup: a mode left on by a crash must be reported,
// since a user who believes they are not in cognito mode and are would send
// work that is then discarded.
type cognitoState struct {
	on    bool
	crash bool
}

// cognitoPath returns the marker path in the home directory.
func cognitoPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("locating the home directory: %w", err)
	}
	return filepath.Join(home, cognitoMarker), nil
}

// readCognito reports whether the marker is present.
//
// A marker left behind by a crash is reported rather than silently honoured, so
// that a session recorded as ephemeral is never lost without saying so.
func readCognito() (cognitoState, error) {
	path, err := cognitoPath()
	if err != nil {
		return cognitoState{}, err
	}

	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return cognitoState{}, nil
	}
	if err != nil {
		// A marker that cannot be read is an error rather than an absence,
		// since treating it as absent would record work that was meant to be
		// discarded.
		return cognitoState{}, fmt.Errorf("reading %s: %w", path, err)
	}

	st := cognitoState{on: true}
	// A marker written by a running session carries the process that wrote
	// it. A marker naming a process that is gone is one left by a crash.
	if pid := strings.TrimSpace(string(data)); pid != "" {
		if n, err := strconv.Atoi(pid); err == nil && n > 0 {
			if processAlive(n) {
				return st, nil
			}
			st.crash = true
		}
	}
	return st, nil
}

// setCognito writes or removes the marker.
func setCognito(on bool) error {
	path, err := cognitoPath()
	if err != nil {
		return err
	}

	if !on {
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("removing %s: %w", path, err)
		}
		return nil
	}

	// The process identifier is recorded so that a marker left by a crash is
	// distinguishable from one held by a running session.
	body := []byte(strconv.Itoa(os.Getpid()))
	if err := os.WriteFile(path, body, 0o600); err != nil {
		return fmt.Errorf("writing %s: %w", path, err)
	}
	return nil
}

// toggleCognito turns recording on or off.
//
// The marker is written on the way in and removed on the way out, so that a
// session left in the mode by a crash is still recognisable afterwards. Nothing
// else is written: the mode changes what is recorded, not what is displayed.
func (s *Session) toggleCognito() {
	if s.cognito {
		if err := setCognito(false); err != nil {
			s.frame.Reply = append(s.frame.Reply, "(error) "+err.Error())
			return
		}
		s.cognito = false
		// The main conversation is restored rather than the current one, since
		// a thread is unrecorded either way and switching the mode off inside
		// one would otherwise be refused.
		s.conv = s.mainConv
		s.conv.clearEphemeral()
		// Whatever was recorded before the mode went on is dropped with it.
		// Restoring recording while keeping that history would leave the model
		// still carrying it, which is what the mode was meant to prevent.
		dropped := s.conv.DiscardRecorded()
		s.frame.Reply = append(s.frame.Reply,
			fmt.Sprintf("cognito mode off: recording again, %s discarded",
				dropped))
		return
	}

	if err := setCognito(true); err != nil {
		s.frame.Reply = append(s.frame.Reply, "(error) "+err.Error())
		return
	}
	s.cognito = true
	// Both conversations are marked, since the mode is a statement about the
	// session rather than about whichever conversation happens to be in force.
	// Marking only the one in force would leave the main conversation
	// recording, so a thread left open at the time would return the reader to a
	// conversation that keeps what the mode promised to discard.
	if s.mainConv != nil {
		s.mainConv.setEphemeral()
	}
	s.conv.setEphemeral()
	// A thread is already unrecorded, so saying so avoids the impression that
	// the mode changed anything while it is in one.
	if s.thread != nil {
		s.frame.Reply = append(s.frame.Reply,
			"cognito mode on: nothing is recorded. The thread already records nothing.")
		return
	}
	s.frame.Reply = append(s.frame.Reply,
		"cognito mode on: nothing is recorded from now on")
}

// AdoptCognito reports whether a session should start in the mode.
//
// A marker left by a crash is reported at startup rather than honoured without
// comment, since a user who believes nothing is being recorded, and is, would
// lose work with nothing said.
func (s *Session) AdoptCognito() error {
	st, err := readCognito()
	if err != nil {
		return err
	}
	if !st.on {
		return nil
	}

	s.cognito = true
	// The conversation is marked in the same step as the flag, since the mode is
	// reported as in force from the next note onwards. A session that adopted
	// the marker and went on recording would keep work the reader was told was
	// being discarded.
	s.conv.setEphemeral()
	if s.mainConv != nil && s.mainConv != s.conv {
		s.mainConv.setEphemeral()
	}
	if st.crash {
		s.Note("cognito mode was left on by a session that did not exit cleanly. " +
			"It is on, and nothing will be recorded.")
		return nil
	}
	s.Note("cognito mode is on from another session: nothing will be recorded")
	return nil
}
