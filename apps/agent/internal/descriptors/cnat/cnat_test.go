package cnat_test

import (
	"context"
	"errors"
	"testing"

	"go.fd.io/govpp/api"
	"google.golang.org/protobuf/types/known/structpb"

	cnatapi "ngfw/agent/binapi/cnat"
	"ngfw/agent/binapi/feature"
	interfaces "ngfw/agent/binapi/interface"
	"ngfw/agent/binapi/interface_types"
	"ngfw/agent/binapi/ip_types"
	"ngfw/agent/binapi/memclnt"
	"ngfw/agent/binapi/vlib"
	"ngfw/agent/internal/descriptors/cnat"
	"ngfw/agent/internal/descriptors/natcommon"
	"ngfw/agent/internal/descriptors/natcommon/nattest"
	"ngfw/agent/internal/scheduler"
	"ngfw/agent/internal/vpp/fake"
)

type fakeCnat struct {
	*fake.Client
	t        *testing.T
	nextID   uint32
	trs      map[uint32]cnatapi.CnatTranslation
	snat     *cnatapi.CnatGetSnatAddressesReply
	policy   cnatapi.CnatSnatPolicies
	snatIfs  map[[2]uint32]bool
	excluded map[string]bool
	pfxRefs  map[string]int // VPP: every add bumps a per-prefix refcount (D-076)
	vppPID   uint32
	feat     map[uint32]bool
}

var noIf = ^interface_types.InterfaceIndex(0)

