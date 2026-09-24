// Command vrx-opgen generates the CLI's REST operation table from the API's OpenAPI document
// (packages/api-client/openapi.json, written by `pnpm gen`). The CLI calls operations only by operationId through
// that table, so every command maps to a documented REST call and a renamed/removed route breaks the build of the
// CLI instead of failing at run time.
//
//	go run ./cmd/vrx-opgen -in ../../packages/api-client/openapi.json -out internal/api/operations_gen.go
package main

import (
	"flag"
	"fmt"
	"os"

	"ngfw/cli/internal/api/opgen"
)

func main() {
	in := flag.String("in", "../../packages/api-client/openapi.json", "OpenAPI document")
	out := flag.String("out", "internal/api/operations_gen.go", "generated Go file")
	flag.Parse()
	doc, err := os.ReadFile(*in)
	if err != nil {
		fmt.Fprintln(os.Stderr, "vrx-opgen:", err)
		os.Exit(1)
	}
	src, err := opgen.Generate(doc)
	if err != nil {
		fmt.Fprintln(os.Stderr, "vrx-opgen:", err)
		os.Exit(1)
	}
	if err := os.WriteFile(*out, src, 0o644); err != nil { //nolint:gosec // generated source file, world-readable like the rest of the repo
		fmt.Fprintln(os.Stderr, "vrx-opgen:", err)
		os.Exit(1)
	}
	fmt.Printf("vrx-opgen: %s written\n", *out)
}
