package snif

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
)

// BindResult says what Bind found and did.
type BindResult struct {
	PageID string `json:"page_id"`
	Hash   string `json:"hash"`          // the page's background_hash
	Image  string `json:"image"`         // the template image file
	Was    string `json:"was,omitempty"` // the hash that file used to have
	Bound  bool   `json:"bound"`         // the file was replaced
	Reason string `json:"reason,omitempty"`
}

// Bind makes a generated template image match what the device actually stores.
//
// A template is selected by the sha256 of a page's decoded background, so the
// image file named in the config must hash to exactly what the device wrote. That
// usually holds — snif generates the device's own form, 8-bit greyscale with no
// ancillary chunks — but nothing guarantees a tablet imports a PNG byte for byte,
// and a re-encode on import leaves a template that silently matches no page.
//
// Bind fixes that from the evidence: ingest one page drawn on the new template,
// and it copies that page's stored background over the generated image. The
// config keeps pointing at the same path; the path now holds the bytes the device
// produces. Nothing is written when the template already matches.
func (c *Client) Bind(pageID, imagePath string, force bool) (BindResult, error) {
	res := BindResult{PageID: pageID, Image: imagePath}

	fileID, err := c.sn.FindPage(pageID)
	if err != nil {
		return res, err
	}
	pd, err := c.sn.ReadPage(pageID)
	if err != nil {
		return res, err
	}
	if pd.BackgroundHash == "" {
		return res, fmt.Errorf("page %s records no background: it was drawn on a blank page, or ingested before background hashing", pageID)
	}
	res.Hash = pd.BackgroundHash

	if t := c.sn.Templates().MatchBackground(pd.BackgroundHash); t != nil && !force {
		res.Reason = fmt.Sprintf("already bound to %s", t.Image)
		return res, nil
	}

	src := filepath.Join(c.sn.ArchivePath(), fileID, "backgrounds", pd.BackgroundHash+".png")
	data, err := os.ReadFile(src)
	if err != nil {
		return res, fmt.Errorf("the page's background is not in the archive (%s): re-ingest with ingest.svg.background: extract", src)
	}
	if old, err := os.ReadFile(imagePath); err == nil {
		sum := sha256.Sum256(old)
		res.Was = hex.EncodeToString(sum[:])
		if res.Was == res.Hash {
			res.Reason = "the image already matches this page's background"
			return res, nil
		}
	} else if !force {
		return res, fmt.Errorf("%s does not exist: pass the generated template image to bind, or --force to create it", imagePath)
	}
	if err := os.WriteFile(imagePath, data, 0o644); err != nil {
		return res, err
	}
	res.Bound = true
	return res, nil
}
