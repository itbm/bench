//go:build darwin || server

package ui

import (
	"errors"
	"reflect"
	"testing"

	"github.com/itbm/bench/internal/workspace"
)

// TestWindowsTakeDrops checks that Bench's windows take drops from the
// Finder: the main window, a popped-out terminal's, and the drop-down's.
func TestWindowsTakeDrops(t *testing.T) {
	look := windowLook{}
	for name, on := range map[string]bool{
		"main":      mainWindowOptions(workspace.WindowStart{}, false).EnableFileDrop,
		"terminal":  terminalWindowOptions("w4", "zsh", look).EnableFileDrop,
		"drop-down": dropdownWindowOptions("w4", "zsh", look).EnableFileDrop,
	} {
		if !on {
			t.Errorf("the %s window doesn't take drops from the Finder", name)
		}
	}
	if acknowledgementsWindowOptions(look).EnableFileDrop {
		t.Errorf("the Acknowledgements window takes drops, but has nowhere to put them")
	}
}

// TestDropRefusalShowsWhereItWasDropped checks that the shell hands a drop
// to the engine, with what it's on, and shows a refusal in the window that
// got the drop: Bench's window's toast, or a terminal's own window's.
func TestDropRefusalShowsWhereItWasDropped(t *testing.T) {
	var got []any
	refuse := errors.New("The command has finished. Press Return, then drop again.")
	var answer error
	s := &appShell{
		dropFiles: func(target workspace.DropTarget, paths []string) error {
			got = append(got, target, paths)
			return answer
		},
		toast:   func(message string) { got = append(got, "toast: "+message) },
		command: func(c workspace.Command) { got = append(got, c) },
	}
	on := map[string]string{"data-file-drop-target": "terminal", "data-drop-item": "w4"}
	target := workspace.DropTarget{Kind: workspace.DropTerminal, Item: "w4"}
	paths := []string{"/tmp/a b"}

	s.dropped("w4", on, paths)
	if want := []any{target, paths}; !reflect.DeepEqual(got, want) {
		t.Errorf("a drop taken: %v, want %v", got, want)
	}
	got, answer = nil, refuse
	s.dropped("w4", on, paths)
	if want := []any{target, paths, workspace.Command{Name: "drop-refused", ItemID: "w4", Text: refuse.Error()}}; !reflect.DeepEqual(got, want) {
		t.Errorf("a drop refused on a terminal's window: %v, want %v", got, want)
	}
	got = nil
	s.dropped("", on, paths)
	if want := []any{target, paths, "toast: " + refuse.Error()}; !reflect.DeepEqual(got, want) {
		t.Errorf("a drop refused on Bench's window: %v, want %v", got, want)
	}
}
