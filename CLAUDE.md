# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code
in this repository.

## What this is

SNIF (supernote interface) turns Supernote pages into forms: it generates a
template (a greyscale PNG plus the snorg `templates:` config describing it) and
reads pages drawn on it back as typed values. Read `docs/principles.md` (project
rules), `docs/widgets.md` (the box-id grammar — the core contract) and
`docs/templates.md` (generation, device import, `bind`) before substantial work.
snif is built on snorg; `../snorg/docs/{templates,architecture,api}.md` are the
backend's contracts.

## Project rules

- Go. External deps: `internal/draw` uses `golang.org/x/image` (font rasterization
  via `opentype` + the embedded Go fonts); `cmd/snif` uses `urfave/cli/v3`;
  everything else is the standard library plus `pkg/snorg`. `internal/spec`,
  `internal/ink`, `internal/form`, `internal/layout` and `internal/genconf`
  deliberately depend on nothing outside the module, so the grammar, the ink rules
  and the layout are testable standalone.
- **snorg is the only backend.** snif never parses `.note`, never writes archive
  files and never rasterizes SVG. It calls `pkg/snorg` for the page render, the
  template boxes, the transcription and (on demand) the analysis.
- **snif has no config of its own.** A widget lives in the id of a snorg template
  box; captions live in snorg's `label`, geometry in its `rect`. Anything that
  matters only while drawing (themes, styles) lives in the `# command:` header of
  the generated YAML, which `snif gen -f` re-runs. Never add a parallel config
  section.
- No backward compatibility: when something must change, rewrite it cleanly.
- All operational data is plaintext; docs in English, maximally concise.

## Commands

- `go build ./... && go vet ./... && gofmt -l .` — build, vet, format check (gofmt
  output must be empty)
- `go test ./...` — all tests; `go test ./pkg/snif -update` rewrites the golden
  generated config in `pkg/snif/testdata/`
- `go run ./cmd/snif gen <name> [widget flags]` — generate a template (needs no
  archive); `snif gen --help` prints the widget syntax
- `go run ./cmd/snif -a <archive> -c <form.yaml> read [PAGEID ...]` — read pages as
  JSON; PAGEIDs from args or stdin
- `go run ./cmd/snif -a <archive> -c <form.yaml> check [PAGEID ...]` — validate the
  configured forms, or show how pages read
- Local development against an unreleased snorg: `go mod edit -replace
  github.com/jdlugosz963/snorg=../snorg` (drop it before committing)

## Architecture

The two halves meet at `internal/spec`, and the round-trip
`Build(Widget.Boxes()) == identity` is what keeps them from drifting — the
generator cannot emit an id the reader cannot read. `TestRoundTrip` in
`pkg/snif/roundtrip_test.go` is the headline check: generate a template, paint ink
into the cells its own layout declares, read the values back.

- `internal/spec` — the box-id grammar (`Parse`/`ID.String`), the widget model
  (`Build`/`Widget.Boxes`), `Rect`, and `Validate` (the checks `snif check` runs).
  Depends on nothing.
- `internal/ink` — a page render thresholded to a 1-bit mask, with the only two
  measurements snif needs: `Fill(rect)` and `Rightmost(rect)`. The threshold is
  snorg's own (darker than mid-grey), so "holds ink" means the same in both
  projects.
- `internal/form` — resolution: widgets + mask + a text lookup → typed values,
  with the sparse `Widget` result and the `AmbiguousError`/`IncompleteError`
  vocabulary. Forgiving about unanswered widgets, strict about contradictory ones.
- `internal/layout` — placement: a `Template` of `Request`s → a `Page` of widgets
  with pixel rects. The rule: a widget's `Rect` is the area ink is *measured* in,
  never the area it occupies; captions and frames sit outside it.
- `internal/draw` — the PNG: an `image.Gray` canvas, themes, frames, fonts. The
  output must stay 8-bit greyscale, non-interlaced, `IHDR`/`IDAT`/`IEND` only —
  the template is selected by the sha256 of these bytes (`TestPNGShape` pins it).
- `internal/genconf` — the authored form: the `--check name:label=…` micro-syntax
  (`ParseArgs`/`ParseWidget`), its canonical inverse (`Command`), and the YAML
  writer whose header carries that command. Output is deterministic and carries no
  timestamp, so regenerating an unchanged template rewrites nothing.
- `pkg/snif` — the public facade: `Generate`/`Regenerate`/`ParseGen`,
  `Client.Form`/`Forms`, the archive-free `ReadRender`, `Decode`, `Validate`,
  `Bind`. Every internal type a caller can touch is aliased in `types.go`.
- `cmd/snif` — a consumer of the above. `gen` sets `SkipFlagParsing` because widget
  **order is layout order** and a flag framework loses the interleaving between
  different flags.

## Gotchas

- `snorg.RenderRegion` rasterizes the whole SVG and then crops, so snif renders the
  **full page once** and crops cells in memory. Never call it per cell.
- A box that was analyzed and held no writing is an empty answer; a box never
  analyzed is *unknown*. The difference is `PageDoc.Analysis.Regions[].SourceHash`,
  not the transcription text.
- A generated PNG's hash need not equal the device's after import; `snif bind`
  repairs that from an ingested page. Do not try to make the encoder bit-match a
  device instead.
