package acl

import (
	"errors"
	"fmt"
	"reflect"
	"testing"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/structpb"
)

func sampleRules() []Rule {
	return []Rule{
		{Action: ActionPermit, Src: "10.0.0.0/8", Dst: AnyV4, Proto: 6, SrcPortFirst: 0, SrcPortLast: 65535, DstPortFirst: 80, DstPortLast: 80},
		{Action: ActionReflect, Src: AnyV4, Dst: "192.168.1.10/32", Proto: 17, SrcPortFirst: 1024, SrcPortLast: 65535, DstPortFirst: 53, DstPortLast: 53},
		{Action: ActionPermit, Src: "2001:db8::/32", Dst: AnyV6, Proto: 58, SrcPortFirst: 128, SrcPortLast: 129, DstPortFirst: 0, DstPortLast: 255},
		{Action: ActionPermit, Src: AnyV4, Dst: AnyV4, Proto: 6, SrcPortFirst: 0, SrcPortLast: 65535, DstPortFirst: 0, DstPortLast: 65535, TCPFlagsMask: 0x12, TCPFlagsValue: 0x02},
		{Action: ActionDeny, Src: AnyV4, Dst: AnyV4},
		{Action: ActionDeny, Src: AnyV6, Dst: AnyV6},
		{Action: ActionPermit, Src: "fe80::1/128", Dst: "::/0", Proto: 1, SrcPortFirst: 0, SrcPortLast: 255, DstPortFirst: 0, DstPortLast: 255},
	}
}

// manyRules builds n distinct, valid rules (the 50-rule encoder-at-size case).
func manyRules(n int) []Rule {
	out := make([]Rule, 0, n)
	for i := 0; i < n; i++ {
		action := []Action{ActionPermit, ActionDeny, ActionReflect}[i%3]
		var r Rule
		if i%4 == 3 {
			r = Rule{Action: action, Src: fmt.Sprintf("2001:db8:%x::/48", i), Dst: AnyV6, Proto: 6,
				SrcPortFirst: 0, SrcPortLast: 65535, DstPortFirst: uint16(1000 + i), DstPortLast: uint16(1000 + i)} //nolint:gosec // small i
		} else {
			r = Rule{Action: action, Src: fmt.Sprintf("10.%d.0.0/16", i), Dst: fmt.Sprintf("172.16.%d.0/24", i), Proto: uint8(6 + 11*(i%2)), //nolint:gosec // 6 or 17
				SrcPortFirst: uint16(i), SrcPortLast: uint16(60000 + i), DstPortFirst: 0, DstPortLast: 65535, TCPFlagsMask: uint8(i), TCPFlagsValue: uint8(i)} //nolint:gosec // small i
		}
		out = append(out, r)
	}
	return out
}

func TestRulesRoundTrip(t *testing.T) {
	cases := sampleRules()
	// prefix-length sweeps, ports and ICMP ranges at their bounds
	for l := 0; l <= 32; l++ {
		cases = append(cases, Rule{Action: ActionPermit, Src: prefixOfLen(t, "10.20.30.40", l), Dst: AnyV4, Proto: 0})
	}
	for l := 0; l <= 128; l += 7 {
		cases = append(cases, Rule{Action: ActionDeny, Src: AnyV6, Dst: prefixOfLen(t, "2001:db8:1:2:3:4:5:6", l), Proto: 17, SrcPortLast: 65535, DstPortLast: 65535})
	}
	cases = append(cases,
		Rule{Action: ActionPermit, Src: AnyV4, Dst: AnyV4, Proto: 6, SrcPortFirst: 65535, SrcPortLast: 65535, DstPortFirst: 65535, DstPortLast: 65535, TCPFlagsMask: 0xff, TCPFlagsValue: 0xff},
		Rule{Action: ActionPermit, Src: AnyV4, Dst: AnyV4, Proto: 1, SrcPortFirst: 8, SrcPortLast: 8, DstPortFirst: 0, DstPortLast: 0},
		Rule{Action: ActionPermit, Src: AnyV4, Dst: AnyV4, Proto: 255},
	)
	cases = append(cases, manyRules(50)...)

	wire, err := encodeRules(cases)
	if err != nil {
		t.Fatal(err)
	}
	back, err := decodeRules(wire)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(back, cases) {
		for i := range cases {
			if i < len(back) && back[i] != cases[i] {
				t.Errorf("rule %d: got %+v, want %+v", i, back[i], cases[i])
			}
		}
		t.Fatalf("round trip is not byte-identical (%d rules)", len(cases))
	}
	// order is preserved: the wire order is the desired order
	if wire[0].DstportOrIcmpcodeFirst != 80 || wire[1].IsPermit != 2 || wire[2].Proto != 58 {
		t.Fatalf("wire order/encoding wrong: %+v", wire[:3])
	}
}

