package config

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sync"
)

// File names in the support folder.
const (
	ConfigFile = "config.toml"
	BackupFile = "config.toml.bak"
	StateFile  = "state.json"
	BadState   = "state.json.bad"
	ParkedFile = "parked.json" // Parked windows, for restoring after a crash.
	VSCodeFile = "vscode.json" // The running VS Code for the Web server, for stopping it after a crash.
)

// The permissions for files and folders Bench creates: only the user can
// read them, as config.toml can hold commands and addresses. An existing
// file keeps its own.
const (
	filePerm = 0o600
	dirPerm  = 0o700
)

// saveMu makes saves in this process take turns, so that config.toml.bak is
// always the version before the one written.
var saveMu sync.Mutex

// Dir returns Bench's support folder: $BENCH_HOME if it's set, otherwise
// "Bench" in os.UserConfigDir() (~/Library/Application Support/Bench on a
// Mac). It checks $BENCH_HOME's folder with CheckDir, which creates it if
// it doesn't exist, but it doesn't create the usual folder.
func Dir() (string, error) {
	if dir := os.Getenv("BENCH_HOME"); dir != "" {
		if err := CheckDir(dir); err != nil {
			return "", fmt.Errorf("using BENCH_HOME: %w", err)
		}
		return dir, nil
	}
	base, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("finding Bench's support folder: %w", err)
	}
	return filepath.Join(base, "Bench"), nil
}

// CheckDir checks a support folder given by $BENCH_HOME or a --dir flag,
// which may be somewhere shared, such as /tmp. Bench runs the commands in
// config.toml, stops the processes that vscode.json names, and moves the
// windows that parked.json lists, so another user mustn't be able to put
// files there. CheckDir creates the folder, private to the user, if it
// doesn't exist, so that no one else can create it first, and refuses one
// that another user owns or that others can write to, or that is in a
// folder where others could replace it.
func CheckDir(dir string) error {
	if err := os.MkdirAll(dir, dirPerm); err != nil {
		return fmt.Errorf("creating the support folder: %w", err)
	}
	if err := checkPrivate(dir); err != nil {
		return fmt.Errorf("can't use %s as the support folder: %w", dir, err)
	}
	return nil
}

// Load reads and parses a config file. If the file doesn't exist, it writes
// DefaultTOML there (creating the folder) and returns the default config.
// Problems in the file are returned as problems, with a nil config; err is
// for files that can't be read or written, including a symbolic link to a
// file that isn't there, which Load leaves alone rather than replace.
func Load(path string) (cfg *Config, problems []Problem, err error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		// The link may point into a folder that isn't there yet, such as
		// a synced folder at login. Writing the default config would
		// replace the link, and Bench would then use that copy from then
		// on without saying so.
		if info, lerr := os.Lstat(path); lerr == nil && info.Mode()&fs.ModeSymlink != 0 {
			return nil, nil, fmt.Errorf("reading the config: %w", danglingLink(path))
		}
		if err := writeFileAtomic(path, DefaultTOML(), filePerm); err != nil {
			return nil, nil, fmt.Errorf("writing the default config: %w", err)
		}
		return Default(), nil, nil
	}
	if err != nil {
		return nil, nil, fmt.Errorf("reading the config: %w", err)
	}
	return Parse(data)
}

// Save validates a config and writes it to path atomically, first copying
// any existing file to path plus ".bak" (config.toml.bak for config.toml) in
// the same folder. It refuses to save a config with problems, returning them
// as a Problems error. If path is a symbolic link, Save writes the file it
// points to, so the link survives.
func Save(path string, c *Config) error {
	if c == nil {
		return errors.New("saving the config: there's no config to save")
	}
	if problems := c.Validate(); len(problems) > 0 {
		return fmt.Errorf("can't save the config: %w", Problems(problems))
	}
	data := Format(c)
	// If Format and Parse disagree, which would be a bug in this package,
	// saving would write a file that Bench can't read back.
	if _, problems, err := Parse(data); err != nil {
		return fmt.Errorf("can't save the config, because it doesn't read back: %w", err)
	} else if len(problems) > 0 {
		return fmt.Errorf("can't save the config, because it doesn't read back: %w", Problems(problems))
	}

	saveMu.Lock()
	defer saveMu.Unlock()

	target, err := resolveLink(path)
	if err != nil {
		return fmt.Errorf("saving the config: %w", err)
	}
	perm := os.FileMode(filePerm)
	old, err := os.ReadFile(target)
	switch {
	case err == nil:
		if info, err := os.Stat(target); err == nil {
			perm = info.Mode().Perm()
		}
		if err := writeFileAtomic(path+".bak", old, perm); err != nil {
			return fmt.Errorf("keeping a copy of the previous config: %w", err)
		}
	case !errors.Is(err, fs.ErrNotExist):
		return fmt.Errorf("reading the previous config: %w", err)
	}
	if err := writeFileAtomic(target, data, perm); err != nil {
		return fmt.Errorf("saving the config: %w", err)
	}
	return nil
}

// resolveLink returns the file a symbolic link points to, or path itself if
// it isn't a link. A link that can't be followed is an error: writing path
// itself would replace the link.
func resolveLink(path string) (string, error) {
	info, err := os.Lstat(path)
	if err != nil || info.Mode()&fs.ModeSymlink == 0 {
		return path, nil
	}
	target, err := filepath.EvalSymlinks(path)
	if errors.Is(err, fs.ErrNotExist) {
		return "", danglingLink(path)
	}
	if err != nil {
		return "", fmt.Errorf("following the link %s: %w", path, err)
	}
	return target, nil
}

// danglingLink describes a symbolic link whose file isn't there.
func danglingLink(path string) error {
	if target, err := os.Readlink(path); err == nil {
		return fmt.Errorf("%s is a link to %s, which isn't there", path, target)
	}
	return fmt.Errorf("%s is a link to a file that isn't there", path)
}

// writeFileAtomic writes data to a temporary file in path's folder and
// renames it over path, so that readers see the old file or the new one,
// never part of either. It creates the folder if needed.
func writeFileAtomic(path string, data []byte, perm os.FileMode) (err error) {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, dirPerm); err != nil {
		return fmt.Errorf("creating %s: %w", dir, err)
	}
	f, err := os.CreateTemp(dir, "."+filepath.Base(path)+".*.tmp")
	if err != nil {
		return fmt.Errorf("creating a temporary file in %s: %w", dir, err)
	}
	tmp := f.Name()
	defer func() {
		if err != nil {
			_ = f.Close()
			_ = os.Remove(tmp)
		}
	}()
	if _, err := f.Write(data); err != nil {
		return fmt.Errorf("writing %s: %w", tmp, err)
	}
	if err := f.Chmod(perm); err != nil {
		return fmt.Errorf("setting the permissions of %s: %w", tmp, err)
	}
	if err := f.Sync(); err != nil {
		return fmt.Errorf("flushing %s: %w", tmp, err)
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("closing %s: %w", tmp, err)
	}
	if err := os.Rename(tmp, path); err != nil {
		return fmt.Errorf("replacing %s: %w", path, err)
	}
	syncDir(dir)
	return nil
}

// syncDir flushes a folder's entries, so that a rename survives a power cut.
// Not every file system supports it, so failures are ignored.
func syncDir(dir string) {
	d, err := os.Open(dir)
	if err != nil {
		return
	}
	_ = d.Sync()
	_ = d.Close()
}
