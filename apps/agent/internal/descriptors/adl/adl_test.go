package adl

import (
	"context"
	"errors"
	"testing"

	"go.fd.io/govpp/api"
	"google.golang.org/protobuf/proto"

	adlapi "ngfw/agent/binapi/adl"
	featureapi "ngfw/agent/binapi/feature"
	interfaces "ngfw/agent/binapi/interface"
	"ngfw/agent/binapi/memclnt"
	"ngfw/agent/internal/descriptors/acl"
	"ngfw/agent/internal/descriptors/df2"
	"ngfw/agent/internal/descriptors/dfkit"
	"ngfw/agent/internal/scheduler"
	"ngfw/agent/internal/vpp"
	"ngfw/agent/internal/vpp/bootid"
	"ngfw/agent/internal/vpp/fake"
)

// family indexes of VPP's adl config mains (plugins/adl/adl.h: VNET_ADL_IP4, _IP6, _DEFAULT).
const (
	famIP4 = iota
	famIP6
	famDefault
)

// adlModel is VPP 26.06's adl plugin state per sw_if_index, as plugins/adl/adl.c and
// vnet/config.c behave (see the package doc):
//   - adl_interface_enable_disable adds/removes one adl-input feature on device-input (stacking);
//   - adl_allowlist_enable_disable, per family: flag set → one more allow-list instance; flag
//     clear → one removed, or — when none is configured — the family's config index becomes ~0
//     ("wiped"), which adl-input dereferences for the next packet of that family (crash);
//   - feature_is_enabled answers true for every error (V23 a): unknown feature, an index beyond
//     the device-input config vector (outOfRange).
type adlModel struct {
	input      map[uint32]int    // adl-input feature instances
	allow      map[uint32][3]int // allow-list instances per family
	wiped      map[uint32][3]bool
	outOfRange map[uint32]bool
	adlUnknown bool // the adl plugin is not loaded: adl-input is an unknown feature
	pid        uint32
}

// crashes reports whether a packet of some family arriving on idx would make VPP read config
// index ~0, or hit the leaking default stub.
func (m *adlModel) crashes(idx uint32) bool {
	if m.input[idx] == 0 {
		return false
	}
	w := m.wiped[idx]
	return w[famIP4] || w[famIP6] || w[famDefault] || m.allow[idx][famDefault] > 0
}

// newFake is a fake VPP with the adl model: loop300 (ours, 5), loop200 (another owner's, ADL
// on), an untagged port (9).
func newFake() (*fake.Client, *adlModel) {
	m := &adlModel{input: map[uint32]int{7: 1}, allow: map[uint32][3]int{}, wiped: map[uint32][3]bool{}, outOfRange: map[uint32]bool{}, pid: 1000}
	f := fake.New()
	f.On("control_ping", func(api.Message) ([]api.Message, error) {
		return []api.Message{&memclnt.ControlPingReply{VpePID: m.pid}}, nil
	})
	f.On("sw_interface_dump", func(api.Message) ([]api.Message, error) {
		return []api.Message{
			&interfaces.SwInterfaceDetails{SwIfIndex: 0, InterfaceName: "local0"},
			&interfaces.SwInterfaceDetails{SwIfIndex: 5, InterfaceName: "loop300", Tag: "w3:loop300"},
			&interfaces.SwInterfaceDetails{SwIfIndex: 7, InterfaceName: "loop200", Tag: "w2:loop200"},
			&interfaces.SwInterfaceDetails{SwIfIndex: 9, InterfaceName: "GigabitEthernet0/0/0"},
		}, nil
	})
	f.On("adl_interface_enable_disable", func(req api.Message) ([]api.Message, error) {
		r := req.(*adlapi.AdlInterfaceEnableDisable)
		idx := uint32(r.SwIfIndex)
		if r.EnableDisable {
			m.input[idx]++
			delete(m.outOfRange, idx) // vec_validate on enable
		} else if m.input[idx] > 0 {
			m.input[idx]--
		}
		return []api.Message{&adlapi.AdlInterfaceEnableDisableReply{}}, nil
	})
	f.On("feature_is_enabled", func(req api.Message) ([]api.Message, error) {
		r := req.(*featureapi.FeatureIsEnabled)
		idx := uint32(r.SwIfIndex)
		known := r.ArcName == "device-input" && (r.FeatureName == "ethernet-input" || (r.FeatureName == "adl-input" && !m.adlUnknown))
		switch {
		case !known, m.outOfRange[idx]:
			return []api.Message{&featureapi.FeatureIsEnabledReply{IsEnabled: true}}, nil // V23 (a): the error as a bool
		case r.FeatureName == "adl-input":
			return []api.Message{&featureapi.FeatureIsEnabledReply{IsEnabled: m.input[idx] > 0}}, nil
		default:
			return []api.Message{&featureapi.FeatureIsEnabledReply{IsEnabled: false}}, nil
		}
	})
	f.On("adl_allowlist_enable_disable", func(req api.Message) ([]api.Message, error) {
		r := req.(*adlapi.AdlAllowlistEnableDisable)
		idx := uint32(r.SwIfIndex)
		c, w := m.allow[idx], m.wiped[idx]
		for fam, add := range [3]bool{r.IP4, r.IP6, r.DefaultAdl} {
			switch {
			case add:
				c[fam]++
			case w[fam]: // ci == ~0: VPP skips the delete
			case c[fam] > 0:
				c[fam]--
			default:
				w[fam] = true // vnet_config_del_feature: not found → ~0 stored
			}
		}
		m.allow[idx], m.wiped[idx] = c, w
		return []api.Message{&adlapi.AdlAllowlistEnableDisableReply{}}, nil
	})
	dfkit.IdentitySource = func(ctx context.Context, c vpp.Client) (bootid.Identity, error) {
		id, err := bootid.Reader{ProcRoot: "/nonexistent"}.Current(ctx, c)
		id.BootID, id.StartTime = "fake", 1
		return id, err
	}
	return f, m
}

