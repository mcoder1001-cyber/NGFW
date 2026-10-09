package agent

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	ngfwv1 "ngfw/agent/gen/ngfw/v1"
	"ngfw/agent/internal/descriptors/core"
	"ngfw/agent/internal/desired"
	"ngfw/agent/internal/scheduler"
	"ngfw/agent/internal/secretchannel"
	"ngfw/agent/internal/subsystems"
)

// TestProjectSchemaExamples projects every valid example document of packages/schema: no panic,
// no ERROR issue, and assemble(project(doc)) round-trips the implemented domains of the document
// modulo the leaves the core descriptors cannot represent.
func TestProjectSchemaExamples(t *testing.T) {
	t.Setenv(subsystems.EnvIDRange, subsystems.IDRangeAll) // F-tunnels: tunnel instances must lie in the agent's id range (TD-8b)
	// This schema corpus has references without secret material; PKI behavior is covered with sealed-cache fixtures separately.
	restorePKI := subsystems.SetPKIRuntimeForTest("schema-corpus", nil, "schema corpus without secrets")
	t.Cleanup(restorePKI)
	// F-snmp: no daemon parse run / secret resolution here (the examples hold refs only); restore after.
	saved := desired.SnapshotSnmpChecks()
	desired.RestoreSnmpChecks(nil)
	t.Cleanup(func() { desired.RestoreSnmpChecks(saved) })
	// Bind reference-only SNMP examples to one sealed fixture owner, just like
	// production projection; unrelated registered agents must not select its keys.
	const corpusOwner = "schema-corpus"
	cache, err := secretchannel.Open(t.TempDir(), corpusOwner)
	if err != nil {
		t.Fatal(err)
	}
	desired.SetSnmpSecretGenerations(corpusOwner, cache.Ref)
	t.Cleanup(func() { desired.SetSnmpSecretGenerations(corpusOwner, nil) })
	projectCorpus := func(ds *ngfwv1.DesiredState, native desired.IKEv2Env) *projected {
		return projectOwned(ds, implementedDomains(), nil, nil, corpusOwner, native)
	}
	files, err := filepath.Glob("../../../../packages/schema/examples/*.json")
	if err != nil || len(files) == 0 {
		t.Skipf("no schema examples: %v", err)
	}
	lenient := protojson.UnmarshalOptions{DiscardUnknown: true} // secret leaves are stripped by the API
	native := desired.IKEv2Env{SecretRef: func(context.Context, string) (string, error) { return "hmac:" + strings.Repeat("a", 64), nil }, CheckReady: func(context.Context) error { return nil }}
	n := 0
	for _, f := range files {
		base := filepath.Base(f)
		if strings.HasPrefix(base, "invalid-") || strings.Contains(base, "-invalid-") || strings.Contains(base, "semantic") {
			continue
		}
		b, err := os.ReadFile(f) //nolint:gosec // test corpus
		if err != nil {
			t.Fatal(err)
		}
		ds := &ngfwv1.DesiredState{}
		if err := lenient.Unmarshal(b, ds); err != nil {
			t.Logf("%s: not a DesiredState (%v) — skipped", base, err)
			continue
		}
		n++
		material := map[string][]byte{}
		for _, ref := range desired.SnmpSecretRefs(ds.GetServices().GetSnmp()) {
			material[ref] = []byte("NGFW_TEST_PSK_SCHEMA_SNMP")
		}
		generation, err := cache.Stage(material)
		if err != nil {
			t.Fatal(err)
		}
		if err = cache.Activate(generation); err != nil {
			t.Fatal(err)
		}
		pj := projectCorpus(ds, native)
		if base == "vpn-remote-access.json" {
			refused := false
			for _, issue := range pj.issues {
				if issue.severity == ngfwv1.IssueSeverity_ISSUE_SEVERITY_ERROR && issue.rule == "vpn.remote-access-native-capability" {
					refused = true
				}
			}
			if !refused {
				t.Fatal("unprovisioned corpus owner accepted an enabled independent engine")
			}
			for _, kv := range pj.kvs {
				if strings.HasPrefix(string(kv.Key), "remote-access.") {
					t.Fatal("refused example emitted private transport or engine")
				}
			}
			ds = proto.Clone(ds).(*ngfwv1.DesiredState)
			disabled := false
			for _, profile := range ds.GetVpn().GetRemoteAccess() {
				profile.Enabled = &disabled
			}
			pj = projectCorpus(ds, native)
		}
		for _, is := range pj.issues {
			// Host-service references deliberately have no selected material in this
			// corpus; owner-bound sealed host credentials have separate coverage.
			if is.severity == ngfwv1.IssueSeverity_ISSUE_SEVERITY_ERROR && is.rule != "agent.secret-channel-pending" {
				t.Errorf("%s: %s %s: %s", base, is.pointer, is.rule, is.message)
			}
		}
		// Round trip of what the core descriptors represent.
		out := assemble(pj.kvs, implementedDomains(), nil, ds.GetInterfaces(), nil)
		for _, kv := range pj.kvs {
			if kv.Key == "" || kv.Value == nil {
				t.Fatalf("%s: empty kv", base)
			}
		}
		again := projectCorpus(out, native)
		if !sameKVs(withoutWriteOnly(pj.kvs), again.kvs) { // write-only objects cannot round-trip (F-loopback-bvi-gso-lldp-span)
			t.Errorf("%s: project(assemble(project(doc))) != project(doc)", base)
			wantByKey := map[scheduler.Key]proto.Message{}
			for _, kv := range withoutWriteOnly(pj.kvs) {
				wantByKey[kv.Key] = kv.Value
			}
			for _, kv := range again.kvs {
				if !proto.Equal(wantByKey[kv.Key], kv.Value) {
					t.Logf("%s %s: want %v; got %v", base, kv.Key, wantByKey[kv.Key], kv.Value)
				}
				delete(wantByKey, kv.Key)
			}
			for key, value := range wantByKey {
				t.Logf("%s missing %s: %v", base, key, value)
			}
		}
	}
	t.Logf("projected %d example documents", n)
}

