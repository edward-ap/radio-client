// Package ui contains small fyne widgets and helpers shared across the app.
package ui

import "fyne.io/fyne/v2"

type runOnMainDriver interface {
	RunOnMain(func())
}

type callOnMainDriver interface {
	CallOnMain(func())
}

// CallOnMain dispatches f onto the UI thread if the current Fyne driver
// supports it; otherwise executes f inline (best-effort fallback).
func CallOnMain(f func()) {
	if f == nil {
		return
	}
	app := fyne.CurrentApp()
	if app == nil {
		f()
		return
	}
	drv := app.Driver()
	if drv == nil {
		f()
		return
	}
	if r, ok := drv.(runOnMainDriver); ok {
		r.RunOnMain(f)
		return
	}
	if c, ok := drv.(callOnMainDriver); ok {
		c.CallOnMain(f)
		return
	}
	f()
}

// currentScale returns the current UI scale, defaulting to 1 when unavailable.
func currentScale() float64 {
	app := fyne.CurrentApp()
	if app == nil {
		return 1
	}
	set := app.Settings()
	if set == nil {
		return 1
	}
	if sc := set.Scale(); sc > 0 {
		return float64(sc)
	}
	return 1
}

// clampFloat64 constrains v to the [min, max] interval.
func clampFloat64(v, min, max float64) float64 {
	if max <= min {
		return min
	}
	if v < min {
		return min
	}
	if v > max {
		return max
	}
	return v
}

const tickerWidthEpsilon float32 = 0.5

// tickerNeedsScroll decides whether marquee scrolling is required.
func tickerNeedsScroll(textWidth, viewportWidth float32) bool {
	if textWidth <= 0 {
		return false
	}
	if viewportWidth < 0 {
		viewportWidth = 0
	}
	return textWidth-viewportWidth > tickerWidthEpsilon
}
