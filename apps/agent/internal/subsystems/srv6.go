package subsystems

// F-srv6 wiring: DF-6's sr family (local SIDs, policies, steering; the encap source / hop limit
// globals) registered with the persisted df6 claim store (TD-11b: df6.WithClaims(PairClaims("df6")))
// and the D-071 globals flag, the Srv6State RPC's read path, and the SRv6 assembler's environment.
//
// State the projection and the RPC need is kept per process (project()/assemble() carry no owner — the
// F-wireguard / F-rpf-adl-pbr pattern): the last registered family serves the assembler, the RPC looks
// its family up by owner. One product agent per process registers exactly one.

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"sync"

	"google.golang.org/protobuf/proto"

	"ngfw/agent/internal/descriptors/df6"
	"ngfw/agent/internal/descriptors/sr"
	"ngfw/agent/internal/desired"
	"ngfw/agent/internal/scheduler"
	"ngfw/agent/internal/vpp"
)

// Descriptor names of routing.srv6 in Domains[Routing] (subsystems.go, under the F-srv6 anchor; no sr
// import there).
const (
	srLocalSidName      = sr.LocalSidName
	srPolicyName        = sr.PolicyName
	srSteeringName      = sr.SteeringName
	srEncapSourceName   = sr.EncapSourceName
	srEncapHopLimitName = sr.EncapHopLimitName
)

// srv6Family is one agent's SRv6 family.
type srv6Family struct {
	owner    string
	client   vpp.Client
	localSid *sr.LocalSidDescriptor
	policy   *sr.PolicyDescriptor
	steering *sr.SteeringDescriptor

	mu     sync.Mutex
	source string // global encap source this process applied (globals owner; "" = none / reset)
}

var (
	srv6Mu      sync.Mutex
	srv6ByOwner = map[string]*srv6Family{}
	srv6Current *srv6Family
)

// ErrSrv6NotWired means the agent build has no SRv6 family for the owner (Srv6State).
var ErrSrv6NotWired = errors.New("srv6: the sr family is not registered for this owner")

// registerSrv6 registers sr.localsid, sr.policy, sr.steering and the two globals (setters only for
// the globals owner, D-071), all claims in the persisted df6 store (TD-11b).
func (w *Wiring) registerSrv6(r scheduler.Registry) error {
	pairs, err := w.PairClaims("df6")
	if err != nil {
		return fmt.Errorf("srv6: %w", err)
	}
	c, owner := w.env.Client, w.env.Owner
	opts := []df6.Option{df6.WithClaims(pairs), df6.WithGlobalsOwner(w.env.GlobalsOwner)}
	fam := &srv6Family{
		owner: owner, client: c,
		// read-only views over the same claim store for Srv6State (Retrieve = the claimed objects)
		localSid: sr.NewLocalSid(c, owner, opts...), policy: sr.NewPolicy(c, owner, opts...), steering: sr.NewSteering(c, owner, opts...),
	}
	sr.Register(srv6Registry{Registry: r, fam: fam}, c, owner, opts...)
	srv6Mu.Lock()
	srv6ByOwner[owner], srv6Current = fam, fam
	srv6Mu.Unlock()
	return nil
}

// srv6Registry records the global encap source the globals owner applies (the assembler's
// Srv6Env): VPP has no getter for it (write-only), and a policy that inherited it must not come back
// with an encapSource of its own.
type srv6Registry struct {
	scheduler.Registry
	fam *srv6Family
}

func (g srv6Registry) Register(d scheduler.Descriptor) {
	if gl, ok := d.(*sr.Global); ok && d.Name() == sr.EncapSourceName {
		d = &srv6SourceRecorder{Global: gl, fam: g.fam}
	}
	g.Registry.Register(d)
}

// srv6SourceRecorder is sr.encap-source that remembers what it applied (every other method is
// sr.Global's, the TD-11b declaration included).
type srv6SourceRecorder struct {
	*sr.Global
	fam *srv6Family
}

func (d *srv6SourceRecorder) record(obj proto.Message) {
	e, ok := obj.(*sr.EncapSource)
	if !ok {
		return
	}
	a, err := df6.ParseAddr6(e.GetAddress())
	if err != nil {
		return
	}
	d.fam.mu.Lock()
	d.fam.source = a.String()
	d.fam.mu.Unlock()
}

