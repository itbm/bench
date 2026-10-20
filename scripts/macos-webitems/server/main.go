// Command server serves the test pages for scripts/macos-webitems/run.sh,
// which checks on a GitHub macOS runner how Bench's Web items handle pages
// that try to leave the item's site or to flood it with dialogs.
//
// Usage:
//
//	server -cert <cert.pem> -key <key.pem> -log <requests.jsonl> [-item-port 8443] [-other-port 9443]
//
// It serves the same pages over https on two sites: the items' own, at
// https://localhost:8443, and another, at https://127.0.0.1:9443. Bench
// counts each local host and port as a site of its own, and so does WebKit
// for the two hosts, so a frame from the other site is a cross-site frame.
// The certificate must name both hosts.
//
// It logs every request, and every failed TLS handshake, as a line of JSON
// (see entry), so that the script can tell which pages the web view
// fetched, and which the default browser opened.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"
)

func main() {
	cert := flag.String("cert", "", "the certificate, in PEM, for localhost and 127.0.0.1")
	key := flag.String("key", "", "the certificate's private key, in PEM")
	logPath := flag.String("log", "", "the file to add a line of JSON to for each request")
	itemPort := flag.Int("item-port", 8443, "the port of the items' own site, on localhost")
	otherPort := flag.Int("other-port", 9443, "the port of the other site, on 127.0.0.1")
	flag.Parse()
	if *cert == "" || *key == "" || *logPath == "" {
		fmt.Fprintln(os.Stderr, "server: -cert, -key, and -log are needed")
		os.Exit(2)
	}
	f, err := os.OpenFile(*logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		fmt.Fprintln(os.Stderr, "server:", err)
		os.Exit(1)
	}
	defer f.Close()
	requests := &requestLog{w: f, now: time.Now}
	other := fmt.Sprintf("https://127.0.0.1:%d", *otherPort)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	sites := []struct {
		name  string
		addrs []string
	}{
		// localhost can be either loopback address; ::1 may not be there.
		{"item", []string{fmt.Sprintf("127.0.0.1:%d", *itemPort), fmt.Sprintf("[::1]:%d", *itemPort)}},
		{"other", []string{fmt.Sprintf("127.0.0.1:%d", *otherPort)}},
	}
	failed := make(chan error, 3)
	var servers []*http.Server
	for _, site := range sites {
		srv := &http.Server{
			Handler:           newHandler(site.name, other, requests),
			ErrorLog:          log.New(requests.errors(site.name), "", 0),
			ReadHeaderTimeout: 10 * time.Second,
		}
		servers = append(servers, srv)
		for i, addr := range site.addrs {
			ln, err := net.Listen("tcp", addr)
			if err != nil && i > 0 {
				fmt.Fprintf(os.Stderr, "server: not listening on %s: %v\n", addr, err)
				continue
			}
			if err != nil {
				fmt.Fprintln(os.Stderr, "server:", err)
				os.Exit(1)
			}
			go func() {
				if err := srv.ServeTLS(ln, *cert, *key); err != http.ErrServerClosed {
					failed <- err
				}
			}()
		}
	}
	fmt.Printf("server: serving the items' site on https://localhost:%d and the other site on %s\n", *itemPort, other)
	select {
	case <-ctx.Done():
	case err := <-failed:
		fmt.Fprintln(os.Stderr, "server:", err)
		os.Exit(1)
	}
	for _, srv := range servers {
		shutdown, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		_ = srv.Shutdown(shutdown)
		cancel()
	}
}

// entry is a line of the request log.
type entry struct {
	// T is when the request came, in seconds since 1970.
	T float64 `json:"t"`
	// Site is "item" or "other".
	Site   string `json:"site"`
	Method string `json:"method,omitempty"`
	Path   string `json:"path,omitempty"`
	Query  string `json:"query,omitempty"`
	// FetchSite and FetchDest are the request's Sec-Fetch-Site and
	// Sec-Fetch-Dest headers, and Referer its Referer.
	FetchSite string `json:"fetchSite,omitempty"`
	FetchDest string `json:"fetchDest,omitempty"`
	Referer   string `json:"referer,omitempty"`
	// Fresh is whether no page asked for the request (see isFresh).
	Fresh     bool   `json:"fresh"`
	UserAgent string `json:"userAgent,omitempty"`
	// Error is the server's error, such as a failed TLS handshake, for a
	// line that isn't a request.
	Error string `json:"error,omitempty"`
}

// isFresh reports whether no page asked for a request: one that the
// browser makes by itself, such as for an address that another app, like
// Bench, opened with it, or that Bench's web view makes for the item's own
// address. Safari and Chrome say so with Sec-Fetch-Site: none; without that
// header, a request with no Referer is taken for one. A page's navigation,
// such as a frame setting top.location, comes with Sec-Fetch-Site and a
// Referer.
func isFresh(r *http.Request) bool {
	site := r.Header.Get("Sec-Fetch-Site")
	return site == "none" || (site == "" && r.Referer() == "")
}

// requestLog writes entries as lines of JSON. Its methods are safe for
// concurrent use.
type requestLog struct {
	mu  sync.Mutex
	w   io.Writer
	now func() time.Time
}

