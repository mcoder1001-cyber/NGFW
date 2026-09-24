package desired

import (
	"fmt"
	"testing"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	vrxv1 "ngfw/agent/gen/vrx/v1"
	"ngfw/agent/internal/descriptors/bond"
	iface "ngfw/agent/internal/descriptors/interface"
	"ngfw/agent/internal/scheduler"
)

type sink struct {
	kvs      []scheduler.KV
	pointers map[scheduler.Key]string
	errs     []string
}

func (s *sink) Add(k scheduler.Key, v proto.Message, pointer string) {
	s.kvs = append(s.kvs, scheduler.KV{Key: k, Value: v})
	if s.pointers == nil {
		s.pointers = map[scheduler.Key]string{}
	}
	s.pointers[k] = pointer
}
func (s *sink) Errorf(pointer, rule, format string, a ...any) {
	s.errs = append(s.errs, pointer+" "+rule+": "+fmt.Sprintf(format, a...))
}
func (s *sink) Warnf(string, string, string, ...any) {}

func (s *sink) value(k scheduler.Key) proto.Message {
	for _, kv := range s.kvs {
		if kv.Key == k {
			return kv.Value
		}
	}
	return nil
}

func ifsOf(t *testing.T, js string) map[string]*vrxv1.Interface {
	t.Helper()
	ds := &vrxv1.DesiredState{}
	if err := protojson.Unmarshal([]byte(`{"interfaces":`+js+`}`), ds); err != nil {
		t.Fatal(err)
	}
	return ds.GetInterfaces()
}

func TestBondKind(t *testing.T) {
	for name, want := range map[string]Kind{"BondEthernet0": KindBond, "BondEthernet6000": KindBond, "BondEthernet01": KindExisting, "BondEthernet4294967295": KindExisting, "bond0": KindExisting} {
		if got, _ := KindOf(name); got != want {
			t.Errorf("KindOf(%s) = %v, want %v", name, got, want)
		}
	}
	// the alias names bond.bond as creator only when the document carries the bond leaf
	s := &sink{}
	Interfaces(s, ifsOf(t, `{"BondEthernet1": {"bond": {"mode": "xor"}}, "BondEthernet2": {"enabled": true}}`), func(string) (uint32, bool) { return 0, true }, nil)
	if a := s.value("interface/BondEthernet1").(*iface.InterfaceAlias); a.GetCreator() != "bond.bond/BondEthernet1" {
		t.Fatalf("alias %v", a)
	}
	if a := s.value("interface/BondEthernet2").(*iface.InterfaceAlias); a.GetCreator() != "" {
		t.Fatalf("pre-existing bond alias %v", a)
	}
}

