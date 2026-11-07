package mac

import "testing"

// Key equivalents as the Accessibility API gives them.
var (
	noKey         = menuKey{"", -1}
	cmdN          = menuKey{"N", 0}
	cmdW          = menuKey{"W", 0}
	cmdT          = menuKey{"T", 0}
	shiftCmdN     = menuKey{"N", 1}
	shiftCmdW     = menuKey{"W", 1}
	optionCmdN    = menuKey{"N", 2}
	optionShiftW  = menuKey{"W", 3}
	controlCmdN   = menuKey{"N", 4}
	noCommandN    = menuKey{"N", 8}
	unknownModsN  = menuKey{"N", -1}
	lowerCaseCmdN = menuKey{"n", 0}
)

func TestNewWindowRank(t *testing.T) {
	tests := []struct {
		title string
		key   menuKey
		want  int
	}{
		{"New Window", noKey, newWindowExact},
		{"New window", noKey, newWindowExact},
		{"  NEW WINDOW  ", noKey, newWindowExact},
		{"New Window…", noKey, newWindowExact},
		{"New Window...", noKey, newWindowExact},
		{"New Window", cmdN, newWindowExactKey},
		{"New Window", lowerCaseCmdN, newWindowExactKey},
		// Only Command alone confirms: VS Code's New Window is Shift-Command-N.
		{"New Window", shiftCmdN, newWindowExact},
		{"New Window", optionCmdN, newWindowExact},
		{"New Window", controlCmdN, newWindowExact},
		{"New Window", noCommandN, newWindowExact},
		{"New Window", unknownModsN, newWindowExact},
		{"New Window", cmdT, newWindowExact},
		{"New Finder Window", noKey, newWindowOther},
		{"New Finder Window", cmdN, newWindowOtherKey},
		{"New Terminal Window", noKey, newWindowOther},
		{"New Empty Window", noKey, newWindowOther},
		// Mail: New Viewer Window has Option-Command-N.
		{"New Viewer Window", optionCmdN, newWindowOther},
		{"New Private Window", noKey, 0},
		{"New Private Window", shiftCmdN, 0},
		{"New Private Window", cmdN, 0},
		{"New Incognito Window", shiftCmdN, 0},
		{"New InPrivate Window", noKey, 0},
		// Command-N alone is never enough: Mail, Notes, Xcode, Safari.
		{"New Message", cmdN, 0},
		{"New Note", cmdN, 0},
		{"New File", cmdN, 0},
		{"New Document", cmdN, 0},
		{"New", cmdN, 0},
		{"New Tab", cmdT, 0},
		{"New Tab", cmdN, 0},
		{"New Window with Profile", noKey, 0},
		{"Window", noKey, 0},
		{"Renew Window", noKey, 0},
		{"", cmdN, 0},
	}
	for _, tt := range tests {
		if got := newWindowRank(tt.title, tt.key); got != tt.want {
			t.Errorf("newWindowRank(%q, %v) = %d, want %d", tt.title, tt.key, got, tt.want)
		}
	}
}

