package nftables_test

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	vrxv1 "ngfw/agent/gen/vrx/v1"
	"ngfw/agent/internal/renderers/nftables"
	"ngfw/agent/internal/renderers/nftables/nftest"
)

// F-global-blocking: block lists with protectHost become the in__gb chain (before conntrack) and the
// b4_/b6_ sets; overlapping entries are reduced to the outermost prefix; lists without protectHost,
// disabled or empty lists render nothing; anti-lockout accepts come first only with explicit sources.

func gbDoc(t *testing.T, js string) *vrxv1.DesiredState {
	t.Helper()
	ds := &vrxv1.DesiredState{}
	if err := protojson.Unmarshal([]byte(js), ds); err != nil {
		t.Fatal(err)
	}
	return ds
}

const gbList = `{"enabled": true, "source": {"kind": "upload"}, "allInterfaces": true, "direction": "both", "protectHost": %t, "log": %t, "entries": [%s]}`

func TestBlockListsBuild(t *testing.T) {
	ds := gbDoc(t, `{"acl": {"globalBlocking": {"lists": {
	  "bad":  `+fmt.Sprintf(gbList, true, true, `"10.0.0.0/8", "10.1.0.0/16", "192.0.2.7/32", "2001:db8::/32"`)+`,
	  "vpp":  `+fmt.Sprintf(gbList, false, false, `"198.51.100.1/32"`)+`,
	  "none": `+fmt.Sprintf(gbList, true, false, ``)+`
	}}}}`)
	v, issues := nftables.Build(nftables.Input{ACL: ds.GetAcl()})
	if len(issues) != 0 || v == nil {
		t.Fatalf("build: %v %v", v, issues)
	}
	if len(v.GetChains()) != 1 {
		t.Fatalf("chains: %v", v.GetChains())
	}
	c := v.GetChains()[0]
	if c.GetName() != nftables.BlockChain || c.GetHook() != "input" || c.GetPriority() != nftables.BlockPriority || c.GetPolicy() != "accept" || len(c.GetRules()) != 2 {
		t.Fatalf("chain: %v", c)
	}
	if got := c.GetRules()[0].GetText(); got != `ip saddr @b4_bad counter log prefix "vrx:gb:bad " drop` {
		t.Fatalf("rule text %q", got)
	}
	if r := c.GetRules()[1]; r.GetText() != `ip6 saddr @b6_bad counter log prefix "vrx:gb:bad " drop` || r.GetKind() != nftables.KindGlobalBlocking || r.GetList() != "bad" {
		t.Fatalf("rule %v", r)
	}
	if len(v.GetSets()) != 2 || strings.Join(v.GetSets()[0].GetElements(), ",") != "10.0.0.0/8,192.0.2.7/32" || v.GetSets()[1].GetName() != "b6_bad" {
		t.Fatalf("sets: %v", v.GetSets())
	}
	if _, err := nftables.RenderText("vrx", v); err != nil {
		t.Fatalf("render: %v", err)
	}

	// with host lists and anti-lockout sources: the block chain comes first and accepts management first
	ds.Acl.HostSettings = &vrxv1.HostAclSettings{AntiLockout: &vrxv1.HostAclAntiLockout{Enabled: proto.Bool(true), Sources: []string{"192.0.2.0/24"}, Ports: []uint32{22}}}
	v, _ = nftables.Build(nftables.Input{ACL: ds.GetAcl()})
	if c := v.GetChains()[0]; c.GetName() != nftables.BlockChain || c.GetRules()[0].GetKind() != nftables.KindAntiLockout || len(c.GetRules()) != 3 {
		t.Fatalf("with anti-lockout: %v", c)
	}

	// nothing protects the box → no table
	off := gbDoc(t, `{"acl": {"globalBlocking": {"lists": {"vpp": `+fmt.Sprintf(gbList, false, false, `"198.51.100.1/32"`)+`}}}}`)
	if v, _ := nftables.Build(nftables.Input{ACL: off.GetAcl()}); v != nil {
		t.Fatalf("no protectHost list must render nothing: %v", v)
	}
}

func manyHosts(n, skip int) string {
	var b strings.Builder
	for i := range n {
		if i == skip {
			continue
		}
		if b.Len() > 0 {
			b.WriteString(", ")
		}
		fmt.Fprintf(&b, `"10.%d.%d.%d/32"`, 64+i>>16&63, i>>8&255, i&255)
	}
	return b.String()
}

// Integration (VRX_INTEGRATION=1): a block list with the peer's address drops its connections to the box
// (the drop rule counts them); without it the peer connects; a 200 000-entry set applies in a measured
// time and Retrieve == desired; a one-entry change re-applies.
func TestIntegrationBlockListProtectsHost(t *testing.T) {
	h := nftest.New(t)
	peer := h.PeerAddr.String()
	a := newAgent(h)
	h.Listen(t, 2525)

	blocked := value(t, gbDoc(t, `{"acl": {"globalBlocking": {"lists": {"bad": `+fmt.Sprintf(gbList, true, false, `"`+peer+`/32", "203.0.113.0/24"`)+`}}}}`))
	a.apply(t, blocked, false)
	t.Logf("`nft list table inet %s`:\n%s", h.Paths().Table, h.Nft(t, "list", "table", "inet", h.Paths().Table))
	if got := a.retrieve(t); !proto.Equal(got, blocked) {
		t.Fatalf("Retrieve != desired:\n got %v\nwant %v", got, blocked)
	}
	if err := h.Dial(2525, 1500*time.Millisecond); err == nil {
		t.Fatal("the listed peer must be dropped")
	}
	if n := blockCounter(t, a); n == 0 {
		t.Fatal("the block rule counted nothing")
	} else {
		t.Logf("peer %s dropped; block rule counter %d packets", peer, n)
	}

	other := value(t, gbDoc(t, `{"acl": {"globalBlocking": {"lists": {"bad": `+fmt.Sprintf(gbList, true, false, `"203.0.113.0/24"`)+`}}}}`))
	a.apply(t, other, false)
	if err := h.Dial(2525, 3*time.Second); err != nil {
		t.Fatalf("an unlisted peer must connect: %v", err)
	}

	big := value(t, gbDoc(t, `{"acl": {"globalBlocking": {"lists": {"big": `+fmt.Sprintf(gbList, true, false, manyHosts(200_000, -1))+`}}}}`))
	start := time.Now()
	a.apply(t, big, false)
	t.Logf("200 000-entry set applied in %s", time.Since(start))
	start = time.Now()
	if got := a.retrieve(t); !proto.Equal(got, big) {
		t.Fatal("Retrieve != desired for 200 000 entries")
	}
	t.Logf("200 000-entry Retrieve in %s", time.Since(start))
	oneLess := value(t, gbDoc(t, `{"acl": {"globalBlocking": {"lists": {"big": `+fmt.Sprintf(gbList, true, false, manyHosts(200_000, 777))+`}}}}`))
	start = time.Now()
	a.apply(t, oneLess, false)
	t.Logf("one-entry change applied in %s", time.Since(start))
	if got := a.retrieve(t); !proto.Equal(got, oneLess) {
		t.Fatal("Retrieve != desired after the one-entry change")
	}
	a.apply(t, nil, false)
}

func blockCounter(t *testing.T, a *agent) uint64 {
	t.Helper()
	st, err := a.rt.State(context.Background(), time.Now())
	if err != nil {
		t.Fatal(err)
	}
	var n uint64
	for _, c := range st.GetChains() {
		for _, r := range c.GetRules() {
			if r.GetKind() == nftables.KindGlobalBlocking {
				n += r.GetPackets()
			}
		}
	}
	return n
}
