package agent

import (
	"context"
	"strings"
	"testing"

	"google.golang.org/protobuf/proto"

	vrxv1 "ngfw/agent/gen/vrx/v1"
	"ngfw/agent/internal/descriptors/core/coretest"
	"ngfw/agent/internal/descriptors/lcp"
	"ngfw/agent/internal/desired"
	"ngfw/agent/internal/scheduler"
	"ngfw/agent/internal/subsystems"
)

// p12Doc: two paired interfaces (one with the default Linux name), BGP and a filter.
const p12Doc = `{
  "interfaces": {
    "loop821": {"enabled": true, "ipv4": ["10.8.21.1/24"], "lcp": {"hostIfName": "w8-l21", "hostIfType": "tap", "netns": "ns-w8-frr"}},
    "loop822": {"enabled": true, "ipv4": ["10.8.22.1/24"], "lcp": {"hostIfType": "tap"}}
  },
  "routing": {
    "policy": {"prefixLists": {"pl": {"family": "ipv4", "rules": [{"seq": 5, "action": "permit", "prefix": "10.8.0.0/16", "le": 32}]}}},
    "bgp": {"asn": 65080, "neighbors": {"10.8.21.2": {"remoteAs": 65081, "updateSource": "loop821"}}}
  }
}`

func TestP12ProjectionWithoutFRR(t *testing.T) {
	fixedIdentity(t)
	s := newSvc(t, coretest.New(), t.TempDir())
	ds := doc(t, p12Doc)
	pj := project(ds, []string{"interfaces", "routing"}, nil, nil)
	keys := map[scheduler.Key]bool{}
	for _, kv := range pj.kvs {
		keys[kv.Key] = true
	}
	for _, k := range []scheduler.Key{"lcp.itf-pair/loop821", "lcp.itf-pair/loop822"} {
		if !keys[k] {
			t.Fatalf("%s not projected: %v", k, keys)
		}
	}
	if keys[desired.FRRConfigKey] {
		t.Fatal("an agent without FRR must not project frr.config")
	}
	warned := false
	for _, is := range pj.issues {
		if is.rule == "agent.unsupported-field" && strings.Contains(is.message, "drives no FRR") {
			warned = true
		}
		if is.severity == vrxv1.IssueSeverity_ISSUE_SEVERITY_ERROR {
			t.Fatalf("error issue %+v", is)
		}
	}
	if !warned {
		t.Fatalf("no warning about FRR content: %+v", pj.issues)
	}

	// the pairs reach VPP (the tap gate lets lcp_itf_pair_get through once a tap exists) and Retrieve reports them
	mustStatus(t, apply(t, s, &vrxv1.ApplyRequest{TxnId: "l1", DesiredState: ds, Subsystems: []string{"interfaces"}}), vrxv1.ApplyStatus_APPLY_STATUS_APPLIED)
	got, err := s.Retrieve(context.Background(), &vrxv1.RetrieveRequest{Subsystems: []string{"interfaces"}})
	if err != nil {
		t.Fatal(err)
	}
	for name, want := range map[string]*vrxv1.InterfaceLcp{
		"loop821": {HostIfName: proto.String("w8-l21"), HostIfType: proto.String("tap"), Netns: proto.String("ns-w8-frr")},
		"loop822": {HostIfType: proto.String("tap")}, // hostIfName = the VPP name: canonical form leaves it out
	} {
		if l := got.GetDesiredState().GetInterfaces()[name].GetLcp(); !proto.Equal(l, want) {
			t.Fatalf("Retrieve %s.lcp = %v, want %v", name, l, want)
		}
	}
	// idempotent: the second apply changes nothing
	resp := apply(t, s, &vrxv1.ApplyRequest{TxnId: "l2", DesiredState: ds, Subsystems: []string{"interfaces"}})
	mustStatus(t, resp, vrxv1.ApplyStatus_APPLY_STATUS_APPLIED)
	if len(resp.GetResults()) != 0 {
		t.Fatalf("second apply: %v", resp.GetResults())
	}
	// removing the leaf removes the pair (and VPP's end of the tap)
	ds.Interfaces["loop822"].Lcp = nil
	mustStatus(t, apply(t, s, &vrxv1.ApplyRequest{TxnId: "l3", DesiredState: ds, Subsystems: []string{"interfaces"}}), vrxv1.ApplyStatus_APPLY_STATUS_APPLIED)
	kvs, err := s.sched.Retrieve(context.Background(), scheduler.Only(lcp.NameItfPair))
	if err != nil || len(kvs) != 1 || kvs[0].Key != "lcp.itf-pair/loop821" {
		t.Fatalf("pairs after removal: %v %v", kvs, err)
	}
}

