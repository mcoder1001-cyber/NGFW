package agent

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	vrxv1 "ngfw/agent/gen/vrx/v1"
	"ngfw/agent/internal/descriptors/core/coretest"
	"ngfw/agent/internal/objects"
	"ngfw/agent/internal/subsystems"
)

// F-object-model: the objects domain end to end on the fake VPP — Apply puts every object into the
// agent's store (pointers /objects/<kind>/<name>), Retrieve returns the applied document, a second
// Apply is empty, FqdnObjectState reports the FQDN objects, a confirm-timeout revert restores the
// previous object set, a restarted agent retrieves the objects from its state dir, and DryRun
// reports the defence-in-depth findings.

const objectsJSON = `{
  "objects": {
    "tags": {"prod": {"color": "#1e88e5", "description": "production"}},
    "addresses": {
      "web1": {"type": "host", "address": "192.0.2.10", "tags": ["prod"]},
      "web2": {"type": "host", "address": "192.0.2.11"},
      "cdn":  {"type": "fqdn", "fqdn": "cdn.w7.test"},
      "lan":  {"type": "network", "prefix": "10.7.1.0/24"}
    },
    "addressGroups": {"web-servers": {"members": ["web1", "web2"], "tags": ["prod"]}},
    "services": {"https": {"protocol": "tcp", "destinationPorts": ["443"]}},
    "serviceGroups": {"web": {"members": ["https"]}},
    "schedules": {"office-hours": {"type": "recurring", "days": ["mon", "tue", "wed", "thu", "fri"], "start": "08:00", "end": "18:00"}},
    "zones": {"lan": {"interfaces": ["loop701"], "description": "LAN"}}
  }
}`

func newObjectsSvc(t *testing.T, v *coretest.VPP, dir string) *Service {
	t.Helper()
	t.Setenv(subsystems.EnvDNSServers, "127.0.0.1:9") // nothing answers: FQDN lookups fail at once, the test never reaches a real resolver
	s := newSvc(t, v, dir)
	t.Cleanup(func() {
		if rt := objects.RuntimeFor(dir, testOwner); rt != nil {
			rt.Close()
		}
	})
	return s
}

func retrieveObjects(t *testing.T, s *Service) *vrxv1.ObjectsConfig {
	t.Helper()
	got, err := s.Retrieve(context.Background(), &vrxv1.RetrieveRequest{Subsystems: []string{"objects"}})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(got.GetSubsystems(), ",") != "objects" {
		t.Fatalf("subsystems %v", got.GetSubsystems())
	}
	return got.GetDesiredState().GetObjects()
}

