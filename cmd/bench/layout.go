package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"os"
	"strconv"

	"github.com/itbm/bench/internal/config"
	"github.com/itbm/bench/internal/layout"
)

// defaultAreaY is the pane area's default top, as in the spec's monitor
// example: below a 25-point menu bar.
const defaultAreaY = 25

var layoutCommand = &command{
	name:     "layout",
	synopsis: "bench layout --width w --height h [flags]",
	summary:  "Print every tab's pane frames for a screen size, as JSON.",
	about: "Lays out every tab of config.toml in a pane area at x 0 and y --y, --width by --height points,\n" +
		"with the config's gap. The size class comes from the width and compact_below, unless --compact\n" +
		"forces the compact one. For each tab it prints the size class, the tree as displayed after the\n" +
		"tab's small-screen setting, and each pane's frame by pane ID, in reading order.",
	flags: func(fs *flag.FlagSet) func(*cli, []string) error {
		width := fs.Float64("width", 0, "the pane area's width, in points (required)")
		height := fs.Float64("height", 0, "the pane area's height, in points (required)")
		y := fs.Float64("y", defaultAreaY, "the pane area's top, in points")
		compact := fs.Bool("compact", false, "use the compact size class whatever the width")
		path := configFlag(fs)
		return func(c *cli, args []string) error {
			if len(args) > 0 {
				return unexpectedArgs(args)
			}
			if !(*width > 0) || !(*height > 0) {
				return usageErrorf("give a positive --width and --height in points, such as --width 2560 --height 1415")
			}
			area := layout.Rect{Y: *y, W: *width, H: *height}
			return printLayout(c, *path, area, *compact)
		}
	},
}

// layoutReport is what `bench layout` prints.
type layoutReport struct {
	Config    string           `json:"config"`
	Area      layout.Rect      `json:"area"`
	Gap       float64          `json:"gap"`
	SizeClass layout.SizeClass `json:"sizeClass"`
	Tabs      []tabLayout      `json:"tabs"`
}

// tabLayout is one tab's layout.
type tabLayout struct {
	Name         string           `json:"name"`
	SizeClass    layout.SizeClass `json:"sizeClass"`
	SmallScreens string           `json:"smallScreens"`
	// Root is the tree as displayed, after the small-screen setting.
	Root *layout.Node `json:"root"`
	// Frames are the displayed panes' frames, in reading order.
	Frames []layout.PaneRect `json:"frames"`
	// Items describes each item ID in the tree.
	Items map[string]string `json:"items"`
}

// printLayout lays out the config's tabs and prints them as JSON. If the
// support folder has no config.toml yet, it uses the default config, as
// Bench would; a config named with --config must exist.
func printLayout(c *cli, flagValue string, area layout.Rect, compact bool) error {
	path, err := configPath(flagValue)
	if err != nil {
		return err
	}
	cfg := config.Default()
	if _, err := os.Stat(path); errors.Is(err, fs.ErrNotExist) && flagValue == "" {
		fmt.Fprintf(c.stderr, "There's no config file at %s, so this is the default config.\n", path)
		path = ""
	} else {
		var problems []string
		if cfg, problems, err = readConfig(path); err != nil {
			return err
		}
		if len(problems) > 0 {
			for _, line := range problems {
				fmt.Fprintln(c.stderr, line)
			}
			return fmt.Errorf("%s has %s; run bench check-config", path, count(len(problems), "problem", "problems"))
		}
	}
	report, err := layoutFor(cfg, area, compact)
	if err != nil {
		return err
	}
	report.Config = path
	out, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return fmt.Errorf("writing the layout: %w", err)
	}
	fmt.Fprintf(c.stdout, "%s\n", out)
	return nil
}

