package ui

import (
	"math"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

// MiniThumbSlider is a simple horizontal slider widget with a thumb drawn at
// half the usual size compared to the standard Fyne slider.
type MiniThumbSlider struct {
	widget.BaseWidget
	Min       float64
	Max       float64
	Step      float64
	Value     float64
	OnChanged func(float64)
}

// NewMiniThumbSlider creates a horizontal slider constrained to [min, max].
func NewMiniThumbSlider(min, max float64) *MiniThumbSlider {
	s := &MiniThumbSlider{Min: min, Max: max, Step: 1}
	s.ExtendBaseWidget(s)
	return s
}

func (s *MiniThumbSlider) CreateRenderer() fyne.WidgetRenderer {
	r := &miniSliderRenderer{
		s:     s,
		track: canvas.NewRectangle(theme.ShadowColor()),
		fill:  canvas.NewRectangle(theme.PrimaryColor()),
		thumb: canvas.NewCircle(theme.ForegroundColor()),
	}
	r.objs = []fyne.CanvasObject{r.track, r.fill, r.thumb}
	return r
}

// SetValue sets the slider value and triggers refresh and callback.
func (s *MiniThumbSlider) SetValue(v float64) {
	if s.Max <= s.Min {
		return
	}
	newValue := normalizeSliderValue(s.Min, s.Max, s.Step, v)
	if newValue == s.Value {
		return
	}
	s.Value = newValue
	s.Refresh()
	if s.OnChanged != nil {
		s.OnChanged(newValue)
	}
}

func normalizeSliderValue(min, max, step, value float64) float64 {
	if max <= min {
		return min
	}
	v := clampFloat64(value, min, max)
	if step > 0 {
		span := max - min
		if span > 0 {
			steps := (v - min) / step
			n := math.Round(steps)
			v = clampFloat64(min+n*step, min, max)
		}
	}
	return v
}

// Dragged updates the value based on pointer drag position.
func (s *MiniThumbSlider) Dragged(e *fyne.DragEvent) {
	s.updateFromPos(e.Position.X, s.Size().Width)
}

func (s *MiniThumbSlider) DragEnd() {}

// Tapped moves the thumb to the tapped position.
func (s *MiniThumbSlider) Tapped(e *fyne.PointEvent) {
	s.updateFromPos(e.Position.X, s.Size().Width)
}

// Scrolled adjusts the slider value using mouse wheel input.
func (s *MiniThumbSlider) Scrolled(ev *fyne.ScrollEvent) {
	if ev == nil {
		return
	}
	step := s.Step
	if step <= 0 {
		step = 1
	}
	if ev.Scrolled.DY > 0 {
		s.SetValue(s.Value + step)
	} else if ev.Scrolled.DY < 0 {
		s.SetValue(s.Value - step)
	}
}

func (s *MiniThumbSlider) updateFromPos(px float32, w float32) {
	if w <= 0 || s.Max <= s.Min {
		return
	}
	frac := float64(px / w)
	if frac < 0 {
		frac = 0
	}
	if frac > 1 {
		frac = 1
	}
	val := s.Min + frac*(s.Max-s.Min)
	s.SetValue(val)
	// ensure visual update immediately
	s.Refresh()
}

// MinSize provides a reasonable touch target height.
func (s *MiniThumbSlider) MinSize() fyne.Size {
	// keep height large enough for comfortable interaction
	return fyne.NewSize(100, theme.IconInlineSize())
}

// Refresh ensures the renderer picks up theme changes.
func (s *MiniThumbSlider) Refresh() {
	// Colors are bound to theme during renderer Layout/Refresh; just propagate.
	s.BaseWidget.Refresh()
}

// VerticalSlider is a compact vertical slider used in the Equalizer drawer.
type VerticalSlider struct {
	widget.BaseWidget
	Min       float64
	Max       float64
	Step      float64
	Value     float64
	OnChanged func(float64)
}

// NewVerticalSlider creates a vertical slider with a default 0.5 step.
func NewVerticalSlider(min, max float64) *VerticalSlider {
	s := &VerticalSlider{Min: min, Max: max, Step: 0.5}
	s.ExtendBaseWidget(s)
	return s
}

func (s *VerticalSlider) CreateRenderer() fyne.WidgetRenderer {
	r := &verticalSliderRenderer{
		s:     s,
		track: canvas.NewRectangle(theme.ShadowColor()),
		fill:  canvas.NewRectangle(theme.PrimaryColor()),
		thumb: canvas.NewCircle(theme.ForegroundColor()),
	}
	r.objs = []fyne.CanvasObject{r.track, r.fill, r.thumb}
	return r
}

func (s *VerticalSlider) clampStep(v float64) float64 {
	if s.Max <= s.Min {
		return s.Min
	}
	if v < s.Min {
		v = s.Min
	}
	if v > s.Max {
		v = s.Max
	}
	if s.Step > 0 {
		span := s.Max - s.Min
		if span > 0 {
			steps := (v - s.Min) / s.Step
			n := float64(int(steps + 0.5))
			v = s.Min + n*s.Step
			if v > s.Max {
				v = s.Max
			}
		}
	}
	return v
}

func (s *VerticalSlider) SetValue(v float64) {
	v = s.clampStep(v)
	if v == s.Value {
		return
	}
	s.Value = v
	s.Refresh()
	if s.OnChanged != nil {
		s.OnChanged(v)
	}
}

// Dragged updates value based on Y position (top=max, bottom=min).
func (s *VerticalSlider) Dragged(e *fyne.DragEvent) {
	s.updateFromPos(e.Position.Y, s.Size().Height)
}

func (s *VerticalSlider) DragEnd() {}

// Tapped moves the thumb to the tapped position.
func (s *VerticalSlider) Tapped(e *fyne.PointEvent) { s.updateFromPos(e.Position.Y, s.Size().Height) }

func (s *VerticalSlider) updateFromPos(py float32, h float32) {
	if h <= 0 || s.Max <= s.Min {
		return
	}
	// invert: y=0 -> max
	frac := 1 - float64(py/h)
	if frac < 0 {
		frac = 0
	}
	if frac > 1 {
		frac = 1
	}
	val := s.Min + frac*(s.Max-s.Min)
	s.SetValue(val)
	s.Refresh()
}

func (s *VerticalSlider) MinSize() fyne.Size {
	// narrow width, tall height by default
	w := theme.IconInlineSize() // ~24
	if w < 20 {
		w = 20
	}
	return fyne.NewSize(w, 180)
}

func (s *VerticalSlider) Refresh() { s.BaseWidget.Refresh() }

type verticalSliderRenderer struct {
	s     *VerticalSlider
	track *canvas.Rectangle
	fill  *canvas.Rectangle
	thumb *canvas.Circle
	objs  []fyne.CanvasObject
}

func (r *verticalSliderRenderer) Layout(sz fyne.Size) {
	// track centered horizontally
	trackW := float32(4)
	x := (sz.Width - trackW) / 2
	r.track.Move(fyne.NewPos(x, 0))
	r.track.Resize(fyne.NewSize(trackW, sz.Height))

	// fill height proportionate to value (from bottom up)
	span := r.s.Max - r.s.Min
	frac := float32(0)
	if span > 0 {
		frac = float32((r.s.Value - r.s.Min) / span)
	}
	if frac < 0 {
		frac = 0
	}
	if frac > 1 {
		frac = 1
	}
	fillH := sz.Height * frac
	r.fill.Move(fyne.NewPos(x, sz.Height-fillH))
	r.fill.Resize(fyne.NewSize(trackW, fillH))

	// small thumb circle
	thumbR := theme.IconInlineSize() / 4
	cy := sz.Height - fillH
	if cy < float32(thumbR) {
		cy = float32(thumbR)
	}
	if cy > sz.Height-float32(thumbR) {
		cy = sz.Height - float32(thumbR)
	}
	cx := sz.Width / 2
	r.thumb.Resize(fyne.NewSize(thumbR*2, thumbR*2))
	r.thumb.Move(fyne.NewPos(cx-float32(thumbR), cy-float32(thumbR)))
}

func (r *verticalSliderRenderer) MinSize() fyne.Size { return r.s.MinSize() }

func (r *verticalSliderRenderer) Refresh() {
	r.Layout(r.s.Size())
	canvas.Refresh(r.track)
	canvas.Refresh(r.fill)
	canvas.Refresh(r.thumb)
}

func (r *verticalSliderRenderer) Destroy() {}

func (r *verticalSliderRenderer) Objects() []fyne.CanvasObject { return r.objs }

// --- Horizontal mini slider renderer (for MiniThumbSlider) ---

type miniSliderRenderer struct {
	s     *MiniThumbSlider
	track *canvas.Rectangle
	fill  *canvas.Rectangle
	thumb *canvas.Circle
	objs  []fyne.CanvasObject
}

func (r *miniSliderRenderer) Layout(sz fyne.Size) {
	// track centered vertically
	trackH := float32(4)
	y := (sz.Height - trackH) / 2
	r.track.Move(fyne.NewPos(0, y))
	r.track.Resize(fyne.NewSize(sz.Width, trackH))

	// fill width proportionate to value (left to right)
	span := r.s.Max - r.s.Min
	frac := float32(0)
	if span > 0 {
		frac = float32((r.s.Value - r.s.Min) / span)
	}
	if frac < 0 {
		frac = 0
	}
	if frac > 1 {
		frac = 1
	}
	fillW := sz.Width * frac
	r.fill.Move(fyne.NewPos(0, y))
	r.fill.Resize(fyne.NewSize(fillW, trackH))

	// small thumb circle centered on the fill end
	thumbR := theme.IconInlineSize() / 4
	cx := fillW
	if cx < float32(thumbR) {
		cx = float32(thumbR)
	}
	if cx > sz.Width-float32(thumbR) {
		cx = sz.Width - float32(thumbR)
	}
	cy := sz.Height / 2
	r.thumb.Resize(fyne.NewSize(thumbR*2, thumbR*2))
	r.thumb.Move(fyne.NewPos(cx-float32(thumbR), cy-float32(thumbR)))
}

func (r *miniSliderRenderer) MinSize() fyne.Size { return r.s.MinSize() }

func (r *miniSliderRenderer) Refresh() {
	r.Layout(r.s.Size())
	canvas.Refresh(r.track)
	canvas.Refresh(r.fill)
	canvas.Refresh(r.thumb)
}

func (r *miniSliderRenderer) Destroy() {}

func (r *miniSliderRenderer) Objects() []fyne.CanvasObject { return r.objs }
