package ink

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"testing"

	"github.com/jdlugosz963/snif/internal/spec"
)

// pageWith renders a white page with the given rects filled black, encoded as PNG
// — a stand-in for snorg's canonical page render.
func pageWith(w, h int, rects ...spec.Rect) []byte {
	img := image.NewGray(image.Rect(0, 0, w, h))
	for i := range img.Pix {
		img.Pix[i] = 0xff
	}
	for _, r := range rects {
		for y := r.Y; y < r.Y+r.H; y++ {
			for x := r.X; x < r.X+r.W; x++ {
				img.SetGray(x, y, color.Gray{})
			}
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		panic(err)
	}
	return buf.Bytes()
}

func TestDecodeFillAndBlank(t *testing.T) {
	cell := spec.Rect{X: 10, Y: 10, W: 20, H: 20}
	m, err := Decode(pageWith(100, 100, spec.Rect{X: 15, Y: 15, W: 10, H: 10}))
	if err != nil {
		t.Fatal(err)
	}
	if got := m.Fill(cell); got != 0.25 {
		t.Errorf("Fill = %v, want 0.25", got)
	}
	if m.Blank(cell) {
		t.Error("an inked cell must not read as blank")
	}
	if !m.Blank(spec.Rect{X: 50, Y: 50, W: 20, H: 20}) {
		t.Error("an untouched cell must read as blank")
	}
}

// TestFillIsPerRect: ink is split at the rect boundary exactly, so a stroke that
// crosses into the next cell contributes only its own pixels to each — the
// property that lets adjacent tick boxes sit 12px apart.
func TestFillIsPerRect(t *testing.T) {
	m, err := Decode(pageWith(100, 100, spec.Rect{X: 18, Y: 10, W: 8, H: 10}))
	if err != nil {
		t.Fatal(err)
	}
	a := m.Count(spec.Rect{X: 0, Y: 0, W: 20, H: 100})
	b := m.Count(spec.Rect{X: 20, Y: 0, W: 20, H: 100})
	if a != 20 || b != 60 {
		t.Errorf("split = %d/%d, want 20/60", a, b)
	}
}

func TestRightmost(t *testing.T) {
	track := spec.Rect{X: 100, Y: 100, W: 200, H: 50}
	m, err := Decode(pageWith(400, 400, spec.Rect{X: 140, Y: 110, W: 4, H: 30}))
	if err != nil {
		t.Fatal(err)
	}
	x, ok := m.Rightmost(track)
	if !ok || x != 143 {
		t.Errorf("Rightmost = %d,%v, want 143,true", x, ok)
	}
	if _, ok := m.Rightmost(spec.Rect{X: 0, Y: 0, W: 50, H: 50}); ok {
		t.Error("an empty track must report no ink")
	}
}

// TestClipping: a rect running past the page edge is measured on the pixels that
// exist rather than panicking or reporting nonsense.
func TestClipping(t *testing.T) {
	m := New(10, 10)
	m.Set(9, 9)
	if got := m.Count(spec.Rect{X: 5, Y: 5, W: 100, H: 100}); got != 1 {
		t.Errorf("Count = %d, want 1", got)
	}
	if got := m.Fill(spec.Rect{X: -50, Y: -50, W: 10, H: 10}); got != 0 {
		t.Errorf("Fill of an off-page rect = %v, want 0", got)
	}
}

// TestGreyThreshold: mid-grey and lighter is not ink, darker is — snorg's rule, so
// the two projects never disagree about a blank box.
func TestGreyThreshold(t *testing.T) {
	img := image.NewGray(image.Rect(0, 0, 3, 1))
	img.Pix[0], img.Pix[1], img.Pix[2] = 127, 128, 255
	m := FromImage(img)
	if !m.At(0, 0) || m.At(1, 0) || m.At(2, 0) {
		t.Errorf("threshold wrong: %v %v %v", m.At(0, 0), m.At(1, 0), m.At(2, 0))
	}
}
