package snif

import (
	"github.com/jdlugosz963/snif/internal/form"
	"github.com/jdlugosz963/snif/internal/genconf"
	"github.com/jdlugosz963/snif/internal/layout"
	"github.com/jdlugosz963/snif/internal/spec"
	snorg "github.com/jdlugosz963/snorg/pkg/snorg"
)

// The widget kinds, and the first segment of every box id snif understands.
type Kind = spec.Kind

const (
	// Check is a tick box: ink present or absent.
	Check = spec.Check
	// Button is a Check that means "act on this page" rather than "this is true"
	// — the same ink rule, a different intent, so a consumer can react to one and
	// merely store the other.
	Button = spec.Button
	// Input is a handwriting area: the one widget whose value is text, and so the
	// only box snif marks analyze: true for snorg's vision model.
	Input = spec.Input
	// Slider is a track read by the position of its rightmost ink.
	Slider = spec.Slider
	// Radio is a group of cells of which exactly one may be ticked.
	Radio = spec.Radio
	// Date is three such groups — day, month, year — read as one date.
	Date = spec.Date
)

// Part names a Date widget's sub-group.
type Part = spec.Part

const (
	Day   = spec.Day
	Month = spec.Month
	Year  = spec.Year
)

// Rect is an axis-aligned rectangle in page pixel space (PageW x PageH, origin
// top-left) — the same shape and units as a snorg template box or title rect.
type Rect = spec.Rect

// The Supernote page pixel space every rect lives in.
const (
	PageW = spec.PageW
	PageH = spec.PageH
)

// Widget is one resolved interface element on a page: what it is, where it is, how
// much ink each of its cells holds and the value that follows. It is sparse — Bool
// is a Check's, Choice a Radio's, Number a Slider's, Text an Input's, Date a
// Date's — and all of them are meaningless unless Set.
type Widget = form.Widget

// Cell is one option of a Radio or Date widget, with its measured ink.
type Cell = form.Cell

// AmbiguousError reports a group in which more than one cell was ticked, naming
// every one with its ink fraction. It is the error the "exactly one box per group"
// rule produces, and it is deliberately not resolved by guessing.
type AmbiguousError = form.AmbiguousError

// IncompleteError reports a date with some but not all of day, month and year.
type IncompleteError = form.IncompleteError

// ErrNotAnalyzed marks an Input widget whose box has never been transcribed, so
// its text is unknown rather than empty (see ReadOptions.Analyze).
var ErrNotAnalyzed = form.ErrNotAnalyzed

// DefaultThreshold is the inked fraction at which a cell counts as ticked.
const DefaultThreshold = form.DefaultThreshold

// Theme is a background fill pattern, applied page-wide or inside one widget.
type Theme = layout.Theme

const (
	Blank = layout.Blank
	Dots  = layout.Dots
	Grid  = layout.Grid
	Lines = layout.Lines
)

// Themes lists the accepted theme values.
var Themes = layout.Themes

// Template is a page to generate: its name, an optional title, a background theme
// and the widgets in the order they should appear down the page.
type Template = layout.Template

// WidgetSpec is one widget as the author asks for it, before placement. Sparse per
// kind: Options/Labels are a Radio's, Year0/Year1 a Date's, Min/Max/Step a
// Slider's; W and H size the body of any widget (an Input field's dimensions).
type WidgetSpec = layout.Request

// Layout is a placed template: every widget with its pixel rects, as generation
// computed them. It is what the PNG is drawn from and what the YAML records.
type Layout = layout.Page

// PlacedWidget is one widget of a Layout: its box contract plus the geometry only
// the renderer needs.
type PlacedWidget = layout.Widget

// PlacedRegion is one region of a Layout: a plain snorg box snif places, frames
// and never reads — a figure area, whose rect a consumer crops a drawing out of.
type PlacedRegion = layout.Region

// FigureKind is the id prefix (and `--figure` flag name) of a region. It is not a
// widget kind: `figure:<name>` is an ordinary snorg box, so the form model and
// `snif check` both pass over it.
const FigureKind = layout.FigureKind

// GenArgs is a parsed `snif gen` argument list: the template it describes plus
// where to write it.
type GenArgs = genconf.Args

// GenUsage is the `snif gen` grammar, for help text and argument errors.
const GenUsage = genconf.Usage

// Rect and page documents come from snorg unchanged; these aliases save a caller
// importing both packages to name what snif hands back.
type (
	// PageDoc is a page's raw <PAGEID>.json document.
	PageDoc = snorg.PageDoc
	// SnorgTemplate is one entry of the archive's templates: config.
	SnorgTemplate = snorg.Template
)