func TestInterface(t *testing.T) {
	ctx := context.Background()
	f, _ := newFake()
	d := NewInterface(f, "w3")
	desired := &Interface{Interface: "loop300"}
	if k := d.KeyOf(desired); k != "adl.interface/loop300" || !scheduler.ValidName(d.Name()) {
		t.Fatalf("KeyOf = %s", k)
	}
	if deps := d.Dependencies(desired); len(deps) != 2 || deps[0].Key != "interface/loop300" || deps[0].Optional ||
		deps[1].Key != "adl.allowlist/loop300" || !deps[1].Optional {
		t.Fatalf("Dependencies = %+v (the allow-list goes first, optionally)", deps)
	}
	meta, err := d.Create(ctx, desired)
	if err != nil || meta != (Meta{SwIfIndex: 5}) {
		t.Fatalf("Create: %v %+v", err, meta)
	}
	req := f.CallsNamed("adl_interface_enable_disable")[0].(*adlapi.AdlInterfaceEnableDisable)
	if req.SwIfIndex != 5 || !req.EnableDisable {
		t.Fatalf("request = %+v", req)
	}
	actual, err := d.Retrieve(ctx)
	if err != nil || len(actual) != 1 || !proto.Equal(actual[0].Value, desired) || actual[0].Meta != meta {
		t.Fatalf("Retrieve = %+v, %v (another owner's loop200 must not appear)", actual, err)
	}
	if _, err := d.Update(ctx, desired, &Interface{Interface: "loop301"}, meta); !errors.Is(err, scheduler.ErrRecreate) {
		t.Fatalf("Update: %v", err)
	}
	if err := d.Delete(ctx, desired, meta); err != nil {
		t.Fatal(err)
	}
	req = f.CallsNamed("adl_interface_enable_disable")[1].(*adlapi.AdlInterfaceEnableDisable)
	if req.SwIfIndex != 5 || req.EnableDisable {
		t.Fatalf("delete request = %+v", req)
	}
	if actual, err := d.Retrieve(ctx); err != nil || len(actual) != 0 {
		t.Fatalf("after Delete = %+v, %v", actual, err)
	}
	if _, err := d.Create(ctx, &Interface{Interface: "loop200"}); !errors.Is(err, df2.ErrForeignInterface) {
		t.Fatalf("foreign interface: %v", err)
	}
	// Physical port: visible only while claimed.
	claims := acl.NewMemoryClaimStore()
	pd := NewInterface(f, "w3", df2.WithClaims(claims))
	phys := &Interface{Interface: "GigabitEthernet0/0/0"}
	pmeta, err := pd.Create(ctx, phys)
	if err != nil {
		t.Fatal(err)
	}
	if actual, _ := NewInterface(f, "w3", df2.WithClaims(claims)).Retrieve(ctx); len(actual) != 1 || !proto.Equal(actual[0].Value, phys) {
		t.Fatalf("claimed physical port not retrieved: %+v", actual)
	}
	if actual, _ := NewInterface(f, "w3").Retrieve(ctx); len(actual) != 0 {
		t.Fatalf("unclaimed physical port retrieved: %+v", actual)
	}
	if err := pd.Delete(ctx, phys, pmeta); err != nil || claims.Claimed(string(pd.KeyOf(phys))) {
		t.Fatalf("Delete: %v, claim kept=%v", err, claims.Claimed(string(pd.KeyOf(phys))))
	}
	if _, err := d.Create(ctx, &Interface{Interface: "nope"}); !errors.Is(err, df2.ErrNoSuchInterface) {
		t.Fatalf("unknown interface: %v", err)
	}
	if err := d.Delete(ctx, desired, nil); !errors.Is(err, df2.ErrBadMeta) {
		t.Fatalf("bad meta: %v", err)
	}
	f.Reply("adl_interface_enable_disable", &adlapi.AdlInterfaceEnableDisableReply{Retval: -2})
	if _, err := d.Create(ctx, desired); err == nil {
		t.Fatal("retval must surface")
	}
}

