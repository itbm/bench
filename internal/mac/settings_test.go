package mac

import (
	"strings"
	"testing"

	"github.com/itbm/bench/internal/platform"
)

func TestSettingsURL(t *testing.T) {
	for pane, want := range map[platform.SettingsPane]string{
		platform.SettingsAccessibility:   "?Privacy_Accessibility",
		platform.SettingsScreenRecording: "?Privacy_ScreenCapture",
	} {
		got, err := settingsURL(pane)
		if err != nil {
			t.Fatalf("settingsURL(%s): %v", pane, err)
		}
		if !strings.HasPrefix(got, "x-apple.systempreferences:com.apple.preference.security?") || !strings.HasSuffix(got, want) {
			t.Errorf("settingsURL(%s) = %q, want Privacy & Security's %s list", pane, got, want)
		}
	}
	// Login Items opens with SMAppService, not by address.
	if got, err := settingsURL(platform.SettingsLoginItems); err == nil {
		t.Errorf("settingsURL(login-items) = %q, want an error", got)
	}
}
