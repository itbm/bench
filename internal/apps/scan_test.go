package apps

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"howett.net/plist"

	"github.com/itbm/bench/internal/platform"
)

// plistFormat says how a test bundle's Info.plist is written.
type plistFormat int

const (
	xmlPlist plistFormat = iota
	binaryPlist
	brokenPlist // Not a property list.
	noPlist     // No Info.plist at all.
)

// bundle describes a test app bundle.
type bundle struct {
	name   string         // File name, such as "Zed.app".
	info   map[string]any // Info.plist's contents.
	format plistFormat
	// wrapped puts Info.plist in WrappedBundle, as for an iPad app.
	wrapped bool
}

// makeBundle creates a bundle in dir and returns its path.
func makeBundle(t *testing.T, dir string, b bundle) string {
	t.Helper()
	path := filepath.Join(dir, b.name)
	rel := filepath.Join("Contents", "Info.plist")
	if b.wrapped {
		rel = filepath.Join("WrappedBundle", "Info.plist")
	}
	plistPath := filepath.Join(path, rel)
	if err := os.MkdirAll(filepath.Dir(plistPath), 0o755); err != nil {
		t.Fatal(err)
	}
	var data []byte
	switch b.format {
	case xmlPlist, binaryPlist:
		format := plist.XMLFormat
		if b.format == binaryPlist {
			format = plist.BinaryFormat
		}
		var err error
		if data, err = plist.Marshal(b.info, format); err != nil {
			t.Fatal(err)
		}
	case brokenPlist:
		data = []byte("this isn't a property list")
	case noPlist:
		return path
	}
	if err := os.WriteFile(plistPath, data, 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

// app returns Info.plist contents with a bundle ID and a name.
func app(id, name string) map[string]any {
	return map[string]any{"CFBundleIdentifier": id, "CFBundleName": name}
}

func TestReadBundle(t *testing.T) {
	tests := []struct {
		name    string
		bundle  bundle
		want    platform.AppInfo // Path is filled in.
		wantErr error            // Checked with errors.Is, if set.
		anyErr  bool
	}{
		{
			name:   "XML",
			bundle: bundle{name: "Zed.app", info: app("dev.zed.Zed", "Zed")},
			want:   platform.AppInfo{BundleID: "dev.zed.Zed", Name: "Zed"},
		},
		{
			name:   "binary",
			bundle: bundle{name: "Brave Browser.app", info: app("com.brave.Browser", "Brave Browser"), format: binaryPlist},
			want:   platform.AppInfo{BundleID: "com.brave.Browser", Name: "Brave Browser"},
		},
		{
			name: "display name first",
			bundle: bundle{name: "Safari.app", info: map[string]any{
				"CFBundleIdentifier": "com.apple.Safari", "CFBundleName": "Safari Browser", "CFBundleDisplayName": "Safari",
			}},
			want: platform.AppInfo{BundleID: "com.apple.Safari", Name: "Safari"},
		},
		{
			name:   "file name last",
			bundle: bundle{name: "Linear.app", info: map[string]any{"CFBundleIdentifier": "com.linear"}},
			want:   platform.AppInfo{BundleID: "com.linear", Name: "Linear"},
		},
		{
			name: "names lose .app",
			bundle: bundle{name: "Figma.app", info: map[string]any{
				"CFBundleIdentifier": "com.figma.Desktop", "CFBundleDisplayName": "Figma.app",
			}},
			want: platform.AppInfo{BundleID: "com.figma.Desktop", Name: "Figma"},
		},
		{
			name: "unexpanded build settings are skipped",
			bundle: bundle{name: "Tool.app", info: map[string]any{
				"CFBundleIdentifier":  "com.example.tool",
				"CFBundleDisplayName": "${PRODUCT_NAME}",
				"CFBundleName":        "$(PRODUCT_NAME)",
			}},
			want: platform.AppInfo{BundleID: "com.example.tool", Name: "Tool"},
		},
		{
			name: "names that aren't strings are skipped",
			bundle: bundle{name: "Odd.app", info: map[string]any{
				"CFBundleIdentifier": "com.example.odd", "CFBundleDisplayName": 42, "CFBundleName": " Odd One ",
			}},
			want: platform.AppInfo{BundleID: "com.example.odd", Name: "Odd One"},
		},
		{
			name:   "an iPad app",
			bundle: bundle{name: "Game.app", info: app("com.example.game", "Game"), wrapped: true, format: binaryPlist},
			want:   platform.AppInfo{BundleID: "com.example.game", Name: "Game"},
		},
		{
			name:    "no bundle ID",
			bundle:  bundle{name: "Nameless.app", info: map[string]any{"CFBundleName": "Nameless"}},
			wantErr: ErrNoBundleID,
		},
		{
			name:    "an empty bundle ID",
			bundle:  bundle{name: "Blank.app", info: app("  ", "Blank")},
			wantErr: ErrNoBundleID,
		},
		{
			name:   "not a property list",
			bundle: bundle{name: "Broken.app", format: brokenPlist},
			anyErr: true,
		},
		{
			name:    "no Info.plist",
			bundle:  bundle{name: "Empty.app", format: noPlist},
			wantErr: os.ErrNotExist,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := makeBundle(t, t.TempDir(), tt.bundle)
			got, err := ReadBundle(path)
			if tt.wantErr != nil || tt.anyErr {
				if err == nil {
					t.Fatalf("ReadBundle = %+v, want an error", got)
				}
				if tt.wantErr != nil && !errors.Is(err, tt.wantErr) {
					t.Errorf("ReadBundle error = %v, want %v", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("ReadBundle error: %v", err)
			}
			tt.want.Path = path
			if got != tt.want {
				t.Errorf("ReadBundle = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestScan(t *testing.T) {
	root := t.TempDir()
	apps := filepath.Join(root, "Applications")
	utilities := filepath.Join(apps, "Utilities")
	system := filepath.Join(root, "System", "Applications")
	elsewhere := filepath.Join(root, "Elsewhere")
	for _, dir := range []string{utilities, system, elsewhere} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}

	zed := makeBundle(t, apps, bundle{name: "Zed.app", info: app("dev.zed.Zed", "Zed")})
	brave := makeBundle(t, apps, bundle{name: "Brave Browser.app", info: app("com.brave.Browser", "Brave Browser"), format: binaryPlist})
	obsidian := makeBundle(t, apps, bundle{name: "obsidian.app", info: app("md.obsidian", "obsidian")})
	terminal := makeBundle(t, utilities, bundle{name: "Terminal.app", info: app("com.apple.Terminal", "Terminal"), format: binaryPlist})
	safari := makeBundle(t, system, bundle{name: "Safari.app", info: app("com.apple.Safari", "Safari")})
	upper := makeBundle(t, system, bundle{name: "NOTES.APP", info: app("com.apple.Notes", "Notes")})

	// Skipped: duplicates of earlier IDs (in any case), bundles that can't
	// be read, hidden bundles, folders that aren't bundles, files named like
	// bundles, and bundles inside folders inside dirs.
	makeBundle(t, system, bundle{name: "Zed.app", info: app("dev.zed.Zed", "Zed (system)")})
	makeBundle(t, system, bundle{name: "Brave.app", info: app("COM.BRAVE.BROWSER", "Brave again")})
	makeBundle(t, apps, bundle{name: "Nameless.app", info: map[string]any{"CFBundleName": "Nameless"}})
	makeBundle(t, apps, bundle{name: "Broken.app", format: brokenPlist})
	makeBundle(t, apps, bundle{name: "Empty.app", format: noPlist})
	makeBundle(t, apps, bundle{name: ".Hidden.app", info: app("com.example.hidden", "Hidden")})
	makeBundle(t, apps, bundle{name: "Folder", info: app("com.example.folder", "Folder")})
	makeBundle(t, filepath.Join(apps, "Suite"), bundle{name: "Nested.app", info: app("com.example.nested", "Nested")})
	if err := os.WriteFile(filepath.Join(apps, "File.app"), []byte("not a folder"), 0o644); err != nil {
		t.Fatal(err)
	}

	// A link to a bundle elsewhere counts, and so does its name.
	linked := makeBundle(t, elsewhere, bundle{name: "Real Figma.app", info: app("com.figma.Desktop", "Figma")})
	link := filepath.Join(apps, "Figma.app")
	if err := os.Symlink(linked, link); err != nil {
		t.Fatal(err)
	}
	// A broken link is skipped.
	if err := os.Symlink(filepath.Join(root, "gone.app"), filepath.Join(apps, "Gone.app")); err != nil {
		t.Fatal(err)
	}

	dirs := []string{apps, utilities, filepath.Join(root, "Missing"), system, filepath.Join(system, "Utilities")}
	got := Scan(dirs)
	want := []platform.AppInfo{
		{BundleID: "com.brave.Browser", Name: "Brave Browser", Path: brave},
		{BundleID: "com.figma.Desktop", Name: "Figma", Path: link},
		{BundleID: "com.apple.Notes", Name: "Notes", Path: upper},
		{BundleID: "md.obsidian", Name: "obsidian", Path: obsidian},
		{BundleID: "com.apple.Safari", Name: "Safari", Path: safari},
		{BundleID: "com.apple.Terminal", Name: "Terminal", Path: terminal},
		{BundleID: "dev.zed.Zed", Name: "Zed", Path: zed},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Scan =\n%+v\nwant\n%+v", got, want)
	}

	// The first folder wins.
	reversed := Scan([]string{system, apps})
	for _, a := range reversed {
		if a.BundleID == "dev.zed.Zed" && a.Name != "Zed (system)" {
			t.Errorf("Scan(system first) has %+v, want the system Zed", a)
		}
	}
}

func TestScanNothing(t *testing.T) {
	tests := []struct {
		name string
		dirs []string
	}{
		{"no folders", nil},
		{"missing folders", []string{filepath.Join(t.TempDir(), "Missing")}},
		{"an empty folder", []string{t.TempDir()}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Scan(tt.dirs)
			if got == nil || len(got) != 0 {
				t.Errorf("Scan = %#v, want an empty, non-nil slice", got)
			}
		})
	}
}

func TestScanSortsIgnoringCase(t *testing.T) {
	dir := t.TempDir()
	for i, name := range []string{"beta", "Alpha", "alpha", "Gamma", "BETA"} {
		makeBundle(t, dir, bundle{
			name: string(rune('a'+i)) + ".app",
			info: app("com.example."+string(rune('a'+i)), name),
		})
	}
	var names []string
	for _, a := range Scan([]string{dir}) {
		names = append(names, a.Name)
	}
	want := []string{"Alpha", "alpha", "BETA", "beta", "Gamma"}
	if !reflect.DeepEqual(names, want) {
		t.Errorf("names = %q, want %q", names, want)
	}
}

func TestDirs(t *testing.T) {
	tests := []struct {
		home string
		want []string
	}{
		{"/Users/sam", []string{
			"/Applications",
			"/Applications/Utilities",
			"/Users/sam/Applications",
			"/Users/sam/Applications/Utilities",
			"/Users/sam/Applications/Orion/WebApps",
			"/System/Applications",
			"/System/Applications/Utilities",
		}},
		{"/Users/sam/", []string{
			"/Applications",
			"/Applications/Utilities",
			"/Users/sam/Applications",
			"/Users/sam/Applications/Utilities",
			"/Users/sam/Applications/Orion/WebApps",
			"/System/Applications",
			"/System/Applications/Utilities",
		}},
		{"", []string{
			"/Applications",
			"/Applications/Utilities",
			"/System/Applications",
			"/System/Applications/Utilities",
		}},
	}
	for _, tt := range tests {
		if got := Dirs(tt.home); !reflect.DeepEqual(got, tt.want) {
			t.Errorf("Dirs(%q) = %q, want %q", tt.home, got, tt.want)
		}
	}
}

func TestDisplayName(t *testing.T) {
	tests := []struct{ in, want string }{
		{"Zed.app", "Zed"},
		{"Visual Studio Code.APP", "Visual Studio Code"},
		{"  Brave Browser.app  ", "Brave Browser"},
		{"Slack", "Slack"},
		{"Approach", "Approach"},
		{".app", ".app"},
		{"", ""},
	}
	for _, tt := range tests {
		if got := DisplayName(tt.in); got != tt.want {
			t.Errorf("DisplayName(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}
