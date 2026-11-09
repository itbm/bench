// Package privacy has no code. Its tests check that Bench connects only
// where README.md's "Permissions and privacy" says it does. They read
// Bench's Go, Objective-C, C, C++, and frontend files, whatever their
// build tags, so the Mac's code is checked on Linux too. The check is
// static. It looks for the known ways to connect, listen, or look a name
// up, which the lists below name. The tests fail for each use of one that
// allowed doesn't list, with the file and line. They also fail for an
// entry in allowed that the code no longer uses, so that the list stays
// exact.
//
// The tests read the code. They don't run it. So they can't see what other
// programs do, such as the user's shell or `code serve-web`, or what modules
// from outside Bench do, such as Wails' server for the browser preview.
// They also can't see a client that one file makes and another file uses
// without the name of its type. And they can't see a way to connect that
// the lists don't name, or one in a file of a kind that they don't read.
// Nor can they see a name that the code spells another way, such as
// window["fetch"], or an address that the code makes as it runs, such as
// an image's src that it joins from parts.
package privacy

import (
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"maps"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
)

// root is the repository's root, from this package's folder.
var root = filepath.Join("..", "..")

// notBench are the folders at the root that Bench.app and the browser
// preview don't include, so the tests don't read them, each with the
// reason. The tests read every other folder, so new code is checked
// wherever it goes. They also skip testdata, node_modules, and folders
// whose names start with a dot or an underscore, which go test skips too.
var notBench = map[string]string{
	"docs":    "The documentation. Its prototype page loads web fonts from Google.",
	"e2e":     "Tests, which drive the browser preview through its port on localhost.",
	"scripts": "Scripts for CI, releases, and the checks on a macOS runner, such as the web server for the web items check.",
}

// connects lists, by import path, the names that open a connection or a
// listener, look a name up, or make or hold a client that can. A file that
// uses one of them must have it in allowed. Names that only parse or
// serve, such as net.ParseIP, net/url, http.Handler, or http.Error, aren't
// listed. A client that a function is given has a type, *http.Client,
// which names http.Client, so the file that takes it counts too. "C" is
// cgo's name for the C functions that a Go file calls.
var connects = map[string][]string{
	"net": {
		"Dial", "DialTimeout", "DialTCP", "DialUDP", "DialIP", "DialUnix", "Dialer",
		"Listen", "ListenConfig", "ListenTCP", "ListenUDP", "ListenIP", "ListenUnix",
		"ListenUnixgram", "ListenPacket", "ListenMulticastUDP",
		"FileConn", "FileListener", "FilePacketConn",
		"LookupHost", "LookupIP", "LookupNetIP", "LookupAddr", "LookupCNAME",
		"LookupMX", "LookupNS", "LookupPort", "LookupSRV", "LookupTXT",
		"ResolveIPAddr", "ResolveTCPAddr", "ResolveUDPAddr",
		"DefaultResolver", "Resolver",
	},
	"net/http": {
		"Get", "Head", "Post", "PostForm", "NewRequest", "NewRequestWithContext",
		"Client", "DefaultClient", "Transport", "DefaultTransport", "RoundTripper",
		"ListenAndServe", "ListenAndServeTLS", "Serve", "ServeTLS", "Server",
	},
	"net/http/httptest":     {"NewServer", "NewTLSServer", "NewUnstartedServer", "Server"},
	"net/http/httputil":     {"NewSingleHostReverseProxy", "ReverseProxy"},
	"crypto/tls":            {"Dial", "DialWithDialer", "Dialer", "Client", "Listen", "NewListener"},
	"net/rpc":               {"Dial", "DialHTTP", "DialHTTPPath"},
	"net/rpc/jsonrpc":       {"Dial"},
	"net/smtp":              {"Dial", "NewClient", "SendMail"},
	"net/textproto":         {"Dial"},
	"log/syslog":            {"Dial", "New", "NewLogger"},
	"syscall":               {"Socket", "Connect", "Bind", "Listen"},
	"golang.org/x/sys/unix": {"Socket", "Connect", "Bind", "Listen"},
	"C":                     {"socket", "connect", "getaddrinfo", "gethostbyname"},
}

// networkOnly are packages that are for the network and nothing else, so
// importing one, outside a test, is enough to fail.
var networkOnly = []string{
	"github.com/coder/websocket", // The browser preview's tests use it.
	"github.com/gorilla/websocket",
	"nhooyr.io/websocket",
	"golang.org/x/net/websocket",
	"golang.org/x/net/http2",
	"golang.org/x/net/proxy",
	"google.golang.org/grpc",
	"net/http/cookiejar",
	"net/http/pprof",
}

