package config

import (
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
)

// TestTabColoursMatchFrontend checks that the frontend's copies of the tab
// colours are TabColours, in order (decision 74): util.js's TAB_COLOURS,
// which Tab properties shows and colourAttr gives an element, the rules
// in styles.css that give each its --tab-colour, the tokens in styles.css
// and light.css, and appearance.test.js's list, which checks each
// token's contrast. A colour that Go knows and the page doesn't would
// show no colour, and a save from Tab properties could lose it.
func TestTabColoursMatchFrontend(t *testing.T) {
	read := func(parts ...string) string {
		b, err := os.ReadFile(filepath.Join(append([]string{"..", ".."}, parts...)...))
		if err != nil {
			t.Skipf("can't read %s: %v", filepath.Join(parts...), err)
		}
		return string(b)
	}
	all := func(re, text string) []string {
		var got []string
		for _, m := range regexp.MustCompile(re).FindAllStringSubmatch(text, -1) {
			got = append(got, m[1])
		}
		return got
	}
	want := TabColours()

	util := read("frontend", "js", "util.js")
	_, list, ok := strings.Cut(util, "export const TAB_COLOURS = [")
	if !ok {
		t.Fatal("util.js has no TAB_COLOURS")
	}
	list, _, _ = strings.Cut(list, "];")
	styles := read("frontend", "styles.css")
	rules := regexp.MustCompile(`\[data-colour="(\w+)"\] \{ --tab-colour: var\(--tab-(\w+)\); \}`).FindAllStringSubmatch(styles, -1)
	var ruled []string
	for _, r := range rules {
		if r[1] != r[2] {
			t.Errorf("styles.css gives data-colour=%q the token --tab-%s", r[1], r[2])
		}
		ruled = append(ruled, r[1])
	}
	test := read("e2e", "frontend", "appearance.test.js")
	_, checked, _ := strings.Cut(test, "const TAB_COLOURS = [")
	checked, _, _ = strings.Cut(checked, "]")

	for _, c := range []struct {
		what string
		got  []string
	}{
		{"util.js's TAB_COLOURS", all(`id: '(\w+)'`, list)},
		{"styles.css's [data-colour] rules", ruled},
		{"styles.css's dark tokens", all(`--tab-(\w+): #[0-9A-Fa-f]{6};`, styles)},
		{"light.css's tokens", all(`--tab-(\w+): #[0-9A-Fa-f]{6};`, read("frontend", "light.css"))},
		{"appearance.test.js's TAB_COLOURS", all(`'(\w+)'`, checked)},
	} {
		if !slices.Equal(c.got, want) {
			t.Errorf("%s are %q, want TabColours(), %q", c.what, c.got, want)
		}
	}
}
