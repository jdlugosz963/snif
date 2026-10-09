package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/jdlugosz963/snif/pkg/snif"
	"github.com/urfave/cli/v3"
)

// genCmd parses its own arguments rather than declaring cli flags, because widget
// order is layout order: a flag framework hands back one slice per flag and loses
// the interleaving between --check and --input. Parsing the raw list keeps the
// page in the order it was typed — and lets `-f` reuse exactly the same parser on
// the command stored in a generated file's header.
func genCmd() *cli.Command {
	return &cli.Command{
		Name:            "gen",
		Usage:           "generate a template image and its snorg templates: config",
		ArgsUsage:       "<name> [options]",
		Description:     snif.GenUsage,
		SkipFlagParsing: true,
		Action: func(_ context.Context, cmd *cli.Command) error {
			argv := cmd.Args().Slice()
			for _, a := range argv {
				if a == "-h" || a == "--help" {
					fmt.Println(snif.GenUsage)
					return nil
				}
			}
			args, err := snif.ParseGen(argv)
			if err != nil {
				return fmt.Errorf("%w\n\n%s", err, snif.GenUsage)
			}
			if args.From != "" {
				return regenerate(args)
			}
			g, err := snif.Generate(args.Template)
			if err != nil {
				return err
			}
			return write(g, args.OutDir, args.Force)
		},
	}
}

// regenerate re-renders a template from the command recorded in its own YAML —
// the round-trip that makes the header the template's source.
func regenerate(args *snif.GenArgs) error {
	data, err := os.ReadFile(args.From)
	if err != nil {
		return err
	}
	g, err := snif.Regenerate(data)
	if err != nil {
		return fmt.Errorf("%s: %w", args.From, err)
	}
	dir := args.OutDir
	if dir == "." {
		dir = filepath.Dir(args.From) // rewrite in place by default
	}
	return write(g, dir, true)
}

func write(g *snif.Generated, dir string, force bool) error {
	paths, err := g.Write(dir, force)
	if err != nil {
		return err
	}
	for _, p := range paths {
		fmt.Println(p)
	}
	logf("%s: %s", g.Name, g.Summary())
	logf("image sha256 %s — if the device re-encodes it on import, run `snif bind <PAGEID> %s`",
		g.Hash[:12], filepath.Join(dir, g.Image))
	return nil
}
