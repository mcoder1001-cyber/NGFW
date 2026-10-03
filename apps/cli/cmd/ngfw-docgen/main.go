// Command ngfw-docgen writes the CLI command reference (docs/user/cli/reference.md) from the code.
package main

import (
	"flag"
	"fmt"
	"os"

	"ngfw/cli/internal/cli"
)

func main() {
	out := flag.String("out", "../../docs/user/cli/reference.md", "output file")
	flag.Parse()
	if err := os.WriteFile(*out, []byte(cli.Markdown()), 0o644); err != nil { //nolint:gosec // documentation file
		fmt.Fprintln(os.Stderr, "ngfw-docgen:", err)
		os.Exit(1)
	}
	fmt.Printf("ngfw-docgen: %s written\n", *out)
}
