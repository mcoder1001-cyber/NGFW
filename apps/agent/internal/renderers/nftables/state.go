package nftables

import (
	"context"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	vrxv1 "ngfw/agent/gen/vrx/v1"
)

// Runtime is the host firewall of one running agent: its renderer and descriptor (for HostAclState).
type Runtime struct {
	Owner      string
	Descriptor *Descriptor
}

var runtimes sync.Map // stateDir + "\x00" + owner → *Runtime

func runtimeKey(stateDir, owner string) string { return filepath.Clean(stateDir) + "\x00" + owner }

// Register makes rt the runtime of stateDir and owner (subsystems wiring).
func Register(stateDir string, rt *Runtime) { runtimes.Store(runtimeKey(stateDir, rt.Owner), rt) }

// RuntimeFor returns the runtime registered for stateDir and owner (nil if none).
func RuntimeFor(stateDir, owner string) *Runtime {
	if v, ok := runtimes.Load(runtimeKey(stateDir, owner)); ok {
		return v.(*Runtime)
	}
	return nil
}

// State is the HostAclState answer: the kernel table (mode check: the stored rendering, never loaded)
// annotated with the stored value, per-rule counters and the in-sync flag.
func (rt *Runtime) State(ctx context.Context, now time.Time) (*vrxv1.HostAclStateResponse, error) {
	d := rt.Descriptor
	p := d.r.paths
	resp := &vrxv1.HostAclStateResponse{Owner: rt.Owner, RetrievedAt: timestamppb.New(now), Table: p.Table, Mode: p.Mode}
	stored, err := d.st.Load()
	if err != nil {
		return nil, err
	}
	if p.Mode == ModeCheck {
		resp.InSync = true
		fill(resp, stored, nil)
		return resp, nil
	}
	v, k, err := d.actual(ctx)
	if err != nil {
		return nil, err
	}
	resp.Present = k != nil
	want := stored
	if want == nil {
		want = &HostTable{}
	}
	got := v
	if got == nil {
		got = &HostTable{}
	}
	resp.InSync = proto.Equal(stripConfig(got), stripConfig(want))
	fill(resp, v, k)
	return resp, nil
}

func stripConfig(v *HostTable) *HostTable {
	c := proto.Clone(v).(*HostTable)
	c.Config = nil
	return c
}

// fill copies sets and chains of v into resp, with the counters of k (same chain and rule order).
func fill(resp *vrxv1.HostAclStateResponse, v *HostTable, k *KernelTable) {
	for _, s := range v.GetSets() {
		resp.Sets = append(resp.Sets, &vrxv1.HostAclSetState{Name: s.GetName(), Type: s.GetType(), Object: objectOfSet(s.GetName()), Elements: s.GetElements()})
	}
	for ci, c := range v.GetChains() {
		cs := &vrxv1.HostAclChainState{Name: c.GetName(), Hook: c.GetHook(), Priority: c.GetPriority(), Policy: c.GetPolicy(), List: listOfChain(c.GetName())}
		for ri, r := range c.GetRules() {
			rs := &vrxv1.HostAclRuleState{Kind: r.GetKind(), List: r.GetList(), Sequence: r.GetSequence(), Pointer: r.GetPointer(), Text: r.GetText(), Verdict: r.GetVerdict(), Comment: r.GetComment()}
			if k != nil && ci < len(k.Chains) && ri < len(k.Chains[ci].Rules) {
				kr := k.Chains[ci].Rules[ri]
				rs.Packets, rs.Bytes = kr.Packets, kr.Bytes
			}
			cs.Rules = append(cs.Rules, rs)
		}
		resp.Chains = append(resp.Chains, cs)
	}
}

func objectOfSet(name string) string {
	if strings.HasPrefix(name, "a4_") || strings.HasPrefix(name, "a6_") {
		return name[3:]
	}
	return ""
}

func listOfChain(name string) string {
	for _, p := range []string{"in_", "out_", "fwd_"} {
		if strings.HasPrefix(name, p) {
			return name[len(p):]
		}
	}
	return ""
}
