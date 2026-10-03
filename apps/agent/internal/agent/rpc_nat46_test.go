package agent

import (
	"context"
	"strings"
	"testing"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	ngfwv1 "ngfw/agent/gen/ngfw/v1"
	"ngfw/agent/internal/descriptors/core/coretest"
)

// F-nat46: nat.nat46 through the whole service on the fake VPP (slot owner w7), sharing host-w7w0's map-t binding
// with nat.map: commit → Retrieve == canonical (the shared interface under both, no false drift) → re-apply empty →
// loss of a domain + agent restart + resync recreates it → rollback leaves no MAP object.

const nat46Part = `,
  "nat": {
    "map": {"interfaces": [{"interface": "host-w7w0", "mode": "map-t"}],
      "domains": [{"name": "mt", "mode": "map-t", "ipv4Prefix": "10.7.64.0/24", "ipv6Prefix": "fd00:7:64::/48", "ipv6Source": "fd00:7:ff::/96", "eaBitsLength": 8}]},
    "nat46": {"interfaces": ["host-w7l0", "host-w7w0"],
      "mappings": [{"name": "web", "ipv4": "10.7.2.80", "ipv6": "fd00:7:46::80"}]}
  }`

const canonicalNat46 = `{
  "map": {"interfaces": [{"interface": "host-w7w0", "mode": "map-t"}],
    "domains": [{"name": "mt", "mode": "map-t", "ipv4Prefix": "10.7.64.0/24", "ipv6Prefix": "fd00:7:64::/48", "ipv6Source": "fd00:7:ff::/96", "eaBitsLength": 8, "psidOffset": 0, "psidLength": 0}]},
  "nat46": {"clientPrefix": "64:ff9b::/96", "interfaces": ["host-w7l0", "host-w7w0"],
    "mappings": [{"name": "web", "ipv4": "10.7.2.80", "ipv6": "fd00:7:46::80"}]}
}`

func mapObjects(v *coretest.VPP) (domains, trans, encap int) {
	m := v.Map()
	m.Lock()
	defer m.Unlock()
	return len(m.Domains), len(m.Trans), len(m.Encap)
}

func TestNat46DomainOnFake(t *testing.T) {
	v := coretest.New()
	dir := t.TempDir()
	s := newSvc(t, v, dir)
	with := doc(t, strings.Replace(natIfDoc, "%s", nat46Part, 1))
	mustStatus(t, apply(t, s, &ngfwv1.ApplyRequest{TxnId: "n1", DesiredState: with}), ngfwv1.ApplyStatus_APPLY_STATUS_APPLIED)
	want := doc(t, `{"nat":`+canonicalNat46+`}`).GetNat()
	if got := natNat(t, s); !proto.Equal(got, want) {
		t.Fatalf("Retrieve nat != canonical:\n got %s\nwant %s", protojson.Format(got), protojson.Format(want))
	}
	if d, tr, _ := mapObjects(v); d != 2 || tr != 2 {
		t.Fatalf("after commit: %d domains, %d map-t interfaces (want 2, 2)", d, tr)
	}
	if resp := apply(t, s, &ngfwv1.ApplyRequest{TxnId: "n2", DesiredState: with}); len(resp.GetResults()) != 0 {
		t.Fatalf("re-apply changed %v", resp.GetResults())
	}

	// loss of every MAP domain behind the agent's back + agent restart → resync recreates them
	m := v.Map()
	m.Lock()
	for k := range m.Domains {
		delete(m.Domains, k)
	}
	m.Unlock()
	s.Close()
	s2 := newSvc(t, v, dir)
	s2.Resync(context.Background())
	if got := natNat(t, s2); !proto.Equal(got, want) {
		t.Fatalf("after restart + resync:\n got %s\nwant %s", protojson.Format(got), protojson.Format(want))
	}

	mustStatus(t, apply(t, s2, &ngfwv1.ApplyRequest{TxnId: "n3", DesiredState: doc(t, `{"vrfs": {"cust": {"id": 7001}}, "interfaces": {}, "nat": {}}`)}),
		ngfwv1.ApplyStatus_APPLY_STATUS_APPLIED)
	if got := natNat(t, s2); proto.Size(got) != 0 {
		t.Fatalf("Retrieve after rollback: %s", protojson.Format(got))
	}
	if d, tr, e := mapObjects(v); d+tr+e != 0 {
		t.Fatalf("after rollback: %d domains, %d map-t, %d map-e", d, tr, e)
	}
}