// networkTools are programs that connect, which Go code mustn't start by
// name, whether it names them with a string or a constant.
var networkTools = []string{
	"curl", "wget", "nc", "ncat", "ssh", "scp", "sftp", "rsync", "ftp", "telnet",
	"nscurl", "dig", "nslookup", "ping",
}

// native are the Objective-C, C, and C++ calls that connect, look a name
// up, or start something that does. They're looked for in Objective-C, C,
// and C++ files, and in the C code above a Go file's import "C".
var native = regexp.MustCompile(`NSURLSession|NSURLConnection|NSURLDownload|(data|download|upload|stream|webSocket)TaskWith|` +
	`nw_connection|nw_listener|nw_browser|CFStreamCreatePairWithSocket|CFSocketCreate|CFHTTP|CFHost|NSHost\b|NSNetService|` +
	`SCNetworkReachabilityCreateWith|DNSService|\bres_n?(query|search)|` +
	`getStreamsToHost|ContentsOfURL|getaddrinfo|gethostbyname|\bconnect\(|\bsocket\(|` +
	`loadRequest:|startDownloadUsingRequest:|resumeDownloadFromResumeData:|SPUStandardUpdaterController|SUUpdater\b`)

// web are the frontend's ways to reach the network: the calls that
// connect, and an address on another host in an attribute, a style, or a
// meta refresh. Its pages and scripts come from Bench's own asset server,
// and so do the tiles' icons (decision 51), so it needs none of them. An
// address that starts with // counts, as it means the same scheme on
// another host.
var web = regexp.MustCompile(`\bfetch\s*\(|XMLHttpRequest|\bWebSocket\s*\(|WebSocketStream|WebTransport|RTCPeerConnection|` +
	`EventSource|sendBeacon|\b(import|from)\s*\(?\s*['"](https?:)?//|` +
	`\b(src|srcset|href|action|poster|data)\s*=\s*['"]?(https?:)?//|` +
	`setAttribute\(\s*['"](src|srcset|href|action|poster|data)['"]\s*,\s*['"](https?:)?//|` +
	`url\(\s*['"]?(https?:)?//|image-set\(\s*['"]?(https?:)?//|\b(?i:url)\s*=\s*['"]?(https?:)?//`)

// allowed is every place that may connect: the file, the names or calls it
// may use, and where it connects and why. Keep it in step with README.md's
// "Permissions and privacy", and the user guide's "Update Bench".
var allowed = []struct {
	file string
	uses []string
	why  string
}{
	{"internal/update/update.go", []string{"http.Client", "http.NewRequestWithContext"},
		"Asks api.github.com for Bench's latest release, so that a notice can say when a newer version is out. " +
			"Only a Bench without Sparkle asks, at most once a day, and only with check_for_updates on (decision 48)."},
	{"internal/ui/app.go", []string{"http.DefaultClient"},
		"Gives update.Latest Go's default client, for the look for a new version above. The browser preview never looks (decision 48)."},
	{"internal/icons/page.go", []string{"http.Client", "http.NewRequestWithContext"},
		"Fetches a Web item's page icon from the address that the page names, or from /favicon.ico on the page's site, " +
			"without cookies, so that its tile can show the icon (decision 51). Bench serves the icon to the frontend itself."},
	{"internal/icons/pages.go", []string{"http.Client"},
		"Keeps the client from icons.NewClient, for the fetches of page icons above."},
	{"internal/vscode/vscode.go", []string{"net.Listen"},
		"Listens on 127.0.0.1 for a moment, to find a free port for code serve-web. " +
			"The VS Code for the Web item's web view then loads the page from that port, on this Mac."},
	{"internal/control/socket.go", []string{"net.ListenUnix", "net.DialTimeout"},
		"bench.sock, a Unix socket in the support folder, which answers bench list while allow_scripts is on (decision 77). " +
			"It's a file on this Mac, not a network address."},
	{"cmd/bench/app_server.go", []string{"net.Listen"},
		"Listens on the browser preview's port on localhost for a moment, to check that the port is free, " +
			"before Wails serves the preview there (decision 17)."},
	{"internal/mac/updater_darwin.m", []string{"SPUStandardUpdaterController"},
		"Sparkle, in a Bench.app built with an update key. It reads the appcast at Info.plist's SUFeedURL, on GitHub, " +
			"with check_for_updates on or for Check for Updates…, and downloads the new version that the user installs (decision 48)."},
	{"internal/mac/web_darwin.m", []string{"loadRequest:"},
		"Web items load the pages that the user adds, and VS Code for the Web from its port on 127.0.0.1."},
}