func TestBondsBuilder(t *testing.T) {
	s := &sink{}
	Bonds(s, ifsOf(t, `{
	  "BondEthernet6000": {"bond": {"mode": "lacp", "loadBalance": "l34", "numaOnly": true,
	    "members": {"tap1": {"passive": true}, "tap0": {"longTimeout": true}}}},
	  "BondEthernet6001": {"bond": {"mode": "active-backup", "id": 6001, "members": {"tap2": {"weight": 7}, "tap3": {}}}},
	  "BondEthernet6002": {"bond": {"mode": "xor"}},
	  "tap0": {}, "tap1": {}, "tap2": {}, "tap3": {}}`))
	if len(s.errs) != 0 {
		t.Fatal(s.errs)
	}
	for k, want := range map[scheduler.Key]proto.Message{
		"bond.bond/BondEthernet6000":               &bond.Bond{Name: "BondEthernet6000", Id: 6000, Mode: bond.Mode_MODE_LACP, Lb: bond.LoadBalance_LOAD_BALANCE_L34, NumaOnly: true},
		"bond.bond/BondEthernet6001":               &bond.Bond{Name: "BondEthernet6001", Id: 6001, Mode: bond.Mode_MODE_ACTIVE_BACKUP, Lb: bond.LoadBalance_LOAD_BALANCE_ACTIVE_BACKUP},
		"bond.bond/BondEthernet6002":               &bond.Bond{Name: "BondEthernet6002", Id: 6002, Mode: bond.Mode_MODE_XOR, Lb: bond.LoadBalance_LOAD_BALANCE_L2},
		"bond.member/BondEthernet6000/tap0":        &bond.Member{Bond: "interface/BondEthernet6000", Interface: "interface/tap0", LongTimeout: true},
		"bond.member/BondEthernet6000/tap1":        &bond.Member{Bond: "interface/BondEthernet6000", Interface: "interface/tap1", Passive: true},
		"bond.member/BondEthernet6001/tap3":        &bond.Member{Bond: "interface/BondEthernet6001", Interface: "interface/tap3"},
		"bond.member-weight/BondEthernet6001/tap2": bond.Weight{Bond: "interface/BondEthernet6001", Interface: "interface/tap2", Weight: 7}.Proto(),
	} {
		if got := s.value(k); !proto.Equal(got, want) {
			t.Errorf("%s = %v, want %v", k, got, want)
		}
	}
	if len(s.kvs) != 8 || s.pointers["bond.member-weight/BondEthernet6001/tap2"] != "/interfaces/BondEthernet6001/bond/members/tap2/weight" ||
		s.pointers["bond.member/BondEthernet6000/tap1"] != "/interfaces/BondEthernet6000/bond/members/tap1" {
		t.Fatalf("objects %v pointers %v", s.kvs, s.pointers)
	}
}

func TestBondsBuilderErrors(t *testing.T) {
	for js, want := range map[string]string{
		`{"lag0": {"bond": {"mode": "xor"}}}`:                                                                                                           "/interfaces/lag0/bond interfaces.bonding-name",
		`{"BondEthernet1": {"bond": {"mode": "xor", "id": 2}}}`:                                                                                         "/interfaces/BondEthernet1/bond/id interfaces.bonding-name",
		`{"BondEthernet1": {"bond": {"mode": "802.3ad"}}}`:                                                                                              "/interfaces/BondEthernet1/bond/mode interfaces.bonding-mode",
		`{"BondEthernet1": {"bond": {"mode": "broadcast", "loadBalance": "l2"}}}`:                                                                       "/interfaces/BondEthernet1/bond/loadBalance interfaces.bonding-load-balance",
		`{"BondEthernet1": {"bond": {"mode": "xor", "loadBalance": "l4"}}}`:                                                                             "/interfaces/BondEthernet1/bond/loadBalance interfaces.bonding-load-balance",
		`{"BondEthernet1": {"bond": {"mode": "xor", "members": {"x": {}}}}}`:                                                                            "/interfaces/BondEthernet1/bond/members/x interfaces.bonding-member-exists",
		`{"BondEthernet1": {"bond": {"mode": "xor", "members": {"loop1": {}}}}, "loop1": {}}`:                                                           "/interfaces/BondEthernet1/bond/members/loop1 interfaces.bonding-member-kind",
		`{"BondEthernet1": {"bond": {"mode": "xor", "members": {"BondEthernet2": {}}}}, "BondEthernet2": {}}`:                                           "/interfaces/BondEthernet1/bond/members/BondEthernet2 interfaces.bonding-member-kind",
		`{"BondEthernet1": {"bond": {"mode": "xor", "members": {"a": {}}}}, "BondEthernet2": {"bond": {"mode": "xor", "members": {"a": {}}}}, "a": {}}`: "/interfaces/BondEthernet2/bond/members/a interfaces.bonding-member-unique",
		`{"BondEthernet1": {"bond": {"mode": "xor", "members": {"a": {"passive": true}}}}, "a": {}}`:                                                    "/interfaces/BondEthernet1/bond/members/a interfaces.bonding-lacp-options",
		`{"BondEthernet1": {"bond": {"mode": "lacp", "members": {"a": {"weight": 1}}}}, "a": {}}`:                                                       "/interfaces/BondEthernet1/bond/members/a/weight interfaces.bonding-weight",
	} {
		s := &sink{}
		Bonds(s, ifsOf(t, js))
		if len(s.errs) != 1 || len(s.errs[0]) < len(want) || s.errs[0][:len(want)] != want {
			t.Errorf("%s: errors %v, want %q", js, s.errs, want)
		}
	}
}

