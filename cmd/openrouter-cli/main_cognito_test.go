package main

import (
	"os"
	"strings"
	"testing"
)

// The in-cognito marker was written by /cognito and never read back, so a
// reader who turned the mode on and left recorded every exchange of the next
// session. The function that adopts the marker was correct and complete, and
// nothing called it, which is the whole of the defect: a function with no call
// site behaves exactly as though it were absent.
//
// This is a wiring guard rather than a behavioural one, since driving the
// interface needs a terminal and the pty helper that opens one is FreeBSD only.
// It reads the source, so what it checks is that the call is present at all,
// which is the part that was missing.
func TestTheCognitoMarkerIsAdoptedAtStartup(t *testing.T) {
	src, err := os.ReadFile("main.go")
	if err != nil {
		t.Fatalf("reading main.go: %v", err)
	}
	text := string(src)

	if strings.Contains(text, "func (s *Session) AdoptCognito") {
		t.Fatal("main.go defines AdoptCognito, which belongs to the session package")
	}

	var adopted bool
	for i, line := range strings.Split(text, "\n") {
		if !strings.Contains(line, "AdoptCognito()") {
			continue
		}
		adopted = true
		// It has to be before the session runs, or a first exchange is
		// recorded before the mode is noticed.
		if !strings.Contains(line, "if err := session.AdoptCognito()") {
			t.Errorf("main.go:%d adopts the marker without handling its error: %q",
				i+1, strings.TrimSpace(line))
		}
	}
	if !adopted {
		t.Error("nothing adopts the in-cognito marker, so a session started while " +
			"the mode is in force records everything")
	}
}