func TestObjectsDomainOnFake(t *testing.T) {
	v := coretest.New()
	dir := t.TempDir()
	s := newObjectsSvc(t, v, dir)
	if !strings.Contains(strings.Join(s.Health().GetSubsystems(), ","), "objects") {
		t.Fatalf("objects not in Health.subsystems: %v", s.Health().GetSubsystems())
	}
	want := doc(t, objectsJSON)
	resp := apply(t, s, &vrxv1.ApplyRequest{TxnId: "o1", DesiredState: want})
	mustStatus(t, resp, vrxv1.ApplyStatus_APPLY_STATUS_APPLIED)
	byKey := map[string]*vrxv1.ObjectResult{}
	for _, r := range resp.GetResults() {
		byKey[r.GetKey()] = r
	}
	for k, ptr := range map[string]string{
		"objects.tag/prod":                  "/objects/tags/prod",
		"objects.address/cdn":               "/objects/addresses/cdn",
		"objects.address-group/web-servers": "/objects/addressGroups/web-servers",
		"objects.service/https":             "/objects/services/https",
		"objects.service-group/web":         "/objects/serviceGroups/web",
		"objects.schedule/office-hours":     "/objects/schedules/office-hours",
		"objects.zone/lan":                  "/objects/zones/lan",
	} {
		if r := byKey[k]; r == nil || r.GetPointer() != ptr || r.GetSubsystem() != "objects" || r.GetOp() != vrxv1.ApplyOperation_APPLY_OPERATION_CREATE {
			t.Fatalf("result %s = %v, want pointer %s", k, r, ptr)
		}
	}
	if resp.GetSummary().GetCreated() != 10 {
		t.Fatalf("summary %v", resp.GetSummary())
	}
	if got := retrieveObjects(t, s); !proto.Equal(got, want.GetObjects()) {
		t.Fatalf("Retrieve != desired:\n%s", protojson.Format(got))
	}
	again := apply(t, s, &vrxv1.ApplyRequest{TxnId: "o2", DesiredState: want})
	if sm := again.GetSummary(); sm.GetCreated()+sm.GetUpdated()+sm.GetDeleted() != 0 {
		t.Fatalf("second apply not empty: %v", sm)
	}

	// the state RPC: the FQDN object, never a Retrieve field
	st, err := s.FqdnObjectState(&vrxv1.FqdnObjectStateRequest{})
	if err != nil || len(st.GetObjects()) != 1 || st.GetObjects()[0].GetName() != "cdn" || st.GetObjects()[0].GetFqdn() != "cdn.w7.test" || st.GetOwner() != testOwner {
		t.Fatalf("FqdnObjectState: %v %v", st, err)
	}
	if st, _ := s.FqdnObjectState(&vrxv1.FqdnObjectStateRequest{Names: []string{"web1"}}); len(st.GetObjects()) != 0 {
		t.Fatalf("filter: %v", st)
	}
	if _, err := s.FqdnObjectState(&vrxv1.FqdnObjectStateRequest{Owner: "w9"}); grpcCode(err) != codes.InvalidArgument {
		t.Fatalf("foreign owner: %v", err)
	}

	// confirm timeout: the unconfirmed change (web2 changed, zone and schedule gone) is reverted
	changed := proto.Clone(want).(*vrxv1.DesiredState)
	changed.Objects.Addresses["web2"].Address = proto.String("192.0.2.99")
	delete(changed.Objects.Zones, "lan")
	delete(changed.Objects.Schedules, "office-hours")
	mustStatus(t, apply(t, s, &vrxv1.ApplyRequest{TxnId: "o3", DesiredState: changed, ConfirmTimeoutSec: 60}), vrxv1.ApplyStatus_APPLY_STATUS_APPLIED)
	if got := retrieveObjects(t, s); !proto.Equal(got, changed.GetObjects()) {
		t.Fatalf("pending change not applied:\n%s", protojson.Format(got))
	}
	s.revert("o3")
	if got := retrieveObjects(t, s); !proto.Equal(got, want.GetObjects()) {
		t.Fatalf("revert did not restore the object set:\n%s", protojson.Format(got))
	}

	// restart: a new agent on the same state dir retrieves the objects from its store before any
	// transaction (D-063: real agent state), and the resync is empty
	s.Close()
	s2 := newObjectsSvc(t, v, dir)
	if got := retrieveObjects(t, s2); !proto.Equal(got, want.GetObjects()) {
		t.Fatalf("Retrieve after restart:\n%s", protojson.Format(got))
	}
	if r := s2.Resync(context.Background()); r.GetStatus() != vrxv1.ApplyStatus_APPLY_STATUS_APPLIED || r.GetSummary().GetCreated()+r.GetSummary().GetDeleted()+r.GetSummary().GetUpdated() != 0 {
		t.Fatalf("resync after restart: %v", r)
	}

	// an explicitly empty objects domain deletes every object (D-041)
	mustStatus(t, apply(t, s2, &vrxv1.ApplyRequest{TxnId: "o4", DesiredState: doc(t, `{"objects": {}}`)}), vrxv1.ApplyStatus_APPLY_STATUS_APPLIED)
	if got := retrieveObjects(t, s2); got != nil {
		t.Fatalf("objects left: %s", protojson.Format(got))
	}
	if st, _ := s2.FqdnObjectState(&vrxv1.FqdnObjectStateRequest{}); len(st.GetObjects()) != 0 {
		t.Fatalf("fqdn state left: %v", st)
	}
}

