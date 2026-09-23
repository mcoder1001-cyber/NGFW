package df2

import (
	"context"
	"errors"
	"testing"

	"go.fd.io/govpp/adapter"
	"google.golang.org/protobuf/proto"

	"ngfw/agent/binapi/fib_types"
	interfaces "ngfw/agent/binapi/interface"
	"ngfw/agent/binapi/ip_types"
	"ngfw/agent/binapi/memclnt"
	"ngfw/agent/internal/vpp/fake"
)

func snapshot(t *testing.T) *Interfaces {
	t.Helper()
	f := fake.New(fake.WithControlPingReply(&memclnt.ControlPingReply{}))
	f.Reply("sw_interface_dump",
		&interfaces.SwInterfaceDetails{SwIfIndex: 0, InterfaceName: "local0"},
		&interfaces.SwInterfaceDetails{SwIfIndex: 5, InterfaceName: "loop300", Tag: "w3:loop300\x00\x00"},
		&interfaces.SwInterfaceDetails{SwIfIndex: 7, InterfaceName: "loop200", Tag: "w2:loop200"},
	)
	ifs, err := DumpInterfaces(context.Background(), f, "w3")
	if err != nil {
		t.Fatal(err)
	}
	return ifs
}

func TestKeys(t *testing.T) {
	if InterfaceKey("loop300") != "interface/loop300" || VRFKey(3001) != "vrf/3001" || ACLKey("web") != "acl/web" ||
		InterfaceIPKey("loop300", "2001:db8:3::/64") != "interface-ip/loop300/2001:db8:3::/64" {
		t.Fatal("key builders")
	}
	if VRFDeps(0) != nil || len(VRFDeps(3001)) != 1 || VRFDeps(3001)[0].Optional || InterfaceDep("x").Optional {
		t.Fatal("dependency builders")
	}
}

func TestAddresses(t *testing.T) {
	a, err := ParseAddr(" ::ffff:10.3.0.1 ")
	if err != nil || a.String() != "10.3.0.1" || FamilyOf(a) != AddressFamily_IPV4 {
		t.Fatalf("ParseAddr = %v %v", a, err)
	}
	if FromAddress(ToAddress(a)) != a || ToAddress(a).Af != ip_types.ADDRESS_IP4 {
		t.Fatal("v4 round trip")
	}
	a6, _ := ParseAddr("2001:DB8::1%eth0")
	if a6.String() != "2001:db8::1" || FromAddress(ToAddress(a6)) != a6 || FamilyOf(a6) != AddressFamily_IPV6 {
		t.Fatal("v6 round trip")
	}
	p, err := ParsePrefix("2001:db8:3::5/64")
	if err != nil || p.String() != "2001:db8:3::/64" || FromPrefix(ToPrefix(p)) != p {
		t.Fatalf("prefix = %v %v", p, err)
	}
	if _, err := ParseAddr("nope"); err == nil {
		t.Fatal("bad address accepted")
	}
	if _, err := ToIP4(a6); err == nil {
		t.Fatal("v6 accepted as v4")
	}
	if _, err := ToIP6(a); err == nil {
		t.Fatal("v4 accepted as v6")
	}
	m, err := ParseMAC("AA-BB-CC-00-11-22")
	if err != nil || MACString(m) != "aa:bb:cc:00:11:22" {
		t.Fatalf("mac = %v %v", m, err)
	}
	if ToIPTypesAF(FromIPTypesAF(ip_types.ADDRESS_IP6)) != ip_types.ADDRESS_IP6 {
		t.Fatal("af round trip")
	}
}

func TestInterfaces(t *testing.T) {
	ifs := snapshot(t)
	idx, err := ifs.Index("loop300")
	if err != nil || idx != 5 {
		t.Fatalf("Index = %d %v", idx, err)
	}
	if _, err := ifs.Index("nope"); !errors.Is(err, ErrNoSuchInterface) {
		t.Fatalf("missing: %v", err)
	}
	if n, ok := ifs.Name(7); !ok || n != "loop200" {
		t.Fatal("Name")
	}
	if !ifs.Owned(5) || ifs.Owned(7) || ifs.Owned(0) || ifs.Owned(99) {
		t.Fatal("Owned must follow the owner tag (trailing NULs stripped)")
	}
	if n, ok := ifs.OwnedName(5); !ok || n != "loop300" {
		t.Fatal("OwnedName")
	}
	if _, ok := ifs.OwnedName(7); ok {
		t.Fatal("foreign interface owned")
	}
}

