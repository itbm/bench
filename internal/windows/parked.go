package windows

import (
	"cmp"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/itbm/bench/internal/layout"
	"github.com/itbm/bench/internal/platform"
)

// ParkedWindow is a window the manager has parked, as recorded in the
// parked-windows file.
type ParkedWindow struct {
	ID       platform.WindowID `json:"id"`
	PID      int               `json:"pid"`
	BundleID string            `json:"bundleId"`
	Title    string            `json:"title"`
	// Frame is where the window was before it was parked, in global
	// coordinates.
	Frame layout.Rect `json:"frame"`
	// Screen is the ID of the screen it was parked on.
	Screen string `json:"screen,omitempty"`
	// Minimised is set when the manager minimised the window rather than
	// moving it, because every place to park it would have shown part of
	// it on another display. Putting it back unminimises it.
	Minimised bool `json:"minimised,omitempty"`
}

// HiddenApp is an app the manager has hidden, as recorded in the
// parked-windows file.
type HiddenApp struct {
	PID      int    `json:"pid"`
	BundleID string `json:"bundleId"`
	Name     string `json:"name,omitempty"`
}

// ParkedFile is the parked-windows file (parked.json): what a run of Bench
// has moved out of sight, so that it can be put back after a crash.
type ParkedFile struct {
	Windows []ParkedWindow `json:"windows"`
	Hidden  []HiddenApp    `json:"hidden,omitempty"`
}

// Empty reports whether the file records nothing.
func (f ParkedFile) Empty() bool { return len(f.Windows) == 0 && len(f.Hidden) == 0 }

// ReadParked reads a parked-windows file. A missing file reads as an empty
// one.
func ReadParked(path string) (ParkedFile, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return ParkedFile{}, nil
	}
	if err != nil {
		return ParkedFile{}, fmt.Errorf("reading the parked-windows file: %w", err)
	}
	var f ParkedFile
	if err := json.Unmarshal(data, &f); err != nil {
		return ParkedFile{}, fmt.Errorf("reading the parked-windows file %s: %w", path, err)
	}
	return f, nil
}

// WriteParked writes a parked-windows file atomically, through a temporary
// file in the same folder (created if needed), or removes it if f is empty.
func WriteParked(path string, f ParkedFile) error {
	if f.Empty() {
		if err := os.Remove(path); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return fmt.Errorf("removing the parked-windows file: %w", err)
		}
		return nil
	}
	data, err := json.MarshalIndent(f, "", "  ")
	if err != nil {
		return fmt.Errorf("writing the parked-windows file: %w", err)
	}
	data = append(data, '\n')
	if err := writeFileAtomic(path, data); err != nil {
		return fmt.Errorf("writing the parked-windows file: %w", err)
	}
	return nil
}

// writeFileAtomic writes data to a temporary file next to path and renames
// it over path, so readers see the old file or the new one, never part of
// one. It flushes the temporary file before the rename, and the folder
// after it, so that a power cut or a kernel panic can't leave path empty or
// cut short: without that, the file system may store the rename before the
// data.
func writeFileAtomic(path string, data []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, "."+filepath.Base(path)+".*.tmp")
	if err != nil {
		return err
	}
	name := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(name)
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		os.Remove(name)
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(name)
		return err
	}
	if err := os.Rename(name, path); err != nil {
		os.Remove(name)
		return err
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

// fileLocked returns what the parked-windows file should hold, sorted so
// that it's the same for the same windows.
func (m *Manager) fileLocked() ParkedFile {
	var f ParkedFile
	for _, w := range m.parked {
		f.Windows = append(f.Windows, w)
	}
	slices.SortFunc(f.Windows, func(a, b ParkedWindow) int { return cmp.Compare(a.ID, b.ID) })
	for _, app := range m.hidden {
		f.Hidden = append(f.Hidden, HiddenApp{PID: app.PID, BundleID: app.BundleID, Name: app.Name})
	}
	slices.SortFunc(f.Hidden, func(a, b HiddenApp) int {
		if c := strings.Compare(a.BundleID, b.BundleID); c != 0 {
			return c
		}
		return cmp.Compare(a.PID, b.PID)
	})
	return f
}

// saveLocked updates the parked-windows file, if the manager has one.
func (m *Manager) saveLocked() error {
	if m.path == "" {
		return nil
	}
	err := WriteParked(m.path, m.fileLocked())
	m.fileStale = err != nil
	return err
}
