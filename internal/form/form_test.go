package form

import (
	"errors"
	"testing"

	"github.com/jdlugosz963/snif/internal/ink"
	"github.com/jdlugosz963/snif/internal/spec"
)

// fill paints a rect's whole area as ink in the mask.
func fill(m *ink.Mask, r spec.Rect) {
	for y := r.Y; y < r.Y+r.H; y++ {
		for x := r.X; x < r.X+r.W; x++ {
			m.Set(x, y)
		}
	}
}

func cellRect(i int) spec.Rect { return spec.Rect{X: 100 + i*60, Y: 100, W: 50, H: 50} }

func radio() spec.Widget {
	w := spec.Widget{Kind: spec.Radio, Name: "prio", Label: "Priority", Rect: spec.Rect{X: 100, Y: 60, W: 500, H: 100}}
	for i, k := range []string{"low", "mid", "high"} {
		w.Cells = append(w.Cells, spec.Cell{Key: k, Label: k, Rect: cellRect(i)})
	}
	return w
}

func TestRadio(t *testing.T) {
	w := radio()
	m := ink.New(spec.PageW, spec.PageH)
	fill(m, spec.Rect{X: 110, Y: 110, W: 20, H: 20}) // a tick in cell 0

	got := Resolve([]spec.Widget{w}, m, nil, Options{})[0]
	if got.Err != nil || !got.Set || got.Choice != "low" {
		t.Fatalf("got %+v", got)
	}
	if got.Value() != "low" {
		t.Errorf("Value = %v", got.Value())
	}

	// Untouched: unanswered, not an error.
	empty := Resolve([]spec.Widget{w}, ink.New(spec.PageW, spec.PageH), nil, Options{})[0]
	if empty.Err != nil || empty.Set {
		t.Errorf("an untouched radio must be unset without error, got %+v", empty)
	}

	// Two ticks: an error that names both.
	fill(m, spec.Rect{X: 170, Y: 110, W: 20, H: 20})
	two := Resolve([]spec.Widget{w}, m, nil, Options{})[0]
	var amb *AmbiguousError
	if !errors.As(two.Err, &amb) {
		t.Fatalf("want AmbiguousError, got %v", two.Err)
	}
	if len(amb.Cells) != 2 {
		t.Errorf("AmbiguousError names %d cells, want 2", len(amb.Cells))
	}
	if two.Set {
		t.Error("an ambiguous radio must not carry a value")
	}
}

func TestCheck(t *testing.T) {
	w := spec.Widget{Kind: spec.Check, Name: "done", Rect: cellRect(0)}
	m := ink.New(spec.PageW, spec.PageH)
	if got := Resolve([]spec.Widget{w}, m, nil, Options{})[0]; !got.Set || got.Bool {
		t.Errorf("empty check = %+v, want set and false", got)
	}
	fill(m, spec.Rect{X: 110, Y: 110, W: 10, H: 10})
	if got := Resolve([]spec.Widget{w}, m, nil, Options{})[0]; !got.Set || !got.Bool {
		t.Errorf("ticked check = %+v, want true", got)
	}
}

// TestThreshold: a stray speck below the threshold is not an answer, and the
// threshold is tunable.
func TestThreshold(t *testing.T) {
	w := spec.Widget{Kind: spec.Check, Name: "done", Rect: spec.Rect{X: 0, Y: 0, W: 100, H: 100}}
	m := ink.New(200, 200)
	fill(m, spec.Rect{X: 0, Y: 0, W: 2, H: 2}) // 4 of 10000 px = 0.04%
	if Resolve([]spec.Widget{w}, m, nil, Options{})[0].Bool {
		t.Error("a speck must not tick a box")
	}
	if !Resolve([]spec.Widget{w}, m, nil, Options{Threshold: 0.0001})[0].Bool {
		t.Error("a lowered threshold must see the speck")
	}
}

func TestSlider(t *testing.T) {
	track := spec.Rect{X: 100, Y: 500, W: 1000, H: 120}
	w := spec.Widget{Kind: spec.Slider, Name: "mood", Rect: track, Max: 10}
	cases := []struct {
		markX int
		step  float64
		want  float64
	}{
		{100, 0, 0.01},  // the very first column
		{599, 0, 5},     // halfway
		{1099, 0, 10},   // the last column
		{650, 0.5, 5.5}, // snapped
		{640, 1, 5},
	}
	for _, c := range cases {
		w.Step = c.step
		m := ink.New(spec.PageW, spec.PageH)
		fill(m, spec.Rect{X: c.markX, Y: 520, W: 1, H: 40})
		got := Resolve([]spec.Widget{w}, m, nil, Options{})[0]
		if !got.Set || got.Number != c.want {
			t.Errorf("mark at x=%d step=%v → %v (set=%v), want %v", c.markX, c.step, got.Number, got.Set, c.want)
		}
	}
	// Only the rightmost mark counts: a user who crosses out one value and marks a
	// higher one gets the higher one.
	m := ink.New(spec.PageW, spec.PageH)
	fill(m, spec.Rect{X: 200, Y: 520, W: 4, H: 40})
	fill(m, spec.Rect{X: 599, Y: 520, W: 1, H: 40})
	w.Step = 0
	if got := Resolve([]spec.Widget{w}, m, nil, Options{})[0]; got.Number != 5 {
		t.Errorf("rightmost ink = %v, want 5", got.Number)
	}
	if got := Resolve([]spec.Widget{w}, ink.New(spec.PageW, spec.PageH), nil, Options{})[0]; got.Set || got.Err != nil {
		t.Errorf("an untouched slider = %+v, want unset without error", got)
	}
}

