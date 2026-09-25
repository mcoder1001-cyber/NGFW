package nat44ed

import (
	"context"
	"errors"
	"fmt"
	"io"

	"ngfw/agent/binapi/nat44_ed"
	"ngfw/agent/binapi/nat_types"
	"ngfw/agent/internal/descriptors/natcommon"
)

// Retrieve-only state and action helpers for F-nat44-ed-sessions. They are not
// descriptors: sessions are not desired state. VPP dumps sessions per user
// (nat44_user_session_v3_dump takes the user's address and VRF), so listing pages over
// users first and never asks for everything in one message.

// User is one inside host with sessions (nat44_user_dump).
type User struct {
	IP             string `json:"ip"`
	VRF            uint32 `json:"vrf"`
	Sessions       uint32 `json:"sessions"`
	StaticSessions uint32 `json:"static_sessions"`
}

// Session is one translation (nat44_user_session_v3_details).
type Session struct {
	Inside     Endpoint `json:"inside"`
	Outside    Endpoint `json:"outside"`
	ExtHost    Endpoint `json:"ext_host"`
	ExtHostNAT Endpoint `json:"ext_host_nat"`
	Protocol   string   `json:"protocol"`
	Static     bool     `json:"static"`
	TwiceNAT   bool     `json:"twice_nat"`
	TimedOut   bool     `json:"timed_out"`
	// Seconds since the session was last heard from (VPP-relative).
	IdleSeconds uint64 `json:"idle_seconds"`
	TotalBytes  uint64 `json:"total_bytes"`
	TotalPkts   uint32 `json:"total_pkts"`
}

// Users lists the inside hosts that currently have sessions.
func (p *Plugin) Users(ctx context.Context) ([]User, error) {
	stream, err := p.svc.Nat44UserDump(ctx, &nat44_ed.Nat44UserDump{})
	if err != nil {
		return nil, fmt.Errorf("nat44_user_dump: %w", err)
	}
	var out []User
	for {
		d, err := stream.Recv()
		if errors.Is(err, io.EOF) {
			return out, nil
		}
		if err != nil {
			return nil, fmt.Errorf("nat44_user_dump: %w", err)
		}
		out = append(out, User{IP: natcommon.IP4String(d.IPAddress), VRF: d.VrfID, Sessions: d.Nsessions, StaticSessions: d.Nstaticsessions})
	}
}

// UserSessions returns one page (offset, limit) of the sessions of user; limit 0 means all.
// The dump itself is per user, so the page size bounds what crosses the API.
func (p *Plugin) UserSessions(ctx context.Context, user User, offset, limit int) ([]Session, error) {
	var out []Session
	i := 0
	err := p.EachUserSession(ctx, user, func(s Session) bool {
		defer func() { i++ }()
		if i < offset {
			return true
		}
		if limit > 0 && len(out) >= limit {
			return false
		}
		out = append(out, s)
		return true
	})
	return out, err
}

// EachUserSession streams the sessions of user to fn in VPP's order, one detail at a time (F-nat44-ed-sessions
// review L4: a filtered scan keeps only its page, never a whole user's session list). fn returns false to stop
// using the details; the stream is still drained to its control_ping_reply.
func (p *Plugin) EachUserSession(ctx context.Context, user User, fn func(Session) bool) error {
	ip, err := natcommon.IP4(user.IP)
	if err != nil {
		return err
	}
	stream, err := p.svc.Nat44UserSessionV3Dump(ctx, &nat44_ed.Nat44UserSessionV3Dump{IPAddress: ip, VrfID: user.VRF})
	if err != nil {
		return fmt.Errorf("nat44_user_session_v3_dump: %w", err)
	}
	more := true
	for {
		d, err := stream.Recv()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("nat44_user_session_v3_dump: %w", err)
		}
		if !more {
			continue // keep draining: the stream must reach control_ping_reply
		}
		more = fn(Session{
			Inside:     Endpoint{IP: natcommon.IP4String(d.InsideIPAddress), Port: uint32(d.InsidePort)},
			Outside:    Endpoint{IP: natcommon.IP4String(d.OutsideIPAddress), Port: uint32(d.OutsidePort)},
			ExtHost:    Endpoint{IP: natcommon.IP4String(d.ExtHostAddress), Port: uint32(d.ExtHostPort)},
			ExtHostNAT: Endpoint{IP: natcommon.IP4String(d.ExtHostNatAddress), Port: uint32(d.ExtHostNatPort)},
			Protocol:   natcommon.ProtoName(uint8(d.Protocol)), //nolint:gosec // IP protocol numbers are 8-bit
			Static:     d.Flags&nat_types.NAT_IS_STATIC != 0,
			TwiceNAT:   d.Flags&nat_types.NAT_IS_TWICE_NAT != 0,
			TimedOut:   d.IsTimedOut, IdleSeconds: d.TimeSinceLastHeard, TotalBytes: d.TotalBytes, TotalPkts: d.TotalPkts,
		})
	}
}

// DeleteSession removes one session (nat44_del_session): the inside endpoint, protocol and
// VRF identify it; ExtHost narrows it to one destination when set.
func (p *Plugin) DeleteSession(ctx context.Context, inside Endpoint, protocol string, vrf uint32, extHost Endpoint) error {
	addr, err := natcommon.IP4(inside.IP)
	if err != nil {
		return err
	}
	proto, err := natcommon.ProtoNumber(protocol)
	if err != nil {
		return err
	}
	pt, err := port(inside.Port)
	if err != nil {
		return err
	}
	req := &nat44_ed.Nat44DelSession{Address: addr, Protocol: proto, Port: pt, VrfID: vrf, Flags: nat_types.NAT_IS_INSIDE}
	if extHost.IP != "" {
		ext, err := natcommon.IP4(extHost.IP)
		if err != nil {
			return err
		}
		ep, err := port(extHost.Port)
		if err != nil {
			return err
		}
		req.ExtHostAddress, req.ExtHostPort, req.Flags = ext, ep, req.Flags|nat_types.NAT_IS_EXT_HOST_VALID
	}
	if _, err := p.svc.Nat44DelSession(ctx, req); err != nil {
		return fmt.Errorf("nat44_del_session: %w", err)
	}
	return nil
}
