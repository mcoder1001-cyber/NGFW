package sr_mpls_test

import (
	"context"
	"errors"
	"testing"

	"go.fd.io/govpp/api"
	"google.golang.org/protobuf/proto"

	"ngfw/agent/binapi/fib"
	"ngfw/agent/binapi/fib_types"
	"ngfw/agent/binapi/ip"
	"ngfw/agent/binapi/ip_types"
	"ngfw/agent/binapi/mpls"
	srmplsapi "ngfw/agent/binapi/sr_mpls"
	"ngfw/agent/internal/descriptors/df6"
	"ngfw/agent/internal/descriptors/df6/df6test"
	iface "ngfw/agent/internal/descriptors/interface"
	"ngfw/agent/internal/descriptors/sr_mpls"
	"ngfw/agent/internal/scheduler"
)

type fakeSRMPLS struct {
	*df6test.FakeVPP
	tables   map[[2]uint32]bool
	policies map[uint32][][]uint32
	steer    map[[2]string]*srmplsapi.SrMplsSteeringAddDel // {table, prefix}
	ec       map[uint32]uint32
	crashes  int
}

func newFake() *fakeSRMPLS {
	f := &fakeSRMPLS{FakeVPP: df6test.NewFakeVPP(), tables: map[[2]uint32]bool{}, policies: map[uint32][][]uint32{}, steer: map[[2]string]*srmplsapi.SrMplsSteeringAddDel{}, ec: map[uint32]uint32{}}
	has := func(id uint32, ip6 bool) bool {
		v := uint32(0)
		if ip6 {
			v = 1
		}
		return id == 0 || f.tables[[2]uint32{id, v}]
	}
	f.On("ip_table_dump", func(api.Message) ([]api.Message, error) {
		var out []api.Message
		for k := range f.tables {
			out = append(out, &ip.IPTableDetails{Table: ip.IPTable{TableID: k[0], IsIP6: k[1] == 1}})
		}
		return out, nil
	})
	f.On("sr_mpls_policy_add", func(req api.Message) ([]api.Message, error) {
		r := req.(*srmplsapi.SrMplsPolicyAdd)
		if len(r.Segments) == 0 {
			f.crashes++
		}
		if _, ok := f.policies[r.Bsid]; ok {
			return []api.Message{&srmplsapi.SrMplsPolicyAddReply{Retval: -12}}, nil
		}
		f.policies[r.Bsid] = [][]uint32{r.Segments}
		return []api.Message{&srmplsapi.SrMplsPolicyAddReply{}}, nil
	})
	f.On("sr_mpls_policy_mod", func(req api.Message) ([]api.Message, error) {
		r := req.(*srmplsapi.SrMplsPolicyMod)
		if _, ok := f.policies[r.Bsid]; !ok {
			return []api.Message{&srmplsapi.SrMplsPolicyModReply{Retval: -1}}, nil
		}
		f.policies[r.Bsid] = append(f.policies[r.Bsid], r.Segments)
		return []api.Message{&srmplsapi.SrMplsPolicyModReply{}}, nil
	})
	f.On("sr_mpls_policy_del", func(req api.Message) ([]api.Message, error) {
		r := req.(*srmplsapi.SrMplsPolicyDel)
		if _, ok := f.policies[r.Bsid]; !ok {
			return []api.Message{&srmplsapi.SrMplsPolicyDelReply{Retval: -1}}, nil
		}
		delete(f.policies, r.Bsid)
		delete(f.ec, r.Bsid)
		return []api.Message{&srmplsapi.SrMplsPolicyDelReply{}}, nil
	})
	f.On("mpls_route_dump", func(api.Message) ([]api.Message, error) {
		var out []api.Message
		for b := range f.policies {
			for eos := uint8(0); eos < 2; eos++ {
				out = append(out, &mpls.MplsRouteDetails{MrRoute: mpls.MplsRoute{MrLabel: b, MrEos: eos, MrPaths: []fib_types.FibPath{
					{SwIfIndex: ^uint32(0), Proto: fib_types.FIB_API_PATH_NH_PROTO_MPLS, Type: fib_types.FIB_API_PATH_TYPE_NORMAL},
				}}})
			}
		}
		return out, nil
	})
	f.On("sr_mpls_steering_add_del", func(req api.Message) ([]api.Message, error) {
		r := req.(*srmplsapi.SrMplsSteeringAddDel)
		ip6 := r.Prefix.Address.Af == ip_types.ADDRESS_IP6
		if !has(r.TableID, ip6) {
			f.crashes++
			return []api.Message{&srmplsapi.SrMplsSteeringAddDelReply{Retval: -1}}, nil
		}
		k := [2]string{df6.U32(r.TableID), df6.PrefixString(r.Prefix)}
		if r.IsDel {
			if _, ok := f.steer[k]; !ok {
				return []api.Message{&srmplsapi.SrMplsSteeringAddDelReply{Retval: -1}}, nil
			}
			delete(f.steer, k)
			return []api.Message{&srmplsapi.SrMplsSteeringAddDelReply{}}, nil
		}
		if _, ok := f.policies[r.Bsid]; !ok {
			f.crashes++ // VPP leaves a half-created steering entry
			return []api.Message{&srmplsapi.SrMplsSteeringAddDelReply{Retval: -1}}, nil
		}
		if _, dup := f.steer[k]; dup {
			return []api.Message{&srmplsapi.SrMplsSteeringAddDelReply{Retval: -1}}, nil // VPP refuses re-adding BSID steering
		}
		f.steer[k] = r
		return []api.Message{&srmplsapi.SrMplsSteeringAddDelReply{}}, nil
	})
	f.On("ip_route_v2_dump", func(req api.Message) ([]api.Message, error) {
		r := req.(*ip.IPRouteV2Dump)
		var out []api.Message
		for _, s := range f.steer {
			if s.TableID == r.Table.TableID && (r.Src == 0 || r.Src == 5) {
				p := fib_types.FibPath{Proto: fib_types.FIB_API_PATH_NH_PROTO_MPLS, SwIfIndex: ^uint32(0)}
				if s.VPNLabel != ^uint32(0) {
					p.NLabels = 1
					p.LabelStack[0].Label = s.VPNLabel
				}
				out = append(out, &ip.IPRouteV2Details{Route: ip.IPRouteV2{TableID: s.TableID, Prefix: s.Prefix, Paths: []fib_types.FibPath{p}}})
			}
		}
		return out, nil
	})
	f.Reply("fib_source_dump", &fib.FibSourceDetails{Src: fib.FibSource{ID: 5, Name: "SR"}})
	f.On("sr_mpls_policy_assign_endpoint_color", func(req api.Message) ([]api.Message, error) {
		r := req.(*srmplsapi.SrMplsPolicyAssignEndpointColor)
		if _, ok := f.policies[r.Bsid]; !ok {
			return []api.Message{&srmplsapi.SrMplsPolicyAssignEndpointColorReply{Retval: -1}}, nil
		}
		f.ec[r.Bsid] = r.Color
		return []api.Message{&srmplsapi.SrMplsPolicyAssignEndpointColorReply{}}, nil
	})
	return f
}

