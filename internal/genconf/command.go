package genconf

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/jdlugosz963/snif/internal/layout"
	"github.com/jdlugosz963/snif/internal/spec"
)

// Command renders a template back into the `snif gen` argument list that produces
// it — the canonical form, which is what a generated file's header records.
//
// Recording the canonical rendering rather than the characters the user typed is
// deliberate: it is deterministic (so regenerating an unchanged template rewrites
// nothing), it is complete (a template built by a library caller has a header too,
// not only one typed at a shell), and it is verifiable — ParseArgs(Command(t))
// must equal t, which is a test rather than a hope.
func Command(t layout.Template) []string {
	argv := []string{t.Name}
	if t.Title != "" {
		argv = append(argv, "--title", t.Title)
	}
	if t.Theme != "" && t.Theme != layout.Blank {
		argv = append(argv, "--theme", string(t.Theme))
	}
	for _, r := range t.Requests {
		argv = append(argv, "--"+string(r.Kind), widgetArg(r))
	}
	return argv
}

// widgetArg renders one widget request: name, its kind's positional values, then
// its attributes in a fixed order so the output is stable.
func widgetArg(r layout.Request) string {
	tok := []string{escape(r.Name, ':')}

	switch r.Kind {
	case spec.Slider:
		tok = append(tok, num(r.Min), num(r.Max))
	case spec.Radio:
		var opts []string
		for _, o := range r.Options {
			opts = append(opts, escape(o, ':', ','))
		}
		tok = append(tok, strings.Join(opts, ","))
	case spec.Date:
		tok = append(tok, fmt.Sprintf("%d-%d", r.Year0, r.Year1))
	}

	if r.Label != "" {
		tok = append(tok, "label="+escape(r.Label, ':'))
	}
	if len(r.Labels) > 0 {
		var ls []string
		for _, l := range r.Labels {
			ls = append(ls, escape(l, ':', ','))
		}
		tok = append(tok, "labels="+strings.Join(ls, ","))
	}
	if r.Style != "" && r.Style != layout.Blank {
		tok = append(tok, "style="+string(r.Style))
	}
	if r.W > 0 {
		tok = append(tok, "w="+strconv.Itoa(r.W))
	}
	if r.H > 0 {
		tok = append(tok, "h="+strconv.Itoa(r.H))
	}
	if r.Step > 0 {
		tok = append(tok, "step="+num(r.Step))
	}
	return strings.Join(tok, ":")
}

func num(v float64) string { return strconv.FormatFloat(v, 'g', -1, 64) }
