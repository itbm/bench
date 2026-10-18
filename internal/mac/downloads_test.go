package mac

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDecideResponse(t *testing.T) {
	const attachments = "https://attachments.office.net/owa/x/service.svc/s/GetFileAttachment?id=1"
	tests := []struct {
		name        string
		item        string
		response    string
		mainFrame   bool
		canShow     bool
		disposition string
		want        Response
	}{
		{"a page", slack, "https://app.slack.com/client", true, true, "", ResponseShow},
		{"an inline PDF", slack, "https://files.slack.com/x.pdf", true, true, "inline; filename=report.pdf", ResponseShow},
		{"an attachment", outlook, attachments, true, true, "attachment; filename=report.pdf", ResponseDownload},
		{"an attachment in capitals", outlook, attachments, true, true, `ATTACHMENT; filename="report.pdf"`, ResponseDownload},
		{"an attachment with spaces", outlook, attachments, true, true, "  attachment ;filename*=UTF-8''r%C3%A9sum%C3%A9.pdf", ResponseDownload},
		{"a bare attachment", outlook, attachments, true, true, "attachment", ResponseDownload},
		{"something WebKit can't show", slack, "https://files.slack.com/x.zip", true, false, "", ResponseDownload},
		{"something WebKit can't show, inline", slack, "https://files.slack.com/x.zip", true, false, "inline", ResponseDownload},
		{"a malformed header", outlook, attachments, true, true, "attachment; filename=\"unterminated", ResponseDownload},
		{"a type that only starts like attachment", outlook, attachments, true, true, "attachments", ResponseShow},
		{"another site, in the main frame", slack, "https://example.com/x.zip", true, false, "", ResponseDownload},
		{"a frame on the item's site", outlook, attachments, false, true, "attachment", ResponseDownload},
		{"a frame from another site", slack, "https://ads.example/x.dmg", false, false, "attachment", ResponseShow},
		{"a frame from another site, can't show", slack, "https://ads.example/x.dmg", false, false, "", ResponseShow},
		{"a frame's data address", slack, "data:application/octet-stream,x", false, false, "", ResponseShow},
		// Links with a download attribute are judged by their frame's origin
		// (benchDownloadLinkPolicy).
		{"a download link in a frame on the item's site", outlook, "https://outlook.office.com", false, false, "attachment", ResponseDownload},
		{"a download link in a frame from another site", slack, "https://ads.example", false, false, "attachment", ResponseShow},
		{"a download link in a frame with no origin", slack, "", false, false, "attachment", ResponseShow},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := DecideResponse(tt.item, tt.response, tt.mainFrame, tt.canShow, tt.disposition); got != tt.want {
				t.Errorf("DecideResponse(%q, %q, %v, %v, %q) = %v, want %v", tt.item, tt.response, tt.mainFrame, tt.canShow, tt.disposition, got, tt.want)
			}
		})
	}
}

func TestDownloadName(t *testing.T) {
	tests := []struct{ suggested, want string }{
		{"report.pdf", "report.pdf"},
		{"", "Download"},
		{"   ", "Download"},
		{"../../.ssh/authorized_keys", "authorized_keys"},
		{"a/b/c.txt", "c.txt"},
		{".hidden", "hidden"},
		{"...", "Download"},
		{"weird:name.txt", "weird-name.txt"},
		{"tab\tand\nnewline.txt", "tab and newline.txt"},
		{"résumé.pdf", "résumé.pdf"},
	}
	for _, tt := range tests {
		if got := downloadName(tt.suggested); got != tt.want {
			t.Errorf("downloadName(%q) = %q, want %q", tt.suggested, got, tt.want)
		}
	}
	long := strings.Repeat("é", 300) + ".pdf"
	if got := downloadName(long); len(got) > 255 || !strings.HasSuffix(got, ".pdf") {
		t.Errorf("downloadName(300 runes) = %d bytes, %q…", len(got), got[:10])
	}
}

func TestDownloadPath(t *testing.T) {
	dir := t.TempDir()
	var d downloads
	first := d.path(dir, "report.pdf")
	if first != filepath.Join(dir, "report.pdf") {
		t.Fatalf("first = %q", first)
	}
	// A second download of the same name, before the first has written its
	// file, gets a name of its own.
	second := d.path(dir, "report.pdf")
	if second != filepath.Join(dir, "report 2.pdf") {
		t.Errorf("second = %q, want report 2.pdf", second)
	}
	// A file already in the folder is never overwritten.
	if err := os.WriteFile(filepath.Join(dir, "notes.txt"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if got := d.path(dir, "notes.txt"); got != filepath.Join(dir, "notes 2.txt") {
		t.Errorf("existing file: %q, want notes 2.txt", got)
	}
	if got := d.path(dir, "archive.tar.gz"); got != filepath.Join(dir, "archive.tar.gz") {
		t.Errorf("archive: %q", got)
	}
	if got := d.path(dir, "archive.tar.gz"); got != filepath.Join(dir, "archive 2.tar.gz") {
		t.Errorf("second archive: %q, want archive 2.tar.gz", got)
	}
	if got := d.path(dir, "README"); got != filepath.Join(dir, "README") {
		t.Errorf("no extension: %q", got)
	}
	if got := d.path(dir, "README"); got != filepath.Join(dir, "README 2") {
		t.Errorf("second, no extension: %q", got)
	}
	if got := d.path("", "x.txt"); got != "" {
		t.Errorf("no folder: %q, want \"\"", got)
	}
}

func TestDownloadPathReleased(t *testing.T) {
	dir := t.TempDir()
	var d downloads
	first := d.path(dir, "report.pdf")
	// The download failed, or was cancelled, before it wrote its file: the
	// next one of the same name gets the name, not "report 2.pdf".
	d.release(first)
	if got := d.path(dir, "report.pdf"); got != first {
		t.Errorf("after a failed download: %q, want %q", got, first)
	}
	// One that finished has its file, so the next one still gets a name of
	// its own.
	if err := os.WriteFile(first, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	d.release(first)
	if got := d.path(dir, "report.pdf"); got != filepath.Join(dir, "report 2.pdf") {
		t.Errorf("after a finished download: %q, want report 2.pdf", got)
	}
	d.release("")
	d.release(filepath.Join(dir, "never given out.txt"))
}