// TestNewWindowRankLanguages checks New Window as Apple's apps title it in
// other languages, Finder's New Finder Window, which counts only with
// Command-N, and the traps, which mustn't count even with Command-N:
// Safari's New Private Window and New Tab, Mail's New Message, and Notes'
// New Note.
func TestNewWindowRankLanguages(t *testing.T) {
	tests := []struct {
		lang                string
		newWindow, finder   string
		private, newTab     string
		newMessage, newNote string
	}{
		{"de", "Neues Fenster", "Neues Fenster", "Neues privates Fenster", "Neuer Tab", "Neue Nachricht", "Neue Notiz"},
		{"fr", "Nouvelle fenêtre", "Nouvelle fenêtre Finder", "Nouvelle fenêtre privée", "Nouvel onglet", "Nouveau message", "Nouvelle note"},
		{"es", "Nueva ventana", "Nueva ventana del Finder", "Nueva ventana privada", "Nueva pestaña", "Nuevo mensaje", "Nueva nota"},
		{"it", "Nuova finestra", "Nuova finestra del Finder", "Nuova finestra privata", "Nuovo pannello", "Nuovo messaggio", "Nuova nota"},
		{"pt-BR", "Nova Janela", "Nova Janela do Finder", "Nova Janela com Navegação Privada", "Nova Aba", "Nova Mensagem", "Nova Nota"},
		{"pt-PT", "Nova janela", "Nova janela do Finder", "Nova janela privada", "Novo separador", "Nova mensagem", "Nova nota"},
		{"nl", "Nieuw venster", "Nieuw Finder-venster", "Nieuw privévenster", "Nieuw tabblad", "Nieuw bericht", "Nieuwe notitie"},
		{"sv", "Nytt fönster", "Nytt Finder-fönster", "Nytt privat fönster", "Ny flik", "Nytt mejl", "Ny anteckning"},
		{"da", "Nyt vindue", "Nyt Findervindue", "Nyt privat vindue", "Ny fane", "Ny besked", "Ny note"},
		{"no", "Nytt vindu", "Nytt Finder-vindu", "Nytt privat vindu", "Ny fane", "Ny melding", "Nytt notat"},
		{"fi", "Uusi ikkuna", "Uusi Finderin ikkuna", "Uusi yksityinen ikkuna", "Uusi välilehti", "Uusi viesti", "Uusi muistiinpano"},
		{"pl", "Nowe okno", "Nowe okno Findera", "Nowe okno prywatne", "Nowa karta", "Nowa wiadomość", "Nowa notatka"},
		{"ru", "Новое окно", "Новое окно Finder", "Новое частное окно", "Новая вкладка", "Новое письмо", "Новая заметка"},
		{"tr", "Yeni Pencere", "Yeni Finder Penceresi", "Yeni Özel Pencere", "Yeni Sekme", "Yeni İleti", "Yeni Not"},
		{"ja", "新規ウインドウ", "新規Finderウインドウ", "新規プライベートウインドウ", "新規タブ", "新規メッセージ", "新規メモ"},
		{"ko", "새로운 윈도우", "새로운 Finder 윈도우", "새로운 개인정보 보호 윈도우", "새로운 탭", "새로운 메시지", "새로운 메모"},
		{"zh-Hans", "新建窗口", "新建“访达”窗口", "新建无痕浏览窗口", "新建标签页", "新邮件", "新建备忘录"},
		{"zh-Hant", "新增視窗", "新增 Finder 視窗", "新增私密視窗", "新增標籤頁", "新增郵件", "新增備忘錄"},
	}
	for _, tt := range tests {
		t.Run(tt.lang, func(t *testing.T) {
			check := func(title string, key menuKey, want int) {
				t.Helper()
				if got := newWindowRank(title, key); got != want {
					t.Errorf("newWindowRank(%q, %v) = %d, want %d", title, key, got, want)
				}
			}
			check(tt.newWindow, noKey, newWindowExact)
			check(tt.newWindow+"…", noKey, newWindowExact)
			check(tt.newWindow, cmdN, newWindowExactKey)
			check(tt.newWindow, shiftCmdN, newWindowExact)
			if tt.finder != tt.newWindow {
				check(tt.finder, cmdN, newWindowOtherKey)
				check(tt.finder, noKey, 0)
				check(tt.finder, optionCmdN, 0)
			}
			check(tt.private, shiftCmdN, 0)
			check(tt.private, cmdN, 0)
			check(tt.newTab, cmdT, 0)
			check(tt.newTab, cmdN, 0)
			check(tt.newMessage, cmdN, 0)
			check(tt.newNote, cmdN, 0)
		})
	}
}

