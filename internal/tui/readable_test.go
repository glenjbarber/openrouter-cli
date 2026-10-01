package tui

import (
	"errors"
	"os"
	"syscall"
	"testing"
	"time"
)

// A report that arrives in pieces is held until the rest of it turns up, rather
// than being read as the keys it starts out looking like.
//
// The wait is a select on the descriptor rather than a read deadline on the
// file, and this is the case that decides between them. Go leaves a descriptor it
// was handed, rather than one it opened, outside the runtime poller, and a
// deadline cannot be set on a descriptor the runtime is not watching. A
// terminal is one it was handed. A hold that asked for a deadline therefore
// never waited for anything, and a report split across two reads went straight
// back to the key reader as a bare escape, which ends the line, and on an empty
// line ends the session. Scrolling fast is what fills the queue that splits one.
func TestHeldReportIsAssembledWithoutAReadDeadline(t *testing.T) {
	client, server := socketPair(t)
	if err := client.SetReadDeadline(time.Now().Add(time.Millisecond)); err == nil {
		t.Skip("this Go watches the descriptor, so it takes a deadline and the " +
			"case under test is not exercised here")
	}

	var notches int
	le := NewLineEditor(client)
	le.OnMouse = func(direction int) {
		if direction != mouseUp {
			t.Errorf("direction = %d, want the wheel up", direction)
		}
		notches++
	}

	// The report is written in three pieces with a pause between them, so the
	// reader is woken by the first one and has to wait for the rest.
	go func() {
		for _, part := range []string{"\x1b[<6", "4;10;20", "Mok\r"} {
			server.Write([]byte(part))
			time.Sleep(5 * time.Millisecond)
		}
	}()

	got, err := le.ReadLine()
	if err != nil {
		t.Fatalf("ReadLine: %v", err)
	}
	if got != "ok" {
		t.Errorf("line = %q, want %q", got, "ok")
	}
	if notches != 1 {
		t.Errorf("notches = %d, want 1", notches)
	}
}

// The wait is bounded, so a lone escape comes back as the interrupt it is rather
// than sitting in the buffer until the reader types something else.
func TestAHeldLoneEscapeIsNotWaitedFor(t *testing.T) {
	client, server := socketPair(t)
	if _, err := server.Write([]byte("\x1b")); err != nil {
		t.Fatalf("write: %v", err)
	}

	le := NewLineEditor(client)
	start := time.Now()
	_, err := le.ReadLine()
	if !errors.Is(err, ErrInterrupt) {
		t.Fatalf("err = %v, want the escape to interrupt", err)
	}
	if waited := time.Since(start); waited < prefixTimeout {
		t.Errorf("waited %v, want the hold to last at least %v", waited, prefixTimeout)
	}
}

// The bounded read gives up on a descriptor with nothing on it, and takes what
// has arrived by the time it is asked for. That is the whole of what it is for.
func TestReadWithin(t *testing.T) {
	client, server := socketPair(t)
	buf := make([]byte, 64)

	start := time.Now()
	n, err := readWithin(int(client.Fd()), prefixTimeout, buf)
	if n != 0 || !errors.Is(err, errPrefixUnfinished) {
		t.Errorf("n = %d, err = %v, want nothing and the prefix unfinished", n, err)
	}
	if waited := time.Since(start); waited < prefixTimeout {
		t.Errorf("waited %v, want the wait to last %v", waited, prefixTimeout)
	}

	if _, err := server.Write([]byte("x")); err != nil {
		t.Fatalf("write: %v", err)
	}
	n, err = readWithin(int(client.Fd()), prefixTimeout, buf)
	if err != nil || n != 1 || buf[0] != 'x' {
		t.Errorf("n = %d, err = %v, buf = %q, want the byte that was written", n, err, buf[:n])
	}
}

// The descriptor is put back the way it was found. The terminal belongs to the
// shell and to whatever runs next, and a non-blocking mode left behind would
// change how they read it.
func TestReadWithinPutsTheDescriptorBack(t *testing.T) {
	client, _ := socketPair(t)
	fd := int(client.Fd())

	before, _, errno := syscall.Syscall(syscall.SYS_FCNTL, uintptr(fd),
		syscall.F_GETFL, 0)
	if errno != 0 {
		t.Fatalf("F_GETFL: %v", errno)
	}
	if _, err := readWithin(fd, time.Millisecond, make([]byte, 64)); err == nil {
		t.Error("a quiet descriptor read something")
	}
	after, _, errno := syscall.Syscall(syscall.SYS_FCNTL, uintptr(fd),
		syscall.F_GETFL, 0)
	if errno != 0 {
		t.Fatalf("F_GETFL: %v", errno)
	}
	if before != after {
		t.Errorf("flags = %#x, want %#x put back", after, before)
	}
}

// socketPair returns the two ends of a connected pair of sockets as files.
//
// A pair rather than a pipe, and the ends built here rather than taken from
// os.Pipe, because that is what makes the descriptor one the runtime is not
// watching. os.Pipe hands back files the poller does watch, and a deadline can
// be set on those, so a pipe passes a hold that asked for a deadline without
// ever exercising the case a terminal is in.
func socketPair(t *testing.T) (*os.File, *os.File) {
	t.Helper()
	fds, err := syscall.Socketpair(syscall.AF_UNIX, syscall.SOCK_STREAM, 0)
	if err != nil {
		t.Fatalf("socketpair: %v", err)
	}
	client := os.NewFile(uintptr(fds[0]), "client")
	server := os.NewFile(uintptr(fds[1]), "server")
	if client == nil || server == nil {
		t.Fatal("socketpair: a file could not be built for an end")
	}
	t.Cleanup(func() {
		client.Close()
		server.Close()
	})
	return client, server
}
