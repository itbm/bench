// Command appcast writes the appcast that Sparkle reads to find a new
// version of Bench: an RSS feed with one item, the release, which names the
// update file (Bench.dmg), its length, and its EdDSA signature.
//
// The release workflow runs it after it signs Bench.dmg with Sparkle's
// sign_update, and signs the appcast with sign_update after it:
//
//	go run ./build/appcast -version 0.3.0 -min-system 14.0 \
//		-url https://github.com/itbm/bench/releases/download/v0.3.0/Bench.dmg \
//		-link https://github.com/itbm/bench/releases/tag/v0.3.0 \
//		-archive dist/Bench.dmg -signature <signature> -public-key <key> \
//		-notes notes.md -o dist/appcast.xml
//
// It checks the signature against the file and the public key, so that a
// release with a wrong signature fails here, not on people's Macs.
//
// With -print-public-key, it reads a private key, as Sparkle's
// generate_keys -x writes it, from standard input instead, and prints its
// public key, for Info.plist's SUPublicEDKey. So the release workflow needs
// only the private key's secret.
//
// See docs/development.md#updates.
package main

import (
	"bytes"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/xml"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/url"
	"os"
	"regexp"
	"strings"
	"time"
)

// item is what the appcast says about a release.
type item struct {
	// Version is the release's version, as CFBundleShortVersionString and
	// CFBundleVersion have it, such as 0.3.0.
	Version string
	// MinSystem is the oldest macOS that the release runs on, as
	// LSMinimumSystemVersion has it, such as 14.0.
	MinSystem string
	// URL is where Sparkle downloads the update file.
	URL string
	// Link is the release's page.
	Link string
	// Length is the update file's length in bytes.
	Length int64
	// Signature is the update file's EdDSA signature, in base64.
	Signature string
	// Notes are the release notes, in Markdown.
	Notes string
	// Date is when the release came out.
	Date time.Time
}

// versionPattern matches a version that Sparkle and update.Compare agree
// about: numbers with dots, and no pre-release suffix.
var versionPattern = regexp.MustCompile(`^\d+(\.\d+){1,2}$`)

func main() {
	if err := run(os.Args[1:], os.Stdin, os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "appcast:", err)
		os.Exit(1)
	}
}

// run runs the command with args.
func run(args []string, stdin io.Reader, stdout io.Writer) error {
	fs := flag.NewFlagSet("appcast", flag.ContinueOnError)
	var (
		it        item
		archive   = fs.String("archive", "", "the update file, to check its signature and length")
		publicKey = fs.String("public-key", "", "the EdDSA public key, in base64, as Info.plist's SUPublicEDKey has it")
		notes     = fs.String("notes", "", "a file of release notes, in Markdown")
		date      = fs.String("date", "", "when the release came out, in RFC 3339 (default now)")
		out       = fs.String("o", "", "the appcast to write (default standard output)")
		printKey  = fs.Bool("print-public-key", false, "read a private key from standard input, and print its public key")
	)
	fs.StringVar(&it.Version, "version", "", "the release's version, such as 0.3.0")
	fs.StringVar(&it.MinSystem, "min-system", "", "the oldest macOS that the release runs on, such as 14.0")
	fs.StringVar(&it.URL, "url", "", "where Sparkle downloads the update file")
	fs.StringVar(&it.Link, "link", "", "the release's page")
	fs.StringVar(&it.Signature, "signature", "", "the update file's EdDSA signature, as sign_update -p prints it")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() > 0 {
		return fmt.Errorf("unexpected argument %q", fs.Arg(0))
	}
	if *printKey {
		private, err := io.ReadAll(io.LimitReader(stdin, 1<<10))
		if err != nil {
			return err
		}
		public, err := publicKeyOf(string(private))
		if err != nil {
			return err
		}
		_, err = fmt.Fprintln(stdout, public)
		return err
	}

	for _, f := range []struct{ name, value string }{
		{"version", it.Version}, {"min-system", it.MinSystem}, {"url", it.URL}, {"link", it.Link},
		{"archive", *archive}, {"signature", it.Signature}, {"public-key", *publicKey},
	} {
		if f.value == "" {
			return fmt.Errorf("-%s is missing", f.name)
		}
	}
	if !versionPattern.MatchString(it.Version) {
		return fmt.Errorf("-version %q isn't a version such as 0.3.0", it.Version)
	}
	minSystem, err := systemVersion(it.MinSystem)
	if err != nil {
		return err
	}
	it.MinSystem = minSystem
	for _, u := range []string{it.URL, it.Link} {
		if !secure(u) {
			return fmt.Errorf("%q isn't an https:// address", u)
		}
	}
	data, err := os.ReadFile(*archive)
	if err != nil {
		return err
	}
	if err := verify(data, it.Signature, *publicKey); err != nil {
		return fmt.Errorf("%s: %w", *archive, err)
	}
	it.Length = int64(len(data))
	if *notes != "" {
		text, err := os.ReadFile(*notes)
		if err != nil {
			return err
		}
		it.Notes = string(text)
	}
	it.Date = time.Now().UTC()
	if *date != "" {
		if it.Date, err = time.Parse(time.RFC3339, *date); err != nil {
			return fmt.Errorf("-date: %w", err)
		}
	}
	feed := render(it)
	if *out == "" {
		_, err = stdout.Write(feed)
		return err
	}
	return os.WriteFile(*out, feed, 0o644)
}

