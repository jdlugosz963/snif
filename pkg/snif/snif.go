// Package snif is the public Go API for the supernote interface (SNIF): pages of
// a Supernote notebook used as forms.
//
// A snif template is an ordinary snorg template — a background image plus a list
// of boxes — whose box ids encode widgets: a tick box, a group of options, a date
// picker, a slider, a handwriting field. snif has two halves, both exposed here:
//
//   - Generate builds a template. It returns the 1920x2560 greyscale PNG to import
//     on the device and the snorg templates: YAML that describes it, ready to pass
//     to either tool with -c.
//   - Client reads a page drawn on such a template back as typed values. Every
//     widget but Input is read from where the ink is — no model, no credentials,
//     no cost — because a pen tablet's page is noise-free: a box either holds ink
//     or it does not, and a slider's mark is simply its rightmost inked column.
//
// snorg is the only backend. snif never parses .note files, never touches the
// archive layout and never rasterizes a page itself; it asks snorg for the page's
// canonical render (once per page) and for the transcription of the one widget
// kind that needs a vision model.
//
//	import "github.com/jdlugosz963/snif/pkg/snif"
package snif

import (
	snorg "github.com/jdlugosz963/snorg/pkg/snorg"
)

// Version is the snif release, recorded in the header of every generated template.
const Version = "v0.1.0"

// Client reads snif forms out of one snorg archive. It is safe to reuse and holds
// no cross-call state.
type Client struct {
	sn *snorg.Client
}

// ResolveOptions configures Resolve, mirroring the snorg CLI's global flags — so a
// snif front-end layers configuration exactly as snorg does, from the same files.
type ResolveOptions = snorg.ResolveOptions

// Open wraps an existing snorg client. Use it when the caller already has one (and
// therefore already resolved the archive and its templates: config).
func Open(sn *snorg.Client) *Client { return &Client{sn: sn} }

// Resolve builds a Client the way the CLI does: the archive path, then the merged
// config in increasing precedence (XDG user config, then -c files). The templates:
// section of that config is the form contract, so the same -c that lets snorg
// analyze a templated page lets snif read it.
func Resolve(opts ResolveOptions) (*Client, error) {
	sn, err := snorg.Resolve(opts)
	if err != nil {
		return nil, err
	}
	return Open(sn), nil
}

// Snorg returns the underlying snorg client, so a caller that has a snif Client
// can reach the whole archive — query, retrieve, tag, export — without building a
// second one.
func (c *Client) Snorg() *snorg.Client { return c.sn }

// ArchivePath returns the archive root the client reads.
func (c *Client) ArchivePath() string { return c.sn.ArchivePath() }
