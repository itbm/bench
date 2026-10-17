package apps

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

// monograms are the fixed monograms of known apps, by lower-case name. The
// first group is the prototype's; the second gives the same monograms to
// the names those apps' bundles use.
var monograms = map[string]string{
	"slack":    "Sl",
	"outlook":  "Ou",
	"web page": "Wb",
	"terminal": ">_",
	"zed":      "Zd",
	"vs code":  "VS",
	"brave":    "Br",
	"obsidian": "Ob",
	"calendar": "Ca",
	"figma":    "Fi",
	"finder":   "Fn",
	"linear":   "Li",
	"notes":    "No",
	"safari":   "Sa",
	"spotify":  "Sp",
	"xcode":    "Xc",

	"visual studio code": "VS",
	"code":               "VS", // VS Code's bundle name.
	"brave browser":      "Br",
	"microsoft outlook":  "Ou",
}

// Mono returns the two-character monogram for a sub-tab: known apps have
// fixed ones ("Zed" is "Zd", "Finder" is "Fn", "Terminal" is ">_",
// "Web page" is "Wb"); names of more than one word use their initials ("VS
// Code" is "VS"); others use their first two letters ("Slack" is "Sl").
//
// Initials are those of the first two words, in upper case ("Activity
// Monitor" is "AM"), except that a first word in capitals is used alone
// ("OBS Studio" is "OB"). A single word keeps its second letter's case, with
// the first capitalised ("iTerm" is "IT", "1Password" is "1P").
//
// Words are runs of letters and digits; apostrophes inside a word are
// dropped rather than splitting it ("Bob's Burgers" is "BB"). Text in
// brackets and numbers after the first word, such as versions, are ignored
// when other words remain ("Teams (work or school)" is "Te", "1Password 7"
// is "1P"). A final ".app" is ignored too, and an empty name has an empty
// monogram.
func Mono(name string) string {
	name = DisplayName(name)
	if m, ok := monograms[strings.ToLower(name)]; ok {
		return m
	}
	words := monoWords(withoutBrackets(name))
	if len(words) == 0 {
		words = monoWords(name)
	}
	switch {
	case len(words) == 0:
		return firstTwo(name)
	case len(words) == 1 || isAcronym(words[0]):
		return firstTwo(words[0])
	default:
		return initial(words[0]) + initial(words[1])
	}
}

// monoWords returns the words of a name that a monogram can use: runs of
// letters, digits, and apostrophes, with the apostrophes removed, leaving
// out words that are only digits unless they come first.
func monoWords(name string) []string {
	fields := strings.FieldsFunc(name, func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r) && !isApostrophe(r)
	})
	words := make([]string, 0, len(fields))
	for _, f := range fields {
		w := strings.Map(func(r rune) rune {
			if isApostrophe(r) {
				return -1
			}
			return r
		}, f)
		if w == "" || (len(words) > 0 && isNumber(w)) {
			continue
		}
		words = append(words, w)
	}
	return words
}

// isApostrophe reports whether r is a straight or curly apostrophe.
func isApostrophe(r rune) bool { return r == '\'' || r == '’' }

// isNumber reports whether word is only digits, such as a version.
func isNumber(word string) bool {
	for _, r := range word {
		if !unicode.IsDigit(r) {
			return false
		}
	}
	return word != ""
}

// withoutBrackets removes text in round or square brackets, with the
// brackets, as in "Teams (work or school)".
func withoutBrackets(s string) string {
	var b strings.Builder
	depth := 0
	for _, r := range s {
		switch {
		case r == '(' || r == '[':
			depth++
		case (r == ')' || r == ']') && depth > 0:
			depth--
		case depth == 0:
			b.WriteRune(r)
		}
	}
	return b.String()
}

// firstTwo returns the first two characters of s, the first capitalised.
func firstTwo(s string) string {
	first, size := utf8.DecodeRuneInString(s)
	if size == 0 {
		return ""
	}
	rest := s[size:]
	if second, n := utf8.DecodeRuneInString(rest); n > 0 {
		return string(unicode.ToUpper(first)) + string(second)
	}
	return string(unicode.ToUpper(first))
}

// initial returns the first character of a word, in upper case.
func initial(word string) string {
	r, _ := utf8.DecodeRuneInString(word)
	return string(unicode.ToUpper(r))
}

// isAcronym reports whether word is two or more capital letters, such as
// "VS" or "OBS".
func isAcronym(word string) bool {
	if utf8.RuneCountInString(word) < 2 {
		return false
	}
	for _, r := range word {
		if !unicode.IsUpper(r) {
			return false
		}
	}
	return true
}

