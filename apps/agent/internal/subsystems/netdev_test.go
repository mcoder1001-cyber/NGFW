package subsystems

import (
	"context"
	"encoding/binary"
	"errors"
	"strings"
	"testing"

	afpacket "ngfw/agent/internal/descriptors/af_packet"
)

// rtattr builds one netlink attribute (host byte order, 4-byte aligned).
func rtattr(typ uint16, val []byte) []byte {
	b := make([]byte, 4, 4+len(val)+3)
	binary.NativeEndian.PutUint16(b[0:2], uint16(4+len(val))) //nolint:gosec // test sizes are tiny
	binary.NativeEndian.PutUint16(b[2:4], typ)
	b = append(b, val...)
	for len(b)%4 != 0 {
		b = append(b, 0)
	}
	return b
}

func TestLinkInfoKind(t *testing.T) {
	data := rtattr(2, []byte{1, 2, 3}) // IFLA_INFO_DATA first
	if got := linkInfoKind(append(data, rtattr(iflaInfoKind, []byte("veth\x00"))...)); got != "veth" {
		t.Fatalf("kind %q", got)
	}
	if got := linkInfoKind(rtattr(2, []byte{1})); got != "" {
		t.Fatalf("kind without IFLA_INFO_KIND %q", got)
	}
	if got := linkInfoKind([]byte{0xff, 0xff, 1, 0, 'v'}); got != "" { // length beyond the buffer
		t.Fatalf("truncated %q", got)
	}
}

// TestLinuxNetdevKindOnThisHost: the loopback device exists and has no link kind; a name that cannot
// exist is reported missing.
func TestLinuxNetdevKindOnThisHost(t *testing.T) {
	kind, ok, err := LinuxNetdevKind("lo")
	if err != nil || !ok || kind != "" {
		t.Fatalf("lo: kind %q exists %v err %v", kind, ok, err)
	}
	if _, ok, err := LinuxNetdevKind("no-such-if-p08"); err != nil || ok {
		t.Fatalf("missing netdev: exists %v err %v", ok, err)
	}
	if err := CheckVeth(LinuxNetdevKind, "lo"); !errors.Is(err, ErrNotVeth) || !strings.Contains(err.Error(), "physical") {
		t.Fatalf("lo accepted: %v", err)
	}
}

func TestVethGuard(t *testing.T) {
	kinds := map[string]string{"w1l0": "veth", "ens192": "", "br0": "bridge"}
	lookup := func(n string) (string, bool, error) {
		k, ok := kinds[n]
		if n == "boom" {
			return "", false, errors.New("netlink down")
		}
		return k, ok, nil
	}
	for netdev, want := range map[string]string{"ens192": "physical", "br0": "a bridge", "gone": `no Linux netdev "gone"`, "boom": "netlink down"} {
		err := CheckVeth(lookup, netdev)
		if !errors.Is(err, ErrNotVeth) || !strings.Contains(err.Error(), want) {
			t.Errorf("%s: %v (want %q)", netdev, err, want)
		}
	}
	if err := CheckVeth(lookup, "w1l0"); err != nil {
		t.Fatal(err)
	}
	g := &vethOnly{Descriptor: afpacket.New(nil, "w1"), kind: lookup}
	if _, err := g.Create(context.Background(), &afpacket.HostInterface{Name: "host-ens192", HostIfName: "ens192"}); !errors.Is(err, ErrNotVeth) {
		t.Fatalf("guard let ens192 through: %v", err)
	}
}
