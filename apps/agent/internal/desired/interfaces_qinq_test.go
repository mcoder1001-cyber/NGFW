package desired_test

// F-vlan-qinq: stacked VLANs through P08's interfaces builder and assembler, with DF-1's real sub-interface
// descriptor on the fake VPP (coretest). The builder cases check the scheduler objects and the sub_if_flags they
// encode; the service cases run the product wiring (subsystems.Register + scheduler + agent.Service) end to end:
// apply → the fake VPP holds the tag stacks → Retrieve == canonical desired (assemble round-trip) → re-apply is
// empty → a tag change is a recreate (dependents follow) → loss + resync → rollback leaves nothing.

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"testing"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	ifapi "ngfw/agent/binapi/interface"
	"ngfw/agent/binapi/interface_types"
	vrxv1 "ngfw/agent/gen/vrx/v1"
	"ngfw/agent/internal/agent"
	"ngfw/agent/internal/descriptors/core"
	"ngfw/agent/internal/descriptors/core/coretest"
	iface "ngfw/agent/internal/descriptors/interface"
	"ngfw/agent/internal/desired"
	"ngfw/agent/internal/ownertable"
	"ngfw/agent/internal/scheduler"
	"ngfw/agent/internal/subsystems"
)

const qinqOwner = "w5"

// qinqDoc: on the slot rig parent host-w5w0, .100 = dot1q 100, .200 = dot1ad 200 + dot1q 100 (QinQ), .300 = dot1q 300 +
// dot1q 30 (dot1q-in-dot1q), each with an address. .1200 reuses the stack of .200 as dot1q (distinct from dot1ad).
const qinqDoc = `{
  "interfaces": {
    "host-w5w0": {"enabled": true, "ipv4": ["10.5.2.1/24"], "subinterfaces": {
      "100":  {"vlanId": 100, "enabled": true, "ipv4": ["10.5.100.1/24"]},
      "200":  {"vlanId": 200, "innerVlanId": 100, "dot1ad": true, "enabled": true, "description": "qinq", "ipv4": ["10.5.200.1/24"]},
      "300":  {"vlanId": 300, "innerVlanId": 30, "enabled": true, "ipv6": ["2001:db8:5:300::1/64"]},
      "1200": {"vlanId": 200, "innerVlanId": 100, "ipv4": ["10.5.201.1/24"]}
    }}
  }
}`

// canonicalQinqDoc is what Retrieve must return for qinqDoc: Zod defaults explicit (D-039) — dot1ad false on the
// dot1q stacks, no innerVlanId on the single-tag one, enabled false where the document leaves it out.
const canonicalQinqDoc = `{
  "interfaces": {
    "host-w5w0": {"enabled": true, "promiscuous": false, "vrf": "default", "ipv4": ["10.5.2.1/24"], "subinterfaces": {
      "100":  {"vlanId": 100, "dot1ad": false, "enabled": true, "vrf": "default", "ipv4": ["10.5.100.1/24"]},
      "200":  {"vlanId": 200, "innerVlanId": 100, "dot1ad": true, "enabled": true, "vrf": "default", "description": "qinq", "ipv4": ["10.5.200.1/24"]},
      "300":  {"vlanId": 300, "innerVlanId": 30, "dot1ad": false, "enabled": true, "vrf": "default", "ipv6": ["2001:db8:5:300::1/64"]},
      "1200": {"vlanId": 200, "innerVlanId": 100, "dot1ad": false, "enabled": false, "vrf": "default", "ipv4": ["10.5.201.1/24"]}
    }}
  }
}`

func qinqParse(t *testing.T, js string) *vrxv1.DesiredState {
	t.Helper()
	ds := &vrxv1.DesiredState{}
	if err := protojson.Unmarshal([]byte(js), ds); err != nil {
		t.Fatalf("doc: %v", err)
	}
	return ds
}

// every netdev is a veth (the D-105 guard is P08's and tested there)
func vethOnly(string) (string, bool, error) { return "veth", true, nil }

// ---- builder -------------------------------------------------------------------------------------------------

type recorder struct {
	objs map[scheduler.Key]proto.Message
	ptrs map[scheduler.Key]string
	errs []string
}

func (r *recorder) Add(k scheduler.Key, v proto.Message, pointer string) {
	r.objs[k], r.ptrs[k] = v, pointer
}

