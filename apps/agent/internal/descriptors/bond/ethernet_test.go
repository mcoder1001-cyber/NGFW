package bond_test

import (
	"errors"
	"testing"

	"ngfw/agent/binapi/interface_types"
	"ngfw/agent/internal/descriptors/bond"
)

// TestMemberMustBeEthernet (F-bonding review F4): VPP 26.06 bond_add_member rejects only a bond and then copies the
// member's hardware address, which a tunnel does not have. bond.member refuses a member without an L2 address, a
// sub-interface and a loopback before it sends anything to VPP.
func TestMemberMustBeEthernet(t *testing.T) {
	f := newFake()
	bd, md := bond.NewBond(f, owner), bond.NewMember(f, owner)
	mustCreate(t, bd, &bond.Bond{Name: "w2-bond0", Id: 200, Mode: bond.Mode_MODE_XOR, Lb: bond.LoadBalance_LOAD_BALANCE_L2})
	wg := f.Add("wg0", "wireguard", "") // an L3 tunnel: VPP reports no L2 address
	f.Ifs[wg].L2Address = [6]uint8{}
	sub := f.Add("tap0.100", "tap", "")
	f.Ifs[sub].Type = interface_types.IF_API_TYPE_SUB
	f.Ifs[sub].SupSwIfIndex = f.tap
	f.Add("loop9", "Loopback", "")
	for _, m := range []string{"wg0", "tap0.100", "loop9"} {
		_, err := md.Create(ctx, &bond.Member{Bond: bondKey, Interface: "interface/" + m})
		if !errors.Is(err, bond.ErrNotEthernet) {
			t.Fatalf("member %s: err = %v, want ErrNotEthernet", m, err)
		}
	}
	if n := len(f.CallsNamed("bond_add_member")); n != 0 {
		t.Fatalf("bond_add_member was sent %d times for non-Ethernet members", n)
	}
	// an Ethernet tap still joins
	mustCreate(t, md, &bond.Member{Bond: bondKey, Interface: tapKey})
	if n := len(f.CallsNamed("bond_add_member")); n != 1 {
		t.Fatalf("bond_add_member calls = %d", n)
	}
}
