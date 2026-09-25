package scheduler

import "testing"

// TD-11c: the traversal only adds edges that exist — an alias without a creator (a physical NIC)
// adds none — a chain through several non-node objects still reaches the node behind it, and a
// cycle among non-node objects terminates.
func TestTopoThroughNonNodes(t *testing.T) {
	s, _ := aliasFixture(t, "x.chain")
	nodes := map[Key]KV{
		"interface.admin-state/a":    kv("interface.admin-state", obj("a", "1", "interface/a")),
		"interface.admin-state/n":    kv("interface.admin-state", obj("n", "1", "interface/nic")),
		"af-packet.host-interface/a": kv("af-packet.host-interface", obj("a", "1")),
	}
	through := map[Key]KV{
		"interface/a":   kv("interface", obj("a", "1", "x.chain/a")), // alias → chain → creator
		"x.chain/a":     kv("x.chain", obj("a", "1", "af-packet.host-interface/a", "x.chain/b")),
		"x.chain/b":     kv("x.chain", obj("b", "1", "x.chain/a")), // a cycle among non-nodes
		"interface/nic": kv("interface", obj("nic", "1")),          // physical: no creator
	}
	order, cyc := s.topoThrough(nodes, through)
	if len(cyc) > 0 || len(order) != 3 {
		t.Fatalf("order %v cycle %v", order, cyc)
	}
	pos := map[Key]int{}
	for i, k := range order {
		pos[k] = i
	}
	if pos["af-packet.host-interface/a"] > pos["interface.admin-state/a"] {
		t.Fatalf("creator not before its attribute through two non-node hops: %v", order)
	}
}
