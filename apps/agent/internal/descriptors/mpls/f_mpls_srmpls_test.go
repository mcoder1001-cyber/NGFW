package mpls

// F-mpls-srmpls gap tests (D-104: gap-only, each a proven defect or a rule that landed after DF-7):
// the D-071 table-0 role, table-0 label routes of an agent that is not the globals owner, the TD-11b
// ownership declarations and claim/record-first Creates, the TD-11c alias key of the tunnel creator,
// and the dependencies DF-7 missed (default-VRF bindings, lookup tables). Every test fails on DF-7's
// code as merged.

import (
	"errors"
	"path/filepath"
	"testing"

	"go.fd.io/govpp/api"

	"ngfw/agent/binapi/mpls"
	"ngfw/agent/internal/descriptors/df7"
	"ngfw/agent/internal/descriptors/df7/df7test"
	"ngfw/agent/internal/descriptors/dfkit"
	"ngfw/agent/internal/descriptors/dfkit/persist"
	iface "ngfw/agent/internal/descriptors/interface"
	"ngfw/agent/internal/scheduler"
)

// D-071: an agent that is not the globals owner only requires MPLS table 0 — a clear error while
// VPP has none, nothing sent; accepted (not adopted) when the globals owner created it; never
// deleted; reported exactly while required.
func TestTableZeroRequiredByNonOwner(t *testing.T) {
	f, tables, _, _, _ := fakeMPLS()
	ctx := t.Context()
	d := NewTableFor(f, df7test.Owner, false, df7.WithIDRange(100, 199))
	zero := df7test.Desired(d, df7.Encode(Table{ID: 0}))
	if zero.Key != "mpls-table/0" {
		t.Fatal(zero.Key)
	}
	if _, err := d.Create(ctx, zero.Value); !errors.Is(err, dfkit.ErrNotGlobalsOwner) {
		t.Fatalf("no table 0: want ErrNotGlobalsOwner, got %v", err)
	}
	if n := len(f.CallsNamed("mpls_table_add_del")); n != 0 {
		t.Fatalf("a non-owner sent %d mpls_table_add_del", n)
	}
	tables[0] = "vrx:0" // the globals owner's table 0
	if _, err := d.Create(ctx, zero.Value); err != nil {
		t.Fatal(err)
	}
	if n := len(f.CallsNamed("mpls_table_add_del")); n != 0 || tables[0] != "vrx:0" {
		t.Fatalf("the requirement must not add a lock or rename table 0: %d calls, name %q", n, tables[0])
	}
	own := df7test.Desired(d, df7.Encode(Table{ID: 101}))
	if _, err := d.Create(ctx, own.Value); err != nil {
		t.Fatal(err)
	}
	df7test.AssertEmptyPlan(t, d, zero, own) // table 0 is reported while required: resync converges
	if err := d.Delete(ctx, zero.Value, nil); err != nil {
		t.Fatal(err)
	}
	if _, ok := tables[0]; !ok {
		t.Fatal("a non-owner deleted MPLS table 0")
	}
	df7test.AssertEmptyPlan(t, d, own) // no longer required → no longer reported (never planned for delete)
	tables[0] = df7test.Owner + ":0"   // even when it carries this owner's name, a non-owner never owns it
	df7test.AssertEmptyPlan(t, d, own)
	delete(tables, 0)
	if _, err := d.Create(ctx, zero.Value); !errors.Is(err, dfkit.ErrNotGlobalsOwner) {
		t.Fatalf("lost table 0: %v", err)
	}

	// the globals owner creates and deletes table 0 like any other table
	g := NewTableFor(f, df7test.Owner, true)
	if _, err := g.Create(ctx, zero.Value); err != nil || tables[0] != df7test.Owner+":0" {
		t.Fatal(err, tables)
	}
	if err := g.Delete(ctx, zero.Value, nil); err != nil {
		t.Fatal(err)
	}
	if _, ok := tables[0]; ok {
		t.Fatal("the globals owner's table 0 was not deleted")
	}
	r := scheduler.NewRegistry()
	RegisterFor(r, f, df7test.Owner, false)
	if r.Len() != 5 {
		t.Fatal(r.Names())
	}
	if td, _ := r.Get(NameTable); td.(*TableDescriptor).zero == nil {
		t.Fatal("RegisterFor(non-owner) must register the requiring table descriptor")
	}
}

