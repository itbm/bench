package mac

import (
	"os"
	"path/filepath"
	"strconv"
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

// TestDownloadNameHidden checks names that would still be hidden, or would
// disguise their extension, after one pass of trimming.
func TestDownloadNameHidden(t *testing.T) {
	tests := []struct{ suggested, want string }{
		{". ..", "Download"},
		{" . . .", "Download"},
		{". .x", "x"},
		{" . .config", "config"},
		{"invoice\u202efdp.command", "invoicefdp.command"},
		{"\u200b.hidden", "hidden"},
		{"a\u2066b\u2069.txt", "ab.txt"},
		{"ends with dots..", "ends with dots.."},
	}
	for _, tt := range tests {
		if got := downloadName(tt.suggested); got != tt.want {
			t.Errorf("downloadName(%q) = %q, want %q", tt.suggested, got, tt.want)
		}
	}
}

// TestDownloadPathLongExtension checks that a name made almost all of
// extension can be numbered: its stem had no room left, which panicked.
func TestDownloadPathLongExtension(t *testing.T) {
	dir := t.TempDir()
	var d downloads
	name := "a." + strings.Repeat("b", 253)
	first := d.path(dir, name)
	if first != filepath.Join(dir, name) {
		t.Fatalf("first = %q", first)
	}
	for range 3 {
		got := d.path(dir, name)
		base := filepath.Base(got)
		if got == "" || got == first || len(base) > maxNameBytes {
			t.Errorf("another = %q (%d bytes)", base, len(base))
		}
	}
	if got := truncateName("abc", -1); got != "" {
		t.Errorf("truncateName(\"abc\", -1) = %q, want \"\"", got)
	}
}

// TestDownloadPathUnreadable checks that a folder whose files can't be
// looked up cancels the download rather than trying names for ever.
func TestDownloadPathUnreadable(t *testing.T) {
	notADir := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(notADir, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	var d downloads
	if got := d.path(notADir, "report.pdf"); got != "" {
		t.Errorf("path in a file = %q, want \"\"", got)
	}
}

// TestDownloadPathGivesUp checks that path stops after maxNumber names.
func TestDownloadPathGivesUp(t *testing.T) {
	dir := t.TempDir()
	d := downloads{taken: map[string]bool{pathKey(filepath.Join(dir, "x.txt")): true}}
	for n := 2; n < maxNumber; n++ {
		d.taken[pathKey(filepath.Join(dir, "x "+strconv.Itoa(n)+".txt"))] = true
	}
	if got := d.path(dir, "x.txt"); got != filepath.Join(dir, "x "+strconv.Itoa(maxNumber)+".txt") {
		t.Fatalf("last name = %q", got)
	}
	if got := d.path(dir, "x.txt"); got != "" {
		t.Errorf("name %d = %q, want \"\"", maxNumber+1, got)
	}
}

// TestDownloadPathIgnoresCase checks that two downloads whose names differ
// only in case, which APFS takes for one file, get files of their own.
func TestDownloadPathIgnoresCase(t *testing.T) {
	dir := t.TempDir()
	var d downloads
	first := d.path(dir, "Invoice.PDF")
	second := d.path(dir, "invoice.pdf")
	if second != filepath.Join(dir, "invoice 2.pdf") {
		t.Errorf("second = %q, want invoice 2.pdf", second)
	}
	if got := d.path(dir, "ÉTÉ.txt"); got != filepath.Join(dir, "ÉTÉ.txt") {
		t.Errorf("ÉTÉ.txt: %q", got)
	}
	if got := d.path(dir, "été.txt"); got != filepath.Join(dir, "été 2.txt") {
		t.Errorf("été.txt: %q, want été 2.txt", got)
	}
	// Releasing a path frees it whatever its case.
	d.release(first)
	if got := d.path(dir, "INVOICE.pdf"); got != filepath.Join(dir, "INVOICE.pdf") {
		t.Errorf("after release: %q, want INVOICE.pdf", got)
	}
}

func TestDownloadLimiter(t *testing.T) {
	var l downloadLimiter
	const at = 1000.0
	if l.allow("w1", 0, at) {
		t.Error("allowed a download with no user event")
	}
	for i := range downloadsPerEvent {
		if !l.allow("w1", at, at+float64(i)) {
			t.Fatalf("download %d after a click refused", i+1)
		}
	}
	if l.allow("w1", at, at+10) {
		t.Errorf("allowed more than %d downloads for one click", downloadsPerEvent)
	}
	// Another item has a budget of its own.
	if !l.allow("w2", at, at+1) {
		t.Error("another item's download refused")
	}
	// A new click allows more, but not long after it.
	next := at + 100
	if !l.allow("w1", next, next+1) {
		t.Error("download after a new click refused")
	}
	if l.allow("w1", next, next+downloadWindow.Seconds()+1) {
		t.Error("allowed a download long after the click")
	}
	// An older event, from a popup, shares the newest one's budget.
	for range downloadsPerEvent - 1 {
		l.allow("w1", next-50, next+2)
	}
	if l.allow("w1", next-50, next+3) {
		t.Error("an older event reset the budget")
	}
	// A clock that went backwards allows nothing.
	if l.allow("w3", at, at-1) {
		t.Error("allowed a download before the click")
	}
}
