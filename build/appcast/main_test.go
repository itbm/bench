package main

import (
	"bytes"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/hex"
	"encoding/xml"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// The second test vector of RFC 8032, section 7.1: a key, and its signature
// of the one byte 0x72. Sparkle signs with the same Ed25519.
const (
	rfcSeed      = "4ccd089b28ff96da9db6c346ec114e0f5b8a319f35aba624da8cf6ed4fb8a6fb"
	rfcPublic    = "3d4017c3e843895a92b70aa74d1b7ebc9c982ccf2ec4968cc0cd55f12af4660c"
	rfcSignature = "92a009a9f0d4cab8720e820b5f642540a2b27b5416503f8fb3762223ebdb69da" +
		"085ac1e43e15996e458f3613d0f11d8c387b2eaeb4302aeeb00d291612bb0c00"
)

// b64 returns hex-encoded bytes in base64.
func b64(t *testing.T, h string) string {
	t.Helper()
	b, err := hex.DecodeString(h)
	if err != nil {
		t.Fatal(err)
	}
	return base64.StdEncoding.EncodeToString(b)
}

func TestPrintPublicKey(t *testing.T) {
	old := strings.Repeat("A", 86) + "==" // 64 bytes of zeros
	oldKey, _ := base64.StdEncoding.DecodeString(old)
	public, _ := hex.DecodeString(rfcPublic)
	tests := []struct {
		name, private, want string
	}{
		{"a seed, as generate_keys -x writes it", b64(t, rfcSeed) + "\n", b64(t, rfcPublic)},
		{"an old key, with its public key at the end", base64.StdEncoding.EncodeToString(append(oldKey, public...)), b64(t, rfcPublic)},
		{"not base64", "not a key!", ""},
		{"the wrong length", base64.StdEncoding.EncodeToString(public[:16]), ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var out bytes.Buffer
			err := run([]string{"-print-public-key"}, strings.NewReader(tt.private), &out)
			if tt.want == "" {
				if err == nil {
					t.Errorf("printed %q, want an error", out.String())
				}
				return
			}
			if err != nil || out.String() != tt.want+"\n" {
				t.Errorf("printed %q, %v; want %q", out.String(), err, tt.want)
			}
		})
	}
}

// feed is the part of an appcast that Sparkle reads.
type feed struct {
	XMLName xml.Name `xml:"rss"`
	Items   []struct {
		Title        string `xml:"title"`
		PubDate      string `xml:"pubDate"`
		Link         string `xml:"link"`
		Version      string `xml:"http://www.andymatuschak.org/xml-namespaces/sparkle version"`
		ShortVersion string `xml:"http://www.andymatuschak.org/xml-namespaces/sparkle shortVersionString"`
		MinSystem    string `xml:"http://www.andymatuschak.org/xml-namespaces/sparkle minimumSystemVersion"`
		History      string `xml:"http://www.andymatuschak.org/xml-namespaces/sparkle fullReleaseNotesLink"`
		Description  struct {
			Format string `xml:"http://www.andymatuschak.org/xml-namespaces/sparkle format,attr"`
			Text   string `xml:",chardata"`
		} `xml:"description"`
		Enclosure struct {
			URL       string `xml:"url,attr"`
			Length    int64  `xml:"length,attr"`
			Type      string `xml:"type,attr"`
			Signature string `xml:"http://www.andymatuschak.org/xml-namespaces/sparkle edSignature,attr"`
		} `xml:"enclosure"`
	} `xml:"channel>item"`
}

// release writes an update file (the byte that the RFC's key signed) and
// release notes to a folder, and returns the arguments for them.
func release(t *testing.T, notes string) (dir string, args []string) {
	t.Helper()
	dir = t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "Bench.dmg"), []byte{0x72}, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "notes.md"), []byte(notes), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir, []string{
		"-version", "0.3.0",
		"-min-system", "14.0",
		"-url", "https://github.com/itbm/bench/releases/download/v0.3.0/Bench.dmg",
		"-link", "https://github.com/itbm/bench/releases/tag/v0.3.0",
		"-archive", filepath.Join(dir, "Bench.dmg"),
		"-signature", b64(t, rfcSignature),
		"-public-key", b64(t, rfcPublic),
		"-notes", filepath.Join(dir, "notes.md"),
		"-date", "2026-10-30T12:00:00Z",
		"-o", filepath.Join(dir, "appcast.xml"),
	}
}

