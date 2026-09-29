package doublecopy

import "testing"

// Issue #199 privacy: the copied text goes to a translation engine, possibly a cloud one. Two
// independent guards keep secrets out: the app being copied from, and the pasteboard's own markers.

func TestDeniedBundles(t *testing.T) {
	for _, id := range []string{
		"com.1password.1password", "com.agilebits.onepassword7", "com.agilebits.onepassword-osx",
		"com.bitwarden.desktop", "com.dashlane.dashlanephonefinal", "com.lastpass.LastPass",
		"com.apple.keychainaccess", "com.apple.Passwords",
	} {
		if reason, blocked := Blocked(id, nil); !blocked || reason == "" {
			t.Errorf("%s is not blocked (reason %q)", id, reason)
		}
	}
}

func TestDeniedBundleMatchIsCaseInsensitiveAndExact(t *testing.T) {
	if _, blocked := Blocked("COM.BITWARDEN.DESKTOP", nil); !blocked {
		t.Error("bundle ids are case-insensitive on macOS; upper-case slipped past")
	}
	for _, ok := range []string{"", "com.apple.Safari", "com.anthropic.claudefordesktop", "com.1password.notes", "com.apple.PasswordsFake"} {
		if _, blocked := Blocked(ok, nil); blocked {
			t.Errorf("%q is blocked, want allowed", ok)
		}
	}
}

func TestConcealedAndTransientPasteboardTypesAreBlocked(t *testing.T) {
	for _, typ := range []string{"org.nspasteboard.ConcealedType", "org.nspasteboard.TransientType", "com.agilebits.onepassword"} {
		if reason, blocked := Blocked("com.apple.Safari", []string{"public.utf8-plain-text", typ}); !blocked || reason == "" {
			t.Errorf("pasteboard carrying %s is not blocked", typ)
		}
	}
	if _, blocked := Blocked("com.apple.Safari", []string{"public.utf8-plain-text", "public.html"}); blocked {
		t.Error("an ordinary pasteboard is blocked")
	}
}
