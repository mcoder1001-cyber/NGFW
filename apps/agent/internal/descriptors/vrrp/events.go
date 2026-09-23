package vrrp

import (
	"context"
	"fmt"

	"go.fd.io/govpp/api"

	"ngfw/agent/binapi/vrrp"
	"ngfw/agent/internal/descriptors/df7"
	"ngfw/agent/internal/vpp"
)

// VR runtime states (vrrp.VrrpVrState).
const (
	StateInit     = "init"
	StateBackup   = "backup"
	StateMaster   = "master"
	StateIntfDown = "interface-down"
)

// StateName maps a VR state to its name.
func StateName(s vrrp.VrrpVrState) string {
	switch s {
	case vrrp.VRRP_API_VR_STATE_INIT:
		return StateInit
	case vrrp.VRRP_API_VR_STATE_BACKUP:
		return StateBackup
	case vrrp.VRRP_API_VR_STATE_MASTER:
		return StateMaster
	case vrrp.VRRP_API_VR_STATE_INTF_DOWN:
		return StateIntfDown
	}
	return fmt.Sprintf("#%d", s)
}

// Event is one vrrp_vr_event: a VR (its vrrp.vr key) moving between states — the shape
// StreamEvents forwards.
type Event struct {
	Key      string
	VR       VR
	OldState string
	NewState string
}

// DecodeEvent converts an event; ok is false for VRs on interfaces this owner does not own.
func DecodeEvent(m *vrrp.VrrpVrEvent, ifs *df7.Interfaces) (Event, bool) {
	name, ok := ifs.Owned(uint32(m.Vr.SwIfIndex), func(n string) string {
		return string(KeyVR(VR{Interface: n, VRID: m.Vr.VrID, IPv6: m.Vr.IsIPv6 != 0}))
	})
	if !ok {
		return Event{}, false
	}
	v := VR{Interface: name, VRID: m.Vr.VrID, IPv6: m.Vr.IsIPv6 != 0}
	return Event{Key: string(KeyVR(v)), VR: v, OldState: StateName(m.OldState), NewState: StateName(m.NewState)}, true
}

// WatchEvents registers for VRRP events (want_vrrp_vr_events) and streams this owner's VR
// transitions until ctx ends.
func WatchEvents(ctx context.Context, c vpp.Client, owner string, opts ...df7.Option) (<-chan Event, error) {
	o := df7.BuildOptions(opts)
	want := func(ctx context.Context, enable bool, pid uint32) error {
		_, err := vrrp.NewServiceClient(c).WantVrrpVrEvents(ctx, &vrrp.WantVrrpVrEvents{EnableDisable: enable, PID: pid})
		if err != nil {
			return fmt.Errorf("want_vrrp_vr_events %v: %w", enable, df7.PluginError("vrrp", err))
		}
		return nil
	}
	decode := func(ctx context.Context, m api.Message) (Event, bool) {
		ev, ok := m.(*vrrp.VrrpVrEvent)
		if !ok {
			return Event{}, false
		}
		ifs, err := df7.DumpInterfaces(ctx, c, owner, o)
		if err != nil {
			return Event{}, false
		}
		return DecodeEvent(ev, ifs)
	}
	return df7.Watch(ctx, c, &vrrp.VrrpVrEvent{}, want, decode)
}

// VRState is the live state of one VR (read-only, for the state API).
type VRState struct {
	Key         string
	State       string
	Priority    uint8 // current (after tracking decrements)
	MasterAdvCS uint16
}

// States returns the live state of this owner's VRs (vrrp_vr_dump runtime part).
func States(ctx context.Context, c vpp.Client, owner string, opts ...df7.Option) ([]VRState, error) {
	vrs, _, err := ownedVRs(ctx, df7.NewBase(NameVR, c, owner, opts))
	if err != nil {
		return nil, err
	}
	out := make([]VRState, 0, len(vrs))
	for _, o := range vrs {
		out = append(out, VRState{Key: string(KeyVR(o.VR)), State: StateName(o.Detail.Runtime.State),
			Priority: o.Detail.Runtime.Tracking.Priority, MasterAdvCS: o.Detail.Runtime.MasterAdvInt})
	}
	return out, nil
}