// TestInterfaceRetrieveV23 is the V23 (a) fix: feature_is_enabled answers true for VPP errors,
// so a "true" counts only when the control query (device-input end node) is false on the same
// index and adl-input is known (false on local0).
func TestInterfaceRetrieveV23(t *testing.T) {
	ctx := context.Background()
	f, m := newFake()
	d := NewInterface(f, "w3")

	// a fresh interface beyond the device-input config vector: raw query says "enabled"
	m.outOfRange[5] = true
	raw, err := featureapi.NewServiceClient(f).FeatureIsEnabled(ctx, &featureapi.FeatureIsEnabled{ArcName: "device-input", FeatureName: "adl-input", SwIfIndex: 5})
	if err != nil || !raw.IsEnabled {
		t.Fatalf("model: the raw query must read true for an out-of-range index (V23 a): %+v %v", raw, err)
	}
	if actual, err := d.Retrieve(ctx); err != nil || len(actual) != 0 {
		t.Fatalf("out-of-range index retrieved as ADL on: %+v, %v", actual, err)
	}
	// enabled for real: reported (the enable validates the vector)
	meta, err := d.Create(ctx, &Interface{Interface: "loop300"})
	if err != nil {
		t.Fatal(err)
	}
	if actual, err := d.Retrieve(ctx); err != nil || len(actual) != 1 || actual[0].Meta != meta {
		t.Fatalf("enabled ADL not retrieved: %+v, %v", actual, err)
	}
	// the adl plugin unknown to VPP: every adl-input query reads true, local0 included → nothing on
	m.adlUnknown = true
	m.input[5] = 0
	if actual, err := d.Retrieve(ctx); err != nil || len(actual) != 0 {
		t.Fatalf("unknown adl-input feature retrieved as ADL on: %+v, %v", actual, err)
	}
	n := len(f.CallsNamed("feature_is_enabled"))
	if _, err := d.Retrieve(ctx); err != nil {
		t.Fatal(err)
	}
	// loop300 is the only candidate this owner owns (the untagged port is not claimed):
	// adl-input + control + one local0 probe per Retrieve
	if got := len(f.CallsNamed("feature_is_enabled")) - n; got != 3 {
		t.Fatalf("feature_is_enabled calls per Retrieve = %d, want 3 (local0 probed once)", got)
	}
	// an error of the control query surfaces
	m.adlUnknown = false
	m.input[5] = 1
	f.On("feature_is_enabled", func(req api.Message) ([]api.Message, error) {
		r := req.(*featureapi.FeatureIsEnabled)
		if r.FeatureName == "ethernet-input" {
			return []api.Message{&featureapi.FeatureIsEnabledReply{Retval: -3}}, nil
		}
		return []api.Message{&featureapi.FeatureIsEnabledReply{IsEnabled: true}}, nil
	})
	if _, err := d.Retrieve(ctx); err == nil {
		t.Fatal("control query error must surface")
	}
}

