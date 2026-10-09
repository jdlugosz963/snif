package snif

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/jdlugosz963/snif/internal/form"
	"github.com/jdlugosz963/snif/internal/ink"
	"github.com/jdlugosz963/snif/internal/spec"
	snorg "github.com/jdlugosz963/snorg/pkg/snorg"
)

// ErrNotTemplated is returned for a page whose background matches no configured
// template — it was not drawn on a form at all.
var ErrNotTemplated = errors.New("page is not drawn on a configured template")

// ErrNotSnif is returned for a page drawn on a template that declares no widgets:
// a perfectly good snorg template, but not a snif form. Snif-compatibility is
// exactly this — a template whose boxes carry widget ids — never a filename.
var ErrNotSnif = errors.New("page's template declares no snif widgets")

// ReadOptions tunes how a page is read.
type ReadOptions struct {
	// Analyze allows snif to transcribe Input widgets whose transcription is
	// missing or out of date, by running snorg's analysis for that page (which
	// needs provider credentials in the config). snorg decides which boxes that
	// is: it skips a page whose per-box ink fingerprints all still match, so an
	// unchanged page costs no LLM call. Left false, an unanalyzed Input widget
	// comes back with ErrNotAnalyzed and nothing is spent, and a stale one reads
	// as whatever was last transcribed. A form with no Input widget never
	// analyzes and never builds a provider either way.
	Analyze bool

	// Threshold is the inked fraction at which a cell counts as ticked; 0 selects
	// DefaultThreshold.
	Threshold float64
}

// Form is one page read as a form.
type Form struct {
	PageID   string   `json:"page_id"`
	FileID   string   `json:"file_id"`
	Template string   `json:"template"` // the matched template's image basename
	Widgets  []Widget `json:"widgets"`
}

// Result is one page's outcome in a batch: the form, or the error that page
// failed with. A failure never aborts the batch.
type Result struct {
	PageID string `json:"page_id"`
	Form   *Form  `json:"form,omitempty"`
	Err    error  `json:"-"`
}

// Box is one template box a form is read against: the widget-bearing id, the
// human label, the rect and whether snorg transcribes it.
type Box = spec.Box

// ReadRender resolves a form from a page's canonical render and the boxes of the
// template it was drawn on — the archive-free core that Client.Form wraps.
//
// render is a PNG of the page as snorg renders it for analysis: handwriting on
// white, no template background (Client.RenderRegion with styled=false). text
// supplies the transcription of each Input box by id; a key present with an empty
// value is a box that was analyzed and held no writing, while a missing key is a
// box nobody has read yet, which is reported as ErrNotAnalyzed rather than as an
// empty answer.
//
// It is what a program with its own rendering pipeline — or a test that paints
// ink into a blank page — reads a form with.
func ReadRender(boxes []Box, render []byte, text map[string]string, opts ReadOptions) (*Form, error) {
	widgets, err := spec.Build(boxes)
	if err != nil {
		return nil, err
	}
	if len(widgets) == 0 {
		return nil, ErrNotSnif
	}
	mask, err := ink.Decode(render)
	if err != nil {
		return nil, err
	}
	return &Form{Widgets: form.Resolve(widgets, mask, lookup(text), form.Options{Threshold: opts.Threshold})}, nil
}

// lookup turns a transcription map into the text seam, keyed by Input box id.
func lookup(text map[string]string) form.Text {
	if text == nil {
		return nil
	}
	return func(name string) (string, bool) {
		t, ok := text[spec.ID{Kind: Input, Name: name}.String()]
		return t, ok
	}
}

