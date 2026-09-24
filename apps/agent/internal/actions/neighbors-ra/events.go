package neighborsra

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"go.fd.io/govpp/api"

	"ngfw/agent/binapi/interface_types"
	"ngfw/agent/binapi/ip_neighbor"
	vrxv1 "ngfw/agent/gen/vrx/v1"
	"ngfw/agent/internal/descriptors/df2"
	"ngfw/agent/internal/vpp"
)

// Subscribe (enable) or unsubscribe this API client from the neighbour events of ONE interface
// (want_ip_neighbor_events_v2, ip = zero address = every neighbour of the interface). It refuses sw_if_index ~0 and 0:
// VPP treats both as "every interface" (ip_neighbor_watch maps 0 to ~0), which on a shared VPP means every slot's.
func Subscribe(ctx context.Context, c vpp.Client, swIfIndex, pid uint32, enable bool) error {
	if swIfIndex == 0 || swIfIndex == df2.NoInterface {
		return fmt.Errorf("neighbors: refusing want_ip_neighbor_events_v2 for sw_if_index %d (all interfaces)", swIfIndex)
	}
	_, err := ip_neighbor.NewServiceClient(c).WantIPNeighborEventsV2(ctx, &ip_neighbor.WantIPNeighborEventsV2{
		Enable: enable, PID: pid, SwIfIndex: interface_types.InterfaceIndex(swIfIndex),
	})
	if err != nil {
		return fmt.Errorf("want_ip_neighbor_events_v2 (sw_if_index %d, enable %v): %w", swIfIndex, enable, err)
	}
	return nil
}

// Change is one decoded ip_neighbor_event_v2.
type Change struct {
	SwIfIndex uint32
	Added     bool
	Removed   bool
	IP        string
	MAC       string
}

// DecodeEvent decodes an ip_neighbor_event_v2 (false for any other message).
func DecodeEvent(m api.Message) (Change, bool) {
	ev, ok := m.(*ip_neighbor.IPNeighborEventV2)
	if !ok {
		return Change{}, false
	}
	return Change{
		SwIfIndex: uint32(ev.Neighbor.SwIfIndex),
		Added:     ev.Flags&ip_neighbor.IP_NEIGHBOR_API_EVENT_FLAG_ADDED != 0,
		Removed:   ev.Flags&ip_neighbor.IP_NEIGHBOR_API_EVENT_FLAG_REMOVED != 0,
		IP:        df2.FromAddress(ev.Neighbor.IPAddress).String(),
		MAC:       df2.MACString(ev.Neighbor.MacAddress),
	}, true
}

// MaxInterfacesPerEvent: above this many changed interfaces in one flush period the coalescer publishes one event
// without `interface` instead of one per interface.
const MaxInterfacesPerEvent = 16

// Coalescer accumulates neighbour changes between two flushes (1 Hz in the watcher): the event stream carries at most
// one EVENT_KIND_NEIGHBOR_CHANGED per interface per period, however busy the table is (prompt default: 1 Hz
// coalescing). Not safe for concurrent use.
type Coalescer struct {
	per map[string]*counts
}

type counts struct{ added, removed, updated int }

// Add records one change on the interface with logical name name.
func (c *Coalescer) Add(name string, ch Change) {
	if c.per == nil {
		c.per = map[string]*counts{}
	}
	n := c.per[name]
	if n == nil {
		n = &counts{}
		c.per[name] = n
	}
	switch {
	case ch.Removed:
		n.removed++
	case ch.Added:
		n.added++
	default:
		n.updated++ // MAC / state change of an existing entry
	}
}

// Pending reports whether a flush would publish anything.
func (c *Coalescer) Pending() bool { return len(c.per) > 0 }

// Flush returns the events of the period (sorted by interface) and resets the counts.
func (c *Coalescer) Flush() []*vrxv1.Event {
	if len(c.per) == 0 {
		return nil
	}
	names := make([]string, 0, len(c.per))
	for n := range c.per {
		names = append(names, n)
	}
	sort.Strings(names)
	defer func() { c.per = nil }()
	attrs := func(n *counts) map[string]string {
		return map[string]string{"added": strconv.Itoa(n.added), "removed": strconv.Itoa(n.removed), "updated": strconv.Itoa(n.updated)}
	}
	if len(names) > MaxInterfacesPerEvent {
		sum := &counts{}
		for _, n := range c.per {
			sum.added += n.added
			sum.removed += n.removed
			sum.updated += n.updated
		}
		a := attrs(sum)
		a["interfaces"] = strconv.Itoa(len(names))
		return []*vrxv1.Event{{
			Kind:       vrxv1.EventKind_EVENT_KIND_NEIGHBOR_CHANGED,
			Message:    fmt.Sprintf("neighbour tables of %d interfaces changed", len(names)),
			Attributes: a,
		}}
	}
	out := make([]*vrxv1.Event, 0, len(names))
	for _, name := range names {
		n := c.per[name]
		var parts []string
		for _, p := range []struct {
			v    int
			what string
		}{{n.added, "added"}, {n.removed, "removed"}, {n.updated, "updated"}} {
			if p.v > 0 {
				parts = append(parts, fmt.Sprintf("%d %s", p.v, p.what))
			}
		}
		out = append(out, &vrxv1.Event{
			Kind:       vrxv1.EventKind_EVENT_KIND_NEIGHBOR_CHANGED,
			Interface:  &name,
			Message:    fmt.Sprintf("neighbours on %s: %s", name, strings.Join(parts, ", ")),
			Attributes: attrs(n),
		})
	}
	return out
}
