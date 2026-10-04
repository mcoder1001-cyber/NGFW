package autoblock

import (
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
	"net/netip"
	ngfwv1 "ngfw/agent/gen/ngfw/v1"
	"testing"
	"time"
)

func TestOverlaySafetyAndExpiry(t *testing.T) {
	now := time.Now()
	ds := &ngfwv1.DesiredState{Security: &ngfwv1.SecurityConfig{AutoBlock: &ngfwv1.AutoBlock{Enabled: proto.Bool(true), Allowlist: []string{"192.0.2.1", "2001:db8:1::/48"}}}}
	entries := []*ngfwv1.AutoBlockRuntimeEntry{}
	for _, source := range []string{"192.0.2.1", "192.0.2.2", "::ffff:192.0.2.3", "127.0.0.1", "2001:db8:1::1", "2001:db8:2::1"} {
		entries = append(entries, &ngfwv1.AutoBlockRuntimeEntry{Source: source, ExpiresAt: timestamppb.New(now.Add(time.Minute))})
	}
	before := proto.Clone(ds)
	out, err := Overlay(ds, entries, now)
	if err != nil {
		t.Fatal(err)
	}
	list := out.GetAcl().GetGlobalBlocking().GetLists()[ListName]
	if len(list.GetEntries()) != 3 || !list.GetAllInterfaces() || !list.GetProtectHost() {
		t.Fatalf("unsafe overlay: %v", list)
	}
	if !proto.Equal(ds, before) {
		t.Fatal("configuration mutated")
	}
	out, err = Overlay(ds, entries, now.Add(2*time.Minute))
	if err != nil || out.GetAcl() != nil {
		t.Fatal("expiry did not remove runtime list")
	}
	if Allowed(ds, netip.MustParseAddr("192.0.2.2")) {
		t.Fatal("bare allowlist entry disabled all blocking")
	}
}
func TestSnapshotRejectsMalformedAndDuplicates(t *testing.T) {
	now := time.Now()
	for _, entries := range [][]*ngfwv1.AutoBlockRuntimeEntry{{{Source: "192.0.2.0/24", ExpiresAt: timestamppb.New(now)}}, {{Source: "192.0.2.1"}}, {{Source: "192.0.2.1", ExpiresAt: timestamppb.New(now.Add(31 * 24 * time.Hour))}}, {{Source: "192.0.2.1", ExpiresAt: timestamppb.New(now)}, {Source: "::ffff:192.0.2.1", ExpiresAt: timestamppb.New(now)}}} {
		if Validate(entries, now) == nil {
			t.Fatal("malformed snapshot accepted")
		}
	}
}
func TestManagementSourcesAndReservedList(t *testing.T) {
	ds := &ngfwv1.DesiredState{Acl: &ngfwv1.AclConfig{HostSettings: &ngfwv1.HostAclSettings{AntiLockout: &ngfwv1.HostAclAntiLockout{Sources: []string{"198.51.100.0/24"}}}}}
	if !Allowed(ds, netip.MustParseAddr("198.51.100.3")) {
		t.Fatal("management source not protected")
	}
	ds.Security = &ngfwv1.SecurityConfig{AutoBlock: &ngfwv1.AutoBlock{Enabled: proto.Bool(true)}}
	ds.Acl.GlobalBlocking = &ngfwv1.GlobalBlocking{Lists: map[string]*ngfwv1.GlobalBlockingList{ListName: {}}}
	if _, err := Overlay(ds, nil, time.Now()); err == nil {
		t.Fatal("reserved list accepted")
	}
}

func TestCapacityIsExplicitAndSupportsAboveTenThousand(t *testing.T) {
	now := time.Now()
	ds := &ngfwv1.DesiredState{Security: &ngfwv1.SecurityConfig{AutoBlock: &ngfwv1.AutoBlock{Enabled: proto.Bool(true), MaxEntries: proto.Uint32(20000)}}}
	entries := make([]*ngfwv1.AutoBlockRuntimeEntry, 10001)
	for i := range entries {
		entries[i] = &ngfwv1.AutoBlockRuntimeEntry{Source: netip.AddrFrom4([4]byte{198, 18, byte(i / 256), byte(i % 256)}).String(), ExpiresAt: timestamppb.New(now.Add(time.Minute))}
	}
	if err := Validate(entries, now); err != nil {
		t.Fatal(err)
	}
	out, err := Overlay(ds, entries, now)
	if err != nil || len(out.GetAcl().GetGlobalBlocking().GetLists()[ListName].GetEntries()) != 10001 {
		t.Fatal("supported snapshot silently truncated", err)
	}
	ds.Security.AutoBlock.MaxEntries = proto.Uint32(10000)
	if out, err := Overlay(ds, entries, now); err != nil || len(out.GetAcl().GetGlobalBlocking().GetLists()[ListName].GetEntries()) != 10001 {
		t.Fatal("soft cap reduction changed authoritative API set", err)
	}
	ds.Security.AutoBlock.MaxEntries = proto.Uint32(1000000)
	if _, err := Overlay(ds, nil, now); err == nil {
		t.Fatal("unsupported host capacity accepted")
	}
}

func TestMaximumSnapshotFitsExistingGRPCBoundary(t *testing.T) {
	now := time.Now()
	request := &ngfwv1.AutoBlockSetRequest{Owner: "ngfw"}
	for i := 0; i < MaxEntries; i++ {
		request.Entries = append(request.Entries, &ngfwv1.AutoBlockRuntimeEntry{Source: netip.AddrFrom16([16]byte{0x20, 1, 0x0d, 0xb8, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, byte(i / 256), byte(i % 256)}).String(), ExpiresAt: timestamppb.New(now.Add(30 * 24 * time.Hour))})
	}
	if err := Validate(request.Entries, now); err != nil {
		t.Fatal(err)
	}
	if size := proto.Size(request); size >= 4<<20 {
		t.Fatalf("snapshot exceeds unchanged transport boundary: %d", size)
	}
}

func TestDisabledRuntimePreservesLegacyUserList(t *testing.T) {
	ds := &ngfwv1.DesiredState{Acl: &ngfwv1.AclConfig{GlobalBlocking: &ngfwv1.GlobalBlocking{Lists: map[string]*ngfwv1.GlobalBlockingList{ListName: {Entries: []string{"192.0.2.1/32"}}}}}, Security: &ngfwv1.SecurityConfig{AutoBlock: &ngfwv1.AutoBlock{Enabled: proto.Bool(false), MaxEntries: proto.Uint32(1000000)}}}
	out, err := Overlay(ds, nil, time.Now())
	if err != nil || !proto.Equal(ds, out) {
		t.Fatal("disabled runtime broke legacy user list", err)
	}
	ds.Security.AutoBlock.Enabled = proto.Bool(true)
	if _, err := Overlay(ds, nil, time.Now()); err == nil {
		t.Fatal("enabled runtime adopted forged user list")
	}
}
