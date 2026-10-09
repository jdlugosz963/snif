# CLI

```
snif [-a <archive>] [-c config.yaml ...] [--no-user-config] [-v] <command>

snif gen <name> [widget options] [-o DIR] [--force]
snif gen -f <generated.yaml>
snif read  [PAGEID ...] [--no-analyze] [--values] [--strict] [--ink-threshold F]
snif check [PAGEID ...]
snif bind  <PAGEID> <template.png> [--force]
```

Globals mirror snorg's and come **before** the command: `-a`/`--archive` (optional
when the merged config sets `archive:`), `-c`/`--config` (repeatable, later wins),
`--no-user-config`, `-v`/`--verbose`. The config is layered exactly as snorg layers
it, over the same files — one `-c` serves both tools, because the `templates:`
section is the form contract.

All of them are optional. `~/.config/snorg/config.yaml` is the one auto-loaded
layer; once it `include:`s the archive's config (see
[templates.md](templates.md), "Wiring it into the config"), `snif read` and
`snif check` run with no globals at all — the archive, the provider and the forms
come from the chain.

Data goes to **stdout**, progress and per-page failures to **stderr**, so
`snorg query … | snif read | jq` works and `2>/dev/null` silences the noise alone.

## gen

Generates `snif-<name>.png` and `snif-<name>.yaml` (see [templates.md](templates.md)
for the widget syntax; `snif gen --help` prints it). Needs no archive. Widget flags
are read in the order typed, which is the order they are laid out — which is why
`gen` parses its own arguments rather than declaring flags.

`-f <yaml>` re-renders a generated template by re-running the command in its
header, rewriting both files in place.

## read

PAGEIDs from the arguments, or one per line from stdin. Prints a JSON array, one
object per page: `page_id`, `template`, the flat `values` map, the `widgets` with
their per-cell ink measurements, and `errors` for widgets that could not be read.

- `--values` prints only `{page_id, values}`.
- `--no-analyze` never transcribes an `input` field.
- `--strict` exits non-zero when any widget could not be read.
- `--ink-threshold` sets the inked fraction at which a cell counts as ticked.

A page that fails (not templated, not a snif form) is reported on stderr and
omitted from the array; the command exits non-zero, but the other pages still
print.

## check

With no arguments, validates every configured template as a form and lists them
(`-v` prints each widget). With PAGEIDs, shows how those pages read, per widget, in
a form meant for a person — and never transcribes anything.

## bind

Points a generated template image at what the device actually stores. See
[templates.md](templates.md), "Importing, and the hash problem".
