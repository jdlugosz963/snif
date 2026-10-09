package spec

import (
	"errors"
	"fmt"
	"sort"
)

// Rect is an axis-aligned rectangle in Supernote page pixel space (PageW x PageH,
// origin top-left, y down) — the same {x,y,w,h} shape and units as a snorg
// template box, so a rect crosses the boundary between the two projects unchanged.
type Rect struct {
	X int `json:"x" yaml:"x"`
	Y int `json:"y" yaml:"y"`
	W int `json:"w" yaml:"w"`
	H int `json:"h" yaml:"h"`
}

// The page pixel space every rect lives in (snorg's snote.PageWidth/PageHeight).
const (
	PageW = 1920
	PageH = 2560
)

// Empty reports whether the rect encloses no pixels.
func (r Rect) Empty() bool { return r.W <= 0 || r.H <= 0 }

// Area is the rect's pixel count.
func (r Rect) Area() int {
	if r.Empty() {
		return 0
	}
	return r.W * r.H
}

// Overlaps reports whether two rects share a pixel — the check that keeps one
// widget's ink out of another's.
func (r Rect) Overlaps(o Rect) bool {
	return !r.Empty() && !o.Empty() &&
		r.X < o.X+o.W && o.X < r.X+r.W && r.Y < o.Y+o.H && o.Y < r.Y+r.H
}

// Contains reports whether o lies wholly inside r.
func (r Rect) Contains(o Rect) bool {
	return o.X >= r.X && o.Y >= r.Y && o.X+o.W <= r.X+r.W && o.Y+o.H <= r.Y+r.H
}

// Box is one snorg template box as snif sees it: the id carries the widget (see
// Parse), the label is the human caption or the glyph drawn in a cell, the rect is
// the geometry and Analyze says whether snorg's vision model transcribes it (true
// for an Input box, false for every other).
type Box struct {
	ID      string
	Label   string
	Rect    Rect
	Analyze bool
}

// Cell is one option of a Radio or Date widget: its key (the value the widget
// takes when this cell is ticked), the glyph drawn in it and its rect.
type Cell struct {
	Key   string
	Label string
	Rect  Rect
	Part  Part // the sub-group, on a Date widget; "" on a Radio
	Num   int  // the key as a number, on a Date widget
}

// Widget is one interface element assembled from a page's boxes: a single box
// (Check/Button/Input/Slider) or a group box plus its cells (Radio/Date). It is
// sparse — Min/Max/Step are a Slider's, Year0/Year1 a Date's, Cells a group's.
type Widget struct {
	Kind  Kind
	Name  string
	Label string
	Rect  Rect // the whole widget: the single box, or the group band

	Cells []Cell

	Min, Max, Step float64
	Year0, Year1   int
}

// Grouped reports whether the kind is built from a group box plus cells.
func (k Kind) Grouped() bool { return k == Radio || k == Date }

// Analyzed reports whether snorg's vision model transcribes this kind's box —
// true for Input alone, which is what makes every other widget free to read.
func (k Kind) Analyzed() bool { return k == Input }

// Parts returns the widget's date cells belonging to sub-group p, in layout order.
func (w Widget) Parts(p Part) []Cell {
	var out []Cell
	for _, c := range w.Cells {
		if c.Part == p {
			out = append(out, c)
		}
	}
	return out
}

// Boxes renders the widget back into the boxes that encode it — the exact inverse
// of Build, and the generator's only way of emitting ids. A round-trip through
// Boxes and Build is the identity, which is what guarantees the generator and the
// reader share one grammar.
func (w Widget) Boxes() []Box {
	id := ID{Kind: w.Kind, Name: w.Name, Min: w.Min, Max: w.Max, Step: w.Step, Year0: w.Year0, Year1: w.Year1}
	boxes := []Box{{ID: id.String(), Label: w.Label, Rect: w.Rect, Analyze: w.Kind.Analyzed()}}
	for _, c := range w.Cells {
		cid := ID{Kind: w.Kind, Name: w.Name, Cell: c.Key, Part: c.Part}
		boxes = append(boxes, Box{ID: cid.String(), Label: c.Label, Rect: c.Rect})
	}
	return boxes
}