func sameKVs(a, b []scheduler.KV) bool {
	if len(a) != len(b) {
		return false
	}
	m := map[scheduler.Key]proto.Message{}
	for _, kv := range a {
		m[kv.Key] = kv.Value
	}
	for _, kv := range b {
		if !proto.Equal(m[kv.Key], kv.Value) {
			return false
		}
	}
	return true
}

func TestSixrdSourceAndUnderlayRoundTrip(t *testing.T) {
	ds := doc(t, `{"vrfs":{"outer":{"id":10},"inner":{"id":11}},"tunnels":{"ipip":{"br":{"src":"198.51.100.2","underlayVrf":"outer","vrf":"inner","sixrd":{"ip6Prefix":"2001:db8:6::/48","ip4Prefix":"198.51.0.0/16","securityCheck":true}}}}}`)
	domains := []string{"vrfs", "interfaces", "tunnels"}
	first := project(ds, domains, nil, nil)
	out := assemble(first.kvs, domains, nil, nil, nil)
	recovered := out.GetTunnels().GetIpip()["br"]
	if recovered.GetSrc() != "198.51.100.2" || recovered.GetUnderlayVrf() != "outer" {
		t.Fatalf("6RD source/underlay lost: %v", recovered)
	}
	again := project(out, domains, nil, nil)
	if !sameKVs(first.kvs, again.kvs) {
		t.Fatalf("6RD roundtrip differs: first %v; again %v", first.kvs, again.kvs)
	}
}

func TestLogicalTunnelStaticRouteRoundTrip(t *testing.T) {
	ds := &ngfwv1.DesiredState{}
	if err := protojson.Unmarshal([]byte(`{"tunnels":{"ipip":{"site":{"instance":6001,"src":"198.51.100.2","dst":"203.0.113.2","ipv4":["10.255.0.1/30"]}}},"routing":{"static":[{"prefix":"10.20.0.0/16","nextHops":[{"interface":"site"}]}]}}`), ds); err != nil {
		t.Fatal(err)
	}
	t.Setenv(subsystems.EnvIDRange, subsystems.IDRangeAll)
	p := project(ds, []string{"tunnels", "routing"}, nil, nil)
	for _, issue := range p.issues {
		if issue.severity == ngfwv1.IssueSeverity_ISSUE_SEVERITY_ERROR {
			t.Fatal(issue)
		}
	}
	found := false
	for _, kv := range p.kvs {
		if route, ok := kv.Value.(*core.Route); ok {
			found = true
			if route.Paths[0].Interface != "ipip6001" {
				t.Fatalf("route uses unresolved logical name: %v", route)
			}
		}
	}
	if !found {
		t.Fatal("static route missing")
	}
	partial := project(ds, []string{"routing"}, nil, nil)
	for _, kv := range partial.kvs {
		if kv.Key.Descriptor() == desired.TunnelMetaName {
			t.Fatal("routing-only projection changes tunnel metadata")
		}
		if route, ok := kv.Value.(*core.Route); ok && route.Paths[0].Interface != "ipip6001" {
			t.Fatalf("routing-only alias unresolved: %v", route)
		}
	}
	got := assemble(p.kvs, []string{"tunnels", "routing"}, nil, nil, nil)
	if got.GetRouting().GetStatic()[0].GetNextHops()[0].GetInterface() != "site" {
		t.Fatalf("logical interface lost: %v", got.GetRouting())
	}
}
