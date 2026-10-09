// Package draw renders a laid-out template as the PNG the Supernote imports.
//
// The output deliberately matches the form the device writes its own templates
// in: 1920x2560, 8-bit greyscale (PNG colour type 0), non-interlaced and carrying
// no ancillary chunks at all — just IHDR, IDAT and IEND. That matters beyond
// tidiness: snorg selects a template by the sha256 of the page's decoded
// background, so any extra chunk or a needless trip through RGBA would change the
// bytes the archive must match. Drawing into an image.Gray and encoding it with
// image/png produces exactly that shape.
//
// Everything here paints background: dots, frames, captions, the digits in a date
// cell. snorg's canonical page render resolves no background image, so none of it
// can ever be read as ink — which is why a widget may be decorated freely without
// disturbing what snif measures inside it.
package draw

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/png"

	"golang.org/x/image/font"
	"golang.org/x/image/font/gofont/gobold"
	"golang.org/x/image/font/gofont/goregular"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/math/fixed"

	"github.com/jdlugosz963/snif/internal/layout"
	"github.com/jdlugosz963/snif/internal/spec"
)

// The greyscale palette. Frames and glyphs stay well short of black so the
// handwriting laid over them is the darkest thing on the page, and decoration
// stays light enough to write across.
var (
	white = color.Gray{Y: 0xff}
	frame = color.Gray{Y: 0x55}
	text  = color.Gray{Y: 0x33}
	faint = color.Gray{Y: 0xc4}
	title = color.Gray{Y: 0x11}
)

// Pattern spacing, in pixels.
const (
	dotStep  = 40
	gridStep = 40
	lineStep = 60
)

// Render draws the page and returns it as PNG bytes.
func Render(p *layout.Page) ([]byte, error) {
	img, err := Image(p)
	if err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return nil, fmt.Errorf("encode png: %w", err)
	}
	return buf.Bytes(), nil
}

// Image draws the page onto a greyscale canvas.
func Image(p *layout.Page) (*image.Gray, error) {
	f, err := newFonts()
	if err != nil {
		return nil, err
	}
	img := image.NewGray(image.Rect(0, 0, spec.PageW, spec.PageH))
	for i := range img.Pix {
		img.Pix[i] = white.Y
	}
	pattern(img, spec.Rect{W: spec.PageW, H: spec.PageH}, p.Theme)

	if p.Title != "" {
		r := p.TitleAt
		drawText(img, f.title, title, r.X, r.Y+64, p.Title)
		fillRect(img, spec.Rect{X: r.X, Y: r.Y + r.H - 12, W: r.W, H: 3}, frame)
	}
	for _, w := range p.Widgets {
		if err := widget(img, f, w); err != nil {
			return nil, err
		}
	}
	for _, r := range p.Regions {
		region(img, f, r)
	}
	return img, nil
}

// region draws a figure area: its caption and a frame to draw inside. The frame
// is the only decoration — a figure box is a place to draw, not a field to fill,
// so anything printed in it would end up in the crop a consumer takes.
func region(img *image.Gray, f *fonts, r layout.Region) {
	if !r.Caption.Empty() && r.Label != "" {
		drawText(img, f.caption, text, r.Caption.X, r.Caption.Y+r.Caption.H-18, r.Label)
	}
	clear(img, r.Rect, r.Style)
	strokeRect(img, r.Rect, 2, frame)
}

func widget(img *image.Gray, f *fonts, w layout.Widget) error {
	if !w.Caption.Empty() && w.Label != "" {
		// A check's caption sits beside its box, vertically centred on it; every
		// other caption sits above its body, on its own line.
		y := w.Caption.Y + w.Caption.H - 18
		if w.Kind == spec.Check || w.Kind == spec.Button {
			y = w.Caption.Y + (w.Caption.H+capHeight(f.caption))/2
		}
		drawText(img, f.caption, text, w.Caption.X, y, w.Label)
	}

	switch w.Kind {
	case spec.Check:
		clear(img, w.Rect, w.Style)
		strokeRect(img, w.Rect, 3, frame)
	case spec.Button:
		clear(img, w.Rect, w.Style)
		// A button reads as a button: a doubled frame, so a page shows at a glance
		// which box means "do this" and which means "this is true".
		strokeRect(img, w.Rect, 3, frame)
		strokeRect(img, inset(w.Rect, 8), 2, faint)
	case spec.Input:
		clear(img, w.Rect, w.Style)
		strokeRect(img, w.Rect, 2, frame)
	case spec.Slider:
		clear(img, w.Rect, w.Style)
		strokeRect(img, w.Rect, 2, frame)
		for _, t := range w.Ticks {
			fillRect(img, spec.Rect{X: clampX(t.X), Y: w.Rect.Y + w.Rect.H, W: 2, H: 12}, frame)
			drawCentered(img, f.scale, text, spec.Rect{
				X: t.X - 60, Y: w.Rect.Y + w.Rect.H + 14, W: 120, H: 28,
			}, t.Label)
		}
	case spec.Radio, spec.Date:
		for _, c := range w.Cells {
			clear(img, c.Rect, w.Style)
			strokeRect(img, c.Rect, 2, frame)
			drawCentered(img, f.cell, faintText(c), c.Rect, c.Label)
		}
	default:
		return fmt.Errorf("draw: unknown widget kind %q", w.Kind)
	}
	return nil
}

