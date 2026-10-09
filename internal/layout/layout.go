// Package layout places widgets on a Supernote page.
//
// It is the half of generation that decides geometry: a Template (what the author
// asked for, in order) goes in, a Page (every widget with a pixel rect, plus the
// decoration a renderer needs) comes out. Nothing here draws — that is
// internal/draw — and nothing here knows about YAML, so a placement can be tested
// by asserting rects.
//
// The rule that shapes every decision: a widget's Rect is the area snif measures
// ink in, never the area it occupies on the page. A check box's rect is the tick
// box alone, with its caption beside it; a radio group's rect is the band its
// cells live in, and only the cells are ever measured. Captions, frames and
// sub-labels are drawn outside any measured rect, so printed decoration can never
// be mistaken for an answer — and cells are laid out with a gutter between them,
// so a tick that strays a little stays in its own box.
package layout

import (
	"fmt"
	"sort"
	"strings"

	"github.com/jdlugosz963/snif/internal/spec"
)

// Theme is a fill pattern, used both page-wide and inside a single widget.
type Theme string

const (
	Blank Theme = "blank" // nothing; the default
	Dots  Theme = "dots"
	Grid  Theme = "grid"
	Lines Theme = "lines"
)

// Themes lists the accepted values, for flag validation and help text.
var Themes = []Theme{Blank, Dots, Grid, Lines}

// ParseTheme validates a theme name.
func ParseTheme(s string) (Theme, error) {
	if s == "" {
		return Blank, nil
	}
	for _, t := range Themes {
		if Theme(s) == t {
			return t, nil
		}
	}
	var names []string
	for _, t := range Themes {
		names = append(names, string(t))
	}
	return "", fmt.Errorf("unknown theme %q (want %s)", s, strings.Join(names, ", "))
}

// The page geometry every template is laid out in.
const (
	Margin  = 80 // page margin
	Gap     = 48 // vertical space between widgets
	Gutter  = 12 // space between neighbouring cells, so a stray tick stays home
	CellH   = 90 // a radio/date cell's height
	TickBox = 84 // a check/button's tick box
)

// Content is the usable width between the margins.
const Content = spec.PageW - 2*Margin

// Request is one widget as the author asked for it, before placement.
type Request struct {
	Kind    spec.Kind
	Name    string
	Label   string
	Style   Theme    // fill inside the widget's body; "" = none
	W, H    int      // body size in pixels; 0 = the kind's default
	Options []string // Radio option keys, in layout order
	Labels  []string // Radio option labels, parallel to Options ("" = the key)

	Year0, Year1   int     // Date
	Min, Max, Step float64 // Slider
}

// FigureKind marks a Request that is not a widget at all but a plain region: an
// area snif places and draws a frame for, and never reads. It is deliberately
// absent from spec's kind table, so `figure:<name>` parses as ErrNotWidget — an
// ordinary snorg box, invisible to the form model and to `snif check`, whose rect
// is still exposed for a consumer to crop a drawing out of
// (snorg/docs/templates.md).
const FigureKind = spec.Kind("figure")

// Template is a whole page as the author asked for it.
type Template struct {
	Name     string
	Title    string
	Theme    Theme
	Requests []Request
}

// Widget is a placed widget: the spec (which is what the YAML and the reader see)
// plus the extra geometry only the renderer needs.
type Widget struct {
	spec.Widget

	Style   Theme     // fill inside Body
	Caption spec.Rect // where the label is drawn; empty when the widget has none
	Body    spec.Rect // the drawn area (the frame): the measured rect, or the cells' band
	Ticks   []Tick    // slider scale marks
}

// Tick is one labelled mark under a slider track.
type Tick struct {
	X     int
	Label string
}

// Region is a placed figure: a plain snorg box with the caption and frame the
// renderer draws around it. It carries no value — which is exactly why it is not
// a Widget — so nothing reads it and nothing transcribes it.
type Region struct {
	Name    string
	Label   string
	Style   Theme     // fill inside Body; "" = none
	Rect    spec.Rect // the box written into the config
	Caption spec.Rect // where the label is drawn; empty when unlabelled
	Body    spec.Rect // the drawn frame; the same rect, named for symmetry with Widget
}

// Box is the region as the config sees it. Never analyzed: there is no text in a
// drawing for a vision model to transcribe, and a box marked otherwise would
// spend a call on every page drawn on the template.
func (r Region) Box() spec.Box {
	return spec.Box{ID: string(FigureKind) + ":" + r.Name, Label: r.Label, Rect: r.Rect}
}

