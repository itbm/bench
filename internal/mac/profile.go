package mac

import (
	"crypto/sha1"
	"fmt"
)

// profileNamespace is the namespace for web profiles' name-based UUIDs:
// f8fa9f4c-22be-52f3-83eb-fa3a6a9ba2b9, the version 5 UUID of
// "https://github.com/itbm/bench/web-profile" in the URL namespace (RFC
// 9562). Never change it: every profile's data store, and so its logins, is
// found by the UUIDs derived from it.
var profileNamespace = [16]byte{
	0xf8, 0xfa, 0x9f, 0x4c, 0x22, 0xbe, 0x52, 0xf3,
	0x83, 0xeb, 0xfa, 0x3a, 0x6a, 0x9b, 0xa2, 0xb9,
}

// defaultProfile names the data store of Web items that don't set a profile.
const defaultProfile = "default"

// ProfileUUID returns the stable, name-based (version 5) UUID for a web
// profile, in its canonical lower-case form. Bench passes it to
// +[WKWebsiteDataStore dataStoreForIdentifier:], so Web items with the same
// profile share a persistent data store, and logins survive restarts. An
// empty profile is the "default" profile. Names are compared exactly, so
// "Work" and "work" are different profiles.
func ProfileUUID(profile string) string {
	if profile == "" {
		profile = defaultProfile
	}
	return uuid5(profileNamespace, profile)
}

// uuid5 returns the version 5 UUID of name in namespace (RFC 9562, section
// 5.5): the first 16 bytes of the SHA-1 hash of the namespace and the name,
// with the version and variant bits set.
func uuid5(namespace [16]byte, name string) string {
	h := sha1.New()
	h.Write(namespace[:])
	h.Write([]byte(name))
	var u [16]byte
	copy(u[:], h.Sum(nil))
	u[6] = (u[6] & 0x0f) | 0x50 // Version 5.
	u[8] = (u[8] & 0x3f) | 0x80 // The RFC 9562 variant.
	return fmt.Sprintf("%x-%x-%x-%x-%x", u[0:4], u[4:6], u[6:8], u[8:10], u[10:16])
}

// safariVersion returns the major version of Safari that ships with a macOS
// major version: Safari 17 with macOS 14, 18 with macOS 15, and, from macOS
// 26, the same number as macOS.
func safariVersion(macOSMajor int) int {
	switch {
	case macOSMajor >= 26:
		return macOSMajor
	case macOSMajor >= 11:
		return macOSMajor + 3
	default:
		return 17
	}
}

// userAgentSuffix is what Bench appends to WKWebView's user agent (with
// WKWebViewConfiguration's applicationNameForUserAgent) so that sites see
// Safari's user agent. Without the Version and Safari tokens, some sites,
// Slack among them, say the browser isn't supported.
func userAgentSuffix(macOSMajor int) string {
	return fmt.Sprintf("Version/%d.0 Safari/605.1.15", safariVersion(macOSMajor))
}
