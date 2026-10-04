package main

import (
	"errors"
	"io"
	"testing"
)

func TestSlotBoundary(t *testing.T) {
	for _, n := range []int{0, 12, 13, 33} {
		if validSlot(n) {
			t.Fatalf("invalid slot %d", n)
		}
	}
	for _, n := range []int{1, 11, 14, 32} {
		if !validSlot(n) {
			t.Fatalf("valid slot %d", n)
		}
	}
}
func TestEmptinessRefusal(t *testing.T) {
	if err := drain(func() (int, error) { return 0, io.EOF }); err != nil {
		t.Fatal(err)
	}
	first := true
	if err := drain(func() (int, error) {
		if first {
			first = false
			return 1, nil
		}
		return 0, io.EOF
	}); err == nil {
		t.Fatal("remaining foreign object accepted")
	}
	expected := errors.New("dump failed")
	if err := drain(func() (int, error) { return 0, expected }); !errors.Is(err, expected) {
		t.Fatal(err)
	}
}
func TestForeignPluginOwnership(t *testing.T) {
	for _, flags := range [][2]bool{{true, false}, {false, true}, {true, true}} {
		if err := ownedState("", flags[0], flags[1], "", ""); err == nil {
			t.Fatal("preexisting foreign NAT mode accepted")
		}
	}
	if err := ownedState("ed", true, false, "owned-config", "owned-config"); err != nil {
		t.Fatal(err)
	}
	if err := ownedState("ed", true, false, "owned-config", "foreign-flags-capacity-timeouts"); err == nil {
		t.Fatal("external same-VRF config change accepted")
	}
	if err := ownedState("ei", true, false, "owned-config", "owned-config"); err == nil {
		t.Fatal("external mode replacement accepted")
	}
}