func TestFibPaths(t *testing.T) {
	ifs := snapshot(t)
	desired := []*FibPath{
		{NextHop: "2001:DB8::1", TableId: 3002, Preference: 1},
		{NextHop: "10.3.1.254", Interface: "loop300", TableId: 99, Weight: 3, ResolveViaHost: true},
		{Type: FibPath_DROP},
	}
	norm, err := NormalizePaths(desired)
	if err != nil {
		t.Fatal(err)
	}
	if norm[0].NextHop != "10.3.1.254" || norm[0].TableId != 0 || norm[0].Proto != FibPath_IP4 || norm[1].NextHop != "2001:db8::1" ||
		norm[1].Weight != 1 || norm[1].Proto != FibPath_IP6 || norm[2].Type != FibPath_DROP || norm[2].Weight != 1 {
		t.Fatalf("Normalize = %+v", norm)
	}
	enc, err := EncodePaths(norm, ifs)
	if err != nil {
		t.Fatal(err)
	}
	if enc[0].SwIfIndex != 5 || enc[0].Weight != 3 || enc[0].Flags != fib_types.FIB_API_PATH_FLAG_RESOLVE_VIA_HOST ||
		enc[1].SwIfIndex != NoInterface || enc[1].TableID != 3002 || enc[1].Preference != 1 || enc[1].Proto != fib_types.FIB_API_PATH_NH_PROTO_IP6 ||
		enc[2].Type != fib_types.FIB_API_PATH_TYPE_DROP {
		t.Fatalf("Encode = %+v", enc)
	}
	// Decoding what VPP would dump (in any order) yields the normalised desired form.
	dec := DecodePaths([]fib_types.FibPath{enc[2], enc[0], enc[1]}, ifs)
	if len(dec) != 3 {
		t.Fatal(dec)
	}
	for i := range dec {
		if !proto.Equal(dec[i], norm[i]) {
			t.Fatalf("Decode[%d] = %+v, want %+v", i, dec[i], norm[i])
		}
	}
	if p := DecodePath(fib_types.FibPath{SwIfIndex: 42}, ifs); p.Interface != "#42" {
		t.Fatalf("unknown sw_if_index = %+v", p)
	}
	if _, err := EncodePath(&FibPath{Interface: "nope"}, ifs); !errors.Is(err, ErrNoSuchInterface) {
		t.Fatalf("unknown interface: %v", err)
	}
	if _, err := EncodePath(&FibPath{Weight: 300}, ifs); err == nil {
		t.Fatal("weight > 255 accepted")
	}
	if _, err := NormalizePaths([]*FibPath{{NextHop: "bad"}}); err == nil {
		t.Fatal("bad next hop accepted")
	}
}

func TestErrorsAndRange(t *testing.T) {
	err := PluginError("ip6_dad_autoremove", &adapter.UnknownMsgError{MsgName: "ip6_dad_dump", MsgCrc: "x"})
	if !errors.Is(err, ErrPluginNotLoaded) {
		t.Fatalf("PluginError = %v", err)
	}
	plain := errors.New("other")
	if PluginError("p", plain) != plain || PluginError("p", nil) != nil {
		t.Fatal("PluginError must pass other errors through")
	}
	var r *IDRange
	if !r.Owns(0) || !r.Owns(^uint32(0)) {
		t.Fatal("nil range owns everything")
	}
	r = &IDRange{Lo: 3000, Hi: 3999}
	if r.Owns(2999) || !r.Owns(3000) || !r.Owns(3999) || r.Owns(4000) {
		t.Fatal("range bounds")
	}
}
