//go:build darwin

package service

import (
	"reflect"
	"testing"
	"unsafe"

	"github.com/ebitengine/purego/objc"
)

// Issue #163, PR-Agent finding (window_restoration_darwin.go:21-37, "Missing Test"): the
// cross-platform window_restoration_test.go only locks the Go-level wiring
// (disableRestoration / WindowWrapper.DisableRestoration) — it cannot exercise the real
// AppKit call, since it must also compile and pass on non-darwin (where SetWindowNotRestorable
// is a no-op stub). This file is darwin-only and locks the actual AppKit effect: it registers a
// tiny Objective-C class at runtime, via github.com/ebitengine/purego/objc (the same
// zero-cgo mechanism SetWindowNotRestorable itself uses), whose sole property is named
// "restorable" — the Objective-C property-naming convention this generates is exactly
// NSWindow's own real accessor pair, getter "restorable" and setter "setRestorable:" (see
// objc.FieldDef's doc comment) — so calling SetWindowNotRestorable on an instance of it
// exercises the identical selector/argument path it sends to a real NSWindow, without needing
// to construct one (NSWindow's designated initializer takes an NSRect, which purego/objc's
// struct-by-value calling convention doesn't support cleanly; a property-only probe class
// avoids that entirely while still using genuine respondsToSelector:/setRestorable: dispatch).
//
// This test only runs on darwin (this file's build tag) and is therefore not part of this
// repo's Linux CI go job (confirmed via the CI logs: "go version go1.27.1 linux/amd64" for
// ci/go/build-lint-test) — but it runs locally on every macOS dev machine, including via the
// exact repo test command from CLAUDE.md, and would fail if SetWindowNotRestorable's body were
// gutted, sent the wrong selector, or sent the wrong boolean.

func newRestorableProbe(t *testing.T) objc.ID {
	t.Helper()
	class, err := objc.RegisterClass(
		"KaiWindowRestorationProbe163",
		objc.GetClass("NSObject"),
		nil,
		[]objc.FieldDef{
			{Name: "restorable", Type: reflect.TypeFor[bool](), Attribute: objc.ReadWrite},
		},
		nil,
	)
	if err != nil {
		t.Fatalf("objc.RegisterClass: %v", err)
	}
	obj := objc.ID(class).Send(objc.RegisterName("new"))
	// Start true, the same as a real NSWindow's default restorable value, so a no-op
	// SetWindowNotRestorable (the exact regression the PR-Agent finding warned about) would
	// leave this test's assertion failing rather than trivially passing on a false-by-default
	// zero value.
	obj.Send(objc.RegisterName("setRestorable:"), true)
	return obj
}

// idToPointer converts an objc.ID (a live Objective-C object reference, itself a uintptr) to
// unsafe.Pointer the same way github.com/ebitengine/purego/objc's own source does internally
// ("circumvent go vet", objc_runtime_darwin.go) — a same-expression reinterpret, not pointer
// arithmetic, so it stays a safe use of package unsafe.
func idToPointer(id objc.ID) unsafe.Pointer {
	return *(*unsafe.Pointer)(unsafe.Pointer(&id)) //nolint:gosec
}

func TestSetWindowNotRestorableSendsSetRestorableFalseToARealAppKitObject(t *testing.T) {
	probe := newRestorableProbe(t)

	SetWindowNotRestorable(idToPointer(probe))

	// objc.FieldDef's doc comment: a bool field's generated getter is named "is<Name>" (e.g.
	// "isRestorable" for field "restorable") -- exactly matching real NSWindow's own
	// @property (getter=isRestorable) BOOL restorable declaration. The setter stays
	// "setRestorable:" either way.
	got := objc.Send[bool](probe, objc.RegisterName("isRestorable"))
	if got {
		t.Errorf("probe.isRestorable = true after SetWindowNotRestorable, want false (setRestorable:NO must have been sent)")
	}
}

func TestSetWindowNotRestorableNoopsOnObjectThatDoesNotRespond(t *testing.T) {
	// A plain NSObject has no "restorable" property at all — respondsToSelector:setRestorable:
	// must be false, and SetWindowNotRestorable's guard must skip the send rather than crash.
	obj := objc.ID(objc.GetClass("NSObject")).Send(objc.RegisterName("new"))

	SetWindowNotRestorable(idToPointer(obj)) // must not panic
}

func TestSetWindowNotRestorableNoopsOnNilPointer(t *testing.T) {
	SetWindowNotRestorable(nil) // must not panic
}
