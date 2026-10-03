// Command ngfw-agentctl is a developer/diagnostic client for the ngfw-agent gRPC socket (the
// product CLI is apps/cli, P13). It sends protobuf-JSON documents exactly as the API would.
//
//	ngfw-agentctl [-s socket] health
//	ngfw-agentctl [-s socket] apply  <doc.json> [-txn id] [-confirm sec] [-subsystems a,b]
//	ngfw-agentctl [-s socket] confirm <txn-id>
//	ngfw-agentctl [-s socket] dryrun <doc.json> [-subsystems a,b]
//	ngfw-agentctl [-s socket] retrieve [-subsystems a,b]
//	ngfw-agentctl [-s socket] events [-n count]
//	ngfw-agentctl [-s socket] stats [-n count] [-interval ms] [-if name,…]
//
// The socket defaults to $NGFW_AGENT_SOCKET, then /run/ngfw/agent.sock.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	ngfwv1 "ngfw/agent/gen/ngfw/v1"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "ngfw-agentctl:", err)
		os.Exit(1)
	}
}

func split(s string) []string {
	if s == "" {
		return nil
	}
	return strings.Split(s, ",")
}

func printMsg(m proto.Message) {
	fmt.Println(protojson.MarshalOptions{Multiline: true, Indent: "  "}.Format(m))
}

func readDoc(path string) (*ngfwv1.DesiredState, error) {
	b, err := os.ReadFile(path) //nolint:gosec // operator-supplied file, read only
	if err != nil {
		return nil, err
	}
	ds := &ngfwv1.DesiredState{}
	if err := protojson.Unmarshal(b, ds); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return ds, nil
}

func run(args []string) error {
	def := os.Getenv("NGFW_AGENT_SOCKET")
	if def == "" {
		def = "/run/ngfw/agent.sock"
	}
	global := flag.NewFlagSet("ngfw-agentctl", flag.ContinueOnError)
	sock := global.String("s", def, "agent socket")
	if err := global.Parse(args); err != nil {
		return err
	}
	if global.NArg() == 0 {
		return fmt.Errorf("usage: ngfw-agentctl [-s socket] health|apply|confirm|dryrun|retrieve|events|stats …")
	}
	cmd, rest := global.Arg(0), global.Args()[1:]
	fs := flag.NewFlagSet(cmd, flag.ContinueOnError)
	txn := fs.String("txn", fmt.Sprintf("ctl-%d", time.Now().UnixNano()), "transaction id")
	confirm := fs.Uint("confirm", 0, "confirm timeout (seconds)")
	subs := fs.String("subsystems", "", "comma-separated subsystems")
	n := fs.Int("n", 0, "stop after n messages (0 = until interrupted)")
	interval := fs.Uint("interval", 1000, "stats interval (ms)")
	ifs := fs.String("if", "", "comma-separated interface names (stats)")
	// flags may follow positional arguments
	var pos []string
	for len(rest) > 0 {
		if err := fs.Parse(rest); err != nil {
			return err
		}
		if fs.NArg() == 0 {
			break
		}
		pos = append(pos, fs.Arg(0))
		rest = fs.Args()[1:]
	}
	cc, err := grpc.NewClient("unix://"+*sock, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return err
	}
	defer func() { _ = cc.Close() }()
	c := ngfwv1.NewDataplaneClient(cc)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	arg := func() (string, error) {
		if len(pos) == 0 {
			return "", fmt.Errorf("%s: missing argument", cmd)
		}
		return pos[0], nil
	}
	switch cmd {
	case "health":
		h, err := c.Health(ctx, &ngfwv1.HealthRequest{})
		if err != nil {
			return err
		}
		printMsg(h)
	case "apply", "dryrun":
		p, err := arg()
		if err != nil {
			return err
		}
		ds, err := readDoc(p)
		if err != nil {
			return err
		}
		if cmd == "dryrun" {
			r, err := c.DryRun(ctx, &ngfwv1.DryRunRequest{TxnId: *txn, DesiredState: ds, Subsystems: split(*subs)})
			if err != nil {
				return err
			}
			printMsg(r)
			return nil
		}
		r, err := c.Apply(ctx, &ngfwv1.ApplyRequest{TxnId: *txn, DesiredState: ds, Subsystems: split(*subs), ConfirmTimeoutSec: uint32(*confirm)}) //nolint:gosec // seconds
		if err != nil {
			return err
		}
		printMsg(r)
	case "confirm":
		id, err := arg()
		if err != nil {
			return err
		}
		r, err := c.Apply(ctx, &ngfwv1.ApplyRequest{ConfirmTxnId: id})
		if err != nil {
			return err
		}
		printMsg(r)
	case "retrieve":
		r, err := c.Retrieve(ctx, &ngfwv1.RetrieveRequest{Subsystems: split(*subs)})
		if err != nil {
			return err
		}
		printMsg(r)
	case "events":
		sctx, scancel := context.WithCancel(context.Background())
		defer scancel()
		st, err := c.StreamEvents(sctx, &ngfwv1.StreamEventsRequest{})
		if err != nil {
			return err
		}
		for i := 0; *n == 0 || i < *n; i++ {
			e, err := st.Recv()
			if err != nil {
				return err
			}
			fmt.Println(protojson.Format(e))
		}
	case "stats":
		sctx, scancel := context.WithCancel(context.Background())
		defer scancel()
		st, err := c.StreamStats(sctx, &ngfwv1.StreamStatsRequest{IntervalMs: uint32(*interval), Interfaces: split(*ifs)}) //nolint:gosec // ms
		if err != nil {
			return err
		}
		for i := 0; *n == 0 || i < *n; i++ {
			b, err := st.Recv()
			if err != nil {
				return err
			}
			fmt.Println(protojson.Format(b))
		}
	default:
		return fmt.Errorf("unknown command %q", cmd)
	}
	return nil
}
