package agent

import (
	"context"
	"strings"
	"testing"
	"time"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	"ngfw/agent/binapi/flowprobe"
	"ngfw/agent/binapi/ip_types"
	"ngfw/agent/binapi/ipfix_export"
	sflowapi "ngfw/agent/binapi/sflow"
	vrxv1 "ngfw/agent/gen/vrx/v1"
	"ngfw/agent/internal/descriptors/core/coretest"
	"ngfw/agent/internal/desired"
	"ngfw/agent/internal/ownertable"
	"ngfw/agent/internal/scheduler"
	"ngfw/agent/internal/subsystems"
)

// newIpfixSvc is newSvc with an explicit D-071 role.
func newIpfixSvc(t *testing.T, v *coretest.VPP, dir string, globalsOwner bool) *Service {
	t.Helper()
	owned, err := ownertable.Open(dir, testOwner)
	if err != nil {
		t.Fatal(err)
	}
	reg := scheduler.NewRegistry()
	w, err := subsystems.Register(reg, subsystems.Env{Client: v, Owner: testOwner, StateDir: dir, Owned: owned, NetdevKind: fakeNetdevs, GlobalsOwner: globalsOwner})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { desired.SetIpfixGlobalsOwner(false) })
	w.Connected(context.Background())
	sched := scheduler.New(reg, nil)
	sched.VerifyRetries = 0
	svc, err := NewService(ServiceConfig{Owner: testOwner, Version: "test", VPP: v, Scheduler: sched, StateDir: dir, BeforeTxn: w.BeforeTxn, NetdevKind: w.NetdevKind()})
	if err != nil {
		t.Fatal(err)
	}
	svc.retryMin, svc.retryMax = time.Hour, time.Hour
	t.Cleanup(svc.Close)
	return svc
}

// ipfixDoc: exporter 0 (lan), an additional IPv6 exporter, flowprobe ip4 rx on loop711, sFlow 1:1000
// on loop712 (the user guide's example, docs/user/services/ipfix-sflow.md).
const ipfixDoc = `{
  "vrfs": {"default": {"id": 0}},
  "interfaces": {
    "loop711": {"ipv4": ["10.7.11.1/24"]},
    "loop712": {"ipv4": ["10.7.12.1/24"], "ipv6": ["2001:db8:712::1/64"]}
  },
  "services": {"ipfix": {
    "exporters": {
      "lan": {"enabled": true, "description": "LAN flows", "collector": {"address": "10.7.11.9", "port": 3771},
              "sourceAddress": "10.7.11.1", "vrf": "default", "pathMtu": 1400, "templateIntervalSec": 10, "udpChecksum": false},
      "v6": {"enabled": true, "collector": {"address": "2001:db8:712::9", "port": 4739},
             "sourceAddress": "2001:db8:712::1", "vrf": "default", "pathMtu": 512, "templateIntervalSec": 20, "udpChecksum": false},
      "off": {"enabled": false, "collector": {"address": "10.7.11.8", "port": 4739}, "sourceAddress": "10.7.11.1"}
    },
    "flowprobe": {"activeTimerSec": 5, "passiveTimerSec": 10, "recordL2": false, "recordL3": true, "recordL4": true,
      "interfaces": [{"interface": "loop711", "direction": "rx", "l2": false, "ip4": true, "ip6": false}]},
    "sflow": {"enabled": true, "samplingN": 1000, "pollingIntervalSec": 20, "headerBytes": 128,
      "collectors": [{"address": "10.7.11.9", "port": 3772}], "agentAddress": "10.7.11.1", "vrf": "default",
      "interfaces": ["loop712"]}
  }}
}`