// Gap: in the shared table 0 created by the globals owner (another name), this owner's own recorded
// label routes were never reported (Retrieve walked only tables named with this owner's tag) and
// Delete forgot them without removing them from VPP.
func TestRouteTableZeroOfTheGlobalsOwner(t *testing.T) {
	f, tables, _, routes, _ := fakeMPLS()
	ctx := t.Context()
	df7.SetBootStore(df7test.Owner, nil)
	tables[0] = "vrx:0"
	routes[rkey{0, 20001, 1}] = mpls.MplsRoute{MrTableID: 0, MrLabel: 20001, MrEos: 1} // another feature's
	d := NewRoute(f, df7test.Owner)
	r := Route{Table: 0, Label: 50016, EOS: true, EOSProto: PayloadIP4, Paths: paths(t, df7.Path{Type: df7.PathDrop})}
	v := df7test.Desired(d, df7.Encode(r))
	if _, err := d.Create(ctx, v.Value); err != nil {
		t.Fatal(err)
	}
	df7test.AssertEmptyPlan(t, d, v)
	if err := d.Delete(ctx, v.Value, nil); err != nil {
		t.Fatal(err)
	}
	if _, ok := routes[rkey{0, 50016, 1}]; ok {
		t.Fatal("Delete left our label route in table 0")
	}
	if _, ok := routes[rkey{0, 20001, 1}]; !ok {
		t.Fatal("another feature's label in table 0 was deleted")
	}
	df7test.AssertEmptyPlan(t, d)
}

// TD-11b (review 3.3): a label route of table 0 is recorded BEFORE its add — a failed record never
// leaves an unrecorded label in VPP — and a failed add drops the record it wrote.
func TestRouteTableZeroRecordFirst(t *testing.T) {
	f, tables, _, routes, _ := fakeMPLS()
	ctx := t.Context()
	tables[0] = df7test.Owner + ":0"
	df7.SetBootStore(df7test.Owner, failingBoot{})
	t.Cleanup(func() { df7.SetBootStore(df7test.Owner, nil) })
	d := NewRoute(f, df7test.Owner)
	r := Route{Table: 0, Label: 50016, EOS: true, EOSProto: PayloadIP4, Paths: paths(t, df7.Path{Type: df7.PathDrop})}
	if _, err := d.Create(ctx, df7.Encode(r)); err == nil {
		t.Fatal("a failed record must fail the Create")
	}
	if _, ok := routes[rkey{0, 50016, 1}]; ok || len(f.CallsNamed("mpls_route_add_del")) != 0 {
		t.Fatal("the label was added although its record could not be written")
	}
	df7.SetBootStore(df7test.Owner, nil)
	f.On("mpls_route_add_del", func(api.Message) ([]api.Message, error) {
		return []api.Message{&mpls.MplsRouteAddDelReply{Retval: -1}}, nil
	})
	if _, err := d.Create(ctx, df7.Encode(r)); err == nil {
		t.Fatal("failed add must surface")
	}
	if ok, err := d.Recorded(ctx, string(KeyRoute(0, 50016, true))); err != nil || ok {
		t.Fatalf("a failed add must drop its record: %v %v", ok, err)
	}
}

// TD-11b (review 3.3): MPLS on an untagged interface is claimed BEFORE the enable; a claim that
// cannot be recorded never enables it.
func TestInterfaceClaimFirst(t *testing.T) {
	f, tables, enabled, _, _ := fakeMPLS()
	ctx := t.Context()
	tables[0] = ""
	iface.SetClaimStore(df7test.Owner, failingClaims{})
	t.Cleanup(func() { iface.SetClaimStore(df7test.Owner, nil) })
	d := NewInterface(f, df7test.Owner)
	if _, err := d.Create(ctx, df7.Encode(Interface{Interface: "eth0"})); err == nil {
		t.Fatal("a failed claim must fail the Create")
	}
	if enabled[4] != 0 || len(f.CallsNamed("sw_interface_set_mpls_enable")) != 0 {
		t.Fatalf("MPLS enabled on eth0 without a claim (counter %d)", enabled[4])
	}
	// a failed enable releases the claim this Create made
	iface.SetClaimStore(df7test.Owner, nil)
	f.On("sw_interface_set_mpls_enable", func(api.Message) ([]api.Message, error) {
		return []api.Message{&mpls.SwInterfaceSetMplsEnableReply{Retval: -1}}, nil
	})
	if _, err := d.Create(ctx, df7.Encode(Interface{Interface: "eth0"})); err == nil {
		t.Fatal("failed enable must surface")
	}
	if df7test.Claimed(ctx, f, df7test.Owner, "eth0", "mpls-interface/eth0") {
		t.Fatal("the claim of a failed enable was kept")
	}
}

