// Command snif generates Supernote form templates and reads pages drawn on them.
//
// It is a consumer of pkg/snif, exactly as snorg's CLI is a consumer of pkg/snorg:
// everything this program does is available to a Go program too.
//
//	snif [-a <archive>] [-c config.yaml ...] [--no-user-config] [-v] <command>
//
// Data goes to stdout and progress to stderr, so `snorg query … | snif read` and
// `snif read … | jq` both work.
package main

import (
	"context"
	"fmt"
	"os"

	"github.com/urfave/cli/v3"
)

// version is stamped by the release build; it is informational only (the version
// recorded in a generated template is the library's snif.Version).
var version = "dev"

func main() {
	cmd := &cli.Command{
		Name:                  "snif",
		Usage:                 "supernote interface: generate form templates and read filled-in pages",
		Version:               version,
		EnableShellCompletion: true,
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:    "archive",
				Aliases: []string{"a"},
				Usage:   "archive root (defaults to the config's archive: key)",
			},
			&cli.StringSliceFlag{
				Name:    "config",
				Aliases: []string{"c"},
				Usage:   "config file; repeat to layer, later wins (the templates: section is the form contract)",
			},
			&cli.BoolFlag{
				Name:  "no-user-config",
				Usage: "skip the XDG user config",
			},
			&cli.BoolFlag{
				Name:    "verbose",
				Aliases: []string{"v"},
				Usage:   "report per page on stderr",
			},
		},
		Commands: []*cli.Command{genCmd(), readCmd(), checkCmd(), bindCmd()},
	}
	if err := cmd.Run(context.Background(), os.Args); err != nil {
		fmt.Fprintln(os.Stderr, "snif:", err)
		os.Exit(1)
	}
}
