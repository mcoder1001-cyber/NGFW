package abf

import (
	"context"
	"errors"
	"testing"

	"go.fd.io/govpp/api"
	"google.golang.org/protobuf/proto"

	abfapi "ngfw/agent/binapi/abf"
	"ngfw/agent/binapi/acl"
	"ngfw/agent/binapi/fib_types"
	interfaces "ngfw/agent/binapi/interface"
	"ngfw/agent/binapi/ip_types"
	"ngfw/agent/binapi/memclnt"
	"ngfw/agent/internal/descriptors/df2"
	"ngfw/agent/internal/scheduler"
	"ngfw/agent/internal/vpp/fake"
)

type fakeVPP struct {
	*fake.Client
	policies map[uint32]abfapi.AbfPolicy
	attaches []abfapi.AbfItfAttach
}

func newFakeVPP() *fakeVPP {
	v := &fakeVPP{Client: fake.New(fake.WithControlPingReply(&memclnt.ControlPingReply{})), policies: map[uint32]abfapi.AbfPolicy{}}
	v.Reply("sw_interface_dump",
		&interfaces.SwInterfaceDetails{SwIfIndex: 0, InterfaceName: "local0"},
		&interfaces.SwInterfaceDetails{SwIfIndex: 5, InterfaceName: "loop300", Tag: "w3:loop300"},
		&interfaces.SwInterfaceDetails{SwIfIndex: 6, InterfaceName: "loop301", Tag: "w3:loop301"},
		&interfaces.SwInterfaceDetails{SwIfIndex: 7, InterfaceName: "loop200", Tag: "w2:loop200"},
	)
	v.Reply("acl_dump",
		&acl.ACLDetails{ACLIndex: 1, Tag: "w2:web"},
		&acl.ACLDetails{ACLIndex: 4, Tag: "w3:web"},
		&acl.ACLDetails{ACLIndex: 9, Tag: "untagged"},
	)
	v.On("abf_policy_add_del", func(req api.Message) ([]api.Message, error) {
		r := req.(*abfapi.AbfPolicyAddDel)
		if cur, ok := v.policies[r.Policy.PolicyID]; ok && cur.ACLIndex != r.Policy.ACLIndex {
			return []api.Message{&abfapi.AbfPolicyAddDelReply{Retval: -1}}, nil // INVALID_VALUE: acl change
		}
		// abf_policy_add_del is additive: is_add appends the given paths, !is_add removes
		// them and an empty list deletes the policy (verified on vrx-a). Paths are kept in
		// reverse order to model VPP's own ordering.
		cur, exists := v.policies[r.Policy.PolicyID]
		if !exists {
			if !r.IsAdd {
				return []api.Message{&abfapi.AbfPolicyAddDelReply{Retval: -6}}, nil
			}
			cur = abfapi.AbfPolicy{PolicyID: r.Policy.PolicyID, ACLIndex: r.Policy.ACLIndex}
		}
		if r.IsAdd {
			for i := len(r.Policy.Paths) - 1; i >= 0; i-- {
				cur.Paths = append([]fib_types.FibPath{r.Policy.Paths[i]}, cur.Paths...)
			}
		} else {
			var keep []fib_types.FibPath
			for _, p := range cur.Paths {
				found := false
				for _, q := range r.Policy.Paths {
					if p == q {
						found = true
					}
				}
				if !found {
					keep = append(keep, p)
				}
			}
			cur.Paths = keep
		}
		if len(cur.Paths) == 0 {
			delete(v.policies, r.Policy.PolicyID)
		} else {
			v.policies[r.Policy.PolicyID] = cur
		}
		return []api.Message{&abfapi.AbfPolicyAddDelReply{}}, nil
	})
	v.On("abf_policy_dump", func(api.Message) ([]api.Message, error) {
		var out []api.Message
		for id := uint32(0); id < 5000; id++ {
			if p, ok := v.policies[id]; ok {
				out = append(out, &abfapi.AbfPolicyDetails{Policy: p})
			}
		}
		return out, nil
	})
	v.On("abf_itf_attach_add_del", func(req api.Message) ([]api.Message, error) {
		r := req.(*abfapi.AbfItfAttachAddDel)
		if _, ok := v.policies[r.Attach.PolicyID]; !ok {
			return []api.Message{&abfapi.AbfItfAttachAddDelReply{Retval: -6}}, nil
		}
		for i, a := range v.attaches {
			if a.PolicyID == r.Attach.PolicyID && a.SwIfIndex == r.Attach.SwIfIndex && a.IsIPv6 == r.Attach.IsIPv6 {
				if !r.IsAdd {
					v.attaches = append(v.attaches[:i], v.attaches[i+1:]...)
				}
				return []api.Message{&abfapi.AbfItfAttachAddDelReply{}}, nil
			}
		}
		if r.IsAdd {
			v.attaches = append(v.attaches, r.Attach)
		}
		return []api.Message{&abfapi.AbfItfAttachAddDelReply{}}, nil
	})
	v.On("abf_itf_attach_dump", func(api.Message) ([]api.Message, error) {
		out := make([]api.Message, 0, len(v.attaches))
		for _, a := range v.attaches {
			out = append(out, &abfapi.AbfItfAttachDetails{Attach: a})
		}
		return out, nil
	})
	return v
}

