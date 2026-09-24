package df2

import (
	"context"
	"path/filepath"
	"testing"

	interfaces "ngfw/agent/binapi/interface"
	"ngfw/agent/binapi/memclnt"
	"ngfw/agent/internal/descriptors/acl"
	"ngfw/agent/internal/scheduler"
	"ngfw/agent/internal/vpp/fake"
)

type named string

func (n named) GetInterface() string { return string(n) }

func TestSkipDeleteReverifiesIdentity(t *testing.T) {
	ctx := context.Background()
	f := fake.New(fake.WithControlPingReply(&memclnt.ControlPingReply{}))
	f.Reply("sw_interface_dump",
		&interfaces.SwInterfaceDetails{SwIfIndex: 0, InterfaceName: "local0"},
		&interfaces.SwInterfaceDetails{SwIfIndex: 5, InterfaceName: "loop300", Tag: "w3:loop300"},
		&interfaces.SwInterfaceDetails{SwIfIndex: 7, InterfaceName: "loop200", Tag: "w2:loop200"},
		&interfaces.SwInterfaceDetails{SwIfIndex: 9, InterfaceName: "GigabitEthernet0/0/0"},
	)
	claims := acl.NewMemoryClaimStore()
	_ = claims.Claim("x/GigabitEthernet0/0/0")
	for _, c := range []struct {
		idx  uint32
		name string
		key  string
		skip bool
	}{
		{5, "loop300", "x/loop300", false},                           // ours
		{99, "loop300", "x/loop300", true},                           // interface gone
		{7, "loop300", "x/loop300", true},                            // index reused by another owner's interface
		{7, "loop200", "x/loop200", true},                            // foreign tag
		{9, "GigabitEthernet0/0/0", "x/GigabitEthernet0/0/0", false}, // untagged + claimed
		{9, "GigabitEthernet0/0/0", "y/GigabitEthernet0/0/0", true},  // untagged, not claimed
	} {
		skip, err := SkipDelete(ctx, f, "w3", c.idx, named(c.name), keyOf(c.key), claims)
		if err != nil || skip != c.skip {
			t.Errorf("SkipDelete(%d, %s, %s) = %v, %v; want %v", c.idx, c.name, c.key, skip, err, c.skip)
		}
	}
}

func TestFileClaimStorePersists(t *testing.T) {
	path := filepath.Join(t.TempDir(), "claims.json")
	s, err := OpenFileClaimStore(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Claim("a"); err != nil {
		t.Fatal(err)
	}
	if err := s.Claim("b"); err != nil {
		t.Fatal(err)
	}
	if err := s.Release("a"); err != nil {
		t.Fatal(err)
	}
	r, err := OpenFileClaimStore(path)
	if err != nil || r.Claimed("a") || !r.Claimed("b") {
		t.Fatalf("reopened: a=%v b=%v err=%v", r.Claimed("a"), r.Claimed("b"), err)
	}
}

func keyOf(s string) scheduler.Key { return scheduler.Key(s) }