// TD-11b: the agent refuses to start unless every descriptor declares how it records ownership and
// records it in stores that survive an agent restart.
func TestOwnershipDeclarations(t *testing.T) {
	f, _, _, _, _ := fakeMPLS()
	const owner = "w5td11b"
	r := scheduler.NewRegistry()
	RegisterFor(r, f, owner, false)
	var volatile []string
	for _, n := range r.Names() {
		d, _ := r.Get(n)
		if err := persist.Declared(d); err != nil {
			t.Errorf("%s: %v", n, err)
		}
		if err := persist.Check(d); err != nil {
			volatile = append(volatile, n)
		}
	}
	if len(volatile) != 2 || volatile[0] != NameInterface || volatile[1] != NameRoute {
		t.Fatalf("with the in-memory defaults exactly mpls-interface and mpls-route must fail the persistence check: %v", volatile)
	}
	boot, err := dfkit.NewFileBootStore(filepath.Join(t.TempDir(), "boot.json"))
	if err != nil {
		t.Fatal(err)
	}
	df7.SetBootStore(owner, boot)
	iface.SetClaimStore(owner, persistedClaims{iface.NewMemoryClaimStore()})
	t.Cleanup(func() { df7.SetBootStore(owner, nil); iface.SetClaimStore(owner, nil) })
	for _, n := range r.Names() {
		d, _ := r.Get(n)
		if err := persist.Check(d); err != nil {
			t.Errorf("%s with persisted stores: %v", n, err)
		}
	}
}

// TD-11c: the tunnel creator provides interface/<name>, so objects on or through the tunnel
// interface order after it on create and before it on delete.
func TestTunnelProvidesInterfaceAlias(t *testing.T) {
	f, _, _, _, _ := fakeMPLS()
	var d scheduler.Descriptor = NewTunnel(f, df7test.Owner)
	kp, ok := d.(scheduler.KeyProvider)
	if !ok {
		t.Fatal("mpls-tunnel does not provide its interface alias")
	}
	tn := df7.Encode(Tunnel{Name: "t1", Paths: paths(t, df7.Path{Interface: "loop0", NextHop: "10.0.0.2", Labels: []df7.Label{{Label: 500}}})})
	if got := kp.ProvidedKeys(tn); len(got) != 1 || got[0] != "interface/t1" {
		t.Fatalf("provided keys %v", got)
	}
}

// Gaps: a binding in the default VRF depended on "vrf/0", which no object provides (never
// plannable); a pop-and-lookup path had no dependency on the VRF table it looks up in.
func TestDependenciesOfBindingsAndLookupPaths(t *testing.T) {
	f, _, _, _, _ := fakeMPLS()
	bd := NewIPBind(f, df7test.Owner)
	deps := bd.Dependencies(df7.Encode(IPBind{Label: 50040, VRF: 0, Prefix: "10.5.40.0/24"}))
	if len(deps) != 1 || deps[0].Key != "mpls-table/0" {
		t.Fatalf("default-VRF binding deps %v", deps)
	}
	rd := NewRoute(f, df7test.Owner)
	r := Route{Table: 0, Label: 50030, EOS: true, EOSProto: PayloadIP4, Paths: paths(t, df7.Path{TableID: 5010})}
	deps = rd.Dependencies(df7.Encode(r))
	if len(deps) != 2 || deps[1].Key != "vrf/5010" || deps[1].Optional {
		t.Fatalf("lookup path deps %v", deps)
	}
	td := NewTunnel(f, df7test.Owner)
	tn := Tunnel{Name: "t1", Paths: paths(t, df7.Path{NextHop: "10.5.1.2", TableID: 5010, Labels: []df7.Label{{Label: 50050}}})}
	if deps := td.Dependencies(df7.Encode(tn)); len(deps) != 1 || deps[0].Key != "vrf/5010" {
		t.Fatalf("recursive tunnel path deps %v", deps)
	}
}

type failingBoot struct{}

func (failingBoot) Get(string) (dfkit.BootRecord, bool) { return dfkit.BootRecord{}, false }
func (failingBoot) Put(dfkit.BootRecord) error          { return errors.New("disk full") }
func (failingBoot) Delete(string) error                 { return nil }

type failingClaims struct{}

func (failingClaims) Claim(string, string) error   { return errors.New("disk full") }
func (failingClaims) Release(string, string) error { return nil }
func (failingClaims) Claimed(string, string) bool  { return false }

// persistedClaims marks a claim store as persisted for the guard (dfkit/persist).
type persistedClaims struct{ iface.ClaimStore }

func (persistedClaims) Persistent() bool { return true }
