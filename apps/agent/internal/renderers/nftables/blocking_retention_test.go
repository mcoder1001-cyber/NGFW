package nftables_test

import (
	"fmt"
	"strings"
	"testing"

	"google.golang.org/protobuf/proto"
	ngfwv1 "ngfw/agent/gen/ngfw/v1"
	"ngfw/agent/internal/renderers/nftables"
)

// Host-independent source controls only: Build/RenderText do not invoke nft or
// certify packets. Explicit management sources overlap the IPv6 block list.
func TestGlobalBlockingIPv6AntiLockoutConstraints(t *testing.T) {
	for _, tc := range []struct {
		name       string
		enabled    bool
		sources    []string
		wantAccept bool
	}{
		{"explicit IPv6 management", true, []string{"2001:db8:1::/64"}, true},
		{"no sources must not bypass blocking", true, nil, false},
		{"disabled must not bypass blocking", false, []string{"2001:db8:1::/64"}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ds := gbDoc(t, `{"acl":{"globalBlocking":{"lists":{"bad":`+fmt.Sprintf(gbList, true, false, `"2001:db8::/32", "2001:db8:1::/64", "2001:db8:1::7/128"`)+`}}}}`)
			ds.Acl.HostSettings = &ngfwv1.HostAclSettings{AntiLockout: &ngfwv1.HostAclAntiLockout{
				Enabled: proto.Bool(tc.enabled), Sources: tc.sources, Interfaces: []string{"mgmt0"}, Ports: []uint32{8443},
			}}
			v, issues := nftables.Build(nftables.Input{ACL: ds.Acl})
			if len(issues) != 0 || v == nil {
				t.Fatalf("Build: %v %v", v, issues)
			}
			var block *nftables.Chain
			for _, c := range v.GetChains() {
				if c.GetName() == nftables.BlockChain {
					block = c
				}
			}
			if block == nil {
				t.Fatal("missing global block input chain")
			}
			wantRules := 1
			if tc.wantAccept {
				wantRules++
			}
			if len(block.GetRules()) != wantRules {
				t.Fatalf("rules: %v", block.GetRules())
			}
			if tc.wantAccept {
				r := block.GetRules()[0]
				if r.GetKind() != nftables.KindAntiLockout {
					t.Fatalf("management accept is not first: %v", r)
				}
				if r.GetText() != `iifname "mgmt0" ip6 saddr 2001:db8:1::/64 tcp dport 8443 counter accept` {
					t.Fatalf("management exception widened or changed: %s", r.GetText())
				}
			}
			drop := block.GetRules()[wantRules-1]
			if drop.GetKind() != nftables.KindGlobalBlocking || drop.GetText() != "ip6 saddr @b6_bad counter drop" {
				t.Fatalf("IPv6 block: %v", drop)
			}
			if len(v.GetSets()) != 1 || v.GetSets()[0].GetType() != "ipv6_addr" || strings.Join(v.GetSets()[0].GetElements(), ",") != "2001:db8::/32" {
				t.Fatalf("overlapping IPv6 entries: %v", v.GetSets())
			}
			if _, err := nftables.RenderText("ngfw_fixture", v); err != nil {
				t.Fatal(err)
			}
		})
	}
}
