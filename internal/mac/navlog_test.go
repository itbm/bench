package mac

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"
)

func TestLogTarget(t *testing.T) {
	tests := []struct{ address, scheme, host string }{
		{"https://app.slack.com/client/T1/C2?thread=abc#m", "https", "app.slack.com"},
		{"https://sam:hunter2@App.Slack.COM./x", "https", "app.slack.com"},
		{"https://localhost:8443/b/hop", "https", "localhost:8443"},
		{"https://127.0.0.1:9443/a/away?token=s3cret", "https", "127.0.0.1:9443"},
		{"http://[::1]:5000/?tkn=abc", "http", "[::1]:5000"},
		{"blob:https://evil.example/1234", "blob", "evil.example"},
		{"mailto:sam@example.com?subject=Pay", "mailto", ""},
		{"tel:+441234567890", "tel", ""},
		{"data:text/html,<p>hi", "data", ""},
		{"about:blank", "about", ""},
		{"", "", ""},
		{"not an address", "", ""},
	}
	for _, tt := range tests {
		scheme, host := logTarget(tt.address)
		if scheme != tt.scheme || host != tt.host {
			t.Errorf("logTarget(%q) = %q, %q, want %q, %q", tt.address, scheme, host, tt.scheme, tt.host)
		}
	}
}

func TestLogDecision(t *testing.T) {
	var buf bytes.Buffer
	log := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
	logDecision(log, "web navigation", "w1", "https://127.0.0.1:9443/a/away?token=s3cret#frag", NavigateInBrowser.String(),
		"kind", navOther.String(), "from", fromFrame.String())
	logDecision(log, "web popup", "w1", "mailto:sam@example.com", PopupBrowser.String())
	logs := buf.String()
	for _, want := range []string{
		`msg="web navigation" item=w1 decision=in-browser scheme=https host=127.0.0.1:9443 kind=other from=frame`,
		`msg="web popup" item=w1 decision=in-browser scheme=mailto host=""`,
	} {
		if !strings.Contains(logs, want) {
			t.Errorf("the log lacks %q:\n%s", want, logs)
		}
	}
	for _, secret := range []string{"away", "s3cret", "frag", "sam@"} {
		if strings.Contains(logs, secret) {
			t.Errorf("the log has %q:\n%s", secret, logs)
		}
	}
	logDecision(nil, "web navigation", "w1", "https://example.com/", "in-view") // Logs nothing, without panicking.
}

func TestDecisionNames(t *testing.T) {
	names := map[string]string{
		NavigateInView.String():    "in-view",
		NavigateInBrowser.String(): "in-browser",
		NavigateCancel.String():    "cancel",
		NavigateAsk.String():       "ask",
		Navigation(9).String():     "navigation-9",
		PopupWindow.String():       "window",
		PopupBrowser.String():      "in-browser",
		PopupCancel.String():       "cancel",
		PopupAsk.String():          "ask",
		Popup(9).String():          "popup-9",
		navOther.String():          "other",
		navLink.String():           "link",
		navHistory.String():        "history",
		navKind(9).String():        "kind-9",
		fromBench.String():         "bench",
		fromNone.String():          "none",
		fromPage.String():          "page",
		fromFrame.String():         "frame",
		fromWindow.String():        "window",
		navSource(9).String():      "source-9",
	}
	for got, want := range names {
		if got != want {
			t.Errorf("got the name %q, want %q", got, want)
		}
	}
}