// Form reads one page. It costs a single page rasterization however many widgets
// the form holds, plus — when the form holds an Input widget and
// ReadOptions.Analyze is set — one snorg analysis pass, which rasterizes the page
// again to fingerprint it and transcribes only the boxes whose ink changed.
func (c *Client) Form(ctx context.Context, pageID string, opts ReadOptions) (*Form, error) {
	fileID, err := c.sn.FindPage(pageID)
	if err != nil {
		return nil, err
	}
	boxes, tmplName, err := c.boxes(fileID, pageID)
	if err != nil {
		return nil, err
	}
	widgets, err := spec.Build(boxes)
	if err != nil {
		return nil, fmt.Errorf("page %s: %w", pageID, err)
	}
	if len(widgets) == 0 {
		return nil, fmt.Errorf("page %s: %w", pageID, ErrNotSnif)
	}

	text, err := c.text(ctx, fileID, pageID, widgets, opts)
	if err != nil {
		return nil, err
	}

	// One rasterization per page: snorg's RenderRegion renders the whole SVG and
	// then crops, so asking it per cell would re-render the page dozens of times.
	png, err := c.sn.RenderRegion(pageID, snorg.Rect{W: PageW, H: PageH}, false)
	if err != nil {
		return nil, fmt.Errorf("render page %s: %w", pageID, err)
	}
	mask, err := ink.Decode(png)
	if err != nil {
		return nil, fmt.Errorf("page %s: %w", pageID, err)
	}

	return &Form{
		PageID:   pageID,
		FileID:   fileID,
		Template: tmplName,
		Widgets:  form.Resolve(widgets, mask, text, form.Options{Threshold: opts.Threshold}),
	}, nil
}

// Forms reads several pages, preserving input order. A page that fails lands in
// its Result.Err instead of aborting the batch — one unreadable page in a query's
// output never costs the rest.
func (c *Client) Forms(ctx context.Context, pageIDs []string, opts ReadOptions) []Result {
	out := make([]Result, 0, len(pageIDs))
	for _, id := range pageIDs {
		f, err := c.Form(ctx, id, opts)
		out = append(out, Result{PageID: id, Form: f, Err: err})
	}
	return out
}

// boxes returns a page's template boxes as snif sees them, plus the matched
// template's image basename. It reads them from snorg's retrieve view, which
// resolves one entry per configured box — id, label, rect and transcription — in
// a single call.
func (c *Client) boxes(fileID, pageID string) ([]spec.Box, string, error) {
	res, err := c.sn.Retrieve([]string{pageID})
	if err != nil {
		return nil, "", err
	}
	regions := regionsOf(res, pageID)
	if len(regions) == 0 {
		return nil, "", fmt.Errorf("page %s: %w", pageID, ErrNotTemplated)
	}
	boxes := make([]spec.Box, 0, len(regions))
	for _, r := range regions {
		boxes = append(boxes, spec.Box{
			ID:    r.ID,
			Label: r.Label,
			Rect:  Rect{X: r.Rect.X, Y: r.Rect.Y, W: r.Rect.W, H: r.Rect.H},
		})
	}
	return boxes, c.templateName(pageID), nil
}

func regionsOf(res *snorg.Result, pageID string) []snorg.RegionView {
	for _, n := range res.Notes {
		for _, p := range n.Pages {
			if p.PageID == pageID && p.Analysis != nil {
				return p.Analysis.Regions
			}
		}
	}
	return nil
}

// templateName is the matched template's image basename, the human name of the
// form a page was drawn on. It is informational only — what makes a page readable
// is its boxes, not this name.
func (c *Client) templateName(pageID string) string {
	pd, err := c.sn.ReadPage(pageID)
	if err != nil {
		return ""
	}
	t := c.sn.Templates().MatchBackground(pd.BackgroundHash)
	if t == nil {
		return ""
	}
	base := strings.TrimSuffix(filepath.Base(t.Image), filepath.Ext(t.Image))
	return strings.TrimPrefix(base, "snif-")
}