// Build assembles boxes into widgets, in the order their widget-level box first
// appears. Boxes whose id is not snif's are skipped silently — a snif template may
// carry ordinary snorg boxes alongside its widgets — while an id that names a known
// kind and is then malformed is reported, because that is an authoring mistake.
//
// It is the reader's entry point: a page's regions in, a form's widgets out.
func Build(boxes []Box) ([]Widget, error) {
	type entry struct {
		w     *Widget
		order int
		cells []Cell
	}
	byName := map[string]*entry{}
	var order []*entry
	seen := map[string]bool{}

	for i, b := range boxes {
		id, err := Parse(b.ID)
		if err != nil {
			if errors.Is(err, ErrNotWidget) {
				continue
			}
			return nil, err
		}
		if seen[b.ID] {
			return nil, fmt.Errorf("duplicate box id %q", b.ID)
		}
		seen[b.ID] = true

		key := string(id.Kind) + ":" + id.Name
		e := byName[key]
		if e == nil {
			e = &entry{order: i}
			byName[key] = e
			order = append(order, e)
		}
		if id.IsCell() {
			c := Cell{Key: id.Cell, Label: b.Label, Rect: b.Rect, Part: id.Part}
			if id.Part != "" {
				n, err := id.CellNumber()
				if err != nil {
					return nil, err
				}
				c.Num = n
			}
			e.cells = append(e.cells, c)
			continue
		}
		if e.w != nil {
			return nil, fmt.Errorf("box %q: widget %s:%s declared twice", b.ID, id.Kind, id.Name)
		}
		e.w = &Widget{
			Kind: id.Kind, Name: id.Name, Label: b.Label, Rect: b.Rect,
			Min: id.Min, Max: id.Max, Step: id.Step, Year0: id.Year0, Year1: id.Year1,
		}
	}

	sort.SliceStable(order, func(i, j int) bool { return order[i].order < order[j].order })

	out := make([]Widget, 0, len(order))
	names := map[string]Kind{}
	for _, e := range order {
		if e.w == nil {
			c := e.cells[0]
			return nil, fmt.Errorf("cell %q has no widget box: add the group box it belongs to", c.Key)
		}
		w := *e.w
		if w.Kind.Grouped() {
			if len(e.cells) == 0 {
				return nil, fmt.Errorf("widget %s:%s has no cells", w.Kind, w.Name)
			}
			w.Cells = sortCells(e.cells)
		} else if len(e.cells) > 0 {
			return nil, fmt.Errorf("widget %s:%s takes no cells", w.Kind, w.Name)
		}
		if k, dup := names[w.Name]; dup {
			return nil, fmt.Errorf("widget name %q used by both %s and %s", w.Name, k, w.Kind)
		}
		names[w.Name] = w.Kind
		if err := validateCells(w); err != nil {
			return nil, err
		}
		out = append(out, w)
	}
	return out, nil
}

// sortCells puts a group's cells in reading order: date cells by sub-group
// (day, month, year) then by number; radio options keep their authored order,
// which is their layout order.
func sortCells(cells []Cell) []Cell {
	rank := map[Part]int{Day: 0, Month: 1, Year: 2}
	if cells[0].Part == "" {
		return cells
	}
	sort.SliceStable(cells, func(i, j int) bool {
		if cells[i].Part != cells[j].Part {
			return rank[cells[i].Part] < rank[cells[j].Part]
		}
		return cells[i].Num < cells[j].Num
	})
	return cells
}

// validateCells checks the invariants a group's cells must hold: unique keys, and
// for a date every sub-group present with its years inside the declared range.
func validateCells(w Widget) error {
	if !w.Kind.Grouped() {
		return nil
	}
	seen := map[string]bool{}
	for _, c := range w.Cells {
		k := string(c.Part) + ":" + c.Key
		if seen[k] {
			return fmt.Errorf("widget %s:%s: duplicate cell %q", w.Kind, w.Name, c.Key)
		}
		seen[k] = true
	}
	if w.Kind != Date {
		return nil
	}
	for _, p := range parts {
		if len(w.Parts(p)) == 0 {
			return fmt.Errorf("widget date:%s has no %s cells", w.Name, p)
		}
	}
	for _, c := range w.Parts(Year) {
		if c.Num < w.Year0 || c.Num > w.Year1 {
			return fmt.Errorf("widget date:%s: year cell %d is outside the declared range %d..%d", w.Name, c.Num, w.Year0, w.Year1)
		}
	}
	return nil
}
