package ui

import (
	"context"
	"sync"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/data/binding"
	"fyne.io/fyne/v2/widget"
)

// TickerController manages ticker text and conditional smooth scroll when
// overflow occurs. SetText is safe to call from any goroutine.
type TickerController struct {
	lbl      *widget.Label
	parent   fyne.CanvasObject // container used to measure visible width
	mu       sync.Mutex
	cancel   context.CancelFunc
	lastText string

	bind binding.String // thread-safe string binding to update label from any goroutine

	// tuning
	speed   time.Duration // interval between steps
	padding string        // spaces padding for cyclic scroll
}

// NewTickerController creates a controller for the given label and measuring parent.
func NewTickerController(lbl *widget.Label, parent fyne.CanvasObject) *TickerController {
	b := binding.NewString()
	lbl.Bind(b)
	_ = b.Set("Ready")
	return &TickerController{
		lbl:     lbl,
		parent:  parent,
		bind:    b,
		speed:   120 * time.Millisecond,
		padding: "   ",
	}
}

// Close stops any scrolling goroutine.
func (tc *TickerController) Close() {
	tc.mu.Lock()
	if tc.cancel != nil {
		tc.cancel()
		tc.cancel = nil
	}
	tc.mu.Unlock()
}

// SetText updates the ticker text and animates scrolling if it no longer fits.
func (tc *TickerController) SetText(text string) {
	if text == "" {
		text = "Streaming."
	}

	// stop previous scroller
	tc.mu.Lock()
	if tc.cancel != nil {
		tc.cancel()
		tc.cancel = nil
	}
	tc.lastText = text
	lbl := tc.lbl
	parent := tc.parent
	speed := tc.speed
	padding := tc.padding
	b := tc.bind
	tc.mu.Unlock()

	// render statically
	_ = b.Set(text)

	// measure overflow once on original text
	visibleW := parent.Size().Width
	textW := measureLabelTextWidth(lbl, text)
	if !tickerNeedsScroll(textW, visibleW) {
		return // fits; no scroll
	}
	neededW := textW

	// start scroll goroutine
	ctx, cancel := context.WithCancel(context.Background())
	tc.mu.Lock()
	tc.cancel = cancel
	orig := tc.lastText
	tc.mu.Unlock()

	go func() {
		workRunes := []rune(padding + orig + padding)
		if len(workRunes) == 0 {
			return
		}
		offset := 0
		for {
			select {
			case <-ctx.Done():
				return
			case <-time.After(speed):
				// detect changes: text or width changed to fit
				tc.mu.Lock()
				stopped := tc.cancel == nil
				current := tc.lastText
				tc.mu.Unlock()
				if stopped || current != orig {
					return
				}
				if !tickerNeedsScroll(neededW, parent.Size().Width) { // now fits -> stop
					return
				}

				// rotate left by one rune
				offset = (offset + 1) % len(workRunes)
				var display string
				if offset == 0 {
					display = string(workRunes)
				} else {
					display = string(workRunes[offset:]) + string(workRunes[:offset])
				}
				_ = b.Set(display)
			}
		}
	}()
}

// measureLabelTextWidth estimates the width the label would need for the text.
func measureLabelTextWidth(lbl *widget.Label, text string) float32 {
	if lbl == nil {
		return 0
	}
	tmp := widget.NewLabel(text)
	tmp.Alignment = lbl.Alignment
	tmp.TextStyle = lbl.TextStyle
	tmp.Importance = lbl.Importance
	tmp.Wrapping = lbl.Wrapping
	tmp.Truncation = lbl.Truncation
	tmp.Refresh()
	return tmp.MinSize().Width
}
