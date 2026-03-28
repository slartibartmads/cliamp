package ui

import (
	tea "github.com/charmbracelet/bubbletea"

	"cliamp/playlist"
)

// provisionalRegistry holds factories registered via RegisterProvisionalPlugin.
var provisionalRegistry = map[string]func() ProvisionalPlugin{}

// RegisterProvisionalPlugin registers a ProvisionalPlugin factory under name.
// Call this from an init() function to make a plugin available for use
// via the provisional_plugin config key. Duplicate names are silently overwritten.
func RegisterProvisionalPlugin(name string, factory func() ProvisionalPlugin) {
	provisionalRegistry[name] = factory
}

// ProvisionalPlugin is a placeholder plugin interface. This is an ad-hoc,
// improvised system — not a final design. Plugins opt into UI sections by
// implementing optional companion interfaces:
//
//   - ProvisionalHeaderProvider: renders content alongside the track info header
//   - ProvisionalHelpProvider: appends a right-aligned suffix to the help bar
//   - ProvisionalKeyHandler: claims keyboard shortcuts
type ProvisionalPlugin interface {
	// OnTick is called every tick with the current track. Returns a Cmd for
	// async work (e.g. fetching art), or nil.
	OnTick(track playlist.Track) tea.Cmd

	// OnMsg processes a Bubbletea message (e.g. async fetch results).
	// Returns a Cmd if the message triggered further async work, or nil.
	OnMsg(msg tea.Msg) tea.Cmd

	// OnResize is called when the terminal is resized. The plugin should
	// invalidate any cached render state that depends on dimensions.
	OnResize()
}

// ProvisionalHeaderProvider is an optional interface for plugins that render
// content alongside the track info header.
type ProvisionalHeaderProvider interface {
	// RenderHeader returns the plugin's rendered content and the number of
	// terminal columns it occupies. height is the number of rows the header
	// spans. Returns ("", 0) when there is nothing to display.
	RenderHeader(height int) (content string, cols int)
}

// ProvisionalHelpProvider is an optional interface for plugins that append a
// suffix to the help bar (right-aligned).
type ProvisionalHelpProvider interface {
	// HelpSuffix returns a short label to display right-aligned on the
	// help bar. Return "" for nothing.
	HelpSuffix() string
}

// ProvisionalKeyHandler is an optional interface that plugins may satisfy to
// claim keyboard shortcuts. Keys not handled here fall through to normal bindings.
type ProvisionalKeyHandler interface {
	// HandleKey processes a key string. Returns handled=true if the key was
	// consumed, and an optional status message to display.
	HandleKey(key string) (handled bool, status string)
}
