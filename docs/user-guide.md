# Bench user guide

Bench gathers any set of apps into one window with tabs. Each tab holds a
layout of panes, and each pane holds windows as sub-tabs. This guide explains
how to use Bench, how to set it up with its config file, and what to do when
something doesn't work.

## Contents

- [Get started](#get-started)
- [Tabs](#tabs)
- [Panes](#panes)
- [Sub-tabs](#sub-tabs)
- [Move windows](#move-windows)
- [Show a window in more than one tab](#show-a-window-in-more-than-one-tab)
- [Add to a pane](#add-to-a-pane)
- [The jump palette](#the-jump-palette)
- [The layout editor](#the-layout-editor)
- [Small screens: Auto, Wide, and Compact](#small-screens-auto-wide-and-compact)
- [Live previews](#live-previews)
- [New windows and links](#new-windows-and-links)
- [Keyboard shortcuts](#keyboard-shortcuts)
- [When you hide Bench](#when-you-hide-bench)
- [When Bench quits](#when-bench-quits)
- [Bench's files](#benchs-files)
- [Command line](#command-line)
- [Config reference](#config-reference)
- [Troubleshooting](#troubleshooting)

## Get started

When you first open Bench, a guide asks for one permission:

1. Click **Turn on Accessibility**. macOS asks whether Bench can control
   your computer: click **Open System Settings**.
2. In **Privacy & Security ▸ Accessibility**, turn on Bench.
3. Back in Bench, wait for the guide to say that Bench has Accessibility
   permission, then click **Continue**.

macOS usually asks only once. If you closed its message, open **System
Settings ▸ Privacy & Security ▸ Accessibility** yourself and turn on Bench.

Bench needs Accessibility permission to arrange other apps' windows: to move
and size them into its panes, and to hide the ones that belong to other
tabs. Its keyboard shortcuts work without it, but Bench can't arrange
windows until you allow it. Until then, a banner at the top of the window
reminds you.

Bench starts with three tabs:

| Tab | What's in it |
| --- | --- |
| Comms | Slack and Outlook on the web, side by side. |
| Code | Zed and VS Code as sub-tabs of the main pane, and a terminal beside them. |
| Research | Brave and Obsidian, side by side. |

Change them in the [layout editor](#the-layout-editor) or in the
[config file](#config-reference).

Bench has a menu bar item that shows the current tab. Click it to switch
tabs, show Bench, or quit.

## Tabs

A tab is a named workspace with its own layout. The tabs are in Bench's title
bar, and the current tab has a raised background and an orange underline.

To switch tabs:

- Click a tab.
- Press Control+Option+1 to 9 for the first nine tabs, or Control+Option+]
  and Control+Option+[ for the next and previous tab.
- Choose a tab in the [jump palette](#the-jump-palette), or from Bench's menu
  bar item.

There's no limit on the number of tabs. When they don't fit, the tab strip
scrolls sideways. Only the first nine tabs get a shortcut.

To add a tab, click **+** after the tabs. The layout editor opens for the new
tab.

## Panes

A tab's layout divides the window into panes. Each pane shows one window at a
time, with a sub-tab in its header for each window it holds.

- **Focus a pane** by clicking anywhere in it, or with Control+Option+arrow.
  The focused pane has an orange border, and what it shows gets the
  keyboard: an App window, a web page, or a terminal.
- **Resize panes** by dragging the gap between them. To resize with the
  keyboard, press Tab until the gap is selected, then press the arrow keys:
  each press moves it by 2%, or 10% with Shift.
- **Arrange the tab again** with Control+Option+0, **Rearrange this tab** in
  the jump palette, or **View ▸ Rearrange**, if windows have drifted out of
  place.

A pane's header holds:

| Control | What it does |
| --- | --- |
| Sub-tabs | Show a window. See [Sub-tabs](#sub-tabs). |
| **+** | [Add a window or app](#add-to-a-pane) to the pane. |
| Badge | The shown item's kind: **Web**, **App**, or **Built-in**. |
| **Show side by side** | Split the pane to the right, with the shown window in the new half. |
| **⋯** | More ways to move the shown window. See [Move windows](#move-windows). |

When a pane loses its last window, Bench removes it and the panes beside it
close up. The last pane in a tab stays, empty, with an **Add a window or
app** button.

## Sub-tabs

Each window in a pane has a sub-tab, showing the app's monogram and the
window's title. Click a sub-tab to show its window, or press
Control+Option+Tab (and Control+Option+Shift+Tab) to move between the
focused pane's sub-tabs.

A pane can hold three kinds of item, from any mix of apps:

| Kind | Badge | What it is | Examples |
| --- | --- | --- | --- |
| Web | **Web** | A web page that Bench shows inside its own window, with its own logins. | Slack, Outlook on the web, VS Code for the Web |
| App | **App** | Another app's real window, which Bench moves and sizes to fit the pane. It works as normal: typing, menus, and drag and drop. | Zed, VS Code, Brave, Obsidian |
| Built-in | **Built-in** | A terminal session that Bench draws itself. Its sub-tab shows the running command and the folder. | `zsh`, `go test` |

macOS doesn't let one app put another app's window inside its own, so an App
item is still a separate window. Bench keeps it exactly over its pane, keeps
a pane's other windows directly behind the shown one, and hides windows that
belong to other tabs.

When an item can't be shown, its pane says why:

| Message | What it means |
| --- | --- |
| Opening *app*… | Bench is opening the app and waiting for its window. |
| *App* isn't running | The app isn't open, and `open_missing_apps` is off. |
| *App* has no window open for this | The app is open, but none of its windows belongs in this sub-tab. Open one in the app, or add one with the pane's **+**. |
| Bench couldn't open *app* | macOS couldn't open the app. Check that it's still installed. |
| Moved out of its pane | You dragged the window away with the mouse. Click **Dock again** to put it back. |
| *App*'s window can't shrink to fit this pane | The window's minimum size is bigger than the pane. Bench places it at the pane's top-left. Try the compact layout. |
| In full screen | The window is in native full screen. Bench leaves it alone. |
| On another desktop | The window is on another desktop (Space). |
| The terminal couldn't start: … | Bench couldn't start the terminal's shell or command. For example, a terminal with a `command` doesn't run it when its folder doesn't exist. |

The sub-tab of an item that can't be shown has a dot on its monogram, and
holding the pointer over it shows why. When the window a pane shows needs
more room than the pane has, the pane's header also says **Needs more
room**.

When an app moves or resizes its own docked window, as some apps do just
after Bench sizes them, Bench puts the window back. If the app keeps doing
it, Bench stops after three tries in ten seconds and leaves the window where
the app put it, with no message, until the tab is next arranged. See
[A window isn't in its pane](#a-window-isnt-in-its-pane).

## Move windows

There are three ways to move a window. They all do the same thing.

**Drag its sub-tab:**

| Drop it on | What happens |
| --- | --- |
| The middle of a pane | The window joins that pane, and shows. |
| The outer quarter of a pane, on any side | The pane splits in that direction, and the window goes into the new half. |
| A tab in the title bar | The window moves to that tab's focused pane. With Option held, it stays here too: see [Show a window in more than one tab](#show-a-window-in-more-than-one-tab). |

While you drag, Bench highlights where the window lands: half a pane for
an edge, the whole pane for the middle, or a tinted tab. To cancel a drag,
press Escape.

**Use the pane's buttons.** **Show side by side** splits the pane to the
right with the shown window. The **⋯** menu has:

- **Show side by side (split right)** and **Split down**.
- **Move to pane**, listing each other pane by its shown window.
- **Move to tab**, listing each other tab.
- **Also show in** *tab*, for each tab that the window isn't in yet.
- **Remove from this tab**, for a window that's in other tabs too.
- **Dock again**, for a window you dragged out of its pane.
- **Pop out of Bench**.

**Use the keyboard.** Control+Option+Shift+arrow moves the focused pane's
shown window into the neighbouring pane in that direction, or splits the pane
at that edge if there's no neighbour.

Things to know:

- A pane can't be split by its only window. Bench explains why instead of
  doing nothing: add another window first, or drag one in.
- Moving a window makes the tab's layout custom. The layout editor says so,
  and choosing a layout there starts again from it, keeping the windows in
  order.
- When a tab shows one pane with sub-tabs on a small screen, there are no
  splits, so the split actions are turned off. Moving to another tab still
  works.
- **Pop out of Bench** returns an App window to a normal, free-floating
  window, and gives a Web or Built-in item a window of its own. The item then
  appears under **Not in a tab** in the jump palette, and under **Windows
  not in a tab** in the add dialog. Closing a popped-out web page's or
  terminal's window closes the item, as closing an App window does: a
  terminal's session ends. A web page or terminal that still has its own
  window when Bench quits gets it again when Bench next opens.

## Show a window in more than one tab

One window can be in several tabs. For example, your email can sit beside a
browser in one tab and beside Obsidian in another. A window can only be in
one place at a time, so Bench docks it in its pane in whichever of those
tabs you switch to.

To show a window in another tab as well, do one of these:

- In the window's **⋯** menu, choose **Also show in** followed by the tab's
  name, such as **Also show in Research**. The window joins that tab's
  focused pane.
- Hold Option while you drag the window's sub-tab onto a tab. A **+**
  appears on the tab and on the sub-tab you're dragging.
- Switch to the other tab, click a pane's **+**, and choose the window under
  **Windows in other tabs**. The window joins that pane.

A window that's in more than one tab has a mark after the title on its
sub-tab. Hold the pointer over the mark to see which other tabs it's in.

To take the window out of one tab, switch to that tab, and choose **Remove
from this tab** in the window's **⋯** menu. The window stays in its other
tabs.

Things to know:

- A window can be in each tab once.
- Dragging the window to another pane, or splitting a pane with it, moves it
  only in the tab you're in. **Move to tab** takes it out of the tab you're
  in. If it's in that tab already, it stays where it is there.
- Closing the window, or choosing **Pop out of Bench**, takes it out of
  every tab.
- Deleting a tab doesn't change a window that's also in another tab.
- When you click a window that's in the current tab and in other tabs, for
  example in the Dock, Bench shows it in the current tab rather than
  switching tabs.
- Web pages and terminals work the same way.
- Bench remembers which tabs a window is in, in `state.json`. The config
  file can't list one window in several tabs.

## Add to a pane

Click a pane's **+** to open the add dialog. Type to search, then choose a
row:

| Group | What it adds |
| --- | --- |
| **Windows not in a tab** | An open window that Bench isn't managing. |
| **Windows in other tabs** | A window, web page, or terminal from another tab, to show in this pane as well. It stays in its other tabs. |
| **New window** | A new window of an app that's already in this tab. |
| **Built in** | **New terminal**, or **Web page…**, which asks for an `https` address. |
| **Apps** | Any installed app, from `/Applications`, `~/Applications`, and `/System/Applications`, including their `Utilities` folders. Bench opens the app if it isn't running, and adds its windows that aren't in a tab, or opens a new window if it has none. An app that Bench opens can take a while to open its windows, and some of them can belong to other sub-tabs: Bench opens a new window only if none of the app's windows comes to the pane, or if it opens none within `launch_timeout`. |

## The jump palette

Press Control+Option+Space, or click **Jump to…** in the title bar. Type to
filter by name, then use the Up and Down arrows and press Return. Press
Escape, or click outside the palette, to close it.

| Group | What's in it |
| --- | --- |
| **Tabs** | Every tab, with its apps and shortcut. |
| **Windows** | Every window in every tab, by title, with its tab and app. A window in several tabs is listed once for each. Choosing one switches to its tab, focuses its pane, and shows it. |
| **Not in a tab** | Windows that Bench isn't managing. Choosing one adds it to the current tab's focused pane. |
| **Actions** | **Rearrange this tab**, **Edit the layout of** the current tab, **New tab…**, and switching between Auto, Wide, and Compact. |

## The layout editor

To open the layout editor, click the layout button at the right of the title
bar, choose **Edit the layout of** the current tab in the jump palette, or
choose **Bench ▸ Settings…**. Click **+** after the tabs to open it for a new
tab.

| Field | What it does |
| --- | --- |
| **Name** | The tab's name. Names must be unique. |
| **Shortcut** | The tab's shortcut. Shortcuts follow tab order: the first nine tabs get Control+Option+1 to 9, and later tabs have none. |
| **Layout** | One of six layouts, with a preview of which windows land in which pane. |
| **On small screens** | **Stack panes**, **One pane with sub-tabs**, or **Keep the layout**. See [Small screens](#small-screens-auto-wide-and-compact). |

The layouts:

| Layout | Panes | Starting sizes |
| --- | --- | --- |
| **Single** | One pane. | — |
| **Side by side** | Two, in a row. | 50%, 50% |
| **Main and side** | Two, in a row. | 65%, 35% |
| **Three columns** | Three, in a row. | 40%, 30%, 30% |
| **Top and bottom** | Two, in a column. | 65%, 35% |
| **Grid of four** | Two rows of two. | 50%, 50% each way |

Choosing a layout keeps the windows in order: the first pane's windows go to
the new first pane, and so on. If the new layout has fewer panes, the extra
panes' windows join the last one. No window is lost. If you've moved windows,
the layout is custom, no layout is selected, and choosing one starts again
from it.

The editor also holds these settings, which apply to every tab:

| Setting | Config setting |
| --- | --- |
| **Small screens are narrower than** *number* **points** | `compact_below` |
| **Open apps that aren't running** | `open_missing_apps` |
| **Hide windows that belong to other tabs** | `hide_other_tabs` |
| **Switch tabs when another tab's window comes forward** | `follow_focus` |
| **Show live previews of windows (needs Screen Recording)** | `live_previews` |
| **Open Bench at login** | — |

At the bottom of the editor:

- **Delete tab** removes the tab, and any [rules](#rules) that send windows
  to it. Its windows aren't closed: they move to **Not in a tab**. You
  can't delete the last tab.
- **Reset layout** returns the tab to the layout in its config, undoing your
  dragging and splitting.
- **Cancel** closes the editor without changes, and **Save** applies them and
  saves `config.toml`, keeping the previous version as `config.toml.bak`.
  Renaming a tab also renames it in the rules that name it.

While `config.toml` has problems, Bench runs with the default config, and
the editor's changes apply but aren't saved. See
[Bench reports problems with config.toml](#bench-reports-problems-with-configtoml).
If Bench can't write `config.toml`, it says so, and it stops saving the
arrangement too, so that `state.json` still matches the file; saving a tab
in the editor tries again.

## Small screens: Auto, Wide, and Compact

Bench chooses a wide or a compact layout for the screen its window is on.
The control at the right of the title bar sets how:

| Choice | What it does |
| --- | --- |
| **Auto** | Compact on screens narrower than `compact_below` (1,800 points by default), wide on others. |
| **Wide** | Always the wide layout, until you choose Auto again. |
| **Compact** | Always the compact layout, until you choose Auto again. |

The status bar says which layout is in use and why, such as "Wide layout,
chosen for a 2,560-pt screen". When a display is connected or disconnected,
or Bench's window moves to another screen, Bench waits for macOS to settle,
then chooses again and rearranges the current tab.

In the compact layout, each tab follows its small-screen setting:

| Setting | In the editor | What it does |
| --- | --- | --- |
| `stack` | **Stack panes** | The top-level row becomes a column, so side by side becomes top and bottom. |
| `tabs` | **One pane with sub-tabs** | The tab shows one pane, with every window as a sub-tab. The layout comes back on a larger screen. |
| `same` | **Keep the layout** | No change. |

In the default config, Code stacks its panes, and Comms and Research show
one pane with every window as a sub-tab.

## Live previews

With live previews on, hovering over an inactive sub-tab for a moment shows
a small, live picture of its window, with its title. In the jump palette,
the window you select or hover over shows its preview beside the palette.

Previews are off until you turn on **Show live previews of windows** in the
layout editor, because they need Screen Recording permission. macOS asks for
it when you turn them on. macOS asks you to approve Screen Recording again
from time to time.

Chromium-based apps, such as Brave, VS Code, Slack, and Obsidian, can stop
drawing windows that are covered, so their previews can show a frozen
picture.

## New windows and links

When an app opens a new window, Bench places it in the first case that
matches:

1. A [rule](#rules) with `float = true` matches: Bench leaves the window
   floating.
2. Bench is waiting for the window: you asked for it with **New window** in
   the add dialog, it belongs to a sub-tab that Bench remembers from before
   the app quit, or Bench opened the app for a tab or for the add dialog.
   The window goes to that sub-tab or pane.
3. A rule with `tab = "…"` matches: the window joins that tab, in the pane
   that holds the app's most recently used window, or else that tab's focused
   pane.
4. Otherwise, `new_windows` decides. By default, the window joins the pane
   that holds the app's most recently used window, or else the pane the
   config gives the app, or else the current tab's focused pane.

So a rule's `tab` comes before an item's `title`: with a Research item
`{ app = "com.brave.Browser", title = "Docs" }` and a rule sending Brave's
windows to Comms, a new Brave window called "Go Docs" joins Comms.

Dialogs, sheets, settings windows, and floating panels always stay floating
above Bench.

Links and notifications often bring another tab's window forward: a link in
Slack can open in a Brave window that lives in Research. By default, Bench
switches to that tab. The `follow_focus` setting can instead bring the window
into the current tab, or do nothing.

A Web item's page has no address bar, so Bench keeps other sites out of it:

- A link to another site that you click opens in your default browser.
  Pages on the item's own site, and the sign-in pages of well-known identity
  providers, stay in the item. Anyone can have a sign-in page with some of
  those providers, so after you click a link to one, a page it sends you on
  to that's neither the item's site nor another sign-in page opens in your
  browser, and the item goes back to its own address.
- A link for another app, such as a Zoom meeting, works only when you click
  it, not when the page follows it by itself. Email addresses and phone
  numbers open in their app at once. For other apps, Bench asks first,
  naming the app, while the page is on screen. **Cancel** is the default
  button, and **Open** works after a moment. Tick **Don't allow more from
  this page** to stop the page asking.
- Downloads go to your Downloads folder. A page can start up to 5
  downloads in the minute after each click or key press in it, and none by
  itself.

## Keyboard shortcuts

These shortcuts work from any app. You can change each one in the
[`[shortcuts]` table](#shortcuts).

| Shortcut | Action | Config setting |
| --- | --- | --- |
| Control+Option+1 to 9 | Switch to tab 1 to 9. | `tab_modifiers` |
| Control+Option+] and Control+Option+[ | Next and previous tab. | `next_tab`, `previous_tab` |
| Control+Option+Space | Open the jump palette. | `jump` |
| Control+Option+0 | Rearrange the current tab. | `rearrange` |
| Control+Option+arrow | Focus the pane in that direction. | `focus_pane_modifiers` |
| Control+Option+Shift+arrow | Move the shown window to the pane in that direction, or split there. | `move_window_modifiers` |
| Control+Option+Tab, and with Shift | Next and previous sub-tab in the focused pane. | `next_window` |

If another app or macOS already uses a shortcut, the status bar says so and
suggests another. See [A shortcut doesn't work](#a-shortcut-doesnt-work).

In Bench's menus, dialogs, and palette, Escape closes, the arrow keys move
the selection, and Return chooses.

## When you hide Bench

When you hide Bench (**Bench ▸ Hide**, Command+H, or **Hide** in its Dock
menu), minimise it, or close its window, the current tab's apps hide with
it, and so do its web pages.
Closing the window doesn't quit Bench. To bring it back, choose **Show
Bench** from its menu bar item, or press the jump palette's shortcut. The
tab's apps come back with it.

**Hide Others** in another app hides Bench as it hides any app, but leaves
that app's windows where they are, even if Bench docks some of them.

## When Bench quits

When you quit Bench, from **Bench ▸ Quit Bench** or its menu bar item:

- App windows stay where they are, and work as normal windows.
- Windows that Bench moved off screen to hide them come back on screen, where
  they were before, or centred on the screen. Apps that Bench hid are shown
  again.
- Web items and terminals close with Bench. Terminal sessions end, and VS
  Code for the Web stops.
- Bench saves the arrangement. When you open Bench again, it matches windows
  to their sub-tabs: by app and exact title first, then by the order they
  were created in, then by putting the rest in the pane that holds their app.

If Bench can't put a window back as it quits, for example because the
window is on another desktop, it tries again the next time it opens.

If Bench crashes, a small helper process that Bench starts puts hidden
windows back on screen. If that fails too, Bench puts them back the next time
it opens, or you can run `bench restore`.

## Bench's files

Bench keeps its files in `~/Library/Application Support/Bench/`, or in the
folder that `$BENCH_HOME` names, which must be one that only you can change:

| File | What it is |
| --- | --- |
| `config.toml` | Tabs, layouts, settings, shortcuts, and rules. Bench creates it on first run, and the layout editor updates it. See the [config reference](#config-reference). |
| `config.toml.bak` | The previous `config.toml`, kept by every save. |
| `state.json` | The live arrangement after dragging and splitting. Bench sets it aside as `state.json.bad` if it doesn't parse or match the config. |
| `parked.json` | The windows that Bench has moved off screen, and the apps it has hidden, right now, so that they can be put back after a crash. Bench sets it aside as `parked.json.bad` if it doesn't parse. |
| `vscode.json` | The VS Code for the Web server that Bench is running, so that it can be stopped after a crash. |
| `vscode-token-*` | The connection token of the VS Code for the Web server that Bench is running, readable only by you. |
| `bench.lock` | A lock that Bench and `bench arrange` hold while they run, and `bench restore` and the watchdog while they put windows back, so that they never act on `parked.json` at the same time. |
| `first-run-done` | Present once you've closed the permissions guide. |
| `view.json` | Only with `BENCH_DEBUG` set: what Bench's window shows, for debugging. See [Read the log](development.md#read-the-log). |

To remove Bench completely, quit it, move `Bench.app` to the Bin, and delete
this folder. Then remove Bench from **System Settings ▸ Privacy & Security ▸
Accessibility**, and from **Screen Recording** if you turned on live
previews.

## Command line

`Bench.app` contains a command-line tool, `bench`. With no command, it runs
the app. Its commands are for checking your config and for recovery:

| Command | What it does |
| --- | --- |
| `bench check-config [--config path]` | Checks `config.toml` and reports each problem with its tab and field. |
| `bench layout --width w --height h [--y points] [--compact] [--config path]` | Prints every tab's pane frames for a screen size, as JSON. |
| `bench arrange <tab> [--config path]` | Arranges one tab across the screen without Bench's window, until you press Control-C or close the terminal. It refuses while Bench is running, and Bench can't open until it has finished. |
| `bench restore [--dir path]` | Puts back windows that a crashed Bench left off screen, and says how many it put back. It refuses while Bench or `bench arrange` runs from the same folder. |
| `bench watchdog --dir path` | Waits for Bench to quit, then puts back what it left off screen. Bench starts it itself. |
| `bench version` | Prints the version. |
| `bench help [command]` | Explains a command and its flags. |

If Bench is in your Applications folder, run a command like this:

```sh
/Applications/Bench.app/Contents/MacOS/bench check-config
```

## Config reference

Bench's config file is `~/Library/Application Support/Bench/config.toml`.
Bench creates it on first run, and the layout editor updates it, keeping the
previous version as `config.toml.bak`. You only need to edit it for things
the layout editor doesn't cover, such as shortcuts, rules, and web pages.

Edit the file while Bench isn't running. Bench reads it when it starts, and
saving in the layout editor, or deleting a tab, writes Bench's own copy over
it, in the layout of the [default file](#the-default-file): your comments
and the order you wrote things in aren't kept. `config.toml.bak` keeps the
version before the last save.

When Bench starts, it keeps the arrangement it saved when it quit, and
applies your edits to a tab's items to it: an item takes its entry's new
settings, such as a `name`, `profile`, or `url`, or a changed address or
command; the items of an entry you removed leave the tab; and an entry you
added joins the pane the file lists it in. An `app` entry whose `title` you
change counts as a new entry, so its app's windows are placed again by the
new `title`. An item you closed in Bench stays in the file until the layout
editor next saves the tab, but isn't added back unless you change its
entry's address, folder, command, or `title`.

Bench checks the file when it starts and when you save in the layout editor.
If the file has problems, Bench lists them in the status bar, with the tab
and field for each, and runs with the default config without overwriting
your file. Until you fix the file, Bench doesn't save the layout editor's
changes or the arrangement (`state.json`), so they're lost when you quit.
The same goes for a `config.toml` that's a link to a file that isn't there,
such as one in a synced folder that isn't available yet: Bench leaves the
link alone rather than write the default config over it.
To check the file yourself, run `bench check-config` with the binary inside
the app. If Bench is in your Applications folder:

```sh
/Applications/Bench.app/Contents/MacOS/bench check-config
```

The file is [TOML](https://toml.io). It has four parts: `[settings]`,
`[shortcuts]`, one `[[tab]]` per tab, and any number of `[[rule]]` entries.
The [default file](#the-default-file) shows them all.

### Settings

The `[settings]` table. Any setting you leave out takes its default.

| Setting | Default | Values | What it does |
| --- | --- | --- | --- |
| `compact_below` | `1800` | A positive number of points | Screens narrower than this are small, and use the compact layout in Auto. |
| `gap` | `8` | 0 to 100 points | The space between panes, and around them. |
| `open_missing_apps` | `true` | `true` or `false` | Open a tab's apps when you switch to it, if they aren't running. |
| `hide_other_tabs` | `true` | `true` or `false` | Hide windows that belong to other tabs. |
| `hide_other_apps` | `false` | `true` or `false` | When switching tabs, also hide every app that Bench doesn't manage. |
| `new_windows` | `"app-pane"` | `"app-pane"`, `"focused-pane"`, or `"float"` | Where an app's new windows go. See below. |
| `follow_focus` | `"switch-tab"` | `"switch-tab"`, `"bring-here"`, or `"off"` | What happens when another tab's window comes forward. See below. |
| `live_previews` | `false` | `true` or `false` | Show live previews of windows. Needs Screen Recording permission. |
| `launch_timeout` | `"15s"` | A duration, such as `"15s"` or `"1m30s"` | How long to wait for a tab's apps to open their windows. |

`new_windows` values:

| Value | Where a new window goes |
| --- | --- |
| `app-pane` | The pane that holds the app's most recently used window, looking in the current tab first, then other tabs. Then the pane the config gives the app: the one with an item whose `title` the window's title contains, or else the first that lists the app without a `title`. If there's none, the current tab's focused pane. |
| `focused-pane` | The current tab's focused pane. |
| `float` | Nowhere: it floats, for you to add. |

`follow_focus` values:

| Value | When another tab's window comes forward |
| --- | --- |
| `switch-tab` | Bench switches to that tab. |
| `bring-here` | Bench moves the window into the current tab's focused pane. |
| `off` | Bench does nothing. |

### Shortcuts

The `[shortcuts]` table. Any shortcut you leave out takes its default.

| Setting | Default | What it is |
| --- | --- | --- |
| `tab_modifiers` | `"ctrl+alt"` | Modifiers that, with 1 to 9, switch to tabs 1 to 9. |
| `next_tab` | `"ctrl+alt+]"` | Next tab. |
| `previous_tab` | `"ctrl+alt+["` | Previous tab. |
| `jump` | `"ctrl+alt+space"` | Open the jump palette. |
| `rearrange` | `"ctrl+alt+0"` | Rearrange the current tab. |
| `focus_pane_modifiers` | `"ctrl+alt"` | Modifiers that, with an arrow key, focus the pane in that direction. |
| `move_window_modifiers` | `"ctrl+alt+shift"` | Modifiers that, with an arrow key, move the shown window in that direction. |
| `next_window` | `"ctrl+alt+tab"` | Next sub-tab in the focused pane. With Shift added, the previous sub-tab. |

Write a shortcut as modifiers and a key joined with `+`, such as
`"ctrl+alt+space"`:

- **Modifiers**: `ctrl` (or `control`), `alt` (or `option`), `shift`, and
  `cmd` (or `command`).
- **Keys**: `a` to `z`, `0` to `9`, `space`, `tab`, `return`, `escape`,
  `delete`, `forwarddelete`, `home`, `end`, `pageup`, `pagedown`, `left`,
  `right`, `up`, `down`, `f1` to `f20`, and the punctuation keys
  ``= - [ ] ' ; \ , / . ` ``. Punctuation keys are the keys in those places
  on a US keyboard, whatever your layout prints on them.

A shortcut needs at least one of `ctrl`, `alt`, and `cmd`, so that it
doesn't take over typing; only the function keys can do without. To turn a
shortcut off, set it to `""`. If two settings make the same shortcut, the
first in the table above keeps it.

A shortcut that Bench can't read, or can't register, stays off: Bench says
so in the status bar, and the rest of the config still applies.

### Tabs

Each `[[tab]]` entry is a tab, in order:

| Field | Required | What it is |
| --- | --- | --- |
| `name` | Yes | The tab's name. Names must be unique, ignoring case. |
| `layout` | Yes | A layout: `single`, `side`, `main-side`, `three`, `top-bottom`, or `grid`. |
| `sizes` | No | Percentages for the layout's top-level split, one per part, such as `[60, 40]`. Bench scales them if they don't add up to 100. |
| `small_screens` | No | `stack`, `tabs`, or `same`. The default is `stack`. |
| `[[tab.pane]]` | No | One entry per pane, in reading order, each with an `items` list. Extra panes are an error; missing ones start empty. |

### Items

Each pane's `items` list holds items, written as inline tables, such as
`{ app = "dev.zed.Zed" }`. Each item needs exactly one of `app`, `web`,
`vscode`, or `terminal`:

| Field | Item kind | What it is |
| --- | --- | --- |
| `app` | App | The app's bundle ID. Every standard window of the app in this tab starts in this pane, unless another pane claims it with `title`. |
| `title` | App | Only windows whose title contains this text, ignoring case. |
| `url` | App, with `title` | A link to open in the app when no matching window exists. It needs a scheme, such as `https://`, or the app's own, such as `notion://`. Apps often title the window they open for a link only once the page has loaded, so while Bench waits for the window, the window joins the item as soon as its title matches. |
| `web` | Web | The address to load. It must start with `https://`. |
| `profile` | Web (`web` items only) | The name of the item's data store. Items with the same profile share logins and cookies. Items without a profile share one default profile. Profile names are case-sensitive: `Work` and `work` are different profiles. |
| `vscode` | Web | A folder to open in VS Code for the Web. |
| `terminal` | Built-in | The terminal's starting folder. `~` means your home folder. |
| `command` | Built-in | A command to run instead of your shell. |
| `name` | Web and Built-in | The sub-tab's label until the page or shell sets a title. |

A `terminal` item runs your login shell, or its `command` through that
shell, in its folder. If the folder doesn't exist, a shell starts in your
home folder instead and says so, but a `command` doesn't run at all, because
its relative paths would name other files there: its pane says that the
terminal couldn't start. Terminals have `TERM=xterm-256color`,
`TERM_PROGRAM=Bench`, and Bench's version as `TERM_PROGRAM_VERSION`. Bench opened from the Finder, the Dock, or at login has
no `LANG`, so terminals get one that matches the language and region in
System Settings, such as `de_DE.UTF-8`, or `en_US.UTF-8` if macOS has no
such locale. When you start Bench from another terminal, its terminals leave
out the variables that describe that terminal's session, such as `TMUX`,
`ITERM_SESSION_ID`, and `SHLVL`.

An app can appear in several tabs and panes. Use `title` to divide its
windows; windows that match no `title` go to the first pane that lists the
app without one.

To find an app's bundle ID, run:

```sh
osascript -e 'id of app "Brave Browser"'
```

A `vscode` item runs `code serve-web` from VS Code 1.82 or later: the `code`
command in Bench's `PATH`, or else the one in
`/Applications/Visual Studio Code.app`. Bench opened from the Finder or the
Dock gets macOS's short default `PATH`, without `/usr/local/bin` or
`/opt/homebrew/bin`, so keep VS Code in `/Applications`. It shows VS Code
for the Web, working on your Mac's own files and terminals. Check that the
extensions you rely on work there; if they don't, use the desktop app as an
App item. If the server can't start, or stops, as it can when VS Code
updates itself, the item says so, and Bench starts the server again when
the item is shown, but not within 10 seconds of the last try.

Some examples, each the `items` line of a `[[tab.pane]]` entry:

```toml
# Slack's desktop app, for Google sign-in.
items = [{ app = "com.tinyspeck.slackmacgap" }]

# Two web pages that share logins.
items = [
  { web = "https://github.com/notifications", profile = "github", name = "Notifications" },
  { web = "https://github.com/pulls", profile = "github", name = "Pull requests" },
]

# Brave windows divided by title. Go's documentation page is called
# "Documentation - The Go Programming Language".
items = [{ app = "com.brave.Browser", title = "Documentation", url = "https://go.dev/doc/" }]

# VS Code for the Web, and a terminal running a command.
items = [{ vscode = "~/code/bench" }, { terminal = "~/code/bench", command = "go test ./...", name = "Tests" }]
```

### Rules

Each `[[rule]]` entry says where new windows of an app go:

| Field | Required | What it is |
| --- | --- | --- |
| `app` | Yes | The app's bundle ID. |
| `title` | No | Only windows whose title contains this text, ignoring case. |
| `float` | One of `float` and `tab` | `true` leaves matching windows floating, such as a browser's picture-in-picture window. |
| `tab` | One of `float` and `tab` | The name of the tab that new windows of the app join. |

```toml
[[rule]]
app = "com.apple.mail"
tab = "Comms"
```

### Problems Bench reports

Bench reports these problems, naming the tab and field for each, and runs
with the default config until you fix them:

- No tabs, an empty tab name, or two tabs with the same name.
- An unknown layout or small-screen setting.
- More `[[tab.pane]]` entries than the layout has panes.
- `sizes` with the wrong number of parts for the layout, or numbers that
  aren't positive.
- An item with none, or more than one, of `app`, `web`, `vscode`, and
  `terminal`, or with a field that doesn't apply to its kind.
- A `web` address that isn't `https`, or an app item's `url` with no scheme.
- A rule with no `app`, with both or neither of `float` and `tab`, or naming
  a tab that doesn't exist.
- An unknown `new_windows` or `follow_focus` value, a `compact_below` that
  isn't positive, a `gap` below 0 or above 100, or a `launch_timeout` that
  isn't a positive duration.
- A field Bench doesn't know, which is often a typo. Bench suggests the field
  you might have meant.

`bench check-config` also reports shortcuts it can't read, and two settings
that make the same shortcut.

If `state.json`, where Bench saves the live arrangement, doesn't parse or
doesn't match the config's tabs, Bench sets it aside as `state.json.bad` and
starts from the config.

### The default file

```toml
[settings]
compact_below = 1800          # Screens narrower than this, in points, are small.
gap = 8                       # Points between panes.
open_missing_apps = true      # Open a tab's apps if they aren't running.
hide_other_tabs = true        # Hide windows that belong to other tabs.
hide_other_apps = false       # Also hide every app Bench doesn't manage.
new_windows = "app-pane"      # app-pane, focused-pane, or float
follow_focus = "switch-tab"   # switch-tab, bring-here, or off
live_previews = false         # Needs Screen Recording permission.
launch_timeout = "15s"

[shortcuts]
tab_modifiers = "ctrl+alt"          # Plus 1 to 9.
next_tab = "ctrl+alt+]"
previous_tab = "ctrl+alt+["
jump = "ctrl+alt+space"
rearrange = "ctrl+alt+0"
focus_pane_modifiers = "ctrl+alt"   # Plus an arrow key.
move_window_modifiers = "ctrl+alt+shift"
next_window = "ctrl+alt+tab"

[[tab]]
name = "Comms"
layout = "side"
sizes = [45, 55]
small_screens = "tabs"

  [[tab.pane]]
  items = [{ web = "https://app.slack.com/client", profile = "slack", name = "Slack" }]

  [[tab.pane]]
  items = [{ web = "https://outlook.office.com/mail/", profile = "outlook", name = "Outlook" }]

[[tab]]
name = "Code"
layout = "main-side"
sizes = [65, 35]
small_screens = "stack"

  [[tab.pane]]
  items = [{ app = "dev.zed.Zed" }, { app = "com.microsoft.VSCode" }]

  [[tab.pane]]
  items = [{ terminal = "~/code" }]

[[tab]]
name = "Research"
layout = "side"
sizes = [60, 40]
small_screens = "tabs"

  [[tab.pane]]
  items = [{ app = "com.brave.Browser" }]

  [[tab.pane]]
  items = [{ app = "md.obsidian" }]

[[rule]]
app = "com.brave.Browser"
title = "Picture in picture"
float = true
```

## Troubleshooting

### Bench can't move windows

Bench needs Accessibility permission. Open **System Settings ▸ Privacy &
Security ▸ Accessibility** and turn on Bench. Bench notices within a few
seconds, and its banner goes away.

If Bench is already turned on there but still can't move windows, which can
happen after updating Bench, select Bench in the list, remove it with the
minus button, then open Bench and allow it again.

### Live previews don't appear

Previews need Screen Recording permission. Open **System Settings ▸ Privacy
& Security ▸ Screen Recording** (called **Screen & System Audio Recording**
in macOS 15 and later), turn on Bench, and reopen Bench if macOS asks you
to. macOS asks you to approve this permission again from time to time.

If a preview shows a picture that doesn't change, the app may have stopped
drawing its covered window. Chromium-based apps such as Brave, VS Code,
Slack, and Obsidian do this.

### A shortcut doesn't work

When Bench can't register a shortcut, the status bar says which one and
suggests another, such as "Try ⌃⌥⌘ with the arrow keys: set
focus_pane_modifiers in config.toml." The usual causes:

- **Input sources.** With more than one input source, macOS uses
  Control+Option+Space to switch between them. Turn that shortcut off in
  **System Settings ▸ Keyboard ▸ Keyboard Shortcuts ▸ Input Sources**, or
  change `jump` in the `[shortcuts]` table.
- **Window tools.** Rectangle and similar tools use Control+Option+arrow by
  default. Change their shortcuts, or change `focus_pane_modifiers` and
  `move_window_modifiers`.

After editing `config.toml`, quit and reopen Bench.

### Google sign-in doesn't work in a web item

Google blocks its sign-in pages in embedded web views, with the error
`disallowed_useragent`, and Web items are embedded web views. If you sign
in to Slack (or another service) with Google, use its desktop app as an App
item instead:

```toml
items = [{ app = "com.tinyspeck.slackmacgap" }]
```

### There's a thin line in a corner of the screen

To hide a window of an app that also has windows in the current tab, Bench
moves it to a bottom corner of the screen, outside the visible area. macOS
doesn't allow a window fully off screen, so a 1-pixel line can stay visible
in the corner.

With more than one display, Bench uses a corner that doesn't touch another
display, so that the window doesn't show there. If both bottom corners do,
it moves the window along the bottom edge to a place clear of the other
displays, which leaves a longer line, 1 pixel high. If there's no such
place, as for a window wider than the screen between two displays that
reach lower, Bench minimises the window instead, and restores it when it's
needed again.

To make it less noticeable, put the Dock at the bottom of the screen and
have it hide automatically: in **System Settings ▸ Desktop & Dock**, set
**Position on screen** to **Bottom** and turn on **Automatically hide and
show the Dock**.

### A window isn't in its pane

If an app keeps moving or resizing its own docked window, Bench puts it back
three times in ten seconds, then leaves it where the app put it. Arrange the
tab again with Control+Option+0, **Rearrange this tab** in the jump palette,
or **View ▸ Rearrange**.

If you dragged the window away yourself, its pane says "Moved out of its
pane": click **Dock again**.

### An app isn't responding

When an app hangs (it shows the spinning wait cursor), Bench waits up to a
second for it, then leaves it alone for a few seconds, so that its tabs and
panes keep responding. The app's windows keep their items and places, and
Bench docks them again the next time it arranges the tab once the app
answers. Rearrange the tab with Control+Option+0 if they don't.

### A window shows "On another desktop"

Bench only sees windows on the current desktop (Space). Keep Bench and the
apps it manages on one desktop. Move the window back to Bench's desktop, and
Bench shows it in its pane again.

### A window doesn't fit its pane

Some apps have a minimum window size that's bigger than a small pane. Bench
places the window at the pane's top-left and says it can't shrink. Make the
pane bigger by dragging the gap beside it, use a layout with fewer panes, or
choose **Compact** in the title bar.

### Notifications from a web item don't appear

Web pages in Bench can show notifications only while their item is loaded:
web views on macOS don't support push notifications. Check that Bench is
allowed in **System Settings ▸ Notifications**. For notifications when the
page isn't open, use the service's desktop app as an App item.

### Bench doesn't go full screen

Bench's window doesn't use native full screen, because App windows can't
follow it into a full-screen space. Choose **View ▸ Fill the Screen**
instead.

### Windows are stuck off screen after a crash

Open Bench again: it puts back windows that a crashed Bench left off screen.
Or run `bench restore` from Terminal, using the binary inside the app. If
Bench is in your Applications folder:

```sh
/Applications/Bench.app/Contents/MacOS/bench restore
```

It says how many windows it put back, and how many had closed since. It
refuses while Bench is running, because a running Bench puts its windows
back itself as it quits: quit Bench first.

### Bench reports problems with config.toml

The status bar lists each problem with its tab and field, and Bench runs
with the default config until you fix them. Until then, Bench doesn't save
the layout editor's changes or the arrangement (`state.json`), so they're
lost when you quit. Run `bench check-config` to see the list, fix the file,
then quit and reopen Bench. If you can't fix it,
rename the file: Bench writes a new default one when it next opens. Your
previous version is in `config.toml.bak` after any save from the layout
editor.
