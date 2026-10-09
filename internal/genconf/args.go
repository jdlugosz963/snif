// Package genconf is the authored form of a template: the command line that
// describes it and the YAML that records it.
//
// snif keeps no configuration of its own, so a generated template must carry
// everything needed to regenerate it. Two halves make that work: ParseArgs turns a
// `snif gen …` argument list into a layout.Template, and the YAML written beside
// the image quotes that exact argument list in its header. Re-rendering is then
// literally re-running what was run before (`snif gen -f <yaml>`), with no second
// grammar to keep in step — the styles and themes that only matter while drawing
// live in the command line and nowhere else.
package genconf

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/jdlugosz963/snif/internal/layout"
	"github.com/jdlugosz963/snif/internal/spec"
)

// Args is a parsed `snif gen` invocation.
type Args struct {
	Template layout.Template
	OutDir   string
	Force    bool
	From     string // the -f/--from path, when re-rendering an existing template
}

// Usage is the gen grammar, for command help and for the error a bad argument
// list produces.
const Usage = `snif gen <name> [options]

  --title TEXT            a heading drawn at the top of the page
  --theme THEME           page background: blank (default), dots, grid, lines
  -o, --out DIR           where to write snif-<name>.{png,yaml} (default: .)
  --force                 overwrite existing files
  -f, --from FILE         re-render a generated yaml (reruns the command in its header)

  Widget options, in the order they should appear down the page:

  --check   NAME[:ATTR…]              a tick box
  --button  NAME[:ATTR…]              a tick box that means "act on this page"
  --input   NAME[:ATTR…]              a handwriting area, transcribed by snorg
  --slider  NAME:MIN:MAX[:ATTR…]      a track read by the position of its mark
  --radio   NAME:OPT,OPT,…[:ATTR…]    one choice out of several
  --date    NAME[:YEAR0-YEAR1][:ATTR…] a day/month/year picker
  --figure  NAME[:ATTR…]              an area to draw in; never read, never
                                      transcribed — a plain snorg box whose rect
                                      a consumer crops the drawing out of

  ATTR is key=value, ':'-separated like the rest:
    label=TEXT     the caption drawn for the widget (default: none)
    labels=A,B,…   per-option labels for --radio (default: the option keys)
    style=THEME    background inside the widget's writing area
    w=PX h=PX      the widget body's size (an input field's dimensions)
    step=N         a slider's snap

  Example:
    snif gen daily --theme dots --title "Daily plan" \
      --date "due:2026-2030:label=Due date" \
      --radio "prio:low,mid,high:label=Priority" \
      --slider "mood:0:10:step=0.5:label=Mood" \
      --check "done:label=Done today" \
      --input "notes:style=lines:h=600:label=Notes"`

// kindOf maps a widget flag to its kind.
var kindOf = map[string]spec.Kind{
	"--check": spec.Check, "--button": spec.Button, "--input": spec.Input,
	"--slider": spec.Slider, "--radio": spec.Radio, "--date": spec.Date,
	"--figure": layout.FigureKind,
}

// ParseArgs reads a `snif gen` argument list (everything after "gen"). Widget
// flags are read in the order given, which is the order they are laid out, so the
// argument list alone fixes the page.
func ParseArgs(argv []string) (*Args, error) {
	a := &Args{OutDir: "."}
	need := func(i int, flag string) (string, error) {
		if i+1 >= len(argv) {
			return "", fmt.Errorf("%s needs a value", flag)
		}
		return argv[i+1], nil
	}
	for i := 0; i < len(argv); i++ {
		arg := argv[i]
		if kind, ok := kindOf[arg]; ok {
			v, err := need(i, arg)
			if err != nil {
				return nil, err
			}
			r, err := ParseWidget(kind, v)
			if err != nil {
				return nil, err
			}
			a.Template.Requests = append(a.Template.Requests, r)
			i++
			continue
		}
		switch arg {
		case "--title", "--theme", "-o", "--out", "-f", "--from":
			v, err := need(i, arg)
			if err != nil {
				return nil, err
			}
			i++
			switch arg {
			case "--title":
				a.Template.Title = v
			case "--theme":
				t, err := layout.ParseTheme(v)
				if err != nil {
					return nil, err
				}
				a.Template.Theme = t
			case "-o", "--out":
				a.OutDir = v
			case "-f", "--from":
				a.From = v
			}
		case "--force":
			a.Force = true
		default:
			if strings.HasPrefix(arg, "-") {
				return nil, fmt.Errorf("unknown option %q", arg)
			}
			if a.Template.Name != "" {
				return nil, fmt.Errorf("unexpected argument %q (the template name is already %q)", arg, a.Template.Name)
			}
			a.Template.Name = arg
		}
	}
	if a.From == "" {
		if a.Template.Name == "" {
			return nil, fmt.Errorf("a template name is required")
		}
		if len(a.Template.Requests) == 0 {
			return nil, fmt.Errorf("a template needs at least one widget")
		}
	}
	return a, nil
}