func TestAllowlist(t *testing.T) {
	ctx := context.Background()
	f, m := newFake()
	boot := dfkit.NewMemoryBootStore()
	d := NewAllowlist(f, "w3", WithBootStore(boot))
	desired := &Allowlist{Interface: "loop300", FibId: 3001, Ip4: true}
	if k := d.KeyOf(desired); k != "adl.allowlist/loop300" || !scheduler.ValidName(d.Name()) {
		t.Fatalf("KeyOf = %s", k)
	}
	deps := d.Dependencies(desired)
	if len(deps) != 2 || deps[0].Key != "interface/loop300" || deps[1].Key != "vrf/3001" || deps[1].Optional {
		t.Fatalf("Dependencies = %+v", deps)
	}
	if deps := d.Dependencies(&Allowlist{Interface: "loop300", Ip6: true}); len(deps) != 1 {
		t.Fatalf("table 0: Dependencies = %+v", deps)
	}
	meta, err := d.Create(ctx, desired)
	if err != nil || meta != (Meta{SwIfIndex: 5}) {
		t.Fatalf("Create: %v %+v", err, meta)
	}
	calls := f.CallsNamed("adl_allowlist_enable_disable")
	if len(calls) != 2 {
		t.Fatalf("add sequence = %d calls, want 2", len(calls))
	}
	c1, c2 := calls[0].(*adlapi.AdlAllowlistEnableDisable), calls[1].(*adlapi.AdlAllowlistEnableDisable)
	if !c1.IP4 || !c1.IP6 || !c1.DefaultAdl || !c2.IP4 || c2.IP6 || c2.DefaultAdl || c1.FibID != 3001 || c2.FibID != 3001 || c1.SwIfIndex != 5 {
		t.Fatalf("add sequence = %+v, %+v; want (1,1,1) then (1,0,0)", c1, c2)
	}
	if got := m.allow[5]; got != [3]int{2, 0, 0} || m.wiped[5] != [3]bool{} {
		t.Fatalf("model after add = %v wiped %v; want ip4 ×2, nothing wiped", got, m.wiped[5])
	}
	// adl-input on: no family of the interface is left in a crashing state
	m.input[5] = 1
	if m.crashes(5) {
		t.Fatal("a packet would crash VPP after the add sequence")
	}
	// resync (D-063 re-apply): the record makes the repeat a no-op (D-076: no stacking)
	if _, err := d.Create(ctx, desired); err != nil || len(f.CallsNamed("adl_allowlist_enable_disable")) != 2 {
		t.Fatalf("repeated Create sent %d calls (%v), want none", len(f.CallsNamed("adl_allowlist_enable_disable"))-2, err)
	}
	// a fresh descriptor on the same persisted store (agent restart) does not re-add either
	if _, err := NewAllowlist(f, "w3", WithBootStore(boot)).Create(ctx, desired); err != nil || len(f.CallsNamed("adl_allowlist_enable_disable")) != 2 {
		t.Fatalf("Create after an agent restart re-added (%v)", err)
	}
	// desired changed while the agent was down: the old value is removed first, then the new added
	changed := &Allowlist{Interface: "loop300", FibId: 3002, Ip6: true}
	if _, err := NewAllowlist(f, "w3", WithBootStore(boot)).Create(ctx, changed); err != nil {
		t.Fatal(err)
	}
	if got := m.allow[5]; got != [3]int{0, 2, 0} || m.wiped[5] != [3]bool{} || m.crashes(5) {
		t.Fatalf("model after a changed re-apply = %v wiped %v", got, m.wiped[5])
	}
	// VPP restarted (new identity): the add runs once more (VPP lost its state)
	m.pid++
	m.allow[5] = [3]int{}
	n := len(f.CallsNamed("adl_allowlist_enable_disable"))
	if _, err := d.Create(ctx, changed); err != nil || len(f.CallsNamed("adl_allowlist_enable_disable")) != n+2 {
		t.Fatalf("Create after a VPP restart: %v, %d calls", err, len(f.CallsNamed("adl_allowlist_enable_disable"))-n)
	}
	if _, err := d.Update(ctx, changed, desired, meta); !errors.Is(err, scheduler.ErrRecreate) {
		t.Fatalf("Update: %v (every change is a recreate)", err)
	}
	// Delete: the remove sequence, back to nothing, nothing wiped
	if err := d.Delete(ctx, changed, meta); err != nil {
		t.Fatal(err)
	}
	if got := m.allow[5]; got != [3]int{} || m.wiped[5] != [3]bool{} || m.crashes(5) {
		t.Fatalf("model after Delete = %v wiped %v", got, m.wiped[5])
	}
	if _, ok := boot.Get(string(d.KeyOf(changed))); ok {
		t.Fatal("record kept after Delete")
	}
	// Delete without a record of this VPP instance sends nothing (nothing of ours in VPP)
	n = len(f.CallsNamed("adl_allowlist_enable_disable"))
	if err := d.Delete(ctx, changed, meta); err != nil || len(f.CallsNamed("adl_allowlist_enable_disable")) != n {
		t.Fatalf("Delete without record: %v", err)
	}
	if _, err := d.Retrieve(ctx); !errors.Is(err, df2.ErrRetrieveUnsupported) {
		t.Fatalf("Retrieve: %v", err)
	}
	if _, err := d.Create(ctx, &Allowlist{Interface: "loop300", FibId: 3001}); err == nil {
		t.Fatal("neither ip4 nor ip6 accepted")
	}
	if _, err := d.Create(ctx, &Allowlist{Interface: "loop300", Ip4: true, DefaultAdl: true}); !errors.Is(err, ErrDefaultADL) {
		t.Fatalf("default_adl accepted: %v", err)
	}
	if _, err := d.Create(ctx, &Allowlist{Interface: "loop200", Ip4: true}); !errors.Is(err, df2.ErrForeignInterface) {
		t.Fatalf("foreign interface: %v", err)
	}
}

