//go:build unix

package apps

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"
)

func TestReadBundleRefusesFilesThatArentRegular(t *testing.T) {
	tests := []struct {
		name string
		make func(t *testing.T, plistPath string)
	}{
		{"a named pipe", func(t *testing.T, plistPath string) {
			if err := syscall.Mkfifo(plistPath, 0o644); err != nil {
				t.Skipf("can't make a named pipe: %v", err)
			}
		}},
		{"a link to /dev/zero", func(t *testing.T, plistPath string) {
			if _, err := os.Stat("/dev/zero"); err != nil {
				t.Skip("there's no /dev/zero")
			}
			if err := os.Symlink("/dev/zero", plistPath); err != nil {
				t.Fatal(err)
			}
		}},
		{"a folder", func(t *testing.T, plistPath string) {
			if err := os.Mkdir(plistPath, 0o755); err != nil {
				t.Fatal(err)
			}
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "Crafted.app")
			plistPath := filepath.Join(path, "Contents", "Info.plist")
			if err := os.MkdirAll(filepath.Dir(plistPath), 0o755); err != nil {
				t.Fatal(err)
			}
			tt.make(t, plistPath)
			if err := readBundleWithin(t, path); err == nil {
				t.Error("ReadBundle read it, want an error")
			}
		})
	}
}

func TestReadBundleFollowsLinksToFiles(t *testing.T) {
	// A link to a regular file still reads, as some bundles are put
	// together with links.
	real := makeBundle(t, t.TempDir(), bundle{name: "Real.app", info: app("com.example.linked", "Linked")})
	path := filepath.Join(t.TempDir(), "Linked.app")
	plistPath := filepath.Join(path, "Contents", "Info.plist")
	if err := os.MkdirAll(filepath.Dir(plistPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(real, "Contents", "Info.plist"), plistPath); err != nil {
		t.Fatal(err)
	}
	got, err := ReadBundle(path)
	if err != nil {
		t.Fatalf("ReadBundle error: %v", err)
	}
	if got.BundleID != "com.example.linked" {
		t.Errorf("ReadBundle = %+v, want com.example.linked", got)
	}
}
