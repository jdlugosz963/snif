package main

import (
	"bufio"
	"fmt"
	"io"
	"os"

	"github.com/jdlugosz963/snif/pkg/snif"
	"github.com/urfave/cli/v3"
)

// client resolves the archive and config the global flags describe — the same
// layering snorg applies, over the same files, so one -c serves both tools.
func client(cmd *cli.Command) (*snif.Client, error) {
	return snif.Resolve(snif.ResolveOptions{
		ArchivePath:  cmd.String("archive"),
		ConfigFiles:  cmd.StringSlice("config"),
		NoUserConfig: cmd.Bool("no-user-config"),
	})
}

// pageIDs takes the command's arguments, or reads them one per line from stdin
// when there are none — snorg's convention, so `snorg query … | snif read` works.
func pageIDs(cmd *cli.Command) ([]string, error) {
	if cmd.Args().Len() > 0 {
		return cmd.Args().Slice(), nil
	}
	return readLines(os.Stdin)
}

func readLines(r io.Reader) ([]string, error) {
	var out []string
	s := bufio.NewScanner(r)
	for s.Scan() {
		if line := s.Text(); line != "" {
			out = append(out, line)
		}
	}
	return out, s.Err()
}

// logf reports on stderr, keeping stdout the machine-readable channel.
func logf(format string, a ...any) { fmt.Fprintf(os.Stderr, format+"\n", a...) }
