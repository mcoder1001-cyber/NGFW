package ldp

import (
	"fmt"
	"testing"
)

func TestPathCountBoundAndDeduplication(t *testing.T) {
	base := Binding{FEC: "198.51.100.0/24", LocalLabel: 16000, RemoteLabel: 17000, NextHop: "192.0.2.1", LinuxInterface: "tap"}
	duplicates := make([]Binding, 256)
	for i := range duplicates {
		duplicates[i] = base
	}
	mapping := map[string]string{"tap": "wan"}
	routes, err := Translate(duplicates, 7000, mapping)
	if err != nil || len(routes) != 1 || len(routes[0].Paths) != 1 {
		t.Fatalf("duplicate paths: %+v %v", routes, err)
	}
	distinct := make([]Binding, 256)
	for i := range distinct {
		distinct[i] = base
		distinct[i].NextHop = fmt.Sprintf("10.0.%d.%d", i/254, i%254+1)
	}
	if routes, err := Translate(distinct, 7000, mapping); err == nil || routes != nil {
		t.Fatalf("overflow accepted: %v", err)
	}
	if routes, err := Translate(distinct[:255], 7000, mapping); err != nil || len(routes[0].Paths) != 255 {
		t.Fatalf("255 paths rejected: %v", err)
	}
}

func TestPHPAndECMP(t *testing.T) {
	bindings := []Binding{
		{FEC: "198.51.100.0/24", LocalLabel: 16000, RemoteLabel: 3, NextHop: "192.0.2.1", LinuxInterface: "tap1"},
		{FEC: "198.51.100.0/24", LocalLabel: 16000, RemoteLabel: 17000, NextHop: "192.0.2.2", LinuxInterface: "tap2"},
		{FEC: "203.0.113.0/24", LocalLabel: 3},
	}
	got, err := Translate(bindings, 7000, map[string]string{"tap1": "wan1", "tap2": "wan2"})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Table != 7000 || got[0].Label != 16000 || len(got[0].Paths) != 2 {
		t.Fatalf("unexpected routes %+v", got)
	}
	for _, path := range got[0].Paths {
		if path.Interface == "wan1" && len(path.Labels) != 0 {
			t.Fatal("implicit null must pop")
		}
		if path.Interface == "wan2" && (len(path.Labels) != 1 || path.Labels[0].Label != 17000) {
			t.Fatal("remote label missing")
		}
	}
}

func TestInvalidReadDoesNotBecomeEmpty(t *testing.T) {
	valid := Binding{FEC: "198.51.100.0/24", LocalLabel: 16000, RemoteLabel: 17000, NextHop: "192.0.2.1", LinuxInterface: "tap1"}
	for _, mutate := range []func(*Binding){
		func(b *Binding) { b.FEC = "198.51.100.1/24" },
		func(b *Binding) { b.LocalLabel = 1048576 },
		func(b *Binding) { b.RemoteLabel = 0 },
		func(b *Binding) { b.NextHop = "224.0.0.2" },
		func(b *Binding) { b.LinuxInterface = "foreign" },
	} {
		invalid := valid
		mutate(&invalid)
		got, err := Translate([]Binding{valid, invalid}, 7000, map[string]string{"tap1": "wan"})
		if err == nil || got != nil {
			t.Fatalf("failed read produced %+v, %v", got, err)
		}
	}
	other := valid
	other.FEC = "203.0.113.0/24"
	if _, err := Translate([]Binding{valid, other}, 7000, map[string]string{"tap1": "wan"}); err == nil {
		t.Fatal("label collision accepted")
	}
}