// text builds the lookup Input widgets are resolved through, handing the page to
// snorg's analysis first when it holds an Input box and the caller allowed it.
//
// Which boxes need transcribing is snorg's decision, not snif's: it compares each
// analyze box's stored fingerprint against the page's current ink and re-reads
// only what moved, so ink added to a box that was already transcribed is picked
// up — which asking "has this box ever been read?" here would miss. A form with
// no Input widget never reaches the provider at all, so credential-free forms
// stay credential-free.
func (c *Client) text(ctx context.Context, fileID, pageID string, widgets []spec.Widget, opts ReadOptions) (form.Text, error) {
	input := false
	for _, w := range widgets {
		if w.Kind == Input {
			input = true
			break
		}
	}
	if !input {
		return nil, nil
	}

	if opts.Analyze {
		if err := c.analyze(ctx, pageID); err != nil {
			return nil, err
		}
	}
	analyzed, err := c.analyzedBoxes(pageID)
	if err != nil {
		return nil, err
	}

	res, err := c.sn.Retrieve([]string{pageID})
	if err != nil {
		return nil, err
	}
	content := map[string]string{}
	for _, r := range regionsOf(res, pageID) {
		content[r.ID] = r.Content
	}
	return func(name string) (string, bool) {
		id := spec.ID{Kind: Input, Name: name}.String()
		if !analyzed[id] {
			return "", false
		}
		return content[id], true
	}, nil
}

// analyzedBoxes lists the template boxes this page has a stored fingerprint for.
// That is what distinguishes a box transcribed as empty (the user wrote nothing)
// from one that has never been looked at — the page JSON records a region per box
// analysis has seen, whether or not it held writing.
func (c *Client) analyzedBoxes(pageID string) (map[string]bool, error) {
	pd, err := c.sn.ReadPage(pageID)
	if err != nil {
		return nil, err
	}
	out := map[string]bool{}
	if pd.Analysis == nil {
		return out, nil
	}
	for _, r := range pd.Analysis.Regions {
		if r.SourceHash != "" {
			out[r.ID] = true
		}
	}
	return out, nil
}

// analyze runs snorg's analysis for one page, building the provider the archive's
// configuration describes.
func (c *Client) analyze(ctx context.Context, pageID string) error {
	prov, err := c.sn.NewProvider()
	if err != nil {
		return fmt.Errorf("analyze %s: %w", pageID, err)
	}
	res, err := c.sn.Analyze(ctx, prov, []string{pageID}, snorg.AnalyzeOptions{})
	if err != nil {
		return fmt.Errorf("analyze %s: %w", pageID, err)
	}
	for _, r := range res {
		if r.Err != nil {
			return fmt.Errorf("analyze %s: %w", pageID, r.Err)
		}
	}
	return nil
}

// Widget returns the named widget.
func (f *Form) Widget(name string) (Widget, bool) {
	for _, w := range f.Widgets {
		if w.Name == name {
			return w, true
		}
	}
	return Widget{}, false
}

// Values is the form's answers by widget name, in the Go type each kind produces
// (bool, string, float64, and a "YYYY-MM-DD" string for a date). A widget that is
// unset or failed maps to nil, so the set of keys is always the set of widgets.
func (f *Form) Values() map[string]any {
	out := make(map[string]any, len(f.Widgets))
	for _, w := range f.Widgets {
		out[w.Name] = w.Value()
	}
	return out
}

// Err reports the first widget that failed to read (ambiguous group, incomplete
// date, untranscribed input), or nil. Per-widget errors stay on the widgets, so a
// caller can read the rest of a form that has one bad answer.
func (f *Form) Err() error {
	for _, w := range f.Widgets {
		if w.Err != nil {
			return fmt.Errorf("%s: %w", w.Name, w.Err)
		}
	}
	return nil
}

// Errs returns every widget error on the form.
func (f *Form) Errs() []error {
	var out []error
	for _, w := range f.Widgets {
		if w.Err != nil {
			out = append(out, fmt.Errorf("%s: %w", w.Name, w.Err))
		}
	}
	return out
}

// get is the shared accessor behind the typed readers: it finds the widget, checks
// its kind, and refuses to answer for one that failed or was never filled in.
func (f *Form) get(name string, kind Kind) (Widget, error) {
	w, ok := f.Widget(name)
	if !ok {
		return w, fmt.Errorf("%s: no such widget on this form", name)
	}
	if w.Kind != kind && !(kind == Check && w.Kind == Button) {
		return w, fmt.Errorf("%s is a %s, not a %s", name, w.Kind, kind)
	}
	if w.Err != nil {
		return w, fmt.Errorf("%s: %w", name, w.Err)
	}
	if !w.Set {
		return w, fmt.Errorf("%s: not filled in", name)
	}
	return w, nil
}