// ParseWidget reads one widget argument: a name, kind-specific positional values
// and key=value attributes, all ':'-separated. A token holding '=' is an
// attribute; anything else is positional.
func ParseWidget(kind spec.Kind, arg string) (layout.Request, error) {
	r := layout.Request{Kind: kind}
	tok := splitEsc(arg, ':')
	if tok[0] == "" {
		return r, fmt.Errorf("%s: a widget needs a name", kind)
	}
	r.Name = tok[0]

	var pos []string
	for _, t := range tok[1:] {
		k, v, isAttr := strings.Cut(t, "=")
		if !isAttr {
			pos = append(pos, t)
			continue
		}
		if err := attr(&r, k, v); err != nil {
			return r, fmt.Errorf("%s %q: %w", kind, r.Name, err)
		}
	}

	switch kind {
	case spec.Check, spec.Button, spec.Input, layout.FigureKind:
		if len(pos) > 0 {
			return r, fmt.Errorf("%s %q: takes no values, got %q", kind, r.Name, strings.Join(pos, ":"))
		}
	case spec.Slider:
		if len(pos) != 2 {
			return r, fmt.Errorf("slider %q: needs min and max (--slider %s:MIN:MAX)", r.Name, r.Name)
		}
		var err error
		if r.Min, err = strconv.ParseFloat(pos[0], 64); err != nil {
			return r, fmt.Errorf("slider %q: min %q is not a number", r.Name, pos[0])
		}
		if r.Max, err = strconv.ParseFloat(pos[1], 64); err != nil {
			return r, fmt.Errorf("slider %q: max %q is not a number", r.Name, pos[1])
		}
	case spec.Radio:
		if len(pos) != 1 || pos[0] == "" {
			return r, fmt.Errorf("radio %q: needs its options (--radio %s:low,mid,high)", r.Name, r.Name)
		}
		r.Options = splitEsc(pos[0], ',')
	case spec.Date:
		r.Year0, r.Year1 = time.Now().Year(), time.Now().Year()+4
		if len(pos) > 1 {
			return r, fmt.Errorf("date %q: takes one year range (--date %s:2026-2030)", r.Name, r.Name)
		}
		if len(pos) == 1 {
			y0, y1, err := yearRange(pos[0])
			if err != nil {
				return r, fmt.Errorf("date %q: %w", r.Name, err)
			}
			r.Year0, r.Year1 = y0, y1
		}
	}
	if len(r.Labels) > 0 && len(r.Labels) != len(r.Options) {
		return r, fmt.Errorf("radio %q: %d labels for %d options", r.Name, len(r.Labels), len(r.Options))
	}
	return r, nil
}

func attr(r *layout.Request, k, v string) error {
	switch k {
	case "label":
		r.Label = v
	case "labels":
		r.Labels = splitEsc(v, ',')
	case "style":
		t, err := layout.ParseTheme(v)
		if err != nil {
			return err
		}
		r.Style = t
	case "w", "h":
		n, err := strconv.Atoi(v)
		if err != nil || n <= 0 {
			return fmt.Errorf("%s=%q must be a positive number of pixels", k, v)
		}
		if k == "w" {
			r.W = n
		} else {
			r.H = n
		}
	case "step":
		n, err := strconv.ParseFloat(v, 64)
		if err != nil || n <= 0 {
			return fmt.Errorf("step=%q must be a positive number", v)
		}
		r.Step = n
	default:
		return fmt.Errorf("unknown attribute %q", k)
	}
	return nil
}

func yearRange(s string) (int, int, error) {
	a, b, ok := strings.Cut(s, "-")
	if !ok {
		return 0, 0, fmt.Errorf("year range %q is not YEAR0-YEAR1", s)
	}
	y0, err0 := strconv.Atoi(a)
	y1, err1 := strconv.Atoi(b)
	if err0 != nil || err1 != nil || y0 < 1000 || y1 > 9999 || y1 < y0 {
		return 0, 0, fmt.Errorf("year range %q is not YEAR0-YEAR1", s)
	}
	return y0, y1, nil
}

// splitEsc splits on sep, honouring a backslash escape so a caption may hold the
// separator itself (label=Due\: date). The backslash is dropped from the result,
// and Command writes it back — so a label survives the round-trip through a
// generated file's header.
func splitEsc(s string, sep byte) []string {
	var out []string
	var cur strings.Builder
	for i := 0; i < len(s); i++ {
		switch {
		case s[i] == '\\' && i+1 < len(s):
			i++
			cur.WriteByte(s[i])
		case s[i] == sep:
			out = append(out, cur.String())
			cur.Reset()
		default:
			cur.WriteByte(s[i])
		}
	}
	return append(out, cur.String())
}

// escape is splitEsc's inverse for one token.
func escape(s string, seps ...byte) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == '\\' {
			b.WriteString(`\\`)
			continue
		}
		for _, sep := range seps {
			if s[i] == sep {
				b.WriteByte('\\')
				break
			}
		}
		b.WriteByte(s[i])
	}
	return b.String()
}
