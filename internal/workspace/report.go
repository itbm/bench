package workspace

// This file has what Help ▸ Report a Problem… fills in.

// ProblemReport is what Help ▸ Report a Problem… fills in for the user to
// check before they send it: the displays, the layout in use, the
// permissions, and the notices. It leaves out the tabs, windows, and pages.
type ProblemReport struct {
	Screens []ReportScreen
	// Mode is the layout mode (auto, wide, or compact), SizeClass the size
	// class in use (wide or compact), and Density the density in use
	// (standard or compact).
	Mode      string
	SizeClass string
	Density   string
	// Accessibility and ScreenRecording say whether Bench has those
	// permissions.
	Accessibility   bool
	ScreenRecording bool
	// Notices are the notices' texts, as the status bar lists them.
	Notices []string
}

// ReportScreen is a display, in a ProblemReport.
type ReportScreen struct {
	Name string
	// W and H are its size, in points.
	W, H float64
	// Primary is set for the display with the menu bar, and Bench for the
	// one that Bench's window is on.
	Primary bool
	Bench   bool
}

// ProblemReport returns what Help ▸ Report a Problem… fills in.
func (e *Engine) ProblemReport() ProblemReport {
	e.lock()
	defer e.unlock()
	v := e.buildViewLocked()
	r := ProblemReport{
		Mode: v.Mode, SizeClass: v.SizeClass, Density: v.Density,
		Accessibility: v.Permissions.Accessibility, ScreenRecording: v.Permissions.ScreenRecording,
	}
	for _, s := range e.p.Screens() {
		r.Screens = append(r.Screens, ReportScreen{
			Name: s.Name, W: s.Frame.W, H: s.Frame.H, Primary: s.Primary, Bench: s.ID != "" && s.ID == e.screen.ID,
		})
	}
	for _, n := range v.Notices {
		r.Notices = append(r.Notices, n.Text)
	}
	return r
}
