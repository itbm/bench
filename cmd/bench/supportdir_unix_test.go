//go:build unix

package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSupportDirRefusesSharedFolders(t *testing.T) {
	shared := filepath.Join(t.TempDir(), "bench")
	if err := os.Mkdir(shared, 0o777); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(shared, 0o777); err != nil {
		t.Fatal(err)
	}
	if got, err := supportDir(shared); err == nil {
		t.Errorf("supportDir(a folder anyone can change) = %q, want an error", got)
	}
	private := t.TempDir()
	if got, err := supportDir(private); err != nil || got != private {
		t.Errorf("supportDir(a private folder) = %q, %v; want it back", got, err)
	}
}
