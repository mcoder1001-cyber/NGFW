package bfd

import (
	"context"
	"fmt"

	"go.fd.io/govpp/api"

	"ngfw/agent/binapi/bfd"
	"ngfw/agent/internal/descriptors/df7"
	"ngfw/agent/internal/vpp"
)

// Session states (bfd.BfdState) as reported in events and Sessions.
const (
	StateAdminDown = "admin-down"
	StateDown      = "down"
	StateInit      = "init"
	StateUp        = "up"
)

// StateName maps a BFD state to its name.
func StateName(s bfd.BfdState) string {
	switch s {
	case bfd.BFD_STATE_API_ADMIN_DOWN:
		return StateAdminDown
	case bfd.BFD_STATE_API_DOWN:
		return StateDown
	case bfd.BFD_STATE_API_INIT:
		return StateInit
	case bfd.BFD_STATE_API_UP:
		return StateUp
	}
	return fmt.Sprintf("#%d", s)
}

// SessionEvent is one bfd_udp_session_event: the session key and its new local state. It is
// the shape StreamEvents forwards (key = the bfd.udp-session key of the session).
type SessionEvent struct {
	Key       string
	Interface string
	Local     string
	Peer      string
	State     string
}

// DecodeEvent converts an event; ok is false for multihop sessions and sessions on interfaces
// this owner does not own.
func DecodeEvent(m *bfd.BfdUDPSessionEvent, ifs *df7.Interfaces) (SessionEvent, bool) {
	local, peer := df7.FromAddress(m.LocalAddr).String(), df7.FromAddress(m.PeerAddr).String()
	name, ok := ifs.Owned(uint32(m.SwIfIndex), func(n string) string { return string(KeySession(n, local, peer)) })
	if !ok {
		return SessionEvent{}, false
	}
	e := SessionEvent{Interface: name, Local: local, Peer: peer, State: StateName(m.State)}
	e.Key = string(KeySession(e.Interface, e.Local, e.Peer))
	return e, true
}

// WatchEvents registers for BFD events (want_bfd_events) and streams this owner's session state
// changes until ctx ends.
func WatchEvents(ctx context.Context, c vpp.Client, owner string, opts ...df7.Option) (<-chan SessionEvent, error) {
	o := df7.BuildOptions(opts)
	want := func(ctx context.Context, enable bool, pid uint32) error {
		_, err := bfd.NewServiceClient(c).WantBfdEvents(ctx, &bfd.WantBfdEvents{EnableDisable: enable, PID: pid})
		if err != nil {
			return fmt.Errorf("want_bfd_events %v: %w", enable, err)
		}
		return nil
	}
	decode := func(ctx context.Context, m api.Message) (SessionEvent, bool) {
		ev, ok := m.(*bfd.BfdUDPSessionEvent)
		if !ok {
			return SessionEvent{}, false
		}
		ifs, err := df7.DumpInterfaces(ctx, c, owner, o)
		if err != nil {
			return SessionEvent{}, false
		}
		return DecodeEvent(ev, ifs)
	}
	return df7.Watch(ctx, c, &bfd.BfdUDPSessionEvent{}, want, decode)
}

// SessionState is the read-only state of a session (for the state API).
type SessionState struct {
	Key   string
	State string
}

// Sessions returns the live state of this owner's sessions (bfd_udp_session_dump).
func Sessions(ctx context.Context, c vpp.Client, owner string, opts ...df7.Option) ([]SessionState, error) {
	ifs, err := df7.DumpInterfaces(ctx, c, owner, df7.BuildOptions(opts))
	if err != nil {
		return nil, err
	}
	dets, err := dumpSessions(ctx, c)
	if err != nil {
		return nil, err
	}
	var out []SessionState
	for _, d := range dets {
		local, peer := df7.FromAddress(d.LocalAddr).String(), df7.FromAddress(d.PeerAddr).String()
		name, ok := ifs.Owned(uint32(d.SwIfIndex), func(n string) string { return string(KeySession(n, local, peer)) })
		if !ok {
			continue
		}
		out = append(out, SessionState{Key: string(KeySession(name, local, peer)), State: StateName(d.State)})
	}
	return out, nil
}
