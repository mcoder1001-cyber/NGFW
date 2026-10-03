package agent

import (
	"context"
	"net/netip"
	"strings"
	"testing"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	cnatapi "ngfw/agent/binapi/cnat"
	"ngfw/agent/binapi/det44"
	"ngfw/agent/binapi/ip_types"
	ngfwv1 "ngfw/agent/gen/ngfw/v1"
	"ngfw/agent/internal/descriptors/core/coretest"
)

// F-det44-map-dslite-cnat: DET44, DS-Lite, MAP and CNAT through the whole service on the fake VPP (slot owner w7,
// not the globals owner, D-071: det44 is enabled, the DS-Lite AFTR and the cnat default SNAT entry are set by the
// fixture; the agent only requires them): commit → Retrieve == canonical → re-apply empty → loss + resync (the
// restart simulation: a fresh Service on the same state dir) recreates everything → rollback removes every object,
// det44 stays enabled (V9: never disabled) and no call that would crash VPP 26.06 was made (V10).

const cgnatPart = `,
  "nat": {
    "det44": {"enabled": true, "inside": ["host-w7l0"], "outside": ["host-w7w0"],
      "mappings": [{"inside": "10.7.1.0/24", "outside": "10.7.2.200/30"}]},
    "dslite": {"enabled": true, "aftr": {"ipv6": "fd00:7::1"}, "pools": [{"range": "10.7.5.1-10.7.5.4"}]},
    "map": {"interfaces": [{"interface": "host-w7w0", "mode": "map-e"}],
      "domains": [{"name": "lw", "mode": "lw4o6", "ipv4Prefix": "10.7.65.0/24", "ipv6Prefix": "fd00:7:65::/64", "ipv6Source": "fd00:7::1/128", "psidLength": 4,
        "rules": [{"psid": 1, "ipv6Destination": "fd00:7:65::1"}]}]},
    "cnat": {"translations": [{"name": "web", "protocol": "tcp", "vip": {"ip": "10.7.2.100", "port": 80},
      "backends": [{"ip": "10.7.2.2", "port": 8080}, {"ip": "10.7.2.3", "port": 8080}]}],
      "snat": {"addresses": {"ipv4": "10.7.2.1"}}}
  }`

// canonicalCgnat is what a slot agent retrieves: no globals (timeouts, AFTR, SNAT entry), no labels.
const canonicalCgnat = `{
  "det44": {"enabled": true, "inside": ["host-w7l0"], "outside": ["host-w7w0"], "mappings": [{"inside": "10.7.1.0/24", "outside": "10.7.2.200/30"}]},
  "dslite": {"enabled": true, "pools": [{"range": "10.7.5.1-10.7.5.4"}]},
  "map": {"interfaces": [{"interface": "host-w7w0", "mode": "map-e"}],
    "domains": [{"name": "lw", "mode": "lw4o6", "ipv4Prefix": "10.7.65.0/24", "ipv6Prefix": "fd00:7:65::/64", "ipv6Source": "fd00:7::1/128", "eaBitsLength": 0, "psidOffset": 0, "psidLength": 4,
      "rules": [{"psid": 1, "ipv6Destination": "fd00:7:65::1"}]}]},
  "cnat": {"translations": [{"protocol": "tcp", "vip": {"ip": "10.7.2.100", "port": 80}, "backends": [{"ip": "10.7.2.2", "port": 8080}, {"ip": "10.7.2.3", "port": 8080}], "lbType": "default"}]}
}`

func cgnatFixtures(v *coretest.VPP) {
	d := v.Det44()
	d.Lock()
	d.Enabled = true
	d.Unlock()
	ds := v.Dslite()
	ds.Lock()
	ds.Aftr.IP6Addr = netip.MustParseAddr("fd00:7::1").As16()
	ds.Unlock()
	c := v.Cnat()
	c.Lock()
	c.Snat = &cnatapi.CnatGetSnatAddressesReply{SnatIP4: ip_types.IP4Address(netip.MustParseAddr("10.7.2.1").As4())}
	c.Unlock()
}

func cgnatEmpty(v *coretest.VPP) bool {
	d, ds, m, c := v.Det44(), v.Dslite(), v.Map(), v.Cnat()
	d.Lock()
	defer d.Unlock()
	ds.Lock()
	defer ds.Unlock()
	m.Lock()
	defer m.Unlock()
	c.Lock()
	defer c.Unlock()
	n := len(d.Ifaces) + len(d.Maps) + len(ds.Pool) + len(m.Domains) + len(m.Encap) + len(m.Trans) + len(c.Trs) + len(c.Feat)
	return n == 0
}

