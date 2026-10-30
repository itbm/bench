package mac

import (
	"encoding/json"

	"github.com/itbm/bench/internal/icons"
)

// This file has the pure-Go side of icons: which bundle IDs Bench asks
// macOS to draw the icon of, and the <link> elements that a script reads
// from a Web item's page, for its icon (icons.Pages).

// maxIconLinks is the most of a page's <link> elements that Bench reads.
const maxIconLinks = 64

// validBundleID reports whether a bundle ID is one that Bench asks macOS
// for an icon of: 1 to 255 letters, digits, dots, hyphens, and
// underscores, as bundle IDs are, and as an address can carry unchanged.
func validBundleID(id string) bool {
	if id == "" || len(id) > 255 {
		return false
	}
	for _, r := range id {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '.', r == '-', r == '_':
		default:
			return false
		}
	}
	return true
}

// iconLinks decodes the JSON that the page's script returns: its <link>
// elements, at most maxIconLinks. Anything else is none.
func iconLinks(text string) []icons.Link {
	var links []icons.Link
	if err := json.Unmarshal([]byte(text), &links); err != nil {
		return nil
	}
	if len(links) > maxIconLinks {
		links = links[:maxIconLinks]
	}
	return links
}
