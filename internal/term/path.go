package term

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
)

// homeDir returns the user's home folder, or "" if it isn't known.
func homeDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Clean(home)
}

// expandHome turns "~" and "~/..." into paths in home. Other paths, including
// "~user", are returned unchanged. An empty home leaves path unchanged.
func expandHome(path, home string) string {
	if home == "" {
		return path
	}
	switch {
	case path == "~":
		return home
	case strings.HasPrefix(path, "~/"):
		return filepath.Join(home, path[2:])
	}
	return path
}

// abbreviate shows folders in home with ~, as shells do: home itself is "~",
// and anything inside it is "~/...". Other paths are cleaned but otherwise
// unchanged.
func abbreviate(path, home string) string {
	if path == "" {
		return ""
	}
	path = filepath.Clean(path)
	if home == "" || home == "/" {
		return path
	}
	if path == home {
		return "~"
	}
	if rest, ok := strings.CutPrefix(path, home+string(filepath.Separator)); ok {
		return "~/" + rest
	}
	return path
}

// FullPath returns a terminal's folder as the full path a session starts
// in: "~" and "~/..." in the home folder, as are other relative folders,
// cleaned. An empty folder is the home folder. It doesn't check that the
// folder exists.
func FullPath(dir string) string {
	home := homeDir()
	if dir == "" {
		dir = "~"
	}
	path := expandHome(dir, home)
	if !filepath.IsAbs(path) && home != "" {
		path = filepath.Join(home, path)
	}
	return filepath.Clean(path)
}

// ShortPath shows a path as terminals' folders are written, with the home
// folder as ~, as titles show it.
func ShortPath(path string) string {
	return abbreviate(path, homeDir())
}

// absCString returns the NUL-terminated string at the start of b, if it's an
// absolute path. It reads paths that the kernel fills in.
func absCString(b []byte) (string, bool) {
	i := bytes.IndexByte(b, 0)
	if i < 0 {
		return "", false
	}
	path := string(b[:i])
	return path, strings.HasPrefix(path, "/")
}

// isDir reports whether path is an existing folder.
func isDir(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

// isExecutable reports whether path is an existing file that someone can
// run.
func isExecutable(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir() && info.Mode()&0o111 != 0
}
