// Command daily is a worked example of building an interface on Supernote pages:
// it generates a form template, then reads the pages drawn on it into a Go struct.
//
//	go run ./examples/daily gen
//	go run ./examples/daily read
//
// Neither takes an argument. Configuration is layered exactly as snorg layers it,
// from the same files: the XDG user config is the one auto-loaded layer, and it
// reaches everything else with include:.
//
//	# ~/.config/snorg/config.yaml — the only file loaded on its own
//	archive: ~/notes/sn
//	include: [~/notes/sn/config.yaml]     # provider: and the templates: chain
//
//	# ~/notes/sn/config.yaml
//	include: [snif-daily.yaml]            # the generated templates: section
//
// Everything here is pkg/snif; there is nothing the CLI can do that a program
// cannot.
package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/jdlugosz963/snif/pkg/snif"
	snorg "github.com/jdlugosz963/snorg/pkg/snorg"
)

// Entry is one day's answers — the shape the form decodes into. A field is
// matched to a widget by its snif tag; ",required" makes an unanswered widget an
// error instead of a zero value.
type Entry struct {
	Due   time.Time `snif:"due"`
	Prio  string    `snif:"prio"`
	Mood  float64   `snif:"mood"`
	Done  bool      `snif:"done"`
	File  bool      `snif:"submit"`
	Notes string    `snif:"notes,required"`
}

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: daily gen | daily read")
		os.Exit(2)
	}
	var err error
	switch os.Args[1] {
	case "gen":
		err = generate()
	case "read":
		err = read()
	default:
		err = fmt.Errorf("unknown command %q", os.Args[1])
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "daily:", err)
		os.Exit(1)
	}
}

// generate builds the template. The widget list is written in Go here, but
// snif.ParseGen would accept the same one-line syntax the CLI takes — either way
// the YAML records a canonical command, so the template stays reproducible.
func generate() error {
	c, err := snif.Resolve(snif.ResolveOptions{})
	if err != nil {
		return err
	}
	dir := c.ArchivePath() // beside the archive config that will include it

	year := time.Now().Year()
	g, err := snif.Generate(snif.Template{
		Name:  "daily",
		Title: "Daily plan",
		Theme: snif.Dots,
		Requests: []snif.WidgetSpec{
			{Kind: snif.Date, Name: "due", Label: "Due date", Year0: year, Year1: year + 4},
			{Kind: snif.Radio, Name: "prio", Label: "Priority", Options: []string{"low", "mid", "high"}},
			{Kind: snif.Slider, Name: "mood", Label: "Mood", Min: 0, Max: 10, Step: 0.5},
			{Kind: snif.Check, Name: "done", Label: "Done today"},
			{Kind: snif.Button, Name: "submit", Label: "File this page"},
			{Kind: snif.Input, Name: "notes", Label: "Notes", Style: snif.Lines, H: 520},
		},
	})
	if err != nil {
		return err
	}
	paths, err := g.Write(dir, true)
	if err != nil {
		return err
	}
	for _, p := range paths {
		fmt.Println(p)
	}
	fmt.Fprintf(os.Stderr, "%s — import %s on the device; if its hash changes, run `snif bind`\n",
		g.Summary(), filepath.Join(dir, g.Image))
	fmt.Fprintf(os.Stderr, "add to %s:  include: [%s]\n", filepath.Join(dir, "config.yaml"), g.File)
	return nil
}

// read finds every page drawn on a configured template and decodes it. Only the
// notes field costs anything: the date, choice, slider and tick boxes are read
// from ink.
func read() error {
	// No options: the archive, the provider and the templates: section all come
	// from the config chain, the way they do for a bare `snorg` command.
	c, err := snif.Resolve(snif.ResolveOptions{})
	if err != nil {
		return err
	}

	// snif's client carries the snorg one, so a program needs only the one handle:
	// query the archive for templated pages, then read each as a form.
	matches, err := c.Snorg().Query(snorg.MatchTemplated)
	if err != nil {
		return err
	}
	ids := make([]string, 0, len(matches))
	for _, m := range matches {
		ids = append(ids, m.PageID)
	}

	for _, r := range c.Forms(context.Background(), ids, snif.ReadOptions{Analyze: true}) {
		if r.Err != nil {
			fmt.Fprintf(os.Stderr, "%s: %v\n", r.PageID, r.Err)
			continue
		}
		var e Entry
		if err := r.Form.Decode(&e); err != nil {
			fmt.Fprintf(os.Stderr, "%s: %v\n", r.PageID, err)
			continue
		}
		fmt.Printf("%s  %s  prio=%s mood=%.1f done=%t  %s\n",
			r.PageID, e.Due.Format("2006-01-02"), e.Prio, e.Mood, e.Done, e.Notes)
		if e.File {
			fmt.Fprintf(os.Stderr, "%s asked to be filed\n", r.PageID)
		}
	}
	return nil
}