// canonical services.ipfix Retrieve returns for ipfixDoc as the globals owner.
const ipfixCanonical = `{"ipfix": {
  "exporters": {
    "lan": {"enabled": true, "collector": {"address": "10.7.11.9", "port": 3771}, "sourceAddress": "10.7.11.1",
            "vrf": "default", "pathMtu": 1400, "templateIntervalSec": 10, "udpChecksum": false},
    "v6": {"enabled": true, "collector": {"address": "2001:db8:712::9", "port": 4739}, "sourceAddress": "2001:db8:712::1",
           "vrf": "default", "pathMtu": 512, "templateIntervalSec": 20, "udpChecksum": false}
  },
  "flowprobe": {"activeTimerSec": 5, "passiveTimerSec": 10, "recordL2": false, "recordL3": true, "recordL4": true,
    "interfaces": [{"interface": "loop711", "direction": "rx", "l2": false, "ip4": true, "ip6": false}]},
  "sflow": {"enabled": true, "samplingN": 1000, "pollingIntervalSec": 20, "headerBytes": 128, "interfaces": ["loop712"]}
}}`

func services(t *testing.T, js string) *vrxv1.ServicesConfig {
	t.Helper()
	s := &vrxv1.ServicesConfig{}
	if err := protojson.Unmarshal([]byte(js), s); err != nil {
		t.Fatal(err)
	}
	return s
}

// warnings are DryRun's warning pointers for ds (Apply reports only errors).
func warnings(t *testing.T, s *Service, ds *vrxv1.DesiredState) map[string]bool {
	t.Helper()
	rep, err := s.DryRun(context.Background(), &vrxv1.DryRunRequest{TxnId: "w", DesiredState: ds, Subsystems: []string{"services"}})
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]bool{}
	for _, i := range rep.GetErrors() {
		if i.GetSeverity() == vrxv1.IssueSeverity_ISSUE_SEVERITY_WARNING {
			out[i.GetPointer()] = true
		}
	}
	return out
}

