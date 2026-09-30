package buildinfo

import (
	"os"
	"strings"
	"testing"
)

// setDev switches the build flag for one test and restores it afterwards.
func setDev(t *testing.T, v string) {
	t.Helper()
	old := Dev
	Dev = v
	t.Cleanup(func() { Dev = old })
}

// The prod build keeps the historical single-instance ID: changing it would let an old and a new
// release run side by side and fight over the database, tray and hotkeys.
func TestSingleInstanceIDProdIsUnchanged(t *testing.T) {
	setDev(t, "false")
	if got, want := SingleInstanceID(), "cnb.cool.dtapp.kai"; got != want {
		t.Errorf("prod SingleInstanceID() = %q, want %q", got, want)
	}
}

// The dev build needs its own ID. With a shared ID, launching Kai-dev while a released Kai runs
// makes the dev process lose the lock, notify the running prod app and exit, so the dev build
// never starts.
func TestSingleInstanceIDDevIsDistinct(t *testing.T) {
	setDev(t, "false")
	prod := SingleInstanceID()
	setDev(t, "true")
	dev := SingleInstanceID()
	if dev == prod {
		t.Fatalf("dev and prod share the single-instance ID %q", dev)
	}
	if want := prod + ".dev"; dev != want {
		t.Errorf("dev SingleInstanceID() = %q, want %q", dev, want)
	}
}

// main.go is not part of the authoritative go test target, so guard its source text: the
// single-instance ID must come from buildinfo, never a literal that both variants would share.
func TestMainGoUsesBuildinfoSingleInstanceID(t *testing.T) {
	src, err := os.ReadFile("../../main.go")
	if err != nil {
		t.Fatal(err)
	}
	text := string(src)
	if !strings.Contains(text, "UniqueID: buildinfo.SingleInstanceID()") {
		t.Error("main.go must set SingleInstance.UniqueID from buildinfo.SingleInstanceID()")
	}
	if strings.Contains(text, `UniqueID: "`) {
		t.Error("main.go must not hardcode a single-instance UniqueID literal")
	}
}
