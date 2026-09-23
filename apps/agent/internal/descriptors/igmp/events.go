package igmp

import (
	"context"
	"fmt"
	"net/netip"

	"go.fd.io/govpp/api"

	"ngfw/agent/binapi/igmp"
	"ngfw/agent/internal/descriptors/df7"
	"ngfw/agent/internal/vpp"
)

// Event is one igmp_event: a (S,G) membership on an interface was added (filter include) or
// removed (filter exclude) — the shape StreamEvents forwards.
type Event struct {
	Interface string
	Group     string
	Source    string
	Filter    string // "include" | "exclude"
}

// DecodeEvent converts an event; ok is false unless the interface carries this owner's IGMP
// configuration (its igmp.interface object owns it).
func DecodeEvent(m *igmp.IgmpEvent, ifs *df7.Interfaces) (Event, bool) {
	name, ok := ifs.Owned(uint32(m.SwIfIndex), func(n string) string { return string(KeyInterface(n)) })
	if !ok {
		return Event{}, false
	}
	f := "exclude"
	if m.Filter == igmp.INCLUDE {
		f = "include"
	}
	return Event{Interface: name, Group: netip.AddrFrom4(m.Gaddr).String(), Source: netip.AddrFrom4(m.Saddr).String(), Filter: f}, true
}

// WatchEvents registers for IGMP events (want_igmp_events) and streams this owner's
// membership changes until ctx ends.
func WatchEvents(ctx context.Context, c vpp.Client, owner string, opts ...df7.Option) (<-chan Event, error) {
	o := df7.BuildOptions(opts)
	want := func(ctx context.Context, enable bool, pid uint32) error {
		var e uint32
		if enable {
			e = 1
		}
		_, err := igmp.NewServiceClient(c).WantIgmpEvents(ctx, &igmp.WantIgmpEvents{Enable: e, PID: pid})
		if err != nil {
			return fmt.Errorf("want_igmp_events %v: %w", enable, df7.PluginError("igmp", err))
		}
		return nil
	}
	decode := func(ctx context.Context, m api.Message) (Event, bool) {
		ev, ok := m.(*igmp.IgmpEvent)
		if !ok {
			return Event{}, false
		}
		ifs, err := df7.DumpInterfaces(ctx, c, owner, o)
		if err != nil {
			return Event{}, false
		}
		return DecodeEvent(ev, ifs)
	}
	return df7.Watch(ctx, c, &igmp.IgmpEvent{}, want, decode)
}
