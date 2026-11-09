package mac

import "strings"

// Bench presses other apps' menu items in two places: New Window, to open a
// window of a running app (NewWindow), and Close Window, or Close, to close
// a window that has no close button (PressClose). The native side reads each
// item of the app's menus, File menu first, and Go ranks it here, from its
// title and its key equivalent: the highest rank wins, and of two items with
// the same rank, the first. The native side stops after the menu in which
// it finds an exact title (newWindowExact, closeWindowExact). Titles match
// in English and in the languages of menuLanguages. The key equivalent
// only breaks ties, or confirms a title that matches less surely. It never
// counts on its own: Command-N is New Message in Mail and New Note in
// Notes, and Command-W closes a tab in Safari.
//
// A key equivalent only ever raises a rank, and Command-N, or Command-W,
// raises it most. So the native side first ranks each title with that key,
// and reads the item's own key only if the title could then beat the best
// so far. Most titles can't, and reading a key takes one or two more
// Accessibility calls, which Bench makes with the engine's lock held.

// menuKey is a menu item's key equivalent, as the Accessibility API gives
// it: its character (AXMenuItemCmdChar), empty for none, and its modifier
// keys (AXMenuItemCmdModifiers), where 0 is Command alone, and -1 means that
// the app didn't say.
type menuKey struct {
	char      string
	modifiers int
}

// commandAnd reports whether the key equivalent is Command and letter, with
// no other modifier key.
func (k menuKey) commandAnd(letter string) bool {
	return k.modifiers == 0 && strings.EqualFold(strings.TrimSpace(k.char), letter)
}

// menuTitle returns a menu item's title ready to compare: in lower case,
// without spaces at either end, or an ellipsis at the end.
func menuTitle(title string) string {
	t := strings.ToLower(strings.TrimSpace(title))
	return strings.TrimSpace(strings.TrimSuffix(strings.TrimSuffix(t, "…"), "..."))
}

// menuLanguage is how Apple's apps title the menu items that Bench presses
// in one of the languages that macOS comes in.
type menuLanguage struct {
	code string
	// newWindow is New Window, from SwiftUI's standard menu
	// (MainMenu.loctable) and Safari.
	newWindow string
	// closeWindow is Close Window, from AppKit's standard menu
	// (MenuCommands.loctable).
	closeWindow string
	// close is Close, from AppKit's document menu (Document.loctable) and
	// Preview.
	close string
	// newWord and windowWord are the words for new and window in New Window.
	// Other New … Window items, such as Finder's New Finder Window, start
	// with newWord and contain windowWord: "Nouvelle fenêtre Finder",
	// "Nyt Findervindue", "新規Finderウインドウ".
	newWord, windowWord string
	// private is the word for private in Safari's New Private Window, which
	// doesn't count.
	private string
}

// menuLanguages are the titles that Apple's own apps give in macOS 27, as
// the Apple Localization Terms Glossary (applelocalization.com) lists them
// from the system's resources. Portuguese is the same in Brazil and
// Portugal, but for case, and Chinese is the same in Hong Kong as in
// Taiwan. A language that isn't here matches only in English, and an app
// that titles its items in its own way may not match at all: then New
// Window opens the app again instead, and Bench says that it can't close a
// window that has no close button.
var menuLanguages = []menuLanguage{
	{"de", "Neues Fenster", "Fenster schließen", "Schließen", "Neues", "Fenster", "privat"},
	{"fr", "Nouvelle fenêtre", "Fermer la fenêtre", "Fermer", "Nouvelle", "fenêtre", "privée"},
	{"es", "Nueva ventana", "Cerrar ventana", "Cerrar", "Nueva", "ventana", "privada"},
	{"it", "Nuova finestra", "Chiudi finestra", "Chiudi", "Nuova", "finestra", "privata"},
	{"pt", "Nova janela", "Fechar janela", "Fechar", "Nova", "janela", "privada"},
	{"nl", "Nieuw venster", "Sluit venster", "Sluit", "Nieuw", "venster", "privé"},
	{"sv", "Nytt fönster", "Stäng fönster", "Stäng", "Nytt", "fönster", "privat"},
	{"da", "Nyt vindue", "Luk vindue", "Luk", "Nyt", "vindue", "privat"},
	{"no", "Nytt vindu", "Lukk vindu", "Lukk", "Nytt", "vindu", "privat"},
	{"fi", "Uusi ikkuna", "Sulje ikkuna", "Sulje", "Uusi", "ikkuna", "yksityinen"},
	{"pl", "Nowe okno", "Zamknij okno", "Zamknij", "Nowe", "okno", "prywatne"},
	{"ru", "Новое окно", "Закрыть окно", "Закрыть", "Новое", "окно", "частное"},
	{"tr", "Yeni Pencere", "Pencereyi Kapat", "Kapat", "Yeni", "Pencere", "Özel"},
	{"ja", "新規ウインドウ", "ウインドウを閉じる", "閉じる", "新規", "ウインドウ", "プライベート"},
	{"ko", "새로운 윈도우", "윈도우 닫기", "닫기", "새로운", "윈도우", "개인정보 보호"},
	{"zh-Hans", "新建窗口", "关闭窗口", "关闭", "新建", "窗口", "无痕"},
	{"zh-Hant", "新增視窗", "關閉視窗", "關閉", "新增", "視窗", "私密"},
}

