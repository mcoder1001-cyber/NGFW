package lb

// VPP's lb garbage collection, triggered by the globals owner after lb deletes (D-090 (2); F-lb gap: a missing
// helper — TestGarbageCollect*). The only CLI use the agent makes: one constant command through the cli_inband
// binary-API message, no user input (apps/agent/internal/renderers/ALLOWLIST.md).

import (
	"context"
	"errors"
	"fmt"
	"net/netip"
	"strings"
	"time"

	"ngfw/agent/binapi/vlib"
	"ngfw/agent/internal/descriptors/df7"
	"ngfw/agent/internal/vpp"
)

// GCCommand is the constant CLI line that runs lb_garbage_collection() without changing anything else.
//
// Why this line (VPP 26.06 src/plugins/lb/cli.c): `lb conf` without arguments returns before its garbage collection
// (unformat_line_input fails on an empty line), and every `lb conf` argument overwrites a global; `lb as` needs an
// existing VIP. `lb vip <prefix> del` parses, runs lb_garbage_collection(), then fails its lookup with
// "lb_vip_find_index error -6" for a VIP that does not exist — and 0.0.0.0/32 ("this host on this network", RFC
// 1122) is never a VIP: the schema and the projection refuse every VIP in 0.0.0.0/8 (services.lb-vip-reserved).
const GCCommand = "lb vip 0.0.0.0/32 del"

// GCSentinelRange holds the VIP GCCommand names (0.0.0.0/32); no VIP may use a prefix inside it.
var GCSentinelRange = netip.MustParsePrefix("0.0.0.0/8")

// GCDelay is how long after the last lb delete the garbage collection runs: VPP frees a removed AS only when it was
// removed more than LB_CONCURRENCY_TIMEOUT (10 s) ago, and collects one VIP at most every LB_GARBAGE_RUN (60 s) after
// its creation or its last collection (lb.c) — a collection right after the transaction would free nothing it deleted.
const GCDelay = 65 * time.Second

// gcDone is the reply text of GCCommand after the collection ran (the sentinel VIP's lookup fails).
const gcDone = "lb_vip_find_index error"

// ErrGCUnsafe is returned when VPP state makes the collection dangerous (see GCSafe); nothing was sent.
var ErrGCUnsafe = errors.New("lb: garbage collection skipped")

// GCSafe reports whether lb_garbage_collection() can run safely. VPP 26.06 keys the SNAT mapping of a NAT VIP with a
// port by (AS address, target port) only (lb.c lb_vip_add_ass): two VIP entries — typically a NAT VIP and its own
// "removed" predecessor after a change (delete + add) — with an AS of the same address and target port share one
// mapping; collecting the removed one frees the live VIP's mapping, and collecting the second then calls pool_put on
// a NULL mapping (lb_vip_garbage_collection, ASSERT compiled out) — a likely VPP crash. So the collection is refused
// while any (AS address, target port) of NAT port VIPs occurs in more than one lb_as_dump row (V20 follow-up).
func GCSafe(vips []VIPState, ases []ASState) (bool, string) {
	type group struct {
		prefix string
		port   uint16
	}
	targets := map[group][]uint16{}
	for _, v := range vips {
		if (v.Encap == EncapNAT4 || v.Encap == EncapNAT6) && v.Port != 0 {
			g := group{v.Prefix, v.Port}
			targets[g] = append(targets[g], v.TargetPort)
		}
	}
	seen := map[string]bool{}
	for _, a := range ases {
		for _, tp := range targets[group{a.Prefix, a.Port}] {
			k := fmt.Sprintf("%s|%d", a.Address, tp)
			if seen[k] {
				return false, fmt.Sprintf("NAT server %s target port %d appears in more than one VIP entry (a changed or duplicated NAT VIP): collecting it would free a shared SNAT mapping (V20)", a.Address, tp)
			}
			seen[k] = true
		}
	}
	return true, ""
}

// GarbageCollect runs VPP's lb garbage collection once (GCCommand through cli_inband) after checking GCSafe. Only
// the globals owner calls it (D-071, D-090): the collection is VPP-wide.
func GarbageCollect(ctx context.Context, c vpp.Client) error {
	vips, err := DumpVIPs(ctx, c)
	if err != nil {
		return err
	}
	if len(vips) == 0 {
		return nil // nothing to collect
	}
	ases, err := DumpASes(ctx, c)
	if err != nil {
		return err
	}
	if ok, why := GCSafe(vips, ases); !ok {
		return fmt.Errorf("%w: %s", ErrGCUnsafe, why)
	}
	rep, err := vlib.NewServiceClient(c).CliInband(ctx, &vlib.CliInband{Cmd: GCCommand})
	if rep != nil && strings.Contains(rep.Reply, gcDone) {
		return nil // the collection ran; the sentinel VIP does not exist (as intended)
	}
	if err == nil {
		// VPP answered success: the sentinel VIP existed and was deleted — impossible for a VRX VIP (the projection
		// refuses GCSentinelRange); report it loudly
		return fmt.Errorf("lb: %q deleted an existing VIP 0.0.0.0/32 (not created by VRX)", GCCommand)
	}
	reply := ""
	if rep != nil {
		reply = strings.TrimSpace(rep.Reply)
	}
	return fmt.Errorf("lb: cli_inband %q: %w (reply %q)", GCCommand, df7.PluginError("lb", err), reply)
}
