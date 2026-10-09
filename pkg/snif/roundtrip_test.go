package snif_test

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"strings"
	"testing"
	"time"

	"github.com/jdlugosz963/snif/pkg/snif"
)

// template is the one-of-every-widget form the round-trip is exercised on, built
// through the same argument syntax a user types.
func template(t *testing.T) *snif.Generated {
	t.Helper()
	args, err := snif.ParseGen([]string{
		"daily",
		"--title", "Daily plan",
		"--theme", "dots",
		"--date", "due:2026-2030:label=Due date",
		"--radio", "prio:low,mid,high:label=Priority",
		"--slider", "mood:0:10:step=0.5:label=Mood",
		"--check", "done:label=Done today",
		"--button", "submit:label=File this page",
		"--input", "notes:style=lines:w=1400:h=520:label=Notes",
	})
	if err != nil {
		t.Fatalf("ParseGen: %v", err)
	}
	g, err := snif.Generate(args.Template)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	return g
}

// page is a blank canonical page render that ink can be painted into — what snorg
// hands back for a page whose strokes land where the test puts them.
type page struct{ img *image.Gray }

func newPage() *page {
	img := image.NewGray(image.Rect(0, 0, snif.PageW, snif.PageH))
	for i := range img.Pix {
		img.Pix[i] = 0xff
	}
	return &page{img}
}

// tick paints a stroke inside the box with the given id, the way a pen would.
func (p *page) tick(t *testing.T, g *snif.Generated, id string) {
	t.Helper()
	for _, b := range g.Boxes() {
		if b.ID != id {
			continue
		}
		r := b.Rect
		p.mark(r.X+r.W/4, r.Y+r.H/4, r.W/2, r.H/2)
		return
	}
	t.Fatalf("template has no box %q", id)
}

// mark paints a filled rectangle of ink.
func (p *page) mark(x, y, w, h int) {
	for yy := y; yy < y+h; yy++ {
		for xx := x; xx < x+w; xx++ {
			p.img.SetGray(xx, yy, color.Gray{})
		}
	}
}

// markAt paints a short vertical stroke at an absolute x inside a box — a slider
// mark.
func (p *page) slider(t *testing.T, g *snif.Generated, id string, frac float64) {
	t.Helper()
	for _, b := range g.Boxes() {
		if b.ID != id {
			continue
		}
		x := b.Rect.X + int(frac*float64(b.Rect.W-1))
		p.mark(x, b.Rect.Y+10, 3, b.Rect.H-20)
		return
	}
	t.Fatalf("template has no box %q", id)
}

func (p *page) render(t *testing.T) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := png.Encode(&buf, p.img); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// TestRoundTrip is the headline check: a template is generated, ink is painted
// into the cells its own layout declares, and the form reads back exactly the
// answers that were painted. It ties the two halves of snif together — the boxes
// the generator writes are the boxes the reader resolves — without an archive.
func TestRoundTrip(t *testing.T) {
	g := template(t)
	p := newPage()
	p.tick(t, g, "date:due:day:14")
	p.tick(t, g, "date:due:month:09")
	p.tick(t, g, "date:due:year:2026")
	p.tick(t, g, "radio:prio:mid")
	p.tick(t, g, "check:done")
	p.slider(t, g, "slider:mood:0:10:0.5", 0.7)

	form, err := snif.ReadRender(g.Boxes(), p.render(t), map[string]string{
		"input:notes": "call the dentist",
	}, snif.ReadOptions{})
	if err != nil {
		t.Fatalf("ReadRender: %v", err)
	}
	if err := form.Err(); err != nil {
		t.Fatalf("form errors: %v", err)
	}

	want := map[string]any{
		"due": "2026-09-14", "prio": "mid", "mood": 7.0,
		"done": true, "submit": false, "notes": "call the dentist",
	}
	got := form.Values()
	if len(got) != len(want) {
		t.Fatalf("read %d widgets, want %d: %v", len(got), len(want), got)
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("%s = %#v, want %#v", k, got[k], v)
		}
	}

	// The typed accessors and struct decoding see the same answers.
	var v struct {
		Due   time.Time `snif:"due"`
		Prio  string    `snif:"prio"`
		Mood  float64   `snif:"mood"`
		Done  bool      `snif:"done"`
		Notes string    `snif:"notes,required"`
		Skip  string    `snif:"-"`
	}
	if err := form.Decode(&v); err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if v.Due.Format("2006-01-02") != "2026-09-14" || v.Prio != "mid" || v.Mood != 7 || !v.Done || v.Notes != "call the dentist" {
		t.Errorf("decoded %+v", v)
	}
	if d, err := form.Date("due"); err != nil || !d.Equal(time.Date(2026, 9, 14, 0, 0, 0, 0, time.UTC)) {
		t.Errorf("Date = %v, %v", d, err)
	}
}