func TestP12ProjectionWithFRR(t *testing.T) {
	ds := doc(t, p12Doc)
	o := subsystems.FRRProjection()
	o.Disabled = false
	pj := &projected{pointers: map[scheduler.Key]string{}}
	desired.FRR(pj, ds, map[string]bool{"routing": true}, o)
	if len(pj.kvs) != 1 || pj.kvs[0].Key != desired.FRRConfigKey || pj.pointers[desired.FRRConfigKey] != "/routing" {
		t.Fatalf("kvs %v issues %+v", pj.kvs, pj.issues)
	}
	fdoc, status, err := desired.ParseFRRValue(pj.kvs[0].Value)
	if err != nil || status != desired.FRRApplied || fdoc.GetRouting().GetBgp().GetAsn() != 65080 || len(fdoc.GetInterfaces()) != 2 ||
		fdoc.GetInterfaces()["loop821"].GetIpv4()[0] != "10.8.21.1/24" {
		t.Fatalf("value %v %q %v", fdoc, status, err)
	}

	// passwordRef without the secret channel: a validation error at the reference
	ds.Routing.Bgp.Neighbors["10.8.21.2"].PasswordRef = proto.String("password/bgp-peer")
	pj = &projected{pointers: map[scheduler.Key]string{}}
	desired.FRR(pj, ds, map[string]bool{"routing": true}, o)
	if len(pj.kvs) != 0 || len(pj.issues) != 1 || pj.issues[0].pointer != "/routing/bgp/neighbors/10.8.21.2/passwordRef" ||
		pj.issues[0].rule != "routing.bgp-password-unavailable" || !pj.hasErrors() {
		t.Fatalf("password: kvs %v issues %+v", pj.kvs, pj.issues)
	}

	// what FRR cannot render is a validation error before anything is applied (an unpaired update-source)
	ds.Routing.Bgp.Neighbors["10.8.21.2"].PasswordRef = nil
	ds.Routing.Bgp.Neighbors["10.8.21.2"].UpdateSource = proto.String("loop9")
	pj = &projected{pointers: map[scheduler.Key]string{}}
	desired.FRR(pj, ds, map[string]bool{"routing": true}, o)
	if len(pj.kvs) != 0 || !pj.hasErrors() || !strings.Contains(pj.issues[0].message, "has no Linux interface") {
		t.Fatalf("render check: %+v", pj.issues)
	}

	// paired interfaces alone are FRR content: the tap addresses stay while the pair exists (linux-nl would otherwise
	// mirror their removal into VPP); an agent without FRR says nothing about them
	lcpOnly := doc(t, `{"interfaces":{"loop821":{"ipv4":["10.8.21.1/24"],"lcp":{}}},"routing":{}}`)
	pj = &projected{pointers: map[scheduler.Key]string{}}
	desired.FRR(pj, lcpOnly, map[string]bool{"routing": true}, o)
	if len(pj.kvs) != 1 || len(pj.issues) != 0 {
		t.Fatalf("lcp-only: %v %+v", pj.kvs, pj.issues)
	}
	off := o
	off.Disabled = true
	pj = &projected{pointers: map[scheduler.Key]string{}}
	desired.FRR(pj, lcpOnly, map[string]bool{"routing": true}, off)
	if len(pj.kvs) != 0 || len(pj.issues) != 0 {
		t.Fatalf("lcp-only without FRR: %v %+v", pj.kvs, pj.issues)
	}

	// no FRR content: no object
	pj = &projected{pointers: map[scheduler.Key]string{}}
	desired.FRR(pj, doc(t, `{"routing":{"static":[{"prefix":"10.0.0.0/8","blackhole":true}]}}`), map[string]bool{"routing": true}, o)
	if len(pj.kvs) != 0 || len(pj.issues) != 0 {
		t.Fatalf("no FRR content: %v %+v", pj.kvs, pj.issues)
	}
}

func TestP12AssembleFRR(t *testing.T) {
	ds := doc(t, `{"routing":{"bgp":{"asn":65080},"policy":{"routeMaps":{"rm":{"entries":[{"seq":1,"action":"permit"}]}}},
	  "static":[{"prefix":"10.9.0.0/16","vrf":"default","blackhole":true,"viaFrr":true,"tag":7}]}}`)
	fdoc := desired.FRRDoc(ds, func(_ int, sr *vrxv1.StaticRoute) bool { return sr.GetViaFrr() })
	out := &vrxv1.DesiredState{Routing: &vrxv1.RoutingConfig{Static: []*vrxv1.StaticRoute{
		{Prefix: proto.String("10.10.0.0/16"), Vrf: proto.String("default"), Blackhole: proto.Bool(true)},
		{Prefix: proto.String("10.8.0.0/16"), Vrf: proto.String("default"), Blackhole: proto.Bool(true)},
	}}}
	desired.AssembleFRR(out, []scheduler.KV{{Key: desired.FRRConfigKey, Value: desired.FRRValue(fdoc, desired.FRRApplied)}})
	if out.GetRouting().GetBgp().GetAsn() != 65080 || len(out.GetRouting().GetPolicy().GetRouteMaps()) != 1 {
		t.Fatalf("assembled %v", out)
	}
	var order []string
	for _, r := range out.GetRouting().GetStatic() {
		order = append(order, r.GetPrefix())
	}
	if strings.Join(order, " ") != "10.10.0.0/16 10.8.0.0/16 10.9.0.0/16" || out.GetRouting().GetStatic()[2].GetTag() != 7 {
		t.Fatalf("static order %v", order)
	}
	// drift: nothing is reported for the FRR leaves
	out = &vrxv1.DesiredState{}
	desired.AssembleFRR(out, []scheduler.KV{{Key: desired.FRRConfigKey, Value: desired.FRRValue(fdoc, desired.FRRDrift)}})
	if out.GetRouting().GetBgp() != nil {
		t.Fatal("a drifted FRR config is not reported as the desired one")
	}
}

func TestP12RoutingWarningTable(t *testing.T) {
	ds := doc(t, `{"routing":{"bgp":{"asn":1},"ospf":{},"bfd":{}}}`)
	pj := project(ds, []string{"routing"}, nil, nil)
	var ptrs []string
	for _, is := range pj.issues {
		if is.rule == "agent.unsupported-field" && strings.Contains(is.message, "sections this agent build does not have") {
			ptrs = append(ptrs, is.pointer)
		}
	}
	if strings.Join(ptrs, " ") != "/routing/ospf /routing/bfd" {
		t.Fatalf("warnings %v (bgp is handled by P12)", ptrs)
	}
}