func prefixOfLen(t *testing.T, addr string, l int) string {
	t.Helper()
	p, err := parsePrefixLoose(fmt.Sprintf("%s/%d", addr, l))
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func TestMacipRulesRoundTrip(t *testing.T) {
	cases := []MacipRule{
		{Action: ActionPermit, SrcMac: "02:00:00:00:00:01", SrcMacMask: "ff:ff:ff:ff:ff:ff", SrcPrefix: "10.10.1.0/24"},
		{Action: ActionDeny, SrcMac: "00:00:00:00:00:00", SrcMacMask: "00:00:00:00:00:00", SrcPrefix: AnyV4},
		{Action: ActionPermit, SrcMac: "aa:bb:cc:00:00:00", SrcMacMask: "ff:ff:ff:00:00:00", SrcPrefix: "2001:db8::/64"},
		{Action: ActionDeny, SrcMac: "ff:ff:ff:ff:ff:ff", SrcMacMask: "ff:ff:ff:ff:ff:ff", SrcPrefix: AnyV6},
	}
	wire, err := encodeMacipRules(cases)
	if err != nil {
		t.Fatal(err)
	}
	back, err := decodeMacipRules(wire)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(back, cases) {
		t.Fatalf("macip round trip: got %+v, want %+v", back, cases)
	}
	if _, err := encodeMacipRules([]MacipRule{{Action: ActionReflect, SrcMac: "02:00:00:00:00:01", SrcMacMask: "ff:ff:ff:ff:ff:ff", SrcPrefix: AnyV4}}); !errors.Is(err, ErrSpec) {
		t.Fatalf("reflect must be rejected for MACIP rules, got %v", err)
	}
}

func TestPrefixCanonical(t *testing.T) {
	bad := []string{"", "10.0.0.1/24", "010.0.0.0/8", "2001:DB8::/32", "1.2.3.4", "1.2.3.4/33", "::1/129", "0.0.0.0/0 ", "2001:db8:0:0::/32"}
	for _, s := range bad {
		if _, err := parsePrefix(s); !errors.Is(err, ErrSpec) {
			t.Errorf("parsePrefix(%q) = %v, want ErrSpec", s, err)
		}
	}
	good := []string{AnyV4, AnyV6, "10.0.0.0/8", "192.168.1.10/32", "2001:db8::/32", "fe80::1/128", "::ffff:1.2.3.4/128"}
	for _, s := range good {
		if _, err := parsePrefix(s); err != nil {
			t.Errorf("parsePrefix(%q) = %v", s, err)
		}
	}
}

func TestMACCanonical(t *testing.T) {
	for _, s := range []string{"", "AA:BB:CC:DD:EE:FF", "aa-bb-cc-dd-ee-ff", "aabb.ccdd.eeff", "01:02:03:04:05:06:07:08", "zz:00:00:00:00:00"} {
		if _, err := parseMAC(s); !errors.Is(err, ErrSpec) {
			t.Errorf("parseMAC(%q) = %v, want ErrSpec", s, err)
		}
	}
	if _, err := parseMAC("aa:bb:cc:dd:ee:ff"); err != nil {
		t.Error(err)
	}
}

func TestSpecValidation(t *testing.T) {
	anyPorts := Rule{Action: ActionPermit, Src: AnyV4, Dst: AnyV4, SrcPortLast: 65535, DstPortLast: 65535}
	cases := map[string]error{
		"empty name":          ACL{Name: "", Rules: []Rule{anyPorts}}.Validate(),
		"mixed family":        ACL{Name: "x", Rules: []Rule{{Action: ActionPermit, Src: AnyV4, Dst: AnyV6}}}.Validate(),
		"first>last src":      ACL{Name: "x", Rules: []Rule{{Action: ActionPermit, Src: AnyV4, Dst: AnyV4, SrcPortFirst: 2, SrcPortLast: 1}}}.Validate(),
		"first>last dst":      ACL{Name: "x", Rules: []Rule{{Action: ActionPermit, Src: AnyV4, Dst: AnyV4, DstPortFirst: 2, DstPortLast: 1}}}.Validate(),
		"bad action":          ACL{Name: "x", Rules: []Rule{{Action: "allow", Src: AnyV4, Dst: AnyV4}}}.Validate(),
		"binding dup":         InterfaceBinding{Interface: "loop1", Input: []string{"a", "a"}}.Validate(),
		"binding empty name":  InterfaceBinding{Interface: "loop1", Output: []string{""}}.Validate(),
		"binding no iface":    InterfaceBinding{Input: []string{"a"}}.Validate(),
		"binding >255":        InterfaceBinding{Interface: "loop1", Input: names(200), Output: names(56)}.Validate(),
		"etype unsorted":      EtypeWhitelist{Interface: "loop1", Input: []uint16{0x88cc, 0x0806}}.Validate(),
		"etype dup":           EtypeWhitelist{Interface: "loop1", Output: []uint16{0x0806, 0x0806}}.Validate(),
		"macip binding noacl": MacipBinding{Interface: "loop1"}.Validate(),
		"macip bad mask":      MacipACL{Name: "m", Rules: []MacipRule{{Action: ActionPermit, SrcMac: "02:00:00:00:00:01", SrcMacMask: "FF:ff:ff:ff:ff:ff", SrcPrefix: AnyV4}}}.Validate(),
	}
	for name, err := range cases {
		if !errors.Is(err, ErrSpec) {
			t.Errorf("%s: got %v, want ErrSpec", name, err)
		}
	}
	ok := []error{
		ACL{Name: "x", Rules: sampleRules()}.Validate(),
		ACL{Name: "empty-rules"}.Validate(),
		InterfaceBinding{Interface: "loop1", Input: []string{"a", "b"}, Output: []string{"a"}}.Validate(),
		InterfaceBinding{Interface: "loop1"}.Validate(),
		EtypeWhitelist{Interface: "loop1", Input: []uint16{0x0806, 0x88cc}, Output: []uint16{0x0806}}.Validate(),
		MacipBinding{Interface: "loop1", ACL: "m"}.Validate(),
	}
	for i, err := range ok {
		if err != nil {
			t.Errorf("valid case %d rejected: %v", i, err)
		}
	}
}

func names(n int) []string {
	out := make([]string, n)
	for i := range out {
		out[i] = fmt.Sprintf("acl%d", i)
	}
	return out
}

func TestProtoCodec(t *testing.T) {
	a := ACL{Name: "lan-in", Rules: sampleRules()}
	back, err := FromProto(a.Proto())
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(back, a) {
		t.Fatalf("ACL proto round trip: got %+v, want %+v", back, a)
	}
	if !proto.Equal(a.Proto(), a.Proto()) {
		t.Fatal("Proto() must be deterministic")
	}
	m := MacipACL{Name: "m", Rules: []MacipRule{{Action: ActionPermit, SrcMac: "02:00:00:00:00:01", SrcMacMask: "ff:ff:ff:ff:ff:ff", SrcPrefix: AnyV4}}}
	if mb, err := MacipACLFromProto(m.Proto()); err != nil || !reflect.DeepEqual(mb, m) {
		t.Fatalf("MacipACL round trip: %+v, %v", mb, err)
	}
	b := InterfaceBinding{Interface: "loop1040", Input: []string{"a", "b"}, Output: []string{"c"}}
	if bb, err := InterfaceBindingFromProto(b.Proto()); err != nil || !reflect.DeepEqual(bb, b) {
		t.Fatalf("InterfaceBinding round trip: %+v, %v", bb, err)
	}
	w := EtypeWhitelist{Interface: "loop1040", Input: []uint16{0x0806, 0x88cc}, Output: []uint16{}}
	if wb, err := EtypeWhitelistFromProto(w.Proto()); err != nil || !reflect.DeepEqual(wb, w) {
		t.Fatalf("EtypeWhitelist round trip: %+v, %v", wb, err)
	}
	mbind := MacipBinding{Interface: "loop1040", ACL: "m"}
	if x, err := MacipBindingFromProto(mbind.Proto()); err != nil || x != mbind {
		t.Fatalf("MacipBinding round trip: %+v, %v", x, err)
	}
	if s, err := StatsEnableFromProto(StatsEnable{Enabled: true}.Proto()); err != nil || !s.Enabled {
		t.Fatalf("StatsEnable round trip: %+v, %v", s, err)
	}

	// tolerance: missing fields read as zero; wrong kinds and non-Struct values are errors
	partial, _ := structpb.NewStruct(map[string]any{FieldName: "p", FieldRules: []any{map[string]any{FieldAction: "deny", FieldSrc: AnyV4, FieldDst: AnyV4}}})
	if got, err := FromProto(partial); err != nil || got.Rules[0].Proto != 0 || got.Rules[0].SrcPortLast != 0 {
		t.Fatalf("partial decode: %+v, %v", got, err)
	}
	wrong, _ := structpb.NewStruct(map[string]any{FieldName: 5})
	if _, err := FromProto(wrong); !errors.Is(err, ErrSpec) {
		t.Fatalf("wrong kind: %v", err)
	}
	tooBig, _ := structpb.NewStruct(map[string]any{FieldName: "p", FieldRules: []any{map[string]any{FieldProto: 256.0}}})
	if _, err := FromProto(tooBig); !errors.Is(err, ErrSpec) {
		t.Fatalf("out of range: %v", err)
	}
	if _, err := InterfaceBindingFromProto(structpb.NewStringValue("x")); !errors.Is(err, ErrSpec) {
		t.Fatalf("non-struct: %v", err)
	}
	if _, err := EtypeWhitelistFromProto(mustStruct(map[string]any{FieldInterface: "l", FieldInput: []any{70000.0}})); !errors.Is(err, ErrSpec) {
		t.Fatalf("etype out of range: %v", err)
	}
}