func TestCloseWindowRank(t *testing.T) {
	tests := []struct {
		title string
		key   menuKey
		want  int
	}{
		{"Close Window", noKey, closeWindowExact},
		{"close window", noKey, closeWindowExact},
		{"  CLOSE WINDOW  ", noKey, closeWindowExact},
		{"Close Window…", noKey, closeWindowExact},
		{"Close Window", cmdW, closeWindowExactKey},
		// Safari's and Terminal's Close Window is Shift-Command-W.
		{"Close Window", shiftCmdW, closeWindowExact},
		{"Close", noKey, closeWindowPlain},
		{"Close...", noKey, closeWindowPlain},
		{"Close", cmdW, closeWindowPlainKey},
		{"Close", optionShiftW, closeWindowPlain},
		// Command-W alone is never enough: it's Close Tab in Safari and
		// Chrome.
		{"Close Tab", cmdW, 0},
		{"Close Tab", noKey, 0},
		{"Close All", noKey, 0},
		{"Close All Windows", optionShiftW, 0},
		{"Close Other Tabs", noKey, 0},
		{"Close Others", noKey, 0},
		{"Closed", noKey, 0},
		{"Window", cmdW, 0},
		{"Minimise", cmdW, 0},
		{"", cmdW, 0},
	}
	for _, tt := range tests {
		if got := closeWindowRank(tt.title, tt.key); got != tt.want {
			t.Errorf("closeWindowRank(%q, %v) = %d, want %d", tt.title, tt.key, got, tt.want)
		}
	}
}

// TestCloseWindowRankLanguages checks Close Window and Close as Apple's apps
// title them in other languages, and the traps: Safari's Close Tab, which
// has Command-W, Close All, and Minimise, which also has a window's word in
// Polish.
func TestCloseWindowRankLanguages(t *testing.T) {
	tests := []struct {
		lang                         string
		closeWindow, close           string
		closeTab, closeAll, minimise string
	}{
		{"de", "Fenster schließen", "Schließen", "Tab schließen", "Alle schließen", "Im Dock ablegen"},
		{"fr", "Fermer la fenêtre", "Fermer", "Fermer l’onglet", "Tout fermer", "Minimiser"},
		{"es", "Cerrar ventana", "Cerrar", "Cerrar pestaña", "Cerrar todo", "Minimizar"},
		{"it", "Chiudi finestra", "Chiudi", "Chiudi pannello", "Chiudi tutto", "Contrai"},
		{"pt-BR", "Fechar Janela", "Fechar", "Fechar Aba", "Fechar Todas", "Minimizar"},
		{"pt-PT", "Fechar janela", "Fechar", "Fechar separador", "Fechar todas", "Minimizar"},
		{"nl", "Sluit venster", "Sluit", "Sluit tabblad", "Sluit alles", "Minimaliseer"},
		{"sv", "Stäng fönster", "Stäng", "Stäng flik", "Stäng alla", "Minimera"},
		{"da", "Luk vindue", "Luk", "Luk fane", "Luk alt", "Minimer"},
		{"no", "Lukk vindu", "Lukk", "Lukk fane", "Lukk alle", "Minimer"},
		{"fi", "Sulje ikkuna", "Sulje", "Sulje välilehti", "Sulje kaikki", "Pienennä"},
		{"pl", "Zamknij okno", "Zamknij", "Zamknij kartę", "Zamknij wszystkie", "Minimalizuj okno"},
		{"ru", "Закрыть окно", "Закрыть", "Закрыть вкладку", "Закрыть все", "Убрать в\u00a0Dock"},
		{"tr", "Pencereyi Kapat", "Kapat", "Sekmeyi Kapat", "Tümünü Kapat", "Simge Durumuna Küçült"},
		{"ja", "ウインドウを閉じる", "閉じる", "タブを閉じる", "すべてを閉じる", "しまう"},
		{"ko", "윈도우 닫기", "닫기", "탭 닫기", "모두 닫기", "최소화"},
		{"zh-Hans", "关闭窗口", "关闭", "关闭标签页", "全部关闭", "最小化"},
		{"zh-Hant", "關閉視窗", "關閉", "關閉標籤頁", "關閉全部", "縮到最小"},
	}
	for _, tt := range tests {
		t.Run(tt.lang, func(t *testing.T) {
			check := func(title string, key menuKey, want int) {
				t.Helper()
				if got := closeWindowRank(title, key); got != want {
					t.Errorf("closeWindowRank(%q, %v) = %d, want %d", title, key, got, want)
				}
			}
			check(tt.closeWindow, noKey, closeWindowExact)
			check(tt.closeWindow, shiftCmdW, closeWindowExact)
			check(tt.closeWindow, cmdW, closeWindowExactKey)
			check(tt.close, noKey, closeWindowPlain)
			check(tt.close+"…", cmdW, closeWindowPlainKey)
			check(tt.closeTab, cmdW, 0)
			check(tt.closeAll, optionShiftW, 0)
			check(tt.minimise, cmdW, 0)
		})
	}
}

