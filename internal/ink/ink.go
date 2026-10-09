// Package ink turns a rendered page into the only two measurements snif's
// non-text widgets need: how much of a rect is inked, and where its rightmost ink
// is.
//
// The image it reads is snorg's canonical page render (RenderRegion with
// styled=false): every stroke forced black on white, with the template background
// and the nav/link overlays dropped. That is what makes the measurements sound —
// the dotted or ruled decoration snif draws into a template's background image is
// simply not in this image, so decoration can never be counted as a tick, and the
// pen shades a user picked cannot change a reading.
//
// The threshold is snorg's own (a pixel is ink iff it is darker than mid-grey), so
// snif's idea of "this box holds ink" and snorg's idea of "this box is blank, skip
// the LLM call" can never disagree.
package ink

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	_ "image/png" // the format snorg renders to

	"github.com/jdlugosz963/snif/internal/spec"
)

// threshold is the ink cutoff on an 8-bit grey scale, matching snorg's mask
// (internal/analyze/geom.go): darker than mid-grey is ink. It keeps anti-aliased
// stroke edges stable.
const threshold = 128

// Mask is a 1-bit ink map of one rendered page, packed one bit per pixel.
type Mask struct {
	w, h int
	bits []byte
}

// Decode reads a PNG (snorg's page render) into a Mask.
func Decode(data []byte) (*Mask, error) {
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("decode page render: %w", err)
	}
	return FromImage(img), nil
}

// FromImage thresholds any image into a Mask. The image's own bounds origin is
// dropped: mask coordinates are page coordinates, starting at (0,0).
func FromImage(img image.Image) *Mask {
	b := img.Bounds()
	m := New(b.Dx(), b.Dy())
	for y := 0; y < m.h; y++ {
		for x := 0; x < m.w; x++ {
			if color.GrayModel.Convert(img.At(b.Min.X+x, b.Min.Y+y)).(color.Gray).Y < threshold {
				m.Set(x, y)
			}
		}
	}
	return m
}

// New returns an empty mask of the given size.
func New(w, h int) *Mask {
	if w < 0 {
		w = 0
	}
	if h < 0 {
		h = 0
	}
	return &Mask{w: w, h: h, bits: make([]byte, (w*h+7)/8)}
}

// Size returns the mask's pixel dimensions.
func (m *Mask) Size() (w, h int) { return m.w, m.h }

// Set marks a pixel as ink (out-of-bounds is ignored) — used by tests and by the
// synthetic pages the round-trip check draws.
func (m *Mask) Set(x, y int) {
	if x < 0 || y < 0 || x >= m.w || y >= m.h {
		return
	}
	i := y*m.w + x
	m.bits[i/8] |= 1 << uint(i%8)
}

// At reports whether a pixel is ink; out-of-bounds is not ink.
func (m *Mask) At(x, y int) bool {
	if x < 0 || y < 0 || x >= m.w || y >= m.h {
		return false
	}
	i := y*m.w + x
	return m.bits[i/8]&(1<<uint(i%8)) != 0
}

// clip intersects r with the page, so a rect running off the edge is measured on
// the pixels that exist rather than rejected.
func (m *Mask) clip(r spec.Rect) spec.Rect {
	x0, y0 := max(r.X, 0), max(r.Y, 0)
	x1, y1 := min(r.X+r.W, m.w), min(r.Y+r.H, m.h)
	return spec.Rect{X: x0, Y: y0, W: x1 - x0, H: y1 - y0}
}

// Count returns the number of inked pixels in r.
func (m *Mask) Count(r spec.Rect) int {
	c := m.clip(r)
	n := 0
	for y := c.Y; y < c.Y+c.H; y++ {
		for x := c.X; x < c.X+c.W; x++ {
			if m.At(x, y) {
				n++
			}
		}
	}
	return n
}

// Fill is the inked fraction of r, in [0,1] — the measurement a tick box is read
// by. An empty rect is 0, never a division by zero.
func (m *Mask) Fill(r spec.Rect) float64 {
	c := m.clip(r)
	if c.Area() == 0 {
		return 0
	}
	return float64(m.Count(c)) / float64(c.Area())
}

// Rightmost returns the page-space x of the rightmost inked column inside r, and
// whether r holds any ink at all. It is the whole of a slider's reading: the
// device is digital, so the last inked column is the mark the user drew, with no
// noise to reject.
func (m *Mask) Rightmost(r spec.Rect) (x int, ok bool) {
	c := m.clip(r)
	for x := c.X + c.W - 1; x >= c.X; x-- {
		for y := c.Y; y < c.Y+c.H; y++ {
			if m.At(x, y) {
				return x, true
			}
		}
	}
	return 0, false
}

// Blank reports whether r holds no ink.
func (m *Mask) Blank(r spec.Rect) bool { return m.Count(r) == 0 }
