package pppoe

import (
	"context"
	"errors"
	"fmt"

	"go.fd.io/govpp/api"

	"ngfw/agent/binapi/interface_types"
	pppoeapi "ngfw/agent/binapi/pppoe"
	"ngfw/agent/internal/descriptors/df6"
	"ngfw/agent/internal/scheduler"
	"ngfw/agent/internal/vpp"
)

// SessionName is the descriptor name; keys are "pppoe.session/<client_mac>/<session_id>".
const SessionName = "pppoe.session"

// Plugin is the VPP plugin providing the messages.
const Plugin = "pppoe"

// SessionDescriptor manages PPPoE session interfaces.
type SessionDescriptor = df6.IfDescriptor[*Session, *pppoeapi.PppoeSessionDetails]

// NewSession returns the descriptor for the given owner.
func NewSession(c vpp.Client, owner string) *SessionDescriptor {
	return df6.NewIfDescriptor(sessionSpec, c, owner)
}

// SessionID is the object id of a session: "<mac>/<session_id>" with the canonical MAC.
func SessionID(mac string, id uint32) (string, error) {
	m, err := df6.CanonicalMAC(mac)
	if err != nil {
		return "", err
	}
	return m + "/" + df6.U32(id), nil
}

func encode(s *Session, isAdd bool) (*pppoeapi.PppoeAddDelSession, error) {
	ip, err := df6.AddressOf(s.GetClientIp())
	if err != nil {
		return nil, err
	}
	mac, err := df6.ParseMAC(s.GetClientMac())
	if err != nil {
		return nil, err
	}
	return &pppoeapi.PppoeAddDelSession{
		IsAdd:      isAdd,
		SessionID:  uint16(s.GetSessionId()), //nolint:gosec // validated ≤ 65535
		ClientIP:   ip,
		DecapVrfID: s.GetDecapVrfId(),
		ClientMac:  mac,
	}, nil
}

var sessionSpec = df6.IfSpec[*Session, *pppoeapi.PppoeSessionDetails]{
	Name:   SessionName,
	Plugin: Plugin,
	ID: func(s *Session) (string, error) {
		if s.GetSessionId() == 0 || s.GetSessionId() > 0xffff {
			return "", fmt.Errorf("%w: session_id must be 1–65535", df6.ErrBadValue)
		}
		if _, err := df6.ParseAddr(s.GetClientIp()); err != nil {
			return "", err
		}
		return SessionID(s.GetClientMac(), s.GetSessionId())
	},
	Deps: func(s *Session) []scheduler.Dependency { return df6.VRFDeps(s.GetDecapVrfId()) },
	Add: func(ctx context.Context, c vpp.Client, _ *df6.Interfaces, s *Session) (interface_types.InterfaceIndex, error) {
		req, err := encode(s, true)
		if err != nil {
			return 0, err
		}
		rep, err := pppoeapi.NewServiceClient(c).PppoeAddDelSession(ctx, req)
		if err != nil {
			var vErr api.VPPApiError
			if errors.As(err, &vErr) && vErr == retvalInvalidSwIfIndex {
				return 0, fmt.Errorf("pppoe_add_del_session: %w: %s (%w)", ErrClientNotLearned, s.GetClientMac(), err)
			}
			return 0, fmt.Errorf("pppoe_add_del_session: %w", err)
		}
		return rep.SwIfIndex, nil
	},
	Del: func(ctx context.Context, c vpp.Client, s *Session, _ interface_types.InterfaceIndex) error {
		req, err := encode(s, false)
		if err != nil {
			return err
		}
		if _, err := pppoeapi.NewServiceClient(c).PppoeAddDelSession(ctx, req); err != nil {
			return fmt.Errorf("pppoe_add_del_session (del): %w", err)
		}
		return nil
	},
	Dump: func(ctx context.Context, c vpp.Client) ([]*pppoeapi.PppoeSessionDetails, error) {
		stream, err := pppoeapi.NewServiceClient(c).PppoeSessionDump(ctx, &pppoeapi.PppoeSessionDump{SwIfIndex: interface_types.InterfaceIndex(df6.NoInterface)})
		if err != nil {
			return nil, fmt.Errorf("pppoe_session_dump: %w", err)
		}
		return df6.Collect(stream.Recv)
	},
	Decode: func(d *pppoeapi.PppoeSessionDetails, _ *df6.Interfaces) (*Session, uint32, bool) {
		return &Session{
			SessionId:  uint32(d.SessionID),
			ClientIp:   df6.AddressString(d.ClientIP),
			ClientMac:  df6.MACString(d.ClientMac),
			DecapVrfId: d.DecapVrfID,
		}, uint32(d.SwIfIndex), true
	},
}

// retvalInvalidSwIfIndex is VNET_API_ERROR_INVALID_SW_IF_INDEX.
const retvalInvalidSwIfIndex = -2

// ErrClientNotLearned: VPP creates a PPPoE session only for a client MAC its pppoe-input node
// has already learned from discovery (PADI/PADR) packets on some interface; for an unknown MAC
// pppoe_add_del_session fails with INVALID_SW_IF_INDEX. The scheduler retries on the next
// reconcile, after the control plane has seen the client.
var ErrClientNotLearned = errors.New("pppoe client mac not learned by vpp (no discovery seen)")
