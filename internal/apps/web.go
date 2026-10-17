package apps

import (
	"net"
	"net/url"
	"strings"
	"unicode/utf8"
)

// webPage is the name of a Web item whose address has no site name.
const webPage = "Web page"

// knownSites name sites whose name isn't their main domain name, by host.
var knownSites = map[string]string{
	"outlook.office.com":      "Outlook",
	"outlook.office365.com":   "Outlook",
	"outlook.live.com":        "Outlook",
	"outlook.cloud.microsoft": "Outlook",
	"app.slack.com":           "Slack",
	"mail.google.com":         "Gmail",
	"calendar.google.com":     "Google Calendar",
	"chat.google.com":         "Google Chat",
	"docs.google.com":         "Google Docs",
	"drive.google.com":        "Google Drive",
	"keep.google.com":         "Google Keep",
	"meet.google.com":         "Google Meet",
	"gemini.google.com":       "Gemini",
	"music.youtube.com":       "YouTube Music",
	"teams.microsoft.com":     "Teams",
	"teams.live.com":          "Teams",
	"teams.cloud.microsoft":   "Teams",
	"m365.cloud.microsoft":    "Microsoft 365",
	"onedrive.live.com":       "OneDrive",
	"github.com":              "GitHub",
	"web.whatsapp.com":        "WhatsApp",
	"web.telegram.org":        "Telegram",
	"chat.openai.com":         "ChatGPT",
	"news.ycombinator.com":    "Hacker News",
	"vscode.dev":              "VS Code",
}

// siteNames are the capitalisations of sites' main names that capitalising
// the first letter doesn't give, by lower-case main name.
var siteNames = map[string]string{
	"github":        "GitHub",
	"gitlab":        "GitLab",
	"youtube":       "YouTube",
	"linkedin":      "LinkedIn",
	"chatgpt":       "ChatGPT",
	"openai":        "OpenAI",
	"whatsapp":      "WhatsApp",
	"paypal":        "PayPal",
	"icloud":        "iCloud",
	"duckduckgo":    "DuckDuckGo",
	"stackoverflow": "Stack Overflow",
	"clickup":       "ClickUp",
	"hubspot":       "HubSpot",
	"sharepoint":    "SharePoint",
	"bbc":           "BBC",
	"imdb":          "IMDb",
	"ebay":          "eBay",
}

// genericPrefixes are host labels that come before a site's name without
// naming anything, as in www.example.com or app.slack.com.
var genericPrefixes = map[string]bool{
	"www":    true,
	"www2":   true,
	"app":    true,
	"apps":   true,
	"web":    true,
	"m":      true,
	"mobile": true,
}

// secondLevels are the second-level labels that, under a two-letter country
// domain, make a two-part suffix such as co.uk or com.au.
var secondLevels = map[string]bool{
	"ac":   true,
	"co":   true,
	"com":  true,
	"edu":  true,
	"go":   true,
	"gov":  true,
	"govt": true,
	"ltd":  true,
	"me":   true,
	"ne":   true,
	"net":  true,
	"or":   true,
	"org":  true,
	"plc":  true,
	"sch":  true,
}

// WebName names a Web item from its address: the site's main name,
// capitalised ("https://app.slack.com/client" is "Slack",
// "https://outlook.office.com/mail/" is "Outlook").
//
// Some sites have fixed names ("mail.google.com" is "Gmail"). Otherwise
// generic first labels such as www, app, web, and m are ignored, and the
// main name is the label before the domain suffix, which may have two parts
// ("www.bbc.co.uk" is "BBC", "news.example.co.uk" is "Example"). An address
// without a scheme is read as https. An IP address or a single-label host
// such as localhost is returned as it is, and an address without a host
// gives "Web page".
func WebName(address string) string {
	host := hostOf(address)
	if host == "" {
		return webPage
	}
	if name, ok := knownSites[host]; ok {
		return name
	}
	if net.ParseIP(host) != nil {
		return host
	}
	labels := strings.Split(host, ".")
	if len(labels) == 1 {
		return host
	}
	suffix := suffixLen(labels)
	for len(labels) > suffix+1 && genericPrefixes[labels[0]] {
		labels = labels[1:]
	}
	if name, ok := knownSites[strings.Join(labels, ".")]; ok {
		return name
	}
	main := labels[0]
	if len(labels) > suffix {
		main = labels[len(labels)-suffix-1]
	}
	return siteName(main)
}

// hostOf returns an address's host in lower case, without a port or a final
// dot, or "" if it has none.
func hostOf(address string) string {
	address = strings.TrimSpace(address)
	if address == "" {
		return ""
	}
	if !strings.Contains(address, "://") {
		address = "https://" + address
	}
	u, err := url.Parse(address)
	if err != nil {
		return ""
	}
	return strings.TrimSuffix(strings.ToLower(u.Hostname()), ".")
}

// suffixLen returns how many labels at the end of a host are its domain
// suffix: 2 for suffixes such as co.uk, otherwise 1.
func suffixLen(labels []string) int {
	n := len(labels)
	if n >= 3 && len(labels[n-1]) == 2 && secondLevels[labels[n-2]] {
		return 2
	}
	return 1
}

// siteName capitalises a site's main name.
func siteName(main string) string {
	if name, ok := siteNames[main]; ok {
		return name
	}
	r, size := utf8.DecodeRuneInString(main)
	if size == 0 {
		return webPage
	}
	return strings.ToUpper(string(r)) + main[size:]
}