func TestAppcast(t *testing.T) {
	notes := "## What's new\n\n- Sparkle installs updates.\n- A CDATA end, ]]>, in the notes.\n"
	dir, args := release(t, notes)
	if err := run(args, nil, nil); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(dir, "appcast.xml"))
	if err != nil {
		t.Fatal(err)
	}
	var f feed
	if err := xml.Unmarshal(data, &f); err != nil {
		t.Fatalf("the appcast isn't XML: %v\n%s", err, data)
	}
	if len(f.Items) != 1 {
		t.Fatalf("the appcast has %d items, want 1:\n%s", len(f.Items), data)
	}
	it := f.Items[0]
	date, _ := time.Parse(time.RFC3339, "2026-10-30T12:00:00Z")
	for _, c := range []struct{ name, got, want string }{
		{"title", it.Title, "Bench 0.3.0"},
		{"pubDate", it.PubDate, date.Format(time.RFC1123Z)},
		{"link", it.Link, "https://github.com/itbm/bench/releases/tag/v0.3.0"},
		{"sparkle:version", it.Version, "0.3.0"},
		{"sparkle:shortVersionString", it.ShortVersion, "0.3.0"},
		{"sparkle:minimumSystemVersion", it.MinSystem, "14.0.0"},
		{"sparkle:fullReleaseNotesLink", it.History, "https://github.com/itbm/bench/releases"},
		{"description's format", it.Description.Format, "markdown"},
		{"description", it.Description.Text, notes},
		{"enclosure's url", it.Enclosure.URL, "https://github.com/itbm/bench/releases/download/v0.3.0/Bench.dmg"},
		{"enclosure's type", it.Enclosure.Type, "application/octet-stream"},
		{"enclosure's signature", it.Enclosure.Signature, b64(t, rfcSignature)},
	} {
		if c.got != c.want {
			t.Errorf("%s = %q, want %q", c.name, c.got, c.want)
		}
	}
	if it.Enclosure.Length != 1 {
		t.Errorf("enclosure's length = %d, want 1", it.Enclosure.Length)
	}
}

// TestAppcastOnThisMac checks that an http:// address on this Mac, which
// the update check on a macOS runner serves the update from, is allowed.
func TestAppcastOnThisMac(t *testing.T) {
	dir, args := release(t, "")
	args = append(args, "-url", "http://127.0.0.1:8765/Bench.dmg", "-link", "http://localhost:8765/")
	if err := run(args, nil, nil); err != nil {
		t.Fatal(err)
	}
	out, _ := os.ReadFile(filepath.Join(dir, "appcast.xml"))
	if !strings.Contains(string(out), `url="http://127.0.0.1:8765/Bench.dmg"`) {
		t.Errorf("the appcast doesn't have the address on this Mac:\n%s", out)
	}
}

// TestAppcastSignedByGo checks a signature that Go's Ed25519 makes for a
// bigger file, as sign_update would.
func TestAppcastSignedByGo(t *testing.T) {
	dir, args := release(t, "")
	data := bytes.Repeat([]byte("Bench.dmg "), 100000)
	if err := os.WriteFile(filepath.Join(dir, "Bench.dmg"), data, 0o644); err != nil {
		t.Fatal(err)
	}
	seed, _ := hex.DecodeString(rfcSeed)
	sig := base64.StdEncoding.EncodeToString(ed25519.Sign(ed25519.NewKeyFromSeed(seed), data))
	args = append(args, "-signature", sig)
	if err := run(args, nil, nil); err != nil {
		t.Fatal(err)
	}
	out, _ := os.ReadFile(filepath.Join(dir, "appcast.xml"))
	if strings.Contains(string(out), "<description") {
		t.Error("the appcast has a description without release notes")
	}
	if !strings.Contains(string(out), `length="1000000"`) {
		t.Errorf("the appcast doesn't give the length, 1000000:\n%s", out)
	}
}

func TestAppcastRefuses(t *testing.T) {
	tests := []struct {
		name string
		edit func(dir string, args []string) []string
		want string
	}{
		{"a file that someone changed", func(dir string, args []string) []string {
			os.WriteFile(filepath.Join(dir, "Bench.dmg"), []byte{0x73}, 0o644)
			return args
		}, "doesn't match"},
		{"another key", func(_ string, args []string) []string {
			return append(args, "-public-key", base64.StdEncoding.EncodeToString(make([]byte, 32)))
		}, "doesn't match"},
		{"a signature that isn't one", func(_ string, args []string) []string {
			return append(args, "-signature", "abc")
		}, "isn't 64 bytes"},
		{"a pre-release version", func(_ string, args []string) []string {
			return append(args, "-version", "0.3.0-beta.1")
		}, "isn't a version"},
		{"a macOS version that isn't one", func(_ string, args []string) []string {
			return append(args, "-min-system", "Sonoma")
		}, "isn't a macOS version"},
		{"an address without https", func(_ string, args []string) []string {
			return append(args, "-url", "http://example.com/Bench.dmg")
		}, "isn't an https:// address"},
		{"an address without a host", func(_ string, args []string) []string {
			return append(args, "-link", "https:///releases")
		}, "isn't an https:// address"},
		{"no version", func(_ string, args []string) []string {
			return append(args, "-version", "")
		}, "-version is missing"},
		{"an argument", func(_ string, args []string) []string {
			return append(args, "extra")
		}, "unexpected argument"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir, args := release(t, "")
			err := run(tt.edit(dir, args), nil, nil)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Errorf("error %v, want one that says %q", err, tt.want)
			}
			if _, err := os.Stat(filepath.Join(dir, "appcast.xml")); err == nil {
				t.Error("wrote the appcast anyway")
			}
		})
	}
}