func TestPolicySteeringEndpointColor(t *testing.T) {
	ctx := context.Background()
	f := newFake()
	f.tables[[2]uint32{11012, 0}] = true
	reg := scheduler.NewRegistry()
	owner := "w11" + t.Name()
	iface.SetClaimStore(owner, nil)
	sr_mpls.Register(reg, f, owner)
	if reg.Len() != 3 {
		t.Fatalf("registered %d", reg.Len())
	}
	p := sr_mpls.NewPolicy(f, owner)
	s := sr_mpls.NewSteering(f, owner)
	ec := sr_mpls.NewEndpointColor(f, owner)

	pol := &sr_mpls.Policy{Bsid: 11600, SegmentLists: []*sr_mpls.SegmentList{
		{Labels: []uint32{11700, 11701}, Weight: 1},
		{Labels: []uint32{11702}, Weight: 2},
	}}
	if k := p.KeyOf(pol); k != "sr-mpls.policy/11600" {
		t.Fatalf("KeyOf = %s", k)
	}
	if deps := p.Dependencies(pol); len(deps) != 1 || deps[0].Key != "mpls-table/0" {
		t.Fatalf("deps = %+v", deps)
	}
	st := &sr_mpls.Steering{Prefix: "10.11.13.9/24", TableId: 11012, Bsid: 11600, VpnLabel: 11800}
	if k := s.KeyOf(st); k != "sr-mpls.steering/11012/10.11.13.0/24" {
		t.Fatalf("steering KeyOf = %s", k)
	}
	if deps := s.Dependencies(st); len(deps) != 2 || deps[0].Key != "sr-mpls.policy/11600" || deps[1].Key != "vrf/11012" {
		t.Fatalf("steering deps = %+v", deps)
	}
	// Steering before its policy: refused without reaching VPP.
	if _, err := s.Create(ctx, st); !errors.Is(err, sr_mpls.ErrNoSuchPolicy) {
		t.Fatalf("steering without policy = %v", err)
	}
	if _, err := p.Create(ctx, pol); err != nil {
		t.Fatal(err)
	}
	if got := f.policies[11600]; len(got) != 2 || got[1][0] != 11702 {
		t.Fatalf("policy in VPP = %v", got)
	}
	if _, err := s.Create(ctx, st); err != nil {
		t.Fatal(err)
	}
	req := f.CallsNamed("sr_mpls_steering_add_del")[0].(*srmplsapi.SrMplsSteeringAddDel)
	if req.VPNLabel != 11800 || req.Color != ^uint32(0) || req.MaskWidth != 24 || df6.PrefixString(req.Prefix) != "10.11.13.0/24" {
		t.Fatalf("steering request = %+v", req)
	}
	if _, err := s.Create(ctx, &sr_mpls.Steering{Prefix: "10.11.14.0/24", TableId: 11999, Bsid: 11600}); !errors.Is(err, df6.ErrNoSuchTable) {
		t.Fatalf("missing table = %v", err)
	}
	e := &sr_mpls.EndpointColor{Bsid: 11600, Endpoint: "10.11.0.9", Color: 7}
	if _, err := ec.Create(ctx, e); err != nil || f.ec[11600] != 7 {
		t.Fatalf("endpoint color: %v", err)
	}
	if _, err := ec.Update(ctx, e, &sr_mpls.EndpointColor{Bsid: 11600, Endpoint: "10.11.0.9", Color: 8}, nil); err != nil || f.ec[11600] != 8 {
		t.Fatalf("endpoint color update: %v", err)
	}
	// review M4: documented no-op (VPP clears the assignment with the policy), never blocks
	// the policy's deletion
	if err := ec.Delete(ctx, e, nil); err != nil {
		t.Fatalf("endpoint color delete with policy = %v", err)
	}
	for _, d := range []scheduler.Descriptor{p, s, ec} {
		if _, err := d.Retrieve(ctx); !errors.Is(err, df6.ErrRetrieveUnsupported) {
			t.Fatalf("%s Retrieve = %v", d.Name(), err)
		}
		if _, err := d.Update(ctx, pol, pol, nil); d != ec && !errors.Is(err, scheduler.ErrRecreate) {
			t.Fatalf("%s Update = %v", d.Name(), err)
		}
	}
	// Delete (dependents first), twice each.
	for i := 0; i < 2; i++ {
		if err := s.Delete(ctx, st, nil); err != nil {
			t.Fatal(err)
		}
	}
	for i := 0; i < 2; i++ {
		if err := p.Delete(ctx, pol, nil); err != nil {
			t.Fatal(err)
		}
	}
	if err := ec.Delete(ctx, e, nil); err != nil {
		t.Fatalf("endpoint color delete after policy = %v", err)
	}
	if len(f.policies) != 0 || len(f.steer) != 0 || f.crashes != 0 {
		t.Fatalf("leftovers %v %v or crashes %d", f.policies, f.steer, f.crashes)
	}
	for _, b := range []*sr_mpls.Policy{
		{Bsid: 5, SegmentLists: []*sr_mpls.SegmentList{{Labels: []uint32{11700}, Weight: 1}}},
		{Bsid: 11600},
		{Bsid: 11600, SegmentLists: []*sr_mpls.SegmentList{{Weight: 1}}},
		{Bsid: 11600, SegmentLists: []*sr_mpls.SegmentList{{Labels: []uint32{11700}}}},
		{Bsid: 11600, SegmentLists: []*sr_mpls.SegmentList{{Labels: []uint32{11702}, Weight: 1}, {Labels: []uint32{11700}, Weight: 1}}},
		{Bsid: 11600, SegmentLists: []*sr_mpls.SegmentList{{Labels: []uint32{1 << 20}, Weight: 1}}},
	} {
		if _, err := p.Create(ctx, b); !errors.Is(err, df6.ErrBadValue) {
			t.Errorf("%v: %v", b, err)
		}
	}
	for _, b := range []*sr_mpls.EndpointColor{{Bsid: 11600, Color: 1}, {Bsid: 11600, Endpoint: "0.0.0.0", Color: 1}, {Bsid: 11600, Endpoint: "10.0.0.1", Color: ^uint32(0)}} {
		if _, err := ec.Create(ctx, b); !errors.Is(err, df6.ErrBadValue) {
			t.Errorf("%v: %v", b, err)
		}
	}
}

