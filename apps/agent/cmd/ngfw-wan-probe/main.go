// ngfw-wan-probe is a fixed carrier-namespace executor. Only the privileged
// carrier broker enters a verified namespace; this binary cannot enter one or
// select a device, execute commands, resolve names, or change network policy.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"time"

	"google.golang.org/protobuf/proto"
	ngfwv1 "ngfw/agent/gen/ngfw/v1"
	"ngfw/agent/internal/multiwan"
)

type result struct {
	Sent        int  `json:"sent"`
	Received    int  `json:"received"`
	LatencyMS   int  `json:"latencyMs"`
	Unavailable bool `json:"unavailable"`
}

func run(ctx context.Context, args []string, output io.Writer, probe multiwan.Probe) error {
	flags := flag.NewFlagSet("ngfw-wan-probe", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	kind := flags.String("kind", "", "")
	target := flags.String("target", "", "")
	timeout := flags.Uint("timeout-ms", 3000, "")
	if flags.Parse(args) != nil || flags.NArg() != 0 || *timeout < 1 || *timeout > 3000 || !multiwan.CarrierProbeTarget(*kind, *target) {
		return errors.New("invalid fixed carrier probe request")
	}
	deadline, cancel := context.WithTimeout(ctx, time.Duration(*timeout)*time.Millisecond)
	defer cancel()
	monitor := &ngfwv1.WanMonitor{Type: kind, Target: target, TimeoutMs: proto.Uint32(uint32(*timeout))} //nolint:gosec // timeout bounded to 1..3000 above
	observed := probe(deadline, "ppp0", monitor)
	if observed.Sent != 1 || observed.Received < 0 || observed.Received > 1 || observed.AvgLatencyMs < 0 || observed.AvgLatencyMs > int(*timeout) {
		observed = multiwan.CheckResult{Sent: 1, Unavailable: true}
	}
	return json.NewEncoder(output).Encode(result{Sent: observed.Sent, Received: observed.Received, LatencyMS: observed.AvgLatencyMs, Unavailable: observed.Unavailable})
}

func main() {
	probe := multiwan.DeviceProbe(func(string) (string, error) { return "ppp0", nil })
	if err := run(context.Background(), os.Args[1:], os.Stdout, probe); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, "fixed carrier probe failed")
		os.Exit(2)
	}
}