func TestFileMenuRank(t *testing.T) {
	tests := []struct {
		index int
		title string
		want  int
	}{
		{2, "File", fileMenuTitle},
		{2, "file", fileMenuTitle},
		// An app that puts a menu before File.
		{3, "File", fileMenuTitle},
		{2, "Ablage", fileMenuPlace},
		{2, "Fichier", fileMenuPlace},
		{2, "ファイル", fileMenuPlace},
		{2, "", fileMenuPlace},
		// Terminal keeps New Window in Shell, in the File menu's place.
		{2, "Shell", fileMenuPlace},
		{1, "TextEdit", 0},
		{3, "Bearbeiten", 0},
		{3, "Edit", 0},
		{0, "Apple", 0},
	}
	for _, tt := range tests {
		if got := fileMenuRank(tt.index, tt.title); got != tt.want {
			t.Errorf("fileMenuRank(%d, %q) = %d, want %d", tt.index, tt.title, got, tt.want)
		}
	}
}

// menuItem is a menu item as the native side reads it.
type menuItem struct {
	title string
	key   menuKey
}

// menuSearch is the item that searchMenus finds, and what it reads to find
// it.
type menuSearch struct {
	// title is the item that wins, or "" if none ranks, and menu and at are
	// its place, or -1: its menu, File menu first, and its index there.
	title    string
	menu, at int
	// menus is how many menus it reads, and keys how many items' key
	// equivalents.
	menus, keys int
}

// searchMenus looks for an item as benchFindNewWindowItem and
// benchFindCloseWindowItem do, in the menus in their order, File menu
// first. It keeps the first item with the highest rank above 0. It reads an
// item's key equivalent only if the item's title, with confirm (Command-N
// or Command-W), could beat the best so far. It stops at once at the best
// rank, and after the menu in which it finds the exact rank.
func searchMenus(menus [][]menuItem, rank func(string, menuKey) int, confirm menuKey, exact, best int) menuSearch {
	found := menuSearch{menu: -1, at: -1}
	bestRank := 0
	for m, menu := range menus {
		found.menus++
		for i, item := range menu {
			if item.title == "" || rank(item.title, confirm) <= bestRank {
				continue
			}
			found.keys++
			r := rank(item.title, item.key)
			if r <= bestRank {
				continue
			}
			found.title, found.menu, found.at = item.title, m, i
			bestRank = r
			if r == best {
				break
			}
		}
		if bestRank >= exact {
			break
		}
	}
	return found
}

// searchNewWindow and searchCloseWindow search as benchFindNewWindowItem
// and benchFindCloseWindowItem do, with the key that each ranks titles with
// first, and the ranks at which each stops.
func searchNewWindow(menus [][]menuItem) menuSearch {
	return searchMenus(menus, newWindowRank, menuKey{"N", 0}, newWindowExact, newWindowExactKey)
}

func searchCloseWindow(menus [][]menuItem) menuSearch {
	return searchMenus(menus, closeWindowRank, menuKey{"W", 0}, closeWindowExact, closeWindowExactKey)
}

