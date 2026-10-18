package term

import (
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
)

func TestSessionEnv(t *testing.T) {
	const dir = "/Users/me/code"
	tests := []struct {
		name  string
		base  []string
		extra []string
		want  []string // compared in any order; each key appears once
	}{
		{
			name: "adds terminal variables, PWD, and LANG",
			base: []string{"HOME=/Users/me", "PATH=/usr/bin"},
			want: []string{"HOME=/Users/me", "PATH=/usr/bin", "TERM=xterm-256color",
				"COLORTERM=truecolor", "TERM_PROGRAM=Bench", "PWD=" + dir, "LANG=en_GB.UTF-8"},
		},
		{
			name: "keeps LANG",
			base: []string{"LANG=fr_FR.UTF-8"},
			want: []string{"LANG=fr_FR.UTF-8", "TERM=xterm-256color", "COLORTERM=truecolor", "TERM_PROGRAM=Bench", "PWD=" + dir},
		},
		{
			name: "sets an empty LANG",
			base: []string{"LANG="},
			want: []string{"LANG=en_GB.UTF-8", "TERM=xterm-256color", "COLORTERM=truecolor", "TERM_PROGRAM=Bench", "PWD=" + dir},
		},
		{
			name: "replaces TERM, TERM_PROGRAM, and Bench's own PWD",
			base: []string{"TERM=dumb", "TERM_PROGRAM=Apple_Terminal", "LANG=en_GB.UTF-8", "PWD=/Applications"},
			want: []string{"TERM=xterm-256color", "TERM_PROGRAM=Bench", "LANG=en_GB.UTF-8", "COLORTERM=truecolor", "PWD=" + dir},
		},
		{
			name:  "extra comes last and wins",
			base:  []string{"LANG=en_GB.UTF-8"},
			extra: []string{"EDITOR=vim", "TERM=screen", "bad", "=nokey"},
			want:  []string{"LANG=en_GB.UTF-8", "TERM=screen", "COLORTERM=truecolor", "TERM_PROGRAM=Bench", "PWD=" + dir, "EDITOR=vim"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := sessionEnv(tt.base, dir, tt.extra)
			gotSorted, wantSorted := slices.Sorted(slices.Values(got)), slices.Sorted(slices.Values(tt.want))
			if !reflect.DeepEqual(gotSorted, wantSorted) {
				t.Errorf("sessionEnv() = %q, want %q", gotSorted, wantSorted)
			}
		})
	}
}

func TestChooseShell(t *testing.T) {
	dir := t.TempDir()
	fake := filepath.Join(dir, "fakesh")
	writeExecutable(t, fake, "#!/bin/sh\n")
	missing := filepath.Join(dir, "missing")

	tests := []struct {
		name     string
		explicit string
		env      string
		want     string
		wantErr  bool
	}{
		{name: "explicit", explicit: fake, env: "/bin/sh", want: fake},
		{name: "explicit that doesn't exist", explicit: missing, wantErr: true},
		{name: "$SHELL", env: fake, want: fake},
		{name: "$SHELL that doesn't exist falls back", env: missing, want: firstExisting(t, fallbackShells)},
		{name: "no $SHELL falls back", want: firstExisting(t, fallbackShells)},
		{name: "a name in PATH", explicit: "sh", want: lookPath(t, "sh")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := chooseShell(tt.explicit, tt.env)
			if (err != nil) != tt.wantErr {
				t.Fatalf("chooseShell(%q, %q) error = %v, want error %v", tt.explicit, tt.env, err, tt.wantErr)
			}
			if got != tt.want {
				t.Errorf("chooseShell(%q, %q) = %q, want %q", tt.explicit, tt.env, got, tt.want)
			}
		})
	}
}

func TestShellArgs(t *testing.T) {
	if got, want := shellArgs(""), []string{"-l"}; !reflect.DeepEqual(got, want) {
		t.Errorf("shellArgs(\"\") = %q, want %q", got, want)
	}
	if got, want := shellArgs("go test ./..."), []string{"-l", "-c", "go test ./..."}; !reflect.DeepEqual(got, want) {
		t.Errorf("shellArgs(command) = %q, want %q", got, want)
	}
}

func TestResolveDir(t *testing.T) {
	home := t.TempDir()
	code := filepath.Join(home, "code")
	if err := os.Mkdir(code, 0o755); err != nil {
		t.Fatal(err)
	}
	other := t.TempDir()

	tests := []struct {
		name       string
		dir        string
		want       string
		wantNotice string
	}{
		{name: "empty", dir: "", want: home},
		{name: "tilde", dir: "~", want: home},
		{name: "in home", dir: "~/code", want: code},
		{name: "in home, with a slash", dir: "~/code/", want: code},
		{name: "relative", dir: "code", want: code},
		{name: "absolute", dir: other, want: other},
		{name: "missing", dir: "~/nope", want: home, wantNotice: "bench: ~/nope isn't a folder, so this terminal starts in ~.\r\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, notice := resolveDir(tt.dir, home)
			if got != tt.want || notice != tt.wantNotice {
				t.Errorf("resolveDir(%q) = %q, %q; want %q, %q", tt.dir, got, notice, tt.want, tt.wantNotice)
			}
		})
	}
}

func TestAbbreviateAndExpand(t *testing.T) {
	const home = "/Users/me"
	abbrev := []struct{ in, want string }{
		{"/Users/me", "~"},
		{"/Users/me/", "~"},
		{"/Users/me/code/bench", "~/code/bench"},
		{"/Users/meg", "/Users/meg"},
		{"/tmp", "/tmp"},
		{"", ""},
	}
	for _, tt := range abbrev {
		if got := abbreviate(tt.in, home); got != tt.want {
			t.Errorf("abbreviate(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
	if got := abbreviate("/Users/me/x", ""); got != "/Users/me/x" {
		t.Errorf("abbreviate with no home = %q, want the path unchanged", got)
	}

	expand := []struct{ in, want string }{
		{"~", "/Users/me"},
		{"~/code", "/Users/me/code"},
		{"~other/code", "~other/code"},
		{"/tmp", "/tmp"},
	}
	for _, tt := range expand {
		if got := expandHome(tt.in, home); got != tt.want {
			t.Errorf("expandHome(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestAbsCString(t *testing.T) {
	tests := []struct {
		in     string
		want   string
		wantOK bool
	}{
		{"/Users/me/code\x00\x00garbage", "/Users/me/code", true},
		{"/\x00", "/", true},
		{"\x00/Users", "", false},
		{"relative\x00", "relative", false},
		{"/no/terminator", "", false},
		{"", "", false},
	}
	for _, tt := range tests {
		got, ok := absCString([]byte(tt.in))
		if ok != tt.wantOK || (ok && got != tt.want) {
			t.Errorf("absCString(%q) = %q, %v; want %q, %v", tt.in, got, ok, tt.want, tt.wantOK)
		}
	}
}

func writeExecutable(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o755); err != nil {
		t.Fatal(err)
	}
}

func firstExisting(t *testing.T, paths []string) string {
	t.Helper()
	for _, p := range paths {
		if isExecutable(p) {
			return p
		}
	}
	t.Skipf("none of %s exists", strings.Join(paths, ", "))
	return ""
}

func lookPath(t *testing.T, name string) string {
	t.Helper()
	p, err := findShell(name)
	if err != nil {
		t.Skipf("%s isn't in PATH", name)
	}
	return p
}