func newFakeCnat(t *testing.T) *fakeCnat {
	f := &fakeCnat{Client: fake.New(fake.WithControlPingReply(&memclnt.ControlPingReply{})), t: t,
		trs: map[uint32]cnatapi.CnatTranslation{}, snatIfs: map[[2]uint32]bool{}, excluded: map[string]bool{}, pfxRefs: map[string]int{}, feat: map[uint32]bool{}, vppPID: 4242}
	f.Reply("sw_interface_dump",
		&interfaces.SwInterfaceDetails{SwIfIndex: 0, InterfaceName: "local0"},
		&interfaces.SwInterfaceDetails{SwIfIndex: 1, InterfaceName: "loop940", Tag: "w9:loop940"},
		&interfaces.SwInterfaceDetails{SwIfIndex: 2, InterfaceName: "loop941", Tag: "w9:loop941"},
		&interfaces.SwInterfaceDetails{SwIfIndex: 3, InterfaceName: "loop340", Tag: "w3:loop340"})
	f.On("cnat_translation_update", func(req api.Message) ([]api.Message, error) {
		tr := req.(*cnatapi.CnatTranslationUpdate).Translation
		if tr.NPaths == 0 {
			t.Fatal("cnat_translation_update with n_paths=0 would crash VPP 26.06")
		}
		for id, cur := range f.trs { // same vip/port/proto → update in place
			if cur.Vip.Addr == tr.Vip.Addr && cur.Vip.Port == tr.Vip.Port && cur.IPProto == tr.IPProto {
				tr.ID = id
				f.trs[id] = details(tr)
				return []api.Message{&cnatapi.CnatTranslationUpdateReply{ID: id}}, nil
			}
		}
		tr.ID = f.nextID
		f.nextID++
		f.trs[tr.ID] = details(tr)
		return []api.Message{&cnatapi.CnatTranslationUpdateReply{ID: tr.ID}}, nil
	})
	f.On("cnat_translation_del", func(req api.Message) ([]api.Message, error) {
		id := req.(*cnatapi.CnatTranslationDel).ID
		if _, ok := f.trs[id]; !ok {
			return []api.Message{&cnatapi.CnatTranslationDelReply{Retval: int32(api.NO_SUCH_ENTRY)}}, nil
		}
		delete(f.trs, id)
		return []api.Message{&cnatapi.CnatTranslationDelReply{}}, nil
	})
	f.On("cnat_translation_dump", func(api.Message) ([]api.Message, error) {
		var out []api.Message
		for id := uint32(0); id < f.nextID; id++ {
			if tr, ok := f.trs[id]; ok {
				out = append(out, &cnatapi.CnatTranslationDetails{Translation: tr})
			}
		}
		return out, nil
	})
	f.On("cnat_get_snat_addresses", func(api.Message) ([]api.Message, error) {
		if f.snat == nil {
			return []api.Message{&cnatapi.CnatGetSnatAddressesReply{Retval: int32(api.FEATURE_DISABLED)}}, nil
		}
		r := *f.snat
		return []api.Message{&r}, nil
	})
	f.On("cnat_set_snat_addresses", func(req api.Message) ([]api.Message, error) {
		r := req.(*cnatapi.CnatSetSnatAddresses)
		if r.SnatIP4 == (ip_types.IP4Address{}) && r.SnatIP6 == (ip_types.IP6Address{}) && r.SwIfIndex == noIf {
			if f.snat == nil {
				return []api.Message{&cnatapi.CnatSetSnatAddressesReply{Retval: int32(api.FEATURE_DISABLED)}}, nil
			}
			f.snat, f.policy, f.snatIfs, f.excluded, f.pfxRefs = nil, 0, map[[2]uint32]bool{}, map[string]bool{}, map[string]int{}
			return []api.Message{&cnatapi.CnatSetSnatAddressesReply{}}, nil
		}
		if f.snat == nil {
			f.snat = &cnatapi.CnatGetSnatAddressesReply{}
		}
		f.snat.SnatIP4, f.snat.SnatIP6 = r.SnatIP4, r.SnatIP6
		if r.SnatIP6 != (ip_types.IP6Address{}) || r.SwIfIndex != noIf {
			f.snat.SwIfIndex = r.SwIfIndex // VPP reports the IPv6 endpoint's sw_if_index
		}
		return []api.Message{&cnatapi.CnatSetSnatAddressesReply{}}, nil
	})
	f.On("cnat_set_snat_policy", func(req api.Message) ([]api.Message, error) {
		if f.snat == nil {
			t.Fatal("cnat_set_snat_policy without a default SNAT entry would crash VPP 26.06")
		}
		f.policy = req.(*cnatapi.CnatSetSnatPolicy).Policy
		return []api.Message{&cnatapi.CnatSetSnatPolicyReply{}}, nil
	})
	f.On("cnat_snat_policy_add_del_exclude_pfx", func(req api.Message) ([]api.Message, error) {
		if f.snat == nil {
			t.Fatal("cnat_snat_policy_add_del_exclude_pfx without a default SNAT entry would crash VPP 26.06")
		}
		r := req.(*cnatapi.CnatSnatPolicyAddDelExcludePfx)
		k := natcommon.PrefixString(r.Prefix)
		if r.IsAdd == 1 {
			f.excluded[k] = true
			f.pfxRefs[k]++
		} else {
			delete(f.excluded, k)
			if f.pfxRefs[k] > 0 {
				f.pfxRefs[k]--
			}
		}
		return []api.Message{&cnatapi.CnatSnatPolicyAddDelExcludePfxReply{}}, nil
	})
	f.On("cnat_snat_policy_add_del_if", func(req api.Message) ([]api.Message, error) {
		if f.snat == nil {
			return []api.Message{&cnatapi.CnatSnatPolicyAddDelIfReply{Retval: int32(api.FEATURE_DISABLED)}}, nil
		}
		r := req.(*cnatapi.CnatSnatPolicyAddDelIf)
		k := [2]uint32{uint32(r.SwIfIndex), uint32(r.Table)}
		if r.IsAdd == 1 {
			f.snatIfs[k] = true
		} else {
			delete(f.snatIfs, k)
		}
		return []api.Message{&cnatapi.CnatSnatPolicyAddDelIfReply{}}, nil
	})
	f.On("feature_cnat_enable_disable", func(req api.Message) ([]api.Message, error) {
		r := req.(*cnatapi.FeatureCnatEnableDisable)
		f.feat[uint32(r.SwIfIndex)] = r.EnableDisable
		return []api.Message{&cnatapi.FeatureCnatEnableDisableReply{}}, nil
	})
	f.On("feature_is_enabled", func(req api.Message) ([]api.Message, error) {
		r := req.(*feature.FeatureIsEnabled)
		on := r.ArcName == "ip4-unicast" && r.FeatureName == "cnat-input-ip4" && f.feat[uint32(r.SwIfIndex)]
		return []api.Message{&feature.FeatureIsEnabledReply{IsEnabled: on}}, nil
	})
	f.Reply("cnat_session_dump", &cnatapi.CnatSessionDetails{Session: cnatapi.CnatSession{Tuple: cnatapi.Cnat5tuple{
		Addr: [2]ip_types.Address{mustAddr("10.9.47.1"), mustAddr("10.9.48.1")}, Port: []uint16{1234, 80}, IPProto: ip_types.IP_API_PROTO_TCP}}})
	f.On("show_threads", func(api.Message) ([]api.Message, error) {
		return []api.Message{&vlib.ShowThreadsReply{Count: 1, ThreadData: []vlib.ThreadData{{ID: 0, PID: f.vppPID}}}}, nil
	})
	f.Reply("cnat_session_purge", &cnatapi.CnatSessionPurgeReply{})
	return f
}

