package tui

import "testing"

func TestPaneZeroValueIsMain(t *testing.T) {
	var p paneSet
	if p.Len() != 1 {
		t.Fatalf("Len = %d, want 1", p.Len())
	}
	if p.Current() != mainPane || !p.OnMain() {
		t.Fatalf("Current = %d, want %d", p.Current(), mainPane)
	}
	if p.Name(mainPane) != mainPaneName {
		t.Fatalf("Name(0) = %q, want %q", p.Name(mainPane), mainPaneName)
	}
}

func TestPaneSessionStartsOnMain(t *testing.T) {
	s := &Session{}
	if !s.panes.OnMain() {
		t.Fatal("a new session is not on pane 0")
	}
}

func TestPaneAddKeepsCurrent(t *testing.T) {
	var p paneSet
	if got := p.Add("extra"); got != 1 {
		t.Fatalf("Add = %d, want 1", got)
	}
	if p.Len() != 2 || !p.OnMain() {
		t.Fatalf("Len = %d, OnMain = %v after Add", p.Len(), p.OnMain())
	}
	if p.Name(1) != "extra" {
		t.Fatalf("Name(1) = %q", p.Name(1))
	}
}

func TestPaneSelect(t *testing.T) {
	var p paneSet
	p.Add("extra")
	if !p.Select(1) || p.Current() != 1 || p.OnMain() {
		t.Fatalf("Select(1) did not show pane 1: current %d", p.Current())
	}
	for _, bad := range []int{-1, 2, 99} {
		if p.Select(bad) {
			t.Errorf("Select(%d) = true, want false", bad)
		}
		if p.Current() != 1 {
			t.Errorf("Select(%d) moved the current pane to %d", bad, p.Current())
		}
	}
	if !p.Select(mainPane) || !p.OnMain() {
		t.Fatal("Select(0) did not return to the main pane")
	}
}

func TestPaneNameOutOfRange(t *testing.T) {
	var p paneSet
	if p.Name(-1) != "" || p.Name(1) != "" {
		t.Fatal("Name of a missing pane is not empty")
	}
}
