// Command vrx-vpp-preflight is the `tools/ci.sh full` pre-flight of D-095 (d): before any
// integration test touches the shared VPP it dumps the interfaces and their classify / SPD
// bindings and fails fast, naming the offending interface, when a binding (or a classify DPO in
// the FIB) points at a classify table that no longer exists — the state that crashed VPP on
// 2026-09-24 04:50:27 (V19: SIGSEGV in vnet_classify_find_entry on the first packet).
//
//	vrx-vpp-preflight [-socket /run/vpp/api.sock] [-timeout 30s]
//
// Exit status: 0 = no crash vector (warnings may be printed), 1 = crash vector found,
// 2 = VPP not reachable / API error. Read-only: it never changes VPP.
package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"time"

	"ngfw/agent/internal/vpp"
	"ngfw/agent/internal/vpp/ifsanitize"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("vrx-vpp-preflight", flag.ContinueOnError)
	fs.SetOutput(stderr)
	socket := fs.String("socket", "/run/vpp/api.sock", "VPP binary API socket")
	timeout := fs.Duration("timeout", 30*time.Second, "overall timeout")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()
	c := vpp.Dial(*socket, vpp.ConnOptions{Logger: slog.New(slog.NewTextHandler(io.Discard, nil))})
	defer c.Close()
	if err := c.WaitConnected(ctx); err != nil {
		_, _ = fmt.Fprintf(stderr, "vrx-vpp-preflight: VPP API %s not reachable: %v\n", *socket, err)
		return 2
	}
	findings, err := ifsanitize.Preflight(ctx, c)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "vrx-vpp-preflight: %v\n", err)
		return 2
	}
	fatal := 0
	for _, f := range findings {
		_, _ = fmt.Fprintln(stdout, f.String())
		if f.Fatal {
			fatal++
		}
	}
	if fatal > 0 {
		_, _ = fmt.Fprintf(stdout, "V19 pre-flight FAILED: %d classify binding(s) point at deleted classify tables — the first packet through them crashes VPP (vnet_classify_find_entry). Remove the binding or the interface named above before running integration tests; do not send traffic through it.\n", fatal)
		return 1
	}
	_, _ = fmt.Fprintf(stdout, "V19 pre-flight ok: no classify binding or classify DPO points at a missing table (%d warning(s))\n", len(findings))
	return 0
}
