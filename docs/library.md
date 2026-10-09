# Library

```go
import "github.com/jdlugosz963/snif/pkg/snif"
```

Everything the CLI does is here — generation included.

## Generating

```go
args, _ := snif.ParseGen([]string{"daily", "--check", "done:label=Done", "--input", "notes:h=600"})
g, err := snif.Generate(args.Template)      // or build a snif.Template by hand
paths, err := g.Write("templates", false)   // snif-daily.png + snif-daily.yaml
```

`Generate` returns the PNG, the YAML, the sha256 the device should report, the
canonical argument list recorded in the header, and the `Layout` with every widget's
rects. `ParseGen` is the CLI's own parser, exported so a program can accept the same
one-line widget syntax its users already know; `Regenerate(yaml)` re-runs the
command in a generated file's header.

A `Template` built in Go needs no argument list — `GenCommand` renders one, and
that is what the header stores, so a programmatic template is as reproducible as a
typed one.

## Configuration

snif has none of its own: `Resolve` is `snorg.Resolve`, so a program layers exactly
what the two CLIs layer, from the same files. The XDG user config
(`~/.config/snorg/config.yaml`) is the only file loaded on its own; everything else
it reaches with `include:`, recursively. Chain it to the archive once —

```yaml
# ~/.config/snorg/config.yaml
archive: ~/archive
include: [~/archive/config.yaml]     # provider:, and the templates: chain
```

```yaml
# ~/archive/config.yaml
include: [templates/snif-daily.yaml]
```

— and the archive, the provider credentials and the `templates:` section all arrive
on their own. `ResolveOptions` mirrors the CLI globals for overriding that:
`ArchivePath` (`-a`), `ConfigFiles` (`-c`, later wins), `NoUserConfig`.

Only one `templates:` list survives the merge — a later layer replaces it whole
rather than appending — so every template must be declared in a **single** file.
Two generated YAMLs in one `include:` leave only the last one configured.

## Reading

```go
c, err := snif.Resolve(snif.ResolveOptions{})   // archive and forms from the config chain
form, err := c.Form(ctx, pageID, snif.ReadOptions{Analyze: true})

due, err := form.Date("due")      // time.Time
prio, err := form.Choice("prio")  // string
mood, err := form.Number("mood")  // float64
done, err := form.Bool("done")    // bool
notes, err := form.Text("notes")  // string
values := form.Values()           // map[string]any, ready for JSON
```

or straight into a struct:

```go
var v struct {
    Due   time.Time `snif:"due"`
    Prio  string    `snif:"prio"`
    Mood  float64   `snif:"mood"`
    Done  bool      `snif:"done"`
    Notes string    `snif:"notes,required"`
}
err := form.Decode(&v)
```

A field matches a widget by its `snif` tag, or by its lower-cased name; `snif:"-"`
skips it. An unanswered widget leaves the field at its zero value — a form may be
partly filled in — unless the tag says `,required`. A widget that *failed* to read
is always an error: the page is contradictory, not merely blank.

`Forms(ctx, ids, opts)` reads a batch, preserving order; a page that fails lands in
its `Result.Err` without aborting the rest. Per-widget failures stay on the widget
(`Widget.Err`), so one ambiguous group never hides the other answers —
`Form.Err()`/`Errs()` collect them.

## Without an archive

```go
form, err := snif.ReadRender(g.Boxes(), pageRenderPNG, map[string]string{"input:notes": "…"}, snif.ReadOptions{})
```

`ReadRender` is the core `Client.Form` wraps: template boxes plus a canonical page
render (handwriting on white, no background — snorg's `RenderRegion` with
`styled=false`). It is what a program with its own pipeline — or a test that paints
ink into a blank page — reads a form with.

## Validating

```go
widgets, problems := snif.Validate(boxes)   // one config's boxes
forms, err := c.TemplateForms()             // every configured template, with problems
```

## Reaching snorg

`c.Snorg()` returns the underlying `*snorg.Client`, so a program holding a snif
client can query, retrieve, tag and export the archive without building a second
one.