// TestResync (review H3/D-076): write-only SR-MPLS objects are re-applied on every resync
// without a second add (VPP refuses duplicate adds); after a VPP restart (boot id change) the
// endpoint/color is re-assigned exactly once; objects of other owners are never taken over.
func TestResync(t *testing.T) {
	ctx := context.Background()
	f := newFake()
	f.tables[[2]uint32{11012, 0}] = true
	owner := "w11" + t.Name()
	iface.SetClaimStore(owner, nil)
	iface.SetClaimStore(owner+"x", nil)
	p := sr_mpls.NewPolicy(f, owner)
	s := sr_mpls.NewSteering(f, owner)
	ec := sr_mpls.NewEndpointColor(f, owner)
	pol := &sr_mpls.Policy{Bsid: 11600, SegmentLists: []*sr_mpls.SegmentList{{Labels: []uint32{11700}, Weight: 1}}}
	st := &sr_mpls.Steering{Prefix: "10.11.13.0/24", TableId: 11012, Bsid: 11600}
	e := &sr_mpls.EndpointColor{Bsid: 11600, Endpoint: "10.11.0.9", Color: 7}
	f.SetBoot(1)
	for i := 0; i < 3; i++ {
		for _, x := range []struct {
			d interface {
				Create(context.Context, proto.Message) (any, error)
			}
			o proto.Message
		}{{p, pol}, {s, st}, {ec, e}} {
			if _, err := x.d.Create(ctx, x.o); err != nil {
				t.Fatalf("apply %d: %v", i, err)
			}
		}
	}
	for name, want := range map[string]int{"sr_mpls_policy_add": 1, "sr_mpls_steering_add_del": 1, "sr_mpls_policy_assign_endpoint_color": 1} {
		if n := len(f.CallsNamed(name)); n != want {
			t.Errorf("%s sent %d times, want %d", name, n, want)
		}
	}
	if f.crashes != 0 {
		t.Fatalf("%d requests VPP would reject", f.crashes)
	}
	// VPP restart: VPP lost everything; every claim expired with the old identity (D-080).
	f.SetBoot(2)
	f.policies = map[uint32][][]uint32{}
	f.steer = map[[2]string]*srmplsapi.SrMplsSteeringAddDel{}
	for i := 0; i < 2; i++ {
		for _, x := range []struct {
			d interface {
				Create(context.Context, proto.Message) (any, error)
			}
			o proto.Message
		}{{p, pol}, {s, st}, {ec, e}} {
			if _, err := x.d.Create(ctx, x.o); err != nil {
				t.Fatalf("re-apply after VPP restart %d: %v", i, err)
			}
		}
	}
	for name, want := range map[string]int{"sr_mpls_policy_add": 2, "sr_mpls_steering_add_del": 2, "sr_mpls_policy_assign_endpoint_color": 2} {
		if n := len(f.CallsNamed(name)); n != want {
			t.Errorf("after VPP restart %s sent %d times in total, want %d", name, n, want)
		}
	}
	// policy lost and re-added on the SAME VPP boot (review N1): VPP cleared the
	// endpoint/color with it, so it is re-assigned exactly once.
	delete(f.policies, 11600)
	delete(f.steer, [2]string{"11012", "10.11.13.0/24"})
	for i := 0; i < 2; i++ {
		for _, x := range []struct {
			d interface {
				Create(context.Context, proto.Message) (any, error)
			}
			o proto.Message
		}{{p, pol}, {s, st}, {ec, e}} {
			if _, err := x.d.Create(ctx, x.o); err != nil {
				t.Fatalf("re-apply after loss %d: %v", i, err)
			}
		}
	}
	if n := len(f.CallsNamed("sr_mpls_policy_assign_endpoint_color")); n != 3 {
		t.Fatalf("endpoint/color after policy loss: %d assigns in total, want 3", n)
	}
	// another owner: no take-over, no delete
	po := sr_mpls.NewPolicy(f, owner+"x")
	if _, err := po.Create(ctx, pol); !errors.Is(err, df6.ErrNotOurs) {
		t.Fatalf("take-over = %v", err)
	}
	if err := po.Delete(ctx, pol, nil); err != nil || len(f.policies) != 1 {
		t.Fatalf("foreign delete: %v", err)
	}
	// ours: steering, then policy (endpoint-color no-op) — deletable after agent restart
	if err := sr_mpls.NewSteering(f, owner).Delete(ctx, st, nil); err != nil {
		t.Fatal(err)
	}
	if err := sr_mpls.NewEndpointColor(f, owner).Delete(ctx, e, nil); err != nil {
		t.Fatal(err)
	}
	if err := sr_mpls.NewPolicy(f, owner).Delete(ctx, pol, nil); err != nil || len(f.policies) != 0 {
		t.Fatalf("policy delete: %v", err)
	}
}