// appMenus are real apps' menus, File menu first, in English and in other
// languages, with the items that should win, and the menus they're in.
var appMenus = []struct {
	name                     string
	menus                    [][]menuItem
	newWindow, close         string
	newWindowMenu, closeMenu int
}{
	{
		name: "Safari",
		menus: [][]menuItem{{
			{"New Window", cmdN}, {"New Private Window", shiftCmdN}, {"New Tab", cmdT},
			{"Close Window", shiftCmdW}, {"Close All Windows", optionShiftW}, {"Close Tab", cmdW},
		}},
		newWindow: "New Window",
		close:     "Close Window",
	},
	{
		name: "Safari in German",
		menus: [][]menuItem{{
			{"Neues Fenster", cmdN}, {"Neues privates Fenster", shiftCmdN}, {"Neuer Tab", cmdT},
			{"Fenster schließen", shiftCmdW}, {"Alle Fenster schließen", optionShiftW}, {"Tab schließen", cmdW},
		}},
		newWindow: "Neues Fenster",
		close:     "Fenster schließen",
	},
	{
		name: "Safari in Japanese",
		menus: [][]menuItem{{
			{"新規ウインドウ", cmdN}, {"新規プライベートウインドウ", shiftCmdN}, {"新規タブ", cmdT},
			{"ウインドウを閉じる", shiftCmdW}, {"タブを閉じる", cmdW},
		}},
		newWindow: "新規ウインドウ",
		close:     "ウインドウを閉じる",
	},
	{
		name: "Mail",
		menus: [][]menuItem{
			{{"New Message", cmdN}, {"New Viewer Window", optionCmdN}, {"Close", cmdW}},
			{{"Undo", menuKey{"Z", 0}}},
		},
		newWindow: "New Viewer Window",
		close:     "Close",
	},
	{
		// Mail titles New Viewer Window as New Window in French.
		name: "Mail in French",
		menus: [][]menuItem{{
			{"Nouveau message", cmdN}, {"Nouvelle fenêtre", optionCmdN}, {"Fermer", cmdW},
		}},
		newWindow: "Nouvelle fenêtre",
		close:     "Fermer",
	},
	{
		// Another New … Window item counts in other languages only with
		// Command-N, so Bench opens Mail again instead.
		name: "Mail in Japanese",
		menus: [][]menuItem{{
			{"新規メッセージ", cmdN}, {"新規ビューアウインドウ", optionCmdN}, {"閉じる", cmdW},
		}},
		newWindow: "",
		close:     "閉じる",
	},
	{
		name: "Notes in Japanese",
		menus: [][]menuItem{{
			{"新規メモ", cmdN}, {"新規フォルダ", shiftCmdN}, {"閉じる", cmdW},
		}},
		newWindow: "",
		close:     "閉じる",
	},
	{
		name: "Finder in French",
		menus: [][]menuItem{{
			{"Nouvelle fenêtre Finder", cmdN}, {"Nouveau dossier", shiftCmdN}, {"Nouvel onglet", cmdT},
			{"Fermer la fenêtre", cmdW},
		}},
		newWindow: "Nouvelle fenêtre Finder",
		close:     "Fermer la fenêtre",
	},
	{
		// TextEdit has Close, not Close Window, and its New makes a
		// document, which doesn't count.
		name: "TextEdit",
		menus: [][]menuItem{
			{{"New", cmdN}, {"Open…", menuKey{"O", 0}}, {"Close", cmdW}, {"Save…", menuKey{"S", 0}}},
			{{"Undo", menuKey{"Z", 0}}, {"Copy", menuKey{"C", 0}}},
			{{"Minimise", menuKey{"M", 0}}, {"Zoom", noKey}},
		},
		newWindow: "",
		close:     "Close",
	},
	{
		name: "TextEdit in Spanish",
		menus: [][]menuItem{{
			{"Nuevo", cmdN}, {"Abrir…", menuKey{"O", 0}}, {"Cerrar", cmdW},
		}},
		newWindow: "",
		close:     "Cerrar",
	},
	{
		// Terminal keeps New Window, which opens a submenu and has no key
		// equivalent, in Shell, in the File menu's place.
		name: "Terminal",
		menus: [][]menuItem{
			{{"New Window", noKey}, {"New Tab", noKey}, {"New Command…", menuKey{"N", 9}}, {"Close Window", shiftCmdW}, {"Close Tab", cmdW}},
			{{"Undo", menuKey{"Z", 0}}, {"Copy", menuKey{"C", 0}}},
			{{"Minimise", menuKey{"M", 0}}, {"Zoom", noKey}},
		},
		newWindow: "New Window",
		close:     "Close Window",
	},
	{
		// Visual Studio Code's New Window is Shift-Command-N, and its
		// Command-N makes a file.
		name: "Visual Studio Code",
		menus: [][]menuItem{
			{{"New Text File", cmdN}, {"New File…", menuKey{"N", 6}}, {"New Window", shiftCmdN}, {"Close Editor", cmdW}, {"Close Window", shiftCmdW}},
			{{"Undo", menuKey{"Z", 0}}},
			{{"Minimise", menuKey{"M", 0}}},
		},
		newWindow: "New Window",
		close:     "Close Window",
	},
	{
		// Of two New Window items, the first wins, even if a later
		// menu's has Command-N: the search ends with the first one's
		// menu.
		name: "New Window in two menus",
		menus: [][]menuItem{
			{{"New Window", noKey}},
			{{"Minimise", menuKey{"M", 0}}, {"New Window", cmdN}},
		},
		newWindow:     "New Window",
		newWindowMenu: 0,
	},
	{
		// Command-N doesn't lift another New … Window item above New
		// Window itself.
		name: "New Window and another with Command-N",
		menus: [][]menuItem{{
			{"New Empty Window", cmdN}, {"New Window", shiftCmdN},
		}},
		newWindow: "New Window",
	},
	{
		// Close doesn't end the search, so Close Window in a later menu
		// wins.
		name: "Close, and Close Window in another menu",
		menus: [][]menuItem{
			{{"Close", cmdW}},
			{{"Minimise", menuKey{"M", 0}}, {"Close Window", shiftCmdW}},
		},
		close:     "Close Window",
		closeMenu: 1,
	},
}