// layoutFor lays out every tab of cfg in area.
func layoutFor(cfg *config.Config, area layout.Rect, compact bool) (layoutReport, error) {
	mode := layout.ModeAuto
	if compact {
		mode = layout.ModeCompact
	}
	class := layout.ChooseSizeClass(mode, area.W, cfg.Settings.CompactBelow)
	report := layoutReport{Area: area, Gap: cfg.Settings.Gap, SizeClass: class}
	ids := &idMaker{}
	// As in the app, every item gets its ID before any pane or split.
	paneItems := make([][][]string, len(cfg.Tabs))
	items := make([]map[string]string, len(cfg.Tabs))
	for i, tab := range cfg.Tabs {
		paneItems[i], items[i] = numberItems(tab, ids)
	}
	for i, tab := range cfg.Tabs {
		root, err := tabTree(tab, paneItems[i], ids)
		if err != nil {
			return layoutReport{}, fmt.Errorf("laying out tab %d (%q): %w", i+1, tab.Name, err)
		}
		tl := layoutTab(tab, root, class, area, cfg.Settings.Gap)
		tl.Items = items[i]
		report.Tabs = append(report.Tabs, tl)
	}
	return report, nil
}

// layoutTab lays out one tab's tree for a size class: it applies the tab's
// small-screen setting, then works out the displayed panes' frames.
func layoutTab(tab config.Tab, root *layout.Node, class layout.SizeClass, area layout.Rect, gap float64) tabLayout {
	focus := ""
	if leaves := layout.Leaves(root); len(leaves) > 0 {
		focus = leaves[0].ID
	}
	small := tab.SmallScreens
	if small == "" {
		small = string(layout.Stack)
	}
	shown := layout.Adapt(root, layout.SmallScreens(small), class, focus, "")
	return tabLayout{
		Name:         tab.Name,
		SizeClass:    class,
		SmallScreens: small,
		Root:         shown,
		Frames:       layout.Frames(shown, area, gap),
	}
}

// idMaker numbers items ("w1"), panes ("p2"), and splits ("s3") from one
// sequence, as the app does when it builds its model from a config, so that
// the IDs here are the ones the app starts with.
type idMaker struct {
	seq int
}

func (m *idMaker) next(prefix string) string {
	m.seq++
	return prefix + strconv.Itoa(m.seq)
}

// newID is a layout.NewID: "s" for a split, "p" for a pane.
func (m *idMaker) newID(nodeType string) string {
	if nodeType == layout.TypeSplit {
		return m.next("s")
	}
	return m.next("p")
}

// numberItems gives each of a tab's items an ID, and returns the IDs pane
// by pane, with a description of each item by ID.
func numberItems(tab config.Tab, ids *idMaker) ([][]string, map[string]string) {
	paneItems := make([][]string, len(tab.Panes))
	described := map[string]string{}
	for i, p := range tab.Panes {
		for _, it := range p.Items {
			id := ids.next("w")
			described[id] = describeItem(it)
			paneItems[i] = append(paneItems[i], id)
		}
	}
	return paneItems, described
}

// tabTree builds a tab's tree from its preset and sizes, with a pane for
// each [[tab.pane]] holding its items (the preset adds any missing panes,
// empty).
func tabTree(tab config.Tab, paneItems [][]string, ids *idMaker) (*layout.Node, error) {
	panes := make([]*layout.Node, len(paneItems))
	for i, itemIDs := range paneItems {
		panes[i] = layout.NewPane(ids.newID(layout.TypePane), itemIDs...)
	}
	return layout.Build(tab.Layout, panes, tab.Sizes, ids.newID)
}

// describeItem names an item for the layout's items list.
func describeItem(it config.Item) string {
	var what string
	switch {
	case it.App != "":
		what = it.App
		if it.Title != "" {
			what += " (" + it.Title + ")"
		}
	case it.Web != "":
		what = it.Web
	case it.VSCode != "":
		what = "VS Code for the Web: " + it.VSCode
	case it.Terminal != "":
		what = "Terminal: " + it.Terminal
	}
	if it.Name != "" {
		return it.Name + " — " + what
	}
	return what
}
