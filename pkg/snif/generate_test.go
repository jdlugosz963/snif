package snif_test

import (
	"bytes"
	"encoding/binary"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jdlugosz963/snif/pkg/snif"
)

var update = flag.Bool("update", false, "rewrite the golden files")

// TestPNGShape pins the encoding of a generated template against the form the
// device itself uses (see docs/templates.md): 1920x2560, 8-bit greyscale,
// non-interlaced, and no ancillary chunks at all. It matters beyond tidiness —
// the template is selected by the sha256 of these bytes, so a stray pHYs or a trip
// through RGBA would change what the archive has to match.
func TestPNGShape(t *testing.T) {
	g := template(t)

	if !bytes.HasPrefix(g.PNG, []byte("\x89PNG\r\n\x1a\n")) {
		t.Fatal("not a PNG")
	}
	var chunks []string
	var ihdr []byte
	for i := 8; i+8 <= len(g.PNG); {
		n := int(binary.BigEndian.Uint32(g.PNG[i:]))
		typ := string(g.PNG[i+4 : i+8])
		chunks = append(chunks, typ)
		if typ == "IHDR" {
			ihdr = g.PNG[i+8 : i+8+n]
		}
		i += 12 + n
	}
	for _, c := range chunks {
		if c != "IHDR" && c != "IDAT" && c != "IEND" {
			t.Errorf("unexpected chunk %q: a template must carry no metadata", c)
		}
	}
	if len(chunks) < 3 || chunks[0] != "IHDR" || chunks[len(chunks)-1] != "IEND" {
		t.Errorf("chunks = %v", chunks)
	}
	w := binary.BigEndian.Uint32(ihdr[0:])
	h := binary.BigEndian.Uint32(ihdr[4:])
	depth, colorType, interlace := ihdr[8], ihdr[9], ihdr[12]
	if w != snif.PageW || h != snif.PageH {
		t.Errorf("size = %dx%d, want %dx%d", w, h, snif.PageW, snif.PageH)
	}
	if depth != 8 || colorType != 0 {
		t.Errorf("bit depth %d colour type %d, want 8 and 0 (greyscale)", depth, colorType)
	}
	if interlace != 0 {
		t.Error("a template must not be interlaced")
	}
}

// TestDeterministic: generating the same template twice produces the same bytes,
// including no timestamp in the header — so a template under version control does
// not churn, and the recorded hash stays the one the device was given.
func TestDeterministic(t *testing.T) {
	a, b := template(t), template(t)
	if !bytes.Equal(a.PNG, b.PNG) || !bytes.Equal(a.YAML, b.YAML) {
		t.Fatal("two runs differ")
	}
	if a.Hash != b.Hash {
		t.Errorf("hash %s != %s", a.Hash, b.Hash)
	}
	if bytes.Contains(a.YAML, []byte("20")) && bytes.Contains(a.YAML, []byte("generated on")) {
		t.Error("the header must carry no timestamp")
	}
}

// TestRegenerate: the command in a generated file's header reproduces the file
// exactly. That is what makes the header the template's source — styles, themes
// and sizes need no home in the config because re-rendering re-runs the command.
func TestRegenerate(t *testing.T) {
	g := template(t)
	again, err := snif.Regenerate(g.YAML)
	if err != nil {
		t.Fatalf("Regenerate: %v", err)
	}
	if !bytes.Equal(g.PNG, again.PNG) {
		t.Error("regenerated image differs")
	}
	if !bytes.Equal(g.YAML, again.YAML) {
		t.Errorf("regenerated config differs:\n%s", again.YAML)
	}
	if _, err := snif.Regenerate([]byte("templates:\n  - image: x.png\n")); err == nil {
		t.Error("a config snif did not write has no header command and must be rejected")
	}
}

// TestGolden pins the generated config, so a change to the id grammar, the layout
// or the header is visible in a diff rather than silent.
func TestGolden(t *testing.T) {
	g := template(t)
	path := filepath.Join("testdata", "snif-daily.yaml")
	if *update {
		if err := os.MkdirAll("testdata", 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, g.YAML, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("%v (run go test -update to create it)", err)
	}
	if !bytes.Equal(g.YAML, want) {
		t.Errorf("generated config differs from %s:\n--- got ---\n%s", path, g.YAML)
	}
}

// TestWriteRefusesOverwrite: regenerating must not quietly replace a template a
// device is already using.
func TestWriteRefusesOverwrite(t *testing.T) {
	g := template(t)
	dir := t.TempDir()
	if _, err := g.Write(dir, false); err != nil {
		t.Fatal(err)
	}
	if _, err := g.Write(dir, false); err == nil {
		t.Fatal("a second write must refuse without --force")
	}
	if _, err := g.Write(dir, true); err != nil {
		t.Fatalf("--force must overwrite: %v", err)
	}
	for _, name := range []string{g.Image, g.File} {
		if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
			t.Errorf("%s not written: %v", name, err)
		}
	}
}

