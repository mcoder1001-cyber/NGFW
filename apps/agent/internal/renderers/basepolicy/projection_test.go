package basepolicy

import (
	"ngfw/agent/internal/descriptors/lcp"
	"testing"
)

func TestProjectionEffectiveNamespaceAndOwnership(t *testing.T) {
	pairs := []lcp.ItfPair{{Interface: "loop1", HostIfName: "tap1", HostIfType: "tap"}, {Interface: "loop2", HostIfName: "tap2", HostIfType: "tap", Netns: "other"}}
	got, err := Project("mgmt0", nil, pairs, "", true)
	if err != nil || len(got) != 1 || got[0].Pair != "lcp.itf-pair/loop1" {
		t.Fatalf("%v %v", got, err)
	}
	got, err = Project("mgmt0", nil, pairs, "default-ns", true)
	if err != nil || len(got) != 0 {
		t.Fatalf("default namespace admitted %v %v", got, err)
	}
	if _, err = Project("mgmt0", nil, pairs, "", false); err == nil {
		t.Fatal("unknown namespace admitted")
	}
	if _, err = Project("mgmt0", []string{"tap1"}, pairs, "", true); err == nil {
		t.Fatal("permanent overlap admitted")
	}
	pairs[1].Netns = ""
	pairs[1].HostIfName = "tap1"
	if _, err = Project("mgmt0", nil, pairs, "", true); err == nil {
		t.Fatal("ambiguous host admitted")
	}
	pairs[1].Netns = "other"
	if _, err = Project("mgmt0", nil, pairs, "", true); err != nil {
		t.Fatal("foreign namespace collision affected root projection", err)
	}
}
