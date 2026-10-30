package icons

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"
)

// This file finds a web page's icon: which of the icons its <link>
// elements name to fetch, or /favicon.ico, and fetching it.

// MaxBytes is the most an icon's file may hold. Icons are far smaller.
const MaxBytes = 256 << 10

// fetchTimeout is how long fetching an icon may take, redirects and all.
const fetchTimeout = 10 * time.Second

// maxCandidates is how many of a page's icons Bench tries, best first,
// before it gives up and shows the monogram.
const maxCandidates = 3

// Link is one of a page's <link> elements, as a script in the page reads
// it: its address, resolved against the page's, and its rel, sizes, and
// type attributes.
type Link struct {
	Href  string `json:"href"`
	Rel   string `json:"rel"`
	Sizes string `json:"sizes"`
	Type  string `json:"type"`
}

// touchSize is how big an apple-touch-icon is taken to be when its link
// doesn't say: the size iOS uses.
const touchSize = 180

// Candidates returns the addresses of a page's icon to try, best first, at
// most three: the page's own icons that Fit can read, then /favicon.ico at
// the page's origin. Of the page's icons, those at least Pixels wide come
// first, the smallest first, then its apple-touch-icons, then icons that
// don't say their size, then smaller ones, the biggest first. Only http and
// https addresses count, and data: addresses of images; a page that isn't
// http or https has only the icons it names.
func Candidates(pageURL string, links []Link) []string {
	type candidate struct {
		address string
		rank    int
	}
	var found []candidate
	for _, l := range links {
		rels := strings.Fields(strings.ToLower(l.Rel))
		icon := slices.Contains(rels, "icon")
		touch := slices.Contains(rels, "apple-touch-icon") || slices.Contains(rels, "apple-touch-icon-precomposed")
		if !icon && !touch || !readable(l) {
			continue
		}
		address, ok := iconAddress(l.Href)
		if !ok {
			continue
		}
		size := largestSize(l.Sizes)
		var rank int
		switch {
		case touch:
			if size == 0 {
				size = touchSize
			}
			rank = 2000 + distance(size)
		case size >= Pixels:
			rank = 1000 + distance(size)
		case size == 0:
			rank = 3000
		default:
			rank = 4000 + Pixels - size
		}
		found = append(found, candidate{address, rank})
	}
	slices.SortStableFunc(found, func(a, b candidate) int { return a.rank - b.rank })
	var out []string
	for _, c := range found {
		if !slices.Contains(out, c.address) {
			out = append(out, c.address)
		}
	}
	if u, err := url.Parse(pageURL); err == nil && (u.Scheme == "http" || u.Scheme == "https") && u.Host != "" {
		fallback := (&url.URL{Scheme: u.Scheme, Host: u.Host, Path: "/favicon.ico"}).String()
		if !slices.Contains(out, fallback) {
			if len(out) >= maxCandidates {
				out = out[:maxCandidates-1]
			}
			out = append(out, fallback)
		}
	}
	if len(out) > maxCandidates {
		out = out[:maxCandidates]
	}
	return out
}

// distance is how far a size at least Pixels is from Pixels.
func distance(size int) int {
	return min(size-Pixels, 999)
}

// readable reports whether Fit may read a link's icon: not an SVG, nor a
// Safari pinned tab's mask, which is one colour.
func readable(l Link) bool {
	t := strings.ToLower(strings.TrimSpace(l.Type))
	if t == "image/svg+xml" || strings.Contains(strings.ToLower(l.Rel), "mask-icon") {
		return false
	}
	u, err := url.Parse(l.Href)
	return err == nil && !strings.HasSuffix(strings.ToLower(u.Path), ".svg")
}

// largestSize returns the largest width in a sizes attribute, such as
// "16x16 32x32", or 0 if it names none, as for "any".
func largestSize(sizes string) int {
	largest := 0
	for _, s := range strings.Fields(strings.ToLower(sizes)) {
		w, _, ok := strings.Cut(s, "x")
		if n, err := strconv.Atoi(w); ok && err == nil && n > largest {
			largest = n
		}
	}
	return largest
}

// iconAddress returns an icon's address if Bench may fetch it: http or
// https, or a data: address of an image other than SVG.
func iconAddress(href string) (string, bool) {
	u, err := url.Parse(strings.TrimSpace(href))
	if err != nil {
		return "", false
	}
	switch u.Scheme {
	case "http", "https":
		if u.Host == "" {
			return "", false
		}
		u.Fragment = ""
		return u.String(), true
	case "data":
		media, _, _ := strings.Cut(u.Opaque, ",")
		media, _, _ = strings.Cut(media, ";")
		media = strings.ToLower(media)
		if strings.HasPrefix(media, "image/") && media != "image/svg+xml" {
			return href, true
		}
	}
	return "", false
}

// ErrTooBig: the icon's file holds more than MaxBytes.
var ErrTooBig = errors.New("the icon is too big")

// Fetch returns the bytes of the icon at address: decoded from a data:
// address, or fetched from an http or https one with client, which mustn't
// send cookies, without the page's session. It reads at most MaxBytes, and
// fails for more, or for a status other than 200.
func Fetch(ctx context.Context, client *http.Client, address string) ([]byte, error) {
	if strings.HasPrefix(strings.ToLower(address), "data:") {
		return dataBytes(address)
	}
	u, err := url.Parse(address)
	if err != nil || u.Scheme != "http" && u.Scheme != "https" {
		return nil, fmt.Errorf("can't fetch an icon from %q", address)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "image/png,image/x-icon,image/*;q=0.8")
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("fetching %s: %s", u.Redacted(), resp.Status)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, MaxBytes+1))
	if err != nil {
		return nil, err
	}
	if len(data) > MaxBytes {
		return nil, ErrTooBig
	}
	return data, nil
}

// dataBytes decodes a data: address, base64 or percent-encoded.
func dataBytes(address string) ([]byte, error) {
	header, payload, ok := strings.Cut(address[len("data:"):], ",")
	if !ok {
		return nil, ErrNotImage
	}
	params := strings.Split(header, ";")
	if media := strings.ToLower(params[0]); media != "" {
		if t, _, err := mime.ParseMediaType(media); err != nil || !strings.HasPrefix(t, "image/") {
			return nil, ErrNotImage
		}
	}
	var data []byte
	if slices.Contains(params[1:], "base64") {
		if base64.StdEncoding.DecodedLen(len(payload)) > MaxBytes+3 {
			return nil, ErrTooBig
		}
		d, err := base64.StdEncoding.DecodeString(payload)
		if err != nil {
			return nil, ErrNotImage
		}
		data = d
	} else {
		d, err := url.PathUnescape(payload)
		if err != nil {
			return nil, ErrNotImage
		}
		data = []byte(d)
	}
	if len(data) > MaxBytes {
		return nil, ErrTooBig
	}
	return data, nil
}

// NewClient returns a client for Fetch: no cookies, at most 5 redirects,
// each to http or https, and the whole fetch within ten seconds.
func NewClient() *http.Client {
	return &http.Client{
		Timeout: fetchTimeout,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 5 {
				return errors.New("too many redirects")
			}
			if req.URL.Scheme != "http" && req.URL.Scheme != "https" {
				return fmt.Errorf("can't follow a redirect to %s", req.URL.Scheme)
			}
			return nil
		},
	}
}