// Top is the y a band starts at: its caption when it has one, its body otherwise.
func (r Region) Top() int { return bandTop(r.Caption, r.Rect) }

// Top is the y a band starts at: its caption when it has one, its body otherwise.
func (w Widget) Top() int { return bandTop(w.Caption, w.Rect) }

func bandTop(caption, body spec.Rect) int {
	if !caption.Empty() {
		return caption.Y
	}
	return body.Y
}

// Page is a laid-out template.
type Page struct {
	Name    string
	Theme   Theme
	Title   string
	TitleAt spec.Rect // empty when there is no title
	Widgets []Widget
	Regions []Region
}

// Specs returns the placed widgets as the spec model — the input to the YAML and
// the thing the reader later rebuilds from a page's boxes.
func (p Page) Specs() []spec.Widget {
	out := make([]spec.Widget, 0, len(p.Widgets))
	for _, w := range p.Widgets {
		out = append(out, w.Widget)
	}
	return out
}

// Item is one laid-out thing as the config records it: the boxes it contributes,
// plus the kind and name that name it. Widgets and regions are kept in separate
// slices because only one of the two is a form field; Items puts them back into
// the order they were drawn in, which is the order a reader of the YAML expects.
type Item struct {
	Kind  spec.Kind
	Name  string
	Boxes []spec.Box
}

// Items returns the page's widgets and regions together, top to bottom.
func (p Page) Items() []Item {
	out := make([]Item, 0, len(p.Widgets)+len(p.Regions))
	type placed struct {
		top  int
		item Item
	}
	var all []placed
	for _, w := range p.Widgets {
		all = append(all, placed{w.Top(), Item{Kind: w.Kind, Name: w.Name, Boxes: w.Boxes()}})
	}
	for _, r := range p.Regions {
		all = append(all, placed{r.Top(), Item{Kind: FigureKind, Name: r.Name, Boxes: []spec.Box{r.Box()}}})
	}
	sort.SliceStable(all, func(i, j int) bool { return all[i].top < all[j].top })
	for _, p := range all {
		out = append(out, p.item)
	}
	return out
}

// Build places a template's widgets down the page, in request order.
//
// Each widget occupies a band: an optional caption line and a body. Bands stack
// from the top margin down with Gap between them, and a widget that does not fit
// what is left is an error naming it — a template that silently ran off the page
// would read as a form with missing fields.
func Build(t Template) (*Page, error) {
	p := &Page{Name: t.Name, Theme: t.Theme, Title: t.Title}
	y := Margin
	if t.Title != "" {
		p.TitleAt = spec.Rect{X: Margin, Y: y, W: Content, H: titleH}
		y += titleH + Gap
	}
	names := map[string]bool{}
	for _, r := range t.Requests {
		if !identOK(r.Name) {
			return nil, fmt.Errorf("widget name %q: use letters, digits, '-', '_' or '.', starting alphanumeric", r.Name)
		}
		if names[r.Name] {
			return nil, fmt.Errorf("duplicate widget name %q", r.Name)
		}
		names[r.Name] = true

		fits := func(h int) error {
			if y+h > spec.PageH-Margin {
				return fmt.Errorf("%s %q does not fit: the page has %d px left and it needs %d", r.Kind, r.Name, spec.PageH-Margin-y, h)
			}
			return nil
		}
		if r.Kind == FigureKind {
			reg, h := placeRegion(r, y)
			if err := fits(h); err != nil {
				return nil, err
			}
			p.Regions = append(p.Regions, reg)
			y += h + Gap
			continue
		}
		w, h, err := place(r, y)
		if err != nil {
			return nil, err
		}
		if err := fits(h); err != nil {
			return nil, err
		}
		p.Widgets = append(p.Widgets, w)
		y += h + Gap
	}
	if err := validate(p); err != nil {
		return nil, err
	}
	return p, nil
}

// Height sizes: the caption line above a body, and the title band.
const (
	captionH = 56
	titleH   = 96
)

