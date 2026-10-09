package spec

import (
	"fmt"
	"sort"
)

// Validate checks a set of template boxes as a form and returns the widgets it
// declares together with every problem found. It is what `snif check` reports and
// what a front-end runs before trusting a hand-edited config.
//
// A broken form is worth catching early because its failure mode is quiet: two
// overlapping cells still read, they just both answer for the same ink, and a
// widget whose analyze flag is wrong simply never gets its text.
func Validate(boxes []Box) ([]Widget, []error) {
	widgets, err := Build(boxes)
	if err != nil {
		return nil, []error{err}
	}
	var problems []error

	// An Input box is the one snorg must transcribe; every other box must be left
	// alone, or analysis spends a vision call on a tick box.
	declared := map[string]Kind{}
	for _, w := range widgets {
		declared[ID{Kind: w.Kind, Name: w.Name, Min: w.Min, Max: w.Max, Step: w.Step, Year0: w.Year0, Year1: w.Year1}.String()] = w.Kind
	}
	for _, b := range boxes {
		kind, ok := declared[b.ID]
		if !ok {
			if id, err := Parse(b.ID); err == nil && id.IsCell() && b.Analyze {
				problems = append(problems, fmt.Errorf("box %q: a cell must not be analyze: true", b.ID))
			}
			continue
		}
		if want := kind.Analyzed(); b.Analyze != want {
			problems = append(problems, fmt.Errorf("box %q: analyze must be %t for a %s", b.ID, want, kind))
		}
	}
	problems = append(problems, geometry(widgets)...)
	return widgets, problems
}

// geometry checks the rects snif measures ink in: each must lie on the page, and
// no two may overlap — an overlap would let one widget read another's answer.
func geometry(widgets []Widget) []error {
	type named struct {
		what string
		r    Rect
	}
	var rects []named
	for _, w := range widgets {
		if w.Kind.Grouped() {
			for _, c := range w.Cells {
				rects = append(rects, named{fmt.Sprintf("%s:%s cell %s", w.Kind, w.Name, c.Key), c.Rect})
			}
			continue
		}
		rects = append(rects, named{fmt.Sprintf("%s:%s", w.Kind, w.Name), w.Rect})
	}
	page := Rect{W: PageW, H: PageH}
	var problems []error
	for _, n := range rects {
		if n.r.Empty() {
			problems = append(problems, fmt.Errorf("%s has an empty rect", n.what))
		} else if !page.Contains(n.r) {
			problems = append(problems, fmt.Errorf("%s runs off the page: %+v", n.what, n.r))
		}
	}
	sort.SliceStable(rects, func(i, j int) bool { return rects[i].r.Y < rects[j].r.Y })
	for i, a := range rects {
		for _, b := range rects[i+1:] {
			if b.r.Y >= a.r.Y+a.r.H {
				break
			}
			if a.r.Overlaps(b.r) {
				problems = append(problems, fmt.Errorf("%s overlaps %s", a.what, b.what))
			}
		}
	}
	return problems
}