// DryRun (defence in depth; the API's schema rules normally reject both first): a membership cycle
// is an error at the group, a group above the expansion cap a warning.
func TestObjectsDryRunFindings(t *testing.T) {
	s := newObjectsSvc(t, coretest.New(), t.TempDir())
	rep, err := s.DryRun(context.Background(), &vrxv1.DryRunRequest{DesiredState: doc(t, `{"objects": {
	  "addressGroups": {"a": {"members": ["b"]}, "b": {"members": ["a"]}}}}`)})
	if err != nil || rep.GetOk() {
		t.Fatalf("cycle accepted: %v %v", err, rep)
	}
	var got []string
	for _, e := range rep.GetErrors() {
		got = append(got, e.GetPointer()+" "+e.GetRule())
	}
	if strings.Join(got, ",") != "/objects/addressGroups/a objects.object-model-expansion,/objects/addressGroups/b objects.object-model-expansion" {
		t.Fatalf("cycle findings %v", got)
	}

	// 46 IPv6 ranges, each <x>::1 – <x>:ffff:…:fffe inside its own /16 (224 prefixes, none mergeable
	// across ranges): 10 304 entries from 47 objects
	big := &vrxv1.ObjectsConfig{Addresses: map[string]*vrxv1.AddressObject{}, AddressGroups: map[string]*vrxv1.AddressGroup{"big": {}}}
	for i := 0; i < 46; i++ {
		n := fmt.Sprintf("r%02d", i)
		x := fmt.Sprintf("%x", 0x3000+i)
		big.Addresses[n] = &vrxv1.AddressObject{Type: proto.String("range"), Start: proto.String(x + "::1"), End: proto.String(x + ":ffff:ffff:ffff:ffff:ffff:ffff:fffe")}
		big.AddressGroups["big"].Members = append(big.AddressGroups["big"].Members, n)
	}
	rep, err = s.DryRun(context.Background(), &vrxv1.DryRunRequest{DesiredState: &vrxv1.DesiredState{Objects: big}})
	if err != nil || !rep.GetOk() || len(rep.GetErrors()) != 1 {
		t.Fatalf("big group: %v %v", err, rep.GetErrors())
	}
	if w := rep.GetErrors()[0]; w.GetPointer() != "/objects/addressGroups/big" || w.GetRule() != "objects.object-model-expansion-limit" || w.GetSeverity() != vrxv1.IssueSeverity_ISSUE_SEVERITY_WARNING {
		t.Fatalf("big group finding %v", w)
	}
	t.Logf("DryRun warning: %s %s: %s", rep.GetErrors()[0].GetPointer(), rep.GetErrors()[0].GetRule(), rep.GetErrors()[0].GetMessage())
}

// Q4 close seam: Agent.Stop closes the objects runtime (resolver stopped, store written, unregistered), and the
// agent's /metrics carries the objects counters.
func TestAgentStopClosesObjectsRuntimeAndMetrics(t *testing.T) {
	cfg := testConfig(t)
	a, err := Start(context.Background(), cfg, "test", nil)
	if err != nil {
		t.Fatal(err)
	}
	if objects.RuntimeFor(cfg.StateDir, cfg.Owner) == nil {
		t.Fatal("objects runtime not opened by Start")
	}
	a.Stop()
	if objects.RuntimeFor(cfg.StateDir, cfg.Owner) != nil {
		t.Fatal("Stop did not close the objects runtime (Wiring.Close)")
	}
	var b strings.Builder
	newMetrics().write(&b)
	for _, m := range []string{"vrx_agent_objects_store_corrupt_total", "vrx_agent_objects_store_persist_errors_total", "vrx_agent_objects_fqdn_stale_expired_total"} {
		if !strings.Contains(b.String(), m+" ") {
			t.Fatalf("metric %s missing", m)
		}
	}
}
