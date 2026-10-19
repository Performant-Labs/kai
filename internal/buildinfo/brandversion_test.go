package buildinfo

import "testing"

// setVersion switches the injected version for one test and restores it afterwards (same
// pattern as setDev in singleinstance_test.go).
func setVersion(t *testing.T, v string) {
	t.Helper()
	old := Version
	Version = v
	t.Cleanup(func() { Version = old })
}

// Issue #69: every persistent window's native title ends in the running build's brand-version
// suffix (main.go composes i18n.T("window.<name>_title") + " — " + BrandVersion()), so the
// build that is running is visible without opening Settings. A release reads "Kai <version>";
// the brand and the number are never translated, only the window word before the dash is.
func TestBrandVersionRelease(t *testing.T) {
	setVersion(t, "0.5.0")
	setDev(t, "false")
	if got, want := BrandVersion(), "Kai 0.5.0"; got != want {
		t.Errorf("release BrandVersion() = %q, want %q", got, want)
	}
}

// A development build must say so (the About card already marks one, issue #41); otherwise a
// dev build is mistaken for a release. The marker mirrors the About card's wording in short
// form: " (dev)".
func TestBrandVersionDevBuildCarriesAMarker(t *testing.T) {
	setVersion(t, "0.5.0")
	setDev(t, "true")
	if got, want := BrandVersion(), "Kai 0.5.0 (dev)"; got != want {
		t.Errorf("dev BrandVersion() = %q, want %q", got, want)
	}
}

// The local default is Version="dev" with Dev="true": the version word already says dev, so
// the suffix must not repeat it ("Kai dev (dev)").
func TestBrandVersionLocalDefaultDoesNotRepeatDev(t *testing.T) {
	setVersion(t, "dev")
	setDev(t, "true")
	if got, want := BrandVersion(), "Kai dev"; got != want {
		t.Errorf("local BrandVersion() = %q, want %q", got, want)
	}
}
