package service

import (
	"reflect"
	"testing"

	kevents "cnb.cool/dtapp/kai/internal/events"
	"cnb.cool/dtapp/kai/internal/model"
)

// Issue #112: the translate window must not be closable. Both close entry points — the
// WindowClosing hook (main.go composes event.Cancel() + this rule) and the Cmd+W key binding
// (main.go calls this rule directly) — delegate to ONE named rule, HideTranslateWindow, whose
// contract is broadcast + hide:
//
//   - broadcast kevents.EventWindowClosing with model.WindowTranslate, so the frontend close
//     cleanup (#57 pending history commit, #81/#109 retained session cancel) runs on EVERY
//     close path. A bare Hide() here (the HideOnEscape shape) would silently drop that cleanup
//     on the Cmd+W path — A's binding ruling (handoff-A.md Finding 2).
//   - Hide, never Destroy/Close: destruction is permanent for this app (window_wrapper.go's
//     GetByName lookup never recreates a window), and a hidden window emits no further close
//     events in Wails beta.24, so Hide is destruction-proof by construction.
//
// The tray toggle's bare Hide (ToggleTranslateWindow) stays deliberately broadcast-free — it
// is not a close path; text typed in the window must survive a tray hide.
//
// The rule is exercised with the fake-window pattern of window_toggle_test.go (no running
// Wails app): fakeToggleWindow (reused below) stands in for the window, and fakeEmitter stands
// in for wails' app.Event — main.go passes app.Event.Emit, whose signature is
// Emit(name string, data ...any), so emit here has that exact shape.

// emittedEvent records one broadcast the close policy made: the event name and its payload.
type emittedEvent struct {
	name string
	data []any
}

// fakeEmitter records every emit call, standing in for wails' app.Event.
type fakeEmitter struct {
	events []emittedEvent
}

func (f *fakeEmitter) emit(name string, data ...any) {
	f.events = append(f.events, emittedEvent{name: name, data: data})
}

func TestHideTranslateWindowBroadcastsWindowClosingForTheTranslateWindow(t *testing.T) {
	win := &fakeToggleWindow{visible: true}
	em := &fakeEmitter{}

	HideTranslateWindow(win, em.emit)

	if len(em.events) != 1 {
		t.Fatalf("broadcasts = %d, want exactly 1 (the rule broadcasts once per close path, not per entry point)", len(em.events))
	}
	if em.events[0].name != kevents.EventWindowClosing {
		t.Fatalf("broadcast name = %q, want %q", em.events[0].name, kevents.EventWindowClosing)
	}
	if !reflect.DeepEqual(em.events[0].data, []any{model.WindowTranslate}) {
		t.Fatalf("broadcast payload = %v, want [%q] (the frontend filters closes by window name; a missing name breaks the #57/#81/#109 close cleanup)", em.events[0].data, model.WindowTranslate)
	}
}

func TestHideTranslateWindowHidesWithoutDestroying(t *testing.T) {
	win := &fakeToggleWindow{visible: true}
	em := &fakeEmitter{}

	HideTranslateWindow(win, em.emit)

	if win.hides != 1 {
		t.Fatalf("Hide calls = %d, want exactly 1 (hide-not-destroy is #112's recovery-by-construction)", win.hides)
	}
	if win.visible {
		t.Fatal("the window must end up hidden")
	}
}

func TestHideTranslateWindowWithoutAWindowIsANoOp(t *testing.T) {
	// The window lookup can legitimately find nothing (nil app, window not registered — see
	// translateWindow()). Nothing can be hidden, so nothing may be broadcast either: there was
	// no close. Mirrors LowerForSettings' nil guard. Must not panic.
	em := &fakeEmitter{}

	HideTranslateWindow(nil, em.emit)

	if len(em.events) != 0 {
		t.Fatalf("broadcasts = %d, want 0 (no window: no close happened)", len(em.events))
	}
}
