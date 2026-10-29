package update

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCompare(t *testing.T) {
	for _, tc := range []struct {
		a, b string
		want int
	}{
		{"0.2.0", "0.2.0", 0},
		{"0.3.0", "0.2.0", 1},
		{"0.2.0", "0.10.0", -1},
		{"1.0.0", "0.99.99", 1},
		{"0.2.1", "0.2.0", 1},
		{"v0.3.0", "0.3.0", 0},
		// The commit and whether the build was modified don't count.
		{"0.2.0 (4f2c1a9e0b7d)", "0.2.0 (a1b2c3d4e5f6, modified)", 0},
		{"0.3.0", "0.2.0 (4f2c1a9e0b7d)", 1},
		// A pre-release comes before its release, and its parts in turn.
		{"0.2.0-beta.1", "0.2.0", -1},
		{"0.2.0", "0.2.0-beta.1", 1},
		{"0.2.0-beta.2", "0.2.0-beta.1", 1},
		{"0.2.0-beta.10", "0.2.0-beta.2", 1},
		{"0.2.0-beta", "0.2.0-alpha", 1},
		{"0.2.0-1", "0.2.0-alpha", -1},
		{"0.2.0-beta", "0.2.0-beta.1", -1},
		{"0.2.0+build.5", "0.2.0", 0},
	} {
		got, err := Compare(tc.a, tc.b)
		if err != nil || got != tc.want {
			t.Errorf("Compare(%q, %q) = %d, %v; want %d", tc.a, tc.b, got, err, tc.want)
		}
	}
	for _, bad := range []string{"dev", "", "1.2", "1.2.3.4", "01.2.3", "1.x.3", "1.2.3-", "1.2.3-beta..1", "-1.2.3"} {
		if _, err := Compare(bad, "0.2.0"); !errors.Is(err, ErrNotVersion) {
			t.Errorf("Compare(%q, 0.2.0) = %v, want ErrNotVersion", bad, err)
		}
		if _, err := Compare("0.2.0", bad); !errors.Is(err, ErrNotVersion) {
			t.Errorf("Compare(0.2.0, %q) = %v, want ErrNotVersion", bad, err)
		}
	}
}

// github answers as GitHub's API does for the latest release, with body,
// and records the request.
func github(t *testing.T, status int, body string) (*httptest.Server, *http.Request) {
	t.Helper()
	got := &http.Request{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		*got = *r.Clone(context.Background())
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return srv, got
}

func TestLatest(t *testing.T) {
	srv, req := github(t, http.StatusOK, `{"tag_name": "v0.3.0", "html_url": "https://github.com/itbm/bench/releases/tag/v0.3.0", "draft": false, "prerelease": false, "body": "Notes"}`)
	r, err := Latest(context.Background(), srv.Client(), srv.URL, "0.2.0 (4f2c1a9e0b7d)")
	if err != nil {
		t.Fatal(err)
	}
	if r.Version != "0.3.0" || r.URL != "https://github.com/itbm/bench/releases/tag/v0.3.0" {
		t.Errorf("Latest = %+v, want 0.3.0 and its page", r)
	}
	if got := req.Header.Get("User-Agent"); got != "Bench/0.2.0" {
		t.Errorf("User-Agent = %q, want Bench/0.2.0, without the commit", got)
	}
	if got := req.Header.Get("Accept"); got != "application/vnd.github+json" {
		t.Errorf("Accept = %q", got)
	}
	if len(req.Cookies()) != 0 || req.Header.Get("Authorization") != "" {
		t.Errorf("the request sends more than it needs: %v", req.Header)
	}
}

func TestLatestOpensOnlyBenchsPages(t *testing.T) {
	srv, _ := github(t, http.StatusOK, `{"tag_name": "v0.3.0", "html_url": "https://example.com/bench-0.3.0.dmg"}`)
	r, err := Latest(context.Background(), srv.Client(), srv.URL, "0.2.0")
	if err != nil {
		t.Fatal(err)
	}
	if r.URL != ReleasesURL {
		t.Errorf("URL = %q, want %q for a page that isn't one of Bench's", r.URL, ReleasesURL)
	}
}

func TestLatestRefuses(t *testing.T) {
	for name, tc := range map[string]struct {
		status int
		body   string
		want   string
	}{
		"not found":   {http.StatusNotFound, `{"message": "Not Found"}`, "404"},
		"rate limit":  {http.StatusForbidden, `{"message": "API rate limit exceeded"}`, "403"},
		"not JSON":    {http.StatusOK, `<html>`, "isn't a release"},
		"no tag":      {http.StatusOK, `{}`, "isn't a version"},
		"odd tag":     {http.StatusOK, `{"tag_name": "nightly"}`, "isn't a version"},
		"pre-release": {http.StatusOK, `{"tag_name": "v0.3.0-beta.1", "prerelease": true}`, "isn't a version"},
	} {
		t.Run(name, func(t *testing.T) {
			srv, _ := github(t, tc.status, tc.body)
			_, err := Latest(context.Background(), srv.Client(), srv.URL, "0.2.0")
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Errorf("Latest = %v, want an error that says %q", err, tc.want)
			}
		})
	}
}

func TestLatestStopsWithItsContext(t *testing.T) {
	srv, _ := github(t, http.StatusOK, `{"tag_name": "v0.3.0"}`)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := Latest(ctx, srv.Client(), srv.URL, "0.2.0"); !errors.Is(err, context.Canceled) {
		t.Errorf("Latest = %v, want context.Canceled", err)
	}
}

func TestReleasesURL(t *testing.T) {
	if !strings.HasPrefix(ReleasesURL, releasePages) || !strings.HasPrefix(LatestAPI, "https://api.github.com/repos/itbm/bench/") {
		t.Errorf("ReleasesURL %q and LatestAPI %q should name Bench's repository", ReleasesURL, LatestAPI)
	}
}
