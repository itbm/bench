// Package update looks for a newer release of Bench on GitHub, for the
// check_for_updates setting, and compares versions. It only looks: Bench
// doesn't download or install anything, and the user gets a new version
// from the release's page, as they got the first.
package update

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"slices"
	"strconv"
	"strings"
)

// ReleasesURL is the page of Bench's latest release on GitHub, with its
// downloads and release notes, which Bench ▸ Check for Updates… opens.
const ReleasesURL = "https://github.com/itbm/bench/releases/latest"

// releasePages is the start of every release's page, which a release's
// address must have: Bench opens it in the browser.
const releasePages = "https://github.com/itbm/bench/releases/"

// LatestAPI is GitHub's address for Bench's latest release: the newest
// that isn't a draft or a pre-release.
const LatestAPI = "https://api.github.com/repos/itbm/bench/releases/latest"

// maxBody is the most of GitHub's answer that Latest reads.
const maxBody = 1 << 20

// Release is a release of Bench.
type Release struct {
	// Version is the release's version, from its tag without the v, such
	// as "0.3.0".
	Version string `json:"version"`
	// URL is the release's page on GitHub.
	URL string `json:"url"`
}

// Latest asks GitHub for Bench's latest release, with a client that the
// caller gives a timeout, or ctx one. It sends no cookies and nothing about
// the user: only the request, which says it's from Bench and which
// version (version, as `bench version` prints it).
func Latest(ctx context.Context, client *http.Client, address, version string) (Release, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, address, nil)
	if err != nil {
		return Release{}, fmt.Errorf("looking for a new version: %w", err)
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "Bench/"+releaseOf(version))
	resp, err := client.Do(req)
	if err != nil {
		return Release{}, fmt.Errorf("looking for a new version: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return Release{}, fmt.Errorf("looking for a new version: GitHub answered %s", resp.Status)
	}
	var body struct {
		Tag        string `json:"tag_name"`
		URL        string `json:"html_url"`
		Draft      bool   `json:"draft"`
		Prerelease bool   `json:"prerelease"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxBody)).Decode(&body); err != nil {
		return Release{}, fmt.Errorf("looking for a new version: GitHub's answer isn't a release: %w", err)
	}
	v := strings.TrimPrefix(body.Tag, "v")
	if _, ok := parse(v); !ok || body.Draft || body.Prerelease {
		return Release{}, fmt.Errorf("looking for a new version: GitHub's latest release, %q, isn't a version", body.Tag)
	}
	r := Release{Version: v, URL: body.URL}
	// Bench opens the page in the browser, so it must be one of Bench's.
	if !strings.HasPrefix(r.URL, releasePages) {
		r.URL = ReleasesURL
	}
	return r, nil
}

// releaseOf returns the version at the start of a version's text, without
// what follows it, such as the commit in "0.2.0 (4f2c1a9e0b7d)".
func releaseOf(text string) string {
	fields := strings.Fields(text)
	if len(fields) == 0 {
		return ""
	}
	return strings.TrimPrefix(fields[0], "v")
}

// version is a version parsed as semantic versioning has it.
type version struct {
	nums [3]int
	pre  []string
}

// parse parses a version such as "0.2.0" or "0.2.0-beta.1", leaving out
// build metadata after a +.
func parse(s string) (version, bool) {
	s, _, _ = strings.Cut(s, "+")
	core, pre, hasPre := strings.Cut(s, "-")
	parts := strings.Split(core, ".")
	if len(parts) != 3 || (hasPre && pre == "") {
		return version{}, false
	}
	var v version
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil || n < 0 || strings.Trim(p, "0123456789") != "" || (len(p) > 1 && p[0] == '0') {
			return version{}, false
		}
		v.nums[i] = n
	}
	if hasPre {
		v.pre = strings.Split(pre, ".")
		if slices.Contains(v.pre, "") {
			return version{}, false
		}
	}
	return v, true
}

// ErrNotVersion is Compare's error for text that isn't a version, such as
// "dev", which a build that isn't a release has.
var ErrNotVersion = errors.New("not a version")

// Compare compares two versions, as semantic versioning orders them: -1 if
// a is older than b, 0 if they're the same version, or 1 if a is newer. A v
// before a version, and the text after it, such as the commit in "0.2.0
// (4f2c1a9e0b7d)", don't count. It returns ErrNotVersion if either isn't a
// version.
func Compare(a, b string) (int, error) {
	va, okA := parse(releaseOf(a))
	vb, okB := parse(releaseOf(b))
	if !okA || !okB {
		return 0, ErrNotVersion
	}
	for i := range va.nums {
		if c := cmpInt(va.nums[i], vb.nums[i]); c != 0 {
			return c, nil
		}
	}
	return comparePre(va.pre, vb.pre), nil
}

// comparePre compares two versions' pre-release parts: a version without
// one is newer than the same version with one, and otherwise their parts
// compare in turn, numbers as numbers and below words, which compare as
// text.
func comparePre(a, b []string) int {
	switch {
	case len(a) == 0 && len(b) == 0:
		return 0
	case len(a) == 0:
		return 1
	case len(b) == 0:
		return -1
	}
	for i := 0; i < len(a) && i < len(b); i++ {
		na, errA := strconv.Atoi(a[i])
		nb, errB := strconv.Atoi(b[i])
		var c int
		switch {
		case errA == nil && errB == nil:
			c = cmpInt(na, nb)
		case errA == nil:
			c = -1
		case errB == nil:
			c = 1
		default:
			c = strings.Compare(a[i], b[i])
		}
		if c != 0 {
			return c
		}
	}
	return cmpInt(len(a), len(b))
}

// cmpInt compares two numbers.
func cmpInt(a, b int) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	}
	return 0
}
