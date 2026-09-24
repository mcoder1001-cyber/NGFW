package agent

import (
	"context"
	"os"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	vrxv1 "ngfw/agent/gen/vrx/v1"
	"ngfw/agent/internal/descriptors/core/coretest"
	"ngfw/agent/internal/descriptors/dfkit"
	"ngfw/agent/internal/vpp"
	"ngfw/agent/internal/vpp/bootid"
)

// fakeBootIdentity gives the fake VPP (vpe_pid 0, no /proc entry) a complete D-080 identity for
// the mactime applied-once records, as dfkittest.NewFake does for the DF-8 packages.
func fakeBootIdentity(t *testing.T) {
	t.Helper()
	prev := dfkit.IdentitySource
	dfkit.IdentitySource = func(ctx context.Context, c vpp.Client) (bootid.Identity, error) {
		id, err := bootid.Reader{ProcRoot: os.DevNull}.Current(ctx, c)
		if err != nil {
			return bootid.Identity{}, err
		}
		id.BootID, id.StartTime = "fake", 1
		return id, nil
	}
	t.Cleanup(func() { dfkit.IdentitySource = prev })
}

// l2Doc is an F-bridge-l2 document as the API sends it (Zod defaults filled): bridge domain "lan"
// (7001) with a loopback BVI, an af_packet member with the MAC filter, a bridged VLAN sub-interface
// with pop-1 and split-horizon 1, a static MAC; a bidirectional L2 cross-connect with a translate
// rewrite on the sub-interface rx; an L3 cross-connect; a time-range MAC filter device.
const l2Doc = `{
  "vrfs": {"default": {"id": 0}},
  "interfaces": {
    "loop7000": {"enabled": true, "promiscuous": false, "vrf": "default", "ipv4": ["10.7.0.1/24"],
      "l2": {"bridgeDomain": "lan", "shg": 0, "bvi": true, "uuFwd": false, "macFilter": false}},
    "host-w7l0": {"enabled": true, "promiscuous": false, "vrf": "default",
      "l2": {"bridgeDomain": "lan", "shg": 0, "bvi": false, "uuFwd": false, "macFilter": true},
      "subinterfaces": {"100": {"vlanId": 100, "enabled": true, "dot1ad": false, "vrf": "default",
        "l2": {"bridgeDomain": "lan", "shg": 1, "bvi": false, "uuFwd": false, "macFilter": false, "tagRewrite": {"op": "pop-1", "dot1ad": false}}}}},
    "host-w7w0": {"enabled": true, "promiscuous": false, "vrf": "default",
      "subinterfaces": {"200": {"vlanId": 200, "enabled": true, "dot1ad": false, "vrf": "default",
        "l2": {"shg": 0, "bvi": false, "uuFwd": false, "macFilter": false, "tagRewrite": {"op": "translate-1-1", "tag1": 300, "dot1ad": true}}}}},
    "loop7001": {"enabled": true, "promiscuous": false, "vrf": "default", "ipv4": ["10.7.1.1/24"]}
  },
  "routing": {"l2": {
    "bridgeDomains": {"lan": {"id": 7001, "flood": true, "uuFlood": true, "forward": true, "learn": true, "arpTerm": false, "macAgeMin": 5,
      "staticMacs": [{"mac": "02:00:00:00:70:01", "interface": "host-w7l0"}]}},
    "xconnects": {"host-w7w0.200": {"tx": "host-w7w0"}, "host-w7w0": {"tx": "host-w7w0.200"}},
    "l3xc": {"loop7001": {"ipv4Paths": [{"nextHop": "10.7.1.254", "interface": "loop7001", "vrf": "default", "weight": 1, "preference": 0}]}},
    "macFilters": {"kids": {"mac": "02:00:00:00:70:09", "action": "allow",
      "ranges": [{"days": ["mon", "tue"], "start": "16:00", "end": "20:00"}, {"days": ["sat"], "start": "09:00", "end": "24:00"}]}}
  }}
}`

// withoutL2 is l2Doc after the rollback: every interface back in L3, no routing.l2.
const withoutL2 = `{
  "vrfs": {"default": {"id": 0}},
  "interfaces": {
    "loop7000": {"enabled": true, "ipv4": ["10.7.0.1/24"]},
    "host-w7l0": {"enabled": true, "subinterfaces": {"100": {"vlanId": 100, "enabled": true}}},
    "host-w7w0": {"enabled": true, "subinterfaces": {"200": {"vlanId": 200, "enabled": true}}},
    "loop7001": {"enabled": true, "ipv4": ["10.7.1.1/24"]}
  },
  "routing": {}
}`

func retrieveAll(t *testing.T, s *Service) *vrxv1.DesiredState {
	t.Helper()
	got, err := s.Retrieve(context.Background(), &vrxv1.RetrieveRequest{Subsystems: []string{"interfaces", "vrfs", "routing"}})
	if err != nil {
		t.Fatal(err)
	}
	return got.GetDesiredState()
}

