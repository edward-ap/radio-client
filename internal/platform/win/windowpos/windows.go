//go:build windows

// Package windowpos provides helpers to persist and restore native window
// coordinates on Windows builds where fyne does not expose them directly.
package windowpos

import (
	"sync"
	"syscall"
	"unsafe"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/driver"
)

var (
	user32            = syscall.NewLazyDLL("user32.dll")
	procGetWindowRect = user32.NewProc("GetWindowRect")
	procSetWindowPos  = user32.NewProc("SetWindowPos")
)

type winRect struct {
	Left   int32
	Top    int32
	Right  int32
	Bottom int32
}

const (
	swpNOSIZE     = 0x0001
	swpNOZORDER   = 0x0004
	swpNOACTIVATE = 0x0010
)

// GetWindowPosition returns the top-left corner of the native HWND associated
// with the provided fyne window. The bool indicates whether a coordinate was
// successfully retrieved.
func GetWindowPosition(w fyne.Window) (int, int, bool) {
	var px, py int
	ok := withNativeHWND(w, func(hwnd uintptr) bool {
		var rect winRect
		ret, _, err := procGetWindowRect.Call(hwnd, uintptr(unsafe.Pointer(&rect)))
		if ret == 0 {
			if err != syscall.Errno(0) {
				fyne.LogError("GetWindowRect failed", err)
			}
			return false
		}
		px = int(rect.Left)
		py = int(rect.Top)
		return true
	})
	return px, py, ok
}

// ApplyWindowPosition moves the native HWND to the given coordinates without
// resizing or changing the Z-order. Returns true on success.
func ApplyWindowPosition(w fyne.Window, x, y int) bool {
	return withNativeHWND(w, func(hwnd uintptr) bool {
		ret, _, err := procSetWindowPos.Call(hwnd, 0, uintptr(int32(x)), uintptr(int32(y)), 0, 0, swpNOSIZE|swpNOZORDER|swpNOACTIVATE)
		if ret == 0 {
			if err != syscall.Errno(0) {
				fyne.LogError("SetWindowPos failed", err)
			}
			return false
		}
		return true
	})
}

// withNativeHWND obtains the underlying Windows HWND for the provided fyne
// window and executes fn on the GUI thread. It waits for completion before
// returning and passes through the handler's boolean result.
func withNativeHWND(w fyne.Window, fn func(hwnd uintptr) bool) bool {
	nw, ok := w.(driver.NativeWindow)
	if !ok {
		return false
	}
	var (
		success bool
		wg      sync.WaitGroup
	)
	wg.Add(1)
	nw.RunNative(func(ctx any) {
		defer wg.Done()
		winCtx, ok := ctx.(driver.WindowsWindowContext)
		if !ok || winCtx.HWND == 0 {
			return
		}
		success = fn(winCtx.HWND)
	})
	wg.Wait()
	return success
}