func TestCgnatDomainOnFake(t *testing.T) {
	v := coretest.New()
	cgnatFixtures(v)
	dir := t.TempDir()
	s := newSvc(t, v, dir)
	withNat := doc(t, strings.Replace(natIfDoc, "%s", cgnatPart, 1))
	resp := apply(t, s, &ngfwv1.ApplyRequest{TxnId: "c1", DesiredState: withNat})
	mustStatus(t, resp, ngfwv1.ApplyStatus_APPLY_STATUS_APPLIED)
	want := doc(t, `{"nat":`+canonicalCgnat+`}`).GetNat()
	if got := natNat(t, s); !proto.Equal(got, want) {
		t.Fatalf("Retrieve nat != canonical:\n got %s\nwant %s", protojson.Format(got), protojson.Format(want))
	}
	if resp = apply(t, s, &ngfwv1.ApplyRequest{TxnId: "c2", DesiredState: withNat}); len(resp.GetResults()) != 0 {
		t.Fatalf("re-apply changed %v", resp.GetResults())
	}

	// loss behind the agent's back + agent restart (a fresh Service on the same state dir and persisted claims)
	d, ds, m, c := v.Det44(), v.Dslite(), v.Map(), v.Cnat()
	d.Lock()
	d.Maps, d.Ifaces = nil, map[uint32]*det44.Det44InterfaceDetails{}
	d.Unlock()
	ds.Lock()
	ds.Pool = map[netip.Addr]bool{}
	ds.Unlock()
	c.Lock()
	c.Trs = map[uint32]cnatapi.CnatTranslation{}
	c.Unlock()
	s.Close()
	s2 := newSvc(t, v, dir)
	s2.Resync(context.Background())
	if got := natNat(t, s2); !proto.Equal(got, want) {
		t.Fatalf("after restart + resync:\n got %s\nwant %s", protojson.Format(got), protojson.Format(want))
	}
	m.Lock()
	domains := len(m.Domains)
	m.Unlock()
	if domains != 1 {
		t.Fatalf("map domains after restart: %d (want exactly 1, no duplicate)", domains)
	}

	// rollback: every object of this owner leaves VPP; det44 stays enabled (V9); the globals are untouched
	resp = apply(t, s2, &ngfwv1.ApplyRequest{TxnId: "c3", DesiredState: doc(t, `{"vrfs": {"cust": {"id": 7001}}, "interfaces": {}, "nat": {}}`)})
	mustStatus(t, resp, ngfwv1.ApplyStatus_APPLY_STATUS_APPLIED)
	if got := natNat(t, s2); proto.Size(got) != 0 {
		t.Fatalf("Retrieve after rollback: %s", protojson.Format(got))
	}
	if !cgnatEmpty(v) {
		t.Fatal("objects left in the models after rollback")
	}
	d.Lock()
	enabled, disables := d.Enabled, d.Disables
	d.Unlock()
	ds.Lock()
	aftr := netip.AddrFrom16(ds.Aftr.IP6Addr)
	ds.Unlock()
	c.Lock()
	crashes, snat := c.Crashes, c.Snat != nil
	c.Unlock()
	if !enabled || disables != 0 || aftr.String() != "fd00:7::1" || !snat || len(crashes) != 0 {
		t.Fatalf("after rollback: det44 enabled %v disables %d, aftr %s, snat entry %v, crashes %v", enabled, disables, aftr, snat, crashes)
	}
}

// TestCgnatSlotRequiresGlobals: without the fixture's AFTR the slot agent refuses (D-071), it never sets it.
func TestCgnatSlotRequiresGlobals(t *testing.T) {
	v := coretest.New()
	s := newSvc(t, v, t.TempDir())
	withNat := doc(t, strings.Replace(natIfDoc, "%s", `, "nat": {"dslite": {"enabled": true, "aftr": {"ipv6": "fd00:7::1"}}}`, 1))
	resp := apply(t, s, &ngfwv1.ApplyRequest{TxnId: "g1", DesiredState: withNat})
	if resp.GetStatus() == ngfwv1.ApplyStatus_APPLY_STATUS_APPLIED {
		t.Fatalf("slot agent applied a DS-Lite AFTR it does not own: %v", resp.GetResults())
	}
	ds := v.Dslite()
	ds.Lock()
	defer ds.Unlock()
	if ds.Sets != 0 {
		t.Fatalf("slot agent wrote the AFTR global %d times", ds.Sets)
	}
}