// l2Parts is the F-bridge-l2 half of a document: routing.l2 and every l2 leaf by interface name.
func l2Parts(ds *vrxv1.DesiredState) (*vrxv1.BridgeL2Config, map[string]*vrxv1.BridgeL2Port) {
	ports := map[string]*vrxv1.BridgeL2Port{}
	for n, itf := range ds.GetInterfaces() {
		if itf.GetL2() != nil {
			ports[n] = itf.GetL2()
		}
		for id, sub := range itf.GetSubinterfaces() {
			if sub.GetL2() != nil {
				ports[n+"."+id] = sub.GetL2()
			}
		}
	}
	return ds.GetRouting().GetL2(), ports
}

func assertL2Equal(t *testing.T, got, want *vrxv1.DesiredState) {
	t.Helper()
	gc, gp := l2Parts(got)
	wc, wp := l2Parts(want)
	if !proto.Equal(gc, wc) {
		t.Fatalf("routing.l2 retrieved:\n%s\nwant:\n%s", protojson.Format(gc), protojson.Format(wc))
	}
	if len(gp) != len(wp) {
		t.Fatalf("l2 leaves retrieved on %d interfaces, want %d: %v", len(gp), len(wp), gp)
	}
	for n, p := range wp {
		if !proto.Equal(gp[n], p) {
			t.Fatalf("interfaces %s l2 retrieved %v, want %v", n, gp[n], p)
		}
	}
}