// place lays one widget out at y and returns it with the height of its whole band.
func place(r Request, y int) (Widget, int, error) {
	w := Widget{Style: r.Style}
	w.Kind, w.Name, w.Label = r.Kind, r.Name, r.Label
	bodyW := r.W
	if bodyW <= 0 || bodyW > Content {
		bodyW = Content
	}

	switch r.Kind {
	case spec.Check, spec.Button:
		// The tick box sits left, the caption beside it on the same line, so a
		// yes/no question costs one line rather than two.
		side := r.H
		if side <= 0 {
			side = TickBox
		}
		w.Rect = spec.Rect{X: Margin, Y: y, W: side, H: side}
		w.Body = w.Rect
		w.Caption = spec.Rect{X: Margin + side + 24, Y: y, W: Content - side - 24, H: side}
		return w, side, nil

	case spec.Input:
		h := r.H
		if h <= 0 {
			h = 480
		}
		w.Caption, w.Rect = captionAt(r.Label, y, bodyW), spec.Rect{X: Margin, Y: capBelow(r.Label, y), W: bodyW, H: h}
		w.Body = w.Rect
		return w, bandH(r.Label, h), nil

	case spec.Slider:
		if r.Max <= r.Min {
			return w, 0, fmt.Errorf("slider %q: max must exceed min", r.Name)
		}
		h := r.H
		if h <= 0 {
			h = 132
		}
		w.Min, w.Max, w.Step = r.Min, r.Max, r.Step
		w.Caption = captionAt(r.Label, y, bodyW)
		w.Rect = spec.Rect{X: Margin, Y: capBelow(r.Label, y), W: bodyW, H: h}
		w.Body = w.Rect
		w.Ticks = ticksOf(w.Rect, r.Min, r.Max)
		return w, bandH(r.Label, h+scaleH), nil

	case spec.Radio:
		if len(r.Options) == 0 {
			return w, 0, fmt.Errorf("radio %q: needs at least one option", r.Name)
		}
		top := capBelow(r.Label, y)
		cells, h := row(r.Options, r.Labels, Margin, top, bodyW, len(r.Options))
		w.Caption = captionAt(r.Label, y, bodyW)
		w.Cells = cells
		w.Rect = spec.Rect{X: Margin, Y: top, W: bodyW, H: h}
		w.Body = w.Rect
		return w, bandH(r.Label, h), nil

	case spec.Date:
		return placeDate(r, y, bodyW)
	}
	return w, 0, fmt.Errorf("unknown widget kind %q", r.Kind)
}

// placeRegion lays a figure out at y: a caption line and a framed area to draw
// in. The default body is deliberately large — a figure area that had to be asked
// for in pixels every time would be a chore.
func placeRegion(r Request, y int) (Region, int) {
	bodyW := r.W
	if bodyW <= 0 || bodyW > Content {
		bodyW = Content
	}
	h := r.H
	if h <= 0 {
		h = 1200
	}
	reg := Region{Name: r.Name, Label: r.Label, Style: r.Style}
	reg.Caption = captionAt(r.Label, y, bodyW)
	reg.Rect = spec.Rect{X: Margin, Y: capBelow(r.Label, y), W: bodyW, H: h}
	reg.Body = reg.Rect
	return reg, bandH(r.Label, h)
}

// scaleH is the strip under a slider track that carries its scale labels.
const scaleH = 44

// captionAt is the caption rect above a body, or the empty rect when unlabelled.
func captionAt(label string, y, w int) spec.Rect {
	if label == "" {
		return spec.Rect{}
	}
	return spec.Rect{X: Margin, Y: y, W: w, H: captionH}
}

// capBelow is the y a body starts at, after the caption if there is one.
func capBelow(label string, y int) int {
	if label == "" {
		return y
	}
	return y + captionH
}

// bandH is a widget's total height: its body plus the caption above it.
func bandH(label string, body int) int {
	if label == "" {
		return body
	}
	return body + captionH
}

// row lays n cells evenly across width w, returning them and the row's height.
// Cell widths are integral and the remainder is spread over the leftmost cells, so
// the row ends exactly on the right margin with no cell a pixel narrower than its
// neighbours by more than one.
func row(keys, labels []string, x, y, w, n int) ([]spec.Cell, int) {
	cw := (w - (n-1)*Gutter) / n
	extra := (w - (n-1)*Gutter) - cw*n
	cells := make([]spec.Cell, 0, len(keys))
	cx := x
	for i, k := range keys {
		width := cw
		if i < extra {
			width++
		}
		label := k
		if i < len(labels) && labels[i] != "" {
			label = labels[i]
		}
		cells = append(cells, spec.Cell{Key: k, Label: label, Rect: spec.Rect{X: cx, Y: y, W: width, H: CellH}})
		cx += width + Gutter
	}
	return cells, CellH
}

