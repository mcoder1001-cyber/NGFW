// Package autoblock projects the API-owned runtime block set through the existing
// Global Blocking engines. No runtime address is written to the configuration.
package autoblock

import (
	"fmt"
	"net/netip"
	"sort"
	"time"

	"google.golang.org/protobuf/proto"
	ngfwv1 "ngfw/agent/gen/ngfw/v1"
)

// ListName is reserved for the system-owned runtime Global Blocking list.
const ListName = "auto-block"

// MaxEntries bounds the entire snapshot below the existing gRPC 4-MiB receive limit.
const MaxEntries = 20000

// Validate rejects an entire malformed snapshot before it can replace the live set.
func Validate(entries []*ngfwv1.AutoBlockRuntimeEntry, now time.Time) error {
	if len(entries) > MaxEntries {
		return fmt.Errorf("auto-block snapshot exceeds %d entries", MaxEntries)
	}
	seen := map[netip.Addr]bool{}
	for _, e := range entries {
		a, err := netip.ParseAddr(e.GetSource())
		if err != nil || a.Zone() != "" || a.IsUnspecified() || a.IsMulticast() {
			return fmt.Errorf("invalid auto-block source")
		}
		a = a.Unmap()
		if seen[a] {
			return fmt.Errorf("duplicate auto-block source")
		}
		seen[a] = true
		if e.GetExpiresAt() == nil || e.GetExpiresAt().CheckValid() != nil || e.GetExpiresAt().AsTime().After(now.Add(30*24*time.Hour)) {
			return fmt.Errorf("invalid auto-block expiry")
		}
	}
	return nil
}

// Allowed always protects loopback and configured management networks, even when
// the host anti-lockout rule is disabled. Invalid prefixes fail safe at projection.
func Allowed(ds *ngfwv1.DesiredState, addr netip.Addr) bool {
	addr = addr.Unmap()
	if addr.IsLoopback() {
		return true
	}
	ps := append([]string{}, ds.GetSecurity().GetAutoBlock().GetAllowlist()...)
	ps = append(ps, ds.GetAcl().GetHostSettings().GetAntiLockout().GetSources()...)
	for _, raw := range ps {
		p, err := netip.ParsePrefix(raw)
		if err != nil {
			if a, e := netip.ParseAddr(raw); e == nil && a.Zone() == "" {
				p = netip.PrefixFrom(a.Unmap(), a.Unmap().BitLen())
				err = nil
			}
		}
		if err != nil {
			return true
		}
		if p.Contains(addr) {
			return true
		}
		if p.Addr().Is4In6() && p.Bits() >= 96 && netip.PrefixFrom(p.Addr().Unmap(), p.Bits()-96).Contains(addr) {
			return true
		}
	}
	return false
}

// Overlay returns a cloned view for VPP ACL and nftables projection. The reserved
// list is never permitted in a user's configuration; the running document remains
// byte-for-byte unchanged. Expiry is evaluated by the agent's clock too.
func Overlay(ds *ngfwv1.DesiredState, entries []*ngfwv1.AutoBlockRuntimeEntry, now time.Time) (*ngfwv1.DesiredState, error) {
	out := &ngfwv1.DesiredState{}
	if ds != nil {
		out = proto.Clone(ds).(*ngfwv1.DesiredState)
	}
	cfg := ds.GetSecurity().GetAutoBlock()
	if !cfg.GetEnabled() {
		return out, nil
	}
	if ds.GetAcl().GetGlobalBlocking().GetLists()[ListName] != nil {
		return nil, fmt.Errorf("global block list %q is reserved for runtime auto-block", ListName)
	}
	if cfg.GetMaxEntries() > MaxEntries {
		return nil, fmt.Errorf("auto-block maxEntries exceeds host capability %d", MaxEntries)
	}
	var prefixes []string
	for _, e := range entries {
		if e.GetExpiresAt() == nil || !e.GetExpiresAt().AsTime().After(now) {
			continue
		}
		a, err := netip.ParseAddr(e.GetSource())
		if err != nil {
			continue
		}
		a = a.Unmap()
		if Allowed(ds, a) {
			continue
		}
		prefixes = append(prefixes, netip.PrefixFrom(a, a.BitLen()).String())
	}
	sort.Strings(prefixes)
	if len(prefixes) > MaxEntries {
		return nil, fmt.Errorf("auto-block snapshot exceeds host capability %d", MaxEntries)
	}
	if len(prefixes) == 0 {
		return out, nil
	}
	if out.Acl == nil {
		out.Acl = &ngfwv1.AclConfig{}
	}
	if out.Acl.GlobalBlocking == nil {
		out.Acl.GlobalBlocking = &ngfwv1.GlobalBlocking{}
	}
	if out.Acl.GlobalBlocking.Lists == nil {
		out.Acl.GlobalBlocking.Lists = map[string]*ngfwv1.GlobalBlockingList{}
	}
	out.Acl.GlobalBlocking.Lists[ListName] = &ngfwv1.GlobalBlockingList{Enabled: proto.Bool(true), AllInterfaces: proto.Bool(true), Direction: proto.String("both"), ProtectHost: proto.Bool(true), Log: proto.Bool(true), Entries: prefixes, Source: &ngfwv1.GlobalBlockingSource{Kind: proto.String("upload")}}
	return out, nil
}
