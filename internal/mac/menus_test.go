package mac

import "testing"

func TestNewWindowRank(t *testing.T) {
	tests := []struct {
		title string
		want  int
	}{
		{"New Window", newWindowExact},
		{"New window", newWindowExact},
		{"  NEW WINDOW  ", newWindowExact},
		{"New Window…", newWindowExact},
		{"New Window...", newWindowExact},
		{"New Finder Window", newWindowOther},
		{"New Terminal Window", newWindowOther},
		{"New Empty Window", newWindowOther},
		{"New Private Window", 0},
		{"New Incognito Window", 0},
		{"New InPrivate Window", 0},
		{"New Tab", 0},
		{"New File", 0},
		{"New Message", 0},
		{"New Window with Profile", 0},
		{"Window", 0},
		{"Renew Window", 0},
		{"", 0},
	}
	for _, tt := range tests {
		if got := newWindowRank(tt.title); got != tt.want {
			t.Errorf("newWindowRank(%q) = %d, want %d", tt.title, got, tt.want)
		}
	}
}

func TestCloseWindowRank(t *testing.T) {
	tests := []struct {
		title string
		want  int
	}{
		{"Close Window", closeWindowExact},
		{"close window", closeWindowExact},
		{"  CLOSE WINDOW  ", closeWindowExact},
		{"Close Window…", closeWindowExact},
		{"Close", closeWindowPlain},
		{"Close...", closeWindowPlain},
		{"Close Tab", 0},
		{"Close All", 0},
		{"Close All Windows", 0},
		{"Close Other Tabs", 0},
		{"Close Others", 0},
		{"Closed", 0},
		{"Window", 0},
		{"", 0},
	}
	for _, tt := range tests {
		if got := closeWindowRank(tt.title); got != tt.want {
			t.Errorf("closeWindowRank(%q) = %d, want %d", tt.title, got, tt.want)
		}
	}
}
