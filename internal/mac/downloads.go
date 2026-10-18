package mac

import (
	"mime"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"unicode"
	"unicode/utf8"
)

// Response is what a web view does with a response to a navigation.
type Response int

const (
	// ResponseShow shows it.
	ResponseShow Response = iota
	// ResponseDownload saves it to the Downloads folder.
	ResponseDownload
)

// DecideResponse says whether a Web item's web view (or one of its popups)
// shows a response to a navigation or downloads it. itemURL is the item's
// own address, and responseURL the response's.
//
// It downloads what WebKit can't show (canShow is false), such as a zip
// file, and what the server marks as an attachment (Content-Disposition:
// attachment), such as a file attached to a message: in the main frame,
// and in frames on the item's site, which some web apps download through.
// A frame from another site, such as an advert, can't start a download:
// WebKit shows what it can there, and nothing else.
func DecideResponse(itemURL, responseURL string, mainFrame, canShow bool, contentDisposition string) Response {
	if canShow && !isAttachment(contentDisposition) {
		return ResponseShow
	}
	if mainFrame {
		return ResponseDownload
	}
	item, ok := parseAddress(itemURL)
	if !ok {
		return ResponseShow
	}
	response, ok := parseAddress(responseURL)
	if !ok || !isWeb(response) || !sameSite(item, response) {
		return ResponseShow
	}
	return ResponseDownload
}

// isAttachment reports whether a Content-Disposition header's type is
// attachment. A header that doesn't parse is judged by its first word.
func isAttachment(header string) bool {
	kind, _, err := mime.ParseMediaType(header)
	if err != nil {
		kind, _, _ = strings.Cut(header, ";")
	}
	return strings.EqualFold(strings.TrimSpace(kind), "attachment")
}

// downloads chooses where downloads go. Its methods are safe for concurrent
// use.
type downloads struct {
	mu sync.Mutex
	// taken holds the paths given out to downloads in progress, which write
	// their files only as they go, so that two at once don't get the same
	// one.
	taken map[string]bool
}

// webDownloads is the process's downloads.
var webDownloads downloads

// maxNameBytes is the longest file name macOS's file systems take.
const maxNameBytes = 255

// path returns where to save a download that the server suggested calling
// suggested: in dir, under a safe version of that name (see downloadName),
// with " 2", " 3", and so on before its extension if a file of that name
// exists or another download has it. It returns "" for no dir.
func (d *downloads) path(dir, suggested string) string {
	if dir == "" {
		return ""
	}
	name := downloadName(suggested)
	stem, ext := splitExt(name)
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.taken == nil {
		d.taken = map[string]bool{}
	}
	for n := 1; ; n++ {
		candidate := name
		if n > 1 {
			suffix := " " + strconv.Itoa(n)
			candidate = truncateName(stem, maxNameBytes-len(suffix)-len(ext)) + suffix + ext
		}
		full := filepath.Join(dir, candidate)
		if d.taken[full] {
			continue
		}
		if _, err := os.Lstat(full); err == nil || !os.IsNotExist(err) {
			continue
		}
		d.taken[full] = true
		return full
	}
}

// release frees a path that path gave out, once its download has finished
// or failed: then the file is there, which keeps the name taken, or it
// isn't, and the name is free again.
func (d *downloads) release(path string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	delete(d.taken, path)
}

// downloadName makes a file name a page suggested safe to use: just its last
// path element, without control characters, colons (which Finder shows as
// slashes), or leading dots (which would hide it), and short enough for the
// file system. An empty name becomes "Download".
func downloadName(suggested string) string {
	suggested = strings.ReplaceAll(suggested, "\\", "/")
	if i := strings.LastIndex(suggested, "/"); i >= 0 {
		suggested = suggested[i+1:]
	}
	var b strings.Builder
	for _, r := range suggested {
		switch {
		case r == ':':
			b.WriteRune('-')
		case unicode.IsControl(r):
			b.WriteRune(' ')
		case r == utf8.RuneError:
		default:
			b.WriteRune(r)
		}
	}
	name := strings.TrimSpace(strings.TrimLeft(strings.TrimSpace(b.String()), "."))
	if name == "" {
		return "Download"
	}
	if len(name) > maxNameBytes {
		stem, ext := splitExt(name)
		if len(ext) > 32 {
			stem, ext = name, ""
		}
		name = truncateName(stem, maxNameBytes-len(ext)) + ext
	}
	return name
}

// splitExt splits a file name into its stem and extension, keeping
// .tar.gz and the like together.
func splitExt(name string) (stem, ext string) {
	lower := strings.ToLower(name)
	for _, double := range []string{".tar.gz", ".tar.bz2", ".tar.xz", ".tar.zst"} {
		if strings.HasSuffix(lower, double) && len(name) > len(double) {
			return name[:len(name)-len(double)], name[len(name)-len(double):]
		}
	}
	ext = filepath.Ext(name)
	if ext == name {
		return name, ""
	}
	return name[:len(name)-len(ext)], ext
}

// truncateName shortens s to at most n bytes without splitting a character.
func truncateName(s string, n int) string {
	if len(s) <= n {
		return s
	}
	for n > 0 && !utf8.RuneStart(s[n]) {
		n--
	}
	return s[:n]
}
