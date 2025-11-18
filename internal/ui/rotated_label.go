package ui

import (
	"image"
	"image/color"
	"image/draw"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/theme"
	"golang.org/x/image/font"
	"golang.org/x/image/font/basicfont"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/math/fixed"
)

// RotatedLabel is a lightweight widget that shows text rotated 90° clockwise.
// It rasterizes once on creation (or when explicitly re-rendered) and reuses
// the cached image to avoid per-frame work. Intended for short static labels.
type RotatedLabel struct {
	text      string
	col       color.Color
	img       *canvas.Image
	target    fyne.Size
	hasTarget bool
}

// NewRotatedStaticLabel creates a rotated label with the current theme colors.
func NewRotatedStaticLabel(text string) *RotatedLabel {
	r := &RotatedLabel{text: text, col: theme.ForegroundColor()}
	r.render()
	return r
}

// CanvasObject exposes the underlying canvas.Image for layout containers.
func (r *RotatedLabel) CanvasObject() fyne.CanvasObject { return r.img }

// SetTargetSize sets a preferred size for the rotated label image and enables
// proportional scaling inside that box. Useful to fit into limited height.
func (r *RotatedLabel) SetTargetSize(w, h float32) {
	r.target = fyne.NewSize(w, h)
	r.hasTarget = true
	if r.img == nil {
		return
	}
	r.img.FillMode = canvas.ImageFillContain
	r.img.SetMinSize(r.target)
}

func (r *RotatedLabel) render() {
	face := r.pickFace()
	if closer, ok := face.(interface{ Close() error }); ok {
		defer closer.Close()
	}

	// measure text and add generous padding to avoid glyph clipping
	d := &font.Drawer{Face: face}
	adv := d.MeasureString(r.text)
	pad := 8
	w := int(adv>>6) + pad
	metrics := face.Metrics()
	h := int((metrics.Ascent+metrics.Descent)>>6) + pad
	if w < 2 {
		w = 2
	}
	if h < 2 {
		h = 2
	}

	src := image.NewRGBA(image.Rect(0, 0, w, h))
	d.Dst = src
	d.Src = image.NewUniform(color.NRGBAModel.Convert(r.col))
	// draw with symmetric padding to keep glyphs fully inside the box
	d.Dot = fixed.P(pad/2, int(metrics.Ascent>>6)+pad/2)
	d.DrawString(r.text)

	// rotate 90 CCW into dst (dst size = h x w)
	dst := image.NewRGBA(image.Rect(0, 0, h, w))
	for dy := 0; dy < w; dy++ { // dy = dstY
		for dx := 0; dx < h; dx++ { // dx = dstX
			sx := (w - 1) - dy // srcX = w-1-dstY
			sy := dx           // srcY = dstX
			dst.Set(dx, dy, src.RGBAAt(sx, sy))
		}
	}
	out := image.NewRGBA(dst.Bounds())
	draw.Draw(out, out.Bounds(), dst, image.Point{}, draw.Src)
	img := canvas.NewImageFromImage(out)
	// Allow image to scale to its allocated box (prevents disappearance in narrow cells)
	img.FillMode = canvas.ImageFillContain
	if r.hasTarget {
		img.SetMinSize(r.target)
	} else {
		// Default to the natural bitmap size so the label stays visible without manual sizing.
		img.SetMinSize(fyne.NewSize(float32(out.Bounds().Dx()), float32(out.Bounds().Dy())))
	}
	r.img = img
}

// pickFace loads the current theme font and falls back to a bitmap face when unavailable.
func (r *RotatedLabel) pickFace() font.Face {
	res := theme.TextFont()
	targetPt := float64(theme.TextSize())
	if targetPt <= 0 {
		targetPt = 14
	}
	scale := currentScale()
	if scale <= 0 {
		scale = 1
	}
	targetPt *= scale * 0.75
	if targetPt < 6 {
		targetPt = 6
	}
	if res != nil {
		if data := res.Content(); len(data) > 0 {
			if ttf, err := opentype.Parse(data); err == nil {
				if face, err := opentype.NewFace(ttf, &opentype.FaceOptions{Size: targetPt, DPI: 96, Hinting: font.HintingFull}); err == nil {
					return face
				}
			}
		}
	}
	return basicfont.Face7x13
}
