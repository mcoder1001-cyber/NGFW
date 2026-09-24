// Command vrx-vppcheck answers the read-only questions deploy/vpp/apply-startup.sh asks VPP
// while it applies a start-up configuration (F-startup-apply, re-review N1/N3). It talks to VPP
// only through the agent's binary-API client (internal/vpp: govpp over the API socket, bindings
// from apps/agent/binapi) — never vppctl, never vpp_papi — and every call has a deadline.
//
//	vrx-vppcheck [--socket PATH] [--timeout DUR] version          print "vpp <version>" (show_version)
//	vrx-vppcheck [--socket PATH] [--timeout DUR] plugins          loaded plugins, one per line (content of `show plugins`)
//	vrx-vppcheck [--socket PATH] [--timeout DUR] ifaces NAME...   every NAME is a VPP interface (sw_interface_dump by name, exact match)
//
// The socket defaults to $VRX_VPP_API_SOCKET, then /run/vpp/api.sock; the timeout (default 10s)
// bounds the whole run: connect plus every request. A VPP that accepts the socket but does not
// answer (hung main loop) therefore ends in exit 2 after the timeout, never in a blocked caller.
// A hard backstop exits the process one second after the deadline even if a library call ignores
// its context.
//
// Exit: 0 ok · 1 answered, but a NAME is missing (the missing names are printed) · 2 VPP not
// reachable / not answering in time / error · 64 usage.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"regexp"
	"sort"
	"strings"
	"time"

	"ngfw/agent/binapi/interface_types"
	interfaces "ngfw/agent/binapi/interface"
	"ngfw/agent/binapi/vlib"
	"ngfw/agent/binapi/vpe"
	"ngfw/agent/internal/vpp"
)

// Exit codes.
const (
	exitOK       = 0
	exitMissing  = 1
	exitVPP      = 2
	exitUsage    = 64
	defaultSock  = "/run/vpp/api.sock"
	defaultLimit = 10 * time.Second
)

// dialer connects to VPP; replaced in tests.
type dialer func(ctx context.Context, socket string) (vpp.Client, func(), error)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr, dialVPP, time.AfterFunc))
}

func dialVPP(ctx context.Context, socket string) (vpp.Client, func(), error) {
	if fi, err := os.Stat(socket); err != nil { //nolint:gosec // operator-supplied socket path, only stat'ed
		return nil, nil, fmt.Errorf("no VPP API socket: %w", err)
	} else if fi.Mode()&os.ModeSocket == 0 {
		return nil, nil, fmt.Errorf("%s is not a socket", socket)
	}
	c := vpp.Dial(socket, vpp.ConnOptions{
		Attempts: 1, Interval: 200 * time.Millisecond,
		MinBackoff: 200 * time.Millisecond, MaxBackoff: time.Second,
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	if err := c.WaitConnected(ctx); err != nil {
		go c.Close() // never block the exit path on govpp's disconnect
		return nil, nil, err
	}
	return c, func() { go c.Close() }, nil
}

func run(args []string, stdout, stderr io.Writer, dial dialer, afterFunc func(time.Duration, func()) *time.Timer) int {
	fs := flag.NewFlagSet("vrx-vppcheck", flag.ContinueOnError)
	fs.SetOutput(stderr)
	sock := os.Getenv("VRX_VPP_API_SOCKET")
	if sock == "" {
		sock = defaultSock
	}
	fs.StringVar(&sock, "socket", sock, "VPP binary API socket")
	limit := fs.Duration("timeout", defaultLimit, "deadline for the whole run (connect + requests)")
	fs.Usage = func() {
		_, _ = fmt.Fprintln(stderr, "usage: vrx-vppcheck [--socket PATH] [--timeout DUR] version | plugins | ifaces NAME...")
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		return exitUsage
	}
	rest := fs.Args()
	if len(rest) == 0 || *limit <= 0 {
		fs.Usage()
		return exitUsage
	}
	cmd, names := rest[0], rest[1:]
	switch cmd {
	case "version", "plugins":
		if len(names) != 0 {
			fs.Usage()
			return exitUsage
		}
	case "ifaces":
		if len(names) == 0 {
			fs.Usage()
			return exitUsage
		}
	default:
		fs.Usage()
		return exitUsage
	}

	// Hard backstop: whatever a library does with the context, the process ends.
	backstop := afterFunc(*limit+time.Second, func() {
		_, _ = fmt.Fprintf(stderr, "vrx-vppcheck: VPP did not answer within %s (hard deadline)\n", *limit)
		os.Exit(exitVPP)
	})
	defer backstop.Stop()

	ctx, cancel := context.WithTimeout(context.Background(), *limit)
	defer cancel()
	client, closeFn, err := dial(ctx, sock)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "vrx-vppcheck: cannot reach VPP at %s: %v\n", sock, err)
		return exitVPP
	}
	defer closeFn()

	switch cmd {
	case "version":
		v, err := version(ctx, client)
		if err != nil {
			_, _ = fmt.Fprintf(stderr, "vrx-vppcheck: show_version: %v\n", err)
			return exitVPP
		}
		_, _ = fmt.Fprintf(stdout, "vpp %s\n", v)
	case "plugins":
		ps, err := plugins(ctx, client)
		if err != nil {
			_, _ = fmt.Fprintf(stderr, "vrx-vppcheck: show plugins: %v\n", err)
			return exitVPP
		}
		for _, p := range ps {
			_, _ = fmt.Fprintln(stdout, p)
		}
	case "ifaces":
		missing, err := missingIfaces(ctx, client, names)
		if err != nil {
			_, _ = fmt.Fprintf(stderr, "vrx-vppcheck: sw_interface_dump: %v\n", err)
			return exitVPP
		}
		if len(missing) > 0 {
			_, _ = fmt.Fprintf(stdout, "missing: %s\n", strings.Join(missing, " "))
			return exitMissing
		}
		_, _ = fmt.Fprintf(stdout, "present: %s\n", strings.Join(names, " "))
	}
	return exitOK
}