// TestOnlyAllowedCodeConnects fails for each use of the network in Bench's
// code that allowed doesn't list, at the file and line of its first use.
func TestOnlyAllowedCodeConnects(t *testing.T) {
	found := scanRepo(t)
	listed := map[string][]string{}
	for _, a := range allowed {
		listed[a.file] = append(listed[a.file], a.uses...)
	}
	for _, file := range slices.Sorted(maps.Keys(found)) {
		for _, use := range slices.Sorted(maps.Keys(found[file])) {
			if !slices.Contains(listed[file], use) {
				t.Errorf("%s:%d: %s can connect, and allowed doesn't list it. If Bench must connect here, "+
					"add it to allowed in build/privacy/network_test.go, with where it connects and why. "+
					"Then say so in README.md's \"Permissions and privacy\".", file, found[file][use], use)
			}
		}
	}
}

// TestListsMatchTheCode fails for an entry in allowed that the code no
// longer uses or that doesn't say why, and for a folder in notBench that
// isn't there any more, so that the lists stay exact.
func TestListsMatchTheCode(t *testing.T) {
	found := scanRepo(t)
	for _, a := range allowed {
		if strings.TrimSpace(a.why) == "" {
			t.Errorf("allowed's entry for %s doesn't say where it connects and why.", a.file)
		}
		for _, use := range a.uses {
			if _, ok := found[a.file][use]; !ok {
				t.Errorf("allowed lists %s for %s, which doesn't use it any more: take it out of the list.", use, a.file)
			}
		}
	}
	for _, dir := range slices.Sorted(maps.Keys(notBench)) {
		if _, err := os.Stat(filepath.Join(root, dir)); err != nil {
			t.Errorf("notBench lists %s, which isn't there any more: take it out of the list.", dir)
		}
	}
}

// uses holds, for each file, by its path from the root, the uses of the
// network in it, each with the line of its first use.
type uses map[string]map[string]int

// note records a use at a line of file, unless the use is known already.
func (u uses) note(file string, line int, use string) {
	if u[file] == nil {
		u[file] = map[string]int{}
	}
	if _, ok := u[file][use]; !ok {
		u[file][use] = line
	}
}

var (
	scanOnce sync.Once
	scanned  uses
	scanErr  error
)

// scanRepo returns the uses in the repository, which it reads once for
// all the tests.
func scanRepo(t *testing.T) uses {
	t.Helper()
	scanOnce.Do(func() { scanned, scanErr = scan(root) })
	if scanErr != nil {
		t.Fatalf("reading Bench's code: %v", scanErr)
	}
	return scanned
}

// scan reads every file of Bench's under dir: Go files outside tests,
// Objective-C, C, and C++ files, and frontend files.
func scan(dir string) (uses, error) {
	u := uses{}
	var errs []error
	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(dir, p)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if d.IsDir() {
			if rel != "." && skipped(rel) {
				return filepath.SkipDir
			}
			return nil
		}
		if err := scanFile(p, rel, u); err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", rel, err))
		}
		return nil
	})
	return u, errors.Join(append(errs, err)...)
}

// skipped says whether the folder at rel isn't Bench's code.
func skipped(rel string) bool {
	name := path.Base(rel)
	_, out := notBench[rel]
	return out || name == "testdata" || name == "node_modules" ||
		strings.HasPrefix(name, ".") || strings.HasPrefix(name, "_")
}

// scanFile notes the uses in the file at p, by its kind.
func scanFile(p, rel string, u uses) error {
	switch name := path.Base(rel); path.Ext(name) {
	case ".go":
		if strings.HasSuffix(name, "_test.go") {
			return nil
		}
		return scanGo(p, rel, u)
	case ".m", ".mm", ".h", ".c", ".cc", ".cpp", ".cxx", ".hh", ".hpp":
		return scanText(p, rel, native, u)
	case ".js", ".mjs", ".html", ".css", ".svg":
		return scanText(p, rel, web, u)
	}
	return nil
}

