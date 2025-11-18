package ui

import (
	"image/color"
	"math"
	"sync/atomic"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/layout"
)

// StreamIndicator is a tiny circular indicator that breathes through green hues
// while streaming is active.
type StreamIndicator struct {
	wrap   *fyne.Container
	circle *canvas.Circle
	on     atomic.Bool
	hue    float64 // 0..360
}

// NewStreamIndicator constructs a StreamIndicator with the given diameter.
func NewStreamIndicator(diameter float32) *StreamIndicator {
	c := canvas.NewCircle(color.NRGBA{0x80, 0x80, 0x80, 0xFF}) // gray by default (stopped)
	c.StrokeColor = color.NRGBA{0, 0, 0, 0}
	// Only the circle centered inside a fixed-size holder; no local background here
	inner := container.New(layout.NewGridWrapLayout(fyne.NewSize(diameter, diameter)), c)
	wrap := container.NewCenter(inner)
	return &StreamIndicator{wrap: wrap, circle: c}
}

// CanvasObject returns the fyne object suitable for embedding in layouts.
func (s *StreamIndicator) CanvasObject() fyne.CanvasObject { return s.wrap }

// SetActive toggles the pulsating animation and color state.
func (s *StreamIndicator) SetActive(on bool) {
	prev := s.on.Load()
	s.on.Store(on)
	if on && !prev {
		go s.animate()
	} else if !on {
		// reset hue and return to dark gray on main thread
		s.hue = 0
		CallOnMain(func() {
			s.circle.FillColor = color.NRGBA{0x80, 0x80, 0x80, 0xFF}
			s.circle.Refresh()
		})
	}
}

func (s *StreamIndicator) animate() {
	t := time.NewTicker(90 * time.Millisecond)
	defer t.Stop()
	for s.on.Load() {
		<-t.C
		// cycle through greenish hues for a subtle breathing effect
		s.hue += 8
		if s.hue >= 360 {
			s.hue = 0
		}
		col := hsvToNRGBA(s.hue, 0.65, 0.95)
		CallOnMain(func() {
			s.circle.FillColor = col
			s.circle.Refresh()
		})
	}
}

// hsvToNRGBA converts HSV (0..360, 0..1, 0..1) to color.NRGBA.
func hsvToNRGBA(h, s, v float64) color.NRGBA {
	c := v * s
	x := c * (1 - math.Abs(math.Mod(h/60.0, 2)-1))
	m := v - c
	var r, g, b float64
	switch {
	case h < 60:

		r, g, b = c, x, 0
	case h < 120:

		r, g, b = x, c, 0
	case h < 180:

		r, g, b = 0, c, x
	case h < 240:

		r, g, b = 0, x, c
	case h < 300:
		r, g, b = x, 0, c
	default:

		r, g, b = c, 0, x
	}
	return color.NRGBA{
		R: uint8((r+m)*255 + 0.5),
		G: uint8((g+m)*255 + 0.5),
		B: uint8((b+m)*255 + 0.5),
		A: 0xFF,
	}
}
