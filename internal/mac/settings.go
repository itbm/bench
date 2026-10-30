package mac

import (
	"fmt"

	"github.com/itbm/bench/internal/platform"
)

// This file has the addresses of the panes of System Settings that
// OpenSettings opens, for the permissions that Bench asks for. It's pure
// Go, so it builds and is tested on any OS.

// settingsURLs are the panes' addresses, which System Settings has kept
// from System Preferences: Privacy & Security, at a permission's list. Login
// Items has no address that every version of macOS keeps, so OpenSettings
// opens it with SMAppService instead.
var settingsURLs = map[platform.SettingsPane]string{
	platform.SettingsAccessibility:   "x-apple.systempreferences:com.apple.preference.security?Privacy_Accessibility",
	platform.SettingsScreenRecording: "x-apple.systempreferences:com.apple.preference.security?Privacy_ScreenCapture",
}

// settingsURL returns the address of a pane of System Settings, or an
// error for a pane that has none.
func settingsURL(pane platform.SettingsPane) (string, error) {
	address, ok := settingsURLs[pane]
	if !ok {
		return "", fmt.Errorf("opening System Settings: Bench has no address for the %q pane", pane)
	}
	return address, nil
}
