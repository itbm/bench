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
- [The status bar](#the-status-bar)
- [Move windows](#move-windows)
- [Show a window in more than one tab](#show-a-window-in-more-than-one-tab)
- [Close windows and tabs](#close-windows-and-tabs)
- [Undo, and reopen a closed tab](#undo-and-reopen-a-closed-tab)
- [Add to a pane](#add-to-a-pane)
- [Web apps](#web-apps)
- [The jump palette](#the-jump-palette)
- [Tab properties](#tab-properties)
- [Settings](#settings)
- [Move and resize dialogs](#move-and-resize-dialogs)
- [Small screens: Auto, Wide, and Compact](#small-screens-auto-wide-and-compact)
- [Live previews](#live-previews)
- [New windows and links](#new-windows-and-links)
- [Keyboard shortcuts](#keyboard-shortcuts)
- [Fill the screen](#fill-the-screen)
- [When you hide Bench](#when-you-hide-bench)
- [When Bench quits](#when-bench-quits)
- [Version and acknowledgements](#version-and-acknowledgements)
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

Bench starts with one tab, Research, which holds your default browser: the
app that opens web links, or Safari if there's none. Bench doesn't guess at
other apps, since your Mac may not have them. To change it, right-click the
tab and choose [Tab properties](#tab-properties). Add your own tabs with
**+** in the title bar, or in the
[config file](#config-reference).

Bench's window fills the screen, below the menu bar and beside the Dock.
See [Fill the screen](#fill-the-screen).

Bench has a menu bar item: Bench's icon, with the current tab's name beside
it. Click it to switch tabs, open **Jump to…** or **Settings**, show Bench,
or quit. Its menu shows each tab's shortcut. To show the icon alone, or no
menu bar item at all, see [Settings](#settings).

## Tabs

A tab is a named workspace with its own layout. The tabs are in Bench's title
bar, and the current tab has a raised background and an orange underline.
Each of the first nine tabs shows its shortcut after its name, except on
small screens, where the tabs need the room; **Show shortcuts on tabs** in
[Settings](#settings) changes that. The current tab, and a tab under the
pointer, show a **×** in the shortcut's place, which closes the tab: see
[Close windows and tabs](#close-windows-and-tabs). Hold the pointer over a
tab to see its whole name and its shortcut.

To switch tabs:

- Click a tab.
- Press Control+Option+1 to 9 for the first nine tabs, or Control+Option+]
  and Control+Option+[ for the next and previous tab.
- Choose a tab in the [jump palette](#the-jump-palette), or from Bench's menu
  bar item.

When you switch to a tab whose apps aren't open, Bench opens them, unless
`open_missing_apps` is off. It doesn't open an app that you quit yourself,
until the app opens again: see [When an app quits](#when-an-app-quits). If
the focused pane is still waiting for its window, the window gets the
keyboard as soon as it arrives, unless you've focused something else or
turned to another app in the meantime, or a menu or dialog is open.

There's no limit on the number of tabs. Only the first nine get a
shortcut. When they don't all fit, the tab strip scrolls: see
[When tabs and sub-tabs don't fit](#when-tabs-and-sub-tabs-dont-fit).

Right-click a tab, or Control-click it, for its menu, which opens where you
click. Right-clicking a tab doesn't switch to it. The menu has:

- **Tab properties…** and **Rename…**, which open the tab's
  [Tab properties](#tab-properties): its name, which **Rename…** selects
  for you to type over, its layout, and what it does on small screens. To
  delete the tab, use **Delete tab** there.
- **Rearrange**, which arranges the tab's windows again. For a tab that
  isn't the current one, Bench switches to it first.
- **Reset layout**, which returns the tab to the layout in its config,
  undoing your dragging and splitting, without asking first. The message
  that says so has **Undo**.
- **Close tab…**, which closes the tab's windows, web pages, and terminals,
  and deletes the tab, and **Delete tab (keep windows)**, which deletes the
  tab and moves its windows to **Not in a tab**. On the only tab, it's
  **Close all windows…**, which leaves the tab, empty, and there's no
  **Delete tab**. See [Close windows and tabs](#close-windows-and-tabs).
- **New tab…**, and **Reopen closed tab**, with the name of the tab you
  deleted or closed last, which brings it back. See
  [Undo, and reopen a closed tab](#undo-and-reopen-a-closed-tab).

To add a tab, click **+** after the tabs, or choose **New tab…** from a
tab's right-click menu. **New tab** opens, to name the tab and choose its
layout.

### When tabs and sub-tabs don't fit

A tab is never wider than 200 points: a longer name is cut short with an
ellipsis, keeping the tab's shortcut at its end, and the tab's tooltip has
the whole name. A sub-tab is never narrower than 96 points, so that its
monogram and the start of its title stay readable, and is between 220 and
360 points at its widest, more in a wider pane. Its tooltip has its whole
title.

When the tabs in the title bar, or a pane's sub-tabs, don't all fit, their
strip scrolls sideways rather than squashing them:

- To scroll a strip, hold the pointer over it and swipe on a trackpad, up
  and down or sideways, or turn a mouse wheel.
- An end of a strip with more to show fades out.
- The current tab, and the sub-tab that each pane shows, stay in view. When
  you switch tabs or sub-tabs, add a sub-tab, or resize Bench, the strip
  scrolls to show them. A strip that you've scrolled yourself, or moved
  along with the Tab key, stays where you left it until it shows another
  tab or sub-tab, one is added or removed, or Bench is resized.
- While you [drag a sub-tab](#move-windows), hold it near either end of
  the tab strip to scroll the strip, so as to drop it on a tab that's out
  of view. A pane's sub-tabs scroll in the same way.
- **Show scroll bars** in [Settings](#settings) chooses whether a strip that
  doesn't fit shows a thin scroll bar below it. It takes no room: the title
  bar and the pane headers stay the same height.

## Panes

A tab's layout divides the window into panes. Each pane shows one window at a
time, with a sub-tab in its header for each window it holds.

- **Focus a pane** by clicking anywhere in it, or with Control+Option+arrow.
  The focused pane has an orange border, and what it shows gets the
  keyboard: an App window, a web page, or a terminal.
- **Resize panes** by dragging the gap between them. The keyboard stays
  where it was, so you can carry on typing in a terminal. To resize with
  the keyboard, press Tab until the gap is selected, then press the arrow
  keys: each press moves it by 2%, or 10% with Shift.
- **Arrange the tab again** with Control+Option+0, **Rearrange this tab** in
  the jump palette, or **View ▸ Rearrange**, if windows have drifted out of
  place.

A pane's header holds:

| Control | What it does |
| --- | --- |
| Sub-tabs | Show a window. See [Sub-tabs](#sub-tabs). |
| **+** | [Add a window or app](#add-to-a-pane) to the pane. |
| Badge | The shown item's kind: **Web**, **App**, or **Built-in**. A web page shows its host instead, such as app.slack.com: see [Where a web page is](#where-a-web-page-is). |
| **Show side by side** | Split the pane to the right, with the shown window in the new half. |
| **⋯** | More ways to move the shown window. See [Move windows](#move-windows). Right-click a sub-tab, or an empty part of the header, to open the same menu where you click. |

Hold the pointer over a button in the title bar or a pane header to see
what it does, with its shortcut if it has one. Tooltips can also show while
you work in another app; if one doesn't, click Bench's title bar first.

### More room for your apps

On a laptop, Bench's own title bar and pane headers take less room:

- **Density** in [Settings](#settings) sizes the title bar, the tabs, the
  pane headers, and their sub-tabs and buttons. **Automatic**, the default,
  is compact in the compact layout, as on a 13- or 14-inch MacBook, and
  standard otherwise. In the compact density, a pane header is 32 points
  tall rather than 40, and the title bar 39 rather than 53. The panes change
  at once; the title bar changes the next time Bench opens, because macOS
  places the traffic lights as it makes the window.
- A tab that shows **one pane with sub-tabs** on a small screen (see
  [Small screens](#small-screens-auto-wide-and-compact)) has no pane
  header. The pane's sub-tabs, its **+**, and the header's other buttons
  are in the title bar, after the tabs, and the pane reaches up to the
  title bar. The tabs and the sub-tabs each scroll on their own. The
  sub-tabs work as in a header: click one to show its window, drag it to
  another tab, or right-click it for its menu. Right-click beside them for
  the shown window's menu.
- **Hide the sub-tab row when a pane holds one app window** in Settings
  is off unless you turn it on. With it on, a pane that holds one App
  window, with nothing to report, has no header: the app's own title bar
  takes its place, just inside the pane's border. The header comes back as
  soon as the pane gets another window, and after a second when the window
  has something to report, such as **Needs more room**, so that a message
  that comes and goes doesn't move the window down and up. Once back, it
  stays for at least two seconds. Web pages and terminals keep their
  headers, as they have no title bar of their own.

  Without the header, the [jump palette](#the-jump-palette) has what its
  buttons do: **Add to this pane…** adds to the focused pane, **Show side
  by side** splits it, with its window in the new half, and **Window
  options…** opens the window's ⋯ menu, with **Pop out of Bench**, **Move
  to tab**, and the rest. Control+Option+Shift+arrow moves the window, as
  ever.

Some apps can hide their own toolbar, which leaves a thinner title bar:
choose **View ▸ Hide Toolbar** in Finder, Mail, or Preview. Bench can't hide
another app's title bar itself: macOS doesn't let one app change another's
window that way. A window with no title bar can't be undocked by dragging,
as there's nothing to drag it by. Use **Pop out of Bench** in its menu
instead (see [Move windows](#move-windows)). While its pane has no header,
**Window options…** in the jump palette opens that menu.

When a pane loses its last window, Bench removes it and the panes beside it
close up, unless the pane is keeping places for the windows of an app that
has quit: see [When an app quits](#when-an-app-quits). The last pane in a
tab stays, empty, with an **Add a window or app** button. Another pane that
shows nothing has a **Close this pane** button too. An empty pane has no
**⋯**: right-click it, or its header, for the same choices.

## Sub-tabs

Each window in a pane has a sub-tab, showing the app's monogram and the
window's title. Click a sub-tab to show its window, or press
Control+Option+Tab (and Control+Option+Shift+Tab) to move between the
focused pane's sub-tabs. The shown sub-tab has a **×** at its right end,
and the others show one under the pointer: it closes the window, as a
middle-click on the sub-tab does. See
[Close windows and tabs](#close-windows-and-tabs).

A pane can hold three kinds of item, from any mix of apps:

| Kind | Badge | What it is | Examples |
| --- | --- | --- | --- |
| Web | **Web**, or the page's host | A web page that Bench shows inside its own window, with its own logins. | Slack, Outlook on the web, VS Code for the Web |
| App | **App** | Another app's real window, which Bench moves and sizes to fit the pane. It works as normal: typing, menus, and drag and drop. | Zed, VS Code, Brave, Obsidian |
| Built-in | **Built-in** | A terminal session that Bench draws itself. Its sub-tab shows the running command and the folder, or its `name`. It closes when its shell exits; a terminal running a `command` stays when the command finishes. | `zsh`, `go test` |

macOS doesn't let one app put another app's window inside its own, so an App
item is still a separate window. Bench keeps it exactly over its pane, keeps
a pane's other windows directly behind the shown one, and hides windows that
belong to other tabs.

When an item can't be shown, its pane says why. A pane that has nothing to
show also says why for each app it's waiting for (see
[When an app quits](#when-an-app-quits)):

| Message | What it means |
| --- | --- |
| Opening *app*… | Bench is opening the app and waiting for its window. |
| *App* isn't running | The app isn't open. Bench opens a tab's apps when you switch to it, unless `open_missing_apps` is off or you quit the app yourself. Click **Open** *app* to open it, or **Remove from this pane** if you don't want it here. |
| *App* isn't installed | macOS doesn't know the app. Bench doesn't try to open it again until it opens, and the pane has no **Open** button for it. Click **Remove from this pane** to take it out of the pane. |
| *App* has no window open for this | The app is open, but none of its windows belongs here. Click **Open** *app* to ask the app for a new window, open one in the app, or add one with the pane's **+**. Click **Remove from this pane** if you don't want it here. |
| Bench couldn't open *app* | macOS couldn't open the item's `url` in the app, for example because it isn't a valid link. Check the item's `url` in `config.toml`, or click **Remove from this pane**. |
| Moved out of its pane | You dragged the window away with the mouse. Click **Dock again** to put it back. |
| *App*'s window can't shrink to fit this pane | The window's minimum size is bigger than the pane. Bench places it at the pane's top-left. Try the compact layout. |
| In full screen | The window is in native full screen. Bench leaves it alone. |
| On another desktop | The window is on another desktop (Space). |
| The terminal couldn't start: … | Bench couldn't start the terminal's shell or command. For example, a terminal with a `command` doesn't run it when its folder doesn't exist. |

The sub-tab of an item that can't be shown has a marker on its monogram,
whose shape says what kind of message it has (see
[What the markers mean](#what-the-markers-mean)). To read the message,
right-click the sub-tab, or click **⋯** while it's shown: the menu's first
line says why. Holding the pointer over a sub-tab shows its whole title,
the message, and the other tabs it's in. When the window a pane shows needs
more room than the pane has, the pane's header also says **Needs more
room**. The status bar counts the current tab's windows that can't be
shown: see [The status bar](#the-status-bar).

When an app moves or resizes its own docked window, as some apps do just
after Bench sizes them, Bench puts the window back. If the app keeps doing
it, Bench stops after three tries in ten seconds and leaves the window where
the app put it, with no message, until the tab is next arranged. See
[A window isn't in its pane](#a-window-isnt-in-its-pane).

### What the markers mean

| Marker | Where | What it means |
| --- | --- | --- |
| Amber warning sign (⚠) | A sub-tab's monogram | The window needs you: its app isn't open or couldn't open, you moved it out of its pane, or it can't shrink to fit. |
| Grey ring (○) | A sub-tab's monogram | Bench is opening the app and waiting for its window. |
| Grey circle with an i (ⓘ) | A sub-tab's monogram | For information: the window is in full screen, or on another desktop. |
| Two overlapping windows | The end of a sub-tab | The window is in other tabs too. See [Show a window in more than one tab](#show-a-window-in-more-than-one-tab). |
| **Needs more room** | A pane's header | The window it shows can't shrink to fit the pane. |
| **Not secure** or **Other site** | A web page's pane header | See [Where a web page is](#where-a-web-page-is). |
| Orange underline | A tab | The current tab. |
| Orange border | A pane | The focused pane: what it shows gets the keyboard. |
| Raised background with an outline | A sub-tab, or the layout control | The window that its pane shows, or the layout you chose. |
| ⚠ ***N* windows need attention** | The status bar, or the title bar as ***N* need attention** while the status bar is hidden | Windows in the current tab that need you. Click it to list them. |
| ⚠ A message | The status bar, or the title bar as ***N* notices** while the status bar is hidden | A problem with Bench itself, such as with `config.toml`, a shortcut, or a permission. Click it to list them all. |

### When an app quits

When you close a window, its sub-tab goes. When an app quits, Bench keeps
the places of its windows instead, so that they go back where they were
when the app opens again:

- The app's sub-tabs are hidden, but each keeps its place in its pane, in
  every tab it's in. When the app's window comes back, its sub-tab shows
  there again, and the pane shows it if it did before. The places that
  `config.toml` gives an app with no window yet are hidden in the same way.
- A pane stays even when all its sub-tabs are hidden. If it has nothing to
  show, it lists each app it's waiting for, saying why there's no window,
  such as "Brave isn't running", with these buttons:
  - **Open** *app* opens the app, or asks it for a new window if it's open,
    and puts the window in this pane. There's no **Open** button for an app
    that isn't installed.
  - **Remove from this pane** forgets the app's places in this pane. A
    window that's in other tabs too stays in them. If that leaves the pane
    with nothing, the pane closes and the panes beside it close up.
- A pane that shows other windows lists the apps it's waiting for in its
  **⋯** menu instead, under **Closed windows**: **Open** *app*, and
  **Remove** *app* **from this pane**.
- **Close this pane**, in a pane that shows nothing, forgets what the pane
  is waiting for and closes it, so that the panes beside it close up. It
  makes the tab's layout custom. You can't close a tab's only pane.
- When a tab shows one pane with sub-tabs on a small screen, that pane lists
  what the whole tab is waiting for, and has no **Close this pane** button.
- While Bench opens an app for one of its places, after a tab switch or
  **Open**, the sub-tab shows, saying "Opening *app*…", until the window
  comes or `launch_timeout` passes.
- Hidden sub-tabs aren't listed anywhere else either: the jump palette, the
  add dialog, the preview in Tab properties, and the tab's window count
  leave them out.
- Without Accessibility permission, Bench can't tell which windows are
  open, so it hides nothing. A sub-tab whose app isn't running says so in
  its pane, with **Open** *app* and **Remove from Bench** buttons.

Bench doesn't reopen what you quit. When you quit an app yourself, Bench
remembers it, in `state.json`, and tab switches don't open the app until it
opens again: from the Dock, for example, or with **Open**. These don't
count as quitting an app yourself:

- An app that quits in the 5 minutes after you choose **Log Out**,
  **Restart**, or **Shut Down**. Bench opens it for its tab as usual.
- A copy of an app that quits while another copy runs, as when an app
  updates itself.

In the same way, when a tab switch asks an app that's open for a window,
and none comes within `launch_timeout`, later tab switches don't ask it
again until the app next starts, or you click **Open**. If you switch away
and back before then, Bench waits for the window it asked for rather than
asking again.

When an app opens again, its windows fill its places: first the places
whose window had the same title, then a place that Bench opened the app
for, after a tab switch or **Open**, then the rest in the order the windows
were made in. A little while later, Bench forgets the places that no window
filled, in every tab, so that a window you open after that, for example
with Command+N, goes where [New windows and links](#new-windows-and-links)
says, not to a hidden place. Bench waits until the app has been open for
twice `launch_timeout` (30 seconds by default), until four times
`launch_timeout` (a minute) after Bench started, and until no new window
of the app has come for twice `launch_timeout`. With a `launch_timeout`
under 15 seconds, these times are as long as with 15 seconds. Bench
doesn't forget:

- The places that `config.toml` gives the app: for each of the app's
  entries, one place in the pane where the file puts the entry, or
  elsewhere in the tab if you've moved it, unless a window of the app is
  there for the entry already. So switching to the tab still opens the app.
- A place that Bench is opening the app for right now.
- Anything while the app isn't running, while you've hidden it, or while
  Bench can't see any of its windows, for example because they're on
  another desktop. Bench waits until it sees a window of the app.

Forgetting places doesn't change the layout: a pane stays where it is, even
if that leaves it empty. To close an empty pane, click **Close this pane**.

Bench also forgets, in the same way, the places of an app that it hasn't
seen running for 30 days. Only the time that Bench is running counts.

## The status bar

The status bar, at the bottom of Bench's window, is one line. From the
left, it shows:

- How many windows the current tab has, in how many panes. When a small
  screen shows the tab as one pane with every window as a sub-tab, it says
  **shown as one pane**.
- ***N* windows need attention**, when the current tab has windows with a
  problem (the amber warning sign). Click it to list them, each with why,
  and choose one to show it.
- The layout in use: **Wide** or **Compact**, or **Auto: wide** or **Auto:
  compact** when Auto chose it. Click it, or hold the pointer over it or
  over the layout control in the title bar, to see why. See
  [Small screens](#small-screens-auto-wide-and-compact).
- Problems with Bench itself, such as with `config.toml`, the saved
  arrangement, a shortcut, or a permission. Click the message to list them all. Some have buttons for
  what you can do about them, such as **Reload** for a `config.toml` you
  changed while Bench was running. Tab and the arrow keys move between the
  buttons.
- Reminders of the main keyboard shortcuts, when there's room.

When there isn't room for all of it, the shortcut reminders go first. Then
the message about a problem is cut short, and then the layout.

To give the panes its height, untick **Show status bar** in
[Settings](#settings). Without it:

- The windows that need attention, and the problems with Bench itself, are
  amber buttons in the title bar, beside Jump to…: ***N* need attention**
  and ***N* notices**. Each opens the same list as in the status bar.
- Messages, such as **Saved settings**, show briefly in the middle of the
  title bar's empty space, or over the tabs when that's too narrow, for 4
  seconds at most. Clicks go through a message: the title bar still moves
  the window under it, and the tabs still switch. A message with a button,
  such as **Undo**, shows for 10 seconds, and its button takes your click.
- Why the layout is in use is in the layout control's tooltip.

## Move windows

There are three ways to move a window. They all do the same thing.

**Drag its sub-tab:**

| Drop it on | What happens |
| --- | --- |
| The middle of a pane | The window joins that pane, and shows. |
| The outer quarter of a pane, on any side | The pane splits in that direction, and the window goes into the new half. |
| A tab in the title bar | The window moves to that tab's focused pane. With Option held, it stays here too: see [Show a window in more than one tab](#show-a-window-in-more-than-one-tab). |

While you drag, Bench highlights where the window lands: half a pane for
an edge, the whole pane for the middle, or a tinted tab. To reach a tab
that's out of view, hold the sub-tab near either end of the tab strip,
which scrolls. To cancel a drag, press Escape.

**Use the pane's buttons.** **Show side by side** splits the pane to the
right with the shown window. The **⋯** menu has the following. Right-click
a sub-tab for its window's menu, or an empty part of a pane's header for
the shown window's: the menu opens where you click.

- **Show side by side (split right)** and **Split down**.
- **Move to pane**, listing each other pane by its shown window.
- **Move to tab**, listing each other tab.
- **Also show in** *tab*, for each tab that the window isn't in yet.
- **Remove from this tab**, for a window that's in other tabs too.
- **Dock again**, for a window you dragged out of its pane.
- **Open in browser**, for a web page: see
  [Where a web page is](#where-a-web-page-is).
- **Open** *app* and **Remove** *app* **from this pane**, under **Closed
  windows**, for each app whose windows the pane keeps places for: see
  [When an app quits](#when-an-app-quits).
- **Pop out of Bench**, or **Remove from Bench** for an app's sub-tab that
  has no window.
- **Close window**, **Close page**, or **Close terminal**, or **Close
  everywhere** for a window that's in other tabs too. See
  [Close windows and tabs](#close-windows-and-tabs).
- For a web page, **Use the** *name* **web app from** *browser*, or **Make a
  web app in** *browser*…, which put a web app of the page's site in its
  place. See [Switch a web page to a web app](#switch-a-web-page-to-a-web-app).

**Use the keyboard.** Control+Option+Shift+arrow moves the focused pane's
shown window into the neighbouring pane in that direction, or splits the pane
at that edge if there's no neighbour.

Things to know:

- A pane can't be split by the only window it shows. Bench explains why
  instead of doing nothing: add another window first, or drag one in.
- Moving a window makes the tab's layout custom. Tab properties says so,
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
  terminal's session ends. If the terminal is running something, Bench
  asks first, in the terminal's window: see
  [When Bench quits](#when-bench-quits). A web page or terminal that still
  has its own window when Bench quits gets it again when Bench next opens.
- **Remove from Bench** takes an app's sub-tab that has no window out of
  every tab, for example one that's waiting while Bench opens its app.
  Bench stops opening the app for it. The sub-tab of an app that has quit
  is hidden: to forget it, use **Remove from this pane** (see
  [When an app quits](#when-an-app-quits)). If `config.toml` lists the app
  in the tab, it stays listed until you next save the tab in
  [Tab properties](#tab-properties), and **Reset layout** brings it back
  until then.

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

To take the window out of one tab, switch to that tab, and click the **×**
on its sub-tab, or choose **Remove from this tab** in the window's **⋯**
menu. The window stays in its other tabs.

Things to know:

- A window can be in each tab once.
- Dragging the window to another pane, or splitting a pane with it, moves it
  only in the tab you're in. **Move to tab** takes it out of the tab you're
  in. If it's in that tab already, it stays where it is there.
- Closing the window, with **Close everywhere** in its **⋯** menu or in its
  app, or choosing **Pop out of Bench**, takes it out of every tab.
- Deleting or closing a tab doesn't change a window that's also in another
  tab.
- When you click a window that's in the current tab and in other tabs, for
  example in the Dock, Bench shows it in the current tab rather than
  switching tabs.
- Web pages and terminals work the same way.
- Bench remembers which tabs a window is in, in `state.json`. The config
  file can't list one window in several tabs.

## Close windows and tabs

To close a window, a web page, or a terminal, do one of these:

- Click the **×** at the right end of its sub-tab. The shown sub-tab has
  one, and the others show one when you hold the pointer over them.
- Middle-click its sub-tab.
- Choose **Close window**, **Close page**, or **Close terminal** in its
  **⋯** menu, or right-click its sub-tab.
- Choose **View ▸ Close Window**, or **Close window** in the
  [jump palette](#the-jump-palette), for the focused pane's shown window.

Bench closes an App window as its red close button does, so the app can
ask about unsaved changes as usual. A web page closes at once, and so does
a terminal, unless it's running something: then Bench asks first, as it
does before quitting (see [When Bench quits](#when-bench-quits)).

The **×** and a middle-click don't close a window that's in other tabs
too: they take it out of this tab only, as **Remove from this tab** does,
and the **×**'s tooltip says so. To close it, choose **Close everywhere** in
its **⋯** menu. On a sub-tab with no window, which only keeps the place of
an app's window, the **×** removes it, as **Remove from Bench** does.

To close a tab and everything in it, do one of these:

- Click the **×** at the right end of the tab, in its shortcut's place, or
  where the shortcut would be on a tab that doesn't show one. The current
  tab has one, and the others show one when you hold the pointer over them.
- Middle-click the tab.
- Choose **Close tab…** in the tab's right-click menu.
- Choose **View ▸ Close Tab…**, or **Close tab…** in the jump palette, for
  the current tab.

Bench closes the tab's windows, web pages, and terminals, other than those
that are in other tabs too, which stay there. It deletes the tab from
`config.toml`, with any [rules](#rules) that send windows to it, so the tab
doesn't come back when Bench next opens. Before it closes two or more
things, or a terminal that's running something, Bench asks, and says what
it will close:

> This closes 2 windows and 1 terminal. Apps can still ask about unsaved
> changes. The tab goes from config.toml, so it won't come back when Bench
> next opens. Undo brings back the tab, but not what closes.

**Close tab** closes, and **Cancel**, Escape, or a click outside the
question keeps everything. Tick **Don't ask again** to stop Bench asking;
**Ask before closing a tab's windows** in [Settings](#settings) turns it
back on. With it off, Bench still asks before it ends a terminal that's
running something, as long as **Ask before quitting while a terminal is
running something** is on; that question's **Don't ask again** turns that
setting off instead.

Things to know:

- On the only tab, **Close tab…** is **Close all windows…**. It closes the
  tab's windows, forgets the places of apps that have quit, and leaves the
  tab, empty. What `config.toml` lists for the tab doesn't open again when
  Bench next opens; **Reset layout** brings it back.
- The message after closing a tab has **Undo**, which brings back the tab
  with what's still open, such as a window whose app is asking about
  unsaved changes, but not the windows, pages, and terminals that closed.
  **Reopen closed tab** brings back the tab later, with its pages and
  terminals: see [Undo, and reopen a closed tab](#undo-and-reopen-a-closed-tab).
- To delete a tab without closing its windows, choose **Delete tab (keep
  windows)** in its right-click menu, or **Delete tab** in its
  [Tab properties](#tab-properties). Its windows move to **Not in a tab**.
- An app can keep its window open, for example to ask whether to save your
  changes. About 1.5 seconds later, Bench shows the window, and a toast says
  so, such as "TextEdit may be asking about unsaved changes." A window from
  a tab you closed is then under **Not in a tab** in the jump palette; any
  other stays in its pane.
- Some apps, such as Slack and Music, hide their window when you close it,
  and keep running. Bench counts the window as closed once it has gone from
  the screen. Open it again from the app, or its icon in the Dock. If Bench
  had moved the window out of sight, as it does with other tabs' windows,
  it puts it back on screen when it quits.
- Bench doesn't keep the places of windows you close, as it does when an
  app quits, even when the app quits with its last window.
- Closing an App window needs Accessibility permission. Without it, **Close
  window** is turned off, and the menu says why.
- If an app's window has no close button, Bench chooses the app's **Close
  Window** or **Close** menu command instead. Bench can't close a window
  that's on another desktop, and says so.
- A web page closes without asking, even one that would ask before you
  leave it, such as a form with unsent text.
- Bench has no keyboard shortcut for closing. In an App window,
  Command+W closes the window as usual, and its sub-tab goes.

## Undo, and reopen a closed tab

After you change the arrangement, a message at the bottom of Bench's
window says what changed, and for some changes it has **Undo**:

- Deleting a tab, and closing one.
- **Reset layout**.
- **Close this pane**.
- Moving a window, by dragging it or from its menu.
- **Also show in**, and **Remove from this tab**.

The message stays for 10 seconds. Click **Undo** to put things back as they
were before that change. Things to know:

- Undo puts back the tabs, their panes and sizes, and which windows are in
  them. A deleted tab comes back where it was, with its entry in
  `config.toml` and the [rules](#rules) that send windows to it, and its
  windows dock in it again.
- Undo after closing a tab brings back the tab and what's still open, not
  the windows, pages, and terminals that closed. If an app was asking about
  unsaved changes, it still is: answer it in the app.
- Undo also takes back the changes you made after that one. Something that
  closed since stays closed, and a window that came since stays where it
  is.
- Bench can undo only its last 10 changes, and none from before you added,
  renamed, or reopened a tab, or reloaded `config.toml`. Then it says it
  can't undo that any more.
- There's no keyboard shortcut. Command+Z stays with the page or app you're
  in.

Bench keeps the last 5 tabs you deleted or closed, even after it quits. To
bring one back, choose **Reopen closed tab** from any tab's right-click
menu, for the tab you deleted or closed last, or type "reopen" in the
[jump palette](#the-jump-palette), which lists them all. The tab comes back
where it was, with its entry in `config.toml`, its rules, and its layout,
and Bench switches to it:

- Its windows under **Not in a tab** dock in it again. A window that's in
  another tab now stays there.
- Its web pages and terminals open again. Terminals start afresh.
- A window that closed keeps its place, and goes back there when you open
  it again, as when an app quits.
- If another tab has its name now, it comes back numbered, such as
  "Code (1)".

## Add to a pane

Click a pane's **+** to open the add dialog. Type to search, then choose a
row:

| Group | What it adds |
| --- | --- |
| **Windows not in a tab** | An open window that Bench isn't managing. |
| **Windows in other tabs** | A window, web page, or terminal from another tab, to show in this pane as well. It stays in its other tabs. |
| **New window** | A new window of an app that already has a sub-tab in this tab. |
| **Built in** | **New terminal**, or **Web page…**, which asks for a website's `https` address (Bench adds `https://` if you leave it out) and adds the site as a web app where it can, or else as a web page that Bench shows itself (see [Web apps](#web-apps)). |
| **Apps** | Any installed app other than a web app, from `/Applications`, `~/Applications`, and `/System/Applications`, including their `Utilities` folders. Bench opens the app if it isn't running, and adds its windows that aren't in a tab, or opens a new window if it has none. An app that Bench opens can take a while to open its windows, and some of them can belong to other sub-tabs: Bench opens a new window only if none of the app's windows comes to the pane, or if it opens none within `launch_timeout`. |
| **Web apps** | Websites that a browser has made into apps of their own (see [Web apps](#web-apps)), with the browser that made each. They work like any other app. |

## Web apps

A web app is a website that your browser has made into an app of its own,
with its own icon in the Dock and its own windows. Bench docks it like any
other app, as an App item, so you can put Gmail, Slack, or Notion in a pane
without a whole browser window. The browser runs the site, with its
protections, saved passwords, and extensions, and Google sign-in works,
which it doesn't in a Web item. A web app from a Chromium browser shares
the browser's sign-ins. A Safari web app keeps its own, starting with the
ones Safari had for the site when you made it.

Because a web app has the browser's protections and sign-ins, Bench adds
websites as web apps where it can. Click a pane's **+**, choose **Web
page…**, type the site's address, such as `mail.google.com`, and press
Return:

- **If you have a web app of the site**, Bench adds it to the pane. A web
  app is the site's if it opens a page on the same host: one of
  `mail.google.com` isn't one of `docs.google.com`. If you have several,
  Bench chooses your default browser's, then Safari's.
- **If you don't, and your default browser can make one** (Safari, Chrome,
  Edge, Brave, Vivaldi, Chromium, or Orion), Bench opens the site there,
  and the dialog shows the steps. Follow them, and Bench adds the web app
  to the pane as soon as it's ready. It does so even if the web app opens a
  page on another host, as long as it's the only web app made since Bench
  opened the site. While the dialog waits, Bench leaves the browser on
  screen and keeps the browser's new windows out of the tab. To stop
  waiting, click **Cancel**, or **Show in Bench instead**.
- **Otherwise**, Bench shows the page itself, in a Web item (see
  [Items](#items)), and says why: it needs Accessibility permission to dock
  web apps, or your default browser can't make them, as Firefox, Arc, and
  Opera can't, or Bench doesn't know its steps, as for other browsers built
  on Chromium. Their web apps are still listed under **Web apps**.

To show the page in Bench instead, press Option-Return, or click **Show in
Bench**. A Web item shows the page in Bench's own window, without an address
bar, and Google sign-in doesn't work there (see
[Google sign-in doesn't work in a web item](#google-sign-in-doesnt-work-in-a-web-item)).

To make a web app yourself, open the site in your browser, then:

| Browser | How |
| --- | --- |
| **Safari** (macOS 14 or later) | Choose **File ▸ Add to Dock**, then click **Add**. |
| **Chrome** | Open the **⋮** menu and choose **Cast, save, and share ▸ Install page as app**, or, for a site that's made to be installed, **Install** and the site's name, such as **Install Gmail**. |
| **Edge** | Open the **…** menu and choose **More tools ▸ Apps ▸ Install this site as an app**. |
| **Brave**, **Vivaldi**, and other browsers built on Chromium | Choose the browser's command to install the page as an app, in its menu or its address bar. |
| **Orion** | Choose **Tools ▸ Install This Site as an App**. |

Firefox, Opera, Arc, DuckDuckGo, and Zen can't make web apps on macOS. For
those, use an App item for the browser, divided with `title` (see
[Items](#items)), or a Web item. If your default browser is an app that asks
which browser to open each link in, Bench can't tell which one will, so it
shows pages itself.

Your web apps are listed in the add dialog too, under **Web apps**, with the
browser that made each. If a Chromium browser's web app asks how to open,
choose a window, not a tab: a web app that opens in a browser tab is part of
the browser's window, and Bench can't tell it apart.

Bench finds web apps in `~/Applications`, where Safari keeps them, in the
folders there that Chromium browsers keep them in, such as
`Chrome Apps.localized`, and in `~/Applications/Orion/WebApps`. Bench can't
tell which site an Orion web app opens, so **Web page…** doesn't find the
ones you have already: choose them from **Web apps**. It does add one that
you make while the dialog waits. To list a web app in `config.toml`, use
its bundle ID: see [Items](#items).

### Switch a web page to a web app

A web page that you have already, such as Slack and Outlook in the
[example config](#example-config)'s Comms tab, can become a web app. Open the page's **⋯** menu, or right-click
its sub-tab. A moment after the menu opens, it has one of these at the end:

- **Use the** *name* **web app from** *browser*, such as **Use the Slack
  web app from Chrome**, if you have a web app of the page's site. Bench
  finds it as **Web page…** does, by the address the page was given.
- **Make a web app in** *browser*…, if you don't, and your default browser
  can make one. Bench opens the page's site in the browser, and shows the
  browser's steps. Follow them, and Bench uses the web app as soon as it's
  ready, finding it as the add dialog does. While it waits, Bench leaves the
  browser on screen and keeps the browser's new windows out of the tab. To
  stop waiting and keep the page, click **Cancel**, or press Escape.

Bench closes the page and puts the web app in its place: in the same pane,
at the same place among the sub-tabs, shown if the page was, and in every
tab that shows the page. The web app's window joins it: one that's open and
not in a tab, or else a new one, opening the web app if it isn't open.

If the page is in `config.toml`, Bench saves the file with the web app's
bundle ID in place of the page's entry, keeping the previous version as
`config.toml.bak`, so that the page doesn't come back when Bench opens
again or when you reset the tab's layout. As when Settings or Tab
properties saves the file, your comments in it aren't kept.

The menu has neither entry for VS Code for the Web, or if you have no web
app of the site and Bench can't have your browser make one: because the
browser can't, or because Bench doesn't have Accessibility permission,
which it needs to dock web apps.

## The jump palette

Press Control+Option+Space, or click **Jump to…** in the title bar. Type to
filter by name, then use the Up and Down arrows and press Return. Press
Escape, click outside the palette, or press Control+Option+Space again, to
close it. If you were typing in a window docked in Bench when you opened the
palette with its shortcut, that window gets the keyboard back, as a web page
or terminal does. A window of another app, or another of the same app's
windows, doesn't: click it to carry on typing.

| Group | What's in it |
| --- | --- |
| **Tabs** | Every tab, with its apps and shortcut. |
| **Windows** | Every window in every tab, by title, with its tab and app. A window in several tabs is listed once for each. Choosing one switches to its tab, focuses its pane, and shows it. |
| **Not in a tab** | Windows that Bench isn't managing. Choosing one adds it to the current tab's focused pane. |
| **Actions** | **Rearrange this tab**, **Add to this pane…**, **Show side by side**, and **Window options…** for the focused pane (see [More room for your apps](#more-room-for-your-apps)), **Fill the screen** (see [Fill the screen](#fill-the-screen)), **Tab properties of** the current tab, **New tab…**, **Close window** (or **Close page**, **Close terminal**, or **Remove from this tab**) for the focused pane's shown item, and **Close tab…** (**Close all windows…** on the only tab) for the current tab (see [Close windows and tabs](#close-windows-and-tabs)), **Reopen closed tab**, for each tab Bench can bring back (see [Undo, and reopen a closed tab](#undo-and-reopen-a-closed-tab)), **Settings…**, **Reload config.toml** (see [Edit the file while Bench runs](#edit-the-file-while-bench-runs)), and switching between Auto, Wide, and Compact. |

## Tab properties

A tab's properties are its own: its name, its layout, and what it does on
small screens. To change them, open the tab's **Tab properties** in one of
these ways:

- Right-click the tab, or Control-click it, and choose **Tab properties…**,
  or **Rename…**, which selects the name for you to type over. This works
  for any tab, not only the current one.
- Choose **View ▸ Tab Properties…**, for the current tab.
- Choose **Tab properties of** the current tab in the
  [jump palette](#the-jump-palette).

To add a tab, click **+** after the tabs, or choose **New tab…** from a
tab's right-click menu or the jump palette. **New tab** opens, with the same
fields.

| Field | What it does |
| --- | --- |
| **Name** | The tab's name. Names must be unique, ignoring case, so a name that another tab has is numbered: a second **Code** becomes **Code (1)**, then **Code (2)**. A number in brackets at the end is replaced, not added to. Before you save, a line under the field says what the tab will be called. Renaming works the same way. A new tab starts with a name that's free, such as **New tab (1)**. |
| **Shortcut** | The tab's shortcut. Shortcuts follow tab order: the first nine tabs get Control+Option+1 to 9, and later tabs have none. |
| **Layout** | One of six layouts, with a preview of which windows land in which pane. |
| **On small screens** | **Stack panes**, **One pane with sub-tabs**, or **Keep the layout**, for screens narrower than the width set in [Settings](#settings). See [Small screens](#small-screens-auto-wide-and-compact). |

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

At the bottom of Tab properties:

- **Delete tab** removes the tab, and any [rules](#rules) that send windows
  to it. Its windows aren't closed: they move to **Not in a tab**. It's in
  the tab's right-click menu too, as **Delete tab (keep windows)**. To
  close the windows as well, use **Close tab…**: see
  [Close windows and tabs](#close-windows-and-tabs). You can't delete the
  last tab. The message that says so has **Undo**, and **Reopen closed
  tab** brings the tab back later: see
  [Undo, and reopen a closed tab](#undo-and-reopen-a-closed-tab).
- **Reset layout** returns the tab to the layout in its config, undoing your
  dragging and splitting. It's in the tab's right-click menu too. The
  message that says so has **Undo**.
- **Cancel** closes Tab properties without changes, and **Save** applies
  them and saves `config.toml`, keeping the previous version as
  `config.toml.bak`. Renaming a tab also renames it in the rules that name
  it. A sub-tab whose app has quit, or isn't installed, stays in the tab, but
  Save writes its app into `config.toml` only if the file already lists that
  app in some tab.

No dialog is taller than Bench's window. On a short window, the middle of
Tab properties scrolls, between the name and the buttons, which stay in
view. You can make it larger, or move it: see
[Move and resize dialogs](#move-and-resize-dialogs).

While `config.toml` has problems, Bench runs with the last config that
worked, and changes in Tab properties and Settings apply but aren't saved
to it: the message after saving says **Applied for now, not saved:
config.toml has problems**. See
[Bench reports problems with config.toml](#bench-reports-problems-with-configtoml).
If you've changed `config.toml` yourself since Bench last read or saved it,
Bench doesn't save over your changes either: the message says **Applied for
now, not saved: config.toml was changed outside Bench**, and the status bar
offers to reload the file or overwrite it. See
[Edit the file while Bench runs](#edit-the-file-while-bench-runs).
If Bench can't write `config.toml`, it says so, and it stops saving the
arrangement too, so that `state.json` still matches the file; saving
Settings or a tab's properties tries again.

## Settings

Settings apply to every tab. To open them, do one of these:

- Click the gear button at the right of the title bar.
- Choose **Bench ▸ Settings…**.
- Choose **Settings…** in the [jump palette](#the-jump-palette).

To change one tab's name, layout, or small-screen setting, use its
[Tab properties](#tab-properties) instead.

| Group | Setting | Config setting |
| --- | --- | --- |
| **General** | **Open Bench at login** | — |
| **General** | **Open Bench filling the screen** | `fill_screen` |
| **General** | **Bring back a hidden Bench with tab shortcuts** | `show_on_tab_shortcut` |
| **General** | **Ask before quitting while a terminal is running something** | `confirm_quit_busy_terminals` |
| **General** | **Ask before closing a tab's windows** | `confirm_close` |
| **General** | **Menu bar item**: **Icon and tab name**, **Icon only**, or **Hidden** | `menu_bar` |
| **Appearance** | **Show status bar** | `show_status_bar` |
| **Appearance** | **Hide the sub-tab row when a pane holds one app window** | `pane_headers` |
| **Appearance** | **Density**: **Automatic**, **Standard**, or **Compact** | `density` |
| **Appearance** | **Space between panes**: **8 pt**, **4 pt**, or **2 pt** | `gap` |
| **Appearance** | **Show shortcuts on tabs**: **Automatic**, **Always**, or **Never** | `tab_shortcut_hints` |
| **Appearance** | **Show scroll bars**: **Automatic**, **Always**, or **Never** | `scroll_bars` |
| **Windows** | **Open apps that aren't running** | `open_missing_apps` |
| **Windows** | **Hide windows that belong to other tabs** | `hide_other_tabs` |
| **Windows** | **Switch tabs when another tab's window comes forward** | `follow_focus` |
| **Windows** | **Show live previews of windows (needs Screen Recording)** | `live_previews` |
| **Small screens** | **Small screens are narrower than** *number* **points** | `compact_below` |

Things to know:

- **Open Bench at login** isn't in `config.toml`: macOS keeps it.
- **Open Bench filling the screen** takes effect the next time Bench
  opens. Unticked, Bench opens at its usual size, 1,440 by 900 points, in
  the middle of the display it was last on. See
  [Fill the screen](#fill-the-screen).
- **Bring back a hidden Bench with tab shortcuts**, on by default, has the tab
  shortcuts show Bench on the tab you choose. See
  [When you hide Bench](#when-you-hide-bench).
- **Ask before quitting while a terminal is running something**, on by
  default, has Bench ask before it quits, or closes a terminal, from its
  sub-tab, its own window, or **Close tab…**, while a terminal is running a
  program, such as a build or `claude`. The questions' **Don't ask again** turns it off;
  tick it here to have Bench ask again. See
  [When Bench quits](#when-bench-quits).
- **Ask before closing a tab's windows**, on by default, has **Close tab…**
  ask before it closes two or more windows, web pages, or terminals, or a
  terminal that's running something. The question's **Don't ask again**
  turns it off. With it off, **Close tab…** still asks before it ends a
  terminal that's running something, unless **Ask before quitting while a
  terminal is running something** is off too. See [Close windows and tabs](#close-windows-and-tabs).
- **Menu bar item** chooses what Bench shows in the menu bar.
  **Icon and tab name** shows Bench's icon and up to 16 characters of the
  current tab's name, which is cut short with an ellipsis. **Icon only**
  saves room, on a small screen or beside the notch. **Hidden** removes the
  item: Settings then reminds you that the jump palette's shortcut and
  Bench's icon in the Dock still bring Bench back.
- Unticking **Show status bar** gives its height to the panes, and moves
  its warnings and messages to the title bar. See
  [The status bar](#the-status-bar).
- **Hide the sub-tab row when a pane holds one app window** is off unless
  you turn it on. See [More room for your apps](#more-room-for-your-apps).
- **Density** is **Automatic** unless you change it: compact in the compact
  layout, and standard otherwise. **Standard** and **Compact** use one
  density everywhere. The panes change at once, and the title bar the next
  time Bench opens, as Settings says. See
  [More room for your apps](#more-room-for-your-apps).
- **Space between panes** is 4 points unless you change it. Less space
  leaves more room for your apps, and narrower dividers to drag. If the
  config file sets another space, Settings offers it too.
- **Show shortcuts on tabs** is **Automatic** unless you change it: tabs
  show their shortcuts, such as ⌃⌥1, except on small screens. **Always**
  shows them there too, and **Never** shows only the tabs' names. A tab's
  tooltip has its shortcut either way.
- **Show scroll bars** is **Automatic** unless you change it: the tab and
  sub-tab strips show scroll bars when macOS shows them all the time, as
  set in System Settings ▸ Appearance ▸ Show scroll bars, and as it does
  with a mouse and no trackpad. When macOS shows them only while scrolling,
  Bench hides them, and the strips' faded ends show that there's more.
  **Always** shows a thin bar below each strip that doesn't fit, and
  **Never** hides them. See
  [When tabs and sub-tabs don't fit](#when-tabs-and-sub-tabs-dont-fit).
- Unticking **Switch tabs when another tab's window comes forward** sets
  `follow_focus` to `off`, unless the config file sets it to `bring-here`,
  which stays.
- Turning on **Show live previews of windows** asks macOS for Screen
  Recording permission. Until Bench has it, Settings says how to give it.
  See [Live previews](#live-previews).
- The rest of the `[settings]` table, such as `new_windows` and
  `launch_timeout`, is only in the [config file](#config-reference).

**Cancel** closes Settings without changes, as do Escape and clicking
outside it. **Save** applies the settings and saves `config.toml`, keeping
the previous version as `config.toml.bak`. If Bench can't apply them, it
says why, and Settings stays open. If it can't change **Open Bench at
login**, it says why, and the other settings are still saved.

## Move and resize dialogs

Tab properties, Settings, the add dialog, the jump palette, and the
welcome guide open in the middle of Bench's window. You can move and
resize each of them inside the window:

- To move a dialog, drag its header: the row with its title, or with its
  search field, but not the field or a button in it. Over the header, the
  pointer shows that it moves the dialog. The jump palette and the add
  dialog move by the row of keys at their foot too.
- To resize a dialog, drag the grip in its bottom-right corner. A dialog is
  at least 420 points wide and 300 points tall, or 200 for the jump palette
  and the add dialog, and no larger than the window. The layout preview in
  Tab properties and the lists in the jump palette and the add dialog grow
  with it.
- To put a dialog back in the middle, double-click its header. It keeps
  its size.

Bench remembers the size you give each dialog, after it quits too, and
opens the dialog at that size next time. While Bench runs, a dialog you
moved opens where you left it. Moving a dialog doesn't change its size: a
dialog you haven't resized stays as tall as what it shows, such as the
jump palette's list as you type.

A dialog always stays inside the window, and clear of the red, yellow, and
green buttons at its top left. When the window gets smaller, the dialog
moves back inside it, and gets smaller too if it must. When the window
gets larger again, the dialog goes back where you left it, at the size you
gave it. No dialog is taller than the window: on a short window, or when
you move a dialog near the window's bottom edge, its middle scrolls, and
its title and buttons stay in view.

While a dialog is open, Bench comes in front of the apps in its panes, so
moving a dialog doesn't show the app behind it. Menus, such as a tab's
right-click menu, don't move.

## Small screens: Auto, Wide, and Compact

Bench chooses a wide or a compact layout for the screen its window is on.
The control at the right of the title bar sets how:

| Choice | What it does |
| --- | --- |
| **Auto** | Compact on screens narrower than `compact_below` (1,800 points by default, set in [Settings](#settings)), wide on others. |
| **Wide** | Always the wide layout, until you choose Auto again. |
| **Compact** | Always the compact layout, until you choose Auto again. |

You can also choose them from **View ▸ Auto Layout**, **Wide Layout**, and
**Compact Layout**, where the one in use is ticked.

When Bench's window is narrower than about 1,200 points, or in the compact
layout, the title bar makes more room for the tabs. The control becomes
one button that names the choice, such as **Auto ▾**: click it for a menu
of the three, with the one in use ticked, and why the layout is in use.
Jump to… becomes its magnifying glass, whose tooltip has its shortcut.

The status bar names the layout in use, such as **Auto: wide**. Click it,
or hold the pointer over it or over the control, to see why, such as "Wide
layout, chosen for a 2,560-pt screen". When a display is connected or
disconnected, or Bench's window moves to another screen, Bench waits for
macOS to settle, then chooses again and rearranges the current tab.

In the compact layout, each tab follows its small-screen setting, which you
choose in its [Tab properties](#tab-properties):

| Setting | In Tab properties | What it does |
| --- | --- | --- |
| `stack` | **Stack panes** | The top-level row becomes a column, so side by side becomes top and bottom. |
| `tabs` | **One pane with sub-tabs** | The tab shows one pane, with every window as a sub-tab, in the title bar (see [More room for your apps](#more-room-for-your-apps)). The layout comes back on a larger screen. |
| `same` | **Keep the layout** | No change. |

In the [example config](#example-config), Code stacks its panes, and Comms
and Research show one pane with every window as a sub-tab.

## Live previews

With live previews on, hovering over an inactive sub-tab for a moment shows
a small, live picture of its window, with its title. In the jump palette,
the window you select or hover over shows its preview beside the palette.

Previews are off until you turn on **Show live previews of windows** in
[Settings](#settings), because they need Screen Recording permission. macOS
asks for it when you turn them on. macOS asks you to approve Screen
Recording again from time to time.

Chromium-based apps, such as Brave, VS Code, Slack, and Obsidian, can stop
drawing windows that are covered, so their previews can show a frozen
picture.

## New windows and links

When an app opens a new window, Bench places it in the first case that
matches:

1. A [rule](#rules) with `float = true` matches: Bench leaves the window
   floating.
2. Bench is waiting for the window. The window goes to the first of these:
   - The pane where you chose **New window** in the add dialog, or the
     place where you clicked **Open** for an app that's open.
   - A place that Bench keeps from before the app quit, whose window had
     the same title.
   - A place that Bench opened the app for, after a tab switch or **Open**.
   - While the app is opening again, a place that Bench keeps from before
     it quit, in the order the windows were made in. See
     [When an app quits](#when-an-app-quits).
   - The pane of the add dialog that opened the app.
3. A rule with `tab = "…"` matches: the window joins that tab, in the pane
   that holds the app's most recently used window, or else that tab's focused
   pane.
4. The add dialog's **Web page…**, or a web page's **Make a web app in**
   *browser*…, is waiting for your default browser to make a web app, and
   the window is the browser's: it stays floating, under **Windows not in a
   tab** in the add dialog (see [Web apps](#web-apps)).
5. Otherwise, `new_windows` decides. By default, the window joins the pane
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

A Web item has no address bar to type in, and its pane's header shows only
the page's host (see [Where a web page is](#where-a-web-page-is)), so Bench
keeps other sites out of it:

- A link to another site that you click opens in your default browser. So
  does the page that a link you click redirects to, however long the
  redirect takes, and the page that a page on the item's site sends you on
  to just after you click through to it, as link trackers do. Pages on the
  item's own site, and the sign-in pages of well-known identity providers,
  stay in the item.
- The item's site is the part of its address that a person or company
  registered, such as `slack.com`, as browsers count it. For some services
  where each customer has a name of their own, that name is the site, so
  another customer's pages are on another site: GitHub Pages, such as
  `alice.github.io`, Jira and Confluence, such as
  `mycompany.atlassian.net`, Zendesk, and ServiceNow, among others. Not for
  services whose apps move between their customers' names, such as Slack,
  Zoom, and Salesforce: there, another customer's pages, such as another
  Slack workspace's, count as the item's site. Pages that anyone can make
  on an app's site are on another site too: Microsoft Forms and Sway, and
  Google Forms, Sites, Apps Script web apps, and documents published to the
  web.
- Without a click, only the item's own page moves the item to another site,
  and so do pages that the item goes to by itself, such as your
  organisation's sign-in pages. Content from another site that's embedded
  in the page, such as an advert or a video, and windows that the page
  opens, can't: the site they send the item to opens in your browser, but
  only if you've just clicked or pressed a key in the page, and otherwise
  nowhere.
- Anyone can have a sign-in page with some identity providers, so after you
  click a link to one, or go back to one, a page it sends you on to that's
  neither the item's site nor another sign-in page opens in your browser,
  once, and the item goes back to its own address. Until the item's page is
  back, the sign-in page can go nowhere unless you click, even if the
  item's page fails to load.
- Each click or key press in a page lets it open one page in your browser,
  or one link for another app, however many it tries to open.
- A link for another app, such as a Zoom meeting, works only when you click
  it, not when the page follows it by itself. Email addresses and phone
  numbers open in their app at once. For other apps, Bench asks first,
  naming the app, while the page is on screen. **Cancel** is the default
  button, and **Open** works after a moment. Tick **Don't allow more from
  this page** to stop the page asking.
- Downloads go to your Downloads folder. A page can start up to 5
  downloads in the minute after each click or key press in it, and none by
  itself.

### Where a web page is

A pane that shows a web page has the page's host in its header, such as
app.slack.com, in place of the **Web** badge. The site's own name, such as
slack.com, is brighter. When there isn't room for the whole host, Bench
leaves out its start, never the site's name; hold the pointer over it to see
the whole host. In a narrow pane, a warning shows just its ⚠ icon, and then
the host goes, until you widen the pane. It names the page you see, not one
that's still loading. The header shows the **Web** badge until the item's
first page has loaded, after Bench restarts too, and for VS Code for the
Web.

A warning, with a ⚠ icon, goes in front of the host when you should take
care:

| Header | What it means | What to do |
| --- | --- | --- |
| **Not secure:** *host* | The page isn't `https`, or it has loaded something that isn't, so others on your network could read or change what you see and send. | Don't enter a password or other private details. |
| **Other site:** *host* | The page has moved to another site than the item's own, as a redirect, a script on the page, or a sign-in page can take it. Anyone can have a sign-in page with some identity providers. | Check that it's the site you expect before you type a password. To see the whole address, choose **Open in browser**. |

A page that's both says **Not secure**.

A page that a site makes itself, with a `blob:` address, has the host of
the site that made it, as browsers show it. Its address isn't `https`, so
it usually says **Not secure**.

A page with no web address, such as a blank page or a `data:` page that the
item has moved to, could show anything, from anyone. So its header has a
warning too, with **page with no address** in place of a host: **Other
site: page with no address**, or, since such a page isn't `https`, usually
**Not secure: page with no address**. Treat it as a page on a site you
don't know.

**Open in browser**, in the pane's **⋯** menu, opens the page in your
default browser, with its whole address, and your browser's own sign-ins
and passwords. For a `blob:` page, it opens the site that made the page,
and for a page with no web address, the item's own address, as it does
before the item's first page has loaded. VS Code for the Web doesn't have
it.

A popped-out web page's window, and a window that a page opens to sign you
in, show the page's host under their title, or **Page with no address**,
after **Not secure —** if the page isn't secure. A page can open a window
with no address, which it then fills with what it likes, only from the
item's own site in the item's own pane, not from a window it opened or a
site embedded in it.

### Alerts and camera requests

A web page can show an alert, or ask you to confirm something. Bench shows
it only while you can see the page, with Bench active, and its heading
names the site that's asking, such as **app.slack.com says**. Content from
another site that's embedded in the page, such as an advert, is named as
embedded, such as **ads.example, embedded in app.slack.com, says**, so that
it can't pass for the page. Embedded content with no site of its own, such
as a `data:` frame, can't show alerts at all. Bench shows up to 2,000
characters of a message, with a few accents or other marks on each letter
at most.

When a page shows another alert within 5 seconds of the last one, you can
tick **Don't allow more dialogs from this page**. The page's alerts then
get their answers without being shown, **OK** for an alert and **Cancel**
for a confirmation, even when the page reloads or goes to another of its
pages. They show again once you click a link to another site, or Bench
loads the item's address again. A page that shows three alerts within 10
seconds gets its answers to more in the same way while it keeps asking. Its
alerts show again once 10 seconds pass without it asking for one, you click
a link to another page, or Bench loads the item's address again. Camera
requests are counted apart from alerts, so a few alerts never stop a video
call, and three camera requests within 10 seconds hold back more in the
same way.

A page can ask to use your camera or microphone, as for a video call.
Bench asks you first, naming the item and the site that's asking, while you
can see the page. **Don't Allow** is the default button, and **Allow** works
after a moment. Once the page has gone, Bench doesn't remember your answer,
so the page asks again next time. Tick **Don't allow more requests from
this page** to stop it asking until you click a link to another site, or
Bench loads the item's address again. Content from another site that's
embedded in the page, such as an advert, can't use them: Bench refuses
without asking. The first time, macOS also asks whether Bench may use the
camera or microphone.

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

While Bench is hidden, the tab shortcuts and the jump palette's bring it
back. See [When you hide Bench](#when-you-hide-bench).

In Bench's menus, dialogs, and palette, Escape closes, the arrow keys move
the selection, and Return chooses. When the add dialog asks for a web
address, Option-Return shows the page in Bench, rather than as a web app.

## Fill the screen

Bench opens filling the screen: its window covers everything below the
menu bar and beside the Dock. It opens on the display it was on last time,
if that display is connected, and otherwise on the main display. Bench
doesn't use native full screen, because other apps' windows can't follow
it into a full-screen space.

To fill the screen again after making the window smaller, choose **View ▸
Fill the Screen**, or **Fill the screen** in the
[jump palette](#the-jump-palette). If the window fills the screen already,
it stays as it is. The green button at the top left zooms the window too,
and so does double-clicking an empty part of the title bar, if that's what
**System Settings ▸ Desktop & Dock** has it do; doing either again gives the
window back the size it had before. However you size it, the window is at
least 900 by 600 points.

When you plug in or unplug a display, or change its resolution, a window
that filled its screen fills the screen it's on afterwards. A window that
you made smaller stays as it is.

To open Bench at its usual size instead, in the middle of the same
display, untick **Open Bench filling the screen** in
[Settings](#settings), or set `fill_screen = false` in the
[config file](#config-reference).

## When you hide Bench

When you hide Bench (**Bench ▸ Hide Bench**, Command+H, or **Hide** in its
Dock menu), minimise it, or close its window, the current tab's apps hide
with it, and so do its web pages.
Closing the window doesn't quit Bench. To bring it back, do one of these:

- Choose **Show Bench**, or a tab, from its menu bar item.
- Press a tab's shortcut, such as Control+Option+2, or the next or previous
  tab's. Bench comes back on that tab.
- Press the jump palette's shortcut, Control+Option+Space.
- Click Bench's icon in the Dock.

The tab's apps come back with it, and its focused pane gets the keyboard.

The other keyboard shortcuts still move between panes and sub-tabs while
Bench is hidden, but windows stay where they are until you bring Bench
back. Then the tab is arranged as you left it.

To have the tab shortcuts switch tabs without bringing Bench back, untick
**Bring back a hidden Bench with tab shortcuts** in [Settings](#settings), or set
`show_on_tab_shortcut = false` in the [config file](#config-reference).
Bench then switches tabs while hidden, and arranges the tab you chose when
you bring it back.

**Hide Others** in another app hides Bench as it hides any app, but leaves
that app's windows where they are, even if Bench docks some of them. A tab
shortcut brings Bench back then too.

## When Bench quits

When you quit Bench, from **Bench ▸ Quit Bench**, its menu bar item, or its
icon in the Dock, and a terminal is running something, such as `claude`,
a build, or a server, Bench asks first, as quitting ends what the terminals
run. It brings its window forward and names the terminals by their
sub-tab titles and tabs:

> 2 terminals are still running: “claude” (Code), “npm run dev” (Web).
> Quit Bench and end them?

**Quit** quits, and **Cancel**, Escape, or a click outside the question
keeps Bench running. A dialog that the question covered, such as Settings,
comes back with your changes. Tick **Don't ask again** before you click
**Quit** to stop Bench asking; **Ask before quitting while a terminal is
running something** in [Settings](#settings) turns it back on. A terminal
that shows only its shell's prompt isn't running anything, and neither is
one whose command has finished.

Closing a popped-out terminal's window while the terminal is running
something asks the same, in that window: **Close** ends the terminal and
closes the window, and **Cancel** keeps it. Closing a terminal's sub-tab
asks too, in Bench's window (see
[Close windows and tabs](#close-windows-and-tabs)).

Bench never asks when you log out, restart, or shut down your Mac, so that
it never holds those up, nor when the terminal you started Bench from
closes, or it's stopped with Control-C or `kill`.

When Bench quits:

- App windows stay where they are, and work as normal windows.
- Windows that Bench moved off screen to hide them come back on screen, where
  they were before, or centred on the screen. Apps that Bench hid are shown
  again.
- Web items and terminals close with Bench. Terminal sessions end, and VS
  Code for the Web stops.
- Bench saves the arrangement. When you open Bench again, it matches windows
  to their sub-tabs: by app and exact title first, then by the order they
  were created in, then by putting the rest in the pane that holds their app.
  Sub-tabs that no window fills stay hidden until their app opens, and are
  forgotten if it doesn't fill them then: see
  [When an app quits](#when-an-app-quits).

If Bench can't put a window back as it quits, for example because the
window is on another desktop, it tries again the next time it opens.

If Bench crashes, a small helper process that Bench starts puts hidden
windows back on screen. If that fails too, Bench puts them back the next time
it opens, or you can run `bench restore`.

## Version and acknowledgements

To see which version of Bench you have, choose **Bench ▸ About Bench**.
`bench version` prints it too: see [Command line](#command-line).

Bench includes open source software. To see what it includes, choose
**Bench ▸ Acknowledgements**. A window lists each one, with its copyright
notices and licence, and those of any code that it took from other
projects. You can select and copy the text.

Choosing **Bench ▸ Acknowledgements** again brings its window forward,
rather than opening another. To close it, press Command+W or click its
close button. On some keyboard layouts, Command+W doesn't close it, and
another shortcut does instead: on French AZERTY, it's Command+Z. On such a
layout, click the close button.

## Bench's files

Bench keeps its files in `~/Library/Application Support/Bench/`, or in the
folder that `$BENCH_HOME` names, which must be one that only you can change:

| File | What it is |
| --- | --- |
| `config.toml` | Tabs, layouts, settings, shortcuts, and rules. Bench creates it on first run, and Settings and Tab properties update it, as does switching a web page in it to a web app. See the [config reference](#config-reference). |
| `config.toml.bak` | The previous `config.toml`, kept by every save. **Use backup** swaps it with a `config.toml` that has problems. |
| `config.toml.good` | A copy of the last `config.toml` that loaded without problems, or that Bench saved. Bench runs with it while `config.toml` has problems. |
| `state.json` | The live arrangement after dragging and splitting, the apps you quit, when Bench last saw each app running, the display that Bench's window was on, and the sizes you gave dialogs. Each tab keeps its arrangement by name: see [Tabs you change in the file](#tabs-you-change-in-the-file). |
| `state.json.bad` | A `state.json` that Bench set aside because it couldn't use it, or a copy of one whose tabs `config.toml` no longer had, when Bench started or reloaded `config.toml`. `state.json.bad.1` and `state.json.bad.2` are the two before. See [When Bench can't keep the arrangement](#when-bench-cant-keep-the-arrangement). |
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
| `bench arrange <tab> [--config path]` | Arranges one tab across the screen without Bench's window, until you press Control-C or close the terminal. It refuses while Bench, or another `bench arrange`, is running, and Bench can't open until it has finished. |
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
Bench creates it on first run, and [Settings](#settings) and
[Tab properties](#tab-properties) update it, keeping the previous version as
`config.toml.bak`. You only need to edit it for things they don't cover,
such as shortcuts, rules, and web pages.

Saving Settings or a tab's properties, deleting a tab, or switching a web
page that the file lists to a web app (see
[Switch a web page to a web app](#switch-a-web-page-to-a-web-app)) writes
Bench's own copy of the file, in the layout of the
[default file](#the-default-file): your comments and the order you wrote
things in aren't kept. `config.toml.bak` keeps the version before the last
save.

### Edit the file while Bench runs

You can edit the file while Bench is running. Bench doesn't save over your
changes: if you've changed the file since Bench last read or saved it, a
save from Settings or Tab properties applies its change but doesn't write
the file, and says **Applied for now, not saved: config.toml was changed
outside Bench**. The status bar then offers:

- **Reload**, to use the file as you left it. Bench's change that wasn't
  saved is dropped.
- **Overwrite**, to write Bench's config over the file. Your version is kept
  as `config.toml.bak`.

Until you choose, Bench keeps its change, and after a change to the tabs it
doesn't save the arrangement either. When you come back to Bench, or to an
app with a window in a tab, after editing the file, or open a menu or
dialog in Bench, the status bar also says the file has changed, with
**Reload**.

To use your edits, choose **Bench ▸ Reload config.toml**, or **Reload
config.toml** in the [jump palette](#the-jump-palette). Bench reads the
file again and applies it without a restart, keeping the arrangement:

- A tab with the same name, ignoring case, keeps its windows and how
  you've arranged them, and takes your edits to its items (see below). If
  you change its `layout` or `sizes`, it takes the new layout, keeping its
  windows in order. A tab you renamed keeps them too, if that's your only
  change to the tabs.
- A tab you added starts as the file lays it out. A tab you removed is
  deleted, and the status bar says it lost its arrangement, with
  **Restore previous arrangement** once a tab has its name again: see
  [When Bench can't keep the arrangement](#when-bench-cant-keep-the-arrangement).
  A tab you renamed along with other changes to the tabs counts as removed
  and added. The tabs take the file's order.
- Settings and rules apply from then on, to new windows. Changed shortcuts
  work at once.
- An item whose entry you removed, or whose tab you removed, leaves its
  tab. A window goes where an `app` entry claims it, as when Bench starts,
  or else to **Not in a tab**. A web page or terminal carries on as
  the item of an entry you added for the same address, or the same folder
  and command, so the page stays loaded and the terminal keeps running.
  Otherwise, a web page or terminal whose entry you removed closes, and
  one whose tab you removed moves to **Not in a tab**.
- A web page whose address or `profile` you changed loads again. A
  terminal whose `command` you changed runs the new one the next time it
  starts.
- Windows that aren't in a tab go where an `app` entry you added claims
  them, as when Bench starts.

If the file has problems, Bench keeps the config it has and lists them. See
[Problems Bench reports](#problems-bench-reports).

When Bench starts, it keeps the arrangement it saved when it quit, and
applies your edits to a tab's items to it, as **Reload config.toml** does:
an item takes its entry's new settings, such as a `name`, `profile`, or
`url`, or a changed address or command; the items of an entry you removed leave the tab; and an entry you
added joins the pane the file lists it in. An `app` entry whose `title` you
change counts as a new entry, so its app's windows are placed again by the
new `title`. An item you closed in Bench stays in the file until you next
save the tab's properties, but isn't added back unless you change its
entry's address, folder, command, or `title`. For a window that's in
several tabs, editing its entry in any one of them changes it in all of
them, and removing its entry from one tab leaves it in the others.

### Tabs you change in the file

Each tab keeps its arrangement by name, ignoring case, so you can add,
remove, and reorder `[[tab]]` entries while Bench isn't running. When
Bench starts:

- A tab keeps the arrangement saved for the tab of its name, wherever it is
  in the file. The tabs take the file's order.
- A tab you added starts as the file lays it out.
- A tab you removed loses its arrangement. Its windows go where the file's
  other entries claim them, as when Bench starts.
- A tab you renamed keeps its arrangement, if that's your only change to
  the tabs. Renamed along with other changes, it can't be told from a new
  tab, so it starts as the file lays it out, and the status bar says so:
  see [When Bench can't keep the arrangement](#when-bench-cant-keep-the-arrangement).
  To be safe, rename tabs one at a time, or in
  [Tab properties](#tab-properties).

Bench checks the file when it starts, when you reload it, and when you
save Settings or a tab's properties. If the file has problems, Bench lists
them in the status bar, with the tab and field for each, and runs with the
last config that loaded without problems, so your tabs stay. Only if there
has never been one does it run with the default config. It doesn't
overwrite your file. Until you fix the file, Bench doesn't save changes
from Settings and Tab properties, so they're lost when you quit. It goes
on saving the arrangement (`state.json`) only if that was saved for the
config it runs with: see [Problems Bench reports](#problems-bench-reports).
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
The [example config](#example-config) shows them all.

### Settings

The `[settings]` table. Any setting you leave out takes its default.

| Setting | Default | Values | What it does |
| --- | --- | --- | --- |
| `fill_screen` | `true` | `true` or `false` | Open Bench filling the screen. Either way, Bench opens on the display it was last on. See [Fill the screen](#fill-the-screen). |
| `menu_bar` | `"name"` | `"name"`, `"icon"`, or `"hidden"` | What Bench's menu bar item shows: its icon and up to 16 characters of the current tab's name, its icon alone, or no item at all. |
| `show_on_tab_shortcut` | `true` | `true` or `false` | A tab shortcut brings back a hidden Bench, on that tab. See [When you hide Bench](#when-you-hide-bench). |
| `confirm_quit_busy_terminals` | `true` | `true` or `false` | Ask before quitting Bench, or closing a terminal, from its sub-tab, its own window, or **Close tab…** (even with `confirm_close` off), while a terminal is running something. See [When Bench quits](#when-bench-quits). |
| `confirm_close` | `true` | `true` or `false` | Ask before **Close tab…** closes two or more windows, web pages, or terminals, or a terminal that's running something. With it off, `confirm_quit_busy_terminals` still asks before ending a busy terminal. See [Close windows and tabs](#close-windows-and-tabs). |
| `compact_below` | `1800` | A positive number of points | Screens narrower than this are small, and use the compact layout in Auto. |
| `gap` | `4` | 0 to 100 points | The space between panes, and around them. Settings offers 8, 4, and 2. |
| `show_status_bar` | `true` | `true` or `false` | Show the status bar. Without it, the panes are taller, and its warnings and messages show in the title bar. See [The status bar](#the-status-bar). |
| `tab_shortcut_hints` | `"auto"` | `"auto"`, `"always"`, or `"never"` | When tabs show their shortcuts after their names: except on small screens, always, or never. |
| `scroll_bars` | `"auto"` | `"auto"`, `"always"`, or `"never"` | When the strips of tabs and sub-tabs that don't fit show a scroll bar: as macOS shows scroll bars, always, or never. See [When tabs and sub-tabs don't fit](#when-tabs-and-sub-tabs-dont-fit). |
| `density` | `"auto"` | `"auto"`, `"standard"`, or `"compact"` | How much room the title bar, tabs, pane headers, and sub-tabs take: compact in the compact layout, always standard, or always compact. A compact pane header is 32 points tall rather than 40. The title bar changes the next time Bench opens. See [More room for your apps](#more-room-for-your-apps). |
| `pane_headers` | `"always"` | `"always"` or `"auto"` | Whether every pane has a header, or a pane that holds one App window, with nothing to report, goes without, leaving the app's own title bar. See [More room for your apps](#more-room-for-your-apps). |
| `open_missing_apps` | `true` | `true` or `false` | Open a tab's apps when you switch to it, if they aren't running, unless you quit them yourself. See [When an app quits](#when-an-app-quits). |
| `hide_other_tabs` | `true` | `true` or `false` | Hide windows that belong to other tabs. |
| `hide_other_apps` | `false` | `true` or `false` | When switching tabs, also hide every app that Bench doesn't manage. |
| `new_windows` | `"app-pane"` | `"app-pane"`, `"focused-pane"`, or `"float"` | Where an app's new windows go. See below. |
| `follow_focus` | `"switch-tab"` | `"switch-tab"`, `"bring-here"`, or `"off"` | What happens when another tab's window comes forward. See below. |
| `live_previews` | `false` | `true` or `false` | Show live previews of windows. Needs Screen Recording permission. |
| `launch_timeout` | `"15s"` | A duration, such as `"15s"` or `"1m30s"` | How long to wait for a tab's apps to open their windows. It also sets how long an app's windows go back to its old places after it opens: see [When an app quits](#when-an-app-quits). |

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
| `name` | Yes | The tab's name. Names must be unique, ignoring case: Bench reports a name that `config.toml` repeats as a problem, rather than rename the tab. Tab properties numbers a taken name instead. |
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
| `web` | Web | The address to load. It must start with `https://`. For a site that you sign in to, its web app, as an `app` item, is often better: it has the browser's protections and sign-ins (see [Web apps](#web-apps)). |
| `profile` | Web (`web` items only) | The name of the item's data store. Items with the same profile share logins and cookies. Items without a profile share one default profile. Profile names are case-sensitive: `Work` and `work` are different profiles. |
| `vscode` | Web | A folder to open in VS Code for the Web. |
| `terminal` | Built-in | The terminal's starting folder. `~` means your home folder. |
| `command` | Built-in | A command to run instead of your shell. When it finishes, the terminal stays with its output, and says so: press Return in it to run the command again. It also runs again when Bench next opens. |
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

Programs can print links in a terminal, and the text a link shows needn't
be its address. Holding the pointer over a link underlines it and shows the
address it opens, with the host in bold. Command-click the link to open it
in your default browser. A plain click doesn't open it, so a click to
select text or to type in the terminal never takes you to a site that a
program's output chose. Only `http` and `https` links open. A host with
letters from other scripts shows in its ASCII form, starting `xn--`, so
letters that look like Latin ones can't pass for them, and a very long
address is cut short, keeping the host.

An app can appear in several tabs and panes. Use `title` to divide its
windows; windows that match no `title` go to the first pane that lists the
app without one.

To find an app's bundle ID, run:

```sh
osascript -e 'id of app "Brave Browser"'
```

For a web app, read the bundle ID from the app itself, since a web app can
have the same name as a desktop app:

```sh
plutil -extract CFBundleIdentifier raw ~/Applications/"Chrome Apps.localized/Gmail.app"/Contents/Info.plist
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

# Gmail as a web app that Chrome made. Its bundle ID is Chrome's, then .app.
# and an ID for the site: read yours as shown above.
items = [{ app = "com.google.Chrome.app.fmgjjmmmlfnkbppncabfkddbjimcfncm" }]

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
| `float` | One of `float` and `tab` | `true` leaves matching windows floating, such as a browser's picture-in-picture window. When you click Bench, it hides other apps whose windows are over it, but these windows stay. |
| `tab` | One of `float` and `tab` | The name of the tab that new windows of the app join. |

```toml
[[rule]]
app = "com.apple.mail"
tab = "Comms"
```

### Problems Bench reports

Bench reports these problems, naming the tab and field for each, and runs
with the last config that loaded without problems until you fix them (or
the default config, if there has never been one). Even one problem, such as
a misspelt field, does this: Bench doesn't run with part of the file, so
that what you see always matches a whole config that worked.

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

The first notice in the status bar says what Bench is running with, and has
buttons:

| Button | What it does |
| --- | --- |
| **Open config.toml** | Opens the file in your default app for plain text, such as TextEdit. |
| **Show in Finder** | Shows the file in the Finder. |
| **Reload** | Reads the file again, once you've fixed it, and uses it. |
| **Use backup** | Puts `config.toml.bak` in place of the file, if the backup has no problems, and uses it. The file with problems becomes `config.toml.bak`, so you can still fix it and swap it back. Only there when the backup has no problems. |

Until you fix the file, Bench doesn't save `config.toml`. Changes in
Settings and Tab properties apply, and say **Applied for now, not saved:
config.toml has problems**; they're lost when you quit. When Bench starts
with problems in the file, it restores the arrangement in `state.json` only
if it has the tabs of the config Bench runs with. If that's the last config
that worked, Bench goes on saving the arrangement, until you add, rename, or
delete a tab; the first notice says whether it does. Otherwise Bench leaves
`state.json` alone, even if it can't use it, so that fixing the file brings
the arrangement back: when Bench next starts, or, if you reload the file,
with **Restore previous arrangement** in the status bar. A `state.json` it
can't use is set aside then, as below.

### When Bench can't keep the arrangement

Bench saves the live arrangement in `state.json`. Whenever it can't keep
some or all of it, when it starts or when you reload `config.toml`, the
status bar says which tabs lost their arrangement, and why, and Bench keeps
the file it couldn't use, or a copy, as `state.json.bad`. **Dismiss** takes
the notice away.

- **config.toml had no tab of that name.** You removed the tab, or renamed
  it along with other changes to the tabs (see
  [Tabs you change in the file](#tabs-you-change-in-the-file)). The other
  tabs keep their arrangement. To get a renamed tab's arrangement back,
  give a tab its old name again, in Tab properties, or in `config.toml`
  and then reload it, and choose **Restore previous arrangement** in the
  notice. The tab gets back its panes, sizes, windows, pages, and
  terminals. A page or terminal that the tab has for the same entry as one
  of the arrangement's takes its place, so the page stays loaded and the
  terminal keeps running. The tab's other pages and terminals move to
  **Not in a tab**, and so do its other windows, unless they go back to
  the arrangement. Then rename it as you like in Tab properties, which
  keeps the arrangement.
- **Bench started with another config while config.toml had problems.**
  The arrangement was saved for your tabs, which that config didn't have.
  Once you fix the file and reload it, choose **Restore previous
  arrangement**.
- **state.json can't be used**, for example because it doesn't parse, or
  a newer version of Bench saved it. The tabs start as `config.toml` lays
  them out, and the file can't be restored.

Bench keeps the last three files it set aside: `state.json.bad` is the
latest, then `state.json.bad.1` and `state.json.bad.2`.

### The default file

On first run, Bench writes this file, with your default browser in place of
Safari if it's another. A browser picker, such as Finicky, doesn't count:
Bench uses Safari then. For a browser other than Safari, the file also has a
[rule](#rules) that leaves the browser's picture-in-picture window floating:

```toml
[settings]
fill_screen = true            # Open Bench filling the screen.
menu_bar = "name"             # name, icon, or hidden
show_on_tab_shortcut = true   # Bring back a hidden Bench with tab shortcuts.
confirm_quit_busy_terminals = true
confirm_close = true          # Ask before closing a tab's windows.
compact_below = 1800          # Screens narrower than this, in points, are small.
gap = 4                       # Points between panes.
show_status_bar = true        # The status bar at the bottom of the window.
tab_shortcut_hints = "auto"   # auto, always, or never
scroll_bars = "auto"          # auto, always, or never
density = "auto"              # auto, standard, or compact
pane_headers = "always"       # always, or auto: none over one app window
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
name = "Research"
layout = "single"

  [[tab.pane]]
  items = [{ app = "com.apple.Safari" }]
```

### Example config

A fuller example, which the browser preview starts with:

```toml
[settings]
fill_screen = true            # Open Bench filling the screen.
menu_bar = "name"             # name, icon, or hidden
show_on_tab_shortcut = true   # Bring back a hidden Bench with tab shortcuts.
confirm_quit_busy_terminals = true
confirm_close = true          # Ask before closing a tab's windows.
compact_below = 1800          # Screens narrower than this, in points, are small.
gap = 4                       # Points between panes.
show_status_bar = true        # The status bar at the bottom of the window.
tab_shortcut_hints = "auto"   # auto, always, or never
scroll_bars = "auto"          # auto, always, or never
density = "auto"              # auto, standard, or compact
pane_headers = "always"       # always, or auto: none over one app window
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

After editing `config.toml`, choose **Bench ▸ Reload config.toml**. Bench
registers the new shortcuts at once.

### Google sign-in doesn't work in a web item

Google blocks its sign-in pages in embedded web views, with the error
`disallowed_useragent`, and Web items are embedded web views. If you sign
in to Slack (or another service) with Google, make it a web app in your
browser: **Web page…** in the add dialog does that where your browser can,
unless you press Option-Return, and so does the **⋯** menu of the page you
have (see [Switch a web page to a web app](#switch-a-web-page-to-a-web-app)).
Or use its desktop app as an App item instead:

```toml
items = [{ app = "com.tinyspeck.slackmacgap" }]
```

### A page opens in the browser instead of the web item

Bench keeps pages from other sites out of a web item (see
[New windows and links](#new-windows-and-links)), so these open in your
browser:

- Another customer's name on a shared service counts as another site. In
  SharePoint, for example, OneDrive's `contoso-my.sharepoint.com` is another
  site than `contoso.sharepoint.com`.
- A sign-in that starts with your click, or that you go back to, can't send
  you on to a site that isn't a sign-in page Bench knows, such as your
  organisation's own sign-in server. The item goes back to its own address:
  sign in from there, so that the item starts the sign-in itself.

If you need such pages often, use the site as a web app in your browser
instead (see [Web apps](#web-apps)).

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
follow it into a full-screen space. Instead, it opens filling the screen,
below the menu bar and beside the Dock. If you've made it smaller, choose
**View ▸ Fill the Screen**, or **Fill the screen** in the jump palette. See
[Fill the screen](#fill-the-screen).

If Bench opens at its usual size, check that **Open Bench filling the
screen** is ticked in [Settings](#settings).

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

The status bar lists each problem with its tab and field (or, while the
status bar is hidden, ***N* notices** in the title bar), and Bench runs
with the last config that worked until you fix them. Until then, Bench
doesn't save changes from Settings and Tab properties, so they're lost when
you quit; the message after saving says so. The first message says
whether Bench saves the arrangement (`state.json`) meanwhile.

1. Click the message in the status bar, or ***N* notices** in the title
   bar, then **Open config.toml**, to open the file in your text editor.
   **Show in Finder** shows where it is.
2. Fix each problem listed. `bench check-config` lists them too.
3. Save the file, and click **Reload**. Bench uses the file at once, and
   saves again.

If you can't fix it, click **Use backup** to go back to `config.toml.bak`,
the version before the last save from Settings or Tab properties. It's
offered only if the backup has no problems. Or rename the file: Bench
writes a new default one when it next opens.
