package ui

import (
	tea "github.com/charmbracelet/bubbletea"

	"cliamp/playlist"
)

// pluginRegistry holds factories registered via RegisterHeaderPlugin.
var pluginRegistry = map[string]func() HeaderPlugin{}

// RegisterHeaderPlugin registers a HeaderPlugin factory under name.
// Call this from an init() function to make a plugin available for use
// via the header_plugin config key. Duplicate names are silently overwritten.
func RegisterHeaderPlugin(name string, factory func() HeaderPlugin) {
	pluginRegistry[name] = factory
}

// KeyHandler is an optional interface that HeaderPlugin implementations may
// satisfy to claim keyboard shortcuts. Keys not handled here fall through to
// the normal key bindings.
type KeyHandler interface {
	// HandleKey processes a key string. Returns handled=true if the key was
	// consumed, and an optional status message to display.
	HandleKey(key string) (handled bool, status string)
}

// HeaderPlugin is an optional component that renders content alongside the
// track info header. Assign an implementation to Model.headerPlugin to add
// features like album art, waveforms, or any other header decoration.
//
// This is the minimal interface an upstream maintainer would need to add to
// support modular header extensions without bundling the implementation.
type HeaderPlugin interface {
	// OnTick is called every tick with the current track. Returns a Cmd for
	// async work (e.g. fetching art), or nil.
	OnTick(track playlist.Track) tea.Cmd

	// OnMsg processes a Bubbletea message (e.g. async fetch results).
	// Returns a Cmd if the message triggered further async work, or nil.
	OnMsg(msg tea.Msg) tea.Cmd

	// OnResize is called when the terminal is resized. The plugin should
	// invalidate any cached render state that depends on dimensions.
	OnResize()

	// Render returns the plugin's rendered content and the number of terminal
	// columns it occupies. height is the number of rows the header spans.
	// Returns ("", 0) when there is nothing to display.
	Render(height int) (content string, cols int)
}
