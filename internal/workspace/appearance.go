package workspace

// This file has the appearance setting: whether Bench's own windows are
// dark, light, or as macOS is. The shell gives its windows the appearance
// (Emitter.Appearance), and the frontend its colours, from the View's
// settings. Web pages and the apps in the panes follow macOS whatever the
// setting, as they would outside Bench.

// appearanceChangedLocked returns the appearance setting, and whether the
// shell has yet to hear it: with the first view, and after it changes.
func (e *Engine) appearanceChangedLocked() (string, bool) {
	look := e.cfg.Settings.Appearance
	if e.appearanceSent && look == e.appearance {
		return look, false
	}
	e.appearance, e.appearanceSent = look, true
	return look, true
}
