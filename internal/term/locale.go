package term

import (
	"context"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
)

// fallbackLang is LANG for sessions when the user's region settings don't
// name a locale that the system has.
const fallbackLang = "en_US.UTF-8"

// localeWait is the longest Bench waits to read the region settings.
const localeWait = 2 * time.Second

// localeDir is where macOS keeps its locales, one folder each.
const localeDir = "/usr/share/locale"

// defaultLang is LANG for sessions when Bench's own environment has none, as
// when it's opened from the Finder, the Dock, or at login. Like Terminal, it
// follows the language and region in the user's macOS settings, as a UTF-8
// locale. It's worked out once, the first time it's needed.
var defaultLang = sync.OnceValue(func() string {
	return localeLang(appleLocale(), localeExists)
})

// localeLang returns the UTF-8 locale for a macOS locale identifier, such as
// "de_DE", "en_GB@rg=dezzzz", or "zh-Hans_CN", if exists reports that the
// system has it. Otherwise, it returns fallbackLang.
func localeLang(id string, exists func(name string) bool) string {
	// Keywords after "@", such as a calendar or another region for
	// formats, and a script after "-", have no place in LANG.
	id, _, _ = strings.Cut(id, "@")
	lang, region, ok := strings.Cut(id, "_")
	lang, _, _ = strings.Cut(lang, "-")
	if !ok || !localePart(lang) || !localePart(region) {
		return fallbackLang
	}
	if name := lang + "_" + region + ".UTF-8"; exists(name) {
		return name
	}
	return fallbackLang
}

// localePart reports whether s can be a language or region code: a few
// ASCII letters or digits (as in es_419).
func localePart(s string) bool {
	if s == "" || len(s) > 8 {
		return false
	}
	for _, c := range []byte(s) {
		if !('a' <= c && c <= 'z' || 'A' <= c && c <= 'Z' || '0' <= c && c <= '9') {
			return false
		}
	}
	return true
}

// localeExists reports whether the system has the locale name.
func localeExists(name string) bool {
	return isDir(filepath.Join(localeDir, name))
}

// appleLocale returns the user's locale from their macOS region settings
// (the AppleLocale default), or "" if it can't be read or this isn't macOS.
func appleLocale() string {
	if runtime.GOOS != "darwin" {
		return ""
	}
	ctx, cancel := context.WithTimeout(context.Background(), localeWait)
	defer cancel()
	out, err := exec.CommandContext(ctx, "/usr/bin/defaults", "read", "-g", "AppleLocale").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}
