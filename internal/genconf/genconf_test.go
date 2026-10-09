package genconf

import (
	"reflect"
	"strings"
	"testing"

	"github.com/jdlugosz963/snif/internal/layout"
	"github.com/jdlugosz963/snif/internal/spec"
)

// full exercises every kind, every attribute and the separators that need
// escaping.
func full() layout.Template {
	return layout.Template{
		Name:  "daily",
		Title: "Daily plan",
		Theme: layout.Dots,
		Requests: []layout.Request{
			{Kind: spec.Date, Name: "due", Label: "Due: date", Year0: 2026, Year1: 2030},
			{Kind: spec.Radio, Name: "prio", Label: "Priority",
				Options: []string{"low", "mid", "high"}, Labels: []string{"Low", "Middling", "High"}},
			{Kind: spec.Slider, Name: "mood", Label: "Mood", Min: -5, Max: 10.5, Step: 0.5},
			{Kind: spec.Check, Name: "done", Label: "Done today"},
			{Kind: spec.Button, Name: "submit"},
			{Kind: spec.Input, Name: "notes", Label: "Notes", Style: layout.Lines, W: 1400, H: 520},
			{Kind: layout.FigureKind, Name: "sketch", Label: "Sketch", H: 300},
		},
	}
}

// TestCommandRoundTrip: the canonical command a template renders to parses back
// into the same template. It is what makes a generated file's header a faithful
// source rather than a comment — including a caption holding the ':' that
// separates the syntax's own tokens.
func TestCommandRoundTrip(t *testing.T) {
	want := full()
	argv := Command(want)
	args, err := ParseArgs(argv)
	if err != nil {
		t.Fatalf("ParseArgs(%v): %v", argv, err)
	}
	if !reflect.DeepEqual(args.Template, want) {
		t.Errorf("round trip changed the template:\n got %+v\nwant %+v", args.Template, want)
	}
}

// TestHeaderRoundTrip: the command survives being written into a YAML header,
// shell-quoted, and read back.
func TestHeaderRoundTrip(t *testing.T) {
	tmpl := full()
	page, err := layout.Build(tmpl)
	if err != nil {
		t.Fatal(err)
	}
	argv := Command(tmpl)
	image, _ := Name(tmpl.Name)
	data, err := YAML(page, Header{Name: tmpl.Name, Version: "v0", Argv: argv, Image: image})
	if err != nil {
		t.Fatal(err)
	}
	got, err := ReadCommand(data)
	if err != nil {
		t.Fatalf("ReadCommand: %v", err)
	}
	if !reflect.DeepEqual(got, argv) {
		t.Errorf("header round trip:\n got %q\nwant %q", got, argv)
	}
	if !strings.Contains(string(data), `id: "date:due:2026:2030"`) {
		t.Error("ids holding ':' must be quoted in the emitted YAML")
	}
	if _, err := ReadCommand([]byte("templates: []\n")); err == nil {
		t.Error("a config without the header must be rejected")
	}
}

func TestShellQuoting(t *testing.T) {
	cases := [][]string{
		{"daily"},
		{"a b", "c'd", `e"f`, "g\\h", "--check", "done:label=Done, today"},
		{""},
	}
	for _, argv := range cases {
		got, err := Split(Quote(argv))
		if err != nil {
			t.Fatalf("Split(Quote(%q)): %v", argv, err)
		}
		if !reflect.DeepEqual(got[2:], argv) { // Quote prefixes "snif gen"
			t.Errorf("round trip %q → %q", argv, got[2:])
		}
	}
	if _, err := Split(`a 'b`); err == nil {
		t.Error("an unterminated quote must be an error, not a guess")
	}
}

// TestDateDefaultYears: a date with no range still gets one, so the commonest
// invocation needs no arithmetic from the user.
func TestDateDefaultYears(t *testing.T) {
	r, err := ParseWidget(spec.Date, "due")
	if err != nil {
		t.Fatal(err)
	}
	if r.Year1-r.Year0 != 4 || r.Year0 < 2000 {
		t.Errorf("default range %d..%d", r.Year0, r.Year1)
	}
}