// secure reports whether an address is an https:// one, or an http:// one
// on this Mac, which the update check on a macOS runner serves the update
// from (scripts/macos-update/run.sh).
func secure(address string) bool {
	u, err := url.Parse(address)
	if err != nil || u.Host == "" {
		return false
	}
	switch u.Scheme {
	case "https":
		return true
	case "http":
		return u.Hostname() == "127.0.0.1" || u.Hostname() == "localhost"
	}
	return false
}

// systemVersion returns a macOS version with three parts, as Sparkle's
// sparkle:minimumSystemVersion has it: 14.0 becomes 14.0.0.
func systemVersion(v string) (string, error) {
	if !versionPattern.MatchString(v) {
		return "", fmt.Errorf("-min-system %q isn't a macOS version such as 14.0", v)
	}
	if strings.Count(v, ".") == 1 {
		v += ".0"
	}
	return v, nil
}

// publicKeyOf returns the public key, in base64, of a private key in one of
// the forms that Sparkle's generate_keys -x writes: base64 of a 32-byte
// seed, or, from keys that Sparkle made before 2.0.0, of 96 bytes that end
// with the public key.
func publicKeyOf(private string) (string, error) {
	b, err := base64.StdEncoding.DecodeString(strings.TrimSpace(private))
	if err != nil {
		return "", errors.New("the private key isn't base64, as generate_keys -x writes it")
	}
	switch len(b) {
	case ed25519.SeedSize:
		public := ed25519.NewKeyFromSeed(b).Public().(ed25519.PublicKey)
		return base64.StdEncoding.EncodeToString(public), nil
	case 96:
		return base64.StdEncoding.EncodeToString(b[64:]), nil
	}
	return "", fmt.Errorf("the private key has %d bytes, want 32, or 96 for an old key", len(b))
}

// verify checks an EdDSA signature, in base64, of data, with a public key in
// base64.
func verify(data []byte, signature, publicKey string) error {
	key, err := base64.StdEncoding.DecodeString(publicKey)
	if err != nil || len(key) != ed25519.PublicKeySize {
		return errors.New("the public key isn't 32 bytes in base64, as generate_keys prints it")
	}
	sig, err := base64.StdEncoding.DecodeString(signature)
	if err != nil || len(sig) != ed25519.SignatureSize {
		return errors.New("the signature isn't 64 bytes in base64, as sign_update -p prints it")
	}
	if !ed25519.Verify(key, data, sig) {
		return errors.New("the signature doesn't match the file and the public key")
	}
	return nil
}

// render returns the appcast for a release.
func render(it item) []byte {
	var b bytes.Buffer
	b.WriteString(xml.Header)
	b.WriteString(`<rss version="2.0" xmlns:sparkle="http://www.andymatuschak.org/xml-namespaces/sparkle">` + "\n")
	b.WriteString("  <channel>\n")
	b.WriteString("    <title>Bench</title>\n")
	b.WriteString("    <language>en</language>\n")
	b.WriteString("    <item>\n")
	element(&b, "title", "Bench "+it.Version)
	element(&b, "pubDate", it.Date.Format(time.RFC1123Z))
	element(&b, "link", it.Link)
	element(&b, "sparkle:version", it.Version)
	element(&b, "sparkle:shortVersionString", it.Version)
	element(&b, "sparkle:minimumSystemVersion", it.MinSystem)
	element(&b, "sparkle:fullReleaseNotesLink", releasesPage(it.Link))
	if strings.TrimSpace(it.Notes) != "" {
		b.WriteString(`      <description sparkle:format="markdown"><![CDATA[`)
		// A CDATA section ends at the first ]]>, so one in the notes is
		// split across two sections.
		b.WriteString(strings.ReplaceAll(it.Notes, "]]>", "]]]]><![CDATA[>"))
		b.WriteString("]]></description>\n")
	}
	b.WriteString(`      <enclosure url="` + attr(it.URL) + `" length="` + fmt.Sprint(it.Length) +
		`" type="application/octet-stream" sparkle:edSignature="` + attr(it.Signature) + `"/>` + "\n")
	b.WriteString("    </item>\n")
	b.WriteString("  </channel>\n")
	b.WriteString("</rss>\n")
	return b.Bytes()
}

// releasesPage returns the page of every release, from a release's page:
// .../releases/tag/v0.3.0 gives .../releases.
func releasesPage(link string) string {
	if i := strings.Index(link, "/releases/"); i >= 0 {
		return link[:i] + "/releases"
	}
	return link
}

// element writes an element of the item, with escaped text.
func element(b *bytes.Buffer, name, text string) {
	b.WriteString("      <" + name + ">")
	xml.EscapeText(b, []byte(text))
	b.WriteString("</" + name + ">\n")
}

// attr returns text escaped for an attribute in double quotes.
func attr(text string) string {
	var b strings.Builder
	xml.EscapeText(&b, []byte(text))
	return b.String()
}
