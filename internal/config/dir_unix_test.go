//go:build unix

package config

import (
	"os"
	"path/filepath"
	"testing"
)

// otherUser is a user ID that isn't the test's, for folders that belong to
// someone else. Only root can give files away, so tests that need it skip
// otherwise.
const otherUser = 4242

func TestCheckDir(t *testing.T) {
	tests := []struct {
		name string
		// make creates the folder to check, or something else at its path,
		// and returns that path.
		make   func(t *testing.T, dir string) string
		wantOK bool
	}{
		{"a private folder", func(t *testing.T, dir string) string {
			return mkdir(t, filepath.Join(dir, "home"), 0o700)
		}, true},
		{"a folder others can read", func(t *testing.T, dir string) string {
			return mkdir(t, filepath.Join(dir, "home"), 0o755)
		}, true},
		{"a folder that doesn't exist yet", func(t *testing.T, dir string) string {
			return filepath.Join(dir, "new", "home")
		}, true},
		{"a link of the user's to a private folder", func(t *testing.T, dir string) string {
			return symlink(t, mkdir(t, filepath.Join(dir, "real"), 0o700), filepath.Join(dir, "home"))
		}, true},
		{"a folder anyone can change", func(t *testing.T, dir string) string {
			return mkdir(t, filepath.Join(dir, "home"), 0o777)
		}, false},
		{"a folder its group can change", func(t *testing.T, dir string) string {
			return mkdir(t, filepath.Join(dir, "home"), 0o770)
		}, false},
		{"a link to a folder anyone can change", func(t *testing.T, dir string) string {
			return symlink(t, mkdir(t, filepath.Join(dir, "real"), 0o777), filepath.Join(dir, "home"))
		}, false},
		{"a file", func(t *testing.T, dir string) string {
			path := filepath.Join(dir, "home")
			writeFile(t, path, "")
			return path
		}, false},
		{"another user's folder", func(t *testing.T, dir string) string {
			path := mkdir(t, filepath.Join(dir, "home"), 0o700)
			giveAway(t, path, os.Chown)
			return path
		}, false},
		{"another user's link to a private folder", func(t *testing.T, dir string) string {
			path := symlink(t, mkdir(t, filepath.Join(dir, "real"), 0o700), filepath.Join(dir, "home"))
			giveAway(t, path, os.Lchown)
			return path
		}, false},
		{"a folder in a shared folder with the sticky bit", func(t *testing.T, dir string) string {
			shared := mkdir(t, filepath.Join(dir, "shared"), 0o777|os.ModeSticky)
			return mkdir(t, filepath.Join(shared, "home"), 0o700)
		}, true},
		{"a folder in a folder anyone can change", func(t *testing.T, dir string) string {
			shared := mkdir(t, filepath.Join(dir, "shared"), 0o777)
			return mkdir(t, filepath.Join(shared, "home"), 0o700)
		}, false},
		{"a link in a folder anyone can change", func(t *testing.T, dir string) string {
			shared := mkdir(t, filepath.Join(dir, "shared"), 0o777)
			return symlink(t, mkdir(t, filepath.Join(dir, "real"), 0o700), filepath.Join(shared, "home"))
		}, false},
		{"a link to a folder in a folder anyone can change", func(t *testing.T, dir string) string {
			shared := mkdir(t, filepath.Join(dir, "shared"), 0o777)
			return symlink(t, mkdir(t, filepath.Join(shared, "real"), 0o700), filepath.Join(dir, "home"))
		}, false},
		{"a folder in another user's folder", func(t *testing.T, dir string) string {
			theirs := mkdir(t, filepath.Join(dir, "theirs"), 0o755)
			path := mkdir(t, filepath.Join(theirs, "home"), 0o700)
			giveAway(t, theirs, os.Chown)
			return path
		}, false},
		{"a folder reached through another user's link", func(t *testing.T, dir string) string {
			real := mkdir(t, filepath.Join(dir, "real"), 0o755)
			mkdir(t, filepath.Join(real, "home"), 0o700)
			via := symlink(t, real, filepath.Join(dir, "via"))
			giveAway(t, via, os.Lchown)
			return filepath.Join(via, "home")
		}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := tt.make(t, t.TempDir())
			err := CheckDir(path)
			if tt.wantOK != (err == nil) {
				t.Fatalf("CheckDir(%s) = %v, want ok %v", path, err, tt.wantOK)
			}
			if tt.wantOK {
				if info, err := os.Stat(path); err != nil || !info.IsDir() {
					t.Errorf("after CheckDir, %s is %v, %v; want a folder", path, info, err)
				}
			}
		})
	}
}

func TestCheckDirCreatesPrivateFolders(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bench")
	if err := CheckDir(path); err != nil {
		t.Fatal(err)
	}
	if info, err := os.Stat(path); err != nil || info.Mode().Perm() != 0o700 {
		t.Errorf("a new support folder has mode %v, %v; want 0700", info.Mode().Perm(), err)
	}
}

// mkdir makes a folder with exactly perm, whatever the umask, and returns
// its path.
func mkdir(t *testing.T, path string, perm os.FileMode) string {
	t.Helper()
	if err := os.Mkdir(path, perm); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, perm); err != nil {
		t.Fatal(err)
	}
	return path
}

// symlink makes a link at path to target, and returns path.
func symlink(t *testing.T, target, path string) string {
	t.Helper()
	if err := os.Symlink(target, path); err != nil {
		t.Fatal(err)
	}
	return path
}

// giveAway changes path's owner to otherUser with chown, skipping the test
// if it can't.
func giveAway(t *testing.T, path string, chown func(string, int, int) error) {
	t.Helper()
	if err := chown(path, otherUser, otherUser); err != nil {
		t.Skipf("can't give %s to another user: %v", path, err)
	}
}
