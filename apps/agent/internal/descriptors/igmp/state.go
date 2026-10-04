package igmp

import (
	"context"
	"net/netip"
	bin "ngfw/agent/binapi/igmp"
	"ngfw/agent/binapi/interface_types"
	"ngfw/agent/internal/descriptors/df7"
	"ngfw/agent/internal/vpp"
	"sort"
)

// Group is an owned live membership, including router mode groups.
type Group struct {
	Interface, Group string
	Sources          []string
}

// DumpGroups retrieves deduplicated memberships on owned IGMP interfaces.
func DumpGroups(ctx context.Context, c vpp.Client, owner string) ([]Group, error) {
	ifs, e := df7.DumpInterfaces(ctx, c, owner, df7.BuildOptions(nil))
	if e != nil {
		return nil, e
	}
	stream, e := bin.NewServiceClient(c).IgmpDump(ctx, &bin.IgmpDump{SwIfIndex: interface_types.InterfaceIndex(^uint32(0))})
	if e != nil {
		return nil, e
	}
	rows, e := df7.Collect(stream.Recv)
	if e != nil {
		return nil, e
	}
	groups := map[string]*Group{}
	for _, r := range rows {
		name, owned := ifs.Owned(uint32(r.SwIfIndex), func(n string) string { return string(KeyInterface(n)) })
		if !owned {
			continue
		}
		g := netip.AddrFrom4(r.Gaddr).String()
		k := name + "/" + g
		v := groups[k]
		if v == nil {
			v = &Group{Interface: name, Group: g}
			groups[k] = v
		}
		source := netip.AddrFrom4(r.Saddr).String()
		found := false
		for _, s := range v.Sources {
			if s == source {
				found = true
			}
		}
		if !found {
			v.Sources = append(v.Sources, source)
		}
	}
	keys := make([]string, 0, len(groups))
	for k := range groups {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var out []Group
	for _, k := range keys {
		v := groups[k]
		sort.Strings(v.Sources)
		out = append(out, *v)
	}
	return out, nil
}
