package snif_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/jdlugosz963/snif/pkg/snif"
)

// TestResolveFromUserConfig: a client built with no options at all finds its
// archive, its provider and its forms through the config chain alone — the XDG
// user config includes the archive's config, which includes the generated
// template. This is snorg's layering, and snif inherits it whole; the example in
// examples/daily is exactly this call.
func TestResolveFromUserConfig(t *testing.T) {
	// Isolate the XDG user config: without this the developer's real
	// ~/.config/snorg/config.yaml leaks an archive: default into the test.
	xdg := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", xdg)
	if err := os.MkdirAll(filepath.Join(xdg, "snorg"), 0o755); err != nil {
		t.Fatal(err)
	}
	arch := t.TempDir()

	// The generated template lands in the archive, beside the config including it.
	g := template(t)
	if _, err := g.Write(arch, true); err != nil {
		t.Fatalf("Write: %v", err)
	}
	write(t, filepath.Join(arch, "config.yaml"), "include: ["+g.File+"]\n")
	write(t, filepath.Join(xdg, "snorg", "config.yaml"),
		"archive: "+arch+"\ninclude: ["+filepath.Join(arch, "config.yaml")+"]\n")

	c, err := snif.Resolve(snif.ResolveOptions{})
	if err != nil {
		t.Fatalf("Resolve with no options: %v", err)
	}
	if got := c.ArchivePath(); got != arch {
		t.Errorf("ArchivePath = %s, want %s", got, arch)
	}

	// The templates: section arrived two includes deep, so the form is configured.
	forms := c.TemplateForms()
	if len(forms) != 1 {
		t.Fatalf("got %d forms, want 1 (the included template)", len(forms))
	}
	f := forms[0]
	if f.Name != "daily" {
		t.Errorf("form name = %q, want daily", f.Name)
	}
	if f.Hash != g.Hash {
		t.Errorf("form hash = %s, want the generated %s", f.Hash, g.Hash)
	}
	if len(f.Problems) != 0 {
		t.Errorf("problems: %v", f.Problems)
	}
	if len(f.Widgets) != 6 {
		t.Errorf("got %d widgets, want 6", len(f.Widgets))
	}

	// --no-user-config drops the only auto-loaded layer, and with it the archive.
	if _, err := snif.Resolve(snif.ResolveOptions{NoUserConfig: true}); err == nil {
		t.Error("NoUserConfig: expected a no-archive-path error")
	}
}

func write(t *testing.T, path, body string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}
