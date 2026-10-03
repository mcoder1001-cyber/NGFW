package strongswan

import (
	"context"
	"strconv"

	"google.golang.org/protobuf/types/known/timestamppb"

	vrxv1 "ngfw/agent/gen/vrx/v1"
)

// StateProto reads charon's state through VICI and returns it as the typed IPsec state message
// (P11 contract, IpsecState rpc). It never contains key material — VICI does not expose it. The
// owner prefix that scopes which connections are reported is the renderer's WithOwnerPrefix.
func (r *Renderer) StateProto(ctx context.Context) (*vrxv1.IpsecStateResponse, error) {
	st, err := r.State(ctx)
	if err != nil {
		return nil, err
	}
	return r.stateProtoFrom(st), nil
}

// stateProtoFrom converts an already-read State to the proto message (pure; used by StateProto and
// unit tests that build a State by hand).
func (r *Renderer) stateProtoFrom(st *State) *vrxv1.IpsecStateResponse {
	out := &vrxv1.IpsecStateResponse{
		Owner:           r.owner,
		RetrievedAt:     timestamppb.New(r.now().UTC()),
		EventsActive:    r.subscribed.Load(), // a live VICI event subscription, not "charon answers"
		CharonRestarted: st.Restarted,
		DaemonVersion:   st.Daemon.Version,
		UnlistedSas:     st.UnlistedSAs,
	}
	for _, c := range st.Conns {
		pc := &vrxv1.IpsecConnState{
			Name: c.Name, Tunnel: c.Tunnel, Version: c.Version,
			LocalAddrs: c.LocalAddrs, RemoteAddrs: c.RemoteAddrs,
			LocalId: c.LocalID, RemoteId: c.RemoteID,
			LocalAuth: c.LocalAuth, RemoteAuth: c.RemoteAuth,
			RekeySec: c.RekeyTime, ReauthSec: c.ReauthTime,
		}
		for _, ch := range c.Children {
			pc.Children = append(pc.Children, &vrxv1.IpsecChildConn{
				Name: ch.Name, Mode: ch.Mode, RekeySec: ch.RekeyTime,
				LocalTs: ch.LocalTS, RemoteTs: ch.RemoteTS,
			})
		}
		out.Conns = append(out.Conns, pc)
	}
	for _, sa := range st.SAs {
		psa := &vrxv1.IpsecIkeSa{
			Name: sa.Name, Tunnel: sa.Tunnel, UniqueId: sa.UniqueID, Version: sa.Version, State: sa.State,
			LocalHost: sa.LocalHost, LocalPort: u32(sa.LocalPort), LocalId: sa.LocalID,
			RemoteHost: sa.RemoteHost, RemotePort: u32(sa.RemotePort), RemoteId: sa.RemoteID,
			Initiator: sa.Initiator, NatAny: sa.NATAny,
			EncrAlg: sa.EncrAlg, EncrKeysize: u32(sa.EncrKeysize), IntegAlg: sa.IntegAlg,
			PrfAlg: sa.PRFAlg, DhGroup: sa.DHGroup,
			EstablishedSec: sa.EstablishedSec, RekeySec: sa.RekeySec, ReauthSec: sa.ReauthSec,
		}
		for _, ch := range sa.Children {
			psa.Children = append(psa.Children, &vrxv1.IpsecChildSa{
				Name: ch.Name, UniqueId: ch.UniqueID, ReqId: u32(ch.ReqID), State: ch.State,
				Mode: ch.Mode, Protocol: ch.Protocol, Encap: ch.Encap,
				SpiIn: ch.SPIIn, SpiOut: ch.SPIOut,
				EncrAlg: ch.EncrAlg, EncrKeysize: u32(ch.EncrKeysize), IntegAlg: ch.IntegAlg, DhGroup: ch.DHGroup,
				Esn:     ch.ESN,
				BytesIn: ch.BytesIn, PacketsIn: ch.PacketsIn, BytesOut: ch.BytesOut, PacketsOut: ch.PacketsOut,
				RekeySec: ch.RekeySec, LifeSec: ch.LifeSec, InstallSec: ch.InstallSec,
				LocalTs: ch.LocalTS, RemoteTs: ch.RemoteTS, IfIdIn: ch.IfIDIn, IfIdOut: ch.IfIDOut,
			})
		}
		out.Sas = append(out.Sas, psa)
	}
	return out
}

// u32 parses a decimal VICI value; anything else (absent, malformed, out of range) is 0.
func u32(s string) uint32 {
	v, err := strconv.ParseUint(s, 10, 32)
	if err != nil {
		return 0
	}
	return uint32(v)
}