// knownApps are the names of well-known apps, by lower-case bundle ID.
var knownApps = map[string]string{
	// The prototype's apps.
	"dev.zed.zed":               "Zed",
	"com.microsoft.vscode":      "VS Code",
	"com.brave.browser":         "Brave",
	"md.obsidian":               "Obsidian",
	"com.apple.terminal":        "Terminal",
	"com.apple.finder":          "Finder",
	"com.apple.safari":          "Safari",
	"com.tinyspeck.slackmacgap": "Slack",
	"com.microsoft.outlook":     "Outlook",
	"com.apple.ical":            "Calendar",
	"com.apple.notes":           "Notes",
	"com.apple.dt.xcode":        "Xcode",
	"com.figma.desktop":         "Figma",
	"com.linear":                "Linear",
	"com.spotify.client":        "Spotify",

	// Browsers.
	"com.google.chrome":                   "Chrome",
	"com.google.chrome.canary":            "Chrome Canary",
	"org.mozilla.firefox":                 "Firefox",
	"org.mozilla.firefoxdeveloperedition": "Firefox Developer Edition",
	"com.apple.safaritechnologypreview":   "Safari Technology Preview",
	"com.microsoft.edgemac":               "Edge",
	"company.thebrowser.browser":          "Arc",
	"com.vivaldi.vivaldi":                 "Vivaldi",
	"com.operasoftware.opera":             "Opera",

	// Editors, terminals, and developer tools.
	"dev.zed.zed-preview":           "Zed Preview",
	"com.microsoft.vscodeinsiders":  "VS Code Insiders",
	"com.todesktop.230313mzl4w4u92": "Cursor",
	"com.sublimetext.4":             "Sublime Text",
	"com.jetbrains.goland":          "GoLand",
	"com.jetbrains.intellij":        "IntelliJ IDEA",
	"com.jetbrains.intellij.ce":     "IntelliJ IDEA CE",
	"com.jetbrains.pycharm":         "PyCharm",
	"com.jetbrains.webstorm":        "WebStorm",
	"com.github.githubclient":       "GitHub Desktop",
	"com.docker.docker":             "Docker",
	"com.postmanlabs.mac":           "Postman",
	"com.googlecode.iterm2":         "iTerm2",
	"com.mitchellh.ghostty":         "Ghostty",
	"net.kovidgoyal.kitty":          "kitty",
	"com.github.wez.wezterm":        "WezTerm",
	"dev.warp.warp-stable":          "Warp",
	"com.apple.iphonesimulator":     "Simulator",
	"com.apple.dt.instruments":      "Instruments",

	// Communication.
	"com.apple.mail":                    "Mail",
	"com.apple.mobilesms":               "Messages",
	"com.apple.facetime":                "FaceTime",
	"com.microsoft.teams":               "Teams",
	"com.microsoft.teams2":              "Teams",
	"us.zoom.xos":                       "Zoom",
	"com.hnc.discord":                   "Discord",
	"ru.keepcoder.telegram":             "Telegram",
	"net.whatsapp.whatsapp":             "WhatsApp",
	"org.whispersystems.signal-desktop": "Signal",

	// Productivity.
	"com.apple.reminders":            "Reminders",
	"com.apple.addressbook":          "Contacts",
	"com.apple.preview":              "Preview",
	"com.apple.textedit":             "TextEdit",
	"com.apple.freeform":             "Freeform",
	"com.apple.iwork.pages":          "Pages",
	"com.apple.iwork.numbers":        "Numbers",
	"com.apple.iwork.keynote":        "Keynote",
	"com.microsoft.word":             "Word",
	"com.microsoft.excel":            "Excel",
	"com.microsoft.powerpoint":       "PowerPoint",
	"com.microsoft.onenote.mac":      "OneNote",
	"notion.id":                      "Notion",
	"com.1password.1password":        "1Password",
	"com.culturedcode.thingsmac":     "Things",
	"com.openai.chat":                "ChatGPT",
	"com.anthropic.claudefordesktop": "Claude",

	// Media and design.
	"com.apple.music":            "Music",
	"com.apple.ibooksx":          "Books",
	"com.apple.voicememos":       "Voice Memos",
	"com.apple.imovieapp":        "iMovie",
	"com.apple.garageband10":     "GarageBand",
	"com.apple.logic10":          "Logic Pro",
	"com.apple.finalcut":         "Final Cut Pro",
	"com.bohemiancoding.sketch3": "Sketch",
	"com.apple.tv":               "TV",
	"com.apple.podcasts":         "Podcasts",
	"com.apple.photos":           "Photos",
	"com.apple.photobooth":       "Photo Booth",
	"com.apple.quicktimeplayerx": "QuickTime Player",

	// macOS.
	"com.apple.systempreferences":      "System Settings",
	"com.apple.appstore":               "App Store",
	"com.apple.activitymonitor":        "Activity Monitor",
	"com.apple.console":                "Console",
	"com.apple.diskutility":            "Disk Utility",
	"com.apple.keychainaccess":         "Keychain Access",
	"com.apple.screenshot.launcher":    "Screenshot",
	"com.apple.scripteditor2":          "Script Editor",
	"com.apple.systemprofiler":         "System Information",
	"com.apple.automator":              "Automator",
	"com.apple.shortcuts":              "Shortcuts",
	"com.apple.calculator":             "Calculator",
	"com.apple.maps":                   "Maps",
	"com.apple.weather":                "Weather",
	"com.apple.findmy":                 "Find My",
	"com.apple.fontbook":               "Font Book",
	"com.apple.image_capture":          "Image Capture",
	"com.apple.screensharing":          "Screen Sharing",
	"com.apple.screencontinuity":       "iPhone Mirroring",
	"com.apple.launchpad.launcher":     "Launchpad",
	"com.apple.exposelauncher":         "Mission Control",
	"com.apple.backup.launcher":        "Time Machine",
	"com.apple.siri.launcher":          "Siri",
	"com.apple.audio.audiomidisetup":   "Audio MIDI Setup",
	"com.apple.colorsyncutility":       "ColorSync Utility",
	"com.apple.voiceoverutility":       "VoiceOver Utility",
	"com.apple.airport.airportutility": "AirPort Utility",
}

