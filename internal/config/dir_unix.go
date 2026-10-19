//go:build unix

package config

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"syscall"
)

// checkPrivate returns why another user could change the files in dir, or
// nil if they can't: dir must be a folder that the user owns and that
// neither its group nor others can write to. If dir is a symbolic link, the
// link must belong to the user or to root, as its owner could point it
// elsewhere. The folders that hold dir, before and after following links,
// must be safe too, as whoever can change one of them could rename dir and
// put a folder of their own in its place.
func checkPrivate(dir string) error {
	uid := os.Getuid()
	link, err := os.Lstat(dir)
	if err != nil {
		return err
	}
	if link.Mode()&fs.ModeSymlink != 0 {
		if owner, ok := ownerOf(link); !ok || (owner != uid && owner != 0) {
			return fmt.Errorf("%s is a link that belongs to another user", dir)
		}
	}
	info, err := os.Stat(dir)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return fmt.Errorf("%s isn't a folder", dir)
	}
	if owner, ok := ownerOf(info); !ok || owner != uid {
		return fmt.Errorf("%s belongs to another user", dir)
	}
	if info.Mode().Perm()&0o022 != 0 {
		return fmt.Errorf("other users can change the files in %s; make it private with chmod 700", dir)
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		return err
	}
	real, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return err
	}
	if err := checkParents(abs, uid); err != nil {
		return err
	}
	if real != abs {
		return checkParents(real, uid)
	}
	return nil
}

// checkParents checks each folder that holds the absolute path, up to the
// root. Each must belong to the user or to root, and if its group or others
// can write to it, it must have the sticky bit, as /tmp does, so that they
// can't rename or remove what's in it. A link on the way, such as /tmp on a
// Mac, must belong to the user or to root too.
func checkParents(path string, uid int) error {
	for dir := filepath.Dir(path); ; dir = filepath.Dir(dir) {
		link, err := os.Lstat(dir)
		if err != nil {
			return err
		}
		if link.Mode()&fs.ModeSymlink != 0 {
			if owner, ok := ownerOf(link); !ok || (owner != uid && owner != 0) {
				return fmt.Errorf("%s is in %s, a link that belongs to another user", path, dir)
			}
		}
		info, err := os.Stat(dir)
		if err != nil {
			return err
		}
		if owner, ok := ownerOf(info); !ok || (owner != uid && owner != 0) {
			return fmt.Errorf("%s is in %s, which belongs to another user", path, dir)
		}
		if info.Mode().Perm()&0o022 != 0 && info.Mode()&fs.ModeSticky == 0 {
			return fmt.Errorf("%s is in %s, where other users can rename or replace it", path, dir)
		}
		if dir == filepath.Dir(dir) {
			return nil
		}
	}
}

// ownerOf returns the user ID that owns a file.
func ownerOf(info fs.FileInfo) (int, bool) {
	st, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return 0, false
	}
	return int(st.Uid), true
}