// Bool reads a Check or Button widget. A tick box always has an answer, so this
// fails only when the widget is missing or of another kind.
func (f *Form) Bool(name string) (bool, error) {
	w, err := f.get(name, Check)
	return w.Bool, err
}

// Choice reads the ticked option of a Radio widget.
func (f *Form) Choice(name string) (string, error) {
	w, err := f.get(name, Radio)
	return w.Choice, err
}

// Number reads a Slider widget's value.
func (f *Form) Number(name string) (float64, error) {
	w, err := f.get(name, Slider)
	return w.Number, err
}

// Text reads an Input widget's transcription.
func (f *Form) Text(name string) (string, error) {
	w, err := f.get(name, Input)
	return w.Text, err
}

// Date reads a Date widget, as midnight UTC on the picked day.
func (f *Form) Date(name string) (time.Time, error) {
	w, err := f.get(name, Date)
	return w.Date, err
}

// MarshalJSON renders a form as the document the CLI prints and a front-end
// consumes: the page it came from, the widgets with their measurements, the flat
// name-to-answer map most consumers actually want, and any widget that could not
// be read.
func (f *Form) MarshalJSON() ([]byte, error) {
	type widgets = []Widget
	out := struct {
		PageID   string         `json:"page_id"`
		FileID   string         `json:"file_id,omitempty"`
		Template string         `json:"template,omitempty"`
		Values   map[string]any `json:"values"`
		Widgets  widgets        `json:"widgets"`
		Errors   []string       `json:"errors,omitempty"`
	}{PageID: f.PageID, FileID: f.FileID, Template: f.Template, Values: f.Values(), Widgets: f.Widgets}
	for _, err := range f.Errs() {
		out.Errors = append(out.Errors, err.Error())
	}
	return json.Marshal(out)
}

// DeclaredWidget is a widget as a template declares it — kind, name, caption and
// the rects its ink is measured in — before any page is read.
type DeclaredWidget = spec.Widget

// TemplateForm is one configured template seen as a form: the image that selects
// it, the widgets its boxes declare and anything wrong with them.
type TemplateForm struct {
	Image    string           `json:"image"`
	Name     string           `json:"name"`
	Hash     string           `json:"hash"`
	Widgets  []DeclaredWidget `json:"widgets"`
	Problems []string         `json:"problems,omitempty"`
}

// Validate checks template boxes as a form: it returns the widgets they declare
// and every problem found — a malformed id, a box whose analyze flag contradicts
// its kind, a rect off the page, two measured rects overlapping. A form with
// problems still reads; the point is to catch quiet mistakes, since overlapping
// cells answer for the same ink rather than failing.
func Validate(boxes []Box) ([]DeclaredWidget, []error) { return spec.Validate(boxes) }

// TemplateForms lists the configured templates that are snif forms — those whose boxes
// declare at least one widget — with their validation problems. A template with no
// widget ids is an ordinary snorg template and is skipped.
func (c *Client) TemplateForms() []TemplateForm {
	tmpls := c.sn.Templates()
	if tmpls == nil {
		return nil
	}
	var out []TemplateForm
	for _, t := range tmpls.List {
		boxes := make([]Box, 0, len(t.Boxes))
		for _, b := range t.Boxes {
			boxes = append(boxes, Box{
				ID: b.ID, Label: b.Label, Analyze: b.Analyze,
				Rect: Rect{X: b.Rect.X, Y: b.Rect.Y, W: b.Rect.W, H: b.Rect.H},
			})
		}
		widgets, problems := Validate(boxes)
		if len(widgets) == 0 && len(problems) == 0 {
			continue // a plain snorg template
		}
		f := TemplateForm{
			Image: t.Image, Hash: t.Hash, Widgets: widgets,
			Name: strings.TrimPrefix(strings.TrimSuffix(filepath.Base(t.Image), filepath.Ext(t.Image)), "snif-"),
		}
		for _, p := range problems {
			f.Problems = append(f.Problems, p.Error())
		}
		out = append(out, f)
	}
	return out
}
