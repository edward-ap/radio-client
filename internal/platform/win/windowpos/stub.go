//go:build !windows

// Package windowpos is compiled as a no-op on non-Windows platforms.
package windowpos

import "fyne.io/fyne/v2"

// GetWindowPosition is unavailable off Windows, therefore it always reports
// failure.
func GetWindowPosition(fw fyne.Window) (int, int, bool) {
	return 0, 0, false
}

// ApplyWindowPosition is a stub that returns false on non-Windows platforms.
func ApplyWindowPosition(fw fyne.Window, x, y int) bool {
	return false
}
