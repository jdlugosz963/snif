package main

import (
	"context"
	"fmt"

	"github.com/urfave/cli/v3"
)

func bindCmd() *cli.Command {
	return &cli.Command{
		Name:      "bind",
		Usage:     "point a generated template image at what the device actually stores",
		ArgsUsage: "<PAGEID> <template.png>",
		Description: "Ingest one page drawn on the new template, then bind it:\n" +
			"  snorg -a A ingest note.note && snif -a A bind <PAGEID> templates/snif-daily.png\n" +
			"The config keeps its image: path; the file gets the bytes the device wrote,\n" +
			"so the template's hash matches every page drawn on it.",
		Flags: []cli.Flag{
			&cli.BoolFlag{Name: "force", Usage: "rebind even when the template already matches"},
		},
		Action: func(_ context.Context, cmd *cli.Command) error {
			if cmd.Args().Len() != 2 {
				return fmt.Errorf("bind takes a PAGEID and the template image path")
			}
			c, err := client(cmd)
			if err != nil {
				return err
			}
			r, err := c.Bind(cmd.Args().Get(0), cmd.Args().Get(1), cmd.Bool("force"))
			if err != nil {
				return err
			}
			switch {
			case r.Bound:
				fmt.Printf("%s\t%s\n", r.Image, r.Hash)
				logf("bound %s to the background of %s (was %s)", r.Image, r.PageID, short(r.Was))
			default:
				logf("%s: %s", r.Image, r.Reason)
			}
			return nil
		},
	}
}

func short(hash string) string {
	if len(hash) > 12 {
		return hash[:12]
	}
	if hash == "" {
		return "missing"
	}
	return hash
}
