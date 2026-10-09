package main

import (
	"context"
	"fmt"

	"github.com/jdlugosz963/snif/pkg/snif"
	"github.com/urfave/cli/v3"
)

// checkCmd is the offline audit: it reads the configured templates (or the given
// pages) and reports what snif makes of them, without touching a provider.
func checkCmd() *cli.Command {
	return &cli.Command{
		Name:      "check",
		Usage:     "validate the configured templates, or report what given pages read as",
		ArgsUsage: "[PAGEID ...]",
		Action: func(ctx context.Context, cmd *cli.Command) error {
			c, err := client(cmd)
			if err != nil {
				return err
			}
			if cmd.Args().Len() > 0 {
				return checkPages(ctx, c, cmd)
			}
			return checkTemplates(c, cmd.Bool("verbose"))
		},
	}
}

func checkTemplates(c *snif.Client, verbose bool) error {
	forms := c.TemplateForms()
	if len(forms) == 0 {
		return fmt.Errorf("the config declares no snif forms (a template's boxes must carry widget ids)")
	}
	problems := 0
	for _, f := range forms {
		fmt.Printf("%s\t%d widgets\t%s\n", f.Name, len(f.Widgets), f.Image)
		if verbose {
			for _, w := range f.Widgets {
				fmt.Printf("  %-7s %-12s %-20q %d cells\t%+v\n", w.Kind, w.Name, w.Label, len(w.Cells), w.Rect)
			}
		}
		for _, p := range f.Problems {
			problems++
			logf("%s: %s", f.Name, p)
		}
	}
	if problems > 0 {
		return fmt.Errorf("%d problems", problems)
	}
	return nil
}

// checkPages shows how pages read, in a form meant for a person rather than for
// jq — the same resolution `read` performs, without transcribing anything.
func checkPages(ctx context.Context, c *snif.Client, cmd *cli.Command) error {
	ids, err := pageIDs(cmd)
	if err != nil {
		return err
	}
	bad := 0
	for _, r := range c.Forms(ctx, ids, snif.ReadOptions{}) {
		if r.Err != nil {
			bad++
			fmt.Printf("%s\tERROR\t%v\n", r.PageID, r.Err)
			continue
		}
		fmt.Printf("%s\t%s\n", r.PageID, r.Form.Template)
		for _, w := range r.Form.Widgets {
			switch {
			case w.Err != nil:
				bad++
				fmt.Printf("  %-7s %-12s ERROR  %v\n", w.Kind, w.Name, w.Err)
			case w.Set:
				fmt.Printf("  %-7s %-12s %v\n", w.Kind, w.Name, w.Value())
			default:
				fmt.Printf("  %-7s %-12s —\n", w.Kind, w.Name)
			}
		}
	}
	if bad > 0 {
		return fmt.Errorf("%d problems", bad)
	}
	return nil
}
