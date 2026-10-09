package spec

import (
	"errors"
	"strings"
	"testing"
)

// TestParseRoundTrip: every well-formed id parses to the documented ID and
// formats back to itself — the invariant that lets the generator emit ids by
// building an ID rather than by printf.
func TestParseRoundTrip(t *testing.T) {
	cases := []struct {
		id   string
		want ID
	}{
		{"check:done", ID{Kind: Check, Name: "done"}},
		{"button:submit", ID{Kind: Button, Name: "submit"}},
		{"input:notes", ID{Kind: Input, Name: "notes"}},
		{"slider:mood:0:10", ID{Kind: Slider, Name: "mood", Max: 10}},
		{"slider:mood:0:10:0.5", ID{Kind: Slider, Name: "mood", Max: 10, Step: 0.5}},
		{"slider:temp:-5:42.5", ID{Kind: Slider, Name: "temp", Min: -5, Max: 42.5}},
		{"radio:prio", ID{Kind: Radio, Name: "prio"}},
		{"radio:prio:low", ID{Kind: Radio, Name: "prio", Cell: "low"}},
		{"date:due:2026:2030", ID{Kind: Date, Name: "due", Year0: 2026, Year1: 2030}},
		{"date:due:day:07", ID{Kind: Date, Name: "due", Part: Day, Cell: "07"}},
		{"date:due:month:12", ID{Kind: Date, Name: "due", Part: Month, Cell: "12"}},
		{"date:due:year:2026", ID{Kind: Date, Name: "due", Part: Year, Cell: "2026"}},
	}
	for _, c := range cases {
		got, err := Parse(c.id)
		if err != nil {
			t.Errorf("Parse(%q): %v", c.id, err)
			continue
		}
		if got != c.want {
			t.Errorf("Parse(%q) = %+v, want %+v", c.id, got, c.want)
		}
		if back := got.String(); back != c.id {
			t.Errorf("String() = %q, want %q", back, c.id)
		}
	}
}

// TestParseNotWidget: an id that does not start with a known kind is an ordinary
// snorg box, reported as ErrNotWidget so a reader can skip it without treating the
// template as broken.
func TestParseNotWidget(t *testing.T) {
	for _, id := range []string{"title", "notes", "figure-1", "", "checkbox:done"} {
		_, err := Parse(id)
		if !errors.Is(err, ErrNotWidget) {
			t.Errorf("Parse(%q) error = %v, want ErrNotWidget", id, err)
		}
	}
}

// TestParseMalformed: a known kind with a broken tail is an authoring bug, not an
// ordinary box — it must be reported rather than skipped.
func TestParseMalformed(t *testing.T) {
	for _, id := range []string{
		"check:", "check:done:extra", "input:a b",
		"slider:mood", "slider:mood:0", "slider:mood:lo:hi", "slider:mood:10:0",
		"slider:mood:0:10:0", "slider:mood:0:10:1:2",
		"radio:prio:a:b", "date:due", "date:due:2030:2026", "date:due:day:32",
		"date:due:month:0", "date:due:year:26", "date:due:20xx:2030",
	} {
		_, err := Parse(id)
		if err == nil {
			t.Errorf("Parse(%q) succeeded, want an error", id)
			continue
		}
		if errors.Is(err, ErrNotWidget) {
			t.Errorf("Parse(%q) = ErrNotWidget, want a syntax error", id)
		}
	}
}

// boxesOf is the generator's path: widgets to boxes.
func boxesOf(ws ...Widget) []Box {
	var out []Box
	for _, w := range ws {
		out = append(out, w.Boxes()...)
	}
	return out
}

func dateWidget() Widget {
	w := Widget{Kind: Date, Name: "due", Label: "Due date", Rect: Rect{X: 0, Y: 0, W: 1920, H: 600}, Year0: 2026, Year1: 2027}
	for d := 1; d <= 31; d++ {
		w.Cells = append(w.Cells, Cell{Key: CellKey(Day, d), Label: "x", Part: Day, Num: d})
	}
	for m := 1; m <= 12; m++ {
		w.Cells = append(w.Cells, Cell{Key: CellKey(Month, m), Label: "x", Part: Month, Num: m})
	}
	for y := 2026; y <= 2027; y++ {
		w.Cells = append(w.Cells, Cell{Key: CellKey(Year, y), Label: "x", Part: Year, Num: y})
	}
	return w
}

