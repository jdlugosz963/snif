# snif — supernote interface

Use Supernote pages as user interfaces.

snif generates a **form template** — a 1920×2560 greyscale PNG to import on the
device, plus the [snorg](https://github.com/jdlugosz963/snorg) `templates:` config
that describes it — and reads a page drawn on that template back as **typed
values**: a bool, a choice, a date, a number, a string.

Everything but free text is read from *where the ink is*. A pen tablet's page has
no noise, so a tick box either holds ink or it does not, and a slider's value is
simply its rightmost inked column — no model, no credentials, no cost. Only an
`input` field goes to snorg's vision model.

```sh
snif gen daily -o ~/archive/templates \
  --title "Daily plan" --theme dots \
  --date   "due:2026-2030:label=Due date" \
  --radio  "prio:low,mid,high:label=Priority" \
  --slider "mood:0:10:step=0.5:label=Mood" \
  --check  "done:label=Done today" \
  --input  "notes:style=lines:h=520:label=Notes"
```

Import `snif-daily.png` on the device, write on a page, export and ingest it, then:

```sh
snorg query templated | snif read --values
```

```json
[{"page_id":"P2026…","values":{"due":"2026-09-14","prio":"mid","mood":7,
  "done":true,"notes":"call the dentist"}}]
```

## How a page is recognized

snif keeps **no configuration of its own**. Every widget is encoded in the id of a
snorg template box:

```yaml
templates:
  - image: snif-daily.png
    boxes:
      - {id: "radio:prio",      label: "Priority", rect: {…}, analyze: false}
      - {id: "radio:prio:low",  label: "low",      rect: {…}, analyze: false}
      - {id: "slider:mood:0:10:0.5", label: "Mood", rect: {…}, analyze: false}
      - {id: "input:notes",     label: "Notes",    rect: {…}, analyze: true}
```

So one file is both tools' whole contract, and a page is a snif form iff snorg
matches its background to a template whose boxes carry widget ids — never because
of a filename. `input` is the only `analyze: true` box.

## Configuration

There is no snif config: it layers snorg's, from the same files. Only
`~/.config/snorg/config.yaml` is loaded on its own, and it reaches the rest with
`include:` — which is why the commands above need no `-a` and no `-c`:

```yaml
# ~/.config/snorg/config.yaml
archive: ~/archive
include: [~/archive/config.yaml]     # provider:, and include: the generated YAMLs
```

## As a library

```go
form, _ := c.Form(ctx, pageID, snif.ReadOptions{Analyze: true})
var v struct {
    Due   time.Time `snif:"due"`
    Prio  string    `snif:"prio"`
    Notes string    `snif:"notes,required"`
}
err := form.Decode(&v)
```

Generation is public too (`snif.Generate`), so a program can build templates as
well as read them.

## Docs

- [principles.md](docs/principles.md) — what snif is and the rules it is built by
- [widgets.md](docs/widgets.md) — the box-id grammar and what each widget measures
- [templates.md](docs/templates.md) — generating, importing, and `snif bind`
- [cli.md](docs/cli.md) — the commands
- [library.md](docs/library.md) — `pkg/snif`

## Build

```sh
go build ./... && go vet ./... && gofmt -l . && go test ./...
```
