package i18n

import (
	"context"
	"os"
	"os/exec"
	"regexp"
	"runtime"
	"strings"
	"time"
)

// Auto is the settings value meaning "follow the system language".
const Auto Locale = "auto"

// systemLocaleFn resolves the system language to a supported locale. A variable so tests can
// substitute it.
var systemLocaleFn = detectSystemLocale

var appleLangRe = regexp.MustCompile(`"([^"]+)"`)

// classifySystemLanguage maps a system language tag (zh-Hans-CN, en_GB.UTF-8, ...) to a supported
// locale. Only a Chinese tag selects Chinese; everything else, including empty or unknown, is
// English.
func classifySystemLanguage(tag string) Locale {
	if strings.HasPrefix(strings.ToLower(strings.TrimSpace(tag)), "zh") {
		return ZH_CN
	}
	return EN_US
}

// parseAppleLanguages returns the first entry of `defaults read -g AppleLanguages` output, the
// user's most preferred language, or "" if none is found.
func parseAppleLanguages(out string) string {
	if m := appleLangRe.FindStringSubmatch(out); m != nil {
		return m[1]
	}
	return ""
}

// detectSystemLocale reads the user's preferred language: on macOS the first AppleLanguages
// entry (a GUI app does not inherit a shell's LANG), elsewhere the standard locale variables.
// Anything unreadable resolves to English.
func detectSystemLocale() Locale {
	if runtime.GOOS == "darwin" {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		if out, err := exec.CommandContext(ctx, "defaults", "read", "-g", "AppleLanguages").Output(); err == nil {
			if tag := parseAppleLanguages(string(out)); tag != "" {
				return classifySystemLanguage(tag)
			}
		}
	}
	for _, v := range []string{"LC_ALL", "LC_MESSAGES", "LANG"} {
		if tag := os.Getenv(v); tag != "" {
			return classifySystemLanguage(tag)
		}
	}
	return EN_US
}