// TestRoundTripBlankPage: an untouched page is a form with no answers, not a
// broken one — every widget reads unset, and only the tick boxes (which always
// have an answer) come back set and false.
func TestRoundTripBlankPage(t *testing.T) {
	g := template(t)
	form, err := snif.ReadRender(g.Boxes(), newPage().render(t), map[string]string{"input:notes": ""}, snif.ReadOptions{})
	if err != nil {
		t.Fatalf("ReadRender: %v", err)
	}
	if err := form.Err(); err != nil {
		t.Fatalf("a blank page must not error: %v", err)
	}
	for _, w := range form.Widgets {
		switch w.Kind {
		case snif.Check, snif.Button:
			if !w.Set || w.Bool {
				t.Errorf("%s = %+v, want set and false", w.Name, w)
			}
		case snif.Input:
			if !w.Set || w.Text != "" {
				t.Errorf("%s = %+v, want an empty transcription", w.Name, w)
			}
		default:
			if w.Set {
				t.Errorf("%s = %+v, want unset", w.Name, w)
			}
		}
	}
}

// TestRoundTripAmbiguous: two ticks in one group is the error the user asked for,
// and it names both cells while the rest of the form still reads.
func TestRoundTripAmbiguous(t *testing.T) {
	g := template(t)
	p := newPage()
	p.tick(t, g, "radio:prio:low")
	p.tick(t, g, "radio:prio:high")
	p.tick(t, g, "check:done")

	form, err := snif.ReadRender(g.Boxes(), p.render(t), nil, snif.ReadOptions{})
	if err != nil {
		t.Fatalf("ReadRender: %v", err)
	}
	w, _ := form.Widget("prio")
	var amb *snif.AmbiguousError
	if !asAmbiguous(w.Err, &amb) {
		t.Fatalf("prio error = %v, want AmbiguousError", w.Err)
	}
	if len(amb.Cells) != 2 || !strings.Contains(amb.Error(), "low") || !strings.Contains(amb.Error(), "high") {
		t.Errorf("error %q must name both ticked cells", amb.Error())
	}
	if b, err := form.Bool("done"); err != nil || !b {
		t.Errorf("one bad group must not hide the rest of the form: done = %v, %v", b, err)
	}
	if form.Values()["prio"] != nil {
		t.Error("an ambiguous group must not report a value")
	}
}

func asAmbiguous(err error, dst **snif.AmbiguousError) bool {
	a, ok := err.(*snif.AmbiguousError)
	if ok {
		*dst = a
	}
	return ok
}

// TestUnanalyzedInput: a box nobody has transcribed is unknown, not empty.
func TestUnanalyzedInput(t *testing.T) {
	g := template(t)
	form, err := snif.ReadRender(g.Boxes(), newPage().render(t), nil, snif.ReadOptions{})
	if err != nil {
		t.Fatalf("ReadRender: %v", err)
	}
	w, _ := form.Widget("notes")
	if w.Err == nil || w.Set {
		t.Errorf("notes = %+v, want ErrNotAnalyzed", w)
	}
}
