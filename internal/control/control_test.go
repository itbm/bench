package control

import (
	"slices"
	"strings"
	"testing"
)

func TestParseLink(t *testing.T) {
	good := []struct {
		link string
		want Request
	}{
		{"bench://tab/Code", Request{Action: ActionTab, Arg: "Code"}},
		{"bench://tab/code", Request{Action: ActionTab, Arg: "code"}},
		{"bench://tab/2", Request{Action: ActionTab, Arg: "2"}},
		{"bench://tab/Mail%20and%20chat", Request{Action: ActionTab, Arg: "Mail and chat"}},
		{"bench://tab/A%2FB", Request{Action: ActionTab, Arg: "A/B"}},
		{"bench://tab/C%23", Request{Action: ActionTab, Arg: "C#"}},
		{"bench://tab/Caf%C3%A9", Request{Action: ActionTab, Arg: "Café"}},
		{"bench://tab/Café", Request{Action: ActionTab, Arg: "Café"}},
		{"bench://tab/a+b", Request{Action: ActionTab, Arg: "a+b"}},
		{"bench://tab/Code/", Request{Action: ActionTab, Arg: "Code"}},
		{"bench://next", Request{Action: ActionNext}},
		{"bench://next/", Request{Action: ActionNext}},
		{"bench://previous", Request{Action: ActionPrevious}},
		{"bench://last", Request{Action: ActionLast}},
		{"bench://jump", Request{Action: ActionJump}},
		{"bench://overview", Request{Action: ActionOverview}},
		{"BENCH://Next", Request{Action: ActionNext}},
		{"bench://TAB/Code", Request{Action: ActionTab, Arg: "Code"}},
	}
	for _, c := range good {
		got, err := ParseLink(c.link)
		if err != nil {
			t.Errorf("ParseLink(%q): %v", c.link, err)
			continue
		}
		if got != c.want {
			t.Errorf("ParseLink(%q) = %+v, want %+v", c.link, got, c.want)
		}
	}

	bad := []struct {
		link, says string
	}{
		{"bench://launch/Notes", "Links can only switch tabs or show Jump to… or every tab. Bench can't open bench://launch/Notes."},
		{"bench://move-to-tab/2", "Links can only switch tabs or show Jump to… or every tab. Bench can't open bench://move-to-tab/2."},
		{"bench://quit", "Links can only switch tabs or show Jump to… or every tab. Bench can't open bench://quit."},
		{"bench://list", "Links can only switch tabs"},
		{"bench://", "Links can only switch tabs"},
		{"bench:next", "Links can only switch tabs"},
		{"bench:///tab/Code", "Links can only switch tabs"},
		{"http://tab/Code", "Links can only switch tabs"},
		{"bench://tab", "A tab link needs a name or number, as in bench://tab/Code. Bench can't open bench://tab."},
		{"bench://tab/", "A tab link needs a name or number"},
		{"bench://tab//Code", "A tab link needs a name or number"},
		{"bench://tab/A/B", "A tab link needs a name or number"},
		{"bench://tab/%FF", "A tab link needs a name or number"},
		{"bench://next/now", "Nothing can come after bench://next. Bench can't open bench://next/now."},
		{"bench://jump/x", "Nothing can come after bench://jump."},
		// A tab's name with #, ?, or % that isn't encoded would name
		// another tab, or none: bench://tab/C# would be the tab C.
		{"bench://tab/C#", "In a link, write # as %23, ? as %3F, and % as %25. Bench can't open bench://tab/C#."},
		{"bench://tab/C#x", "write # as %23"},
		{"bench://tab/What?", "write # as %23, ? as %3F"},
		{"bench://Tab/Code?close=1", "write # as %23, ? as %3F"},
		{"bench://tab/100%", "and % as %25"},
		{"bench://tab/%zz", "and % as %25"},
		{"bench://next?x", "Links can only switch tabs"},
		{"bench://jump#", "Links can only switch tabs"},
		{"bench://me@tab/Code", "Links can only switch tabs"},
		{"bench://tab:80/Code", "Links can only switch tabs"},
	}
	for _, c := range bad {
		_, err := ParseLink(c.link)
		if err == nil {
			t.Errorf("ParseLink(%q) took it", c.link)
			continue
		}
		if !strings.Contains(err.Error(), c.says) {
			t.Errorf("ParseLink(%q) = %q, want it to say %q", c.link, err, c.says)
		}
		// What to do comes first, as the app's one-line toast cuts the end.
		if !strings.Contains(err.Error(), ". Bench can't open ") || !strings.HasSuffix(err.Error(), ".") {
			t.Errorf("ParseLink(%q) = %q, want what links can do, then the link", c.link, err)
		}
	}
}

func TestParseLinkNamesTheLinkSafely(t *testing.T) {
	// A link comes from anywhere: its message shows no line breaks or
	// characters that turn text around, and only the start of a long one,
	// so that the toast's line has room for what links can do.
	_, err := ParseLink("bench://quit/" + strings.Repeat("x", 200))
	want := "Links can only switch tabs or show Jump to… or every tab. Bench can't open bench://quit/" + strings.Repeat("x", 27) + "…."
	if err == nil || err.Error() != want {
		t.Errorf("long link: %v, want %q", err, want)
	}
	_, err = ParseLink("bench://quit/a%0Ab‮")
	if err == nil || strings.ContainsAny(err.Error(), "\n‮") {
		t.Errorf("link with control characters: %q", err)
	}
}