func dateWidget() spec.Widget {
	w := spec.Widget{Kind: spec.Date, Name: "due", Rect: spec.Rect{X: 0, Y: 0, W: 1920, H: 800}, Year0: 2026, Year1: 2027}
	add := func(p spec.Part, n, i, row int) {
		w.Cells = append(w.Cells, spec.Cell{
			Key: spec.CellKey(p, n), Part: p, Num: n,
			Rect: spec.Rect{X: 20 + i*50, Y: 100 + row*100, W: 40, H: 40},
		})
	}
	for d := 1; d <= 31; d++ {
		add(spec.Day, d, d-1, 0)
	}
	for mo := 1; mo <= 12; mo++ {
		add(spec.Month, mo, mo-1, 1)
	}
	for y := 2026; y <= 2027; y++ {
		add(spec.Year, y, y-2026, 2)
	}
	return w
}

func TestDate(t *testing.T) {
	w := dateWidget()
	tick := func(m *ink.Mask, p spec.Part, n int) {
		for _, c := range w.Cells {
			if c.Part == p && c.Num == n {
				fill(m, spec.Rect{X: c.Rect.X + 5, Y: c.Rect.Y + 5, W: 15, H: 15})
			}
		}
	}

	m := ink.New(spec.PageW, spec.PageH)
	tick(m, spec.Day, 14)
	tick(m, spec.Month, 9)
	tick(m, spec.Year, 2026)
	got := Resolve([]spec.Widget{w}, m, nil, Options{})[0]
	if got.Err != nil || !got.Set || got.Date.Format("2006-01-02") != "2026-09-14" {
		t.Fatalf("got %+v (%v)", got, got.Err)
	}
	if got.Value() != "2026-09-14" {
		t.Errorf("Value = %v", got.Value())
	}

	// A day and a month but no year: incomplete, and the message says which.
	partial := ink.New(spec.PageW, spec.PageH)
	tick(partial, spec.Day, 14)
	tick(partial, spec.Month, 9)
	var inc *IncompleteError
	if err := Resolve([]spec.Widget{w}, partial, nil, Options{})[0].Err; !errors.As(err, &inc) {
		t.Fatalf("want IncompleteError, got %v", err)
	} else if len(inc.Missing) != 1 || inc.Missing[0] != spec.Year {
		t.Errorf("missing = %v, want [year]", inc.Missing)
	}

	// 31 February is reported, not normalized into March.
	bad := ink.New(spec.PageW, spec.PageH)
	tick(bad, spec.Day, 31)
	tick(bad, spec.Month, 2)
	tick(bad, spec.Year, 2026)
	if err := Resolve([]spec.Widget{w}, bad, nil, Options{})[0].Err; err == nil {
		t.Error("31 February must be reported as not a real date")
	}

	// Nothing ticked at all: unanswered, not incomplete.
	if got := Resolve([]spec.Widget{w}, ink.New(spec.PageW, spec.PageH), nil, Options{})[0]; got.Set || got.Err != nil {
		t.Errorf("an untouched date = %+v, want unset without error", got)
	}
}

func TestInput(t *testing.T) {
	w := spec.Widget{Kind: spec.Input, Name: "notes", Rect: spec.Rect{X: 0, Y: 0, W: 100, H: 100}}
	m := ink.New(200, 200)

	got := Resolve([]spec.Widget{w}, m, func(string) (string, bool) { return "call the dentist", true }, Options{})[0]
	if !got.Set || got.Text != "call the dentist" {
		t.Errorf("got %+v", got)
	}
	// Analyzed but blank is a real, empty answer.
	blank := Resolve([]spec.Widget{w}, m, func(string) (string, bool) { return "", true }, Options{})[0]
	if !blank.Set || blank.Text != "" || blank.Err != nil {
		t.Errorf("an analyzed blank box = %+v, want set and empty", blank)
	}
	// Never analyzed is unknown, and says so.
	unknown := Resolve([]spec.Widget{w}, m, func(string) (string, bool) { return "", false }, Options{})[0]
	if !errors.Is(unknown.Err, ErrNotAnalyzed) || unknown.Set {
		t.Errorf("an unanalyzed box = %+v (%v), want ErrNotAnalyzed", unknown, unknown.Err)
	}
}