// details mimics the dump: write-only fields (flags, is_real_ip, flow hash) are not
// returned, and path flags carry internal tracker bits (CNAT_TRK_ACTIVE = 4).
func details(tr cnatapi.CnatTranslation) cnatapi.CnatTranslation {
	tr.Flags, tr.IsRealIP, tr.FlowHashConfig = 0, 0, 0
	paths := make([]cnatapi.CnatEndpointTuple, len(tr.Paths))
	copy(paths, tr.Paths)
	for i := range paths {
		paths[i].Flags |= 4
	}
	tr.Paths = paths
	return tr
}

func mustAddr(s string) ip_types.Address {
	a, err := natcommon.Addr(s)
	if err != nil {
		panic(err)
	}
	return a
}

var owner = natcommon.WithGlobalsOwner(true)

func TestRegister(t *testing.T) {
	reg := scheduler.NewRegistry()
	cnat.Register(reg, newFakeCnat(t), "w9", owner)
	if reg.Len() != 6 {
		t.Fatalf("registered %d", reg.Len())
	}
}

func TestTranslation(t *testing.T) {
	f := newFakeCnat(t)
	p := cnat.New(f, "w9", owner, natcommon.WithLockDir(t.TempDir()))
	ctx := context.Background()
	// a foreign translation (w3's VIP) stays invisible
	f.trs[50] = cnatapi.CnatTranslation{ID: 50, Vip: cnatapi.CnatEndpoint{Addr: mustAddr("10.3.47.1"), Port: 80}, IPProto: ip_types.IP_API_PROTO_TCP, NPaths: 1,
		Paths: []cnatapi.CnatEndpointTuple{{DstEp: cnatapi.CnatEndpoint{Addr: mustAddr("10.3.48.1")}}}}
	f.nextID = 51

	tr := natcommon.MustEncode(&cnat.TranslationSpec{VIP: "10.9.47.1", Port: 80, Proto: "6",
		Paths: []cnat.PathSpec{{Dst: "10.9.48.2", DstPort: 8080}, {Dst: "10.9.48.1", DstPort: 8080, Src: "0.0.0.0"}}})
	if deps := p.Translation.Dependencies(tr); len(deps) != 1 || deps[0].Key != "cnat.snat-addresses/global" || !deps[0].Optional {
		t.Fatalf("deps %+v", deps)
	}
	if nattest.Apply(t, p.Translation, tr) != 1 || nattest.Apply(t, p.Translation, tr) != 0 {
		t.Fatal("create / idempotent")
	}
	if keys := nattest.Keys(t, p.Translation); len(keys) != 1 || keys[0] != "cnat.translation/10.9.47.1/tcp/80" {
		t.Fatalf("keys %v", keys)
	}
	req := f.CallsNamed("cnat_translation_update")[0].(*cnatapi.CnatTranslationUpdate).Translation
	if req.NPaths != 2 || req.Flags != 0 || req.IsRealIP != 0 || req.Vip.SwIfIndex != noIf || req.Paths[0].SrcEp.SwIfIndex != noIf || req.IPProto != ip_types.IP_API_PROTO_TCP {
		t.Fatalf("translation request %+v", req)
	}
	// backend set / lb change → in-place update, same id; internal tracker bits are ignored
	tr2 := natcommon.MustEncode(&cnat.TranslationSpec{VIP: "10.9.47.1", Port: 80, Proto: "tcp", LBType: cnat.LBMaglev,
		Paths: []cnat.PathSpec{{Dst: "10.9.48.3", DstPort: 8080, NoNAT: true}}})
	if nattest.Apply(t, p.Translation, tr2) != 1 || nattest.Apply(t, p.Translation, tr2) != 0 || len(f.trs) != 2 {
		t.Fatal("update in place")
	}
	if f.trs[51].LbType != cnatapi.CNAT_LB_TYPE_MAGLEV || f.trs[51].Paths[0].Flags&uint8(cnatapi.CNAT_EPT_NO_NAT) == 0 {
		t.Fatalf("updated %+v", f.trs[51])
	}
	// a fresh process (agent restart) converges without any cached state
	if nattest.Apply(t, cnat.New(f, "w9", owner, natcommon.WithLockDir(t.TempDir())).Translation, tr2) != 0 {
		t.Fatal("restart: Retrieve alone must reproduce the desired value")
	}
	if _, err := p.Translation.Create(ctx, natcommon.MustEncode(&cnat.TranslationSpec{VIP: "10.9.47.9", Port: 1, Proto: "udp"})); !errors.Is(err, cnat.ErrNoPaths) {
		t.Fatalf("no paths: %v", err)
	}
	if _, err := p.Translation.Create(ctx, natcommon.MustEncode(&cnat.TranslationSpec{VIP: "10.9.47.9", Proto: "udp", LBType: "x", Paths: []cnat.PathSpec{{Dst: "10.9.48.1"}}})); err == nil {
		t.Fatal("bad lb type")
	}
	// finding 3: delete by id re-verifies (vip, port, proto) at that id first
	kvs, _ := p.Translation.Retrieve(ctx)
	id := kvs[0].Meta.(cnat.TranslationMeta).ID
	saved := f.trs[id]
	reused := saved
	reused.Vip.Addr = mustAddr("10.3.47.9") // id reused by another owner's translation
	f.trs[id] = reused
	if err := p.Translation.Delete(ctx, kvs[0].Value, kvs[0].Meta); err != nil || len(f.CallsNamed("cnat_translation_del")) != 0 {
		t.Fatalf("delete of a reused id must not be sent: %v", err)
	}
	f.trs[id] = saved
	if nattest.Apply(t, p.Translation) != 1 || len(f.trs) != 1 {
		t.Fatal("delete, foreign kept")
	}
}