var ids = &df2.IDRange{Lo: 3000, Hi: 3999}

func TestPolicyLifecycle(t *testing.T) {
	ctx := context.Background()
	v := newFakeVPP()
	// Foreign policies: another worker's id/ACL and an in-range id whose ACL is not ours.
	v.policies[2001] = abfapi.AbfPolicy{PolicyID: 2001, ACLIndex: 1}
	v.policies[3999] = abfapi.AbfPolicy{PolicyID: 3999, ACLIndex: 9}
	d := NewPolicy(v, "w3", ids)
	if !scheduler.ValidName(d.Name()) {
		t.Fatal(d.Name())
	}
	desired := &Policy{PolicyId: 3001, Acl: "web", Paths: []*df2.FibPath{
		{NextHop: "10.3.1.254", Interface: "loop301", Weight: 2},
		{NextHop: "10.3.1.253", Interface: "loop300"},
		{Type: df2.FibPath_DROP},
		{NextHop: "2001:DB8:3::1", TableId: 3002},
	}}
	if k := d.KeyOf(desired); k != "abf.policy/3001" {
		t.Fatalf("KeyOf = %s", k)
	}
	deps := d.Dependencies(desired)
	if len(deps) != 3 || deps[0].Key != "acl/web" || deps[0].Optional || deps[1].Key != "interface/loop301" || !deps[1].Optional || deps[2].Key != "interface/loop300" {
		t.Fatalf("Dependencies = %+v", deps)
	}
	norm, err := NormalizePolicy(desired)
	if err != nil {
		t.Fatal(err)
	}
	// Normalised: weight 0 → 1, proto derived, canonical v6 text, sorted by type/proto/nh.
	if len(norm.Paths) != 4 || norm.Paths[0].NextHop != "10.3.1.253" || norm.Paths[0].Weight != 1 || norm.Paths[1].NextHop != "10.3.1.254" ||
		norm.Paths[2].NextHop != "2001:db8:3::1" || norm.Paths[2].Proto != df2.FibPath_IP6 || norm.Paths[3].Type != df2.FibPath_DROP {
		t.Fatalf("Normalize = %+v", norm.Paths)
	}

	meta, err := d.Create(ctx, desired)
	if err != nil || meta != (PolicyMeta{ACLIndex: 4}) {
		t.Fatalf("Create: %v %+v", err, meta)
	}
	req := v.CallsNamed("abf_policy_add_del")[0].(*abfapi.AbfPolicyAddDel)
	if !req.IsAdd || req.Policy.PolicyID != 3001 || req.Policy.ACLIndex != 4 || req.Policy.NPaths != 4 || len(req.Policy.Paths) != 4 {
		t.Fatalf("request = %+v", req)
	}
	p0 := req.Policy.Paths[0]
	if p0.SwIfIndex != 5 || p0.Weight != 1 || p0.Proto != fib_types.FIB_API_PATH_NH_PROTO_IP4 || p0.Nh.Address.GetIP4() != (ip_types.IP4Address{10, 3, 1, 253}) {
		t.Fatalf("path[0] = %+v", p0)
	}
	if p2 := req.Policy.Paths[2]; p2.SwIfIndex != ^uint32(0) || p2.TableID != 3002 || p2.Proto != fib_types.FIB_API_PATH_NH_PROTO_IP6 {
		t.Fatalf("path[2] = %+v", p2)
	}
	if p3 := req.Policy.Paths[3]; p3.Type != fib_types.FIB_API_PATH_TYPE_DROP || p3.SwIfIndex != ^uint32(0) {
		t.Fatalf("path[3] = %+v", p3)
	}

	// Retrieve: only ours, equal to the normalised desired state despite VPP's path order.
	actual, err := d.Retrieve(ctx)
	if err != nil || len(actual) != 1 {
		t.Fatalf("Retrieve = %+v, %v", actual, err)
	}
	if actual[0].Key != "abf.policy/3001" || !proto.Equal(actual[0].Value, norm) || actual[0].Meta != meta {
		t.Fatalf("Retrieve = %+v\nwant %+v", actual[0].Value, norm)
	}

	// Update: add the new paths, then remove the old ones (additive API); ACL change → recreate.
	updated := &Policy{PolicyId: 3001, Acl: "web", Paths: []*df2.FibPath{{Interface: "loop300"}, {NextHop: "10.3.1.253", Interface: "loop300"}}}
	if _, err := d.Update(ctx, desired, updated, meta); err != nil {
		t.Fatal(err)
	}
	upd := v.CallsNamed("abf_policy_add_del") // [0] = Create
	if len(upd) != 3 || !upd[1].(*abfapi.AbfPolicyAddDel).IsAdd || len(upd[1].(*abfapi.AbfPolicyAddDel).Policy.Paths) != 1 ||
		upd[2].(*abfapi.AbfPolicyAddDel).IsAdd || len(upd[2].(*abfapi.AbfPolicyAddDel).Policy.Paths) != 3 {
		t.Fatalf("expected add(1 new) then del(3 old), got %+v", upd[1:])
	}
	actual, _ = d.Retrieve(ctx)
	wantUpd, _ := NormalizePolicy(updated)
	if got := actual[0].Value.(*Policy); !proto.Equal(got, wantUpd) {
		t.Fatalf("after Update = %+v, want %+v", got, wantUpd)
	}
	// A no-op update sends nothing.
	if _, err := d.Update(ctx, updated, updated, meta); err != nil || len(v.CallsNamed("abf_policy_add_del")) != 3 {
		t.Fatalf("no-op update: %v, calls %d", err, len(v.CallsNamed("abf_policy_add_del")))
	}
	if _, err := d.Update(ctx, desired, &Policy{PolicyId: 3001, Acl: "other", Paths: updated.Paths}, meta); !errors.Is(err, scheduler.ErrRecreate) {
		t.Fatalf("acl change: %v", err)
	}
	// Delete removes what VPP holds, even when the desired object drifted (stale path list).
	if err := d.Delete(ctx, desired, meta); err != nil {
		t.Fatal(err)
	}
	if actual, _ = d.Retrieve(ctx); len(actual) != 0 || len(v.policies) != 2 {
		t.Fatalf("after Delete = %+v policies=%d", actual, len(v.policies))
	}

	// Errors.
	if _, err := d.Create(ctx, &Policy{PolicyId: 1, Acl: "web", Paths: updated.Paths}); err == nil {
		t.Fatal("id outside range accepted")
	}
	if _, err := d.Create(ctx, &Policy{PolicyId: 3001, Acl: "web"}); err == nil {
		t.Fatal("no paths accepted")
	}
	if _, err := d.Create(ctx, &Policy{PolicyId: 3001, Acl: "nope", Paths: updated.Paths}); err == nil {
		t.Fatal("unknown ACL accepted")
	}
	if _, err := d.Create(ctx, &Policy{PolicyId: 3001, Acl: "web", Paths: []*df2.FibPath{{Interface: "nope"}}}); !errors.Is(err, df2.ErrNoSuchInterface) {
		t.Fatalf("unknown interface: %v", err)
	}
	if _, err := d.Create(ctx, &Policy{PolicyId: 3001, Acl: "web", Paths: []*df2.FibPath{{NextHop: "bad"}}}); err == nil {
		t.Fatal("bad next hop accepted")
	}
	if err := d.Delete(ctx, updated, nil); !errors.Is(err, df2.ErrBadMeta) {
		t.Fatalf("bad meta: %v", err)
	}
}

