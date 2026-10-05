package bfd

import (
	"context"
	"encoding/json"
	"fmt"

	"ngfw/agent/internal/descriptors/df7"
	"ngfw/agent/internal/descriptors/dfkit"
	"ngfw/agent/internal/vpp/bootid"
)

// sessionProof records only a successful native add, never an add intent. It
// permits recovery when the later interface/endpoint claim flush fails.
type sessionProof struct {
	Identity     string
	Interface    string
	LogicalIndex uint32
	NativeIndex  uint32
	Local        string
	Peer         string
}

func recoveryKey(idx uint32, local, peer string) string {
	return fmt.Sprintf("bfd.session-recovery/%d/%s/%s", idx, local, peer)
}

func (d *SessionDescriptor) writeProof(p sessionProof) error {
	raw, err := json.Marshal(p)
	if err != nil {
		return err
	}
	return df7.BootStoreFor(d.Owner).Put(dfkit.BootRecord{Key: recoveryKey(p.NativeIndex, p.Local, p.Peer), Identity: p.Identity, Value: string(raw)})
}

func (d *SessionDescriptor) storedProof(idx uint32, local, peer string) (*sessionProof, error) {
	rec, ok := df7.BootStoreFor(d.Owner).Get(recoveryKey(idx, local, peer))
	if !ok {
		return nil, nil
	}
	var p sessionProof
	if err := json.Unmarshal([]byte(rec.Value), &p); err != nil {
		return nil, fmt.Errorf("invalid BFD recovery record: %w", err)
	}
	if p.Identity != rec.Identity || p.NativeIndex != idx || p.Local != local || p.Peer != peer {
		return nil, fmt.Errorf("BFD recovery record does not match exact session")
	}
	return &p, nil
}

func (d *SessionDescriptor) readProof(ctx context.Context, idx uint32, local, peer string) (*sessionProof, error) {
	p, err := d.storedProof(idx, local, peer)
	if err != nil || p == nil {
		return nil, err
	}
	valid, err := d.validProof(ctx, *p)
	if err != nil || !valid {
		return nil, err
	}
	return p, nil
}

func (d *SessionDescriptor) validProof(ctx context.Context, p sessionProof) (bool, error) {
	id, err := dfkit.IdentitySource(ctx, d.Client)
	if err != nil {
		return false, err
	}
	if !id.Complete() || !bootid.Matches(p.Identity, id) {
		return false, nil
	}
	tg, err := d.Target(ctx, p.Interface, string(KeySession(p.Interface, p.Local, p.Peer)))
	if err != nil {
		return false, err
	}
	return tg.Index == p.LogicalIndex, nil
}
