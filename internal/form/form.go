// Package form resolves a page's widgets into typed values.
//
// It is the semantic half of snif: given the widgets a template declares (spec),
// an ink mask of the page (ink) and a lookup for transcribed text, it decides what
// the user answered. It knows nothing about archives, files or snorg — which is
// what lets the round-trip test paint ink into a synthetic mask and assert the
// values come back out.
//
// Resolution is deliberately forgiving in one direction and strict in the other: a
// widget nobody filled in is Set=false with no error (an unanswered question is
// not a broken page), while a widget filled in two mutually exclusive ways is an
// error naming every cell that was ticked — the user asked for exactly one box per
// group, and guessing between them would be worse than saying so.
package form

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/jdlugosz963/snif/internal/ink"
	"github.com/jdlugosz963/snif/internal/spec"
)

// DefaultThreshold is the inked fraction at which a cell counts as ticked. A pen
// stroke through a tick box covers several percent of it and a deliberate dot
// about one, while an untouched box is exactly zero — the device adds no noise —
// so half a percent separates "answered" from "not" with room to spare.
const DefaultThreshold = 0.005

// Options tunes resolution.
type Options struct {
	// Threshold is the inked fraction at which a cell counts as ticked; 0 selects
	// DefaultThreshold.
	Threshold float64
}

func (o Options) threshold() float64 {
	if o.Threshold <= 0 {
		return DefaultThreshold
	}
	return o.Threshold
}

// Cell is one option of a group widget with its measured ink.
type Cell struct {
	Key    string    `json:"key"`
	Label  string    `json:"label,omitempty"`
	Part   spec.Part `json:"part,omitempty"`
	Rect   spec.Rect `json:"rect"`
	Fill   float64   `json:"fill"`
	Filled bool      `json:"filled"`
}

// Widget is one resolved interface element. It is sparse: Bool is a Check or
// Button's, Choice a Radio's, Date a Date's, Number a Slider's, Text an Input's,
// and all of them are meaningless unless Set. Err is this widget's failure alone —
// one ambiguous group never hides the rest of the form.
type Widget struct {
	Kind  spec.Kind `json:"kind"`
	Name  string    `json:"name"`
	Label string    `json:"label,omitempty"`
	Rect  spec.Rect `json:"rect"`
	Cells []Cell    `json:"cells,omitempty"`

	Set    bool      `json:"set"`
	Bool   bool      `json:"bool,omitempty"`
	Choice string    `json:"choice,omitempty"`
	Number float64   `json:"number,omitempty"`
	Text   string    `json:"text,omitempty"`
	Date   time.Time `json:"-"`

	Err error `json:"-"`
}

// Value is the widget's answer in the one Go type its kind produces, or nil when
// it is unset or failed — the form's JSON and Decode both read it.
func (w Widget) Value() any {
	if !w.Set || w.Err != nil {
		return nil
	}
	switch w.Kind {
	case spec.Check, spec.Button:
		return w.Bool
	case spec.Radio:
		return w.Choice
	case spec.Slider:
		return w.Number
	case spec.Input:
		return w.Text
	case spec.Date:
		return w.Date.Format("2006-01-02")
	}
	return nil
}

// Filled lists the cells that hold ink, in cell order.
func (w Widget) Filled() []Cell {
	var out []Cell
	for _, c := range w.Cells {
		if c.Filled {
			out = append(out, c)
		}
	}
	return out
}

// AmbiguousError reports a group in which more than one cell was ticked. It names
// every one with its ink fraction, so the message points at the page rather than
// merely refusing it.
type AmbiguousError struct {
	Widget string
	Part   spec.Part
	Cells  []Cell
}

func (e *AmbiguousError) Error() string {
	var parts []string
	for _, c := range e.Cells {
		parts = append(parts, fmt.Sprintf("%s (%.1f%% ink)", c.Key, c.Fill*100))
	}
	where := e.Widget
	if e.Part != "" {
		where = fmt.Sprintf("%s %s", e.Widget, e.Part)
	}
	return fmt.Sprintf("%s: %d cells are filled, expected one: %s", where, len(e.Cells), strings.Join(parts, ", "))
}

// IncompleteError reports a date with some but not all of its parts picked.
type IncompleteError struct {
	Widget  string
	Missing []spec.Part
}

func (e *IncompleteError) Error() string {
	var m []string
	for _, p := range e.Missing {
		m = append(m, string(p))
	}
	return fmt.Sprintf("%s: incomplete date, no %s picked", e.Widget, strings.Join(m, " or "))
}

// ErrNotAnalyzed marks an Input widget whose box has never been transcribed, so
// its text is unknown rather than empty. Reading it requires an analysis pass.
var ErrNotAnalyzed = errors.New("box has not been analyzed yet")

// Text is the lookup an Input widget is resolved through: the transcription of the
// widget's box, and whether that box has ever been analyzed (an analyzed box that
// held no writing is text "" with ok true — genuinely empty, not unknown).
type Text func(name string) (text string, ok bool)

// Resolve reads every widget against the page mask, in template order.
func Resolve(widgets []spec.Widget, m *ink.Mask, text Text, opt Options) []Widget {
	out := make([]Widget, 0, len(widgets))
	for _, sw := range widgets {
		out = append(out, resolve(sw, m, text, opt))
	}
	return out
}