func TestIsLink(t *testing.T) {
	for s, want := range map[string]bool{
		"bench://tab/Code": true, "Bench://next": true, "bench:x": true,
		"tab": false, "--": false, "https://bench/": false, "": false, "-psn_0_1": false,
	} {
		if got := IsLink(s); got != want {
			t.Errorf("IsLink(%q) = %v, want %v", s, got, want)
		}
	}
}

func TestParseCommand(t *testing.T) {
	good := []struct {
		args []string
		want Request
	}{
		{[]string{"tab", "Code"}, Request{Action: ActionTab, Arg: "Code"}},
		{[]string{"tab", "Mail and chat"}, Request{Action: ActionTab, Arg: "Mail and chat"}},
		{[]string{"tab", "--", "-x"}, Request{Action: ActionTab, Arg: "-x"}},
		{[]string{"tab", "-"}, Request{Action: ActionTab, Arg: "-"}},
		{[]string{"tab", "3"}, Request{Action: ActionTab, Arg: "3"}},
		{[]string{"next"}, Request{Action: ActionNext}},
		{[]string{"next", "--"}, Request{Action: ActionNext}},
		{[]string{"previous"}, Request{Action: ActionPrevious}},
		{[]string{"last"}, Request{Action: ActionLast}},
		{[]string{"move-to-tab", "Mail"}, Request{Action: ActionMoveToTab, Arg: "Mail"}},
		{[]string{"launch", "--", "Notes"}, Request{Action: ActionLaunch, Arg: "Notes"}},
	}
	for _, c := range good {
		got, err := ParseCommand(c.args)
		if err != nil {
			t.Errorf("ParseCommand(%q): %v", c.args, err)
			continue
		}
		if got != c.want {
			t.Errorf("ParseCommand(%q) = %+v, want %+v", c.args, got, c.want)
		}
		// What the bench command sends reads back the same.
		again, err := ParseCommand(CommandArgs(got))
		if err != nil || again != got {
			t.Errorf("ParseCommand(CommandArgs(%+v)) = %+v, %v", got, again, err)
		}
	}

	bad := []struct {
		args []string
		says string
	}{
		{nil, "there's no command"},
		{[]string{"jump"}, "there's no command called jump"},
		{[]string{"overview"}, "there's no command called overview"},
		{[]string{"list"}, "there's no command called list"},
		{[]string{"bench://next"}, "there's no command called bench://next"},
		{[]string{"tab"}, "bench tab needs a tab's name or number"},
		{[]string{"tab", ""}, "bench tab needs a tab's name or number"},
		{[]string{"tab", "--"}, "bench tab needs a tab's name or number"},
		{[]string{"launch"}, "bench launch needs a launcher's name"},
		{[]string{"tab", "Mail", "and", "chat"}, "bench tab takes one argument, a tab's name or number: put a name with spaces in quotes"},
		{[]string{"tab", "--json"}, "bench tab has no flag --json"},
		{[]string{"next", "-n"}, "bench next has no flag -n"},
		{[]string{"next", "2"}, "bench next takes nothing after it"},
		{[]string{"move-to-tab", "\xff"}, "bench move-to-tab needs a tab's name or number in UTF-8"},
	}
	for _, c := range bad {
		_, err := ParseCommand(c.args)
		if err == nil || err.Error() != c.says {
			t.Errorf("ParseCommand(%q) = %v, want %q", c.args, err, c.says)
		}
	}
}

func TestIsCommand(t *testing.T) {
	for _, name := range CommandNames() {
		if !IsCommand([]string{name}) {
			t.Errorf("IsCommand(%q) is false", name)
		}
	}
	for _, args := range [][]string{nil, {""}, {"jump"}, {"list"}, {"version"}, {"bench://next"}, {"-psn_0_1"}} {
		if IsCommand(args) {
			t.Errorf("IsCommand(%q) is true", args)
		}
	}
}

func TestShown(t *testing.T) {
	cases := map[string]string{
		"bench://tab/Code":      "bench://tab/Code",
		"a\nb\tc\x00d":          "a?b?c?d",
		"x‮y":                   "x?y",
		"\xffok":                "?ok",
		strings.Repeat("é", 80): strings.Repeat("é", 80),
		strings.Repeat("é", 81): strings.Repeat("é", 80) + "…",
		strings.Repeat("a", 90): strings.Repeat("a", 80) + "…",
	}
	for in, want := range cases {
		if got := Shown(in); got != want {
			t.Errorf("Shown(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestFindTab(t *testing.T) {
	names := []string{"Code", "Mail", "code", "2", "Research"}
	for arg, want := range map[string]int{
		"Code": 0, "code": 2, "CODE": 0, "mail": 1,
		// A tab named "2" comes before the second tab.
		"2": 3, "1": 0, "5": 4, "3": 2,
		"0": -1, "6": -1, "-1": -1, "01": -1, "+1": -1, " 1": -1, "": -1, "Nope": -1,
	} {
		if got := FindTab(names, arg); got != want {
			t.Errorf("FindTab(%q) = %d, want %d", arg, got, want)
		}
	}
	if got := FindTab(nil, "1"); got != -1 {
		t.Errorf("FindTab with no tabs = %d", got)
	}
}

func TestCommandNames(t *testing.T) {
	names := CommandNames()
	if !slices.Equal(names, []string{"tab", "next", "previous", "last", "move-to-tab", "launch"}) {
		t.Errorf("CommandNames() = %q", names)
	}
}
