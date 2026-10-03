package nat46_test

import (
	"fmt"
	"testing"

	"google.golang.org/protobuf/proto"

	"ngfw/agent/internal/descriptors/mapnat"
	"ngfw/agent/internal/descriptors/nat46"
	"ngfw/agent/internal/descriptors/natcommon"
	"ngfw/agent/internal/descriptors/natcommon/nattest"
	"ngfw/agent/internal/vpp/vpptest"
)

// TestNat46OnHost (NGFW_INTEGRATION=1, shared lab lock; skipped otherwise): the NAT46 projection
// applied through the mapnat descriptors on the host VPP. It answers the spike's open host
// question — does VPP 26.06 accept a 1:1 MAP-T domain (/32 ↔ /128, ea_bits_len 0) — asserts
// Retrieve == desired, a re-apply plans nothing (restart simulation at descriptor level) and
// that deleting leaves nothing of the slot. Packet-level evidence is the host row's job.
func TestNat46OnHost(t *testing.T) {
	c := nattest.Connect(t)
	ctx := nattest.Ctx(t)
	owner := vpptest.Prefix(t)
	p := mapnat.New(c, owner)

	a, _ := nattest.Loopback(t, c, 46)
	b, _ := nattest.Loopback(t, c, 47)
	cfg := nat46.Config{
		ClientPrefix: fmt.Sprintf("fd00:%x:4646::/96", vpptest.Slot(t)),
		Interfaces:   []string{a, b},
		Mappings:     []nat46.Mapping{{Name: owner + "-web", IPv4: nattest.Addr4(t, 46, 10), IPv6: nattest.Addr6(t, 0x4610), MTU: 1500}},
	}
	pr, err := nat46.Project(cfg)
	if err != nil {
		t.Fatal(err)
	}
	var doms, ifs []proto.Message
	for i := range pr.Domains {
		doms = append(doms, natcommon.MustEncode(&pr.Domains[i]))
	}
	for i := range pr.Interfaces {
		ifs = append(ifs, natcommon.MustEncode(&pr.Interfaces[i]))
	}
	nattest.CreateAll(ctx, t, p.Domain, doms...)
	nattest.AssertPlan(t, p.Domain, doms...)
	nattest.CreateAll(ctx, t, p.Interface, ifs...)
	nattest.AssertPlan(t, p.Interface, ifs...)
	client, _ := nat46.ClientAddress(cfg.ClientPrefix, "192.0.2.33")
	t.Logf("NAT46 domain accepted: %s → %s (IPv4 client 192.0.2.33 appears as %s)", cfg.Mappings[0].IPv4, cfg.Mappings[0].IPv6, client)

	nattest.Pause(t, "nat46") // evidence hook: vppctl show map domain
	nattest.DeleteAll(ctx, t, p.Interface)
	nattest.DeleteAll(ctx, t, p.Domain)
	nattest.AssertPlan(t, p.Domain)
	nattest.AssertPlan(t, p.Interface)
}
