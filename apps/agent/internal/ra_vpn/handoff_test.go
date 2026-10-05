package ravpn

import (
	"ngfw/agent/internal/vpp/bootid"
	"testing"
)

func TestHandoffRejectsNamespaceAndEveryVPPBootIdentityChange(t *testing.T) {
	plan := networkFixture()
	plan.NamespaceInode = 100
	plan.HostNamespaceInode = 99
	boot := bootid.Identity{BootID: "host-a", PID: 1000, StartTime: 2000}
	receipt := Handoff{Format: 1, Instance: plan.Instance, NamespaceInode: 100, HostNamespaceInode: 99, VPPBoot: boot, OuterIndex: 19001, InnerIndex: 19002, OuterName: LinkName(plan.Instance, true), InnerName: LinkName(plan.Instance, false)}
	if ValidateHandoff(plan, receipt, boot, 19001, 19002) != nil {
		t.Fatal("matching complete handoff refused")
	}
	invalid := boot
	invalid.PID = -1
	invalidReceipt := receipt
	invalidReceipt.VPPBoot = invalid
	if ValidateHandoff(plan, invalidReceipt, invalid, 19001, 19002) == nil {
		t.Fatal("matching invalid PID adopted")
	}
	for _, changed := range []bootid.Identity{{BootID: "host-b", PID: 1000, StartTime: 2000}, {BootID: "host-a", PID: 1001, StartTime: 2000}, {BootID: "host-a", PID: 1000, StartTime: 2001}, {BootID: "host-a", PID: 1000}} {
		if ValidateHandoff(plan, receipt, changed, 19001, 19002) == nil {
			t.Fatal("stale or incomplete VPP identity adopted")
		}
	}
	plan.NamespaceInode++
	if ValidateHandoff(plan, receipt, boot, 19001, 19002) == nil {
		t.Fatal("replaced kernel namespace adopted")
	}
	plan.NamespaceInode--
	if ValidateHandoff(plan, receipt, boot, 19003, 19002) == nil {
		t.Fatal("recycled VPP index adopted")
	}
	receipt.InnerName = receipt.OuterName
	if ValidateHandoff(plan, receipt, boot, 19001, 19002) == nil {
		t.Fatal("foreign transit link adopted")
	}
}
