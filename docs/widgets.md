# Widgets and the box-id grammar

snif has no configuration of its own. A widget is encoded in the **id of a snorg
template box**, so one `templates:` section is the whole contract: snorg matches
pages to the template and transcribes the text fields, snif reads the ink, and a
human reading the YAML sees both.

```
<kind> ":" <name> [ ":" <arg> ]*          name = [A-Za-z0-9][A-Za-z0-9_.-]*
```

`:` and `.` are legal inside snorg's `region[…]` scope, so every id below is also
a query term: `snorg query 'region[input:notes]:dentist'`.

| Kind | Boxes | Value | Read from |
|---|---|---|---|
| `check` | `check:done` | bool | ink in the box |
| `button` | `button:submit` | bool | ink in the box |
| `input` | `input:notes` — **the only `analyze: true` box** | string | snorg's transcription |
| `slider` | `slider:mood:0:10[:<step>]` | float | the rightmost inked column |
| `radio` | `radio:prio` + `radio:prio:low`, `radio:prio:mid`, … | string | the one ticked cell |
| `date` | `date:due:2026:2030` + `date:due:day:07`, `date:due:month:09`, `date:due:year:2026` | date | one cell per part |

## Regions: a box that is not a widget

`--figure <name>` places an area to **draw** in. It is not a widget: it has no
value, so nothing reads it, and `figure:<name>` is absent from the kind table
above on purpose — `spec.Parse` returns `ErrNotWidget` for it, which is how an
ordinary snorg box has always been told from snif's own.

```yaml
- {id: "figure:main", label: "Figure", rect: {x: 80, y: 524, w: 1760, h: 1300}, analyze: false}
```

`analyze: false` is the point. snorg never sends a drawing to a vision model, but
it does publish the box's `rect`, so a consumer crops the drawing straight out of
the page (`snorg/docs/templates.md`, "a structural region with no transcription").
`snorg-pdf` is the first such consumer: it turns a figure box into a figure in a
typeset document, captioned from the `input` box beside it.

snif places and frames the area and then forgets about it — `snif check` reports
nothing for it and `snif read` returns no value for it.

A **group box** (`radio:<name>`, `date:<name>:<y0>:<y1>`) declares the widget and
carries its caption in `label`; its **cells** carry the glyph printed in them. A
date cell is told from the group by its third segment: `day`, `month` or `year`
rather than a number. Single-box widgets use their own `label` as the caption.

`label` and `rect` are snorg's own fields, so nothing is duplicated: renaming a
caption is a label edit, moving a widget is a rect edit.

## What each kind measures

- **check / button.** Ticked iff the inked fraction of the box reaches the
  threshold (0.5% by default, `--ink-threshold`). A tick box always has an answer:
  ticked or not. `button` reads identically and means "act on this page" rather
  than "this is true", so a consumer can trigger on one and merely store the other.
- **radio.** Exactly one cell may be ticked. None is an *unanswered* widget, not an
  error. More than one is an error naming every ticked cell with its ink fraction —
  guessing between two deliberate marks would be worse than saying so.
- **date.** Three radio groups read as one day. All three empty is unanswered;
  some-but-not-all is an incomplete-date error naming what is missing; 31 February
  is reported rather than silently rolled into March.
- **slider.** The value is the position of the **rightmost inked column** in the
  track: `min + (x - x₀ + 1)/w · (max - min)`, snapped to `step` when one is
  declared. A digital page has no noise, so the last inked column is exactly the
  mark the user drew — and crossing out a value and marking a higher one gives the
  higher one.
- **input.** The box's transcription, from snorg's `<PAGEID>.md` region sections. A
  box that was analyzed and held no writing is a genuinely empty answer; a box
  nobody has analyzed is *unknown* and reads as `ErrNotAnalyzed` (see
  `ReadOptions.Analyze`).

## Why ink is a sound signal

snif measures against snorg's **canonical page render** — every stroke forced
black on white, the template background dropped along with the nav/link overlays.
Two consequences the design leans on:

1. **Printed decoration is invisible.** Frames, dotted fills, the digit inside a
   day cell — all of it lives in the background image, which the canonical render
   does not resolve. Nothing printed can be read as an answer, so a widget may be
   decorated freely.
2. **Ink is split at the cell boundary exactly.** A stroke crossing an edge
   contributes only its in-box pixels, and cells are laid out with a ≥12px gutter,
   so a tick that strays a little stays in its own box.

The ink threshold is snorg's own (darker than mid-grey), so snif's "this box holds
ink" and snorg's "this box is blank, skip the LLM call" can never disagree.

## Checking a template

`snif check` validates what a hand edit can break — these fail quietly rather than
loudly, which is why they are checked:

- a malformed id under a known kind (an unknown prefix is just an ordinary snorg
  box and is ignored);
- a cell with no group box, a group with no cells, a duplicate id or name;
- `analyze` disagreeing with the kind (a tick box marked `analyze: true` spends a
  vision call; an `input` marked false never gets its text);
- a rect off the page, or two measured rects overlapping — overlapping cells both
  answer for the same ink.