func (r *recorder) Errorf(pointer, rule, format string, a ...any) {
	r.errs = append(r.errs, pointer+" "+rule+": "+fmt.Sprintf(format, a...))
}

func (r *recorder) Warnf(string, string, string, ...any) {}

func build(t *testing.T, js string) *recorder {
	t.Helper()
	r := &recorder{objs: map[scheduler.Key]proto.Message{}, ptrs: map[scheduler.Key]string{}}
	desired.Interfaces(r, qinqParse(t, js).GetInterfaces(), func(n string) (uint32, bool) { return 0, n == "default" }, vethOnly)
	if len(r.errs) != 0 {
		t.Fatalf("builder errors %v", r.errs)
	}
	return r
}

func TestQinQBuilderObjects(t *testing.T) {
	r := build(t, qinqDoc)
	const (
		one   = interface_types.SUB_IF_API_FLAG_ONE_TAG
		two   = interface_types.SUB_IF_API_FLAG_TWO_TAGS
		ad    = interface_types.SUB_IF_API_FLAG_DOT1AD
		exact = interface_types.SUB_IF_API_FLAG_EXACT_MATCH
	)
	for _, c := range []struct {
		id           string
		outer, inner uint32
		dot1ad       bool
		flags        interface_types.SubIfFlags
	}{
		{"100", 100, 0, false, one | exact},       // dot1q
		{"200", 200, 100, true, two | ad | exact}, // dot1ad outer 200 + dot1q inner 100
		{"300", 300, 30, false, two | exact},      // dot1q-in-dot1q
		{"1200", 200, 100, false, two | exact},    // same numbers as .200, 802.1Q outer
	} {
		sname := desired.SubName("host-w5w0", c.id)
		k := scheduler.Join(iface.SubinterfaceName, sname)
		got, ok := r.objs[k].(*iface.Subinterface)
		if !ok {
			t.Fatalf("%s: no sub-interface object (have %v)", k, keys(r))
		}
		want := &iface.Subinterface{Parent: "interface/host-w5w0", SubId: atou(c.id), OuterVlan: c.outer, InnerVlan: c.inner, Dot1Ad: c.dot1ad, ExactMatch: true}
		if !proto.Equal(got, want) {
			t.Errorf("%s = %v, want %v", k, got, want)
		}
		if f := iface.SubifFlags(got); f != c.flags {
			t.Errorf("%s: sub_if_flags %v, want %v", k, f, c.flags)
		}
		if p := r.ptrs[k]; p != "/interfaces/host-w5w0/subinterfaces/"+c.id {
			t.Errorf("%s pointer %q", k, p)
		}
		// the alias carries the creator; every attribute of the sub-interface depends on the alias (D-065/D-069)
		al, ok := r.objs[iface.AliasKey(sname)].(*iface.InterfaceAlias)
		if !ok || al.GetName() != sname || al.GetCreator() != string(k) {
			t.Errorf("alias of %s = %v", sname, al)
		}
	}
	if _, ok := r.objs[core.InterfaceAddrKey("host-w5w0.200", "10.5.200.1/24")]; !ok {
		t.Errorf("address of the QinQ sub-interface missing: %v", keys(r))
	}
	if _, ok := r.objs[scheduler.Join(iface.AdminStateName, "host-w5w0.1200")]; ok {
		t.Error("admin-state object for a sub-interface that is not enabled")
	}
	// decoding a VPP dump row of the QinQ sub-interface gives the object back (Retrieve side of the descriptor)
	so := r.objs[scheduler.Join(iface.SubinterfaceName, "host-w5w0.200")].(*iface.Subinterface)
	row := subifRow(so)
	if back := iface.DecodeSubif("interface/host-w5w0", row); !proto.Equal(back, so) {
		t.Errorf("DecodeSubif(dump row) = %v, want %v", back, so)
	}
}

