package workspace

import (
	"errors"
	"fmt"
	"testing"

	"github.com/itbm/bench/internal/platform"
)

// errSlow is what a platform returns for an app that didn't answer in time.
var errSlow = errors.New("the app didn't respond in time")

func TestStartupKeepsItemsWhoseWindowsCouldNotBeRead(t *testing.T) {
	tests := []struct {
		name string
		// windowErr is what Window returns, or nil if it reads the window.
		windowErr error
	}{
		// Every app's window list timed out, and so did reading the window.
		{"window unreadable", errSlow},
		// The list timed out, but the window could be read afterwards.
		{"window readable", nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data := savedState(t, nil)
			var note platform.WindowID
			h := newHarness(t, setup{state: writeState(data), world: func(p *platform.Fake) {
				standardWorld(p)
				app, _ := p.App(obsidianID)
				ws, _ := p.Windows(app)
				note = ws[0].ID
				p.SetErr("Windows", fmt.Errorf("listing windows: %w", errSlow))
				if tt.windowErr != nil {
					p.SetErr("Window", fmt.Errorf("reading window: %w", tt.windowErr))
				}
			}})
			if got := h.itemOf(note); got != "w7" {
				t.Fatalf("Obsidian's window belongs to %q, want w7: it couldn't be listed, but it's still there", got)
			}
			// The app answers again: the item still has its window.
			h.p.SetErr("Windows", nil)
			h.p.SetErr("Window", nil)
			mustOK(t, h.e.Rearrange())
			h.settle()
			if got := h.itemOf(note); got != "w7" {
				t.Errorf("after the app answered, Obsidian's window belongs to %q, want w7", got)
			}
		})
	}
}

// ownedElsewhere is a platform whose Window says that some windows are on
// another desktop and belong to an app it names, as the macOS platform does
// for a window ID that has been given to another app's window since it was
// saved.
type ownedElsewhere struct {
	*platform.Fake
	owners map[platform.WindowID]string // Window ID to the owner's bundle ID.
}

func (o *ownedElsewhere) Window(id platform.WindowID) (platform.Window, error) {
	if owner, ok := o.owners[id]; ok {
		return platform.Window{ID: id, BundleID: owner}, fmt.Errorf("window %d: %w", id, platform.ErrOtherDesktop)
	}
	return o.Fake.Window(id)
}

func TestStartupChecksTheOwnerOfWindowsOnAnotherDesktop(t *testing.T) {
	tests := []struct {
		name      string
		owner     string
		wantBound bool
	}{
		{"the item's app", obsidianID, true},
		{"another app", "com.apple.controlcenter", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data := savedState(t, nil)
			var note platform.WindowID
			h := newHarness(t, setup{state: writeState(data),
				world: func(p *platform.Fake) {
					standardWorld(p)
					app, _ := p.App(obsidianID)
					ws, _ := p.Windows(app)
					note = ws[0].ID
					p.MoveToOtherDesktop(note)
				},
				options: func(o *Options) {
					o.Platform = &ownedElsewhere{Fake: o.Platform.(*platform.Fake),
						owners: map[platform.WindowID]string{note: tt.owner}}
				},
			})
			it := h.item("w7")
			if bound := h.itemOf(note) == "w7"; bound != tt.wantBound {
				t.Errorf("w7 bound to the window: %v, want %v", bound, tt.wantBound)
			}
			if unavailable := it.Status == StatusUnavailable; unavailable != tt.wantBound {
				t.Errorf("w7: %s %q", it.Status, it.StatusText)
			}
		})
	}
}

// readsAs is a platform whose Window reads some windows as it's told to,
// as the macOS platform does for a window ID that now names another window.
type readsAs struct {
	*platform.Fake
	windows map[platform.WindowID]platform.Window
}

func (r *readsAs) Window(id platform.WindowID) (platform.Window, error) {
	if w, ok := r.windows[id]; ok {
		return w, nil
	}
	return r.Fake.Window(id)
}

func TestStartupChecksWindowsTheScanMissed(t *testing.T) {
	tests := []struct {
		name      string
		bundleID  string
		subrole   string
		wantBound bool
	}{
		{"the item's app's window", obsidianID, platform.SubroleStandard, true},
		{"another app's window", "com.apple.controlcenter", platform.SubroleStandard, false},
		{"the item's app's dialog", obsidianID, "AXDialog", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data := savedState(t, nil)
			var note platform.WindowID
			var pid int
			h := newHarness(t, setup{state: writeState(data),
				world: func(p *platform.Fake) {
					standardWorld(p)
					app, _ := p.App(obsidianID)
					ws, _ := p.Windows(app)
					note, pid = ws[0].ID, app.PID
					// No app's windows can be listed, so the scan finds none.
					p.SetErr("Windows", fmt.Errorf("listing windows: %w", errSlow))
				},
				options: func(o *Options) {
					if tt.bundleID != obsidianID {
						pid += 1000
					}
					o.Platform = &readsAs{Fake: o.Platform.(*platform.Fake), windows: map[platform.WindowID]platform.Window{
						note: {ID: note, PID: pid, BundleID: tt.bundleID, Title: "Weekly note", Subrole: tt.subrole},
					}}
				},
			})
			it := h.item("w7")
			if bound := h.itemOf(note) == "w7"; bound != tt.wantBound {
				t.Fatalf("w7 bound to the window: %v, want %v", bound, tt.wantBound)
			}
			if tt.wantBound && it.Title != "Weekly note" {
				t.Errorf("w7's title = %q, want the window's, %q", it.Title, "Weekly note")
			}
		})
	}
}
