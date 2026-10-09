package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/jdlugosz963/snif/pkg/snif"
	"github.com/urfave/cli/v3"
)

func readCmd() *cli.Command {
	return &cli.Command{
		Name:      "read",
		Usage:     "read filled-in pages as JSON",
		ArgsUsage: "[PAGEID ...]",
		Description: "PAGEIDs come from the arguments, or one per line on stdin:\n" +
			"  snorg -c form.yaml query templated | snif -c form.yaml read",
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:  "no-analyze",
				Usage: "never transcribe an input field; report it as unanalyzed instead",
			},
			&cli.BoolFlag{
				Name:    "values",
				Aliases: []string{"V"},
				Usage:   "print only the name-to-answer map of each page",
			},
			&cli.BoolFlag{
				Name:  "strict",
				Usage: "exit non-zero when any widget is unset or unreadable",
			},
			&cli.FloatFlag{
				Name:  "ink-threshold",
				Usage: "inked fraction at which a cell counts as ticked",
				Value: snif.DefaultThreshold,
			},
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			c, err := client(cmd)
			if err != nil {
				return err
			}
			ids, err := pageIDs(cmd)
			if err != nil {
				return err
			}
			if len(ids) == 0 {
				return fmt.Errorf("no PAGEIDs given (pass them as arguments or pipe them in)")
			}
			opts := snif.ReadOptions{
				Analyze:   !cmd.Bool("no-analyze"),
				Threshold: cmd.Float("ink-threshold"),
			}

			out := []any{} // an empty result prints as [], never null
			failed, bad := 0, 0
			for _, r := range c.Forms(ctx, ids, opts) {
				if r.Err != nil {
					failed++
					logf("%s: %v", r.PageID, r.Err)
					continue
				}
				if errs := r.Form.Errs(); len(errs) > 0 {
					bad += len(errs)
					for _, e := range errs {
						logf("%s: %v", r.PageID, e)
					}
				} else if cmd.Bool("verbose") {
					logf("%s: %s", r.PageID, r.Form.Template)
				}
				if cmd.Bool("values") {
					out = append(out, map[string]any{"page_id": r.PageID, "values": r.Form.Values()})
					continue
				}
				out = append(out, r.Form)
			}

			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			if err := enc.Encode(out); err != nil {
				return err
			}
			if failed > 0 {
				return fmt.Errorf("%d of %d pages could not be read", failed, len(ids))
			}
			if bad > 0 && cmd.Bool("strict") {
				return fmt.Errorf("%d widgets could not be read", bad)
			}
			return nil
		},
	}
}