// ticksOf spreads five labelled marks along a slider track, so the scale is
// readable without printing every value.
func ticksOf(track spec.Rect, min, max float64) []Tick {
	var out []Tick
	for i := 0; i <= 4; i++ {
		f := float64(i) / 4
		out = append(out, Tick{
			X:     track.X + int(f*float64(track.W-1)),
			Label: trim(min + f*(max-min)),
		})
	}
	return out
}

// trim formats a scale label without trailing zeros.
func trim(v float64) string {
	s := fmt.Sprintf("%.2f", v)
	s = strings.TrimRight(s, "0")
	return strings.TrimSuffix(s, ".")
}

// monthLabels are the glyphs drawn in a date's month cells; the cell keys stay
// numeric ("01".."12"), so the value is a number and only the label is language.
var monthLabels = [12]string{"Jan", "Feb", "Mar", "Apr", "May", "Jun", "Jul", "Aug", "Sep", "Oct", "Nov", "Dec"}

// daysPerRow splits 1..31 into two rows of sixteen, the widest split that keeps a
// day cell comfortably wider than the two digits printed in it.
const daysPerRow = 16

func placeDate(r Request, y, bodyW int) (Widget, int, error) {
	if r.Year1 < r.Year0 || r.Year0 < 1000 || r.Year1 > 9999 {
		return Widget{}, 0, fmt.Errorf("date %q: bad year range %d..%d", r.Name, r.Year0, r.Year1)
	}
	if n := r.Year1 - r.Year0 + 1; n > 12 {
		return Widget{}, 0, fmt.Errorf("date %q: %d years is more than a row fits; narrow the range", r.Name, n)
	}
	w := Widget{Style: r.Style}
	w.Kind, w.Name, w.Label, w.Year0, w.Year1 = r.Kind, r.Name, r.Label, r.Year0, r.Year1

	top := capBelow(r.Label, y)
	cy := top
	// Days: two rows of sixteen (1..16, 17..31).
	for start := 1; start <= 31; start += daysPerRow {
		var keys, labels []string
		for d := start; d < start+daysPerRow && d <= 31; d++ {
			keys = append(keys, spec.CellKey(spec.Day, d))
			labels = append(labels, fmt.Sprintf("%d", d))
		}
		cells, h := row(keys, labels, Margin, cy, bodyW, daysPerRow)
		for i := range cells {
			cells[i].Part, cells[i].Num = spec.Day, dayNum(cells[i].Key)
		}
		w.Cells = append(w.Cells, cells...)
		cy += h + Gutter
	}
	// Months: one row of twelve, named.
	var mk, ml []string
	for m := 1; m <= 12; m++ {
		mk = append(mk, spec.CellKey(spec.Month, m))
		ml = append(ml, monthLabels[m-1])
	}
	months, h := row(mk, ml, Margin, cy, bodyW, 12)
	for i := range months {
		months[i].Part, months[i].Num = spec.Month, i+1
	}
	w.Cells = append(w.Cells, months...)
	cy += h + Gutter

	// Years: one row, laid out at the day cell's width so a short range does not
	// stretch into absurdly wide boxes.
	var yk, yl []string
	for yr := r.Year0; yr <= r.Year1; yr++ {
		yk = append(yk, spec.CellKey(spec.Year, yr))
		yl = append(yl, fmt.Sprintf("%d", yr))
	}
	years, h := row(yk, yl, Margin, cy, bodyW, max(len(yk), 6))
	for i := range years {
		years[i].Part, years[i].Num = spec.Year, r.Year0+i
	}
	w.Cells = append(w.Cells, years...)
	cy += h

	w.Rect = spec.Rect{X: Margin, Y: top, W: bodyW, H: cy - top}
	w.Body = w.Rect
	w.Caption = captionAt(r.Label, y, bodyW)
	return w, bandH(r.Label, w.Rect.H), nil
}

func dayNum(key string) int {
	n := 0
	for _, c := range key {
		n = n*10 + int(c-'0')
	}
	return n
}

func identOK(s string) bool {
	if _, err := spec.Parse(string(spec.Check) + ":" + s); err != nil {
		return false
	}
	return true
}

// validate checks the invariants a drawn page must hold before it is written. It
// runs the same rules `snif check` applies to a hand-edited config (spec.Validate),
// so a generated template and an authored one are held to one standard.
func validate(p *Page) error {
	boxes := []spec.Box{}
	for _, w := range p.Widgets {
		boxes = append(boxes, w.Boxes()...)
	}
	if _, problems := spec.Validate(boxes); len(problems) > 0 {
		return problems[0]
	}
	return nil
}
