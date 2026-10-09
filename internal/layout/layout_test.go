package layout

import (
	"errors"
	"strings"
	"testing"

	"github.com/jdlugosz963/snif/internal/spec"
)

func build(t *testing.T, reqs ...Request) *Page {
	t.Helper()
	p, err := Build(Template{Name: "t", Requests: reqs})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	return p
}

// TestStacking: widgets are laid out in request order, down the page, inside the
// margins.
func TestStacking(t *testing.T) {
	p := build(t,
		Request{Kind: spec.Check, Name: "a", Label: "A"},
		Request{Kind: spec.Input, Name: "b", Label: "B", H: 300},
		Request{Kind: spec.Radio, Name: "c", Options: []string{"x", "y"}},
	)
	if len(p.Widgets) != 3 {
		t.Fatalf("got %d widgets", len(p.Widgets))
	}
	prev := 0
	for _, w := range p.Widgets {
		if w.Rect.X < Margin || w.Rect.X+w.Rect.W > spec.PageW-Margin {
			t.Errorf("%s leaves the margins: %+v", w.Name, w.Rect)
		}
		if w.Rect.Y <= prev {
			t.Errorf("%s is not below the previous widget (%d <= %d)", w.Name, w.Rect.Y, prev)
		}
		prev = w.Rect.Y
	}
}

// TestCellsHaveGutters: neighbouring cells never touch, so a tick that strays a
// little stays in its own box — and the gap is the reason two cells can never
// both claim one stroke.
func TestCellsHaveGutters(t *testing.T) {
	p := build(t, Request{Kind: spec.Date, Name: "d", Year0: 2026, Year1: 2030})
	w := p.Widgets[0]
	for _, part := range []spec.Part{spec.Day, spec.Month, spec.Year} {
		cells := w.Parts(part)
		if len(cells) == 0 {
			t.Fatalf("no %s cells", part)
		}
		for i := 1; i < len(cells); i++ {
			a, b := cells[i-1], cells[i]
			if a.Rect.Y != b.Rect.Y {
				continue // a new row
			}
			if gap := b.Rect.X - (a.Rect.X + a.Rect.W); gap < Gutter {
				t.Errorf("%s cells %s and %s are %d px apart, want %d", part, a.Key, b.Key, gap, Gutter)
			}
		}
	}
	if got := len(w.Cells); got != 31+12+5 {
		t.Errorf("date has %d cells, want %d", got, 31+12+5)
	}
}

// TestSizesAreHonoured: the w/h attributes size a widget's body, which is how an
// input field's dimensions are set.
func TestSizesAreHonoured(t *testing.T) {
	p := build(t, Request{Kind: spec.Input, Name: "notes", W: 900, H: 640})
	if r := p.Widgets[0].Rect; r.W != 900 || r.H != 640 {
		t.Errorf("input rect = %+v, want 900x640", r)
	}
}

// TestOverflowNamesTheWidget: a page that runs out of room says which widget did
// not fit.
func TestOverflowNamesTheWidget(t *testing.T) {
	_, err := Build(Template{Name: "t", Requests: []Request{
		{Kind: spec.Input, Name: "a", H: 2000},
		{Kind: spec.Input, Name: "toolong", H: 2000},
	}})
	if err == nil || !strings.Contains(err.Error(), "toolong") {
		t.Fatalf("err = %v, want it to name toolong", err)
	}
}

// TestTitleShiftsTheFirstWidget: a titled page starts lower down.
func TestTitleShiftsTheFirstWidget(t *testing.T) {
	plain := build(t, Request{Kind: spec.Check, Name: "a"})
	titled, err := Build(Template{Name: "t", Title: "Hello", Requests: []Request{{Kind: spec.Check, Name: "a"}}})
	if err != nil {
		t.Fatal(err)
	}
	if titled.Widgets[0].Rect.Y <= plain.Widgets[0].Rect.Y {
		t.Error("a title must push the first widget down")
	}
	if titled.TitleAt.Empty() {
		t.Error("a titled page must place its title")
	}
}

// TestBadRequests: the layout refuses what it cannot place.
func TestBadRequests(t *testing.T) {
	cases := map[string]Request{
		"radio with no options": {Kind: spec.Radio, Name: "a"},
		"slider with no range":  {Kind: spec.Slider, Name: "a"},
		"date backwards":        {Kind: spec.Date, Name: "a", Year0: 2030, Year1: 2026},
		"date too wide":         {Kind: spec.Date, Name: "a", Year0: 2000, Year1: 2030},
		"unknown kind":          {Kind: "dial", Name: "a"},
		"bad name":              {Kind: spec.Check, Name: "a:b"},
	}
	for name, r := range cases {
		if _, err := Build(Template{Name: "t", Requests: []Request{r}}); err == nil {
			t.Errorf("%s: accepted, want an error", name)
		}
	}
}

// TestFigureIsARegionNotAWidget: a --figure request lands in Regions, stacked in
// request order like everything else, and its box is never analyzed — a drawing
// has no text for a vision model to read, and a box marked otherwise would spend
// a call on every page drawn on the template.
func TestFigureIsARegionNotAWidget(t *testing.T) {
	p := build(t,
		Request{Kind: spec.Input, Name: "name", Label: "Name", H: 140},
		Request{Kind: FigureKind, Name: "main", Label: "Figure", H: 900},
		Request{Kind: spec.Input, Name: "notes", Label: "Notes", H: 400},
	)
	if len(p.Widgets) != 2 || len(p.Regions) != 1 {
		t.Fatalf("got %d widgets and %d regions, want 2 and 1", len(p.Widgets), len(p.Regions))
	}
	r := p.Regions[0]
	if r.Rect.H != 900 || r.Rect.X != Margin || r.Rect.W != Content {
		t.Errorf("region rect %+v", r.Rect)
	}
	box := r.Box()
	if box.ID != "figure:main" || box.Analyze {
		t.Errorf("box = %+v, want id figure:main and analyze false", box)
	}
	// The grammar must not recognize it: a figure is an ordinary snorg box, which
	// is what keeps it out of the form model and out of `snif check`.
	if _, err := spec.Parse(box.ID); !errors.Is(err, spec.ErrNotWidget) {
		t.Errorf("spec.Parse(%q) = %v, want ErrNotWidget", box.ID, err)
	}
	// Items recovers the page order the two slices lost.
	var ids []string
	for _, it := range p.Items() {
		ids = append(ids, string(it.Kind)+":"+it.Name)
	}
	want := []string{"input:name", "figure:main", "input:notes"}
	if strings.Join(ids, " ") != strings.Join(want, " ") {
		t.Errorf("Items order = %v, want %v", ids, want)
	}
}