func TestAttachLifecycle(t *testing.T) {
	ctx := context.Background()
	v := newFakeVPP()
	v.policies[3001] = abfapi.AbfPolicy{PolicyID: 3001, ACLIndex: 4}
	v.policies[2001] = abfapi.AbfPolicy{PolicyID: 2001, ACLIndex: 1}
	v.attaches = []abfapi.AbfItfAttach{{PolicyID: 2001, SwIfIndex: 7, Priority: 1}, {PolicyID: 3001, SwIfIndex: 7, Priority: 1}}
	d := NewAttach(v, "w3", ids)
	if !scheduler.ValidName(d.Name()) {
		t.Fatal(d.Name())
	}
	desired := &Attach{PolicyId: 3001, Interface: "loop300", Priority: 10}
	if k := d.KeyOf(desired); k != "abf.attach/3001/loop300/ipv4" {
		t.Fatalf("KeyOf = %s", k)
	}
	if k := d.KeyOf(&Attach{PolicyId: 3001, Interface: "loop300", Ipv6: true}); k != "abf.attach/3001/loop300/ipv6" {
		t.Fatalf("KeyOf = %s", k)
	}
	if deps := d.Dependencies(desired); len(deps) != 2 || deps[0].Key != "abf.policy/3001" || deps[1].Key != "interface/loop300" {
		t.Fatalf("Dependencies = %+v", deps)
	}
	meta, err := d.Create(ctx, desired)
	if err != nil || meta != (AttachMeta{SwIfIndex: 5}) {
		t.Fatalf("Create: %v %+v", err, meta)
	}
	req := v.CallsNamed("abf_itf_attach_add_del")[0].(*abfapi.AbfItfAttachAddDel)
	if !req.IsAdd || req.Attach != (abfapi.AbfItfAttach{PolicyID: 3001, SwIfIndex: 5, Priority: 10}) {
		t.Fatalf("request = %+v", req)
	}
	actual, err := d.Retrieve(ctx)
	if err != nil || len(actual) != 1 || !proto.Equal(actual[0].Value, desired) || actual[0].Meta != meta {
		t.Fatalf("Retrieve = %+v, %v", actual, err)
	}
	if _, err := d.Update(ctx, desired, &Attach{PolicyId: 3001, Interface: "loop300", Priority: 20}, meta); !errors.Is(err, scheduler.ErrRecreate) {
		t.Fatalf("Update: %v", err)
	}
	if err := d.Delete(ctx, desired, meta); err != nil {
		t.Fatal(err)
	}
	if actual, _ = d.Retrieve(ctx); len(actual) != 0 || len(v.attaches) != 2 {
		t.Fatalf("after Delete = %+v attaches=%+v", actual, v.attaches)
	}
	if _, err := d.Create(ctx, &Attach{PolicyId: 3005, Interface: "loop300"}); err == nil {
		t.Fatal("attach to a missing policy must surface the retval")
	}
	if _, err := d.Create(ctx, &Attach{PolicyId: 1, Interface: "loop300"}); err == nil {
		t.Fatal("id outside range accepted")
	}
	if err := d.Delete(ctx, desired, nil); !errors.Is(err, df2.ErrBadMeta) {
		t.Fatalf("bad meta: %v", err)
	}
}

func TestRegister(t *testing.T) {
	reg := scheduler.NewRegistry()
	Register(reg, fake.New(), "w3", ids)
	if got := reg.Names(); len(got) != 2 || got[0] != PolicyName || got[1] != AttachName {
		t.Fatalf("Names = %v", got)
	}
}
