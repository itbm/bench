package apps

import (
	"strings"
	"testing"
)

func TestMono(t *testing.T) {
	tests := []struct{ in, want string }{
		// The prototype's monograms.
		{"Slack", "Sl"},
		{"Outlook", "Ou"},
		{"Web page", "Wb"},
		{"Terminal", ">_"},
		{"Zed", "Zd"},
		{"VS Code", "VS"},
		{"Brave", "Br"},
		{"Obsidian", "Ob"},
		{"Calendar", "Ca"},
		{"Figma", "Fi"},
		{"Finder", "Fn"},
		{"Linear", "Li"},
		{"Notes", "No"},
		{"Safari", "Sa"},
		{"Spotify", "Sp"},
		{"Xcode", "Xc"},

		// The same apps by their bundles' names, in any case.
		{"Visual Studio Code", "VS"},
		{"Code", "VS"},
		{"Brave Browser", "Br"},
		{"Microsoft Outlook", "Ou"},
		{"Zed.app", "Zd"},
		{"terminal", ">_"},
		{"WEB PAGE", "Wb"},

		// Initials for several words.
		{"Activity Monitor", "AM"},
		{"System Settings", "SS"},
		{"font book", "FB"},
		{"Adobe Photoshop 2025", "AP"},
		{"Visual Studio Code - Insiders", "VS"},
		{"Zed Preview", "ZP"},
		{"X Lossless Decoder", "XL"},
		{"OBS Studio", "OB"},
		{"HP Smart", "HP"},

		// Apostrophes, brackets, and version numbers.
		{"Bob's Burgers", "BB"},
		{"O’Reilly Learning", "OL"},
		{"O'Reilly", "OR"},
		{"Microsoft Teams (work or school)", "MT"},
		{"Teams (work or school)", "Te"},
		{"Teams [beta] Classic", "TC"},
		{"1Password 7", "1P"},
		{"Microsoft 365", "Mi"},
		{"Final Cut Pro 11", "FC"},
		{"Unbalanced) Name", "UN"},

		// The first two letters of one word.
		{"Mail", "Ma"},
		{"iTerm", "IT"},
		{"1Password", "1P"},
		{"über", "Üb"},
		{"X", "X"},
		{"(Ghostty)", "Gh"},
		{"Preview.app", "Pr"},

		// Nothing to go on.
		{"", ""},
		{"  ", ""},
		{"—", "—"},
	}
	for _, tt := range tests {
		if got := Mono(tt.in); got != tt.want {
			t.Errorf("Mono(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestNameFromBundleID(t *testing.T) {
	tests := []struct{ in, want string }{
		// The prototype's apps.
		{"dev.zed.Zed", "Zed"},
		{"com.microsoft.VSCode", "VS Code"},
		{"com.brave.Browser", "Brave"},
		{"md.obsidian", "Obsidian"},
		{"com.apple.Terminal", "Terminal"},
		{"com.apple.finder", "Finder"},
		{"com.apple.Safari", "Safari"},
		{"com.tinyspeck.slackmacgap", "Slack"},
		{"com.microsoft.Outlook", "Outlook"},
		{"com.google.Chrome", "Chrome"},
		{"com.apple.iCal", "Calendar"},
		{"com.spotify.client", "Spotify"},
		{"com.figma.Desktop", "Figma"},

		// Others, in any case.
		{"COM.MICROSOFT.VSCODE", "VS Code"},
		{"com.apple.systempreferences", "System Settings"},
		{"com.todesktop.230313mzl4w4u92", "Cursor"},
		{" org.mozilla.firefox ", "Firefox"},
		{"org.whispersystems.signal-desktop", "Signal"},
		{"com.apple.launchpad.launcher", "Launchpad"},
		{"com.jetbrains.intellij.ce", "IntelliJ IDEA CE"},

		// Release channels.
		{"com.google.Chrome.beta", "Chrome Beta"},
		{"com.google.Chrome.canary", "Chrome Canary"},
		{"com.brave.Browser.nightly", "Brave Nightly"},
		{"com.microsoft.edgemac.Dev", "Edge Dev"},
		{"com.example.Notebook.beta", "Notebook Beta"},
		{"com.example.dev", "Dev"},
		{"io.beta", "Beta"},

		// Unknown IDs: the last part that names the app.
		{"com.example.Notebook", "Notebook"},
		{"com.example.notebook", "Notebook"},
		{"com.example.iThing", "iThing"},
		{"com.example.notes.desktop", "Notes"},
		{"com.example.Notes.Mac.App", "Notes"},
		{"com.example.notes-desktop", "Notes"},
		{"com.example.Notes-Mac-App", "Notes"},
		{"com.example.desktop-app", "Desktop"},
		{"com.example.bar-baz", "Bar-baz"},
		{"com.example.notes.launcher", "Notes"},
		{"com.client", "Client"},
		{"io.example", "Example"},
		{"single", "Single"},
		{"com.example.", "Example"},

		// Nothing to go on.
		{"", ""},
		{"...", ""},
	}
	for _, tt := range tests {
		if got := NameFromBundleID(tt.in); got != tt.want {
			t.Errorf("NameFromBundleID(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

// TestTablesUseLowerCaseKeys checks that every lookup key is in lower case,
// because lookups lower-case what they look for.
func TestTablesUseLowerCaseKeys(t *testing.T) {
	names := map[string]map[string]string{
		"monograms":  monograms,
		"knownApps":  knownApps,
		"knownSites": knownSites,
		"siteNames":  siteNames,
	}
	names["channels"] = channels
	sets := map[string]map[string]bool{
		"genericParts":    genericParts,
		"genericPrefixes": genericPrefixes,
		"secondLevels":    secondLevels,
	}
	check := func(table, k string) {
		if k != strings.ToLower(k) {
			t.Errorf("%s has the key %q, which isn't in lower case", table, k)
		}
	}
	for table, m := range names {
		for k := range m {
			check(table, k)
		}
	}
	for table, m := range sets {
		for k := range m {
			check(table, k)
		}
	}
}

// TestKnownAppsHaveMonograms checks that the names of the prototype's apps
// get the prototype's monograms.
func TestKnownAppsHaveMonograms(t *testing.T) {
	want := map[string]string{
		"dev.zed.Zed":               "Zd",
		"com.microsoft.VSCode":      "VS",
		"com.brave.Browser":         "Br",
		"md.obsidian":               "Ob",
		"com.apple.Terminal":        ">_",
		"com.apple.finder":          "Fn",
		"com.apple.Safari":          "Sa",
		"com.tinyspeck.slackmacgap": "Sl",
		"com.microsoft.Outlook":     "Ou",
		"com.apple.iCal":            "Ca",
		"com.figma.Desktop":         "Fi",
		"com.linear":                "Li",
		"com.apple.Notes":           "No",
		"com.spotify.client":        "Sp",
		"com.apple.dt.Xcode":        "Xc",
	}
	for id, mono := range want {
		if got := Mono(NameFromBundleID(id)); got != mono {
			t.Errorf("Mono(NameFromBundleID(%q)) = %q, want %q", id, got, mono)
		}
	}
}
