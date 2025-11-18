package ui

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/theme"
)

// smallThumbTheme is a theme wrapper that reduces the inline icon size, which
// Fyne uses for the slider thumb, making it approximately half size.
type smallThumbTheme struct{ fyne.Theme }

func (t smallThumbTheme) Size(n fyne.ThemeSizeName) float32 {
	if n == theme.SizeNameInlineIcon {
		base := t.Theme.Size(n)
		return base * 0.5
	}
	return t.Theme.Size(n)
}

// UseSmallThumbTheme applies the theme wrapper to the current app.
func UseSmallThumbTheme() {
	app := fyne.CurrentApp()
	if app == nil {
		return
	}
	app.Settings().SetTheme(smallThumbTheme{Theme: app.Settings().Theme()})
}
