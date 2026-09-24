// Command vrx is the VRX command line: `show` commands and a configuration mode with candidate/commit/rollback
// semantics, as a thin client of vrx-api. See docs/user/cli/reference.md.
package main

import (
	"os"

	"ngfw/cli/internal/cli"
)

func main() {
	os.Exit(cli.New().Main(os.Args[1:]))
}