// genericParts are bundle ID parts that name a kind of app rather than the
// app, such as "client" in com.spotify.client or "desktop" in
// signal-desktop.
var genericParts = map[string]bool{
	"app":         true,
	"application": true,
	"browser":     true,
	"client":      true,
	"desktop":     true,
	"launcher":    true,
	"mac":         true,
	"macos":       true,
	"osx":         true,
}

// channels are bundle ID parts that name a release channel, as in
// com.google.Chrome.beta, by lower-case part.
var channels = map[string]string{
	"beta":     "Beta",
	"canary":   "Canary",
	"dev":      "Dev",
	"insiders": "Insiders",
	"nightly":  "Nightly",
	"preview":  "Preview",
}

// NameFromBundleID guesses an app's name from its bundle ID when it isn't
// installed: the last part, with known IDs mapped to their names
// ("com.microsoft.VSCode" is "VS Code", "com.brave.Browser" is "Brave",
// "md.obsidian" is "Obsidian").
//
// For an unknown ID, a last part that names a kind of app, such as
// "client" or "desktop", gives way to the part before it, as does a
// generic ending after a hyphen ("com.example.notes-desktop" is "Notes"),
// and a part in lower case is capitalised ("com.example.notes.desktop" is
// "Notes"). A release channel at the end is added to the app's name
// ("com.google.Chrome.beta" is "Chrome Beta"). IDs are compared without
// regard to case. An empty ID gives "".
func NameFromBundleID(id string) string {
	id = strings.TrimSpace(id)
	if name, ok := knownApps[strings.ToLower(id)]; ok {
		return name
	}
	parts := strings.FieldsFunc(id, func(r rune) bool { return r == '.' })
	if len(parts) == 0 {
		return ""
	}
	if name := channelName(parts); name != "" {
		return name
	}
	i := len(parts) - 1
	// Never fall back to the first part, which is a domain such as "com".
	for i > 1 && genericParts[strings.ToLower(parts[i])] {
		i--
	}
	return capitaliseLower(withoutGenericEnding(parts[i]))
}

// channelName names an app whose bundle ID ends in a release channel, such
// as com.google.Chrome.beta, from the rest of the ID. It returns "" if the
// ID doesn't end in a channel, or the rest isn't a known app and is too
// short to name one: com.example.dev is an app called Dev.
func channelName(parts []string) string {
	n := len(parts)
	channel, ok := channels[strings.ToLower(parts[n-1])]
	if !ok || n < 3 {
		return ""
	}
	base := strings.Join(parts[:n-1], ".")
	if _, known := knownApps[strings.ToLower(base)]; !known && n < 4 {
		return ""
	}
	return NameFromBundleID(base) + " " + channel
}

// withoutGenericEnding removes generic words after hyphens at the end of a
// bundle ID part, as in "signal-desktop", if anything is left.
func withoutGenericEnding(part string) string {
	words := strings.Split(part, "-")
	for len(words) > 1 && genericParts[strings.ToLower(words[len(words)-1])] {
		words = words[:len(words)-1]
	}
	return strings.Join(words, "-")
}

// capitaliseLower capitalises a word that's all in lower case, and leaves
// others, such as "iTerm2", as they are.
func capitaliseLower(word string) string {
	if word == "" || strings.ToLower(word) != word {
		return word
	}
	r, size := utf8.DecodeRuneInString(word)
	return string(unicode.ToUpper(r)) + word[size:]
}
