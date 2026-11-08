//go:build server

package ui

import (
	"os"
	"path/filepath"
	"slices"
	"sync"
	"testing"
)

func osArgs() []string { return os.Args }

// TestForwardNeedsTheApp checks that the preview, which has no
// single-instance lock, refuses to forward a script subcommand, without
// making a Wails app or changing os.Args.
func TestForwardNeedsTheApp(t *testing.T) {
	saved := slices.Clone(osArgs())
	if err := Forward(t.TempDir(), []string{"tab", "--", "Code"}); err == nil || err == ErrNotRunning {
		t.Errorf("Forward in the preview = %v", err)
	}
	if !slices.Equal(osArgs(), saved) {
		t.Errorf("os.Args = %q, want %q", osArgs(), saved)
	}
}

// TestForwardKey checks the key that what other Benches send is encrypted
// with: it's made once, readable only by the user, and the same for every
// Bench that asks, even ones that start at once; and a file that isn't a
// key is replaced.
func TestForwardKey(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "Bench")
	key, err := forwardKey(dir)
	if err != nil {
		t.Fatal(err)
	}
	if key == ([forwardKeySize]byte{}) {
		t.Fatal("the key is all zero, which Wails takes as no key")
	}
	path := filepath.Join(dir, ForwardKeyFile)
	info, err := os.Lstat(path)
	if err != nil {
		t.Fatal(err)
	}
	if !info.Mode().IsRegular() || info.Mode().Perm() != 0o600 || info.Size() != forwardKeySize {
		t.Errorf("the key file: mode %v, %d bytes; want a file only the user can read, of %d bytes", info.Mode(), info.Size(), forwardKeySize)
	}
	again, err := forwardKey(dir)
	if err != nil || again != key {
		t.Errorf("asked again: %v, same key %v", err, again == key)
	}
	// Nothing but the key is left in the folder.
	if entries, _ := os.ReadDir(dir); len(entries) != 1 {
		t.Errorf("the folder has %d files, want the key alone", len(entries))
	}

	// A file that isn't a key is replaced.
	if err := os.WriteFile(path, []byte("short"), 0o600); err != nil {
		t.Fatal(err)
	}
	fresh, err := forwardKey(dir)
	if err != nil || fresh == key || fresh == ([forwardKeySize]byte{}) {
		t.Errorf("replacing a bad key: %v", err)
	}
	if read, err := readKey(path); err != nil || read != fresh {
		t.Errorf("the file has another key: %v", err)
	}

	// Benches that start at once agree.
	dir = filepath.Join(t.TempDir(), "Bench")
	if err := os.Mkdir(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	keys := make([][forwardKeySize]byte, 8)
	errs := make([]error, len(keys))
	var wg sync.WaitGroup
	for i := range keys {
		wg.Add(1)
		go func() {
			defer wg.Done()
			keys[i], errs[i] = forwardKey(dir)
		}()
	}
	wg.Wait()
	for i := range keys {
		if errs[i] != nil || keys[i] != keys[0] {
			t.Errorf("Bench %d: %v, same key %v", i, errs[i], keys[i] == keys[0])
		}
	}
}

// TestForwardKeyNeedsAPrivateFolder checks that there's no key in a
// support folder that other users can change, where they could put their
// own.
func TestForwardKeyNeedsAPrivateFolder(t *testing.T) {
	dir := t.TempDir()
	if err := os.Chmod(dir, 0o777); err != nil {
		t.Fatal(err)
	}
	if _, err := forwardKey(dir); err == nil {
		t.Error("made a key in a folder that others can change")
	}
	if _, err := os.Lstat(filepath.Join(dir, ForwardKeyFile)); err == nil {
		t.Error("there's a key file")
	}
}
