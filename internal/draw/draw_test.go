package draw

import (
	"image"
	"testing"

	"github.com/jdlugosz963/snif/internal/layout"
	"github.com/jdlugosz963/snif/internal/spec"
)

func page(t *testing.T, theme layout.Theme, reqs ...layout.Request) (*layout.Page, *image.Gray) {
	t.Helper()
	p, err := layout.Build(layout.Template{Name: "t", Title: "T", Theme: theme, Requests: reqs})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	img, err := Image(p)
	if err != nil {
		t.Fatalf("Image: %v", err)
	}
	return p, img
}

// TestThemesRender: every theme draws, at the page size, in greyscale.
func TestThemesRender(t *testing.T) {
	for _, theme := range layout.Themes {
		_, img := page(t, theme, layout.Request{Kind: spec.Input, Name: "notes", Label: "Notes", H: 400})
		if img.Bounds().Dx() != spec.PageW || img.Bounds().Dy() != spec.PageH {
			t.Errorf("%s: size %v", theme, img.Bounds())
		}
	}
}

// TestMeasuredAreasAreCleared: a widget with no style of its own holds none of the
// page's pattern, so a dense page theme never makes its tick boxes noisy — while
// a widget that asks for a style gets it.
func TestMeasuredAreasAreCleared(t *testing.T) {
	p, img := page(t, Dots,
		layout.Request{Kind: spec.Check, Name: "plain", Label: "Plain"},
		layout.Request{Kind: spec.Input, Name: "ruled", Label: "Ruled", Style: layout.Lines, H: 400},
	)
	plain, ruled := p.Widgets[0], p.Widgets[1]

	if n := nonWhite(img, inset(plain.Rect, 8)); n != 0 {
		t.Errorf("an unstyled tick box holds %d pattern pixels, want none", n)
	}
	if n := nonWhite(img, inset(ruled.Rect, 20)); n == 0 {
		t.Error("a styled input area must carry its own pattern")
	}
	// The page itself does carry the theme, outside the widgets.
	if n := nonWhite(img, spec.Rect{X: 0, Y: spec.PageH - 200, W: spec.PageW, H: 200}); n == 0 {
		t.Error("the page theme must be drawn outside the widgets")
	}
}

// Dots is spelled out here so the test reads as a page-level theme rather than a
// bare constant reference.
const Dots = layout.Dots

func nonWhite(img *image.Gray, r spec.Rect) int {
	n := 0
	for y := r.Y; y < r.Y+r.H; y++ {
		for x := r.X; x < r.X+r.W; x++ {
			if img.GrayAt(x, y).Y != white.Y {
				n++
			}
		}
	}
	return n
}