func TestBridgeL2OnFake(t *testing.T) {
	fakeBootIdentity(t)
	v := coretest.New()
	dir := t.TempDir()
	s := newSvc(t, v, dir)
	resp := apply(t, s, &vrxv1.ApplyRequest{TxnId: "b1", DesiredState: doc(t, l2Doc)})
	mustStatus(t, resp, vrxv1.ApplyStatus_APPLY_STATUS_APPLIED)
	ptrs := map[string]string{}
	for _, r := range resp.GetResults() {
		ptrs[r.GetKey()] = r.GetPointer()
	}
	for k, p := range map[string]string{
		"l2.bridge-domain/7001":                      "/routing/l2/bridgeDomains/lan",
		"l2.bridge-domain-member/7001/host-w7l0.100": "/interfaces/host-w7l0/subinterfaces/100/l2/bridgeDomain",
		"l2.vlan-tag-rewrite/host-w7l0.100":          "/interfaces/host-w7l0/subinterfaces/100/l2/tagRewrite",
		"l2.xconnect/host-w7w0.200":                  "/routing/l2/xconnects/host-w7w0.200",
		"l2.fib-entry/7001/02:00:00:00:70:01":        "/routing/l2/bridgeDomains/lan/staticMacs/0",
		"l3xc.l3xc/loop7001/ip4":                     "/routing/l2/l3xc/loop7001/ipv4Paths",
		"mactime.range/kids":                         "/routing/l2/macFilters/kids",
		"mactime.enable/host-w7l0":                   "/interfaces/host-w7l0/l2/macFilter",
		"l2.bridge-domain-member/7001/loop7000":      "/interfaces/loop7000/l2/bridgeDomain",
		"l2.vlan-tag-rewrite/host-w7w0.200":          "/interfaces/host-w7w0/subinterfaces/200/l2/tagRewrite",
		"l2.bridge-domain-member/7001/host-w7l0":     "/interfaces/host-w7l0/l2/bridgeDomain",
		"l2.xconnect/host-w7w0":                      "/routing/l2/xconnects/host-w7w0",
	} {
		if ptrs[k] != p {
			t.Errorf("result %s pointer %q, want %q", k, ptrs[k], p)
		}
	}
	members := v.BridgeMembers()
	if members["loop7000"] != 7001 || members["host-w7l0"] != 7001 || members["host-w7l0.100"] != 7001 || len(members) != 3 {
		t.Fatalf("members %v", members)
	}
	if n := v.MactimeCount("host-w7l0"); n != 1 {
		t.Fatalf("mactime count %d", n)
	}
	want := doc(t, l2Doc)
	got := retrieveAll(t, s)
	assertL2Equal(t, got, want)

	// idempotent: nothing to do, the mactime feature does not stack; the retrieved document re-applies as a no-op
	resp = apply(t, s, &vrxv1.ApplyRequest{TxnId: "b2", DesiredState: doc(t, l2Doc)})
	mustStatus(t, resp, vrxv1.ApplyStatus_APPLY_STATUS_APPLIED)
	if len(resp.GetResults()) != 0 || v.MactimeCount("host-w7l0") != 1 {
		t.Fatalf("re-apply changed %v (mactime %d)", resp.GetResults(), v.MactimeCount("host-w7l0"))
	}
	resp = apply(t, s, &vrxv1.ApplyRequest{TxnId: "b3", DesiredState: got})
	if len(resp.GetResults()) != 0 {
		t.Fatalf("re-applying the retrieved document changed %v", resp.GetResults())
	}

	// live state
	st, err := s.BridgeDomainState(context.Background(), &vrxv1.BridgeDomainStateRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if len(st.GetBridgeDomains()) != 1 {
		t.Fatalf("state %v", st)
	}
	bd := st.GetBridgeDomains()[0]
	if bd.GetId() != 7001 || bd.GetName() != "lan" || bd.GetBvi() != "loop7000" || bd.GetMacAgeMin() != 5 || len(bd.GetMembers()) != 3 || bd.GetStaticMacs() != 2 || bd.GetLearnedMacs() != 0 {
		t.Fatalf("bridge domain state %v", bd)
	}
	for _, m := range bd.GetMembers() {
		if m.GetInterface() == "host-w7l0.100" && (m.GetShg() != 1 || m.GetTagRewrite() != "pop-1" || m.GetPortType() != "normal") {
			t.Fatalf("member %v", m)
		}
		if m.GetInterface() == "loop7000" && m.GetPortType() != "bvi" {
			t.Fatalf("BVI member %v", m)
		}
	}
	page, err := s.BridgeDomainMacs(context.Background(), &vrxv1.BridgeDomainMacsRequest{BdId: 7001, Limit: 1})
	if err != nil || page.GetTotal() != 2 || len(page.GetMacs()) != 1 || page.GetMacs()[0].GetMac() != "02:00:00:00:70:01" || page.GetMacs()[0].GetInterface() != "host-w7l0" || !page.GetMacs()[0].GetStatic() {
		t.Fatalf("macs page %v %v", page, err)
	}
	if page, err := s.BridgeDomainMacs(context.Background(), &vrxv1.BridgeDomainMacsRequest{BdId: 7001, Offset: 1, Limit: 5}); err != nil || len(page.GetMacs()) != 1 || !page.GetMacs()[0].GetBvi() {
		t.Fatalf("second page %v %v", page, err)
	}
	if _, err := s.BridgeDomainMacs(context.Background(), &vrxv1.BridgeDomainMacsRequest{BdId: 7001, Limit: 1001}); grpcCode(err) != codes.InvalidArgument {
		t.Fatalf("limit 1001: %v", err)
	}
	if _, err := s.BridgeDomainMacs(context.Background(), &vrxv1.BridgeDomainMacsRequest{BdId: 9999}); grpcCode(err) != codes.NotFound {
		t.Fatalf("foreign bd: %v", err)
	}

	// restart simulation: the agent stops, its L2 objects vanish behind its back (dependents first,
	// D-095c), a new agent over the same state dir reconciles them back
	s.Close()
	v.DropBridgeL2()
	s2 := newSvc(t, v, dir)
	if r := s2.Resync(context.Background()); r.GetStatus() != vrxv1.ApplyStatus_APPLY_STATUS_APPLIED || r.GetSummary().GetCreated() == 0 {
		t.Fatalf("resync %v", r)
	}
	assertL2Equal(t, retrieveAll(t, s2), want)
	if n := v.MactimeCount("host-w7l0"); n != 1 {
		t.Fatalf("mactime after restart %d", n)
	}

	// rollback: every member back in L3, nothing of routing.l2 left
	mustStatus(t, apply(t, s2, &vrxv1.ApplyRequest{TxnId: "b4", DesiredState: doc(t, withoutL2)}), vrxv1.ApplyStatus_APPLY_STATUS_APPLIED)
	if len(v.BridgeMembers()) != 0 || len(v.BridgeDomainIDs()) != 0 || v.MactimeCount("host-w7l0") != 0 {
		t.Fatalf("after rollback: members %v bds %v mactime %d", v.BridgeMembers(), v.BridgeDomainIDs(), v.MactimeCount("host-w7l0"))
	}
	if c, p := l2Parts(retrieveAll(t, s2)); c != nil || len(p) != 0 {
		t.Fatalf("after rollback Retrieve still has %v %v", c, p)
	}
}

func TestBridgeL2Validation(t *testing.T) {
	v := coretest.New()
	s := newSvc(t, v, t.TempDir())
	resp := apply(t, s, &vrxv1.ApplyRequest{TxnId: "v1", DesiredState: doc(t, `{
	  "interfaces": {"loop7000": {"l2": {"bridgeDomain": "nope"}}, "loop7001": {"l2": {"tagRewrite": {"op": "pop-1"}}}},
	  "routing": {"l2": {"l3xc": {"loop7000": {"ipv4Paths": [{"vrf": "missing"}]}}}}
	}`)})
	mustStatus(t, resp, vrxv1.ApplyStatus_APPLY_STATUS_FAILED)
	got := map[string]string{}
	for _, is := range resp.GetValidation().GetErrors() {
		got[is.GetPointer()] = is.GetRule()
	}
	for p, rule := range map[string]string{
		"/interfaces/loop7000/l2/bridgeDomain":      "interfaces.bridge-l2-domain-exists",
		"/interfaces/loop7001/l2/tagRewrite":        "interfaces.bridge-l2-tag-rewrite-l2-only",
		"/routing/l2/l3xc/loop7000/ipv4Paths/0/vrf": "routing.bridge-l2-l3xc",
	} {
		if got[p] != rule {
			t.Errorf("issue at %s = %q, want %q (all: %v)", p, got[p], rule, got)
		}
	}
	if len(v.BridgeDomainIDs()) != 0 {
		t.Fatal("a failed validation touched VPP")
	}
}