// TestAllowlistNaiveSequenceWouldCrash documents why the two-call sequences exist: the call
// DF-2 sent before (one add with ip6/default clear) wipes the families that were not
// configured — the model's crash condition once adl-input is on.
func TestAllowlistNaiveSequenceWouldCrash(t *testing.T) {
	ctx := context.Background()
	f, m := newFake()
	c := adlapi.NewServiceClient(f)
	if _, err := c.AdlAllowlistEnableDisable(ctx, &adlapi.AdlAllowlistEnableDisable{SwIfIndex: 5, FibID: 1, IP4: true}); err != nil {
		t.Fatal(err)
	}
	m.input[5] = 1
	if !m.crashes(5) || m.wiped[5] != [3]bool{false, true, true} {
		t.Fatalf("model: a single ip4-only add must wipe ip6 and default: %v", m.wiped[5])
	}
}

func TestAllowlistClaimsAndSkip(t *testing.T) {
	ctx := context.Background()
	f, m := newFake()
	claims := acl.NewMemoryClaimStore()
	d := NewAllowlist(f, "w3", WithAllowlistClaims(claims))
	phys := &Allowlist{Interface: "GigabitEthernet0/0/0", Ip4: true, Ip6: true}
	meta, err := d.Create(ctx, phys)
	if err != nil || !claims.Claimed(string(d.KeyOf(phys))) {
		t.Fatalf("Create on an untagged port: %v, claimed=%v", err, claims.Claimed(string(d.KeyOf(phys))))
	}
	if got := m.allow[9]; got != [3]int{2, 2, 0} {
		t.Fatalf("model = %v", got)
	}
	if err := d.Delete(ctx, phys, meta); err != nil || claims.Claimed(string(d.KeyOf(phys))) || m.allow[9] != [3]int{} || m.wiped[9] != [3]bool{} {
		t.Fatalf("Delete: %v claimed=%v model %v wiped %v", err, claims.Claimed(string(d.KeyOf(phys))), m.allow[9], m.wiped[9])
	}
	// a stale Meta (the index now names another interface): nothing is sent
	n := len(f.CallsNamed("adl_allowlist_enable_disable"))
	if err := d.Delete(ctx, phys, Meta{SwIfIndex: 5}); err != nil || len(f.CallsNamed("adl_allowlist_enable_disable")) != n {
		t.Fatalf("Delete with a reused index: %v", err)
	}
	if err := d.Delete(ctx, phys, nil); !errors.Is(err, df2.ErrBadMeta) {
		t.Fatalf("bad meta: %v", err)
	}
	// the second call failing undoes the first (0,0,0): nothing stays configured
	calls := 0
	f.On("adl_allowlist_enable_disable", func(api.Message) ([]api.Message, error) {
		calls++
		if calls == 2 {
			return []api.Message{&adlapi.AdlAllowlistEnableDisableReply{Retval: -1}}, nil
		}
		return []api.Message{&adlapi.AdlAllowlistEnableDisableReply{}}, nil
	})
	if _, err := d.Create(ctx, &Allowlist{Interface: "loop300", Ip4: true}); err == nil || calls != 3 {
		t.Fatalf("second call failure: %v after %d calls, want error after 3 (undo)", err, calls)
	}
	undo := f.CallsNamed("adl_allowlist_enable_disable")
	if u := undo[len(undo)-1].(*adlapi.AdlAllowlistEnableDisable); u.IP4 || u.IP6 || u.DefaultAdl {
		t.Fatalf("undo = %+v, want (0,0,0)", u)
	}
}

func TestRegister(t *testing.T) {
	reg := scheduler.NewRegistry()
	Register(reg, fake.New(), "w3")
	if got := reg.Names(); len(got) != 1 || got[0] != InterfaceName {
		t.Fatalf("Names = %v (adl.allowlist is write-only: not in the default Register)", got)
	}
	RegisterWriteOnly(reg, fake.New(), "w3", WithBootStore(dfkit.NewMemoryBootStore()))
	if _, ok := reg.Get(AllowlistName); !ok {
		t.Fatal("RegisterWriteOnly did not register adl.allowlist")
	}
}
