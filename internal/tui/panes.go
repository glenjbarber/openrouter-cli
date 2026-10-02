package tui

// mainPane is the index of the main conversation. It is always pane 0 and is
// never removed, so that a reader always has a pane to return to.
const mainPane = 0

// mainPaneName is the name pane 0 carries.
const mainPaneName = "main"

// paneSet is the numbered list of panes and which one is being shown.
//
// Pane 0 is the main conversation, the screen the client has always drawn.
// Later panes are added after it and are addressed by their index, which is the
// number a next and previous binding steps through.
//
// The zero value is a set holding pane 0 alone and showing it, so a Session
// built without one behaves as it did before panes existed. The set holds no
// lock of its own and is guarded by the mutex of the Session that holds it.
type paneSet struct {
	// names holds one entry per pane, in index order. It is empty until first
	// used, and is filled with pane 0 then.
	names []string
	// current is the index of the pane being shown.
	current int
}

// ensure puts pane 0 in place when the set is still the zero value.
func (p *paneSet) ensure() {
	if len(p.names) == 0 {
		p.names = []string{mainPaneName}
	}
}

// Len returns how many panes there are, which is never less than one.
func (p *paneSet) Len() int {
	p.ensure()
	return len(p.names)
}

// Current returns the index of the pane being shown.
func (p *paneSet) Current() int {
	p.ensure()
	return p.current
}

// OnMain reports whether the main conversation is the pane being shown.
func (p *paneSet) OnMain() bool {
	return p.Current() == mainPane
}

// Name returns the name of a pane, or the empty string for an index that is
// not one.
func (p *paneSet) Name(i int) string {
	p.ensure()
	if i < 0 || i >= len(p.names) {
		return ""
	}
	return p.names[i]
}

// Add appends a pane and returns its index. The pane shown does not change.
func (p *paneSet) Add(name string) int {
	p.ensure()
	p.names = append(p.names, name)
	return len(p.names) - 1
}

// Step shows the pane that many places on from the one shown, wrapping at both
// ends, and returns its index. A step of one is next and minus one is previous.
// With a single pane the set stays where it is.
func (p *paneSet) Step(delta int) int {
	p.ensure()
	n := len(p.names)
	p.current = ((p.current+delta)%n + n) % n
	return p.current
}

// Select shows the pane at the index, and reports whether there is one. An
// index that is not a pane leaves the pane shown as it was.
func (p *paneSet) Select(i int) bool {
	p.ensure()
	if i < 0 || i >= len(p.names) {
		return false
	}
	p.current = i
	return true
}
