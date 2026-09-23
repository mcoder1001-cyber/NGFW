package dfkit

import (
	"context"
	"errors"
	"fmt"

	"google.golang.org/protobuf/proto"

	"ngfw/agent/internal/scheduler"
)

// ErrNotGlobalsOwner is returned when an agent that is not the designated globals owner (D-071)
// would have to set, reset or disable a VPP-global singleton.
var ErrNotGlobalsOwner = errors.New("not the globals owner: VPP-global settings are managed by the globals owner only (D-071)")

// Globals is the D-071 role of the agent for VPP-global singletons (DNS resolver, flowprobe and
// sFlow parameters, IPFIX exporter 0 and classify stream, linux-cp default netns, BPF and pcap
// filter functions, http_static, DHCPv6 DUID). Only the globals owner (agent config
// globalsOwner: true — the product agent on a real box, never a test slot on the shared host)
// sets them. Every other agent may only REQUIRE a value: Create succeeds when VPP already has it
// (checked through the getter, where VPP has one) and fails with ErrNotGlobalsOwner otherwise;
// Delete is a no-op; Retrieve returns ErrRetrieveUnsupported, which makes the reconciler treat the
// requirement as write-only (re-checked on every resync, never deleted because of absence).
type Globals struct{ owner bool }

// GlobalsOwner returns the Globals role; true only for the designated globals owner.
func GlobalsOwner(owner bool) Globals { return Globals{owner: owner} }

// Owner reports whether this agent manages VPP-globals.
func (g Globals) Owner() bool { return g.owner }

// Require is Create/Update for a non-owner: current returns VPP's value (ok=false when VPP has
// no getter or the global is unset); success only if it equals desired.
func (g Globals) Require(ctx context.Context, name string, desired proto.Message,
	current func(context.Context) (proto.Message, bool, error)) error {
	if current == nil {
		return fmt.Errorf("%s: %w; VPP has no getter to check the required value", name, ErrNotGlobalsOwner)
	}
	cur, ok, err := current(ctx)
	if err != nil {
		return fmt.Errorf("%s: %w", name, err)
	}
	if !ok || !proto.Equal(cur, desired) {
		return fmt.Errorf("%s: %w; required %v, VPP has %v", name, ErrNotGlobalsOwner, desired, cur)
	}
	return nil
}

// NonOwnerRetrieve is Retrieve for a non-owner: write-only (see Globals).
func (g Globals) NonOwnerRetrieve(name string) ([]scheduler.KV, error) {
	return nil, fmt.Errorf("%s (not the globals owner, requirement only): %w", name, ErrRetrieveUnsupported)
}