func TestSnat(t *testing.T) {
	f := newFakeCnat(t)
	p := cnat.New(f, "w9", owner, natcommon.WithLockDir(t.TempDir()))
	ctx := context.Background()
	pol := natcommon.MustEncode(&cnat.SnatPolicySpec{Policy: cnat.PolicyIfPfx})
	ex := natcommon.MustEncode(&cnat.SnatExcludePrefixSpec{Prefix: "10.9.49.7/24"})
	sif := natcommon.MustEncode(&cnat.SnatInterfaceSpec{Interface: "loop940", Table: cnat.TableIncludeV4})

	// policy / interface tables / excluded prefixes have no getter: write-only (D-063)
	for _, d := range []scheduler.Descriptor{p.SnatPolicy, p.SnatInterface, p.SnatExcludePfx} {
		nattest.AssertWriteOnly(t, d)
	}
	// without the default entry: guarded errors, nothing sent that would crash VPP
	if _, err := p.SnatPolicy.Create(ctx, pol); !errors.Is(err, cnat.ErrNoSnatDefault) {
		t.Fatalf("policy without default: %v", err)
	}
	if _, err := p.SnatExcludePfx.Create(ctx, ex); !errors.Is(err, cnat.ErrNoSnatDefault) {
		t.Fatalf("exclude without default: %v", err)
	}
	if _, err := p.SnatInterface.Create(ctx, sif); !errors.Is(err, cnat.ErrNoSnatDefault) {
		t.Fatalf("interface without default: %v", err)
	}
	if err := p.SnatExcludePfx.Delete(ctx, ex, nil); err != nil {
		t.Fatalf("exclude delete without default is a no-op: %v", err)
	}
	if err := p.SnatPolicy.Delete(ctx, pol, nil); err != nil {
		t.Fatalf("policy delete without default is a no-op: %v", err)
	}
	for _, c := range []struct {
		d   scheduler.Descriptor
		obj *structpb.Struct
	}{{p.SnatPolicy, pol}, {p.SnatExcludePfx, ex}, {p.SnatInterface, sif}} {
		if deps := c.d.Dependencies(c.obj); len(deps) == 0 || deps[0].Key != "cnat.snat-addresses/global" || deps[0].Optional {
			t.Fatalf("%s deps %+v", c.d.Name(), deps)
		}
	}
	if deps := p.SnatInterface.Dependencies(sif); len(deps) != 2 || deps[1].Key != "interface/loop940" {
		t.Fatalf("snat-interface deps %+v", deps)
	}

	addrs := natcommon.MustEncode(&cnat.SnatAddressesSpec{IP4: "10.9.49.1"})
	if len(nattest.Keys(t, p.SnatAddresses)) != 0 || nattest.Apply(t, p.SnatAddresses, addrs) != 1 || nattest.Apply(t, p.SnatAddresses, addrs) != 0 {
		t.Fatal("snat addresses")
	}
	if r := f.CallsNamed("cnat_set_snat_addresses")[0].(*cnatapi.CnatSetSnatAddresses); r.SwIfIndex != noIf {
		t.Fatalf("no interface must be ~0, got %d (0 would bind local0)", r.SwIfIndex)
	}
	if _, err := p.SnatAddresses.Update(ctx, addrs, natcommon.MustEncode(&cnat.SnatAddressesSpec{IP4: "10.9.49.2"}), nil); !errors.Is(err, scheduler.ErrRecreate) {
		t.Fatalf("update: %v", err)
	}
	// write-only creates are re-applied on every resync: idempotent in VPP
	for i := 0; i < 2; i++ {
		for _, c := range []struct {
			d   scheduler.Descriptor
			obj *structpb.Struct
		}{{p.SnatPolicy, pol}, {p.SnatExcludePfx, ex}, {p.SnatInterface, sif}} {
			if _, err := c.d.Create(ctx, c.obj); err != nil {
				t.Fatalf("%s create #%d: %v", c.d.Name(), i, err)
			}
		}
	}
	if f.policy != cnatapi.CNAT_POLICY_IF_PFX || !f.excluded["10.9.49.0/24"] || !f.snatIfs[[2]uint32{1, 0}] {
		t.Fatalf("snat state policy=%v excluded=%v ifs=%v", f.policy, f.excluded, f.snatIfs)
	}
	// delete without Meta (after an agent restart): the interface is resolved by name
	if err := p.SnatInterface.Delete(ctx, sif, nil); err != nil || len(f.snatIfs) != 0 {
		t.Fatalf("snat interface delete: %v %v", err, f.snatIfs)
	}
	if err := p.SnatInterface.Delete(ctx, natcommon.MustEncode(&cnat.SnatInterfaceSpec{Interface: "gone0", Table: cnat.TableHost}), nil); err != nil {
		t.Fatalf("delete on a vanished interface is a no-op: %v", err)
	}
	if err := p.SnatExcludePfx.Delete(ctx, ex, nil); err != nil || len(f.excluded) != 0 {
		t.Fatalf("exclude delete: %v", err)
	}
	if err := p.SnatPolicy.Delete(ctx, pol, nil); err != nil || f.policy != cnatapi.CNAT_POLICY_NONE {
		t.Fatalf("policy delete → none: %v", err)
	}
	if nattest.Apply(t, p.SnatAddresses) != 1 || f.snat != nil {
		t.Fatal("snat delete")
	}

	// D-071: the default SNAT entry and the policy are globals; a non-owner requires the
	// entry, never creates, replaces or deletes it, and cannot verify the policy
	f.snat = &cnatapi.CnatGetSnatAddressesReply{SnatIP4: [4]uint8{10, 3, 0, 1}}
	w3 := cnat.New(f, "w3", natcommon.WithLockDir(t.TempDir()))
	sets := len(f.CallsNamed("cnat_set_snat_addresses"))
	if _, err := w3.SnatAddresses.Create(ctx, natcommon.MustEncode(&cnat.SnatAddressesSpec{IP4: "10.3.0.1"})); err != nil {
		t.Fatalf("non-owner requirement met: %v", err)
	}
	if _, err := w3.SnatAddresses.Create(ctx, addrs); !errors.Is(err, natcommon.ErrGlobalMismatch) {
		t.Fatalf("non-owner mismatch: %v", err)
	}
	if err := w3.SnatAddresses.Delete(ctx, addrs, nil); err != nil || f.snat == nil || len(f.CallsNamed("cnat_set_snat_addresses")) != sets {
		t.Fatalf("non-owner must never delete the default entry: %v", err)
	}
	if err := w3.SnatPolicy.Delete(ctx, pol, nil); err != nil || len(f.CallsNamed("cnat_set_snat_policy")) != 3 {
		t.Fatalf("non-owner must never reset the policy: %v", err)
	}

	// interface-based entry: addresses are derived, only the interface is reported
	f.snat = nil
	ifs := natcommon.MustEncode(&cnat.SnatAddressesSpec{Interface: "loop940"})
	if nattest.Apply(t, p.SnatAddresses, ifs) != 1 || nattest.Apply(t, p.SnatAddresses, ifs) != 0 || f.snat.SwIfIndex != 1 {
		t.Fatalf("interface snat %+v", f.snat)
	}
	if _, err := p.SnatAddresses.Create(ctx, natcommon.MustEncode(&cnat.SnatAddressesSpec{Interface: "loop940", IP4: "10.9.49.1"})); err == nil {
		t.Fatal("interface and address together must be rejected")
	}
}