// subifRow is what VPP 26.06 reports for a sub-interface (interface_api.c send_sw_interface_details: number of tags =
// one_tag + 2*two_tags, flags masked with SUB_IF_API_FLAG_MASK_VNET).
func subifRow(o *iface.Subinterface) *ifapi.SwInterfaceDetails {
	f := iface.SubifFlags(o)
	d := &ifapi.SwInterfaceDetails{SubID: o.GetSubId(), SubOuterVlanID: uint16(o.GetOuterVlan()), SubInnerVlanID: uint16(o.GetInnerVlan())} //nolint:gosec // test values ≤ 4094
	d.SubIfFlags = f & interface_types.SUB_IF_API_FLAG_MASK_VNET
	if f&interface_types.SUB_IF_API_FLAG_ONE_TAG != 0 {
		d.SubNumberOfTags = 1
	}
	if f&interface_types.SUB_IF_API_FLAG_TWO_TAGS != 0 {
		d.SubNumberOfTags += 2
	}
	return d
}

func keys(r *recorder) []string {
	var out []string
	for k := range r.objs {
		out = append(out, string(k))
	}
	return out
}

func atou(s string) uint32 {
	n, err := strconv.ParseUint(s, 10, 32)
	if err != nil {
		panic(err)
	}
	return uint32(n)
}

// ---- service on the fake VPP -------------------------------------------------------------------------------------

func newQinqSvc(t *testing.T, v *coretest.VPP, dir string) *agent.Service {
	t.Helper()
	owned, err := ownertable.Open(dir, qinqOwner)
	if err != nil {
		t.Fatal(err)
	}
	reg := scheduler.NewRegistry()
	w, err := subsystems.Register(reg, subsystems.Env{Client: v, Owner: qinqOwner, StateDir: dir, Owned: owned, NetdevKind: vethOnly})
	if err != nil {
		t.Fatal(err)
	}
	w.Connected(context.Background())
	sched := scheduler.New(reg, nil)
	sched.VerifyRetries = 0
	svc, err := agent.NewService(agent.ServiceConfig{Owner: qinqOwner, Version: "test", VPP: v, Scheduler: sched, StateDir: dir, BeforeTxn: w.BeforeTxn, NetdevKind: w.NetdevKind()})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(svc.Close)
	return svc
}

func applyDoc(t *testing.T, s *agent.Service, txn string, ds *vrxv1.DesiredState) *vrxv1.ApplyResponse {
	t.Helper()
	resp, err := s.Apply(context.Background(), &vrxv1.ApplyRequest{TxnId: txn, DesiredState: ds})
	if err != nil {
		t.Fatalf("apply %s: %v", txn, err)
	}
	if resp.GetStatus() != vrxv1.ApplyStatus_APPLY_STATUS_APPLIED {
		t.Fatalf("apply %s: %s %s results=%v validation=%v", txn, resp.GetStatus(), resp.GetMessage(), resp.GetResults(), resp.GetValidation())
	}
	return resp
}

func retrieved(t *testing.T, s *agent.Service) *vrxv1.DesiredState {
	t.Helper()
	got, err := s.Retrieve(context.Background(), &vrxv1.RetrieveRequest{Subsystems: []string{"interfaces", "vrfs"}})
	if err != nil {
		t.Fatal(err)
	}
	return got.GetDesiredState()
}

func mustRetrieve(t *testing.T, s *agent.Service, want string) {
	t.Helper()
	if got := retrieved(t, s); !proto.Equal(got, qinqParse(t, want)) {
		t.Fatalf("Retrieve != canonical desired:\n%s", protojson.Format(got))
	}
}

// vppSub asserts the tag stack VPP holds for a sub-interface.
func vppSub(t *testing.T, v *coretest.VPP, name string, outer, inner uint16, flags interface_types.SubIfFlags) coretest.Iface {
	t.Helper()
	i, ok := v.InterfaceByName(name)
	if !ok || !i.IsSub {
		t.Fatalf("%s not in VPP: %s", name, v.Snapshot())
	}
	if i.Outer != outer || i.Inner != inner || i.SubFlags != flags {
		t.Fatalf("%s in VPP: outer %d inner %d flags %v, want %d %d %v", name, i.Outer, i.Inner, i.SubFlags, outer, inner, flags)
	}
	if i.Tag != qinqOwner+":"+name {
		t.Fatalf("%s tag %q", name, i.Tag)
	}
	return i
}

func byKey(resp *vrxv1.ApplyResponse) map[string]*vrxv1.ObjectResult {
	out := map[string]*vrxv1.ObjectResult{}
	for _, r := range resp.GetResults() {
		out[r.GetKey()] = r
	}
	return out
}