func version(ctx context.Context, c vpp.Client) (string, error) {
	rep, err := vpe.NewServiceClient(c).ShowVersion(ctx, &vpe.ShowVersion{})
	if err != nil {
		return "", err
	}
	if rep.Version == "" {
		return "", errors.New("empty version")
	}
	return rep.Version, nil
}

// pluginLine matches one row of VPP's `show plugins` table: "  12. dpdk_plugin.so   26.06 …".
var pluginLine = regexp.MustCompile(`^\s*\d+\.\s+([A-Za-z0-9_-]+_plugin\.so)\s`)

func plugins(ctx context.Context, c vpp.Client) ([]string, error) {
	rep, err := vlib.NewServiceClient(c).CliInband(ctx, &vlib.CliInband{Cmd: "show plugins"})
	if err != nil {
		return nil, err
	}
	return parsePlugins(rep.Reply)
}

func parsePlugins(text string) ([]string, error) {
	seen := map[string]bool{}
	for _, l := range strings.Split(text, "\n") {
		if m := pluginLine.FindStringSubmatch(l + " "); m != nil {
			seen[m[1]] = true
		}
	}
	if len(seen) == 0 {
		// A running VPP always has plugins (at least the ones the start-up file enables); an
		// empty table means the answer is not what we think it is — never report "none loaded".
		return nil, errors.New("no plugin rows in the reply")
	}
	out := make([]string, 0, len(seen))
	for p := range seen {
		out = append(out, p)
	}
	sort.Strings(out)
	return out, nil
}

// missingIfaces asks VPP for every name (name_filter is a substring match, so the result is
// compared exactly) and returns the names VPP does not have, in the order given.
func missingIfaces(ctx context.Context, c vpp.Client, names []string) ([]string, error) {
	svc := interfaces.NewServiceClient(c)
	var missing []string
	for _, name := range names {
		stream, err := svc.SwInterfaceDump(ctx, &interfaces.SwInterfaceDump{
			SwIfIndex:       interface_types.InterfaceIndex(^uint32(0)),
			NameFilterValid: true,
			NameFilter:      name,
		})
		if err != nil {
			return nil, err
		}
		found := false
		for {
			d, err := stream.Recv()
			if errors.Is(err, io.EOF) {
				break
			}
			if err != nil {
				return nil, err
			}
			if d.InterfaceName == name {
				found = true
			}
		}
		if !found {
			missing = append(missing, name)
		}
	}
	return missing, nil
}
