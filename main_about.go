package main

import (
	"github.com/wailsapp/wails/v3/pkg/application"
)

// installAboutMenu makes the menu bar's About item show the About panel in front of kai's windows
// (issue #8). Wails' default application menu wires the item to the native
// orderFrontStandardAboutPanel: selector, which orders the panel front only within the normal
// window level — a pinned (always-on-top, floating-level) translate window stayed above it, and no
// Go code ran at all. The default menu is rebuilt here with the About item rerouted through the
// Swift bridge: NoRole drops the native selector (a plain item's click reaches OnClick), and the
// bridge activates the app and lifts the panel above every kai window. No-op wherever the default
// menu has no About item (non-darwin menus build no app menu). Call it once after
// application.New and before Run, so the startup path installs the menu.
func installAboutMenu(app *application.App, showAbout func()) {
	if app == nil || showAbout == nil {
		return
	}
	menu := application.DefaultApplicationMenu()
	if menu == nil {
		return
	}
	// The app menu (kai's own submenu on darwin) is the only one carrying an About item;
	// FindByRole searches recursively, and finds nothing wherever the platform's default
	// menu has no About item (non-darwin).
	if about := menu.FindByRole(application.About); about != nil {
		about.SetRole(application.NoRole).OnClick(func(*application.Context) { showAbout() })
	}
	app.Menu.Set(menu)
}