// TestBestMenuItem checks the item that wins in real apps' menus, and the
// menu it's in.
func TestBestMenuItem(t *testing.T) {
	for _, tt := range appMenus {
		got := searchNewWindow(tt.menus)
		if got.title != tt.newWindow || tt.newWindow != "" && got.menu != tt.newWindowMenu {
			t.Errorf("%s: New Window is %q in menu %d, want %q in menu %d", tt.name, got.title, got.menu, tt.newWindow, tt.newWindowMenu)
		}
		got = searchCloseWindow(tt.menus)
		if got.title != tt.close || tt.close != "" && got.menu != tt.closeMenu {
			t.Errorf("%s: Close Window is %q in menu %d, want %q in menu %d", tt.name, got.title, got.menu, tt.close, tt.closeMenu)
		}
	}
	// In one menu, Command-N breaks a tie.
	twice := [][]menuItem{{{"New Window", noKey}, {"New Window", cmdN}}}
	if got := searchNewWindow(twice); got.at != 1 {
		t.Errorf("New Window twice: item %d wins, want 1, the one with Command-N", got.at)
	}
	twice = [][]menuItem{{{"Close", noKey}, {"Close", cmdW}}}
	if got := searchCloseWindow(twice); got.at != 1 {
		t.Errorf("Close twice: item %d wins, want 1, the one with Command-W", got.at)
	}
}

// TestMenuSearchCost checks how much the search reads. Each read is an
// Accessibility call to the app, with the engine's lock held. It reads the
// key equivalents only of items whose titles could win, and no more menus
// once it has read the one with New Window or Close Window.
func TestMenuSearchCost(t *testing.T) {
	tests := []struct {
		app                   string
		newMenus, newKeys     int
		closeMenus, closeKeys int
	}{
		// New Window with Command-N ends the search at once.
		{"Safari", 1, 1, 1, 1},
		// New Window without Command-N ends it after its menu.
		{"Terminal", 1, 1, 1, 1},
		{"Visual Studio Code", 1, 1, 1, 1},
		// With nothing that matches, the search reads every menu, but no
		// key equivalent.
		{"TextEdit", 3, 0, 3, 1},
		{"Mail", 2, 1, 2, 1},
		{"Notes in Japanese", 1, 0, 1, 1},
	}
	for _, tt := range tests {
		var menus [][]menuItem
		for _, a := range appMenus {
			if a.name == tt.app {
				menus = a.menus
			}
		}
		if menus == nil {
			t.Fatalf("%s isn't in appMenus", tt.app)
		}
		if got := searchNewWindow(menus); got.menus != tt.newMenus || got.keys != tt.newKeys {
			t.Errorf("%s: New Window reads %d menus and %d keys, want %d and %d", tt.app, got.menus, got.keys, tt.newMenus, tt.newKeys)
		}
		if got := searchCloseWindow(menus); got.menus != tt.closeMenus || got.keys != tt.closeKeys {
			t.Errorf("%s: Close Window reads %d menus and %d keys, want %d and %d", tt.app, got.menus, got.keys, tt.closeMenus, tt.closeKeys)
		}
	}
}

