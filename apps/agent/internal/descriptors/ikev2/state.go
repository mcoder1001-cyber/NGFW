package ikev2

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"ngfw/agent/binapi/ikev2"
	"ngfw/agent/binapi/ikev2_types"
	"ngfw/agent/internal/descriptors/vpn"
	"ngfw/agent/internal/vpp"
)

// SAState is the read-only state of one IKE SA of an owned profile (ikev2_sa_v3_dump) with its
// child SAs (ikev2_child_sa_v2_dump). It is state, not configuration: never part of a desired
// Value. VPP returns the derived keys (SK_d, SK_ai, …) in both dumps; they are zeroed on receipt
// and have no field here.
type SAState struct {
	Index      uint32 // VPP SA index (ikev2_child_sa_v2_dump / traffic_selector_dump argument)
	Profile    string // desired profile name (owner prefix removed)
	State      string // ikev2_state enum name: SA_INIT, AUTHENTICATED, AUTH_FAILED, …
	ISPI       uint64
	RSPI       uint64
	IAddr      string
	RAddr      string
	IID        *IDState
	RID        *IDState
	Encryption string // "aes-cbc/256", "aes-gcm-16/128", …
	Integrity  string
	PRF        string
	DH         string
	Stats      ikev2_types.Ikev2SaStats
	Uptime     float64 // seconds since authentication
	Children   []ChildSAState
}

// IDState is a decoded IKE identity.
type IDState struct{ Type, Value string }

// ChildSAState is one child (ESP) SA.
type ChildSAState struct {
	Index      uint32
	ISPI       uint32
	RSPI       uint32
	Encryption string
	Integrity  string
	ESN        bool
	Uptime     float64
}

// IKE transform types (ikev2.h ikev2_transform_type_t: ENCR=1, PRF=2, INTEG=3, DH=4, ESN=5).
const (
	trEncr  uint8 = 1
	trPRF   uint8 = 2
	trInteg uint8 = 3
	trDH    uint8 = 4
	trESN   uint8 = 5
)

// SAs returns the IKE SAs of every profile owned by owner, with their child SAs.
func SAs(ctx context.Context, c vpp.Client, owner string) ([]SAState, error) {
	svc := ikev2.NewServiceClient(c)
	stream, err := svc.Ikev2SaV3Dump(ctx, &ikev2.Ikev2SaV3Dump{})
	if err != nil {
		return nil, fmt.Errorf("ikev2_sa_v3_dump: %w", err)
	}
	var out []SAState
	for {
		det, err := stream.Recv()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("ikev2_sa_v3_dump: %w", err)
		}
		sa := det.Sa
		zeroKeys(&sa.Keys)
		name, ok := strings.CutPrefix(sa.ProfileName, owner+"-")
		if !ok {
			continue
		}
		out = append(out, SAState{
			Index: sa.SaIndex, Profile: name, State: ikev2_types.Ikev2State_name[uint32(sa.State)],
			ISPI: sa.Ispi, RSPI: sa.Rspi, IAddr: vpn.AddressString(sa.Iaddr), RAddr: vpn.AddressString(sa.Raddr),
			IID: idState(sa.IID), RID: idState(sa.RID),
			Encryption: transformName(sa.Encryption), Integrity: transformName(sa.Integrity),
			PRF: transformName(sa.Prf), DH: transformName(sa.Dh), Stats: sa.Stats, Uptime: sa.Uptime,
		})
	}
	for i := range out {
		children, err := childSAs(ctx, svc, out[i].Index)
		if err != nil {
			return nil, err
		}
		out[i].Children = children
	}
	return out, nil
}

func childSAs(ctx context.Context, svc ikev2.RPCService, saIndex uint32) ([]ChildSAState, error) {
	stream, err := svc.Ikev2ChildSaV2Dump(ctx, &ikev2.Ikev2ChildSaV2Dump{SaIndex: saIndex})
	if err != nil {
		return nil, fmt.Errorf("ikev2_child_sa_v2_dump: %w", err)
	}
	var out []ChildSAState
	for {
		det, err := stream.Recv()
		if errors.Is(err, io.EOF) {
			return out, nil
		}
		if err != nil {
			return nil, fmt.Errorf("ikev2_child_sa_v2_dump: %w", err)
		}
		ch := det.ChildSa
		zeroKeys(&ch.Keys)
		out = append(out, ChildSAState{
			Index: ch.ChildSaIndex, ISPI: ch.ISpi, RSPI: ch.RSpi,
			Encryption: transformName(ch.Encryption), Integrity: transformName(ch.Integrity),
			ESN: ch.Esn.TransformType == trESN && ch.Esn.TransformID == 1, Uptime: ch.Uptime,
		})
	}
}

func zeroKeys(k *ikev2_types.Ikev2Keys) {
	for _, b := range [][]byte{k.SkD, k.SkAi, k.SkAr, k.SkEi, k.SkEr, k.SkPi, k.SkPr} {
		vpn.Zero(b)
	}
}

func idState(id ikev2_types.Ikev2ID) *IDState {
	v := decodeID(id)
	if v == nil {
		return nil
	}
	return &IDState{Type: v.GetType(), Value: v.GetValue()}
}

// transformName names a negotiated transform; "" when VPP reports none.
func transformName(t ikev2_types.Ikev2SaTransform) string {
	id := uint8(t.TransformID) //nolint:gosec // IANA ids of the tables fit in a byte
	switch t.TransformType {
	case trEncr:
		if t.KeyLen != 0 {
			return fmt.Sprintf("%s/%d", encrAlgs.name(id), int(t.KeyLen)*8)
		}
		return encrAlgs.name(id)
	case trPRF:
		return prfAlgs.name(id)
	case trInteg:
		return integAlgs.name(id)
	case trDH:
		return dhGroups.name(id) // transform_id is a union over encr/prf/integ/dh/esn in VPP
	}
	return ""
}