// TestBuildRoundTrip: Build is the exact inverse of Widget.Boxes, so a template
// the generator writes is a template the reader understands — the one invariant
// that keeps the two halves of snif from drifting.
func TestBuildRoundTrip(t *testing.T) {
	want := []Widget{
		{Kind: Check, Name: "done", Label: "Done", Rect: Rect{X: 1, Y: 2, W: 3, H: 4}},
		{Kind: Slider, Name: "mood", Label: "Mood", Rect: Rect{X: 5, Y: 6, W: 7, H: 8}, Max: 10, Step: 0.5},
		{Kind: Radio, Name: "prio", Label: "Priority", Rect: Rect{X: 9, Y: 9, W: 9, H: 9}, Cells: []Cell{
			{Key: "low", Label: "low"}, {Key: "mid", Label: "mid"}, {Key: "high", Label: "high"},
		}},
		dateWidget(),
		{Kind: Input, Name: "notes", Label: "Notes", Rect: Rect{X: 2, Y: 2, W: 2, H: 2}},
	}
	got, err := Build(boxesOf(want...))
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if len(got) != len(want) {
		t.Fatalf("got %d widgets, want %d", len(got), len(want))
	}
	for i := range got {
		if got[i].Kind != want[i].Kind || got[i].Name != want[i].Name ||
			got[i].Label != want[i].Label || got[i].Rect != want[i].Rect ||
			got[i].Min != want[i].Min || got[i].Max != want[i].Max || got[i].Step != want[i].Step ||
			got[i].Year0 != want[i].Year0 || got[i].Year1 != want[i].Year1 ||
			len(got[i].Cells) != len(want[i].Cells) {
			t.Errorf("widget %d = %+v, want %+v", i, got[i], want[i])
		}
	}
	// Radio options keep their authored order; the input box is the only analyzed one.
	if keys := keysOf(got[2].Cells); keys != "low,mid,high" {
		t.Errorf("radio cell order = %s", keys)
	}
	var analyzed []string
	for _, b := range boxesOf(want...) {
		if b.Analyze {
			analyzed = append(analyzed, b.ID)
		}
	}
	if len(analyzed) != 1 || analyzed[0] != "input:notes" {
		t.Errorf("analyze:true boxes = %v, want [input:notes]", analyzed)
	}
}

func keysOf(cells []Cell) string {
	var k []string
	for _, c := range cells {
		k = append(k, c.Key)
	}
	return strings.Join(k, ",")
}

// TestBuildSkipsForeignBoxes: a snif template may hold ordinary snorg boxes; they
// are not widgets and must not break the form.
func TestBuildSkipsForeignBoxes(t *testing.T) {
	boxes := append([]Box{{ID: "title", Label: "Title"}}, Widget{Kind: Check, Name: "done"}.Boxes()...)
	got, err := Build(boxes)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if len(got) != 1 || got[0].Name != "done" {
		t.Fatalf("got %+v, want just the check widget", got)
	}
}

// TestBuildErrors: the authoring mistakes Build must catch.
func TestBuildErrors(t *testing.T) {
	cases := map[string][]Box{
		"cell without group":  {{ID: "radio:prio:low"}},
		"group without cells": {{ID: "radio:prio"}},
		"duplicate id":        {{ID: "check:done"}, {ID: "check:done"}},
		"cells on a check":    {{ID: "check:done"}, {ID: "radio:done:x"}},
		"duplicate cell":      {{ID: "radio:p"}, {ID: "radio:p:a"}, {ID: "radio:p:a"}},
		"date missing part":   {{ID: "date:d:2026:2026"}, {ID: "date:d:day:01"}, {ID: "date:d:month:01"}},
		"year out of range":   {{ID: "date:d:2026:2026"}, {ID: "date:d:day:01"}, {ID: "date:d:month:01"}, {ID: "date:d:year:2030"}},
	}
	for name, boxes := range cases {
		if _, err := Build(boxes); err == nil {
			t.Errorf("%s: Build succeeded, want an error", name)
		}
	}
}

// TestBuildSortsDateCells: date cells come back in calendar order whatever order
// the config listed them in, so a reader can trust Parts(Day)[0] is day 1.
func TestBuildSortsDateCells(t *testing.T) {
	boxes := []Box{{ID: "date:d:2026:2026"}}
	for _, id := range []string{"date:d:year:2026", "date:d:month:03", "date:d:day:09", "date:d:day:02", "date:d:month:01"} {
		boxes = append(boxes, Box{ID: id})
	}
	got, err := Build(boxes)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if keys := keysOf(got[0].Cells); keys != "02,09,01,03,2026" {
		t.Errorf("cell order = %s, want days, months, years each ascending", keys)
	}
}