// TestMenuKeyRaises checks what the native side counts on when it reads an
// item's key equivalent only if the title could win with Command-N, or
// Command-W: a key only ever raises a title's rank, and that key raises it
// most.
func TestMenuKeyRaises(t *testing.T) {
	keys := []menuKey{
		noKey, cmdN, cmdW, cmdT, shiftCmdN, shiftCmdW, optionCmdN, optionShiftW,
		controlCmdN, noCommandN, unknownModsN, lowerCaseCmdN, {"n", -1}, {" N ", 0},
		{"W", 2}, {"w", 0}, {"M", 0}, {"", 0},
	}
	titles := []string{
		"", "New", "New Window", "New window…", "New Finder Window", "New Private Window",
		"New Incognito Window", "New Tab", "New Message", "Close", "Close Window", "Close Tab",
		"Close All", "Window", "Renew Window", "Closed",
	}
	for _, l := range menuLanguages {
		titles = append(titles, l.newWindow, l.closeWindow, l.close, l.newWord, l.windowWord,
			l.newWord+" "+l.windowWord+" Finder", l.newWord+" "+l.private+" "+l.windowWord)
	}
	for _, a := range appMenus {
		for _, menu := range a.menus {
			for _, item := range menu {
				titles = append(titles, item.title)
			}
		}
	}
	ranks := []struct {
		name    string
		rank    func(string, menuKey) int
		confirm menuKey
	}{
		{"newWindowRank", newWindowRank, menuKey{"N", 0}},
		{"closeWindowRank", closeWindowRank, menuKey{"W", 0}},
	}
	for _, r := range ranks {
		for _, title := range titles {
			least, most := r.rank(title, noKey), r.rank(title, r.confirm)
			for _, k := range keys {
				if got := r.rank(title, k); got < least || got > most {
					t.Errorf("%s(%q, %v) = %d, want from %d, with no key, to %d, with %v", r.name, title, k, got, least, most, r.confirm)
				}
			}
		}
	}
}

// TestMenuLanguages checks the table: every title of every language ranks
// as it should, and none is empty or the same as another kind's.
func TestMenuLanguages(t *testing.T) {
	seen := map[string]bool{}
	for _, l := range menuLanguages {
		if seen[l.code] {
			t.Errorf("%s is in menuLanguages twice", l.code)
		}
		seen[l.code] = true
		for _, s := range []string{l.newWindow, l.closeWindow, l.close, l.newWord, l.windowWord, l.private} {
			if menuTitle(s) == "" {
				t.Errorf("%s: an empty title or word in %+v", l.code, l)
			}
		}
		if got := newWindowRank(l.newWindow, noKey); got != newWindowExact {
			t.Errorf("%s: newWindowRank(%q) = %d, want %d", l.code, l.newWindow, got, newWindowExact)
		}
		if got := closeWindowRank(l.closeWindow, noKey); got != closeWindowExact {
			t.Errorf("%s: closeWindowRank(%q) = %d, want %d", l.code, l.closeWindow, got, closeWindowExact)
		}
		if got := closeWindowRank(l.close, noKey); got != closeWindowPlain {
			t.Errorf("%s: closeWindowRank(%q) = %d, want %d", l.code, l.close, got, closeWindowPlain)
		}
		for _, s := range []string{l.newWindow, l.closeWindow, l.close} {
			if newWindowRank(s, noKey) > 0 && closeWindowRank(s, noKey) > 0 {
				t.Errorf("%s: %q ranks as both New Window and Close Window", l.code, s)
			}
		}
	}
}
