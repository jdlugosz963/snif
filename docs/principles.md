# SNIF — Project Principles

SNIF = **s**uper**n**ote **i**nter**f**ace.

**Purpose:** use Supernote pages as user interfaces. snif generates a form
template — a background image plus the snorg `templates:` config that describes it
— and reads a page drawn on that template back as typed values. Handwriting is
read by *where the ink is*, not by what it says; only free-text fields involve a
model.

## Engineering principles

Inherited from snorg (`snorg/docs/principles.md`), plus what follows from being
built on it:

- **No backward compatibility** — when something must change, drop the legacy and
  rewrite cleanly.
- **Go.**
- **Future-proof over cheap** — heavy abstraction is fine when genuinely useful
  long-term.
- **Plaintext-first** — all operational data is plaintext (VCS-friendly).
- **Docs in English, maximally concise**, preserving original meaning.
- **snorg is the only backend** — snif never parses `.note`, never writes the
  archive layout, never rasterizes a page itself. Everything it needs it asks
  `pkg/snorg` for.
- **No configuration of its own** — a widget is encoded in the id of its snorg
  template box, so one `templates:` section is both tools' whole contract. A
  generated file is self-describing and passes to either tool with `-c`.
- **Reading costs nothing** — every widget but `input` is decided from ink, with
  no LLM call and no credentials. A form with no `input` never reaches a provider.
- **The library is the product** — `cmd/snif` is a consumer of `pkg/snif` exactly
  as snorg's CLI consumes `pkg/snorg`. Generation and reading are both public.
- **Refuse rather than guess** — two ticks in a one-of group is an error naming
  both, never a coin toss. An unanswered widget, by contrast, is not an error.