func TestQinQRoundTripOnFake(t *testing.T) {
	const (
		one   = interface_types.SUB_IF_API_FLAG_ONE_TAG
		two   = interface_types.SUB_IF_API_FLAG_TWO_TAGS
		ad    = interface_types.SUB_IF_API_FLAG_DOT1AD
		exact = interface_types.SUB_IF_API_FLAG_EXACT_MATCH
	)
	v := coretest.New()
	dir := t.TempDir()
	s := newQinqSvc(t, v, dir)
	resp := applyDoc(t, s, "q1", qinqParse(t, qinqDoc))
	res := byKey(resp)
	for k, ptr := range map[string]string{
		"interface.subinterface/host-w5w0.200":     "/interfaces/host-w5w0/subinterfaces/200",
		"interface/host-w5w0.200":                  "/interfaces/host-w5w0/subinterfaces/200",
		"interface.admin-state/host-w5w0.200":      "/interfaces/host-w5w0/subinterfaces/200/enabled",
		"interface-ip/host-w5w0.200/10.5.200.1/24": "/interfaces/host-w5w0/subinterfaces/200/ipv4/0",
		"interface.subinterface/host-w5w0.300":     "/interfaces/host-w5w0/subinterfaces/300",
	} {
		if r := res[k]; r == nil || r.GetOp() != vrxv1.ApplyOperation_APPLY_OPERATION_CREATE || r.GetPointer() != ptr || r.GetSubsystem() != "interfaces" {
			t.Fatalf("result %s = %v, want a create at %s", k, r, ptr)
		}
	}
	vppSub(t, v, "host-w5w0.100", 100, 0, one|exact)
	q := vppSub(t, v, "host-w5w0.200", 200, 100, two|ad|exact)
	vppSub(t, v, "host-w5w0.300", 300, 30, two|exact)
	vppSub(t, v, "host-w5w0.1200", 200, 100, two|exact)
	if !q.AdminUp || !q.Addrs["10.5.200.1/24"] || v.AdminUp("host-w5w0.1200") {
		t.Fatalf("QinQ attributes in VPP: %s", v.Snapshot())
	}

	// assemble round-trip: Retrieve == canonical desired; both documents re-apply as empty plans
	mustRetrieve(t, s, canonicalQinqDoc)
	for txn, js := range map[string]string{"q2": qinqDoc, "q3": canonicalQinqDoc} {
		if r := applyDoc(t, s, txn, qinqParse(t, js)); len(r.GetResults()) != 0 {
			t.Fatalf("re-apply %s changed %v", txn, r.GetResults())
		}
	}

	// InterfaceState reports the tag stack of every sub-interface with its parent
	st, err := s.InterfaceState(context.Background(), &vrxv1.InterfaceStateRequest{})
	if err != nil {
		t.Fatal(err)
	}
	live := map[string]*vrxv1.InterfaceState{}
	for _, i := range st.GetInterfaces() {
		live[i.GetName()] = i
	}
	for name, want := range map[string][2]uint32{"host-w5w0.100": {100, 0}, "host-w5w0.200": {200, 100}, "host-w5w0.300": {300, 30}} {
		i := live[name]
		if i == nil || i.GetType() != "sub-interface" || i.GetParent() != "host-w5w0" || i.GetVlanId() != want[0] || i.GetInnerVlanId() != want[1] || !i.GetManaged() {
			t.Fatalf("InterfaceState %s = %v, want vlan %v", name, i, want)
		}
	}
	if d := live["host-w5w0.200"].GetDescription(); d != "qinq" {
		t.Fatalf("description %q", d)
	}

	// a tag change on the same sub-id is a recreate: the sub-interface is deleted and created with the new stack,
	// its alias, admin state and address follow onto the new sw_if_index
	changed := qinqParse(t, qinqDoc)
	changed.Interfaces["host-w5w0"].Subinterfaces["200"].InnerVlanId = proto.Uint32(101)
	resp = applyDoc(t, s, "q4", changed)
	res = byKey(resp)
	if r := res["interface.subinterface/host-w5w0.200"]; r == nil || r.GetOp() != vrxv1.ApplyOperation_APPLY_OPERATION_RECREATE {
		t.Fatalf("inner tag change: %v", resp.GetResults())
	}
	for k := range res {
		if strings.Contains(k, "host-w5w0.100") || strings.Contains(k, "host-w5w0.300") || strings.Contains(k, "host-w5w0.1200") {
			t.Fatalf("a tag change of .200 touched %s: %v", k, resp.GetResults())
		}
	}
	q2 := vppSub(t, v, "host-w5w0.200", 200, 101, two|ad|exact)
	if !q2.AdminUp || !q2.Addrs["10.5.200.1/24"] {
		t.Fatalf("dependents not recreated on the new sub-interface: %s", v.Snapshot())
	}
	wantChanged := strings.Replace(canonicalQinqDoc, `"vlanId": 200, "innerVlanId": 100, "dot1ad": true`, `"vlanId": 200, "innerVlanId": 101, "dot1ad": true`, 1)
	mustRetrieve(t, s, wantChanged)

	// dot1q ↔ dot1ad is a tag change too (dot1q-in-dot1q .300 becomes dot1ad 300 + dot1q 30)
	changed.Interfaces["host-w5w0"].Subinterfaces["300"].Dot1Ad = proto.Bool(true)
	resp = applyDoc(t, s, "q5", changed)
	if r := byKey(resp)["interface.subinterface/host-w5w0.300"]; r == nil || r.GetOp() != vrxv1.ApplyOperation_APPLY_OPERATION_RECREATE {
		t.Fatalf("dot1ad toggle: %v", resp.GetResults())
	}
	vppSub(t, v, "host-w5w0.300", 300, 30, two|ad|exact)
	// QinQ → single tag (inner tag removed)
	changed.Interfaces["host-w5w0"].Subinterfaces["300"].InnerVlanId = nil
	resp = applyDoc(t, s, "q6", changed)
	if r := byKey(resp)["interface.subinterface/host-w5w0.300"]; r == nil || r.GetOp() != vrxv1.ApplyOperation_APPLY_OPERATION_RECREATE {
		t.Fatalf("inner tag removed: %v", resp.GetResults())
	}
	vppSub(t, v, "host-w5w0.300", 300, 0, one|ad|exact)

	// back to the original document, then a simulated loss of both QinQ sub-interfaces (with their addresses)
	applyDoc(t, s, "q7", qinqParse(t, qinqDoc))
	mustRetrieve(t, s, canonicalQinqDoc)
	for _, n := range []string{"host-w5w0.200", "host-w5w0.300"} {
		if !v.DeleteInterface(n) {
			t.Fatalf("loss of %s", n)
		}
	}
	if r := s.Resync(context.Background()); r.GetStatus() != vrxv1.ApplyStatus_APPLY_STATUS_APPLIED || r.GetSummary().GetCreated() == 0 {
		t.Fatalf("resync after loss %v", r)
	}
	vppSub(t, v, "host-w5w0.200", 200, 100, two|ad|exact)
	vppSub(t, v, "host-w5w0.300", 300, 30, two|exact)
	mustRetrieve(t, s, canonicalQinqDoc)

	// a restarted agent (new service, same state dir) converges without re-creating anything
	s.Close()
	s2 := newQinqSvc(t, v, dir)
	v.Reset()
	if r := s2.Resync(context.Background()); r.GetStatus() != vrxv1.ApplyStatus_APPLY_STATUS_APPLIED {
		t.Fatalf("restart resync %v", r)
	}
	for _, c := range v.Calls() {
		if n := c.GetMessageName(); n == "create_subif" || n == "delete_subif" || n == "sw_interface_add_del_address" {
			t.Fatalf("restart resync sent %s", n)
		}
	}
	mustRetrieve(t, s2, canonicalQinqDoc)

	// rollback to a revision without sub-interfaces: every sub-interface leaves VPP, the parent stays
	base := qinqParse(t, qinqDoc)
	base.Interfaces["host-w5w0"].Subinterfaces = nil
	resp = applyDoc(t, s2, "q8", base)
	if n := resp.GetSummary().GetDeleted(); n == 0 {
		t.Fatalf("rollback deleted nothing: %v", resp.GetResults())
	}
	attributesFirst(t, resp)
	for _, n := range []string{"host-w5w0.100", "host-w5w0.200", "host-w5w0.300", "host-w5w0.1200"} {
		if _, ok := v.InterfaceByName(n); ok {
			t.Fatalf("%s left in VPP after rollback: %s", n, v.Snapshot())
		}
	}
	got := retrieved(t, s2).GetInterfaces()["host-w5w0"]
	if got == nil || len(got.GetSubinterfaces()) != 0 {
		t.Fatalf("Retrieve after rollback: %v", got)
	}
}