func TestIpfixSflowGlobalsOwnerLifecycle(t *testing.T) {
	v := coretest.New()
	dir := t.TempDir()
	s := newIpfixSvc(t, v, dir, true)
	all := []string{"interfaces", "vrfs", "routing", "services"}
	resp := apply(t, s, &vrxv1.ApplyRequest{TxnId: "i1", Subsystems: all, DesiredState: doc(t, ipfixDoc)})
	mustStatus(t, resp, vrxv1.ApplyStatus_APPLY_STATUS_APPLIED)
	w := warnings(t, s, doc(t, ipfixDoc))
	for _, p := range []string{"/services/ipfix/exporters/off", "/services/ipfix/exporters/lan/description",
		"/services/ipfix/sflow/collectors", "/services/ipfix/sflow/agentAddress", "/services/ipfix/sflow/vrf"} {
		if !w[p] {
			t.Errorf("no warning at %s: %v", p, resp.GetValidation())
		}
	}
	f := v.Flow()
	if a := f.Exporters[0].CollectorAddress.String(); a != "10.7.11.9" || f.Exporters[0].CollectorPort != 3771 || len(f.Exporters) != 2 {
		t.Fatalf("exporters %+v", f.Exporters)
	}
	loop711, _ := v.InterfaceByName("loop711")
	loop712, _ := v.InterfaceByName("loop712")
	if d, ok := f.Flowprobe[loop711.Index]; !ok || d.Which != flowprobe.FLOWPROBE_WHICH_IP4 || d.Direction != flowprobe.FLOWPROBE_DIRECTION_RX {
		t.Fatalf("flowprobe %+v", f.Flowprobe)
	}
	if f.Params.RecordFlags != flowprobe.FLOWPROBE_RECORD_FLAG_L3|flowprobe.FLOWPROBE_RECORD_FLAG_L4 || f.Params.ActiveTimer != 5 {
		t.Fatalf("params %+v", f.Params)
	}
	if !f.Sflow[loop712.Index] || f.SflowG.Rate != 1000 || len(f.Sflow) != 1 {
		t.Fatalf("sflow %+v", f)
	}

	got, err := s.Retrieve(context.Background(), &vrxv1.RetrieveRequest{Subsystems: []string{"services"}})
	if err != nil {
		t.Fatal(err)
	}
	want := services(t, ipfixCanonical)
	want.Qos = &vrxv1.QosService{} // F-qos-flat: the services Retrieve always carries the (here empty) qos family
	if !proto.Equal(got.GetDesiredState().GetServices(), want) {
		t.Fatalf("retrieve:\n got %s\nwant %s", protojson.Format(got.GetDesiredState().GetServices()), protojson.Format(want))
	}

	st, err := s.IpfixState(context.Background(), &vrxv1.IpfixStateRequest{}, func(string) ([]*vrxv1.IpfixCounter, error) {
		return []*vrxv1.IpfixCounter{{Name: "/err/sflow/sflow packets processed", Value: 42}}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(st.GetExporters()) != 2 || !st.GetExporters()[0].GetDefaultExporter() || st.GetExporters()[0].GetName() != "lan" ||
		st.GetExporters()[1].GetName() != "v6" || st.GetExporters()[1].StatIndex == nil || st.GetExporters()[0].StatIndex != nil {
		t.Fatalf("exporters %v", st.GetExporters())
	}
	if len(st.GetFlowprobeInterfaces()) != 1 || st.GetFlowprobeInterfaces()[0].GetWhich() != "ip4" || st.GetFlowprobeParams().GetActiveTimerSec() != 5 {
		t.Fatalf("flowprobe %v %v", st.GetFlowprobeInterfaces(), st.GetFlowprobeParams())
	}
	if len(st.GetSflowInterfaces()) != 1 || st.GetSflowInterfaces()[0].GetHwIfIndex() != loop712.Index+coretest.HwOffset || st.GetSflowGlobal().GetSamplingN() != 1000 {
		t.Fatalf("sflow %v %v", st.GetSflowInterfaces(), st.GetSflowGlobal())
	}
	if len(st.GetSflowCounters()) != 1 || !st.GetGlobalsOwner() || !strings.Contains(strings.Join(st.GetNotes(), "|"), "hsflowd") {
		t.Fatalf("state %v", st)
	}

	// agent restart: fresh descriptors (no learned sflow mapping, V17) → one sflow re-create, then converged
	s.Close()
	s2 := newIpfixSvc(t, v, dir, true)
	r1 := s2.Resync(context.Background())
	mustStatus(t, r1, vrxv1.ApplyStatus_APPLY_STATUS_APPLIED)
	var recreated []string
	for _, r := range r1.GetResults() {
		recreated = append(recreated, r.GetKey()+":"+r.GetOp().String())
	}
	if len(recreated) != 1 || recreated[0] != "sflow.interface/loop712:APPLY_OPERATION_CREATE" {
		t.Fatalf("restart resync %v", recreated)
	}
	if r2 := s2.Resync(context.Background()); len(r2.GetResults()) != 0 {
		t.Fatalf("second resync %v", r2.GetResults())
	}

	// rollback to a document without services: interfaces gone, globals reset (globals owner)
	resp = apply(t, s2, &vrxv1.ApplyRequest{TxnId: "i2", Subsystems: []string{"services"}, DesiredState: &vrxv1.DesiredState{}})
	mustStatus(t, resp, vrxv1.ApplyStatus_APPLY_STATUS_APPLIED)
	f = v.Flow()
	if len(f.Flowprobe) != 0 || len(f.Sflow) != 0 || len(f.Exporters) != 1 || f.Params.RecordFlags != 0 ||
		!f.Exporters[0].CollectorAddress.ToIP().IsUnspecified() || f.SflowG.Rate != 10000 {
		t.Fatalf("after removal %+v", f)
	}
}

func TestIpfixSflowNonOwnerOnlyRequires(t *testing.T) {
	v := coretest.New()
	s := newIpfixSvc(t, v, t.TempDir(), false)
	all := []string{"interfaces", "vrfs", "routing", "services"}
	// VPP's exporter 0 differs from the required one: the non-owner refuses, never sets it
	resp := apply(t, s, &vrxv1.ApplyRequest{TxnId: "n1", Subsystems: all, DesiredState: doc(t, ipfixDoc)})
	if resp.GetStatus() == vrxv1.ApplyStatus_APPLY_STATUS_APPLIED {
		t.Fatalf("applied without the required globals: %v", resp)
	}
	if !strings.Contains(resp.GetMessage()+protojson.Format(resp), "globals owner") {
		t.Fatalf("no D-071 reason: %s", protojson.Format(resp))
	}
	if f := v.Flow(); !f.Exporters[0].CollectorAddress.ToIP().IsUnspecified() || f.Params.RecordFlags != 0 || f.SflowG.Rate != 10000 {
		t.Fatalf("a non-owner changed a VPP global: %+v", f)
	}
	// the globals owner (another agent) has set exactly what this document requires
	ctx := context.Background()
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	c := ipfix_export.NewServiceClient(v)
	_, err := c.SetIpfixExporter(ctx, &ipfix_export.SetIpfixExporter{CollectorAddress: addr(t, "10.7.11.9"), CollectorPort: 3771,
		SrcAddress: addr(t, "10.7.11.1"), VrfID: 0, PathMtu: 1400, TemplateInterval: 10})
	must(err)
	_, err = flowprobe.NewServiceClient(v).FlowprobeSetParams(ctx, &flowprobe.FlowprobeSetParams{
		RecordFlags: flowprobe.FLOWPROBE_RECORD_FLAG_L3 | flowprobe.FLOWPROBE_RECORD_FLAG_L4, ActiveTimer: 5, PassiveTimer: 10})
	must(err)
	sflowRate(t, v, 1000)
	resp = apply(t, s, &vrxv1.ApplyRequest{TxnId: "n2", Subsystems: all, DesiredState: doc(t, ipfixDoc)})
	mustStatus(t, resp, vrxv1.ApplyStatus_APPLY_STATUS_APPLIED)
	w := warnings(t, s, doc(t, ipfixDoc))
	for _, p := range []string{"/services/ipfix/exporters/lan", "/services/ipfix/flowprobe/activeTimerSec", "/services/ipfix/sflow/samplingN"} {
		if !w[p] {
			t.Errorf("no D-071 warning at %s", p)
		}
	}
	// rollback: the owner's interfaces go, the globals stay (Delete of a required global is a no-op)
	resp = apply(t, s, &vrxv1.ApplyRequest{TxnId: "n3", Subsystems: []string{"services"}, DesiredState: &vrxv1.DesiredState{}})
	mustStatus(t, resp, vrxv1.ApplyStatus_APPLY_STATUS_APPLIED)
	f := v.Flow()
	if len(f.Flowprobe) != 0 || len(f.Sflow) != 0 || len(f.Exporters) != 1 {
		t.Fatalf("owner objects left %+v", f)
	}
	if f.Exporters[0].CollectorPort != 3771 || f.Params.RecordFlags == 0 || f.SflowG.Rate != 1000 {
		t.Fatalf("a non-owner reset a VPP global: %+v", f)
	}
}

func TestIpfixSflowProjection(t *testing.T) {
	desired.SetIpfixGlobalsOwner(true)
	t.Cleanup(func() { desired.SetIpfixGlobalsOwner(false) })
	vrf := func(n string) (uint32, bool) { return 0, n == "default" }
	run := func(js string) *projected {
		p := &projected{pointers: map[scheduler.Key]string{}}
		desired.IpfixSflow(p, services(t, js), vrf)
		return p
	}
	find := func(p *projected, ptr string, sev vrxv1.IssueSeverity) bool {
		for _, i := range p.issues {
			if i.pointer == ptr && i.severity == sev {
				return true
			}
		}
		return false
	}
	// flowprobe without an IPv4 exporter: error at the interface list (the API's semantic rule too)
	p := run(`{"ipfix": {"flowprobe": {"interfaces": [{"interface": "loop1", "ip4": true, "ip6": false}]}}}`)
	if !find(p, "/services/ipfix/flowprobe/interfaces", vrxv1.IssueSeverity_ISSUE_SEVERITY_ERROR) {
		t.Fatalf("issues %v", p.issues)
	}
	// schema default ip4+ip6: realised as ip4, ip6 reported
	p = run(`{"ipfix": {"exporters": {"a": {"collector": {"address": "10.1.1.9"}, "sourceAddress": "10.1.1.1"}},
	  "flowprobe": {"interfaces": [{"interface": "loop1"}]}}}`)
	if p.hasErrors() || !find(p, "/services/ipfix/flowprobe/interfaces/0/ip6", vrxv1.IssueSeverity_ISSUE_SEVERITY_WARNING) {
		t.Fatalf("issues %v", p.issues)
	}
	if len(p.kvs) != 3 || p.kvs[2].Key != "flowprobe.interface/loop1" {
		t.Fatalf("kvs %v", p.kvs)
	}
	// sFlow header not a multiple of 32: error (VPP would round it)
	p = run(`{"ipfix": {"sflow": {"enabled": true, "headerBytes": 100, "collectors": [{"address": "10.1.1.9"}]}}}`)
	if !find(p, "/services/ipfix/sflow/headerBytes", vrxv1.IssueSeverity_ISSUE_SEVERITY_ERROR) {
		t.Fatalf("issues %v", p.issues)
	}
	// other services sub-trees are reported, not applied
	p = run(`{"ntp": {"enabled": true}}`)
	if !find(p, "/services/ntp", vrxv1.IssueSeverity_ISSUE_SEVERITY_WARNING) || len(p.kvs) != 0 {
		t.Fatalf("issues %v", p.issues)
	}
}

func addr(t *testing.T, s string) ip_types.Address {
	t.Helper()
	a, err := ip_types.ParseAddress(s)
	if err != nil {
		t.Fatal(err)
	}
	return a
}

func sflowRate(t *testing.T, v *coretest.VPP, n uint32) {
	t.Helper()
	if _, err := sflowapi.NewServiceClient(v).SflowSamplingRateSet(context.Background(), &sflowapi.SflowSamplingRateSet{SamplingN: n}); err != nil {
		t.Fatal(err)
	}
}

// TestIpfixExporterNamesOnlyFromAppliedState (review item 1): DryRun and a rolled-back apply never
// rename exporters; a successful apply does.
func TestIpfixExporterNamesOnlyFromAppliedState(t *testing.T) {
	v := coretest.New()
	s := newIpfixSvc(t, v, t.TempDir(), true)
	all := []string{"interfaces", "vrfs", "routing", "services"}
	mustStatus(t, apply(t, s, &vrxv1.ApplyRequest{TxnId: "a1", Subsystems: all, DesiredState: doc(t, ipfixDoc)}), vrxv1.ApplyStatus_APPLY_STATUS_APPLIED)
	renamed := doc(t, strings.Replace(ipfixDoc, `"lan":`, `"renamed":`, 1))
	if _, err := s.DryRun(context.Background(), &vrxv1.DryRunRequest{TxnId: "d1", DesiredState: renamed, Subsystems: all}); err != nil {
		t.Fatal(err)
	}
	if n := desired.IpfixExporterName("10.7.11.9"); n != "lan" {
		t.Fatalf("DryRun renamed the exporter: %q", n)
	}
	// a transaction that fails (unknown interface for flowprobe) and is rolled back
	bad := doc(t, strings.Replace(strings.Replace(ipfixDoc, `"lan":`, `"renamed":`, 1), `{"interface": "loop711"`, `{"interface": "loop799"`, 1))
	if r := apply(t, s, &vrxv1.ApplyRequest{TxnId: "a2", Subsystems: all, DesiredState: bad}); r.GetStatus() == vrxv1.ApplyStatus_APPLY_STATUS_APPLIED {
		t.Fatalf("applied %v", r)
	}
	if n := desired.IpfixExporterName("10.7.11.9"); n != "lan" {
		t.Fatalf("failed apply renamed the exporter: %q", n)
	}
	mustStatus(t, apply(t, s, &vrxv1.ApplyRequest{TxnId: "a3", Subsystems: all, DesiredState: renamed}), vrxv1.ApplyStatus_APPLY_STATUS_APPLIED)
	if n := desired.IpfixExporterName("10.7.11.9"); n != "renamed" {
		t.Fatalf("applied rename not seen: %q", n)
	}
}
