package snif

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"

	"github.com/jdlugosz963/snif/internal/draw"
	"github.com/jdlugosz963/snif/internal/genconf"
	"github.com/jdlugosz963/snif/internal/layout"
)

// Generated is a rendered template: the image to import on the device and the
// snorg templates: section that describes it. Both are produced from one layout,
// so the rects in the YAML are exactly the rects in the picture.
type Generated struct {
	Name  string   // the template name
	Image string   // conventional basename of the PNG
	File  string   // conventional basename of the YAML
	PNG   []byte   // 1920x2560, 8-bit greyscale, no ancillary chunks
	YAML  []byte   // the templates: section, with its regeneration header
	Hash  string   // sha256 of PNG — the background_hash the device should report
	Argv  []string // the canonical `snif gen` arguments, recorded in the header

	Layout *Layout // every widget with its pixel rects
}

// Generate lays a template out, draws it and writes its config — the whole of the
// generator, available to a program and not only to the CLI.
//
// The returned YAML is a complete snorg config fragment: pass it to snorg with -c
// and the archive can match pages to this template and transcribe its Input boxes;
// pass it to snif and the same file is the form contract. Nothing else is needed,
// because every widget is encoded in its box id.
func Generate(t Template) (*Generated, error) {
	if t.Name == "" {
		return nil, fmt.Errorf("a template needs a name")
	}
	page, err := layout.Build(t)
	if err != nil {
		return nil, err
	}
	png, err := draw.Render(page)
	if err != nil {
		return nil, err
	}
	image, file := genconf.Name(t.Name)
	argv := genconf.Command(t)
	yaml, err := genconf.YAML(page, genconf.Header{
		Name: t.Name, Version: Version, Argv: argv, Image: image,
	})
	if err != nil {
		return nil, err
	}
	sum := sha256.Sum256(png)
	return &Generated{
		Name: t.Name, Image: image, File: file,
		PNG: png, YAML: yaml, Hash: hex.EncodeToString(sum[:]),
		Argv: argv, Layout: page,
	}, nil
}

// Write saves the pair into dir and returns the paths written. Existing files are
// refused unless force, so a regeneration cannot quietly discard a template a
// device is already using.
func (g *Generated) Write(dir string, force bool) ([]string, error) {
	if dir == "" {
		dir = "."
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	paths := []string{filepath.Join(dir, g.Image), filepath.Join(dir, g.File)}
	for _, p := range paths {
		if _, err := os.Stat(p); err == nil && !force {
			return nil, fmt.Errorf("%s exists (use --force to overwrite)", p)
		}
	}
	for i, data := range [][]byte{g.PNG, g.YAML} {
		if err := os.WriteFile(paths[i], data, 0o644); err != nil {
			return nil, err
		}
	}
	return paths, nil
}

// ParseGen reads a `snif gen` argument list — everything after "gen" — into the
// template it describes. It is the CLI's parser, exported so a program can accept
// the same one-line widget syntax its users already know:
//
//	args, err := snif.ParseGen([]string{"daily", "--check", "done:label=Done"})
//	g, err := snif.Generate(args.Template)
func ParseGen(argv []string) (*GenArgs, error) { return genconf.ParseArgs(argv) }

// GenCommand renders a template back into the canonical `snif gen` arguments that
// reproduce it — what Generate records in the YAML header.
func GenCommand(t Template) []string { return genconf.Command(t) }

// ReadGenCommand returns the arguments recorded in a generated YAML's header, so a
// template can be re-rendered from the file it produced (`snif gen -f`). It fails
// on a config snif did not write, which has no such header.
func ReadGenCommand(yaml []byte) ([]string, error) { return genconf.ReadCommand(yaml) }

// Regenerate re-renders the template a generated YAML describes, by re-running the
// command in its header. It is how an edited style, theme or size is applied: the
// header is the template's source, the image and the boxes are its output.
func Regenerate(yaml []byte) (*Generated, error) {
	argv, err := ReadGenCommand(yaml)
	if err != nil {
		return nil, err
	}
	args, err := ParseGen(argv)
	if err != nil {
		return nil, fmt.Errorf("header command: %w", err)
	}
	return Generate(args.Template)
}

// Summary is a one-line account of what a generated template holds, for a CLI to
// print: "5 widgets (1 date, 1 radio, …), 52 boxes". Regions are counted among
// the items, since a reader of the YAML sees them the same way.
func (g *Generated) Summary() string {
	return fmt.Sprintf("%d widgets (%s), %d boxes",
		len(g.Layout.Items()), genconf.Kinds(g.Layout), genconf.BoxCount(g.Layout))
}

// Boxes returns the template boxes the generated YAML declares — the contract a
// reader rebuilds the form from. Generate and ReadRender meet here: the boxes a
// template was drawn from are the boxes its pages are read against.
func (g *Generated) Boxes() []Box {
	var out []Box
	for _, it := range g.Layout.Items() {
		out = append(out, it.Boxes...)
	}
	return out
}