// add writes an entry, timed now.
func (l *requestLog) add(e entry) {
	l.mu.Lock()
	defer l.mu.Unlock()
	e.T = float64(l.now().UnixMilli()) / 1000
	line, err := json.Marshal(e)
	if err != nil {
		return
	}
	_, _ = l.w.Write(append(line, '\n'))
}

// errors returns a writer for an http.Server's error log, which adds each
// line it gets as an entry for a site.
func (l *requestLog) errors(site string) io.Writer {
	return errorWriter{l: l, site: site}
}

type errorWriter struct {
	l    *requestLog
	site string
}

func (w errorWriter) Write(p []byte) (int, error) {
	w.l.add(entry{Site: w.site, Error: strings.TrimSpace(string(p))})
	return len(p), nil
}

// newHandler serves the test pages for a site ("item" or "other"), and
// logs each request. other is the other site's origin, such as
// https://127.0.0.1:9443.
func newHandler(site, other string, l *requestLog) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		l.add(entry{
			Site:      site,
			Method:    r.Method,
			Path:      r.URL.Path,
			Query:     r.URL.RawQuery,
			FetchSite: r.Header.Get("Sec-Fetch-Site"),
			FetchDest: r.Header.Get("Sec-Fetch-Dest"),
			Referer:   r.Referer(),
			Fresh:     isFresh(r),
			UserAgent: r.UserAgent(),
		})
		// Every load, and each reload, reaches the server, and the log.
		w.Header().Set("Cache-Control", "no-store")
		switch r.URL.Path {
		case "/ready", "/beacon":
			w.WriteHeader(http.StatusNoContent)
			return
		}
		page, ok := pages[r.URL.Path]
		if !ok {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = io.WriteString(w, strings.ReplaceAll(page, "OTHER", other))
	})
}

// style is every page's style sheet: a page that fills the web view, so
// that a click in the middle of the pane lands on what the page wants
// clicked.
const style = `<style>
html, body { margin: 0; height: 100%; font: 20px -apple-system, sans-serif; }
body { display: flex; align-items: center; justify-content: center; text-align: center; }
iframe { position: fixed; inset: 0; width: 100%; height: 100%; border: 0; }
a.whole { position: fixed; inset: 0; display: flex; align-items: center; justify-content: center; }
</style>
`

// pages are the test pages, by path. OTHER stands for the other site's
// origin. The scenarios are run.sh's: (a) a cross-site frame that sets
// top.location just after the user clicks in it; (b) a clicked link to a
// page on the item's site that sends the user to the other site with
// location.replace; (c) an alert whose text is one character with 2,000,000
// combining marks, after 2,000 others; and (d) a page that brings up an
// alert, then reloads itself, again and again. The pages' scripts report
// what they did with beacons, which the log shows.
var pages = map[string]string{
	"/ping": `<!doctype html><meta charset="utf-8"><title>Ping</title>` + style +
		`<p>The default browser can open the test server's pages.</p>`,

	"/a/": `<!doctype html><meta charset="utf-8"><title>Frame</title>` + style +
		`<iframe src="OTHER/a/frame"></iframe>`,
	"/a/frame": `<!doctype html><meta charset="utf-8"><title>Embedded</title>` + style +
		`<body style="background: #fde4ec; cursor: pointer">
<p>Content from another site. Click anywhere.</p>
<script>
document.addEventListener("click", () => {
	navigator.sendBeacon("/beacon?e=a-click");
	setTimeout(() => { top.location.href = "OTHER/a/away"; }, 500);
}, { once: true });
</script>`,
	"/a/away": `<!doctype html><meta charset="utf-8"><title>Another site</title>` + style +
		`<p>This page is on another site, which the frame sent the item to.</p>`,

	"/b/": `<!doctype html><meta charset="utf-8"><title>Redirector</title>` + style +
		`<a class="whole" href="/b/hop">Click to follow a link through a redirector</a>`,
	"/b/hop": `<!doctype html><meta charset="utf-8"><title>Redirecting</title>` + style +
		`<p>Going on…</p>
<script>
location.replace("OTHER/b/away");
</script>`,
	"/b/away": `<!doctype html><meta charset="utf-8"><title>Another site</title>` + style +
		`<p>This page is on another site, which the redirector sent the item to.</p>`,

	"/c/": `<!doctype html><meta charset="utf-8"><title>Long alert</title>` + style +
		`<p>A page that brings up a very long alert.</p>
<script>
addEventListener("load", () => {
	const text = "x".repeat(1999) + "a" + "\u0301".repeat(2000000);
	navigator.sendBeacon("/beacon?e=c-alert&length=" + text.length);
	const started = Date.now();
	alert(text);
	navigator.sendBeacon("/beacon?e=c-answered&ms=" + (Date.now() - started));
});
</script>`,

	"/d/": `<!doctype html><meta charset="utf-8"><title>Alert loop</title>` + style +
		`<p>A page that brings up an alert, then reloads itself.</p>
<script>
addEventListener("load", () => {
	const n = Number(sessionStorage.getItem("n") || 0) + 1;
	sessionStorage.setItem("n", String(n));
	navigator.sendBeacon("/beacon?e=d-alert&n=" + n);
	const started = Date.now();
	alert("Alert " + n + ", from a page that reloads itself");
	navigator.sendBeacon("/beacon?e=d-answered&n=" + n + "&ms=" + (Date.now() - started));
	setTimeout(() => location.reload(), 250);
});
</script>`,
}