func TestInterfaceFeatureAndState(t *testing.T) {
	f := newFakeCnat(t)
	p := cnat.New(f, "w9", owner, natcommon.WithLockDir(t.TempDir()))
	ctx := context.Background()
	f.feat[3] = true // w3
	in := natcommon.MustEncode(&cnat.InterfaceFeatureSpec{Interface: "loop941"})
	if nattest.Apply(t, p.InterfaceFeature, in) != 1 || nattest.Apply(t, p.InterfaceFeature, in) != 0 || !f.feat[2] {
		t.Fatal("feature")
	}
	if nattest.Apply(t, p.InterfaceFeature) != 1 || f.feat[2] || !f.feat[3] {
		t.Fatal("feature delete, foreign kept")
	}
	sess, err := p.Sessions(ctx, 0, 10)
	if err != nil || len(sess) != 1 || sess[0].DstPort != 80 || sess[0].Proto != "tcp" || sess[0].Src != "10.9.47.1" {
		t.Fatalf("sessions %+v %v", sess, err)
	}
	if err := p.PurgeSessions(ctx); err != nil {
		t.Fatal(err)
	}
}

// TestExcludePrefixIdempotentAcrossResyncs is D-076: the reconciler re-applies write-only
// objects on every resync; VPP's excluded-prefix add stacks a refcount, so a second resync must
// not add again while VPP is the same process — and must re-add once after a VPP restart.
func TestExcludePrefixIdempotentAcrossResyncs(t *testing.T) {
	f := newFakeCnat(t)
	p := cnat.New(f, "w9", owner, natcommon.WithLockDir(t.TempDir()))
	ctx := context.Background()
	if nattest.Apply(t, p.SnatAddresses, natcommon.MustEncode(&cnat.SnatAddressesSpec{IP4: "10.9.49.1"})) != 1 {
		t.Fatal("snat entry")
	}
	ex := natcommon.MustEncode(&cnat.SnatExcludePrefixSpec{Prefix: "10.9.50.0/24"})
	for resync := 0; resync < 3; resync++ {
		if _, err := p.SnatExcludePfx.Create(ctx, ex); err != nil {
			t.Fatal(err)
		}
	}
	if f.pfxRefs["10.9.50.0/24"] != 1 || len(f.CallsNamed("cnat_snat_policy_add_del_exclude_pfx")) != 1 {
		t.Fatalf("three resyncs must leave one instance (refs %d)", f.pfxRefs["10.9.50.0/24"])
	}
	// VPP restart: new identity, state gone → re-added exactly once
	f.vppPID, f.excluded, f.pfxRefs = 4343, map[string]bool{}, map[string]int{}
	for resync := 0; resync < 2; resync++ {
		if _, err := p.SnatExcludePfx.Create(ctx, ex); err != nil {
			t.Fatal(err)
		}
	}
	if f.pfxRefs["10.9.50.0/24"] != 1 {
		t.Fatalf("after a VPP restart: refs %d, want 1", f.pfxRefs["10.9.50.0/24"])
	}
	if err := p.SnatExcludePfx.Delete(ctx, ex, nil); err != nil || f.pfxRefs["10.9.50.0/24"] != 0 {
		t.Fatalf("delete: %v", err)
	}
	if _, err := p.SnatExcludePfx.Create(ctx, ex); err != nil || f.pfxRefs["10.9.50.0/24"] != 1 {
		t.Fatal("re-create after delete adds again")
	}
}
