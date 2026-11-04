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
- [Undo, and reopen what you closed](#undo-and-reopen-what-you-closed)
- [Add to a pane](#add-to-a-pane)
- [Launchers](#launchers)
- [Terminals](#terminals)
- [Web apps](#web-apps)
- [The jump palette](#the-jump-palette)
- [The menu bar](#the-menu-bar)
- [Tab properties](#tab-properties)
- [Settings](#settings)
- [Move and resize dialogs](#move-and-resize-dialogs)
- [Small screens: Auto, Wide, and Compact](#small-screens-auto-wide-and-compact)
- [Live previews](#live-previews)
- [New windows and links](#new-windows-and-links)
- [Keyboard shortcuts](#keyboard-shortcuts)
- [VoiceOver](#voiceover)
- [Fill the screen](#fill-the-screen)
- [When you hide Bench](#when-you-hide-bench)
- [When Bench quits](#when-bench-quits)
- [Version and acknowledgements](#version-and-acknowledgements)
- [Update Bench](#update-bench)
- [Bench's files](#benchs-files)
- [Command line](#command-line)
- [Config reference](#config-reference)
- [Troubleshooting](#troubleshooting)

## Get started

When you first open Bench, a welcome guide takes you through three pages.

**1. Permissions, and Open at login.**

1. Click **Turn on Accessibility**. macOS may ask whether Bench can control
   your computer, and Bench opens **System Settings ▸ Privacy & Security ▸
   Accessibility** too.
2. There, turn on Bench.
3. Back in Bench, wait for the guide to say that Bench has Accessibility
   permission.

Screen Recording is optional: it's only for
[live previews](#live-previews). **Turn on Screen Recording** asks for it,
and opens its list in System Settings. macOS applies it only when Bench
opens again, so the guide then offers **Quit and Reopen Bench**.

Tick **Open Bench when you log in** if you'd like your tabs ready when you
log in. It's off until you tick it, and **Settings** can change it later.
If macOS needs you to allow Bench first, the guide says so, with **Open
Login Items**: turn on Bench in **System Settings ▸ General ▸ Login Items**.

Click **Continue**.

**2. Choose a starting set-up.** If you have windows open that aren't in a
tab, the first choice is **A tab of your open windows**, with their apps.
Click **Choose windows…** to open **New tab** with those windows ticked, and
click **Make tab**: see
[Make a tab of the windows that are open](#make-a-tab-of-the-windows-that-are-open).
Bench then comes back to the set-up, which names the tab you made. To make
another tab, click **Choose windows…** again. **Cancel** comes back to the
set-up too. If you click **Undo** in the message, the set-up no longer
names the tab.

Under **Suggested tabs**, Bench suggests a few tabs made of apps you have,
such as Comms with Slack and Mail, Code with VS Code and a terminal, and
Plan with Obsidian and Calendar. It suggests only apps that are installed,
and only tabs you don't have. Click **Remove** beside any you don't want,
then **Add *N* tabs**; or click **Start with one tab** to add none. If you
have more than one tab already, such as from a `config.toml` copied from
another Mac, that button reads **Don't add tabs**. After you make a tab of
your open windows, it reads **Don't add these**, or **Continue** if no
suggested tabs are left. Bench shows this page only the first time.

**3. A few tips**: tabs and their shortcuts, dragging sub-tabs, the jump
palette, and right-click menus. Click **Done**.

To see the guide again, choose **Help ▸ Show Welcome Guide**, or **Show the
welcome guide** in the [jump palette](#the-jump-palette).

Bench needs Accessibility permission to arrange other apps' windows: to move
and size them into its panes, and to hide the ones that belong to other
tabs. It also uses it to read the unread counts on apps' Dock icons (see
[Unread counts](#unread-counts)). Its keyboard shortcuts work without it,
but Bench can't arrange windows until you allow it. Until then, a banner at
the top of the window reminds you, with **Turn on Accessibility**.

After you update Bench, macOS may stop applying its Accessibility
permission, though System Settings still shows Bench turned on: Bench's
releases aren't notarised yet, so macOS takes the new version for a new
app. Bench notices, and the banner says so. Click **Turn on
Accessibility**, select Bench in the list, click **−** to remove it, then
add it again and turn it on. See
[Bench can't move windows](#bench-cant-move-windows).

Bench starts with one tab, Research, which holds your default browser: the
app that opens web links, or Safari if there's none, and the tabs you add
in the starting set-up. To change a tab, right-click it and choose
[Tab properties](#tab-properties). Add your own tabs with **+** in the
title bar, or in the [config file](#config-reference).

Bench's window fills the screen, below the menu bar and beside the Dock.
See [Fill the screen](#fill-the-screen).

Bench has a menu bar item: Bench's icon, with the current tab's name beside
it, and the number of unread messages in your tabs, if there are any (see
[Unread counts](#unread-counts)). Click it to switch tabs, open **Jump
to…** or **Settings**, show Bench, or quit. Its menu shows each tab's
shortcut and unread count. To show the icon alone, or no menu bar item at
all, see [Settings](#settings).

## Tabs

A tab is a named workspace with its own layout. The tabs are in Bench's title
bar, and the current tab has a raised background and an orange underline.
Each of the first nine tabs shows its shortcut after its name, except on
small screens, where the tabs need the room; **Show shortcuts on tabs** in
[Settings](#settings) changes that. The current tab, and a tab under the
pointer or with the keyboard focus, show a **×** in the shortcut's place,
which closes the tab: see
[Close windows and tabs](#close-windows-and-tabs). Hold the pointer over a
tab to see its whole name and its shortcut. A tab whose apps or pages have
unread messages shows how many after its name, in a grey pill: see
[Unread counts](#unread-counts).

To switch tabs:

- Click a tab.
- Press Control+Option+1 to 9 for the first nine tabs, or Control+Option+]
  and Control+Option+[ for the next and previous tab.
- Press Control+Option+&#96; (the key left of 1) to go back to the tab you
  were on, and again to come back: it takes you between the two tabs you
  used last. **Tab ▸ Last Used Tab** does the same.
- Hold Control+Option after you press Control+Option+&#96;, to choose from
  every tab, as Command-Tab does for apps. See
  [Hold to switch tabs](#hold-to-switch-tabs).
- Choose a tab in the overview of every tab: choose **View ▸ Show All
  Tabs**. See [See every tab at once](#see-every-tab-at-once).
- Choose a tab in the [jump palette](#the-jump-palette), from the **Tab**
  menu, or from Bench's menu bar item. **Tab ▸ Next Tab** and **Previous
  Tab** step through them. The jump palette lists the tab you were on
  first, so Control+Option+Space then Return goes back to it too.

When you switch to a tab whose apps aren't open, Bench opens them, unless
`open_missing_apps` is off. It doesn't open an app that you quit yourself,
until the app opens again: see [When an app quits](#when-an-app-quits). If
the focused pane is still waiting for its window, the window gets the
keyboard as soon as it arrives, unless you've focused something else or
turned to another app in the meantime, or a menu or dialog is open.

There's no limit on the number of tabs. Only the first nine get a
shortcut. When they don't all fit, the tab strip scrolls: see
[When tabs and sub-tabs don't fit](#when-tabs-and-sub-tabs-dont-fit).

To put the tabs in another order:

- Drag a tab along the title bar. A line between two tabs shows where it
  will go; let go there. Press Escape to cancel. Pressing a tab's **×**, or
  the title bar's empty space, doesn't drag the tab.
- Choose **Move Tab Left** or **Move Tab Right** from the tab's right-click
  menu, or, for the current tab, from the
  [jump palette](#the-jump-palette) or the menu bar's **Tab** menu.
- Choose a **Position** in [Tab properties](#tab-properties).

Shortcuts follow the order, so a tab you move to third place gets
Control+Option+3. Each tab keeps its windows and its layout, the tab you're
on stays the current one, and `config.toml` lists the tabs in the new order,
keeping the previous version as `config.toml.bak`. The message that says so
has **Undo**.

To rename a tab, double-click it, or choose **Rename…** from its right-click
menu, or **Tab ▸ Rename…** for the current tab. Its name turns into a text
field, with the name selected:

- Type the new name and press Return to save it. The tab keeps its layout,
  its windows, and what it does on small screens, and Bench saves the name
  in `config.toml`, keeping the previous version as `config.toml.bak`.
- Press Escape, or click anywhere else, to keep the old name. Saving the
  same name, or no name, keeps it too.
- Names must be different, ignoring case. If another tab has the name you
  type, the tab gets a number, as in [Tab properties](#tab-properties):
  **Code** becomes **Code (1)**, or **Code (2)** if that's taken too. The
  field shows the name the tab will have, after an arrow, before you press
  Return.

Double-clicking a tab doesn't zoom Bench's window, as double-clicking the
title bar's empty space does. Double-clicking a tab that isn't current
switches to it as well.

Right-click a tab, or Control-click it, for its menu, which opens where you
click. Right-clicking a tab doesn't switch to it. The menu has:

- **Tab Properties…**, which opens the tab's
  [Tab properties](#tab-properties): its name, its position, its layout,
  and what it does on small screens. To delete the tab, use **Delete tab**
  there.
- **Rename…**, which turns the tab's name into a text field, as
  double-clicking the tab does.
- **Move Tab Left** and **Move Tab Right**, which move the tab one place
  along the tabs. The first tab can't move left, and the last can't move
  right.
- **Rearrange**, which arranges the tab's windows again. For a tab that
  isn't the current one, Bench switches to it first.
- **Reset Layout**, which returns the tab to the layout in its config,
  undoing your dragging and splitting, without asking first. The message
  that says so has **Undo**.
- **Close Tab…**, which closes the tab's windows, web pages, and terminals,
  and deletes the tab, and **Delete Tab (Keep Windows)**, which deletes the
  tab and moves its windows to **Not in a tab**. On the only tab, it's
  **Close All Windows…**, which leaves the tab, empty, and there's no
  **Delete Tab (Keep Windows)**. See
  [Close windows and tabs](#close-windows-and-tabs).
- **New Tab…**, and **Reopen Closed Tab**, with the name of the tab you
  deleted or closed last, which brings it back, and **Reopen Closed Page**
  or **Reopen Closed Terminal**, with the name of the web page or terminal
  you closed last. See
  [Undo, and reopen what you closed](#undo-and-reopen-what-you-closed).

To add a tab, click **+** after the tabs, or choose **New Tab…** from a
tab's right-click menu. **New tab** opens, to name the tab and choose its
layout. To make a tab of windows that are open, choose **Windows that are
open** there: see
[Make a tab of the windows that are open](#make-a-tab-of-the-windows-that-are-open).
To give a window a tab of its own, drop its sub-tab on the **+**: see
[Give a window a tab of its own](#give-a-window-a-tab-of-its-own).

### See every tab at once

The overview shows each tab as a card. Each card shows:

- The tab's name and its shortcut. The current tab's card says
  **Current**.
- The tab's panes, as the wide layout shows them, in the shape of the area
  that Bench's panes fill.
- In each pane, the window that the pane shows, with its app's icon and
  its title. After it are the icons of the pane's other windows.
- A bell for a window that needs you, ⚠ for a window with a problem, and
  the camera or microphone for a web page that uses them. See
  [What the markers mean](#what-the-markers-mean).
- The tab's unread total, and the unread count of each window that a pane
  shows. See [Unread counts](#unread-counts).

The overview shows no pictures of your windows, so it needs no Screen
Recording permission.

To open the overview, do one of these:

- Choose **View ▸ Show All Tabs**.
- Choose **Show All Tabs** in the [jump palette](#the-jump-palette).
- Press the **Show all tabs** shortcut. It's off until you give it keys:
  see [Change a shortcut](#change-a-shortcut). Press it again to close the
  overview.

The tab you were on before this one is chosen first. Then:

- Type words to see only the tabs that have each word in their name, or
  in a window's title or app.
- Press the Left and Right arrows to choose the card before or after. Press
  the Up and Down arrows to choose the card in the row above or below.
- Press Return, or click a card, to switch to that tab. A Control-click on
  a card does the same.
- With nothing typed, press 1 to 9 to switch to that tab.
- Press Escape, or click outside the overview, to close it. If you were
  typing in a window docked in Bench when you opened the overview with its
  shortcut, that window gets the keyboard back.

With VoiceOver, each card reads the tab's name and shortcut first. Then it
reads whether the tab is the current one, how many windows the tab has, the
windows that it shows, and its marks.

### Hold to switch tabs

Control+Option+&#96; (**Last used tab**) works as Command-Tab does for
apps:

1. Press Control+Option+&#96;. Keep Control and Option held down.
2. After a moment, the overview opens. It shows the tabs in the order you
   used them, latest first: the current tab, then the tab you were on
   before it. That tab is chosen.
3. To choose the next tab, press &#96; again. To choose the tab before,
   press Shift+&#96;. The arrow keys also move the choice: Left and Right
   to the card before or after, and Up and Down to the card in the row
   above or below. Control+Option+] and Control+Option+[ move it too.
4. Let go of Control and Option. Bench switches to the tab that you chose.

A quick press goes back to the tab you were on, as before. To switch at
once, press Return or click a card. To stay on the current tab, press
Escape, then let go. Control+Option+1 to 9 switch to that tab, as usual.

If you press another shortcut before the overview opens, Bench first goes
back to the tab you were on. The shortcut then works on that tab, as it did
after a quick press.

Shift+&#96; uses the keys of the **Send a window to a tab** shortcut. It
works while that shortcut has its default keys, Control+Option+Shift.

Before you switch tabs for the first time, a quick press tells you that
there's no tab to go back to. Hold the keys to choose a tab in the
overview.

The hold uses the modifier keys of the **Last used tab** shortcut. If you
change the shortcut in **Settings ▸ Keyboard**, hold its new modifier keys.
Bench goes back to the tab you were on at once, without the overview, in
these cases:

- A menu or dialog is open in Bench.
- The shortcut has no modifier keys, such as F6.
- Bench is hidden, and **Bring back a hidden Bench with tab shortcuts** is
  off in [Settings](#settings).

If the Mac sleeps before you let go, or if you hold the keys for more than
a minute, Bench closes the overview and stays on the current tab.

### Make a tab of the windows that are open

To make a tab of windows that you have open, do one of these:

- Click **+** after the tabs. In **New tab**, beside **Start with**, choose
  **Windows that are open**.
- Choose **Tab ▸ New Tab from Open Windows…** in the menu bar.
- Choose **New Tab from Open Windows…** in the
  [jump palette](#the-jump-palette).
- In the welcome guide's starting set-up, click **Choose windows…**. See
  [Get started](#get-started).

**New tab** then lists the windows under **Windows**, grouped by app, with
each app's icon and each window's title:

- The windows that aren't in a tab come first. They're ticked. Untick the
  windows you don't want. The tick box beside an app's name ticks or
  unticks all of its windows.
- **Windows in other tabs** opens a list of the windows, web pages, and
  terminals in your tabs, with the tab that each is in. They aren't ticked.
  A window that you tick there stays in its tab, and shows in the new tab
  as well, as **Also Show In** does. See
  [Show a window in more than one tab](#show-a-window-in-more-than-one-tab).

Each app's windows share a pane, as sub-tabs. Bench suggests a layout with
a pane for each app: **Single** for one app, **Side by side** for two,
**Three columns** for three, and **Grid of four** for four or more. The
panes fill in the order of the list. When there are more apps than panes,
the apps after the last pane go in the last pane. To use another layout,
choose it: the preview shows which windows go in which pane. Bench then
keeps that layout when you tick more windows.

The tab's name is the first ticked app's name, such as **Brave**, until you
type a name. If a tab has that name already, the new tab gets a number:
**Brave (1)**, then **Brave (2)**.

Click **Make tab**. Bench makes the tab, switches to it, and docks the
windows in its panes. Each pane shows its first window. Bench saves the tab
in `config.toml`, keeping the previous version as `config.toml.bak`. The
message says how many windows the tab has. Its **Undo** deletes the tab,
from `config.toml` too, and puts the windows back where they were:

- A window that wasn't in a tab goes back to **Not in a tab**, and moves
  back to its place on the screen.
- A web page or terminal that had a window of its own gets its window back.
- A window from another tab stays in that tab.

Things to know:

- If a ticked window closes before you click **Make tab**, Bench leaves it
  out. If all of them have closed, Bench says so.
- The windows of an app that has quit aren't in the list. See
  [When an app quits](#when-an-app-quits).
- To make a tab with no windows, choose **An empty layout** beside **Start
  with**. The ticks stay as they are if you go back to **Windows that are
  open**.

### When tabs and sub-tabs don't fit

A tab is never wider than 200 points: a longer name is cut short with an
ellipsis, keeping the tab's shortcut at its end, and the tab's tooltip has
the whole name. A sub-tab is never narrower than 96 points, so that its
icon and the start of its title stay readable, and is between 220 and
360 points at its widest, more in a wider pane. The shown sub-tab gets
22 points more, for its **×**. Its tooltip has its whole title, with its
app.

When the tabs in the title bar, or a pane's sub-tabs, don't all fit, their
strip scrolls sideways rather than squashing them:

- To scroll a strip, hold the pointer over it and swipe on a trackpad, up
  and down or sideways, or turn a mouse wheel.
- An end of a strip with more to show fades out.
- In a strip of sub-tabs that doesn't fit, each sub-tab is 96 points
  wide, the shown one too. Its **×** is then over the end of its title.
- The current tab, and the sub-tab that each pane shows, stay in view. When
  you switch tabs or sub-tabs, add a sub-tab, or resize Bench, the strip
  scrolls to show them. A strip that you've scrolled yourself, or moved
  along with the arrow keys, stays where you left it until it shows another
  tab or sub-tab, one is added or removed, or Bench is resized. See
  [Use the keyboard in Bench's window](#use-the-keyboard-in-benchs-window).
- While you [drag a sub-tab](#move-windows), or [a tab](#tabs), hold it
  near either end of a strip to scroll the strip, so as to drop it on a
  tab that's out of view, or between tabs or sub-tabs out of view.
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
- **Resize panes by dragging a window's edge.** Drag an edge of an App
  window, as you would drag the edge of any window. The edge must face
  another pane. When you let go, the gap between the panes goes to where
  the edge is, the windows dock again, and a message says "Resized the
  panes." with **Undo**. Drag a corner to move two gaps at once, such as
  the corner between the right and bottom edges of the top-left pane in a
  Grid of four. Each pane keeps at least 10% of its row or column. If a
  pane can't change any more that way, its window goes back, and the
  message says so. An edge along the outside of the pane area has no pane
  beyond it. Dragging that edge moves the window out of its pane.
  Dragging the window by its title bar does too. This works whether
  **Drag windows onto Bench to add them** is on or off.
- **Make panes the same size** by double-clicking the gap between them, or
  by pressing Return while the gap is selected. The panes in that row or
  column share the room equally. **Undo** in the message puts the sizes
  back.
- **Grow or shrink the focused pane**, make all the panes the same size,
  or change the tab's layout from the keyboard. See
  [Arrange panes from the keyboard](#arrange-panes-from-the-keyboard).
- **Zoom a pane**, to see it alone for a while, with Control+Option+Return.
  See [Zoom a pane](#zoom-a-pane).
- **Arrange the tab again** with Control+Option+0, **Rearrange** in the
  jump palette, or **View ▸ Rearrange**, if windows have drifted out of
  place.

A pane's header holds:

| Control | What it does |
| --- | --- |
| Sub-tabs | Show a window. See [Sub-tabs](#sub-tabs). |
| **+** | [Add a window or app](#add-to-a-pane) to the pane. |
| Badge | The shown item's kind: **Web**, **App**, or **Built-in**. A web page shows its host instead, such as app.slack.com: see [Where a web page is](#where-a-web-page-is). |
| **Show side by side** | Split the pane to the right, with the shown window in the new half. |
| **⋯** | More ways to move the shown window. See [Move windows](#move-windows). Right-click a sub-tab, or an empty part of the header, to open the same menu where you click. |
| **Zoomed** | Only while the pane is zoomed: click it to show all the tab's panes again. See [Zoom a pane](#zoom-a-pane). |

Hold the pointer over a button in the title bar or a pane header to see
what it does, with its shortcut if it has one. Tooltips can also show while
you work in another app; if one doesn't, click Bench's title bar first.

An empty pane says how to fill it: **Add a window or app**, or "Drag a
sub-tab here, or press ⌃⌥⇧ and an arrow to move the shown window here", with
the move shortcut as you've set it. The shown window is the one in the
focused pane: focus the pane beside the empty one, then press the arrow
that points at it.

### More room for your apps

On a laptop, Bench's own title bar and pane headers take less room:

- **Density** in [Settings](#settings) sizes the title bar, the tabs, the
  pane headers, and their sub-tabs and buttons. **Automatic**, the default,
  is compact in the compact layout, as when you tile Bench into half of a
  MacBook's screen, and standard otherwise. To get the compact density in
  the wide layout too, choose **Compact**. In the compact density, a pane header is 32 points
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
  buttons do: **Add to Pane…** adds to the focused pane, and its **This
  window** group has what the window's ⋯ menu has, such as **Split Right**,
  **Pop Out of Bench**, and **Move to Tab ▸** another tab. **Window
  Options…** there opens the menu itself. Control+Option+Shift+arrow moves
  the window, as ever.

Some apps can hide their own toolbar, which leaves a thinner title bar:
choose **View ▸ Hide Toolbar** in Finder, Mail, or Preview. Bench can't hide
another app's title bar itself: macOS doesn't let one app change another's
window that way. A window with no title bar can't be undocked by dragging,
as there's nothing to drag it by. Use **Pop Out of Bench** in its menu
instead (see [Move windows](#move-windows)). While its pane has no header,
**Window Options…** in the jump palette opens that menu.

When a pane loses its last window, Bench removes it and the panes beside it
close up, unless the pane is keeping places for the windows of an app that
has quit: see [When an app quits](#when-an-app-quits). The last pane in a
tab stays, empty, with an **Add a window or app** button. Another pane that
shows nothing has a **Close this pane** button too. An empty pane has no
**⋯**: right-click it, or its header, for the same choices.

### Zoom a pane

Zoom a pane to give it the whole tab for a while: it fills the room that
the tab's panes share, with its own sub-tabs, and the tab's other panes are
out of sight. Their windows wait out of sight too, as they do in the
compact layout, and come back where they were when the panes do.

- To zoom the focused pane, press Control+Option+Return, or choose **Zoom
  Pane** in the [jump palette](#the-jump-palette) or the menu bar's **Pane**
  menu. **Zoom Pane** in a window's **⋯** menu, or in an empty pane's menu,
  zooms that pane.
- A zoomed pane has a **Zoomed** button in its header, and the status bar
  says **One pane zoomed**. Click either to show all the panes again. So
  does Control+Option+Return again, or **Show All Panes** in the jump
  palette, the **⋯** menu, or the menu bar's **Pane** menu.
- The panes also come back on their own when you switch to another tab,
  focus another pane, for example with Control+Option+arrow, or move the
  zoomed pane's window to another pane.

Things to know:

- The tab keeps its panes, their sizes, and their windows while one is
  zoomed. The status bar still counts them all, and **Move to Pane** in the
  **⋯** menu still lists the panes out of sight.
- A zoomed pane always has its header, to show the **Zoomed** button, even
  with **Hide the sub-tab row when a pane holds one app window** on.
- There's nothing to zoom in a tab with one pane, or in a tab shown as one
  pane on a small screen.
- Bench doesn't remember a zoom when it quits: each tab shows all its panes
  when Bench next opens.

### Arrange panes from the keyboard

You can change the size of the focused pane, and the layout of the current
tab, without the mouse. Each action has a row in the
[jump palette](#the-jump-palette) and an item in the
[menu bar](#the-menu-bar). Each action can also have a shortcut that works
from any app. The shortcuts are off until you set them in
**Settings ▸ Keyboard** (see [Change a shortcut](#change-a-shortcut)).

| Action | Jump palette | Menu bar | What it does |
| --- | --- | --- | --- |
| Grow pane | **Grow Pane** | **Pane ▸ Grow Pane** | Makes the focused pane 5% larger. The other panes in its row or column get smaller. |
| Shrink pane | **Shrink Pane** | **Pane ▸ Shrink Pane** | Makes the focused pane 5% smaller. The other panes in its row or column get larger. |
| Make panes equal | **Make Panes Equal** | **Pane ▸ Make Panes Equal** | Gives the panes in each row and each column of the tab the same size. |
| Next layout | **Next Layout** | **Tab ▸ Next Layout** | Gives the tab the next layout. The palette row shows its name. |
| Previous layout | **Previous Layout** | **Tab ▸ Previous Layout** | Gives the tab the previous layout. The palette row shows its name. |

After each action, a message shows the result, for example
"Zed 70% · Terminal 30%" or "Layout: Three columns". Click **Undo** in the
message to put the tab back. If you do these actions one after the other,
while the message shows, one **Undo** puts the tab back as it was before
the first action.

Things to know:

- 5% is 5% of the row or column that holds the focused pane. In
  **Grid of four**, the pane grows along its row. No pane gets smaller than
  10% of its row or column, the same limit as when you drag a gap. When a
  pane can't change more, a message tells you.
- The layouts come in the order of **Tab properties**: Single, Side by
  side, Main and side, Three columns, Top and bottom, and Grid of four.
  After the last layout, the first comes again.
- The windows stay in order, as in Tab properties: the windows of the first
  pane go into the first pane of the new layout, and so on. The new layout
  has its own sizes.
- Bench skips a layout that has fewer panes than the tab has panes with
  windows in them. For example, a tab with two such panes skips Single.
  Empty panes don't count, and the new layout doesn't keep them. But an
  empty pane that the add dialog is open for counts, and stays.
- When you move a window into a tab, out of it, or between its panes, or
  split a pane, the tab's layout becomes custom. The layout actions then
  start from the layout that has the shape of the panes. For example, a
  tab that had **Side by side** is still **Side by side** while it has
  two panes next to each other, so **Next Layout** gives **Main and
  side**. Three panes next to each other are **Three columns**. If the
  panes don't have the shape of a layout, **Next Layout** gives the first
  layout that has room, and **Previous Layout** gives the last.
- Bench remembers the new layout and sizes, as it does after you drag a
  gap, but `config.toml` doesn't change. To keep the layout in
  `config.toml`, open [Tab properties](#tab-properties) and click
  **Save**. **Reset layout** there gives the tab the layout in
  `config.toml` again.
- A zoomed pane can't grow or shrink, because it fills the tab. **Make
  Panes Equal** and the layout actions show all the panes again first.
- In a tab shown as one pane with sub-tabs, on a small screen, there are no
  panes to arrange. The palette doesn't show these rows, the menu items are
  dimmed, and a shortcut shows a message. Choose **Wide** to show the panes.

## Sub-tabs

Each window in a pane has a sub-tab, showing the app's icon and the
window's title. A web page's sub-tab shows the site's icon once the page
has loaded, and a terminal's shows **>_**. Where there's no icon, as for a
page whose site has none, or an app that isn't installed, the sub-tab shows
two letters for the app instead, its monogram, such as **Zd** for Zed. The
jump palette, the add dialog, and **Settings ▸ Launchers** show the same
icons. Click a sub-tab to show its window, or press
Control+Option+Tab (and Control+Option+Shift+Tab) to move between the
focused pane's sub-tabs. The shown sub-tab has a **×** at its right end,
after its title, so that the **×** doesn't cover the title. In a strip that
doesn't fit, the **×** is over the end of the title: see
[When tabs and sub-tabs don't fit](#when-tabs-and-sub-tabs-dont-fit). The
other sub-tabs show a **×** when you hold the pointer over them, or when
they have the keyboard focus. There, the **×** is over the end of the
title. The **×** closes the window, as a middle-click on the sub-tab does.
See [Close windows and tabs](#close-windows-and-tabs).

A pane can hold three kinds of item, from any mix of apps:

| Kind | Badge | What it is | Examples |
| --- | --- | --- | --- |
| Web | **Web**, or the page's host | A web page that Bench shows inside its own window, with its own logins. | Slack, Outlook on the web, VS Code for the Web |
| App | **App** | Another app's real window, which Bench moves and sizes to fit the pane. It works as normal: typing, menus, and drag and drop. | Zed, VS Code, Brave, Obsidian |
| Built-in | **Built-in** | A terminal session that Bench draws itself. Its sub-tab shows the running command and the folder, or its `name`, or a name you give it (see [Name a terminal or web page](#name-a-terminal-or-web-page)). It closes when its shell exits. A terminal that Bench typed a command into, such as a [launcher](#launchers)'s, goes back to the shell's prompt when the command finishes, and one running a config file's `command` stays with its output. | `zsh`, `go test`, `claude` |

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
| Moved out of its pane | You dragged the window away with the mouse, or dragged an edge that has no other pane beyond it. Click **Dock again** to put it back. |
| *App*'s window can't shrink to fit this pane | The window's minimum size is bigger than the pane. Bench places it at the pane's top-left. Try the compact layout. |
| In full screen | The window is in native full screen. Bench leaves it alone. |
| On another desktop | The window is on another desktop (Space). |
| Minimised | You minimised the window, for example with Command+M. It stays in the Dock. Click **Show again** to bring it back, or **Remove from this tab** if you don't want it here. See [When you minimise a window](#when-you-minimise-a-window). |
| The terminal couldn't start: … | Bench couldn't start the terminal's shell or command. For example, a terminal with a `command` doesn't run it when its folder doesn't exist. |

The sub-tab of an item that can't be shown has a marker on its icon,
whose shape says what kind of message it has (see
[What the markers mean](#what-the-markers-mean)). To read the message,
right-click the sub-tab, or click **⋯** while it's shown: the menu's first
line says why. Holding the pointer over a sub-tab shows its whole title
and its app, the title its shell or page sets if you've named it, the
message, why it needs you if it has a bell (see
[When a terminal needs you](#when-a-terminal-needs-you)), and the other
tabs it's in. When the window a pane shows needs more room than the pane
has, the pane's header also says **Needs more room**. The status bar counts
the current tab's windows that can't be shown: see
[The status bar](#the-status-bar).

When an app moves or resizes its own docked window, as some apps do just
after Bench sizes them, Bench puts the window back. If the app keeps doing
it, Bench stops after three tries in ten seconds and leaves the window where
the app put it, with no message, until the tab is next arranged. See
[A window isn't in its pane](#a-window-isnt-in-its-pane).

### When you minimise a window

When you minimise a docked window, with Command+M, its yellow button, or its
app's **Window** menu, Bench leaves it in the Dock. Its sub-tab gets the grey
ⓘ marker. If its pane has another window, the pane shows that window
instead. If not, the pane says **Minimised**, with **Show again** and
**Remove from this tab**.

Bench doesn't bring the window back when you switch tabs or arrange the tab.
To bring it back, do one of these:

- Click its sub-tab.
- Click **Show again** in its pane, or choose **Show Again** in its **⋯**
  menu.
- Choose it in [Jump to…](#the-jump-palette).
- Click the window in the Dock.
- Move it to a pane of its own, for example with **Show side by side**.

Bench then docks it in its pane again. **Remove from this tab** takes it out
of the current tab. If the window isn't in another tab, it moves out of
Bench and stays in the Dock. To add it back, click a pane's **+**, and choose
the window under **Windows not in a tab**. Bench then brings it back from
the Dock.

Bench doesn't remember that a window is minimised. When Bench opens, a
window that's still in the Dock shows as minimised again. A window that an
app opens in the Dock, for example as it reopens its windows after you log
in, also shows as minimised.

Bench sometimes minimises a window itself, when it has no other place to
hide it: see
[There's a thin line in a corner of the screen](#theres-a-thin-line-in-a-corner-of-the-screen).
That window has no marker, and Bench restores it when it's needed.

### Name a terminal or web page

A terminal's sub-tab shows the title its shell sets, such as the running
command and the folder, and a web page's shows the page's title. To give one
a name of your own, which is easier to tell apart:

1. Right-click its sub-tab, or click **⋯** while it's shown, and choose
   **Rename…**. The sub-tab's title turns into a text field, with the title
   selected.
2. Type the name and press Return. Press Escape, or click anywhere else, to
   cancel.

The sub-tab shows your name from then on, in every tab the terminal or page
is in, and in the [jump palette](#the-jump-palette), however its shell or
page changes its title. Bench remembers it in `state.json`, so it's still
there when Bench next opens. Holding the pointer over the sub-tab shows the
title the shell or page sets as well, after **Automatic title**.

To go back to that title, choose **Use Automatic Title** in the same menu,
which shows the title under it, or rename the sub-tab with no name. An app's
window takes its title from the app, so it has no **Rename…**.

### Unread counts

A sub-tab shows how many unread messages its app or web page has, in a grey
pill before its title. A tab shows the total of its sub-tabs after its
name. Bench gets the counts from two places:

- **The badge on an app's Dock icon**, such as the red **8** on Mail's
  icon. Bench reads the Dock's badges with Accessibility permission, every
  5 seconds, and soon after you go to another app. The badge is for the
  whole app, so it shows on one sub-tab only: the first of the app's
  windows in your tabs, from the first tab, in the order of the panes and
  their sub-tabs. When the app has more windows in your tabs, the pill's
  tooltip says **(whole app)**.
- **A count at the start of a web page's title**, such as **(3) Inbox** or
  **[12] Chat**, as many mail and chat sites show. Bench takes the count
  out of the sub-tab's title and shows it in the pill. Bench shows it only
  after the page loads, because a title from the last time Bench ran can
  be out of date. Bench loads a web page the first time you show it, so
  until then, the page's sub-tab and its tab show no count. After Bench
  opens, show each web page once to see its count.

A badge without a number, such as a dot, shows as a grey dot. A count of
more than 999 shows as **999+**. Hold the pointer over a pill to see the
whole number. The counts are the apps' own: when you read your messages,
the app changes its badge or the page changes its title, and Bench shows
the new count in at most 5 seconds.

When the current tab shows **one pane with sub-tabs** on a small screen,
the tab shows no pill. Its sub-tabs, with their counts, are next to it in
the title bar. Hold the pointer over the tab to see its total.

The counts show in other places too:

- Bench's menu bar item shows the total of every tab after the current
  tab's name, such as **Comms · 15**, unless it shows the icon only. The
  first line of its menu says what the number is, such as **15 Unread in
  All Tabs**.
- The menu bar item's menu and the **Tab** menu show each tab's count after
  its name, such as **Comms — 15 Unread**.
- The overview of every tab shows each tab's total, and the count of each
  window it shows. See [See every tab at once](#see-every-tab-at-once).
- VoiceOver reads the count after the name of a tab or sub-tab, such as
  "Comms, 15 unread".

Things to know:

- Bench can read only the badges of apps that have a Dock icon. An app
  without one, such as an app that shows only in the menu bar, has no
  badge for Bench to read.
- Bench reads the Dock's badges when the Dock is hidden too
  (**Automatically hide and show the Dock**).
- A web page that you move out of Bench into a window of its own shows no
  pill. Its window's title keeps the count, such as **(3) Inbox**.
- Without Accessibility permission, Bench can't read the Dock's badges. It
  still shows the counts in web pages' titles.
- To turn the counts off, turn off **Show unread counts on tabs and
  sub-tabs** in **Settings ▸ General**, or set `unread_counts = false` in
  `config.toml`. Then Bench doesn't read the Dock, and web pages' sub-tabs
  show their whole titles.

### What the markers mean

| Marker | Where | What it means |
| --- | --- | --- |
| Amber warning sign (⚠) | A sub-tab's icon | The window needs you: its app isn't open or couldn't open, you moved it out of its pane, or it can't shrink to fit. |
| Grey ring (○) | A sub-tab's icon | Bench is opening the app and waiting for its window. |
| Grey circle with an i (ⓘ) | A sub-tab's icon | For information: the window is in full screen, on another desktop, or minimised. |
| Blue bell | Before a sub-tab's title | The terminal or web page needs you: it rang its bell, posted a notification, or finished its command while you couldn't see it. Its tooltip says which. It goes when you show it. See [When a terminal needs you](#when-a-terminal-needs-you). |
| Blue bell, with a number if there's more than one | After a tab's name | That many of the tab's windows need you, as above. |
| Grey pill with a number | Before a sub-tab's title, after the bell | The app's Dock badge, or the count at the start of the page's title: how many unread messages it has. **999+** is more than 999. See [Unread counts](#unread-counts). |
| Grey pill with a number | After a tab's name | The total of the tab's unread counts. |
| Grey dot | Before a sub-tab's title, or after a tab's name | The app's Dock badge has no number, such as a dot: there's something unread. |
| Two overlapping windows | The end of a sub-tab | The window is in other tabs too. See [Show a window in more than one tab](#show-a-window-in-more-than-one-tab). |
| Green camera or microphone | Before a web page's sub-tab title | The page is using the camera or the microphone, as in a call. See [Calls, sound, and video](#calls-sound-and-video). |
| Grey camera or microphone, struck through | Before a web page's sub-tab title | The page has the camera or the microphone, but you've turned it off. |
| Camera or microphone | After a tab's name | A page in the tab is using the camera or the microphone: a green camera or microphone for the one that's on, or a grey one while they're all off. |
| **Needs more room** | A pane's header | The window it shows can't shrink to fit the pane. |
| **Not secure** or **Other site** | A web page's pane header | See [Where a web page is](#where-a-web-page-is). |
| Orange underline | A tab | The current tab. |
| Orange outline | A tab, a sub-tab, or another control | It has the keyboard focus. On a tab, a sub-tab, or a button of the layout control, the outline is inside its edges. See [Keyboard shortcuts](#keyboard-shortcuts). |
| Orange border | A pane | The focused pane: what it shows gets the keyboard. |
| Raised background with an outline | A sub-tab, or the layout control | The window that its pane shows, or the layout you chose. |
| ⚠ ***N* windows need attention** | The status bar, or the title bar as ***N* need attention** while the status bar is hidden (only ⚠ *N* in a window narrower than 1,000 points) | Windows in the current tab that need you. Click it to list them. |
| ⚠ A message | The status bar, or the title bar as ***N* notices** while the status bar is hidden (only ⚠ *N* in a window narrower than 1,000 points) | A problem with Bench itself, such as with `config.toml`, a shortcut, or a permission. Click it to list them all. |

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
  **⋯** menu instead, under **Closed Windows**: **Open** *app*, and
  **Remove** *app* **from This Pane**.
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
- **One pane zoomed**, while a pane fills the tab on its own. Click it to
  show all the panes again. See [Zoom a pane](#zoom-a-pane).
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
  changed while Bench was running, or **Use this instead** and **Keyboard
  settings** for a shortcut that doesn't work. Tab and the arrow keys move
  between the buttons. Below them are the
  [recent messages](#recent-messages). With no problems, a speech bubble
  button opens the recent messages instead.
- Reminders of the main keyboard shortcuts, when there's room. Click them
  for the [Keyboard Shortcuts](#keyboard-shortcuts) sheet, which lists them
  all.

When there isn't room for all of it, the shortcut reminders go first. Then
the message about a problem is cut short, and then the layout.

To give the panes its height, untick **Show status bar** in
[Settings](#settings). Without it:

- The windows that need attention, and the problems with Bench itself, are
  amber buttons in the title bar, beside Jump to…: ***N* need attention**
  and ***N* notices**. Each opens the same list as in the status bar. In
  a window narrower than 1,000 points, each shows only its number. Its
  tooltip has the rest.
- Messages, such as **Saved settings**, show briefly in the middle of the
  title bar's empty space, or over the tabs when that's too narrow, for 4
  seconds at most. Clicks go through a message: the title bar still moves
  the window under it, and the tabs still switch. A message with a button,
  such as **Undo**, shows for 10 seconds, or 4 if it's over the tabs, and
  its button takes your click. It stays while the pointer is over it, as
  it does over the status bar.
- Why the layout is in use is in the layout control's tooltip.
- **Help ▸ Recent Messages**, or **Recent Messages** in the jump palette,
  shows the [recent messages](#recent-messages).

### Recent messages

Messages such as **Saved settings** go after a few seconds, and only one
shows at a time. Bench keeps the last 20, with the problems with Bench
itself that have gone, so that you can read one again, all of it, or copy
it into a bug report.

To see them, open the list of problems from the status bar, or the speech
bubble button there when there are no problems, or choose **Help ▸ Recent
Messages**, or **Recent Messages** in the jump palette. The list shows the
latest first, each with the time it came and its whole text, which you can
select. A message that came several times in a row says how many. **Copy**
puts a message on the clipboard. Tab and the arrow keys move between the
buttons, and Escape closes the list.

A message that had **Undo** keeps it in the list while its change is still
the latest that Bench can undo, so you can take the change back after the
message has gone. A message that comes while the list is open doesn't
cover the list: it shows at the top of the list instead.

Bench keeps the messages while its window is open: they go when Bench
quits. Its [log](#benchs-files) keeps the problems for longer.

## Move windows

There are three ways to move a window. They all do the same thing.

**Drag its sub-tab:**

| Drop it on | What happens |
| --- | --- |
| A pane's sub-tabs, or anywhere in its header | The window goes between two sub-tabs, where a line shows, in that pane. Along its own pane, this puts the sub-tabs in another order. It never splits the pane. |
| The middle of a pane, below its header | The window joins that pane, and shows. |
| The outer quarter of a pane, below its header, on any side | The pane splits in that direction, and the window goes into the new half. |
| A tab in the title bar | The window moves to that tab's focused pane. With Option held, it stays here too: see [Show a window in more than one tab](#show-a-window-in-more-than-one-tab). |
| The **+** after the tabs, or just to its right | The window moves to a new tab of its own, named after its app. With Option held, it stays here too. See [Give a window a tab of its own](#give-a-window-a-tab-of-its-own). |

While you drag, Bench shows where the window lands: a line between two
sub-tabs, half a pane for an edge, the whole pane for the middle, or a
tinted tab or **+**. The **+** has a dashed outline while you drag, to
show that you can drop there. Over a pane's header, Bench never offers a
split: splits are at the edges of the pane's body. When a tab shows one
pane with sub-tabs on a small screen, its sub-tabs are in the title bar,
and you put them in order there in the same way. A window that's alone in
its pane can go among another pane's windows there: its pane stays,
empty, so the tab keeps its panes in the wide layout, and the message says
so. To reach a tab or sub-tab that's out of view, hold the sub-tab near
either end of its strip, which scrolls. To cancel a drag, press Escape.

**Use the pane's buttons.** **Show side by side** splits the pane to the
right with the shown window. The **⋯** menu has the following, in groups.
Right-click a sub-tab for its window's menu, or an empty part of a pane's
header for the shown window's: the menu opens where you click. The menu
uses the same names as the menu bar's **Pane** menu.

The first group has what you use most:

- **Reload Page**, for a web page.
- **Rename…**, for a terminal or web page, and **Use Automatic Title**, for
  one you've named: see
  [Name a terminal or web page](#name-a-terminal-or-web-page).
- **Close Window**, **Close Page**, or **Close Terminal**. For a window
  that's in other tabs too, **Remove from This Tab** and **Close
  Everywhere**. See [Close windows and tabs](#close-windows-and-tabs).

Then:

- **Split Right** and **Split Down**.
- **Split Right with an Empty Pane** and **Split Down with an Empty Pane**,
  which put a new, empty pane beside this one and open the
  [add dialog](#add-to-a-pane) for it at once. If you close the dialog
  without adding anything, the empty pane goes again.
- **Zoom Pane**, or **Show All Panes** while a pane is zoomed: see
  [Zoom a pane](#zoom-a-pane).
- **Move Left** and **Move Right**, which move the sub-tab one place along
  its pane's sub-tabs.
- **Move to Pane**, listing each other pane as **Pane with** *window*, by
  its shown window, or **Empty Pane**.
- **Move to Tab ▸**, a submenu that lists every tab, and **New Tab**. The tab
  you're in is dimmed.
- **Also Show In ▸**, a submenu that lists every tab, and **New Tab**. The
  tabs that the window is in already are dimmed. See
  [Show a window in more than one tab](#show-a-window-in-more-than-one-tab).
- **Open** *app* and **Remove** *app* **from This Pane**, under **Closed
  Windows**, for each app whose windows the pane keeps places for: see
  [When an app quits](#when-an-app-quits).
- For a web page, its **Page** controls: see [Page controls](#page-controls).
- **New Terminal in** *folder*, for a terminal: a new terminal beside it,
  in the folder its shell is in.
- **Dock Again**, for a window you dragged out of its pane.
- **Show Again**, for a window you minimised: see
  [When you minimise a window](#when-you-minimise-a-window).
- **Open in Browser**, for a web page: see
  [Where a web page is](#where-a-web-page-is).
- **Pop Out of Bench**, or **Remove from Bench** for an app's sub-tab that
  has no window.
- For a web page, **Use the** *name* **Web App from** *browser*, or **Make a
  Web App in** *browser*…, which put a web app of the page's site in its
  place. See [Switch a web page to a web app](#switch-a-web-page-to-a-web-app).

The menu opens with no item chosen, as a Mac menu does, so that pressing
Return twice can't close the window. Press Down Arrow to choose the first
item, or Up Arrow to choose the last.

To open a submenu, such as **Move to Tab**, hold the pointer over it, or
click it. Move the pointer straight to the submenu: it stays open while
the pointer crosses the items between. With the keyboard, press Right
Arrow, Return, or Space on the item. Left Arrow or Escape closes the
submenu again.

**Use the keyboard.** Control+Option+Shift+arrow moves the focused pane's
shown window into the neighbouring pane in that direction, or splits the pane
at that edge if there's no neighbour.

To send the focused pane's shown window to another tab, press
Control+Option+Shift and the tab's number, 1 to 9. To send it to the tab
you were on before this one, press Control+Option+Shift+&#96;. The key is
the one left of 1. The window goes into that tab's focused pane, and you
stay on this tab. A message says which tab the window went to. Click
**Undo** to put it back, or **Go there** to switch to that tab. **Move to
Tab** in the window's **⋯** menu and **Pane ▸ Move to Tab** show these
shortcuts, and do the same.

Things to know:

- A pane can't be split by the only window it shows. Bench explains why
  instead of doing nothing: add another window first, or drag one in, or
  use **Split Right with an Empty Pane** and add a window to the new pane.
- Moving a window to another pane makes the tab's layout custom. Tab
  properties says so, and choosing a layout there starts again from it,
  keeping the windows in order. Putting a pane's sub-tabs in another order
  doesn't.
- When a tab shows one pane with sub-tabs in the compact layout, there
  are no splits. In their place, the **⋯** menu has **Show Side by Side
  (Wide Layout)**, which changes to the wide layout, with **Undo** (see
  [Small screens](#small-screens-auto-wide-and-compact)).
  Control+Option+Shift+Right Arrow does the same there. The other arrows
  don't move the window. Moving to another tab still works.
- **Pop Out of Bench** returns an App window to a normal, free-floating
  window, and gives a Web or Built-in item a window of its own. The item then
  appears under **Not in a tab** in the jump palette, and under **Windows
  not in a tab** in the add dialog. Closing a popped-out web page's or
  terminal's window closes the item, as closing an App window does: a
  terminal's session ends. If the terminal is running something, Bench
  asks first, in the terminal's window: see
  [When Bench quits](#when-bench-quits). A web page or terminal that still
  has its own window when Bench quits gets it again when Bench next opens.
  To bring a popped-out window back, see
  [Bring a window into Bench](#bring-a-window-into-bench).
- **Remove from Bench** takes an app's sub-tab that has no window out of
  every tab, for example one that's waiting while Bench opens its app.
  Bench stops opening the app for it. The sub-tab of an app that has quit
  is hidden: to forget it, use **Remove** *app* **from This Pane** (see
  [When an app quits](#when-an-app-quits)). If `config.toml` lists the app
  in the tab, it stays listed until you next save the tab in
  [Tab properties](#tab-properties), and **Reset Layout** brings it back
  until then.

### Bring a window into Bench

A window that isn't in a tab can come into a pane. That's one you popped
out, one that opened somewhere other than a pane, or one you dragged out
of its pane. There are three ways:

- **Drag it onto a pane.** Drag the window by its title bar over one of
  Bench's panes, and hold it still there for a moment. A guide appears over
  the pane: a target in the middle, and, if the pane has another window in
  it and can be split, one near each edge, with a label that says what a
  drop does. Drop the window in
  the middle to add it to the pane as a sub-tab, or near an edge to split
  the pane and give the window the new half. Keep moving, or move off the
  pane, and the guide goes. Popped-out web pages and terminals can be
  dragged back in the same way.
- **Add the front window.** Click the window, then open
  [Jump to…](#the-jump-palette) and choose **Add Front Window**, or choose
  **Pane ▸ Add Front Window** in the menu bar. It adds
  the window you used last before Bench to the focused pane. You can give
  it a shortcut in **Settings ▸ Keyboard**, to do it without leaving the
  window: see [Change a shortcut](#change-a-shortcut).
- **Back to Bench.** A popped-out web page has a **Back to Bench** button
  at the right of its title bar. A popped-out terminal's window has one in
  the bar along its top, and in its right-click menu. While a popped-out
  window has the keyboard, **Window ▸ Back to Bench** in the menu bar does
  the same. The window goes into the focused pane of the tab you're in.

A message then says so. Its **Pop out** takes the window out again. For a
window you dragged out of a pane, the message has **Undo**, which puts it
back where it was, and dropping it on its own pane again just docks it
there. A window shown in more than one tab keeps its places in the others.

Things to know:

- A drag that changes the window's size, from its edge or corner, never
  adds it. Dragging a window that's in a pane by its title bar moves it
  out of the pane, as before. Hold it over another pane to move it there,
  or over its own to dock it again. Dragging the edge of a window that's
  in a pane resizes the panes instead, if the edge faces another pane:
  see [Panes](#panes).
- Windows that a rule keeps floating (`float = true`, see
  [Rules](#rules)), and panels, dialogs, and windows in full screen, are
  left alone.
- Drags add nothing while Bench is hidden or on another desktop (Space),
  where you can't see its panes, or while a menu or dialog is open in
  Bench.
- To turn dragging off, untick **Drag windows onto Bench to add them** in
  [Settings](#settings) (`drag_to_add`). Bench sees where windows are
  dragged by watching the mouse button, which needs no permission of its
  own; moving them into panes needs Accessibility, as everything else
  Bench does with windows.

### Give a window a tab of its own

To move a window to a new tab, do one of these:

- Choose **Move to Tab ▸ New Tab** in the window's **⋯** menu, or
  right-click its sub-tab and choose it there.
- Choose **Move to Tab ▸ New Tab** in the [jump palette](#the-jump-palette)'s
  **This window** group, for the focused pane's shown window.
- Drag the window's sub-tab onto the **+** after the tabs, or just to its
  right. The **+** is tinted while the window is over it.

Bench adds a tab after the others, with the single layout, named after the
window's app, such as **Terminal**. If a tab has that name already, the new
one gets a number: **Terminal (1)**, then **Terminal (2)**. Bench moves the
window there, switches to the new tab, and saves it in `config.toml`,
keeping the previous version as `config.toml.bak`. The tab the window came
from keeps its other windows, and its layout becomes custom.

**Also Show In ▸ New Tab**, in the same places, or holding Option as you drop
the sub-tab, leaves the window where it is too: see
[Show a window in more than one tab](#show-a-window-in-more-than-one-tab).

The message has **Undo**, which deletes the new tab, from `config.toml`
too, and puts the window back where it was.

## Show a window in more than one tab

One window can be in several tabs. For example, your email can sit beside a
browser in one tab and beside Obsidian in another. A window can only be in
one place at a time, so Bench docks it in its pane in whichever of those
tabs you switch to.

To show a window in another tab as well, do one of these:

- In the window's **⋯** menu, choose **Also Show In**, then the tab, such
  as **Research**. The window joins that tab's focused pane.
- Hold Option while you drag the window's sub-tab onto a tab. A **+**
  appears on the tab and on the sub-tab you're dragging.
- Choose **Also Show In ▸ New Tab** in the window's **⋯** menu or the
  [jump palette](#the-jump-palette), or hold Option while you drop its
  sub-tab on the **+** after the tabs, for a new tab named after its app:
  see [Give a window a tab of its own](#give-a-window-a-tab-of-its-own).
- Switch to the other tab, click a pane's **+**, and choose the window under
  **Windows in other tabs**. The window joins that pane.
- When you make a tab of the windows that are open, tick the window under
  **Windows in other tabs**. See
  [Make a tab of the windows that are open](#make-a-tab-of-the-windows-that-are-open).

A window that's in more than one tab has a mark after the title on its
sub-tab. Hold the pointer over the mark to see which other tabs it's in.

To take the window out of one tab, switch to that tab, and click the **×**
on its sub-tab, or choose **Remove from This Tab** in the window's **⋯**
menu. The window stays in its other tabs.

Things to know:

- A window can be in each tab once.
- Dragging the window to another pane, or splitting a pane with it, moves it
  only in the tab you're in. **Move to Tab**, and Control+Option+Shift with
  a tab's number, take it out of the tab you're in. If it's in that tab
  already, it stays where it is there.
- Closing the window, with **Close Everywhere** in its **⋯** menu or in its
  app, or choosing **Pop Out of Bench**, takes it out of every tab.
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
  one, and the others show one when you hold the pointer over them, or
  move the keyboard focus to them. The **×** isn't a stop for the Tab key.
- Move the keyboard focus to its sub-tab and press Delete. It does what the
  **×** does. See
  [Use the keyboard in Bench's window](#use-the-keyboard-in-benchs-window).
- In a terminal, press ⌘W. It does what the **×** does. In a terminal's own
  window, ⌘W closes the window.
- Middle-click its sub-tab.
- Choose **Close Window**, **Close Page**, or **Close Terminal** in its
  **⋯** menu, or right-click its sub-tab.
- Choose the same in the menu bar's **Pane** menu, or in the
  [jump palette](#the-jump-palette), for the focused pane's shown window.

Bench closes an App window as its red close button does, so the app can
ask about unsaved changes as usual. A web page closes at once, and so does
a terminal, unless it's running something: then Bench asks first, as it
does before quitting (see [When Bench quits](#when-bench-quits)). The
message after closing a web page or a terminal has **Undo**, which opens
it again in its place: see
[Undo, and reopen what you closed](#undo-and-reopen-what-you-closed).

The **×** and a middle-click don't close a window that's in other tabs
too: they take it out of this tab only, as **Remove from This Tab** does,
and the **×**'s tooltip says so. To close it, choose **Close Everywhere** in
its **⋯** menu. On a sub-tab with no window, which only keeps the place of
an app's window, the **×** removes it, as **Remove from Bench** does.

To close a tab and everything in it, do one of these:

- Click the **×** at the right end of the tab, in its shortcut's place, or
  where the shortcut would be on a tab that doesn't show one. The current
  tab has one, and the others show one when you hold the pointer over them,
  or move the keyboard focus to them.
- Move the keyboard focus to the tab and press Delete.
- Middle-click the tab.
- Choose **Close Tab…** in the tab's right-click menu.
- Choose **Close Tab…** in the menu bar's **Tab** menu, or in the jump
  palette, for the current tab.

Bench closes the tab's windows, web pages, and terminals, other than those
that are in other tabs too, which stay there. It deletes the tab from
`config.toml`, with any [rules](#rules) that send windows to it, so the tab
doesn't come back when Bench next opens. Before it closes two or more
things, or a terminal that's running something, Bench asks, in one
sentence that says what it will close:

> Close Plan and its 2 windows and 1 terminal?

A terminal that's running something is named below the question.
**Details** shows the rest: what stays in other tabs, that apps can still
ask about unsaved changes, that the tab goes from `config.toml`, and what
**Undo** brings back.

**Close tab** closes, and **Cancel**, Escape, or a click outside the
question keeps everything. When other apps' windows will close, **Close
tab** isn't the default button, so Return doesn't close: an app could
lose unsaved work. Click **Close tab**, or press Shift-Tab to move to it,
then Space. Tick **Don't ask again** to stop Bench asking;
**Ask before closing a tab's windows** in [Settings](#settings) turns it
back on. With it off, Bench still asks before it ends a terminal that's
running something, as long as **Ask before quitting while a terminal is
running something** is on; that question's **Don't ask again** turns that
setting off instead.

Things to know:

- On the only tab, **Close Tab…** is **Close All Windows…**. It closes the
  tab's windows, forgets the places of apps that have quit, and leaves the
  tab, empty. What `config.toml` lists for the tab doesn't open again when
  Bench next opens; **Reset Layout** brings it back.
- The message after closing a tab has **Undo**, which brings back the tab
  with what's still open, such as a window whose app is asking about
  unsaved changes, but not the windows, pages, and terminals that closed.
  **Reopen Closed Tab** brings back the tab later, with its pages and
  terminals: see [Undo, and reopen what you closed](#undo-and-reopen-what-you-closed).
- To delete a tab without closing its windows, choose **Delete Tab (Keep
  Windows)** in its right-click menu, or **Delete tab** in its
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
  Window** is turned off, and the menu says why.
- If an app's window has no close button, Bench chooses the app's **Close
  Window** or **Close** menu command instead. Bench can't close a window
  that's on another desktop, and says so.
- A web page closes without asking, even one that would ask before you
  leave it, such as a form with unsent text. Undo and **Reopen Closed
  Page** open its address again, not the text you typed.
- Bench has no keyboard shortcut for closing. In an App window,
  Command+W closes the window as usual, and its sub-tab goes.

## Undo, and reopen what you closed

After you change the arrangement, a message says what changed, over the
status bar, or in the title bar while the status bar is hidden, and for
some changes it has **Undo**:

- Deleting a tab, and closing one.
- Closing a web page or a terminal, with its **×**, its menu, or ⌘W.
- **Reset Layout**.
- **Close Pane**, and an empty pane's **Close this pane** button.
- Moving a window, by dragging it, from its menu, or to another tab with
  the keyboard.
- Moving a tab, by dragging it or with **Move Tab Left** and **Move Tab
  Right**.
  Undo puts it back in its place in `config.toml` too.
- **Also Show In**, **Move to Tab ▸ New Tab**, **Also Show In ▸ New Tab**,
  and **Remove from This Tab**.
- Making a tab of the windows that are open. Undo puts the windows back
  where they were: see
  [Make a tab of the windows that are open](#make-a-tab-of-the-windows-that-are-open).
- **Show Side by Side (Wide Layout)**, in a tab shown as one pane with
  sub-tabs. Undo also puts back the layout you had, such as Auto: see
  [Small screens](#small-screens-auto-wide-and-compact).
- Making panes the same size, with a double-click on the gap between them.
- Resizing panes by dragging a window's edge: see [Panes](#panes).
- **Grow Pane**, **Shrink Pane**, **Make Panes Equal**, **Next Layout**,
  and **Previous Layout**. Some of these one after the other are one
  change: see
  [Arrange panes from the keyboard](#arrange-panes-from-the-keyboard).

The message stays for 10 seconds, and longer while the pointer is over it
or the keyboard is in it: it goes 3 seconds after you move away, or when
its time is up, whichever is later. Click **Undo** to put things back as
they were before that change.

After the message has gone, you can still undo the latest change:

- Choose **Tab ▸ Undo** in the menu bar. It names the change, such as
  **Undo Close Tab**, and is dimmed when there's nothing to undo. **Edit ▸
  Undo** stays with the text field, page, or terminal you're in.
- Choose **Undo** in the [jump palette](#the-jump-palette). It's first,
  with what the change did, such as "Closed Docs", but Return with nothing
  typed still goes to the tab you were on before: press Up Arrow to reach
  **Undo**, or type "undo".
- Click **Undo** beside the change's message in the
  [recent messages](#recent-messages).
- Press the `undo` shortcut, from any app. It's off until you set it in
  **Settings ▸ Keyboard**, as **Undo the latest change**. Don't give it
  ⌘Z: Bench would take ⌘Z from every app. If Bench is hidden, it comes
  back to show what Undo put back, or that there's nothing to undo.

Each of these takes back only the latest change, and the message says
what it took back, such as "Undid: closed Docs." Press it again for the
change before that. Things to know:

- Undo puts back the tabs, their panes and sizes, and which windows are in
  them. After **Show Side by Side (Wide Layout)**, it also puts back the
  layout you had. A deleted tab comes back where it was, with its entry in
  `config.toml` and the [rules](#rules) that send windows to it, and its
  windows dock in it again.
- Undo after closing a tab brings back the tab and what's still open, not
  the windows, pages, and terminals that closed. If an app was asking about
  unsaved changes, it still is: answer it in the app.
- Undo after closing a web page or a terminal opens it again in its place:
  its pane, at its place among the sub-tabs. A web page opens at the
  address it had. A terminal starts again in its folder, with its
  command. While **Keep terminal output** is on, it shows the output it
  had first, as when Bench opens again. Closing an App window can't be
  undone: the app closed it.
- Undo also takes back the changes you made after that one. Something that
  closed since stays closed, and a window that came since stays where it
  is.
- **Grow Pane**, **Shrink Pane**, **Make Panes Equal**, **Next Layout**,
  and **Previous Layout** one after the other are one change. If you used
  the same one each time, on the same pane, Undo has its name, such as
  **Undo Grow Pane**. If not, it's **Undo Change Layout** when one of them changed the layout,
  and **Undo Resize Panes** when none did. Undo takes them all back. A
  drag of a window's edge that resized panes is **Undo Resize Panes** too.
- Bench can undo only its last 10 changes, and none from before you added
  a tab in Tab properties, renamed or reopened a tab, or reloaded
  `config.toml`. Then it says it can't undo that any more. A tab that
  **Move to Tab ▸ New Tab** added, or a tab of the windows that are open,
  doesn't count: Undo takes it away again.
- Command+Z stays with the page or app you're in. The `undo` shortcut is
  off until you set it, for that reason. There's no Redo.

Bench keeps the last 5 tabs you deleted or closed, even after it quits. To
bring one back, choose **Reopen Closed Tab** from any tab's right-click
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

Bench also keeps the last 10 web pages and terminals you closed, until it
quits. To open one again, choose **Tab ▸ Reopen Closed Page** (or **Reopen
Closed Terminal**) in the menu bar, or the same in any tab's right-click
menu, for the one you closed last, or type "reopen" in the jump palette,
which lists them all, with the tab each was in. It opens where it was, as
Undo opens it, and Bench switches to its tab:

- If its pane has gone, as a pane does when you close the last thing in
  it, it opens in a pane of its own again, at the size it had, on the
  same side of the pane that was next to it. If that pane has gone too,
  it opens in the tab's focused pane. If its tab has gone, it opens in
  the current tab's focused pane.
- If it's open again already, such as after **Reopen Closed Tab**, Bench
  doesn't offer to reopen it any more.
- A page or terminal that you closed while it was popped out of Bench
  isn't kept.
- Bench keeps them in memory only. A terminal's output isn't written to
  disk for this, and Bench forgets it when you turn **Keep terminal
  output** off.

## Add to a pane

Click a pane's **+** to open the add dialog. Type to search, then choose a
row. The dialog finds what you type as you type it, not word by word as the
jump palette does, so that a command such as `code .` isn't taken for a
window. Return chooses the first row that matches what you type, and your
[launchers](#launchers) come first:

| Group | What it adds |
| --- | --- |
| **Launchers** | Your [launchers](#launchers): a terminal that runs a command, such as `claude`, or an app's window on a folder or link. Type a launcher's name to find it. Each time you choose one, it adds a new one. |
| **Windows not in a tab** | An open window that Bench isn't managing. To add an app that the dialog doesn't list, open it from the Finder, Spotlight, or the Dock: its windows show up here. |
| **Windows in other tabs** | A window, web page, or terminal from another tab, to show in this pane as well. It stays in its other tabs. |
| **New window** | A new window of an app that already has a sub-tab in this tab. |
| **Built in** | **New terminal in *folder***, which comes first when this pane, or else the focused pane, shows a terminal, and starts your shell in the folder that terminal's shell is in, unless that's your home folder; **New terminal**, your shell in your home folder; **New terminal in…**, which asks for a folder with macOS's folder picker, starting in that same folder; **New terminal…**, which asks for a command, a name, and a folder (see below); **Web page…**, which asks for a website's `https` address (Bench adds `https://` if you leave it out) and adds the site as a web app where it can, or else as a web page that Bench shows itself (see [Web apps](#web-apps)). A page on this Mac, such as `localhost:3000`, can be `http`, and Bench adds it as a web page (see [Pages on this Mac](#pages-on-this-mac)); or **Browse…**, which finds an app, or a program or script, on your Mac (see below). |
| **Apps** | Any installed app other than a web app, from `/Applications`, `~/Applications`, and `/System/Applications`, including their `Utilities` folders, and the folders one level inside `/Applications`, such as `/Applications/Setapp`. Bench opens the app if it isn't running, and adds its windows that aren't in a tab, or opens a new window if it has none. An app that Bench opens can take a while to open its windows, and some of them can belong to other sub-tabs: Bench opens a new window only if none of the app's windows comes to the pane, or if it opens none within `launch_timeout`. |
| **Web apps** | Websites that a browser has made into apps of their own (see [Web apps](#web-apps)), with the browser that made each. They work like any other app. |

To add more than one window at once, tick them. Each row under **Windows
not in a tab** and **Windows in other tabs** has a tick box at its left:

1. Click the tick box of each window that you want. Or move to its row with
   the arrow keys, and press Shift-Return.
2. Click **Add *N* windows**, below the list, or press Return.

Bench adds the windows to the pane in the order of the list, and the pane
shows the first. While windows are ticked, a click on a window's row ticks
or unticks it, and Return adds the windows ticked, not the selected row.
With no window ticked, a click on a row adds only that row. If a window
closed, or is in the tab already, Bench adds the others and says how many
it added.

If what you type matches nothing, the dialog says so, and offers **Run
“*what you typed*” in a new terminal…**, which opens **New terminal…** with
it as the command.

### New terminal…

**New terminal…** asks for:

- **Command**: what to run, such as `claude --continue` or `npm run dev`.
  Bench starts your shell as Terminal does, as an interactive login shell,
  and types the command into it, so that it finds the same programs,
  aliases, and functions as in your own terminal. When the command
  finishes, you're back at the shell's prompt, in the folder. Leave it
  empty for just the shell.
- **Name**: the sub-tab's name, such as **Claude**. Leave it empty for the
  title that the terminal sets.
- **Folder**: where the terminal starts, such as `~/code/bench`. **Choose…**
  opens the folder picker. Leave it empty for your home folder.
- **Start again when Bench opens**, ticked unless you untick it: after
  Bench restarts, the terminal runs the command again. Untick it for a
  command to run once, such as a deploy: Bench then types the command but
  doesn't press Return, so that you can run it when you're ready.
- **Save as launcher**: keep the terminal as a [launcher](#launchers), to
  add again with one choice.

Return, or **Add**, adds the terminal to the pane. Escape, or **Back**, goes
back to the list. Bench keeps the terminal, with its command, in
`state.json`, as it keeps the rest of the arrangement (see
[Bench's files](#benchs-files)).

### Browse…

**Browse…** finds something that the dialog doesn't list. The dialog
closes, and macOS's open panel opens in front of Bench, in `/Applications`
the first time, and after that in the folder of what you last chose. Choose
one of these:

- **An app**, anywhere on your Mac, such as one in a folder of its own or on
  another disk. Bench adds it to the pane, as the **Apps** rows add one, and
  lists it under **Apps** until Bench quits. If you switch tabs while the
  panel is open, Bench doesn't add it, and says so.
- **A program or script**, to run in a terminal. The dialog opens again at
  **New terminal…**, with the command filled in, and the file's name as the
  name, for you to check, add arguments to, and add, or save as a
  launcher. If your shell finds the program by its name, as when you type
  it, the command is just its name, such as `claude`. Otherwise it's the
  program's path, and the terminal starts in its folder. A script without
  permission to run as a program runs with the program its first line
  names, such as `#!/usr/bin/env python3`, or else the one for its
  extension, such as `sh` for `.sh` and `python3` for `.py`.

If you click **Cancel**, the add dialog opens again. If you click
**Browse…** again while the panel is open, Bench brings the panel to the
front. The browser preview can't look for apps or programs, and says so.

## Launchers

A launcher is a terminal, or an app's window, that you add often. Your
launchers come first in the add dialog, and after your tabs and windows in
the [jump palette](#the-jump-palette). Type a launcher's name to find it.
Each time you choose one, it adds a new terminal, or the app's window, to
the pane. There are two kinds:

- A **terminal launcher** starts your shell in its folder and types its
  command, as **New terminal…** does.
- An **app launcher** opens a folder or a link in an app, and docks the
  window that the app opens for it, such as Zed on `~/code/bench`. If the
  app has that window open already, Bench adds it instead.

To make a terminal launcher, tick **Save as launcher** in **New terminal…**,
or click **New terminal launcher** in **Settings ▸ Launchers**. A launcher
saved without a name, either way, is named after its command's program,
such as `claude`, or its folder, numbered if another launcher has that
name, such as `claude (1)`. You make app launchers in `config.toml`: see
[Launcher entries](#launcher-entries).

### Claude Code and Codex

To have Claude Code and Codex one choice away in `~/code/bench`:

1. Click a pane's **+**, type `claude --continue`, and press Return.
2. Type **Claude** as the name, and `~/code/bench` as the folder, or click
   **Choose…**.
3. Tick **Save as launcher**, and press Return.

Do the same with `codex resume --last` for Codex. In `config.toml`, they
look like this:

```toml
[[launcher]]
name = "Claude"
folder = "~/code/bench"
run = "claude --continue"

[[launcher]]
name = "Codex"
folder = "~/code/bench"
run = "codex resume --last"
```

Closing a terminal, or quitting Bench, ends what it runs.
`claude --continue` and `codex resume --last` pick up the last conversation
in the folder, so that when Bench next opens and starts them again, you
carry on where you were. Bench asks before it ends a terminal that's
running something (see [When Bench quits](#when-bench-quits)).

### Edit launchers

**Settings ▸ Launchers** lists your launchers. **Edit** shows a launcher's
fields, and **Done** hides them again. **Remove** takes it out of the list.
**Save** keeps the changes, and **Cancel** forgets them. You can change an
app launcher's name, folder, link, and window title there, but not its app:
edit `config.toml` for that.

Bench keeps commands as plain text: launchers' in `config.toml` and
`config.toml.bak`, and those of the terminals you add in `state.json`. Don't
put passwords or API keys in a command. Set them in your shell's profile, or
in the tool's own settings, instead.

## Terminals

Bench's terminals run your shell, as Terminal does. These keys work while a
terminal has the keyboard, in Bench's window or a terminal's own window:

| Keys | What they do |
| --- | --- |
| ⌘C and ⌘V | Copy what's selected, and paste. |
| ⌘F | Find text in the terminal and what has scrolled out of it. |
| ⌘G and ⇧⌘G | The next and previous match of what you last looked for. |
| ⌘+ and ⌘− | Make this terminal's text bigger or smaller, until Bench next opens or you change the text size in Settings. |
| ⌘0 | Put this terminal's text back to the size in Settings. |
| ⌘K | Clear the terminal's screen and what has scrolled out of it. Bench forgets that output too, so it doesn't come back when you pop the terminal out, or when Bench next opens. |
| ⌘A | Select everything in the terminal. |
| ⌘T | In Bench's window, open a new terminal in the same pane, in the folder that this terminal's shell is in. See [A new terminal in the same folder](#a-new-terminal-in-the-same-folder). |
| ⌘W | Close the terminal, as the **×** on its sub-tab does. In a terminal's own window, close the window. If the terminal is running something, Bench asks first. |

Right-click a terminal for a menu with **Copy**, **Paste**, **Find…**,
**Clear**, and **Select All**.

Terminals keep text readable in both light and dark colours. If a program
draws text in a colour that's hard to read on the terminal's background,
such as black on dark, the terminal makes it darker or lighter until it
reaches a contrast of 4.5:1, or 7:1 with **Increase contrast** on in
**System Settings ▸ Accessibility ▸ Display**. Faint text in the terminal's
own colour stays fainter than the rest, but you can still read it. Faint
text in a colour that the program chose can be harder to read.

### Find text

⌘F, or **Find…** in the right-click menu, opens a find bar at the top right
of the terminal. As you type, Bench highlights every match, and the bar says
which match is selected and how many there are, such as **2 of 7**, or
**No matches**. Return and the down arrow button go to the next match, and
Shift-Return and the up arrow button to the previous one. Escape, or the ×
button, closes the bar and gives the keyboard back to the terminal.

### Links

Programs can print links in a terminal, and the text a link shows needn't
be its address. Bench also finds plain `http` and `https` addresses in a
terminal's text. Holding the pointer over a link underlines it and shows the
address it opens, with the host in bold. Command-click the link to open it
in your default browser. A plain click doesn't open it, so a click to
select text or to type in the terminal never takes you to a site that a
program's output chose. Only `http` and `https` links open. A host with
letters from other scripts shows in its ASCII form, starting `xn--`, so
letters that look like Latin ones can't pass for them, and a very long
address is cut short, keeping the host.

### Text size and the Option key

**Settings ▸ Terminals** sets the text size of every terminal, from 9 to 24
points, and 12.5 unless you change it. ⌘+ and ⌘− change one terminal's text
for now, and ⌘0 puts it back.

Tick **Use Option as Meta key** for Option to work as Meta, as with
Terminal's setting of the same name: Option-B and Option-F then move by
words in the shell. Option then no longer types its own characters,
such as `#` or € on some keyboards.

### A new terminal in the same folder

A terminal's menu, which opens when you right-click its sub-tab or click
**⋯** while it's shown, has **New terminal in *folder***, and ⌘T in the
terminal does the same. It adds a new terminal to the same pane, starting
in the folder that the terminal's shell is in now, such as `~/code/bench`
after you `cd bench` in `~/code`. The add dialog has it too: see
[Add to a pane](#add-to-a-pane). **New terminal in…** in the add dialog
asks for another folder with macOS's folder picker. The browser preview has
no folder picker, so it opens **New terminal…** at its **Folder** field
instead.

Bench knows the folder from the shell's own process, or, if it can't read
that, from the folder the shell reports, as zsh does in Terminal. A
terminal that hasn't changed folder is still in the one it started in.

### When a terminal needs you

A program in a terminal can ask for your attention: it can ring the bell,
or ask for a notification, as Claude Code and Codex can when they finish or
need an answer. (Programs send notifications with the OSC 9 or OSC 777
escape sequences.) A terminal whose `command` ends, rather than a shell,
asks for it too.

Claude Code and Codex don't do this until you turn it on:

- To have Claude Code ring the bell, set its notification channel to the
  terminal bell, for example with
  `claude config set --global preferredNotifChannel terminal_bell`.
- To have Codex send notifications, add `notifications = true` under
  `[tui]` in `~/.codex/config.toml`.

When a terminal asks for your attention and you can't see it, because its
tab isn't shown, it isn't the sub-tab its pane shows, or the app in front
is neither Bench nor an app with a window in the current tab:

- Its sub-tab gets a blue bell, before its title. Holding the pointer over
  the sub-tab or the bell says why, such as "Rang the bell" or
  "Notification: Claude needs your permission".
- Its tab gets a blue bell too, after its name, with the number of
  windows that need you if there's more than one.
- Bench's icon in the Dock shows how many windows need you.
- Bench posts a notification, which says the terminal's title and its tab.
  Clicking the notification brings Bench forward on the terminal's tab and
  shows the terminal. Bench posts at most one notification for a terminal
  every 5 seconds, and for the bell only when it marks the terminal, not
  again for each bell while it's marked. Untick **Notify me when a terminal
  in another tab needs attention** in [Settings](#settings) to have only
  the bells.

While VoiceOver is on, Bench also says when a terminal rings the bell or
its command finishes, whether you can see the terminal or not. See
[VoiceOver](#voiceover).

The bells go when you show the terminal. While you work in a window docked
in the same tab, such as Zed beside the terminal, you can see the
terminal, so it gets no bell. A terminal in a window of its own has no
sub-tab, so it gets only the notification, while Bench isn't in front. A web page that posts a notification while you can't see it gets
the same bells: see [Alerts and camera requests](#alerts-and-camera-requests).

The bell is blue, and sits beside the icon rather than on it, so you can
tell it from the amber sign on a sub-tab's icon, which marks a window that
can't be shown. See [What the markers mean](#what-the-markers-mean).

### When Bench opens again

When Bench opens again, each terminal that `state.json` brings back starts
again:

- A shell starts in the folder it was last in, such as `~/code/bench`
  after a `cd bench` in `~/code`. If that folder has gone, it starts in the
  terminal's own folder, and if that has gone too, in your home folder. The
  terminal's `folder` in `config.toml` doesn't change, and Bench still
  matches the terminal to its config entry by it.
- A terminal that runs a `command`, or types one into the shell, starts in
  its own folder, as the command expects.

While **Keep terminal output between launches** is on, as it is unless you
turn it off in [Settings](#settings), each terminal also shows what it
showed when Bench quit, up to 256 KB of it. The text is grey where the
program didn't give it a colour, and the program's colours stay. Then a line
says when the output is from, so that you can tell it from the new shell's:

> Restored output from 14:02. The program that wrote it has ended.

The program that wrote it isn't running any more: the new shell's prompt
comes after it. Bench keeps the text and its colours, but not the escape
sequences that would ask the terminal something or change how it works,
so that showing the text again can't send anything to the new shell.

Bench keeps the output in the `terms` folder of its
[files](#benchs-files), readable only by you, until you close the terminal.
Output can hold secrets, such as passwords, keys, or tokens that a program
printed. Press ⌘K in the terminal to clear what it shows, and Bench won't
keep it. Untick **Keep terminal output between launches** to have Bench
remove what it kept, and keep nothing from then on.

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

### Pages on this Mac

A page that a program on your Mac serves, such as a site you're building,
is often plain `http`, at an address such as `localhost:3000`. Bench shows
such a page itself, in a Web item:

- In the add dialog's **Web page…**, type its address, such as
  `localhost:3000` or `127.0.0.1:8080`, and press Return. If you leave out
  the start, Bench adds `http://` for an address on this Mac, and
  `https://` for any other. Bench doesn't look for a web app of the page.
- In `config.toml`, give it as `web = "http://localhost:3000"`.

Bench accepts `http` only for `localhost` and loopback addresses, such as
`127.0.0.1` and `[::1]`. Their pages don't leave your Mac, so the pane's
header doesn't say **Not secure**. Every other address must be `https`: an
`http` page from another computer can be read or changed on its way to
you. This includes names that end in `.localhost`, such as
`app.localhost`: macOS may look them up on the network. If you type one,
the dialog says so under the address, with the `https` address to try.

### Switch a web page to a web app

A web page that you have already, such as Slack and Outlook in the
[example config](#example-config)'s Comms tab, can become a web app. Open the page's **⋯** menu, or right-click
its sub-tab. A moment after the menu opens, it has one of these at the end:

- **Use the** *name* **Web App from** *browser*, such as **Use the Slack
  Web App from Chrome**, if you have a web app of the page's site. Bench
  finds it as **Web page…** does, by the address the page was given.
- **Make a Web App in** *browser*…, if you don't, and your default browser
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

The menu has neither entry for VS Code for the Web, for a page on this Mac,
such as `localhost:3000`, or if you have no web app of the site and Bench
can't have your browser make one: because the browser can't, or because
Bench doesn't have Accessibility permission, which it needs to dock web
apps.

## The jump palette

Press Control+Option+Space, or click **Jump to…** in the title bar. Type to
filter, then use the Up and Down arrows and press Return. Press Escape,
click outside the palette, or press Control+Option+Space again, to close
it. If you were typing in a window docked in Bench when you opened the
palette with its shortcut, that window gets the keyboard back, as a web page
or terminal does. A window of another app, or another of the same app's
windows, doesn't: click it to carry on typing.

Type words from a row's name, or from the grey text after it, in any
order: "zed spec" finds the Zed window titled "spec.md", and "comms" finds
the tab, its windows, and **Move to Tab ▸ Comms**. Capitals and accents
don't matter. Rows whose name starts with what you typed come first, then
rows with a word that does, and the group with the best row comes first.
If nothing has every word, the palette looks again, letting a word have
letters missing, so "termnal" still finds your terminals.

With nothing typed, the tab you were on before this one is first, marked
**last used**, so Return goes back to it, as Control+Option+&#96; does. The
windows come in the order you last used them, the latest first. When
there's a change to undo, **Undo** comes before them, under **Last change**,
but Return with nothing typed passes it by: press Up Arrow to choose it.
Type "undo" to find it. If you type the name of the change, such as
"close page", the command itself comes before its **Undo**.

| Group | What's in it |
| --- | --- |
| **Last change** | **Undo**, with the name of the latest change that Bench can undo, such as **Undo Close Page**, what it did, and the `undo` shortcut, if you set one. See [Undo, and reopen what you closed](#undo-and-reopen-what-you-closed). |
| **Tabs** | Every tab, with its apps and shortcut: the tab you were on before this one first, then the rest in their order along the title bar. |
| **Windows** | Every window in every tab, by title, with its tab and app, the one you used last first. A window in several tabs is listed once for each. Choosing one switches to its tab, focuses its pane, and shows it. |
| **Not in a tab** | Windows that Bench isn't managing. Choosing one adds it to the current tab's focused pane. |
| **Launchers** | Your [launchers](#launchers), found by name. Choosing one adds a new one to the current tab's focused pane. |
| **This window** | What the focused pane's shown window's **⋯** menu has, with the same names and shortcuts, and the window's name after each: **Reload Page** for a web page, **Rename…** for a terminal or web page, **Close Window** (or **Close Page**, **Close Terminal**, or **Remove from This Tab** and **Close Everywhere**; see [Close windows and tabs](#close-windows-and-tabs)), **Split Right** and **Split Down**, **Move Left** and **Move Right** along its sub-tabs, **Move to Pane ▸** another pane, **Move to Tab ▸** another tab or **New Tab**, **Also Show In ▸** another tab or **New Tab** (see [Move windows](#move-windows) and [Give a window a tab of its own](#give-a-window-a-tab-of-its-own)), a web page's **Page ▸** controls, such as **Page ▸ Find in Page…** (see [Page controls](#page-controls)), **Open in Browser** for a web page, **Dock Again**, and **Pop Out of Bench**. Then **Window Options…**, which opens the menu itself (see [More room for your apps](#more-room-for-your-apps)). |
| **Actions** | The commands of the menu bar, with the same names: **Show All Tabs**, which shows every tab at once (see [See every tab at once](#see-every-tab-at-once)), **Rearrange**, **Add Front Window**, when the window you used last before Bench is in no tab, with its name (see [Bring a window into Bench](#bring-a-window-into-bench)), **Zoom Pane**, or **Show All Panes** while a pane is zoomed (see [Zoom a pane](#zoom-a-pane)), **Grow Pane**, **Shrink Pane**, **Make Panes Equal**, and **Next Layout** and **Previous Layout**, with the layout each gives, when they can do something (see [Arrange panes from the keyboard](#arrange-panes-from-the-keyboard)), **Add to Pane…**, **New Terminal…** and **Add Web Page…**, which open the add dialog for the focused pane at its New terminal… form or its web page's address (see [Add to a pane](#add-to-a-pane)), **Close Pane** for an empty pane when the tab has others, **Close Tab…** (**Close All Windows…** on the only tab), which asks first as the tab's **×** does, and **Delete Tab (Keep Windows)** for the current tab (see [Close windows and tabs](#close-windows-and-tabs)), **Fill the Screen** (see [Fill the screen](#fill-the-screen)), **Tab Properties…** for each tab, the current one first, with the tab's name after it, **Move Tab Left** and **Move Tab Right** for the current tab (see [Tabs](#tabs)), **New Tab…**, **New Tab from Open Windows…** (see [Make a tab of the windows that are open](#make-a-tab-of-the-windows-that-are-open)), **Reopen Closed Tab**, for each tab Bench can bring back, and **Reopen Closed Page** and **Reopen Closed Terminal**, for each web page and terminal, with the tab it was in (see [Undo, and reopen what you closed](#undo-and-reopen-what-you-closed)), **Settings…**, **Reload config.toml** (see [Edit the file while Bench runs](#edit-the-file-while-bench-runs)), **Recent Messages** (see [Recent messages](#recent-messages)), **Keyboard Shortcuts** (see [Keyboard shortcuts](#keyboard-shortcuts)), **Show Welcome Guide** (see [Get started](#get-started)), and **Layout ▸ Auto**, **Wide**, and **Compact**. Typing "help" finds the keyboard shortcuts and the welcome guide, and typing "resize" finds **Grow Pane**, **Shrink Pane**, and **Make Panes Equal**. |

## The menu bar

Bench's menus, in the menu bar at the top of the screen while Bench is the
active app, have most of what its right-click menus and the jump palette
have. Their items show Bench's shortcuts after their names, such as **Next
Tab (⌃⌥])**, as you've set them in **Settings ▸ Keyboard**. The shortcuts
work from any app, so the items don't use them themselves.

| Menu | What's in it |
| --- | --- |
| **Bench** | **About Bench**, **Check for Updates…**, which looks for a new version, or in a release without Sparkle opens Bench's latest release on GitHub in your browser (see [Update Bench](#update-bench)), **Acknowledgements**, **Settings…**, **Reload config.toml**, hiding Bench and other apps, and **Quit Bench**. |
| **Edit** | Undo, cut, copy, paste, and select all, for text fields, web pages, and terminals. |
| **View** | **Show All Tabs**, which shows every tab at once (see [See every tab at once](#see-every-tab-at-once)), **Layout ▸ Auto**, **Wide**, and **Compact**, with the one in use ticked (see [Small screens](#small-screens-auto-wide-and-compact)), **Rearrange**, and **Fill the Screen** (see [Fill the screen](#fill-the-screen)). |
| **Tab** | **Undo**, which names the latest change, such as **Undo Close Tab**, and takes it back (see [Undo, and reopen what you closed](#undo-and-reopen-what-you-closed)). For the current tab: **New Tab…**, **New Tab from Open Windows…** (see [Make a tab of the windows that are open](#make-a-tab-of-the-windows-that-are-open)), **Tab Properties…**, **Rename…**, **Next Layout** and **Previous Layout** (see [Arrange panes from the keyboard](#arrange-panes-from-the-keyboard)), **Move Tab Left** and **Move Tab Right**, **Next Tab** and **Previous Tab**, **Last Used Tab**, which goes back to the tab you were on, **Close Tab…** (**Close All Windows…** on the only tab), **Delete Tab (Keep Windows)**, **Reopen Closed Tab**, for the tab closed or deleted last, and **Reopen Closed Page** or **Reopen Closed Terminal**, for the web page or terminal closed last. Then the tabs, with their shortcuts and the current one ticked: choose one to switch to it. See [Tabs](#tabs) and [Close windows and tabs](#close-windows-and-tabs). |
| **Pane** | For the focused pane and the window it shows: **Jump to…**, **Add to Pane…**, **Add Front Window**, which adds the window you used last before Bench, if it's in no tab (see [Bring a window into Bench](#bring-a-window-into-bench)), **Split Right** and **Split Down**, which put the window in a new pane beside the pane, **Zoom Pane** (**Show All Panes** while a pane is zoomed), **Grow Pane**, **Shrink Pane**, and **Make Panes Equal** (see [Arrange panes from the keyboard](#arrange-panes-from-the-keyboard)), **Move to Tab ▸** another tab, with its shortcut, or a new one, **Pop Out of Bench**, **Dock Again** for a window you dragged out of its pane, **Page ▸**, for a web page (see [Page controls](#page-controls)), **Close Window** (**Close Page** or **Close Terminal** for a web page or terminal, and **Remove from This Tab** for a window that's in other tabs too), and **Close Pane** for an empty pane. An item that wouldn't do anything is dimmed. See [Move windows](#move-windows) and [Zoom a pane](#zoom-a-pane). |
| **Window** | **Minimise**, **Back to Bench**, while one of Bench's popped-out web pages or terminals has the keyboard (see [Bring a window into Bench](#bring-a-window-into-bench)), and Bench's windows. |
| **Help** | A search field, which finds any menu item by name, then **Bench Help**, which opens this guide in your browser, **Troubleshooting**, **Keyboard Shortcuts**, which shows them all (see [Keyboard shortcuts](#keyboard-shortcuts)), **Show Welcome Guide**, the guide that Bench shows when it first opens (see [Get started](#get-started)), **Recent Messages** (see [Recent messages](#recent-messages)), **Show Bench's Files in Finder** (see [Bench's files](#benchs-files)), and **Report a Problem…**. |

**Report a Problem…** opens a new issue on Bench's GitHub page in your
browser, filled in with Bench's version, the version of macOS, your
displays, the layout in use, whether Bench has its permissions, the
problems in the status bar, and where Bench keeps its log. Bench sends
nothing itself: read what it filled in, remove anything you'd rather not
share, say what happened, and submit the issue there. You need a GitHub
account to submit it.

## Tab properties

A tab's properties are its own: its name, its position among the tabs, its
layout, and what it does on small screens. To change them, open the tab's
**Tab properties** in one of these ways:

- Right-click the tab, or Control-click it, and choose **Tab Properties…**.
  This works for any tab, not only the current one. To change only its
  name, double-click the tab instead: see [Tabs](#tabs).
- Choose **Tab ▸ Tab Properties…**, for the current tab.
- Choose **Tab Properties…** in the [jump palette](#the-jump-palette). It
  lists one for each tab, the current one first.

To add a tab, click **+** after the tabs, or choose **New Tab…** from a
tab's right-click menu or the jump palette. **New tab** opens, with the same
fields. Beside its title, **Start with** chooses **An empty layout**, or
**Windows that are open**, which lists the windows to tick and has **Make
tab** in place of **Save**: see
[Make a tab of the windows that are open](#make-a-tab-of-the-windows-that-are-open).

| Field | What it does |
| --- | --- |
| **Name** | The tab's name. Names must be unique, ignoring case, so a name that another tab has is numbered: a second **Code** becomes **Code (1)**, then **Code (2)**. A number in brackets at the end is replaced, not added to. Before you save, a line under the field says what the tab will be called. Renaming works the same way. A new tab starts with a name that's free, such as **New tab (1)**. |
| **Position** | The tab's place among the tabs, such as **1st (⌃⌥1)**, with the shortcut that place has. Shortcuts follow tab order: the first nine tabs get Control+Option+1 to 9, and later tabs have none, so choosing another position moves the tab and changes its shortcut when you save. To use other keys than Control+Option, see [Change a shortcut](#change-a-shortcut). A new tab goes last unless you choose another position. |
| **Layout** | One of six layouts, with a preview of which windows land in which pane. |
| **On small screens** | **Stack panes**, **One pane with sub-tabs**, or **Keep the layout**, for when Bench's window is narrower than the width set in [Settings](#settings). See [Small screens](#small-screens-auto-wide-and-compact). |
| **Shortcut** | The shortcut that the tab's position gives it, such as ⌃⌥2, for the first nine tabs, and where to change the keys: **Settings ▸ Keyboard**. |

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
  the tab's right-click menu too, as **Delete Tab (Keep Windows)**. To
  close the windows as well, use **Close Tab…**: see
  [Close windows and tabs](#close-windows-and-tabs). You can't delete the
  last tab. The message that says so has **Undo**, and **Reopen Closed
  Tab** brings the tab back later: see
  [Undo, and reopen what you closed](#undo-and-reopen-what-you-closed).
- **Reset layout** returns the tab to the layout in its config, undoing your
  dragging and splitting. It's in the tab's right-click menu too, as
  **Reset Layout**. The message that says so has **Undo**.
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
| **General** | **Show unread counts on tabs and sub-tabs** | `unread_counts` |
| **General** | **Check for a new version once a day** | `check_for_updates` |
| **General** | **Ask before quitting while a terminal is running something** | `confirm_quit_busy_terminals` |
| **General** | **Ask before closing a tab's windows** | `confirm_close` |
| **General** | **Menu bar item**: **Icon and tab name**, **Icon only**, or **Hidden** | `menu_bar` |
| **Appearance** | **Show status bar** | `show_status_bar` |
| **Appearance** | **Hide the sub-tab row when a pane holds one app window** | `pane_headers` |
| **Appearance** | **Colours**: **Match macOS**, **Light**, or **Dark** | `appearance` |
| **Appearance** | **Density**: **Automatic**, **Standard**, or **Compact** | `density` |
| **Appearance** | **Space between panes**: **8 pt**, **4 pt**, or **2 pt** | `gap` |
| **Appearance** | **Show shortcuts on tabs**: **Automatic**, **Always**, or **Never** | `tab_shortcut_hints` |
| **Appearance** | **Show scroll bars**: **Automatic**, **Always**, or **Never** | `scroll_bars` |
| **Windows** | **Open apps that aren't running** | `open_missing_apps` |
| **Windows** | **Hide windows that belong to other tabs** | `hide_other_tabs` |
| **Windows** | **Also hide apps that aren't in any tab** | `hide_other_apps` |
| **Windows** | **Show live previews of windows (needs Screen Recording)** | `live_previews` |
| **Windows** | **Drag windows onto Bench to add them** | `drag_to_add` |
| **Windows** | **New windows go to**: **The app's pane**, **The focused pane**, or **Leave floating** | `new_windows` |
| **Windows** | **When another tab's window comes forward**: **Switch to its tab**, **Bring it here**, or **Do nothing** | `follow_focus` |
| **Keyboard** | Each of Bench's shortcuts, with whether it works, **Turn off**, and **Restore defaults** | `[shortcuts]` |
| **Small screens** | **Compact layout when Bench is narrower than** *number* **points** | `compact_below` |
| **Terminals** | **Text size**, from 9 to 24 points | `terminal_font_size` |
| **Terminals** | **Use Option as Meta key** | `terminal_option_as_meta` |
| **Terminals** | **Notify me when a terminal in another tab needs attention** | `notify_terminals` |
| **Terminals** | **Keep terminal output between launches** | `keep_terminal_output` |
| **Launchers** | Your launchers, with **Edit**, **Remove**, and **New terminal launcher** | `[[launcher]]` |
| **Advanced** | **Wait for apps to open**, in seconds | `launch_timeout` |

Things to know:

- **Open Bench at login** isn't in `config.toml`: macOS keeps it. If you
  turned Bench off in **System Settings ▸ General ▸ Login Items** before,
  macOS may need you to allow it there again: Bench opens Login Items, and
  until you turn on Bench there, says so under the tick box, with **Open
  Login Items**.
- **Check for a new version once a day**, off by default, has Bench look
  for a new version on GitHub. See [Update Bench](#update-bench).
- **Open Bench filling the screen** takes effect the next time Bench
  opens. Unticked, Bench opens where you left its window. If the window
  filled the screen, or Bench can't put it back there, Bench opens at its
  usual size, 1,440 by 900 points, in the middle of the display it was last
  on. See
  [Fill the screen](#fill-the-screen).
- **Bring back a hidden Bench with tab shortcuts**, on by default, has the tab
  shortcuts show Bench on the tab you choose. See
  [When you hide Bench](#when-you-hide-bench).
- **Show unread counts on tabs and sub-tabs**, on by default, shows how
  many unread messages your apps and web pages have. Bench reads the badges
  on apps' Dock icons, with Accessibility permission, and the counts at the
  start of web pages' titles. See [Unread counts](#unread-counts).
- **Ask before quitting while a terminal is running something**, on by
  default, has Bench ask before it quits, or closes a terminal, from its
  sub-tab, its own window, or **Close Tab…**, while a terminal is running a
  program, such as a build or `claude`. The questions' **Don't ask again** turns it off;
  tick it here to have Bench ask again. See
  [When Bench quits](#when-bench-quits).
- **Ask before closing a tab's windows**, on by default, has **Close Tab…**
  ask before it closes two or more windows, web pages, or terminals, or a
  terminal that's running something. The question's **Don't ask again**
  turns it off. With it off, **Close Tab…** still asks before it ends a
  terminal that's running something, unless **Ask before quitting while a
  terminal is running something** is off too. See [Close windows and tabs](#close-windows-and-tabs).
- **Menu bar item** chooses what Bench shows in the menu bar.
  **Icon and tab name** shows Bench's icon and up to 16 characters of the
  current tab's name, which is cut short with an ellipsis. After the name,
  it shows the number of unread messages in all your tabs, if there are
  any. **Icon only** saves room, on a small screen or beside the notch.
  **Hidden** removes the item: Settings then reminds you that the jump
  palette's shortcut and Bench's icon in the Dock still bring Bench back.
- Unticking **Show status bar** gives its height to the panes, and moves
  its warnings and messages to the title bar. See
  [The status bar](#the-status-bar).
- **Hide the sub-tab row when a pane holds one app window** is off unless
  you turn it on. See [More room for your apps](#more-room-for-your-apps).
- **Colours** is **Match macOS** unless you change it: Bench is light or
  dark as macOS is, in **System Settings ▸ Appearance**, and changes when
  macOS does, as it can at sunset with **Auto**. **Light** makes Bench
  always light, and **Dark** always dark. If your `config.toml` has
  `appearance = "dark"`, as Bench wrote it while **Dark** was the default,
  Bench stays dark until you choose another. Bench's own windows change at
  once: the main window, popped-out terminals, and **Acknowledgements**.
  The guide that shows where a dragged window goes, and live previews,
  change the next time they show. Web pages and the
  apps in your panes follow macOS whichever you choose, as they would
  outside Bench: a site that has a dark look shows it when macOS is dark.
- Whichever colours you choose, Bench follows **Increase contrast** in
  **System Settings ▸ Accessibility ▸ Display**. With it on, lines are
  stronger, grey text is darker on light and lighter on dark, the focused
  pane has a thicker border, and terminals keep text at a contrast of 7:1
  at least, rather than 4.5:1. See [Terminals](#terminals).
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
- **Also hide apps that aren't in any tab** is off unless you turn it on.
  When you switch tabs, apps with no windows in any tab hide too, as
  **Hide Others** does.
- Turning on **Show live previews of windows** asks macOS for Screen
  Recording permission. Until Bench has it, Settings says how to give it.
  See [Live previews](#live-previews).
- **New windows go to** is **The app's pane** unless you change it: the
  pane with the app's other windows, or the pane the config gives the app,
  and otherwise the focused pane. **The focused pane** puts every new
  window there, and **Leave floating** leaves new windows out of your tabs,
  for you to add where you like. See
  [New windows and links](#new-windows-and-links).
- **When another tab's window comes forward**, as when you click it in the
  Dock or a link opens in it, is **Switch to its tab** unless you change
  it. **Bring it here** moves the window into this tab's focused pane, and
  **Do nothing** leaves Bench on this tab and the window where it is. See
  [New windows and links](#new-windows-and-links).
- **Keyboard** lists Bench's shortcuts and changes them. See
  [Change a shortcut](#change-a-shortcut).
- **Text size** is 12.5 points unless you change it, and applies to every
  terminal at once, in Bench's window and in their own windows. ⌘+ and ⌘−
  change one terminal's text for now. See [Terminals](#terminals).
- **Use Option as Meta key** is off unless you turn it on. See
  [Text size and the Option key](#text-size-and-the-option-key).
- **Notify me when a terminal in another tab needs attention**, on by
  default, posts a notification when a terminal you can't see rings its
  bell, asks for a notification, or finishes its command. Its sub-tab and
  tab get a bell either way. See
  [When a terminal needs you](#when-a-terminal-needs-you).
- **Keep terminal output between launches**, on by default, has each
  terminal show what it last showed when Bench next opens. Output can hold
  secrets: turning it off removes what Bench kept. See
  [When Bench opens again](#when-bench-opens-again).
- **Launchers** lists your launchers: see [Edit launchers](#edit-launchers).
- **Wait for apps to open**, under **Advanced**, is 15 seconds unless you
  change it: how long Bench waits for apps to open their windows, when you
  switch tabs and when you add an app. Give a slow app longer. It also
  sets how long an app's windows go back to their old places after it
  opens: see [When an app quits](#when-an-app-quits). A config file value
  that isn't a whole number of seconds, such as `"1m30s"`, shows as
  seconds, 90.

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

The [Keyboard Shortcuts](#keyboard-shortcuts) sheet, and the questions
Bench asks, such as before quitting, move by their headers too, but have no
grip to resize them.

While a dialog is open, Bench comes in front of the apps in its panes, so
moving a dialog doesn't show the app behind it. Menus, such as a tab's
right-click menu, don't move.

## Small screens: Auto, Wide, and Compact

Bench chooses a wide or a compact layout for the width of its own window.
A window that fills the screen of a 13-inch or 14-inch MacBook gets the
wide layout, with the panes side by side. So does a window at Bench's
usual size, 1,440 points wide. A window that you tile into half of a
MacBook's screen gets the compact layout. The control at the right of the
title bar sets how Bench chooses:

| Choice | What it does |
| --- | --- |
| **Auto** | Compact when Bench's window is narrower than `compact_below` (1,280 points by default, set in [Settings](#settings)), and wide when it is wider. |
| **Wide** | Always the wide layout, until you choose Auto again. |
| **Compact** | Always the compact layout, until you choose Auto again. |

You can also choose them from **View ▸ Layout ▸ Auto**, **Wide**, and
**Compact**, where the one in use is ticked.

When you make the window wider or narrower, Auto chooses again. After it
chooses the compact layout, the window must be 40 points wider than
`compact_below` before Auto chooses the wide layout again. This stops the
layout changing to and fro while you drag the window's edge.

If an earlier version of Bench made your `config.toml`, the file keeps the
default of that version, 1,800 points. Then a window that fills a
MacBook's screen gets the compact layout. To show the panes side by side
on a MacBook, set **Compact layout when Bench is narrower than** in
[Settings](#settings) to 1,280 points.

When Bench's window is narrower than about 1,200 points, or in the compact
layout, the title bar makes more room for the tabs. The control becomes
one button that names the choice, such as **Auto ▾**: click it for a menu
of the three, with the one in use ticked, and why the layout is in use.
Jump to… becomes its magnifying glass, whose tooltip has its shortcut.

The status bar names the layout in use, such as **Auto: wide**. Click it,
or hold the pointer over it or over the control, to see why, such as "Wide
layout, chosen for a window 2,560 pt wide". When a display is connected or
disconnected, or Bench's window moves to another screen, Bench waits for
macOS to settle, then chooses again and rearranges the current tab.

In the compact layout, each tab follows its small-screen setting, which you
choose in its [Tab properties](#tab-properties):

| Setting | In Tab properties | What it does |
| --- | --- | --- |
| `stack` | **Stack panes** | The top-level row becomes a column, so side by side becomes top and bottom. |
| `tabs` | **One pane with sub-tabs** | The tab shows one pane, with every window as a sub-tab, in the title bar (see [More room for your apps](#more-room-for-your-apps)). The layout comes back in the wide layout. |
| `same` | **Keep the layout** | No change. |

In the [example config](#example-config), Code stacks its panes, and Comms
and Research show one pane with every window as a sub-tab.

To see the windows of a tab with one pane side by side, choose **Show side
by side (wide layout)** in a window's **⋯** menu. Bench changes to the
wide layout, which shows the tab's panes, and shows the window in its
pane. The pane's split button in the title bar, the
[jump palette](#the-jump-palette), **Pane ▸ Show Side by Side (Wide
Layout)**, and Control+Option+Shift+Right Arrow do the same. The toast
tells you how to go back, for example: "Changed to the wide layout, to
show Obsidian — Daily note side by side. To go back, choose Auto at the
right of the title bar." To go back, do one of these:

- Choose **Auto**, or **Compact**, at the right of the title bar. In a
  narrow window, the control is one button, such as **Wide ▾**. Click it,
  then choose **Auto**.
- Click **Undo** on the toast.

A tab that has all its windows in one pane shows one pane in the wide
layout too. So Bench also gives the window a pane of its own on the
right, as **Split Right** does. The toast then says
that **Undo** removes the pane. If you choose **Auto** instead, the tab
keeps the new pane.

## Live previews

With live previews on, hovering over an inactive sub-tab for a moment shows
a small, live picture of its window, with its title. In the jump palette,
the window you select or hover over shows its preview beside the palette.

Previews are off until you turn on **Show live previews of windows** in
[Settings](#settings), because they need Screen Recording permission. When
you turn them on, Bench asks macOS for it, and opens **System Settings ▸
Privacy & Security ▸ Screen Recording**: turn on Bench there. macOS applies
the permission only when Bench opens again, so Settings, under the tick
box, and the [welcome guide](#get-started) offer **Quit and Reopen Bench**.
If a terminal is running something, Bench asks first, as it does before
quitting. macOS asks you to approve Screen Recording again from time to
time.

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
4. The add dialog's **Web page…**, or a web page's **Make a Web App in**
   *browser*…, is waiting for your default browser to make a web app, and
   the window is the browser's: it stays floating, under **Windows not in a
   tab** in the add dialog (see [Web apps](#web-apps)).
5. Otherwise, **New windows go to** in [Settings](#settings)
   (`new_windows`) decides. By default, the window joins the pane that
   holds the app's most recently used window, or else the pane the config
   gives the app, or else the current tab's focused pane.

So a rule's `tab` comes before an item's `title`: with a Research item
`{ app = "com.brave.Browser", title = "Docs" }` and a rule sending Brave's
windows to Comms, a new Brave window called "Go Docs" joins Comms.

Dialogs, sheets, settings windows, and floating panels always stay floating
above Bench.

Links and notifications often bring another tab's window forward: a link in
Slack can open in a Brave window that lives in Research. By default, Bench
switches to that tab. **When another tab's window comes forward** in
[Settings](#settings) (`follow_focus`) can instead bring the window into the
current tab, or do nothing.

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

### Page controls

A web page in Bench has no toolbar, and many pages, such as Slack's, put
their own menu in place of the usual one when you right-click them. So
right-click a web page's sub-tab, or click **⋯** while it's shown, for its
**Page** controls:

- **Reload Page**, at the top of the menu, loads the page again.
- The row labelled **Page** has buttons for **Back** and **Forward**, which
  are dimmed when the page has nowhere to go, and **Zoom Out**, **Actual
  Size**, which shows the zoom, such as **125%**, and **Zoom In**. The zoom
  goes from 50% to 300%, and each page keeps its own, even after Bench
  restarts.
- **Find in Page…** opens a find bar at the top of the pane. Type what to
  find and press Return for the next match, or Shift-Return for the one
  before; the page shows it selected. Capitals don't matter, and the search
  goes round to the start at the end. The bar says **No matches** when
  there's none. Press Escape, or click its **×**, to close it, and the page
  has the keyboard again.
- **Copy Page Link** puts the page's address on the clipboard, as **Open in
  Browser** would open it. VS Code for the Web doesn't have it, as its
  address holds the key to its server.
- **Mute Microphone**, **Turn Off Camera**, and **Stop Sound and Video**: see
  [Calls, sound, and video](#calls-sound-and-video).

They're in the [jump palette](#the-jump-palette) too, under **This window**,
such as **Page ▸ Find in Page…**, and in the menu bar under **Pane ▸ Page**,
with the same names. None of
them has a shortcut, so that the page keeps its own, such as ⌘R, ⌘F, and
⌘+. A popped-out page has no find bar: dock it first.

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
| **Other site:** *host* | The page has moved to another site than the item's own, as a redirect, a script on the page, or a sign-in page can take it. Anyone can have a sign-in page with some identity providers. | Check that it's the site you expect before you type a password. To see the whole address, choose **Open in Browser**. |

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

**Open in Browser**, in the pane's **⋯** menu, opens the page in your
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

### Calls, sound, and video

When you switch tabs, or open a menu or dialog, Bench only hides a web
page: a call, its microphone, and any sound carry on. macOS shows that
Bench is using the camera or microphone, but not which page is. So a page
that's using them has a mark of each on its sub-tab, before its title: a
camera and a microphone, green while they're on, and grey and struck
through while you've turned them off. Its tab has a mark after its name
too: a green camera or microphone for the one a page there has on, or a
grey one while they're all off. Hold the pointer over a mark to see which
are on and which are off. A call in a window that the page opened, such
as a meeting's, counts as the page's.

To turn them off without leaving the call, right-click the page's sub-tab,
or click **⋯** while it's shown, and choose **Mute Microphone** or **Turn Off
Camera**. The page can't turn them on again itself: choose **Unmute
Microphone** or **Turn On Camera** when you're ready. **Stop Sound and
Video** pauses everything the page is playing, such as a video, a call's
sound, or music; the page can play it again. Bench can't tell whether a page
is playing sound, so there's no mark for that, and **Stop Sound and Video**
is always there. The [jump palette](#the-jump-palette) and **Pane ▸ Page**
have them too.

## Keyboard shortcuts

These shortcuts work from any app. You can change each one in
**Settings ▸ Keyboard** (see [Change a shortcut](#change-a-shortcut)), or
in the [`[shortcuts]` table](#shortcuts).

| Shortcut | Action | Config setting |
| --- | --- | --- |
| Control+Option+1 to 9 | Switch to tab 1 to 9. | `tab_modifiers` |
| Control+Option+] and Control+Option+[ | Next and previous tab. | `next_tab`, `previous_tab` |
| Control+Option+&#96; | Back to the tab you were on: press it again to come back. The key is the one left of 1. Hold Control+Option after it to choose from every tab: see [Hold to switch tabs](#hold-to-switch-tabs). | `last_tab` |
| Control+Option+Space | Open the jump palette. | `jump` |
| Off until you set it | Show every tab at once, or close the overview. See [See every tab at once](#see-every-tab-at-once). | `show_all_tabs` |
| Control+Option+0 | Rearrange the current tab. | `rearrange` |
| Control+Option+arrow | Focus the pane in that direction. | `focus_pane_modifiers` |
| Control+Option+Shift+arrow | Move the shown window to the pane in that direction, or split there. In a tab shown as one pane with sub-tabs, Right Arrow shows the window side by side, in the wide layout. | `move_window_modifiers` |
| Control+Option+Shift+1 to 9, and Control+Option+Shift+&#96; | Send the shown window to tab 1 to 9, or to the tab you were on before this one. You stay on this tab. See [Move windows](#move-windows). | `send_to_tab_modifiers` |
| Control+Option+Tab, and with Shift | Next and previous sub-tab in the focused pane. | `next_window` |
| Control+Option+Return | Zoom the focused pane, or show all panes again. See [Zoom a pane](#zoom-a-pane). | `zoom_pane` |
| Off until you set it | Make the focused pane 5% larger, or smaller. See [Arrange panes from the keyboard](#arrange-panes-from-the-keyboard). | `grow_pane`, `shrink_pane` |
| Off until you set it | Give all the panes of the current tab the same size. | `equal_panes` |
| Off until you set it | Give the current tab the next, or the previous, layout. | `next_layout`, `previous_layout` |
| Off until you set it | Add the window you used last before Bench to the focused pane, if it's in no tab. See [Bring a window into Bench](#bring-a-window-into-bench). | `add_front_window` |
| Off until you set it | Undo the latest change, from any app, and say what it took back. See [Undo, and reopen what you closed](#undo-and-reopen-what-you-closed). | `undo` |

If another app or macOS already uses a shortcut, or VoiceOver does while
it's on, the status bar says so and suggests another. See
[A shortcut doesn't work](#a-shortcut-doesnt-work).

While Bench is hidden, the tab shortcuts, including the last used tab's,
and the jump palette's bring it back. See
[When you hide Bench](#when-you-hide-bench).

[Bench's menus](#the-menu-bar) show the shortcuts after their items' names.
To see them all as you've set them, choose **Help ▸ Keyboard Shortcuts**,
click the shortcut reminders at the right of the status bar, or choose
**Keyboard Shortcuts** in the jump palette. The sheet lists each of
Bench's shortcuts with its keys, or **Off**, and what's wrong with one that
doesn't work; then, under **In Bench's window**, the keys that work in
Bench's window; then the keys that work in a terminal. **Change
Shortcuts…** opens **Settings ▸ Keyboard**.

In Bench's menus, dialogs, and palette, Escape closes, the arrow keys move
the selection, and Return chooses. When the add dialog asks for a web
address, Option-Return shows the page in Bench, rather than as a web app.

While a terminal has the keyboard, ⌘F finds text, ⌘+, ⌘−, and ⌘0 change its
text size, ⌘K clears it, ⌘A selects everything in it, ⌘T opens a new
terminal in its folder, and ⌘W closes it. See [Terminals](#terminals).

### Use the keyboard in Bench's window

These keys work while Bench's window has the keyboard. You can't change
them.

| Keys | What they do |
| --- | --- |
| ⌘, | Open [Settings](#settings). |
| ⌘T | Open **New tab**, for a new tab. In a terminal, open a new terminal in the same folder instead. |
| ⌘W | In a terminal, close the terminal. In a terminal's own window, close the window. Bench asks first if the terminal is running something. Outside a terminal, ⌘W does nothing: it doesn't close Bench's window. |

If you're renaming a tab or sub-tab, ⌘, and ⌘T save the new name first.
While a dialog is open, such as the jump palette or
[the overview of every tab](#see-every-tab-at-once), ⌘, and ⌘T do nothing.

To move the keyboard focus with the Tab key, turn on **Keyboard
navigation** in **System Settings ▸ Keyboard**. Then Tab and Shift+Tab
move the keyboard focus from one control to the next. The one with the
keyboard focus has an orange outline.

The tabs in the title bar are one stop for the Tab key, and so are each
pane's sub-tabs: Tab goes to the current tab, or to the sub-tab that the
pane shows. Then:

| Keys | What they do |
| --- | --- |
| Left Arrow and Right Arrow | Move the keyboard focus to the previous or next tab or sub-tab, from the last back to the first. The strip scrolls to show it. |
| Home and End | Move the keyboard focus to the first or last tab or sub-tab. |
| Down Arrow | From a tab, go to the sub-tabs of the focused pane. |
| Up Arrow | From a sub-tab, go back to the tab that you came from. |
| Return or Space | Switch to the tab, or show the sub-tab's window. |
| Delete | Close the tab or sub-tab, as its **×** does. Bench asks first where the **×** would. Then the keyboard focus moves to the sub-tab in its place, or to the current tab. |
| Shift+F10 or Control+Return | Open the menu of the tab or sub-tab below it, as a right-click does. Escape closes it again. |

A row of choices, such as a choice in Settings or the layout control at
the right of the title bar, is one stop for the Tab key too. In Settings
and Tab properties, the arrow keys move to the next or previous choice and
choose it. In the layout control, the arrow keys only move the keyboard
focus, because a new layout moves every window. Press Return or Space to
choose the layout.

### Change a shortcut

**Settings ▸ Keyboard** lists Bench's shortcuts: **Tabs 1 to 9**, **Next
tab**, **Previous tab**, **Last used tab**, **Jump to…**, **Show all
tabs**, **Rearrange**, **Focus a pane**, **Move a window**, **Send a window
to a tab**, **Next window**, **Zoom pane**, **Grow pane**, **Shrink pane**,
**Make panes equal**, **Next layout**, **Previous layout**, **Add front
window to this pane**, and **Undo the latest change**. **Show all tabs**
and the last seven are off until you give them keys. Each shows
its keys, such as ⌃⌥Space, and under them whether they work: "Works.",
or what's wrong, with a button to use a shortcut that Bench checked is
free, such as **Use ⌃⌥⌘J**.

To change a shortcut:

1. Click its keys. The field says **Press keys…**, or **Hold
   modifiers…**, and Bench's own shortcuts stop working until you've
   finished, so that you can choose one of them.
2. Press the shortcut you want, such as ⌃⌥J. It needs ⌃, ⌥, or ⌘, so that
   it doesn't take over typing; only the function keys, F1 to F20, can do
   without. For **Tabs 1 to 9**, **Focus a pane**, **Move a window**, and
   **Send a window to a tab**, which take modifiers alone, hold the
   modifiers, such as ⌃⌘, and let go: Bench adds 1 to 9, the arrow keys,
   or 1 to 9 and &#96;. **Next window** with ⇧ added goes to the previous
   window.
3. Click **Save**. Bench uses the new shortcut at once, with no need to
   quit and reopen it.

While a field records, Escape stops without a change, and Delete turns the
shortcut off. **Turn off** does the same. If another of Bench's shortcuts
has the keys already, the line under the field says which one, and **Save**
refuses it until you change one of them. **Restore defaults** puts every
shortcut back as in the table above. **Cancel** keeps
the shortcuts as they were.

A tab's shortcut follows its place among the tabs: to give a tab another
number, change its **Position** in [Tab properties](#tab-properties).

## VoiceOver

Bench works with VoiceOver, the screen reader in macOS. To turn VoiceOver
on or off, press ⌘F5, or use **System Settings ▸ Accessibility ▸
VoiceOver**.

- VoiceOver reads the tabs as tabs: for example, "Code, selected, tab, 2
  of 3". If a tab has unread messages, VoiceOver reads the number after the
  name, such as "Comms, 15 unread". It reads each pane's sub-tabs as tabs
  too, with the title first, then the app, then what's wrong and the
  unread count, such as "zsh — ~/code/bench, Terminal, needs you". See
  [Unread counts](#unread-counts).
- It reads the layout control, and the choices in Tab properties and
  Settings, as radio buttons, and says which one is chosen.
- It reads the jump palette's groups, such as **Tabs** and **Windows**.
- The shortcut reminders at the right of the status bar are a button
  called **Keyboard shortcuts**, which opens the Keyboard Shortcuts sheet.

While VoiceOver is on, VoiceOver can read Bench's terminals, and it reads
new output as it comes. Bench also says:

- When a terminal rings the bell, such as "zsh — ~/code/bench: rang the
  bell."
- When a terminal's command finishes, such as "go test — ~/code/bench: its
  command has finished.", or "go test — ~/code/bench: its command ended
  with an error."

Bench names the terminal by the title on its sub-tab. If the terminal is in
another tab, Bench says which tab too, such as "go test — ~/code/bench in
Code: its command has finished." Bench says each of these at most once
every 5 seconds for one terminal.

They show as a message at the bottom of the window, or in the title bar
while the status bar is hidden. **Help ▸ Recent Messages** keeps them. If
another message shows, Bench waits until you have had time to read it. A
message with a button, such as **Undo**, keeps its button: Bench adds what
it says after the message's text.

VoiceOver uses Control+Option, as Bench's shortcuts do. See
[A shortcut doesn't work](#a-shortcut-doesnt-work) for what Bench
suggests instead.

In the browser preview, the **VoiceOver** button above the window
pretends that VoiceOver is on, so that you can see what changes.

## Fill the screen

Bench opens filling the screen: its window covers everything below the
menu bar and beside the Dock. It opens on the display it was on last time,
if that display is connected, and otherwise on the main display. If you
made the window smaller last time, Bench opens it where you left it (see
below). Bench doesn't use native full screen, because other apps' windows
can't follow it into a full-screen space.

To fill the screen again after making the window smaller, choose **View ▸
Fill the Screen**, or **Fill the Screen** in the
[jump palette](#the-jump-palette). If the window fills the screen already,
it stays as it is. The green button at the top left zooms the window too,
and so does double-clicking an empty part of the title bar, if that's what
**System Settings ▸ Desktop & Dock** has it do; doing either again gives the
window back the size it had before. However you size it, the window is at
least 640 by 480 points, so you can tile it into half of a laptop's
screen.

When you plug in or unplug a display, or change its resolution, a window
that filled its screen fills the screen it's on afterwards. A window that
you made smaller stays as it is.

macOS 15 tiles windows too, with **Window ▸ Fill** and **Window ▸ Move &
Resize**, or when you drag a window to an edge of the screen. Tile Bench's
own window as you like: the windows in its panes follow it. A window in
half of the screen is narrow, so Bench can change to the compact layout
(see [Small screens](#small-screens-auto-wide-and-compact)). Don't tile an
app's window that's in a pane, as it then leaves the pane; split the pane
instead (see [Panes](#panes)). Stage Manager doesn't work with Bench at
all: see [Windows jump out of their panes](#windows-jump-out-of-their-panes).

Bench remembers where its window is when it doesn't fill the screen. If
you tile Bench into half of the screen, or make it smaller, it opens in the
same place, at the same size, the next time. This is so even when **Open
Bench filling the screen** is on. Bench can't put the window back if the
display has changed since then: for example, if you changed its
resolution, moved it among your displays, or disconnected it. Then Bench
opens as usual. To open filling the screen again, choose **View ▸ Fill the
Screen** before you quit.

To stop Bench filling the screen when it opens, untick **Open Bench
filling the screen** in [Settings](#settings), or set
`fill_screen = false` in the [config file](#config-reference). Bench then
opens where you left its window. If the window filled the screen last
time, or Bench can't put it back, Bench opens at its usual size, 1,440 by
900 points, in the middle of the display. By default, that size is
wider than `compact_below`, so Auto gives the window the wide layout (see
[Small screens](#small-screens-auto-wide-and-compact)). If your
`compact_below` is 1,440 or more, as in a `config.toml` that an earlier
version of Bench made, Auto gives it the compact layout. To get the wide
layout then, make the window wider, or choose **Wide**.

## When you hide Bench

When you hide Bench (**Bench ▸ Hide Bench**, Command+H, or **Hide** in its
Dock menu), minimise it, or close its window, the current tab's apps hide
with it, and so do its web pages.
Closing the window doesn't quit Bench. To bring it back, do one of these:

- Choose **Show Bench**, or a tab, from its menu bar item.
- Press a tab's shortcut, such as Control+Option+2, or the next, previous,
  or last used tab's. Bench comes back on that tab.
- Press the jump palette's shortcut, Control+Option+Space.
- Click Bench's icon in the Dock.

The tab's apps come back with it, and its focused pane gets the keyboard.

The other keyboard shortcuts still move between panes and sub-tabs while
Bench is hidden, and still send the shown window to another tab, but
windows stay where they are until you bring Bench back. Then the tab is
arranged as you left it.

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
  Code for the Web stops. Before they end, Bench keeps each terminal's
  output, unless you've turned that off: see
  [When Bench opens again](#when-bench-opens-again).
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

## Update Bench

Releases that can update themselves include Sparkle, the update framework
that many Mac apps use. Their release notes say so. To update one of them:

1. Choose **Bench ▸ Check for Updates…**. Bench looks for a new version on
   GitHub at once, and says that you have the latest one, or shows the new
   version with its release notes.
2. Click **Install Update**. Bench downloads the new version and checks
   that its developer signed it. To keep the version you have, click
   **Remind Me Later**, and Bench tells you about the new version again
   after its next daily look, or **Skip This Version**, and Bench doesn't
   tell you about it again.
3. Click **Install and Relaunch**. Bench quits, as it does when you quit it,
   replaces itself with the new version, and opens again. If a terminal is
   running something, Bench asks first. If you choose not to quit, the new
   version goes in the next time you quit Bench.

To have Bench tell you when there's a new version, turn on **Check for a
new version once a day** in [Settings](#settings) (`check_for_updates`).
It's off until you turn it on. Bench then looks when it opens, and then at
most once a day. As a web browser does, Bench sends GitHub its version and
the cookies that GitHub gave it.
When there's a newer version, the status bar says so, with **Install…**,
which shows the new version as **Check for Updates…** does. There, you
install it, put it off with **Remind Me Later**, or skip it with **Skip
This Version**. Bench doesn't look again until you choose one of them. If
GitHub can't be reached, Bench tries again the next day.

In a release without Sparkle, **Check for Updates…** opens Bench's latest
release on GitHub in your browser, and the status bar's notice has
**Download**, which opens the new version's page, and **Dismiss**, after
which Bench doesn't mention that version again. Bench asks GitHub for the
latest release itself, and the request carries Bench's version and nothing
else. To update, download the new version, quit Bench, and replace Bench
in Applications.

Until releases are notarised, macOS may stop applying Bench's
Accessibility permission after an update: see [Get started](#get-started).

## Bench's files

Bench keeps its files in `~/Library/Application Support/Bench/`, or in the
folder that `$BENCH_HOME` names, which must be one that only you can change:

| File | What it is |
| --- | --- |
| `config.toml` | Tabs, layouts, settings, shortcuts, launchers, and rules. Bench creates it on first run, and Settings, Tab properties, and the other changes listed in the [config reference](#config-reference) update it. |
| `config.toml.bak` | The previous `config.toml`, kept by every save. **Use backup** swaps it with a `config.toml` that has problems. |
| `config.toml.good` | A copy of the last `config.toml` that loaded without problems, or that Bench saved. Bench runs with it while `config.toml` has problems. |
| `state.json` | The live arrangement after dragging and splitting, the terminals you added with their folders and the commands Bench types into them, the folder each shell was last in, the names you gave terminals and web pages, the apps you quit, when Bench last saw each app running, the display that Bench's window was on, the sizes you gave dialogs, the last 5 tabs you deleted or closed, with their windows, pages, and terminals, for **Reopen Closed Tab**, each web page's zoom, and, with **Check for a new version once a day** on, when Bench last looked, what it found, and the version you dismissed. Each tab keeps its arrangement by name: see [Tabs you change in the file](#tabs-you-change-in-the-file). |
| `state.json.bad` | A `state.json` that Bench set aside because it couldn't use it, or a copy of one whose tabs `config.toml` no longer had, when Bench started or reloaded `config.toml`. `state.json.bad.1` and `state.json.bad.2` are the two before. See [When Bench can't keep the arrangement](#when-bench-cant-keep-the-arrangement). |
| `terms/` | Each terminal's output from when Bench last quit, as *item ID*`.out`, readable only by you, while **Keep terminal output between launches** is on. Output can hold secrets, such as passwords and keys that a program printed. Closing a terminal removes its file, and turning the setting off removes the folder. See [When Bench opens again](#when-bench-opens-again). |
| `parked.json` | The windows that Bench has moved off screen, and the apps it has hidden, right now, so that they can be put back after a crash. Bench sets it aside as `parked.json.bad` if it doesn't parse. |
| `vscode.json` | The VS Code for the Web server that Bench is running, so that it can be stopped after a crash. |
| `vscode-token-*` | The connection token of the VS Code for the Web server that Bench is running, readable only by you. |
| `bench.lock` | A lock that Bench and `bench arrange` hold while they run, and `bench restore` and the watchdog while they put windows back, so that they never act on `parked.json` at the same time. |
| `first-run-done` | Present once you've finished the welcome guide. |
| `accessibility-version` | The version of Bench that last had Accessibility permission, so that a newer version that doesn't have it can say that the update is why. |
| `view.json` | Only with `BENCH_DEBUG` set: what Bench's window shows, for debugging. See [Read the log](development.md#read-the-log). |

**Help ▸ Show Bench's Files in Finder** opens this folder, with
`config.toml` selected.

Bench also keeps a log, `~/Library/Logs/Bench/bench.log`, of its warnings
and errors, such as a window it couldn't put back on screen, and the
problems that the status bar lists, each with the time. It's for reporting
a problem: only you can read it, and it names your tabs and apps, but not
your windows' titles. Once it reaches 1 MB, Bench moves it to
`bench.log.1`, in place of the one before, and starts it again. The
**Console** app shows it under **Log Reports**. With `BENCH_DEBUG` set,
the log has much more: see [Read the log](development.md#read-the-log).

To remove Bench completely, quit it, move `Bench.app` to the Bin, and delete
this folder, and `~/Library/Logs/Bench/`. Then remove Bench from **System
Settings ▸ Privacy & Security ▸ Accessibility**, and from **Screen
Recording** if you turned on live previews.

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
such as rules, web pages, and app launchers.

Whenever Bench changes the file itself, it writes its own copy in the
layout of the [default file](#the-default-file): your comments and the
order you wrote things in aren't kept. That happens when you save Settings
or a tab's properties; move, close, delete, or reopen a tab; use **Move to
Tab ▸ New Tab** or **Also Show In ▸ New Tab**; make a tab of the windows that are
open; **Undo** a change to the tabs; save
a launcher; tick **Don't ask again**; choose **Use this instead**; add the
welcome guide's starting set-up; or switch a web page that the file lists
to a web app (see
[Switch a web page to a web app](#switch-a-web-page-to-a-web-app)).
`config.toml.bak` keeps the version before the last save.

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

The file is [TOML](https://toml.io). It has five parts: `[settings]`,
`[shortcuts]`, any number of `[[launcher]]` entries, one `[[tab]]` per tab,
and any number of `[[rule]]` entries. The [example config](#example-config)
shows all but launchers: see [Launcher entries](#launcher-entries) for
those.

### Settings

The `[settings]` table. Any setting you leave out takes its default.

| Setting | Default | Values | What it does |
| --- | --- | --- | --- |
| `fill_screen` | `true` | `true` or `false` | Open Bench filling the screen, unless you left its window smaller than the screen. Either way, Bench opens on the display it was last on. See [Fill the screen](#fill-the-screen). |
| `menu_bar` | `"name"` | `"name"`, `"icon"`, or `"hidden"` | What Bench's menu bar item shows: its icon and up to 16 characters of the current tab's name, with the unread total (see `unread_counts`), its icon alone, or no item at all. |
| `show_on_tab_shortcut` | `true` | `true` or `false` | A tab shortcut brings back a hidden Bench, on that tab. See [When you hide Bench](#when-you-hide-bench). |
| `unread_counts` | `true` | `true` or `false` | Show unread counts on sub-tabs and the totals on tabs: the badges on apps' Dock icons, which Bench reads with Accessibility permission, and the counts at the start of web pages' titles, such as **(3)**. See [Unread counts](#unread-counts). |
| `check_for_updates` | `false` | `true` or `false` | Look for a new version on GitHub when Bench opens, and then at most once a day, and say in the status bar when there's a newer one. Off until you turn it on. See [Update Bench](#update-bench). |
| `confirm_quit_busy_terminals` | `true` | `true` or `false` | Ask before quitting Bench, or closing a terminal, from its sub-tab, its own window, or **Close Tab…** (even with `confirm_close` off), while a terminal is running something. See [When Bench quits](#when-bench-quits). |
| `confirm_close` | `true` | `true` or `false` | Ask before **Close Tab…** closes two or more windows, web pages, or terminals, or a terminal that's running something. With it off, `confirm_quit_busy_terminals` still asks before ending a busy terminal. See [Close windows and tabs](#close-windows-and-tabs). |
| `compact_below` | `1280` | A positive number of points | In Auto, Bench's window uses the compact layout when it is narrower than this. It goes back to the wide layout when it is 40 points wider than this. |
| `gap` | `4` | 0 to 100 points | The space between panes, and around them. Settings offers 8, 4, and 2. |
| `show_status_bar` | `true` | `true` or `false` | Show the status bar. Without it, the panes are taller, and its warnings and messages show in the title bar. See [The status bar](#the-status-bar). |
| `tab_shortcut_hints` | `"auto"` | `"auto"`, `"always"`, or `"never"` | When tabs show their shortcuts after their names: except on small screens, always, or never. |
| `scroll_bars` | `"auto"` | `"auto"`, `"always"`, or `"never"` | When the strips of tabs and sub-tabs that don't fit show a scroll bar: as macOS shows scroll bars, always, or never. See [When tabs and sub-tabs don't fit](#when-tabs-and-sub-tabs-dont-fit). |
| `appearance` | `"system"` | `"system"`, `"light"`, or `"dark"` | Whether Bench's own windows are light or dark: as macOS is, changing when it does, always light, or always dark. Web pages and the apps in your panes follow macOS either way. See [Settings](#settings). |
| `density` | `"auto"` | `"auto"`, `"standard"`, or `"compact"` | How much room the title bar, tabs, pane headers, and sub-tabs take: compact in the compact layout, always standard, or always compact. A compact pane header is 32 points tall rather than 40. The title bar changes the next time Bench opens. See [More room for your apps](#more-room-for-your-apps). |
| `pane_headers` | `"always"` | `"always"` or `"auto"` | Whether every pane has a header, or a pane that holds one App window, with nothing to report, goes without, leaving the app's own title bar. See [More room for your apps](#more-room-for-your-apps). |
| `open_missing_apps` | `true` | `true` or `false` | Open a tab's apps when you switch to it, if they aren't running, unless you quit them yourself. See [When an app quits](#when-an-app-quits). |
| `hide_other_tabs` | `true` | `true` or `false` | Hide windows that belong to other tabs. |
| `hide_other_apps` | `false` | `true` or `false` | When switching tabs, also hide every app that Bench doesn't manage. |
| `new_windows` | `"app-pane"` | `"app-pane"`, `"focused-pane"`, or `"float"` | Where an app's new windows go. See below. |
| `follow_focus` | `"switch-tab"` | `"switch-tab"`, `"bring-here"`, or `"off"` | What happens when another tab's window comes forward. See below. |
| `live_previews` | `false` | `true` or `false` | Show live previews of windows. Needs Screen Recording permission. |
| `drag_to_add` | `true` | `true` or `false` | Add a window that's in no tab to a pane by dragging it there by its title bar and holding it still for a moment: in the middle to join the pane, or near an edge to split it. See [Bring a window into Bench](#bring-a-window-into-bench). |
| `terminal_font_size` | `12.5` | 9 to 24 points | The text size of every terminal. ⌘+ and ⌘− change one terminal's text for now. See [Terminals](#terminals). |
| `terminal_option_as_meta` | `false` | `true` or `false` | Option works as Meta in terminals, as with Terminal's setting of the same name, rather than typing its own characters. |
| `notify_terminals` | `true` | `true` or `false` | Post a notification when a terminal you can't see rings its bell, asks for a notification, or finishes its command. Its sub-tab and tab get a bell either way. See [When a terminal needs you](#when-a-terminal-needs-you). |
| `keep_terminal_output` | `true` | `true` or `false` | Keep each terminal's output when Bench quits, and show it again when Bench next opens, with its programs' colours and the rest in grey. Output can hold secrets: setting it to `false` removes what Bench kept. See [When Bench opens again](#when-bench-opens-again). |
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
[Settings ▸ Keyboard](#change-a-shortcut) changes it for you.

| Setting | Default | What it is |
| --- | --- | --- |
| `tab_modifiers` | `"ctrl+alt"` | Modifiers that, with 1 to 9, switch to tabs 1 to 9. |
| `next_tab` | `"ctrl+alt+]"` | Next tab. |
| `previous_tab` | `"ctrl+alt+["` | Previous tab. |
| `last_tab` | ``"ctrl+alt+`"`` | Back to the tab you were on before this one, and again to come back. Hold its modifiers after it to choose from every tab. See [Hold to switch tabs](#hold-to-switch-tabs). |
| `jump` | `"ctrl+alt+space"` | Open the jump palette. |
| `show_all_tabs` | `""` (off) | Show every tab at once, or close the overview. See [See every tab at once](#see-every-tab-at-once). |
| `rearrange` | `"ctrl+alt+0"` | Rearrange the current tab. |
| `focus_pane_modifiers` | `"ctrl+alt"` | Modifiers that, with an arrow key, focus the pane in that direction. |
| `move_window_modifiers` | `"ctrl+alt+shift"` | Modifiers that, with an arrow key, move the shown window in that direction. |
| `send_to_tab_modifiers` | `"ctrl+alt+shift"` | Modifiers that, with 1 to 9, send the shown window to that tab, and with &#96;, to the tab you were on before this one. |
| `next_window` | `"ctrl+alt+tab"` | Next sub-tab in the focused pane. With Shift added, the previous sub-tab. |
| `zoom_pane` | `"ctrl+alt+return"` | Zoom the focused pane, or show all panes again. |
| `grow_pane` | `""` (off) | Make the focused pane 5% larger along its row or column. See [Arrange panes from the keyboard](#arrange-panes-from-the-keyboard). |
| `shrink_pane` | `""` (off) | Make the focused pane 5% smaller along its row or column. |
| `equal_panes` | `""` (off) | Give all the panes of the current tab the same size. |
| `next_layout` | `""` (off) | Give the current tab the next layout. |
| `previous_layout` | `""` (off) | Give the current tab the previous layout. |
| `add_front_window` | `""` (off) | Add the window you used last before Bench to the focused pane, if it's in no tab. |
| `undo` | `""` (off) | Undo the latest change to the arrangement, from any app, as **Tab ▸ Undo** does. |

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
first in the table above keeps it, except `last_tab`, `add_front_window`,
`undo`, and `send_to_tab_modifiers`, which give way to all the others, in
that order. A `config.toml` that leaves `last_tab` out, and gives its
default to another setting other than `add_front_window`, `undo`, or
`send_to_tab_modifiers`, has `last_tab` off. A `config.toml` that leaves
`send_to_tab_modifiers` out, and gives one of its shortcuts to another
setting, has `send_to_tab_modifiers` off. For example, with
``next_window = "ctrl+alt+`"``, the previous window is
Control+Option+Shift+&#96;, so `send_to_tab_modifiers` is off.

A shortcut that Bench can't read, or can't register, stays off: Bench says
so in the status bar, and the rest of the config still applies. See
[A shortcut doesn't work](#a-shortcut-doesnt-work).

### Launcher entries

Each `[[launcher]]` entry is a [launcher](#launchers), in the order that the
add dialog and the jump palette list them. **Save as launcher** and
**Settings ▸ Launchers** write terminal launchers for you.

| Field | Launcher kind | What it is |
| --- | --- | --- |
| `name` | Both | Required. The name the lists show. Names must be unique, ignoring case. |
| `folder` | Both | A terminal launcher's starting folder, `~` if you leave it out. An app launcher's folder to open in the app. |
| `run` | Terminal | A command to type into your shell and run, as **New terminal…** does. Leave it out for just the shell. It must be one line, without tabs. |
| `start_again` | Terminal, with `run` | Whether the launcher's terminals run the command again when Bench next opens. The default is `true`. With `false`, Bench types the command but doesn't press Return. |
| `app` | App | The app's bundle ID. A launcher with `app` is an app launcher. |
| `url` | App | A link to open in the app instead of a folder. It needs a scheme, such as `https://` or the app's own, and `title`. |
| `title` | App | Part of the title of the window to dock, ignoring case. For a folder, it's the folder's name if you leave it out, which editors such as Zed show in their windows' titles. |

An app launcher needs a `folder` or a `url`, not both. Bench opens a folder
as a `file://` link, so the app must open folders, as editors such as Zed
and VS Code do.

```toml
# Zed on a folder. Its window's title has the folder's name, bench.
[[launcher]]
name = "Bench in Zed"
app = "dev.zed.Zed"
folder = "~/code/bench"

# A page in Brave, docked by its window's title.
[[launcher]]
name = "Go docs"
app = "com.brave.Browser"
url = "https://go.dev/doc/"
title = "Documentation"
```

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
| `web` | Web | The address to load. It must start with `https://`, or, for a page on this Mac, such as `http://localhost:3000`, `http://` (see [Pages on this Mac](#pages-on-this-mac)). For a site that you sign in to, its web app, as an `app` item, is often better: it has the browser's protections and sign-ins (see [Web apps](#web-apps)). |
| `profile` | Web (`web` items only) | The name of the item's data store. Items with the same profile share logins and cookies. Items without a profile share one default profile. Profile names are case-sensitive: `Work` and `work` are different profiles. |
| `vscode` | Web | A folder to open in VS Code for the Web. |
| `terminal` | Built-in | The terminal's starting folder. `~` means your home folder. |
| `command` | Built-in | A command to run instead of your shell. When it finishes, the terminal stays with its output, and says so: press Return in it to run the command again. It also runs again when Bench next opens. Your shell runs it as a login shell that isn't interactive, so zsh doesn't read `~/.zshrc`: for a program that `~/.zshrc` puts on your `PATH`, such as one that nvm installs, use `run`. |
| `run` | Built-in | A command to type into your shell and run as the terminal starts, as **New terminal…** does. Your interactive login shell runs it, as in your own terminal, and when it finishes, the terminal goes back to the shell's prompt. It can't go with `command`, and must be one line, without tabs. |
| `start_again` | Built-in, with `run` | Whether the terminal runs `run` again when Bench next opens. The default is `true`. With `false`, Bench types the command but doesn't press Return, so that you can run it when you're ready. |
| `name` | Web and Built-in | The sub-tab's label until the page or shell sets a title. |

A `terminal` item runs your login shell, or its `command` through that
shell, in its folder. With `run`, the shell is interactive too, and Bench
types `run` into it once its prompt is ready. If the folder doesn't exist, a
shell starts in your home folder instead and says so, but a `command`
doesn't run at all, because its relative paths would name other files
there: its pane says that the terminal couldn't start. For the same reason,
the shell doesn't run `run`, or a launcher's command, there. Terminals have `TERM=xterm-256color`,
`TERM_PROGRAM=Bench`, and Bench's version as `TERM_PROGRAM_VERSION`. Bench opened from the Finder, the Dock, or at login has
no `LANG`, so terminals get one that matches the language and region in
System Settings, such as `de_DE.UTF-8`, or `en_US.UTF-8` if macOS has no
such locale. When you start Bench from another terminal, its terminals leave
out the variables that describe that terminal's session, such as `TMUX`,
`ITERM_SESSION_ID`, and `SHLVL`.

For finding text, links, the text size, and the right-click menu, see
[Terminals](#terminals).

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

# Claude Code, typed into your shell, which stays when it finishes.
items = [{ terminal = "~/code/bench", run = "claude --continue", name = "Claude" }]
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
- A `web` address that isn't `https`, other than an `http` address on this
  Mac, or an app item's `url` with no scheme.
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
unread_counts = true          # Counts from Dock badges and page titles.
check_for_updates = false     # Look for a new version once a day.
confirm_quit_busy_terminals = true
confirm_close = true          # Ask before closing a tab's windows.
compact_below = 1280          # Windows narrower than this, in points, are compact.
gap = 4                       # Points between panes.
show_status_bar = true        # The status bar at the bottom of the window.
tab_shortcut_hints = "auto"   # auto, always, or never
scroll_bars = "auto"          # auto, always, or never
appearance = "system"         # system, light, or dark
density = "auto"              # auto, standard, or compact
pane_headers = "always"       # always, or auto: none over one app window
open_missing_apps = true      # Open a tab's apps if they aren't running.
hide_other_tabs = true        # Hide windows that belong to other tabs.
hide_other_apps = false       # Also hide every app Bench doesn't manage.
new_windows = "app-pane"      # app-pane, focused-pane, or float
follow_focus = "switch-tab"   # switch-tab, bring-here, or off
drag_to_add = true            # Drag a window onto a pane to add it.
live_previews = false         # Needs Screen Recording permission.
terminal_font_size = 12.5     # Points, from 9 to 24.
terminal_option_as_meta = false  # Option sends Meta in terminals.
notify_terminals = true       # When a terminal out of view needs you.
keep_terminal_output = true   # Show terminals' output again next time.
launch_timeout = "15s"

[shortcuts]
tab_modifiers = "ctrl+alt"          # Plus 1 to 9.
next_tab = "ctrl+alt+]"
previous_tab = "ctrl+alt+["
last_tab = "ctrl+alt+`"             # Back to the tab you were on.
jump = "ctrl+alt+space"
show_all_tabs = ""                  # Every tab at once. Off when empty.
rearrange = "ctrl+alt+0"
focus_pane_modifiers = "ctrl+alt"   # Plus an arrow key.
move_window_modifiers = "ctrl+alt+shift"
send_to_tab_modifiers = "ctrl+alt+shift"  # Plus 1 to 9, or ` for the last used tab.
next_window = "ctrl+alt+tab"
zoom_pane = "ctrl+alt+return"
grow_pane = ""                      # The focused pane, by 5%. Off when empty.
shrink_pane = ""
equal_panes = ""
next_layout = ""                    # The six layouts in turn.
previous_layout = ""
add_front_window = ""               # To the focused pane. Off when empty.
undo = ""                           # The latest change, from any app. Off when empty.

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
unread_counts = true          # Counts from Dock badges and page titles.
check_for_updates = false     # Look for a new version once a day.
confirm_quit_busy_terminals = true
confirm_close = true          # Ask before closing a tab's windows.
compact_below = 1280          # Windows narrower than this, in points, are compact.
gap = 4                       # Points between panes.
show_status_bar = true        # The status bar at the bottom of the window.
tab_shortcut_hints = "auto"   # auto, always, or never
scroll_bars = "auto"          # auto, always, or never
appearance = "system"         # system, light, or dark
density = "auto"              # auto, standard, or compact
pane_headers = "always"       # always, or auto: none over one app window
open_missing_apps = true      # Open a tab's apps if they aren't running.
hide_other_tabs = true        # Hide windows that belong to other tabs.
hide_other_apps = false       # Also hide every app Bench doesn't manage.
new_windows = "app-pane"      # app-pane, focused-pane, or float
follow_focus = "switch-tab"   # switch-tab, bring-here, or off
drag_to_add = true            # Drag a window onto a pane to add it.
live_previews = false         # Needs Screen Recording permission.
terminal_font_size = 12.5     # Points, from 9 to 24.
terminal_option_as_meta = false  # Option sends Meta in terminals.
notify_terminals = true       # When a terminal out of view needs you.
keep_terminal_output = true   # Show terminals' output again next time.
launch_timeout = "15s"

[shortcuts]
tab_modifiers = "ctrl+alt"          # Plus 1 to 9.
next_tab = "ctrl+alt+]"
previous_tab = "ctrl+alt+["
last_tab = "ctrl+alt+`"             # Back to the tab you were on.
jump = "ctrl+alt+space"
show_all_tabs = ""                  # Every tab at once. Off when empty.
rearrange = "ctrl+alt+0"
focus_pane_modifiers = "ctrl+alt"   # Plus an arrow key.
move_window_modifiers = "ctrl+alt+shift"
send_to_tab_modifiers = "ctrl+alt+shift"  # Plus 1 to 9, or ` for the last used tab.
next_window = "ctrl+alt+tab"
zoom_pane = "ctrl+alt+return"
grow_pane = ""                      # The focused pane, by 5%. Off when empty.
shrink_pane = ""
equal_panes = ""
next_layout = ""                    # The six layouts in turn.
previous_layout = ""
add_front_window = ""               # To the focused pane. Off when empty.
undo = ""                           # The latest change, from any app. Off when empty.

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

If what you see isn't here, choose **Help ▸ Report a Problem…** to tell us
about it: see [The menu bar](#the-menu-bar). **Help ▸ Recent Messages**
shows what Bench said lately, to copy into the report.

### Bench can't move windows

Bench needs Accessibility permission. Click **Turn on Accessibility** in
the banner, which opens **System Settings ▸ Privacy & Security ▸
Accessibility**, and turn on Bench. Bench notices within a few seconds, and
its banner goes away.

If Bench is already turned on there but still can't move windows, which
happens after updating Bench, select Bench in the list, remove it with the
minus button, then add it again and turn it on. Bench recognises this case
when the version that last had the permission is older, and its banner
says "Bench was updated, so macOS needs you to allow it again: select
Bench, click −, then turn it on again".

If macOS takes the permission away while Bench is open, for example because
you turned Bench off in System Settings, the banner comes back when you go
back to Bench or switch tabs. Bench stops arranging windows, and shows the
apps that it hid, so that you can reach their windows. Windows that Bench
moved off screen stay there: Bench can't move them without the permission.
Turn Bench on again, and Bench arranges the tab again within a few seconds.
If you quit Bench first, it puts those windows back the next time it opens
with the permission.

### Live previews don't appear

Previews need Screen Recording permission. **Open Screen Recording**, under
**Show live previews of windows** in Settings, opens **System Settings ▸
Privacy & Security ▸ Screen Recording** (called **Screen & System Audio
Recording** in macOS 15 and later): turn on Bench there. macOS applies it
only when Bench opens again, so click **Quit and Reopen Bench**. macOS asks
you to approve this permission again from time to time.

If a preview shows a picture that doesn't change, the app may have stopped
drawing its covered window. Chromium-based apps such as Brave, VS Code,
Slack, and Obsidian do this.

### A shortcut doesn't work

When a shortcut doesn't work, or may not, the status bar says which one
and why, and suggests another, such as "Couldn't register ⌃⌥← and ⌃⌥→
(window tools such as Rectangle often use them). Try ⌃⌥⌘ with the arrow
keys instead." Each of these notices has two buttons:

- **Use this instead** changes the shortcut to the one suggested, saves
  `config.toml`, and uses it at once. Bench checks first that it's free.
- **Keyboard settings** opens [Settings ▸ Keyboard](#change-a-shortcut),
  to choose another yourself.

The usual causes:

- **Input sources.** With more than one input source, macOS uses
  Control+Space and Control+Option+Space to switch between them, and keeps
  the keys for itself, though it lets Bench register them: "macOS uses
  ⌃⌥Space to switch input sources, so Bench won't see it." Bench reads
  these shortcuts from System Settings, as you set them. Turn them off in
  **System Settings ▸ Keyboard ▸ Keyboard Shortcuts ▸ Input Sources**, or
  give **Jump to…** another shortcut.
- **Window tools.** Rectangle and similar tools use Control+Option+arrow by
  default. Change their shortcuts, or change **Focus a pane** and **Move a
  window**.
- **Another app.** Some other app has the shortcut. Bench can't tell which.
- **macOS's own shortcuts.** macOS keeps some shortcuts, such as ⌘Space for
  Spotlight, for itself. Bench never suggests them, and says so if you
  choose one.
- **Every app's shortcuts.** Every Mac app uses some shortcuts, such as ⌘Z
  for Undo, ⌘C for Copy, and ⌘Q for Quit. Bench could take them, and then
  no app would get them, so it never suggests them, and says so if you
  choose one: "Mac apps use ⌘Z for their own commands, so Bench would take
  it from every app."

**VoiceOver.** VoiceOver uses Control+Option, its VO keys, with every key
for its own commands, such as VO+arrow to move and VO+Space to press. Every
one of Bench's shortcuts uses Control+Option too, so while VoiceOver is on,
the status bar says "VoiceOver is on, and it uses ⌃⌥ for its own commands,
so Bench's shortcuts with ⌃⌥ may not work." It suggests the same
shortcuts with Control+Command in place of Control+Option, such as ⌃⌘] for
**Next tab**, or another where macOS has that one, such as ⌃⌘J for **Jump
to…**, since ⌃⌘Space opens Emoji & Symbols. For **Send a window to a
tab**, it suggests ⌥⇧⌘, since ⌃⇧⌘3 and ⌃⇧⌘4 take screenshots. Choose
**Use this instead** to use them all. Bench notices when you turn
VoiceOver on or off, and the notice comes and goes with it. Bench can't
tell which VO keys you've chosen in VoiceOver Utility, so it warns
whenever VoiceOver is on.

You can also change the `[shortcuts]` table in `config.toml` and choose
**Bench ▸ Reload config.toml**. Bench registers the new shortcuts at once.

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
tab again with Control+Option+0, **Rearrange** in the jump palette,
or **View ▸ Rearrange**.

macOS can move windows when your Mac wakes from sleep, when you unlock it,
when you switch back to your user account, and when you plug in or unplug
a display. Bench waits until the displays settle, about 2 seconds, then
puts the windows of the current tab back in their panes. It doesn't change
which pane has the keyboard, and it says nothing when this works.

If you dragged the window away yourself, its pane says "Moved out of its
pane": click **Dock again**. Dragging a window's edge resizes the panes
only if another pane is beyond that edge. A tab shown as one pane with
sub-tabs has no pane beyond any edge. A zoomed pane has none either.
Dragging any other edge moves the window out of its pane. To resize the
panes without moving a window, drag the gap between them.

If the window goes back while you drag its edge, wait a second, then drag
the edge again. If it still goes back, drag the gap between the panes
instead.

### Windows jump out of their panes

If Stage Manager is on, the status bar says "Stage Manager is on, so Bench
can't keep windows in their panes." Stage Manager moves the windows of the
apps you aren't using to a strip at the side of the screen, and shrinks the
one you are, so Bench's panes empty and windows jump about. Bench can't
stop it. Turn it off: click Control Centre in the menu bar, then **Stage
Manager**. It's also in **System Settings ▸ Desktop & Dock**. Bench looks
again each time you come back to it, and the message goes.

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

When you use another desktop while Bench is open, Bench can't see the
windows on its own desktop, and their panes can say "On another desktop".
When you go back to Bench's desktop, Bench looks for the windows again and
docks them. It also looks again after your Mac wakes.

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

A page that posts a notification while you can't see it also gets a blue
bell on its sub-tab and tab, as a terminal does (see
[When a terminal needs you](#when-a-terminal-needs-you)), and clicking the
notification shows the page.

### Bench doesn't go full screen

Bench's window doesn't use native full screen, because App windows can't
follow it into a full-screen space. Instead, it opens filling the screen,
below the menu bar and beside the Dock. If you've made it smaller, choose
**View ▸ Fill the Screen**, or **Fill the Screen** in the jump palette. See
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
status bar is hidden, ***N* notices** in the title bar, shown as ⚠ *N* in
a window narrower than 1,000 points), and Bench runs with the last
config that worked until you fix them. Until then, Bench doesn't save
changes from Settings and Tab properties, so they're lost when you quit;
the message after saving says so. The first message says whether Bench
saves the arrangement (`state.json`) meanwhile.

1. Click the message in the status bar, or ***N* notices** (or ⚠ *N*) in
   the title bar, then **Open config.toml**, to open the file in your text
   editor. **Show in Finder** shows where it is.
2. Fix each problem listed. `bench check-config` lists them too.
3. Save the file, and click **Reload**. Bench uses the file at once, and
   saves again.

If you can't fix it, click **Use backup** to go back to `config.toml.bak`,
the version before the last save from Settings or Tab properties. It's
offered only if the backup has no problems. Or rename the file: Bench
writes a new default one when it next opens.