func TestAssembleBonds(t *testing.T) {
	kvs := []scheduler.KV{
		{Key: "bond.bond/BondEthernet1", Value: &bond.Bond{Name: "BondEthernet1", Id: 1, Mode: bond.Mode_MODE_LACP, Lb: bond.LoadBalance_LOAD_BALANCE_L2}},
		{Key: "bond.bond/BondEthernet2", Value: &bond.Bond{Name: "BondEthernet2", Id: 2, Mode: bond.Mode_MODE_XOR, Lb: bond.LoadBalance_LOAD_BALANCE_L23}},
		{Key: "bond.bond/BondEthernet3", Value: &bond.Bond{Name: "BondEthernet3", Id: 3, Mode: bond.Mode_MODE_ACTIVE_BACKUP, Lb: bond.LoadBalance_LOAD_BALANCE_ACTIVE_BACKUP, NumaOnly: true}},
		{Key: "bond.member/BondEthernet3/b", Value: &bond.Member{Bond: "interface/BondEthernet3", Interface: "interface/b"}},
		{Key: "bond.member/BondEthernet3/a", Value: &bond.Member{Bond: "interface/BondEthernet3", Interface: "interface/a", Passive: true}},
		{Key: "bond.member-weight/BondEthernet3/a", Value: bond.Weight{Bond: "interface/BondEthernet3", Interface: "interface/a", Weight: 9}.Proto()},
	}
	stored := ifsOf(t, `{"BondEthernet1": {"description": "uplink", "bond": {"mode": "lacp", "loadBalance": "l2", "id": 1}}, "BondEthernet2": {"bond": {"mode": "xor"}}}`)
	ds := &vrxv1.DesiredState{Interfaces: map[string]*vrxv1.Interface{"BondEthernet2": {Enabled: proto.Bool(true), Promiscuous: proto.Bool(false), Vrf: proto.String("default")}}}
	AssembleBonds(ds, kvs, stored, func(uint32) string { return "default" })
	want := ifsOf(t, `{
	  "BondEthernet1": {"enabled": false, "promiscuous": false, "vrf": "default", "description": "uplink",
	    "bond": {"mode": "lacp", "loadBalance": "l2", "id": 1, "numaOnly": false}},
	  "BondEthernet2": {"enabled": true, "promiscuous": false, "vrf": "default", "bond": {"mode": "xor", "loadBalance": "l23", "numaOnly": false}},
	  "BondEthernet3": {"enabled": false, "promiscuous": false, "vrf": "default",
	    "bond": {"mode": "active-backup", "numaOnly": true, "members": {"a": {"passive": true, "longTimeout": false, "weight": 9}, "b": {"passive": false, "longTimeout": false}}}}}`)
	for name, w := range want {
		if got := ds.GetInterfaces()[name]; !proto.Equal(got, w) {
			t.Errorf("%s = %v, want %v", name, got, w)
		}
	}
	// nothing retrieved: the document is left alone
	empty := &vrxv1.DesiredState{}
	AssembleBonds(empty, nil, stored, func(uint32) string { return "default" })
	if empty.Interfaces != nil {
		t.Fatal("interfaces created without a bond")
	}
	if n, ok := BondID("BondEthernet4294967294"); !ok || n != 4294967294 {
		t.Fatal(n, ok)
	}
	if BondModeName(bond.Mode_MODE_UNSPECIFIED) != "" || BondLBName(bond.LoadBalance_LOAD_BALANCE_BROADCAST) != "broadcast" {
		t.Fatal("names")
	}
}