// scanGo notes the names in connects that a Go file uses, the packages in
// networkOnly that it imports, the network tools it starts, and the
// native calls in its C code.
func scanGo(p, rel string, u uses) error {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, p, nil, parser.ParseComments|parser.SkipObjectResolution)
	if err != nil {
		return err
	}
	line := func(n ast.Node) int { return fset.Position(n.Pos()).Line }
	names := map[string]string{} // the file's name for an import -> its path
	for _, imp := range f.Imports {
		ip, _ := strconv.Unquote(imp.Path.Value)
		name := path.Base(ip)
		if imp.Name != nil {
			name = imp.Name.Name
		}
		names[name] = ip
		if slices.ContainsFunc(networkOnly, func(n string) bool { return ip == n || strings.HasPrefix(ip, n+"/") }) {
			u.note(rel, line(imp), "import "+ip)
		}
		if _, ok := connects[ip]; ok && name == "." {
			u.note(rel, line(imp), "import . "+ip)
		}
	}
	scanPreamble(fset, f, rel, u)
	consts := stringConsts(f)
	ast.Inspect(f, func(n ast.Node) bool {
		switch n := n.(type) {
		case *ast.SelectorExpr:
			if use, ok := connectsUse(n, names); ok {
				u.note(rel, line(n), use)
			}
		case *ast.CallExpr:
			if tool, ok := startedTool(n, names, consts); ok {
				u.note(rel, line(n), "exec "+tool)
			}
		}
		return true
	})
	return nil
}

// connectsUse returns the name in connects that sel uses, such as
// "http.Get". It's named by the package, not the file's own name for it,
// so that importing a package under another name doesn't change the entry.
func connectsUse(sel *ast.SelectorExpr, names map[string]string) (string, bool) {
	id, ok := sel.X.(*ast.Ident)
	if !ok {
		return "", false
	}
	ip, ok := names[id.Name]
	if !ok || !slices.Contains(connects[ip], sel.Sel.Name) {
		return "", false
	}
	return path.Base(ip) + "." + sel.Sel.Name, true
}

// startedTool returns the program in networkTools that call starts, if
// call is exec.Command or exec.CommandContext, and it names the program,
// or a path to it, with a string or a constant.
func startedTool(call *ast.CallExpr, names, consts map[string]string) (string, bool) {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return "", false
	}
	id, ok := sel.X.(*ast.Ident)
	if !ok || names[id.Name] != "os/exec" {
		return "", false
	}
	arg := 0
	switch sel.Sel.Name {
	case "Command":
	case "CommandContext":
		arg = 1
	default:
		return "", false
	}
	if len(call.Args) <= arg {
		return "", false
	}
	prog, ok := stringValue(call.Args[arg], consts)
	tool := path.Base(prog)
	return tool, ok && slices.Contains(networkTools, tool)
}

// stringConsts returns the string constants in a file by name, wherever
// they're declared.
func stringConsts(f *ast.File) map[string]string {
	consts := map[string]string{}
	ast.Inspect(f, func(n ast.Node) bool {
		g, ok := n.(*ast.GenDecl)
		if !ok || g.Tok != token.CONST {
			return true
		}
		for _, s := range g.Specs {
			v := s.(*ast.ValueSpec)
			for i, name := range v.Names {
				if i < len(v.Values) {
					if s, ok := stringValue(v.Values[i], nil); ok {
						consts[name.Name] = s
					}
				}
			}
		}
		return true
	})
	return consts
}

// stringValue returns the value of e, if it's a string or the name of a
// constant in consts.
func stringValue(e ast.Expr, consts map[string]string) (string, bool) {
	switch e := e.(type) {
	case *ast.BasicLit:
		if e.Kind == token.STRING {
			s, err := strconv.Unquote(e.Value)
			return s, err == nil
		}
	case *ast.Ident:
		s, ok := consts[e.Name]
		return s, ok
	}
	return "", false
}

// scanPreamble notes the native calls in the comment above a Go file's
// import "C", which is C code that cgo compiles with the file.
func scanPreamble(fset *token.FileSet, f *ast.File, rel string, u uses) {
	for _, d := range f.Decls {
		g, ok := d.(*ast.GenDecl)
		if !ok || g.Tok != token.IMPORT {
			continue
		}
		for _, s := range g.Specs {
			imp := s.(*ast.ImportSpec)
			doc := imp.Doc
			if doc == nil && len(g.Specs) == 1 {
				doc = g.Doc
			}
			if imp.Path.Value != `"C"` || doc == nil {
				continue
			}
			for _, c := range doc.List {
				matchLines(c.Text, fset.Position(c.Pos()).Line, rel, native, u)
			}
		}
	}
}

// scanText notes each match of re in a file.
func scanText(p, rel string, re *regexp.Regexp, u uses) error {
	data, err := os.ReadFile(p)
	if err != nil {
		return err
	}
	matchLines(string(data), 1, rel, re, u)
	return nil
}

// matchLines notes each match of re in text, line by line, where text
// starts at line first of the file rel.
func matchLines(text string, first int, rel string, re *regexp.Regexp, u uses) {
	for i, l := range strings.Split(text, "\n") {
		for _, m := range re.FindAllString(l, -1) {
			u.note(rel, first+i, m)
		}
	}
}
