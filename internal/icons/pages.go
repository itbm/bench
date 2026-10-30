package icons

import (
	"context"
	"net/http"
	"slices"
	"sync"
)

// Pages keeps each Web item's page icon, as a PNG Pixels square. Found
// fetches the best of the icons a page names, in the background, when they
// differ from the ones the item's last page named, and keeps the icon it
// had until the new one comes, so that moving about a site doesn't fetch
// its icon again. Its methods may be called from any goroutine.
type Pages struct {
	client *http.Client
	// fetch is Fetch; tests replace it.
	fetch func(ctx context.Context, client *http.Client, address string) ([]byte, error)
	// changed is called, from a goroutine of its own, without the lock,
	// when an item's icon changes: has says whether it has one now.
	changed func(itemID string, has bool)

	mu     sync.Mutex
	items  map[string]*pageIcon
	closed bool
}

// pageIcon is an item's icon, and the fetch in progress, if any.
type pageIcon struct {
	// candidates are the addresses the item's last page offered.
	candidates []string
	// fetching counts Found's fetches, so that one that's been overtaken
	// doesn't overwrite the newer one's icon.
	fetching int
	cancel   context.CancelFunc
	png      []byte
}

// NewPages returns a Pages that tells changed when an item's icon changes.
func NewPages(changed func(itemID string, has bool)) *Pages {
	return &Pages{client: NewClient(), fetch: Fetch, changed: changed, items: map[string]*pageIcon{}}
}

// Found takes the <link> elements of the page that a Web item's view has
// just loaded, at pageURL, and fetches the page's icon if its candidates
// (Candidates) differ from the last page's. Icons that couldn't be fetched
// aren't tried again until the item's page offers others.
func (p *Pages) Found(itemID, pageURL string, links []Link) {
	candidates := Candidates(pageURL, links)
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed || itemID == "" {
		return
	}
	st := p.items[itemID]
	if st == nil {
		st = &pageIcon{}
		p.items[itemID] = st
	} else if slices.Equal(st.candidates, candidates) {
		// Fetched, being fetched, or tried in vain.
		return
	}
	if st.cancel != nil {
		st.cancel()
	}
	st.candidates = candidates
	st.fetching++
	ctx, cancel := context.WithCancel(context.Background())
	st.cancel = cancel
	go p.fetchIcon(ctx, cancel, itemID, st, st.fetching, candidates)
}

// fetchIcon fetches the first of the candidates that Fit can read, and
// keeps it as the item's icon, unless Found has started another fetch for
// it since, or it has been forgotten. With none, the item has no icon.
func (p *Pages) fetchIcon(ctx context.Context, cancel context.CancelFunc, itemID string, st *pageIcon, fetching int, candidates []string) {
	defer cancel()
	var icon []byte
	for _, address := range candidates {
		data, err := p.fetch(ctx, p.client, address)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			continue
		}
		if fitted, err := Fit(data, Pixels); err == nil {
			icon = fitted
			break
		}
	}
	p.mu.Lock()
	if p.closed || p.items[itemID] != st || st.fetching != fetching {
		p.mu.Unlock()
		return
	}
	st.cancel = nil
	had := st.png != nil
	st.png = icon
	p.mu.Unlock()
	if icon != nil || had {
		p.changed(itemID, icon != nil)
	}
}

// Icon returns an item's icon, and whether it has one.
func (p *Pages) Icon(itemID string) ([]byte, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if st := p.items[itemID]; st != nil && st.png != nil {
		return st.png, true
	}
	return nil, false
}

// Forget drops an item's icon, as its web view closes, and stops its
// fetch.
func (p *Pages) Forget(itemID string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if st := p.items[itemID]; st != nil && st.cancel != nil {
		st.cancel()
	}
	delete(p.items, itemID)
}

// Close forgets every icon and stops every fetch. Found does nothing after.
func (p *Pages) Close() {
	p.mu.Lock()
	defer p.mu.Unlock()
	for _, st := range p.items {
		if st.cancel != nil {
			st.cancel()
		}
	}
	p.items = map[string]*pageIcon{}
	p.closed = true
}
