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

// The permissions for files Bench creates. An existing file keeps its own.
const (
	filePerm = 0o644
	dirPerm  = 0o755
)

// saveMu makes saves in this process take turns, so that config.toml.bak is
// always the version before the one written.
var saveMu sync.Mutex

// Dir returns Bench's support folder: $BENCH_HOME if it's set, otherwise
// "Bench" in os.UserConfigDir() (~/Library/Application Support/Bench on a
// Mac). It doesn't create it.
func Dir() (string, error) {
	if dir := os.Getenv("BENCH_HOME"); dir != "" {
		return dir, nil
	}
	base, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("finding Bench's support folder: %w", err)
	}
	return filepath.Join(base, "Bench"), nil
}

// Load reads and parses a config file. If the file doesn't exist, it writes
// DefaultTOML there (creating the folder) and returns the default config.
// Problems in the file are returned as problems, with a nil config; err is
// for files that can't be read or written.
func Load(path string) (cfg *Config, problems []Problem, err error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
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

	target := resolveLink(path)
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
// it isn't a link or the link can't be followed.
func resolveLink(path string) string {
	info, err := os.Lstat(path)
	if err != nil || info.Mode()&fs.ModeSymlink == 0 {
		return path
	}
	target, err := filepath.EvalSymlinks(path)
	if err != nil {
		return path
	}
	return target
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
