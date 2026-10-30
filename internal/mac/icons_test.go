package mac

import (
	"reflect"
	"strings"
	"testing"

	"github.com/itbm/bench/internal/icons"
)

func TestValidBundleID(t *testing.T) {
	for id, want := range map[string]bool{
		"com.tinyspeck.slackmacgap":  true,
		"dev.zed.Zed":                true,
		"com.google.Chrome.app.abc_": true,
		"":                           false,
		"../../etc":                  false,
		"com.example/other":          false,
		"com.example app":            false,
		"com.example%2F":             false,
		strings.Repeat("a", 256):     false,
	} {
		if got := validBundleID(id); got != want {
			t.Errorf("validBundleID(%q) = %v, want %v", id, got, want)
		}
	}
}

func TestIconLinks(t *testing.T) {
	got := iconLinks(`[{"href":"https://a.example/i.png","rel":"icon","sizes":"32x32","type":"image/png"},{"href":"https://a.example/s.css","rel":"stylesheet"}]`)
	want := []icons.Link{
		{Href: "https://a.example/i.png", Rel: "icon", Sizes: "32x32", Type: "image/png"},
		{Href: "https://a.example/s.css", Rel: "stylesheet"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("iconLinks = %+v, want %+v", got, want)
	}
	for _, bad := range []string{"", "null", "{}", `"x"`, "[1, 2]"} {
		if got := iconLinks(bad); len(got) != 0 {
			t.Errorf("iconLinks(%q) = %+v, want none", bad, got)
		}
	}
	many := "[" + strings.TrimSuffix(strings.Repeat(`{"href":"https://a.example/","rel":"icon"},`, 100), ",") + "]"
	if got := iconLinks(many); len(got) != maxIconLinks {
		t.Errorf("iconLinks of 100 links gave %d, want %d", len(got), maxIconLinks)
	}
}
