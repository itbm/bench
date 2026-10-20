package apps

import "testing"

func TestWebName(t *testing.T) {
	tests := []struct{ in, want string }{
		// The default config and the brief.
		{"https://app.slack.com/client", "Slack"},
		{"https://outlook.office.com/mail/", "Outlook"},
		{"https://mail.google.com/mail/u/0/", "Gmail"},
		{"https://github.com/itbm/bench", "GitHub"},

		// Microsoft's newer addresses.
		{"https://outlook.cloud.microsoft/mail/", "Outlook"},
		{"https://outlook.office365.com/mail/", "Outlook"},
		{"https://teams.cloud.microsoft/", "Teams"},
		{"https://m365.cloud.microsoft/", "Microsoft 365"},

		// Generic first labels are ignored.
		{"https://www.github.com/", "GitHub"},
		{"https://www.example.com/", "Example"},
		{"https://m.example.com", "Example"},
		{"https://web.example.com", "Example"},
		{"https://app.www.example.com", "Example"},
		{"https://docs.example.com", "Example"},
		{"https://app.com", "App"},

		// Two-part suffixes.
		{"https://www.bbc.co.uk/news", "BBC"},
		{"https://news.example.co.uk", "Example"},
		{"https://shop.example.com.au", "Example"},
		{"https://example.co.jp", "Example"},
		{"https://example.org.uk", "Example"},
		{"https://example.uk", "Example"},
		{"https://example.io", "Example"},

		// Known capitalisations.
		{"https://www.youtube.com/watch?v=1", "YouTube"},
		{"https://gitlab.com/group/project", "GitLab"},
		{"https://www.linkedin.com/feed/", "LinkedIn"},
		{"https://chatgpt.com/", "ChatGPT"},
		{"https://gist.github.com/someone", "GitHub"},

		// Addresses in other forms.
		{"example.com", "Example"},
		{"  HTTPS://WWW.EXAMPLE.COM  ", "Example"},
		{"http://example.com:8080/path", "Example"},
		{"https://example.com./", "Example"},
		{"https://user:secret@example.com/", "Example"},
		{"https://linear.app/team", "Linear"},
		{"https://calendar.google.com/calendar/r", "Google Calendar"},
		{"github.com/login?return_to=https://github.com/org", "GitHub"},

		// No site name to find.
		{"http://localhost:8080/?tkn=abc", "localhost"},
		{"http://127.0.0.1:3000", "127.0.0.1"},
		{"http://[::1]:3000/", "::1"},
		{"", "Web page"},
		{"   ", "Web page"},
		{"https://", "Web page"},
		{"about:blank", "Web page"},
		{"https://exa mple.com", "Web page"},
		{"https://example..com", "Web page"},
	}
	for _, tt := range tests {
		if got := WebName(tt.in); got != tt.want {
			t.Errorf("WebName(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestHasScheme(t *testing.T) {
	tests := []struct {
		in   string
		want bool
	}{
		{"https://example.com", true},
		{"HTTPS://example.com", true},
		{"git+ssh://example.com", true},
		{"x-1.a://example.com", true},
		{"example.com", false},
		{"github.com/login?return_to=https://github.com/org", false},
		{"example.com:8080/a://b", false},
		{"://example.com", false},
		{"1http://example.com", false},
		{"about:blank", false},
		{"", false},
	}
	for _, tt := range tests {
		if got := HasScheme(tt.in); got != tt.want {
			t.Errorf("HasScheme(%q) = %v, want %v", tt.in, got, tt.want)
		}
	}
}

func TestSameWebHost(t *testing.T) {
	tests := []struct {
		a, b string
		want bool
	}{
		{"https://squoosh.app/?utm_medium=PWA&utm_source=launcher", "https://squoosh.app", true},
		{"https://squoosh.app/", "squoosh.app/editor", true},
		// Case, a leading www., a final dot, and default ports don't count.
		{"https://WWW.Example.COM/a", "https://example.com/b", true},
		{"https://example.com./", "https://example.com", true},
		{"https://example.com:443/", "https://example.com", true},
		{"http://example.com:80/", "https://example.com", true},
		{"http://example.com/", "https://example.com", true},
		{"https://user:secret@example.com/", "https://example.com", true},
		// Hosts, not domains.
		{"https://mail.google.com/mail/u/0/", "https://docs.google.com/document/", false},
		{"https://mail.google.com/", "https://google.com/", false},
		{"https://app.slack.com/client", "https://slack.com", false},
		{"https://www.example.com", "https://www2.example.com", false},
		// Other ports.
		{"https://example.com:8443/", "https://example.com", false},
		{"http://example.com:443/", "https://example.com", false},
		{"http://localhost:3000/", "http://localhost:3000/app", true},
		{"http://localhost:3000/", "http://localhost:3001/", false},
		{"http://[::1]:3000/", "http://[::1]:3000/", true},
		// No host, or nothing to read.
		{"", "", false},
		{"", "https://example.com", false},
		{"https://example.com", "  ", false},
		{"https://", "https://", false},
		{"https://exa mple.com", "https://exa mple.com", false},
		{"about:blank", "about:blank", false},
	}
	for _, tt := range tests {
		if got := SameWebHost(tt.a, tt.b); got != tt.want {
			t.Errorf("SameWebHost(%q, %q) = %v, want %v", tt.a, tt.b, got, tt.want)
		}
		if got := SameWebHost(tt.b, tt.a); got != tt.want {
			t.Errorf("SameWebHost(%q, %q) = %v, want %v", tt.b, tt.a, got, tt.want)
		}
	}
}
