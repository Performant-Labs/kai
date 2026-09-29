package doublecopy

import "strings"

// deniedBundles are apps whose copies must never reach a translation engine: password managers and
// the keychain apps. Matched by bundle identifier, case-insensitively and exactly.
var deniedBundles = map[string]string{
	"com.1password.1password":         "1Password",
	"com.agilebits.onepassword7":      "1Password",
	"com.agilebits.onepassword-osx":   "1Password",
	"com.agilebits.onepassword4":      "1Password",
	"com.bitwarden.desktop":           "Bitwarden",
	"com.dashlane.dashlanephonefinal": "Dashlane",
	"com.dashlane.dashlane":           "Dashlane",
	"com.lastpass.lastpass":           "LastPass",
	"com.lastpass.lastpassmacdesktop": "LastPass",
	"com.apple.keychainaccess":        "Keychain Access",
	"com.apple.passwords":             "Passwords",
}

// concealedTypes are pasteboard types by which the copying app says "this is secret or transient"
// (the nspasteboard.org convention, plus 1Password's own marker).
var concealedTypes = []string{
	"org.nspasteboard.ConcealedType",
	"org.nspasteboard.TransientType",
	"com.agilebits.onepassword",
}

// Blocked reports whether a copy made in the app with the given bundle identifier, leaving a
// pasteboard that offers the given types, must not be translated, and why. The reason names the
// rule, never the content.
func Blocked(bundle string, pasteboardTypes []string) (reason string, blocked bool) {
	if name, ok := deniedBundles[strings.ToLower(bundle)]; ok && bundle != "" {
		return "copied from " + name + " (password manager or keychain app)", true
	}
	for _, have := range pasteboardTypes {
		for _, marker := range concealedTypes {
			if have == marker {
				return "pasteboard is marked " + marker, true
			}
		}
	}
	return "", false
}
