package desired

import (
	"testing"

	"google.golang.org/protobuf/proto"

	iface "ngfw/agent/internal/descriptors/interface"
)

func TestUnnumberedProjection(t *testing.T) {
	s := &sink{}
	Interfaces(s, ifsOf(t, `{"loop1":{"ipv4":["192.0.2.1/32"]},"lan":{"unnumbered":"loop1"}}`), func(string) (uint32, bool) { return 0, true }, nil)
	want := &iface.Unnumbered{Interface: "interface/lan", Donor: "interface/loop1"}
	if len(s.errs) != 0 || !proto.Equal(s.value("interface.unnumbered/lan"), want) {
		t.Fatalf("projection=%v errors=%v", s.kvs, s.errs)
	}
}
func TestUnnumberedProjectionRejectsInvalidGraphs(t *testing.T) {
	for _, js := range []string{
		`{"lan":{"unnumbered":"missing"}}`,
		`{"lan":{"unnumbered":"lan"}}`,
		`{"lan":{"unnumbered":"wan"},"wan":{"unnumbered":"lan"}}`,
		`{"lan":{"unnumbered":"wan","vrf":"blue"},"wan":{}}`,
		`{"lan":{"unnumbered":"wan","ipv4":["192.0.2.1/32"]},"wan":{}}`,
		`{"lan":{"unnumbered":"wan","dhcpClient":{}},"wan":{}}`,
	} {
		s := &sink{}
		unnumberedInterfaces(s, ifsOf(t, js))
		if len(s.errs) == 0 || len(s.kvs) != 0 {
			t.Fatalf("accepted %s: %v %v", js, s.kvs, s.errs)
		}
	}
}
func TestUnnumberedSubinterfaceProjection(t *testing.T) {
	s := &sink{}
	unnumberedInterfaces(s, ifsOf(t, `{"loop1":{},"lan":{"subinterfaces":{"10":{"vlanId":10,"unnumbered":"loop1"}}}}`))
	if s.value("interface.unnumbered/lan.10") == nil || len(s.errs) != 0 {
		t.Fatalf("projection=%v errors=%v", s.kvs, s.errs)
	}
}