func resolve(sw spec.Widget, m *ink.Mask, text Text, opt Options) Widget {
	w := Widget{Kind: sw.Kind, Name: sw.Name, Label: sw.Label, Rect: sw.Rect}
	for _, c := range sw.Cells {
		fill := m.Fill(c.Rect)
		w.Cells = append(w.Cells, Cell{
			Key: c.Key, Label: c.Label, Part: c.Part, Rect: c.Rect,
			Fill: fill, Filled: fill >= opt.threshold(),
		})
	}

	switch sw.Kind {
	case spec.Check, spec.Button:
		w.Bool = m.Fill(sw.Rect) >= opt.threshold()
		w.Set = true // a tick box always has an answer: ticked or not

	case spec.Input:
		if text == nil {
			w.Err = ErrNotAnalyzed
			return w
		}
		t, ok := text(sw.Name)
		if !ok {
			w.Err = fmt.Errorf("%s: %w", sw.Name, ErrNotAnalyzed)
			return w
		}
		w.Text, w.Set = t, true

	case spec.Slider:
		x, ok := m.Rightmost(sw.Rect)
		if !ok {
			return w // untouched track: unset, not an error
		}
		w.Number, w.Set = slider(sw, x), true

	case spec.Radio:
		c, err := pick(w.Name, "", w.Cells)
		if err != nil {
			w.Err = err
			return w
		}
		if c == nil {
			return w
		}
		w.Choice, w.Set = c.Key, true

	case spec.Date:
		d, err := date(sw, w.Cells)
		if err != nil {
			w.Err = err
			return w
		}
		if d.IsZero() {
			return w
		}
		w.Date, w.Set = d, true
	}
	return w
}

// slider maps the rightmost inked column to the widget's scale. The mark's column
// is taken as inclusive, so ink at the very left of the track reads as min and ink
// at the very right as max; the result is snapped to Step when one is declared.
func slider(sw spec.Widget, x int) float64 {
	f := 0.0
	if sw.Rect.W > 0 {
		f = float64(x-sw.Rect.X+1) / float64(sw.Rect.W)
	}
	f = math.Min(1, math.Max(0, f))
	v := sw.Min + f*(sw.Max-sw.Min)
	if sw.Step > 0 {
		v = sw.Min + math.Round((v-sw.Min)/sw.Step)*sw.Step
		// Snapping in floating point leaves 0.30000000000000004; round to the
		// decimals the step itself carries so the value prints as authored.
		if d := decimals(sw.Step); d >= 0 {
			p := math.Pow(10, float64(d))
			v = math.Round(v*p) / p
		}
	}
	return math.Min(sw.Max, math.Max(sw.Min, v))
}

// decimals counts the fractional digits a step carries (up to 6), so a snapped
// value can be rounded to the same precision.
func decimals(step float64) int {
	for d := 0; d <= 6; d++ {
		p := math.Pow(10, float64(d))
		if math.Abs(step*p-math.Round(step*p)) < 1e-9 {
			return d
		}
	}
	return -1
}

// pick returns the one filled cell of a group, nil when none is, and an
// AmbiguousError when more than one is.
func pick(widget string, part spec.Part, cells []Cell) (*Cell, error) {
	var filled []Cell
	for _, c := range cells {
		if c.Part == part && c.Filled {
			filled = append(filled, c)
		}
	}
	switch len(filled) {
	case 0:
		return nil, nil
	case 1:
		return &filled[0], nil
	default:
		sort.SliceStable(filled, func(i, j int) bool { return filled[i].Fill > filled[j].Fill })
		return nil, &AmbiguousError{Widget: widget, Part: part, Cells: filled}
	}
}

// date reads the three sub-groups as one day. All three empty is an unanswered
// date (the zero Time); some but not all is an IncompleteError; a combination that
// is not a real day (31 February) is reported rather than silently normalized, the
// way time.Date would.
func date(sw spec.Widget, cells []Cell) (time.Time, error) {
	var picked [3]*Cell
	var missing []spec.Part
	for i, p := range [3]spec.Part{spec.Day, spec.Month, spec.Year} {
		c, err := pick(sw.Name, p, cells)
		if err != nil {
			return time.Time{}, err
		}
		if c == nil {
			missing = append(missing, p)
			continue
		}
		picked[i] = c
	}
	if len(missing) == 3 {
		return time.Time{}, nil
	}
	if len(missing) > 0 {
		return time.Time{}, &IncompleteError{Widget: sw.Name, Missing: missing}
	}
	d, mo, y := num(sw, picked[0]), num(sw, picked[1]), num(sw, picked[2])
	t := time.Date(y, time.Month(mo), d, 0, 0, 0, 0, time.UTC)
	if t.Day() != d || int(t.Month()) != mo || t.Year() != y {
		return time.Time{}, fmt.Errorf("%s: %04d-%02d-%02d is not a real date", sw.Name, y, mo, d)
	}
	return t, nil
}

// num is a picked date cell's number, taken from the spec cell that declared it
// (parsed and range-checked once, at Build).
func num(sw spec.Widget, c *Cell) int {
	for _, sc := range sw.Cells {
		if sc.Part == c.Part && sc.Key == c.Key {
			return sc.Num
		}
	}
	return 0
}

// MarshalJSON renders a widget with its resolved value and, when it failed, the
// reason — so the JSON a front-end reads is self-contained: what the widget is,
// what was measured, what it means and why it could not be read.
func (w Widget) MarshalJSON() ([]byte, error) {
	type alias Widget
	out := struct {
		alias
		Value any    `json:"value"`
		Date  string `json:"date,omitempty"`
		Error string `json:"error,omitempty"`
	}{alias: alias(w), Value: w.Value()}
	if w.Kind == spec.Date && w.Set {
		out.Date = w.Date.Format("2006-01-02")
	}
	if w.Err != nil {
		out.Error = w.Err.Error()
	}
	return json.Marshal(out)
}