// Removing sub-interfaces while their parent stays (a rollback to a revision without them): the attributes of each
// sub-interface (admin state, addresses) are deleted BEFORE the sub-interface itself, and the transaction applies.
// The attributes depend on the observe-only alias interface/<parent>.<id>, which a delete plan never contains; the
// sub-interface provides that key (F-vlan-qinq defect fix in descriptors/interface/subinterface.go), which gives the
// plan the edge. Without it the sub-interface (which depends on the parent's alias, a plan node) was ordered after its
// attributes and so deleted first; the admin-state delete then hit a stale sw_if_index and the rollback rolled back.
func TestQinQDeleteWhileParentStays(t *testing.T) {
	v := coretest.New()
	s := newQinqSvc(t, v, t.TempDir())
	applyDoc(t, s, "d1", qinqParse(t, qinqDoc))
	one := qinqParse(t, qinqDoc)
	delete(one.Interfaces["host-w5w0"].Subinterfaces, "200") // the QinQ sub-interface only
	resp := applyDoc(t, s, "d2", one)
	pos := map[string]int{}
	for i, r := range resp.GetResults() {
		if r.GetOp() != vrxv1.ApplyOperation_APPLY_OPERATION_DELETE || r.GetCode() != vrxv1.ObjectResultCode_OBJECT_RESULT_CODE_OK {
			t.Fatalf("result %v", r)
		}
		pos[r.GetKey()] = i
	}
	sub, ok := pos["interface.subinterface/host-w5w0.200"]
	if !ok || len(pos) != 3 {
		t.Fatalf("deletes %v", resp.GetResults())
	}
	for _, k := range []string{"interface.admin-state/host-w5w0.200", "interface-ip/host-w5w0.200/10.5.200.1/24"} {
		if p, ok := pos[k]; !ok || p > sub {
			t.Fatalf("%s is not deleted before the sub-interface: %v", k, resp.GetResults())
		}
	}
	if _, ok := v.InterfaceByName("host-w5w0.200"); ok {
		t.Fatalf("host-w5w0.200 still in VPP: %s", v.Snapshot())
	}
	for _, n := range []string{"host-w5w0.100", "host-w5w0.300", "host-w5w0.1200"} {
		if _, ok := v.InterfaceByName(n); !ok {
			t.Fatalf("%s was removed too: %s", n, v.Snapshot())
		}
	}
	// and all remaining sub-interfaces in one go (the rollback of the acceptance)
	none := qinqParse(t, qinqDoc)
	none.Interfaces["host-w5w0"].Subinterfaces = nil
	resp = applyDoc(t, s, "d3", none)
	// 3 sub-interfaces + 2 admin states (.1200 is not enabled) + 3 addresses; aliases are observe-only (D-065)
	if resp.GetSummary().GetDeleted() != 8 || len(resp.GetResults()) != 8 {
		t.Fatalf("deletes: %v", resp.GetResults())
	}
	attributesFirst(t, resp)
	if got := retrieved(t, s).GetInterfaces()["host-w5w0"]; len(got.GetSubinterfaces()) != 0 {
		t.Fatalf("Retrieve after removing every sub-interface: %v", got)
	}
}

// attributesFirst: every object of a deleted sub-interface <parent>.<id> is deleted before the sub-interface itself.
func attributesFirst(t *testing.T, resp *vrxv1.ApplyResponse) {
	t.Helper()
	pos := map[string]int{}
	for i, r := range resp.GetResults() {
		pos[r.GetKey()] = i
	}
	for k, p := range pos {
		name, ok := strings.CutPrefix(k, iface.SubinterfaceName+"/")
		if !ok {
			continue
		}
		for other, q := range pos {
			if other != k && (strings.HasSuffix(other, "/"+name) || strings.Contains(other, "/"+name+"/")) && q > p {
				t.Fatalf("%s is deleted after %s: %v", other, k, resp.GetResults())
			}
		}
	}
}