// TestGenErrors: the argument mistakes a user actually makes are reported, with
// the widget named.
func TestGenErrors(t *testing.T) {
	cases := map[string][]string{
		"no name":            {"--check", "done"},
		"no widgets":         {"daily"},
		"unknown option":     {"daily", "--checkbox", "done"},
		"unknown attribute":  {"daily", "--check", "done:colour=red"},
		"unknown theme":      {"daily", "--theme", "paisley", "--check", "done"},
		"slider without max": {"daily", "--slider", "mood:0"},
		"radio without opts": {"daily", "--radio", "prio"},
		"bad size":           {"daily", "--input", "notes:h=-5"},
		"duplicate name":     {"daily", "--check", "done", "--input", "done"},
		"bad name":           {"daily", "--check", "the done flag"},
	}
	for name, argv := range cases {
		args, err := snif.ParseGen(argv)
		if err == nil {
			_, err = snif.Generate(args.Template)
		}
		if err == nil {
			t.Errorf("%s: accepted %v, want an error", name, argv)
		}
	}
}

// TestOverflow: a template that runs off the page is refused, naming the widget
// that did not fit — a form silently cut in half would read as one with missing
// fields.
func TestOverflow(t *testing.T) {
	argv := []string{"big"}
	for _, n := range []string{"a", "b", "c", "d", "e", "f"} {
		argv = append(argv, "--input", n+":h=500:label="+n)
	}
	args, err := snif.ParseGen(argv)
	if err != nil {
		t.Fatal(err)
	}
	_, err = snif.Generate(args.Template)
	if err == nil || !strings.Contains(err.Error(), "does not fit") {
		t.Fatalf("err = %v, want a does-not-fit error", err)
	}
}

// TestValidateCatchesHandEdits: the checks `snif check` runs over a config someone
// edited by hand — an overlapping rect reads another widget's ink, and a wrong
// analyze flag spends (or withholds) a vision call.
func TestValidateCatchesHandEdits(t *testing.T) {
	g := template(t)
	boxes := g.Boxes()

	if _, problems := snif.Validate(boxes); len(problems) > 0 {
		t.Fatalf("a generated template must be clean: %v", problems)
	}

	moved := append([]snif.Box(nil), boxes...)
	for i, b := range moved {
		if b.ID == "radio:prio:high" {
			moved[i].Rect = mustBox(t, boxes, "radio:prio:low").Rect
		}
	}
	if _, problems := snif.Validate(moved); len(problems) == 0 {
		t.Error("two cells on the same rect must be reported")
	}

	flagged := append([]snif.Box(nil), boxes...)
	for i, b := range flagged {
		if b.ID == "check:done" {
			flagged[i].Analyze = true
		}
	}
	if _, problems := snif.Validate(flagged); len(problems) == 0 {
		t.Error("analyze: true on a tick box must be reported")
	}
}

func mustBox(t *testing.T, boxes []snif.Box, id string) snif.Box {
	t.Helper()
	for _, b := range boxes {
		if b.ID == id {
			return b
		}
	}
	t.Fatalf("no box %q", id)
	return snif.Box{}
}

// TestFigureRegion: a --figure area reaches the config as a plain snorg box —
// never analyzed, and invisible to the form model. That is the whole contract a
// consumer cropping a drawing out of the page depends on: the rect is published,
// the ink inside it is nobody's answer.
func TestFigureRegion(t *testing.T) {
	args, err := snif.ParseGen([]string{"fig",
		"--input", "main.name:label=Name:h=140",
		"--figure", "main:label=Figure:h=1300",
		"--input", "main.notes:label=Notes:style=lines:h=500",
	})
	if err != nil {
		t.Fatal(err)
	}
	g, err := snif.Generate(args.Template)
	if err != nil {
		t.Fatal(err)
	}
	var ids []string
	for _, b := range g.Boxes() {
		ids = append(ids, b.ID)
		if b.ID == "figure:main" {
			if b.Analyze {
				t.Error("figure:main is analyze: true — a drawing is not text")
			}
			if b.Rect.H != 1300 || b.Rect.W != 1760 {
				t.Errorf("figure:main rect = %+v", b.Rect)
			}
		}
	}
	// Boxes come out in page order, so the YAML reads top to bottom.
	want := "input:main.name figure:main input:main.notes"
	if got := strings.Join(ids, " "); got != want {
		t.Errorf("boxes = %q, want %q", got, want)
	}
	// Validate is the check `snif check` runs: a figure box must raise nothing,
	// since it declares no widget at all.
	if _, problems := snif.Validate(g.Boxes()); len(problems) > 0 {
		t.Errorf("Validate: %v", problems)
	}
}
