// Package tui renders the interactive terminal interface.
//
// It is built on the standard library only. The terminal is driven through
// syscalls rather than a third-party library, so that mouse reporting, the
// alternate screen, and the input mode are under direct control. That control
// is required by the copyable-text requirement, since a higher-level library
// captures the mouse by default and a capture intercepts the drag that begins
// a selection.
package tui