// faintText keeps a cell's printed glyph lighter than a caption: it is a label to
// aim at, not something to read, and the tick goes straight over it.
func faintText(spec.Cell) color.Gray { return color.Gray{Y: 0x66} }

// clear gives a measured area its own background: the page's theme is wiped out
// of it and the widget's own style (if any) drawn in its place. A box meant to be
// written in starts clean, so the page can carry a dense pattern without making
// its tick boxes noisy — and a widget that asks for lines gets lines alone.
func clear(img *image.Gray, r spec.Rect, style layout.Theme) {
	fillRect(img, r, white)
	pattern(img, inset(r, 6), style)
}

// pattern fills r with a theme. Blank (and the empty theme) draws nothing, so a
// widget with no style keeps the page's own.
func pattern(img *image.Gray, r spec.Rect, t layout.Theme) {
	switch t {
	case layout.Dots:
		for y := r.Y + dotStep; y < r.Y+r.H; y += dotStep {
			for x := r.X + dotStep; x < r.X+r.W; x += dotStep {
				fillRect(img, spec.Rect{X: x, Y: y, W: 3, H: 3}, faint)
			}
		}
	case layout.Grid:
		for y := r.Y + gridStep; y < r.Y+r.H; y += gridStep {
			fillRect(img, spec.Rect{X: r.X, Y: y, W: r.W, H: 1}, faint)
		}
		for x := r.X + gridStep; x < r.X+r.W; x += gridStep {
			fillRect(img, spec.Rect{X: x, Y: r.Y, W: 1, H: r.H}, faint)
		}
	case layout.Lines:
		for y := r.Y + lineStep; y < r.Y+r.H; y += lineStep {
			fillRect(img, spec.Rect{X: r.X, Y: y, W: r.W, H: 1}, faint)
		}
	}
}

func fillRect(img *image.Gray, r spec.Rect, c color.Gray) {
	b := img.Bounds()
	for y := max(r.Y, b.Min.Y); y < min(r.Y+r.H, b.Max.Y); y++ {
		for x := max(r.X, b.Min.X); x < min(r.X+r.W, b.Max.X); x++ {
			img.SetGray(x, y, c)
		}
	}
}

// strokeRect draws r's outline inward, so the stroke never leaves the rect.
func strokeRect(img *image.Gray, r spec.Rect, w int, c color.Gray) {
	fillRect(img, spec.Rect{X: r.X, Y: r.Y, W: r.W, H: w}, c)
	fillRect(img, spec.Rect{X: r.X, Y: r.Y + r.H - w, W: r.W, H: w}, c)
	fillRect(img, spec.Rect{X: r.X, Y: r.Y, W: w, H: r.H}, c)
	fillRect(img, spec.Rect{X: r.X + r.W - w, Y: r.Y, W: w, H: r.H}, c)
}

func inset(r spec.Rect, n int) spec.Rect {
	return spec.Rect{X: r.X + n, Y: r.Y + n, W: r.W - 2*n, H: r.H - 2*n}
}

func clampX(x int) int { return min(max(x, 0), spec.PageW-2) }

// drawText draws s with its baseline at (x, y).
func drawText(img *image.Gray, face font.Face, c color.Gray, x, y int, s string) {
	d := &font.Drawer{
		Dst: img, Src: image.NewUniform(c), Face: face,
		Dot: fixed.P(x, y),
	}
	d.DrawString(s)
}

// drawCentered draws s centred in r, both ways.
func drawCentered(img *image.Gray, face font.Face, c color.Gray, r spec.Rect, s string) {
	w := font.MeasureString(face, s).Round()
	x := r.X + (r.W-w)/2
	y := r.Y + (r.H+capHeight(face))/2
	drawText(img, face, c, x, y, s)
}

// capHeight approximates a face's cap height, for optical vertical centring.
func capHeight(face font.Face) int {
	m := face.Metrics()
	return m.CapHeight.Round()
}

// fonts are the four sizes a template is drawn in, built once per render.
type fonts struct{ title, caption, cell, scale font.Face }

func newFonts() (*fonts, error) {
	reg, err := opentype.Parse(goregular.TTF)
	if err != nil {
		return nil, fmt.Errorf("parse font: %w", err)
	}
	bold, err := opentype.Parse(gobold.TTF)
	if err != nil {
		return nil, fmt.Errorf("parse font: %w", err)
	}
	face := func(f *opentype.Font, size float64) (font.Face, error) {
		return opentype.NewFace(f, &opentype.FaceOptions{Size: size, DPI: 72, Hinting: font.HintingFull})
	}
	var fs fonts
	for _, step := range []struct {
		dst  *font.Face
		f    *opentype.Font
		size float64
	}{
		{&fs.title, bold, 60},
		{&fs.caption, bold, 36},
		{&fs.cell, reg, 30},
		{&fs.scale, reg, 24},
	} {
		f, err := face(step.f, step.size)
		if err != nil {
			return nil, fmt.Errorf("build face: %w", err)
		}
		*step.dst = f
	}
	return &fs, nil
}