// Create implements scheduler.Descriptor.
func (d *srv6SourceRecorder) Create(ctx context.Context, obj proto.Message) (any, error) {
	m, err := d.Global.Create(ctx, obj)
	if err == nil {
		d.record(obj)
	}
	return m, err
}

// Update implements scheduler.Descriptor.
func (d *srv6SourceRecorder) Update(ctx context.Context, oldObj, newObj proto.Message, meta any) (any, error) {
	m, err := d.Global.Update(ctx, oldObj, newObj, meta)
	if err == nil {
		d.record(newObj)
	}
	return m, err
}

// Delete implements scheduler.Descriptor.
func (d *srv6SourceRecorder) Delete(ctx context.Context, obj proto.Message, meta any) error {
	err := d.Global.Delete(ctx, obj, meta)
	if err == nil {
		d.fam.mu.Lock()
		d.fam.source = ""
		d.fam.mu.Unlock()
	}
	return err
}

func (f *srv6Family) appliedSource() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.source
}

// Srv6Env is the SRv6 assembler's environment (projection.go): the global encap source the
// registered family applied, or none.
func Srv6Env() desired.Srv6Env {
	srv6Mu.Lock()
	fam := srv6Current
	srv6Mu.Unlock()
	if fam == nil {
		return desired.Srv6Env{}
	}
	return desired.Srv6Env{EncapSource: fam.appliedSource}
}

// Srv6LocalSidState is one claimed local SID with its counters.
type Srv6LocalSidState struct {
	*sr.LocalSid
	sr.Counters
}

// Srv6Snapshot is the live SRv6 state of one owner: the objects its own Creates claimed, as VPP has
// them (Srv6State).
type Srv6Snapshot struct {
	LocalSids []Srv6LocalSidState
	Policies  []*sr.Policy
	Steering  []*sr.Steering
}

// Srv6State dumps owner's claimed SRv6 objects (local SIDs with sr_localsids_with_packet_stats_dump
// counters, policies, steering). Read-only; the caller serialises walks (D-132).
func Srv6State(ctx context.Context, owner string) (*Srv6Snapshot, error) {
	srv6Mu.Lock()
	fam := srv6ByOwner[owner]
	srv6Mu.Unlock()
	if fam == nil {
		return nil, ErrSrv6NotWired
	}
	return fam.state(ctx)
}

func (f *srv6Family) state(ctx context.Context) (*Srv6Snapshot, error) {
	out := &Srv6Snapshot{}
	sids, err := f.localSid.Retrieve(ctx)
	if err != nil {
		return nil, err
	}
	var counters map[string]sr.Counters
	if len(sids) > 0 {
		if counters, err = sr.LocalSidCounters(ctx, f.client); err != nil {
			return nil, err
		}
	}
	for _, kv := range sids {
		if l, ok := kv.Value.(*sr.LocalSid); ok {
			out.LocalSids = append(out.LocalSids, Srv6LocalSidState{LocalSid: l, Counters: counters[l.GetSid()]})
		}
	}
	pols, err := f.policy.Retrieve(ctx)
	if err != nil {
		return nil, err
	}
	for _, kv := range pols {
		if p, ok := kv.Value.(*sr.Policy); ok {
			out.Policies = append(out.Policies, p)
		}
	}
	steer, err := f.steering.Retrieve(ctx)
	if err != nil {
		return nil, err
	}
	for _, kv := range steer {
		if s, ok := kv.Value.(*sr.Steering); ok {
			out.Steering = append(out.Steering, s)
		}
	}
	sort.Slice(out.LocalSids, func(i, j int) bool { return out.LocalSids[i].GetSid() < out.LocalSids[j].GetSid() })
	sort.Slice(out.Policies, func(i, j int) bool { return out.Policies[i].GetBsid() < out.Policies[j].GetBsid() })
	sort.Slice(out.Steering, func(i, j int) bool {
		a, b := out.Steering[i], out.Steering[j]
		if (a.GetTrafficType() == sr.SteerType_L2) != (b.GetTrafficType() == sr.SteerType_L2) {
			return b.GetTrafficType() == sr.SteerType_L2
		}
		if a.GetTrafficType() == sr.SteerType_L2 {
			return a.GetInterface() < b.GetInterface()
		}
		if a.GetTableId() != b.GetTableId() {
			return a.GetTableId() < b.GetTableId()
		}
		return a.GetPrefix() < b.GetPrefix()
	})
	return out, nil
}