// The titles of menuLanguages, ready to compare (menuTitle).
var (
	newWindowTitles   = map[string]bool{"new window": true}
	closeWindowTitles = map[string]bool{"close window": true}
	closeTitles       = map[string]bool{"close": true}
)

func init() {
	for _, l := range menuLanguages {
		newWindowTitles[menuTitle(l.newWindow)] = true
		closeWindowTitles[menuTitle(l.closeWindow)] = true
		closeTitles[menuTitle(l.close)] = true
	}
}

// How well a menu item names the New Window item.
const (
	// newWindowOther is another "New … Window" item in English, such as
	// Finder's New Finder Window.
	newWindowOther = 1
	// newWindowOtherKey is newWindowOther with Command-N. In other
	// languages, where the words can come in any order, such an item counts
	// only with Command-N, and then has this rank.
	newWindowOtherKey = 2
	// newWindowExact is New Window itself, in any case.
	newWindowExact = 3
	// newWindowExactKey is New Window with Command-N, the best there is.
	newWindowExactKey = 4
)

// newWindowRank ranks a menu item as the New Window item, from its title
// and key equivalent: one of the ranks above, or 0 for anything else.
// Private and incognito windows don't count, and nor do items that open
// something other than a window, such as New Tab, New File, or New
// Message: in many apps those, not New Window, have Command-N.
func newWindowRank(title string, key menuKey) int {
	t := menuTitle(title)
	withKey := 0
	if key.commandAnd("N") {
		withKey = 1
	}
	if newWindowTitles[t] {
		return newWindowExact + withKey
	}
	if strings.HasPrefix(t, "new ") && strings.HasSuffix(t, " window") {
		if strings.Contains(t, "private") || strings.Contains(t, "incognito") {
			return 0
		}
		return newWindowOther + withKey
	}
	if withKey == 0 {
		return 0
	}
	for _, l := range menuLanguages {
		if strings.HasPrefix(t, menuTitle(l.newWord)) && strings.Contains(t, menuTitle(l.windowWord)) && !strings.Contains(t, menuTitle(l.private)) {
			return newWindowOtherKey
		}
	}
	return 0
}

// How well a menu item names the item that closes the app's main window.
const (
	// closeWindowPlain is Close, which closes the main window, or the
	// document it shows, in most apps that have no Close Window.
	closeWindowPlain = 1
	// closeWindowPlainKey is Close with Command-W.
	closeWindowPlainKey = 2
	// closeWindowExact is Close Window itself, in any case.
	closeWindowExact = 3
	// closeWindowExactKey is Close Window with Command-W, the best there is.
	closeWindowExactKey = 4
)

// closeWindowRank ranks a menu item as the item that closes the app's main
// window, from its title and key equivalent: one of the ranks above, or 0
// for anything else, such as Close Tab, Close All, or Close Others, which
// close more or less than the window, even with Command-W.
func closeWindowRank(title string, key menuKey) int {
	t := menuTitle(title)
	withKey := 0
	if key.commandAnd("W") {
		withKey = 1
	}
	switch {
	case closeWindowTitles[t]:
		return closeWindowExact + withKey
	case closeTitles[t]:
		return closeWindowPlain + withKey
	}
	return 0
}

// How surely a menu bar item is the app's File menu, where Bench looks
// first.
const (
	// fileMenuPlace is the item after the app's own menu, where Mac apps
	// keep the File menu, whatever their language calls it: Ablage,
	// Fichier, ファイル.
	fileMenuPlace = 1
	// fileMenuTitle is an item titled File, in English, wherever it is.
	fileMenuTitle = 2
)

// fileMenuRank ranks a menu bar item as the File menu, from its place among
// the menu bar's items (0 is the Apple menu, and 1 the app's own menu) and
// its title: one of the ranks above, or 0 for any other item. An app without
// a File menu, such as System Settings, has another menu in its place, which
// does no harm: Bench looks in every menu, and the order only decides
// between items with the same rank.
func fileMenuRank(index int, title string) int {
	if menuTitle(title) == "file" {
		return fileMenuTitle
	}
	if index == 2 {
		return fileMenuPlace
	}
	return 0
}
