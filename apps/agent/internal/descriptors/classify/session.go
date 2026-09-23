package classify

import (
	"bytes"
	"context"
	"encoding/hex"
	"fmt"

	"google.golang.org/protobuf/proto"

	classifyapi "ngfw/agent/binapi/classify"
	"ngfw/agent/internal/descriptors/df2"
	"ngfw/agent/internal/scheduler"
	"ngfw/agent/internal/vpp"
)

// SessionName is the descriptor name; keys are "classify.session/<table>/<hex(match)>".
const SessionName = "classify.session"

// TrimMatch returns the canonical match: trailing zero bytes removed. The API needs the
// match padded to (skip + match) × 16 bytes; Create pads, Retrieve trims, so desired state
// may be given either way.
func TrimMatch(m []byte) []byte { return bytes.TrimRight(m, "\x00") }

// MatchID is the key segment of a match.
func MatchID(m []byte) string {
	t := TrimMatch(m)
	if len(t) == 0 {
		return "0"
	}
	return hex.EncodeToString(t)
}

// NormalizeSession returns s with the match trimmed.
func NormalizeSession(s *Session) *Session {
	n := proto.Clone(s).(*Session)
	n.Match = TrimMatch(n.Match)
	return n
}

// PadMatch pads m to the table geometry or fails when it is longer.
func PadMatch(m []byte, rec TableRecord) ([]byte, error) {
	n := int(rec.SkipNVectors+rec.MatchNVectors) * VectorSize
	if len(TrimMatch(m)) > n {
		return nil, fmt.Errorf("match of %d bytes exceeds the table's %d bytes", len(TrimMatch(m)), n)
	}
	return fit(m, n), nil
}

// SessionDescriptor manages classifier sessions (classify_add_del_session).
type SessionDescriptor struct {
	client vpp.Client
	store  Store
}

// NewSession returns the descriptor backed by store.
func NewSession(c vpp.Client, store Store) *SessionDescriptor {
	return &SessionDescriptor{client: c, store: store}
}

// SessionMeta is the runtime handle: the table index.
type SessionMeta struct{ TableIndex uint32 }

// Name implements scheduler.Descriptor.
func (*SessionDescriptor) Name() string { return SessionName }

// KeyOf implements scheduler.Descriptor.
func (*SessionDescriptor) KeyOf(obj proto.Message) scheduler.Key {
	s := obj.(*Session)
	return scheduler.Join(SessionName, s.GetTable(), MatchID(s.GetMatch()))
}

// Dependencies implements scheduler.Descriptor: the table.
func (*SessionDescriptor) Dependencies(obj proto.Message) []scheduler.Dependency {
	return []scheduler.Dependency{{Key: TableKey(obj.(*Session).GetTable())}}
}

func (d *SessionDescriptor) addDel(ctx context.Context, s *Session, rec TableRecord, isAdd bool) error {
	match, err := PadMatch(s.GetMatch(), rec)
	if err != nil {
		return fmt.Errorf("%s: %w", SessionName, err)
	}
	req := &classifyapi.ClassifyAddDelSession{
		IsAdd:        isAdd,
		TableIndex:   rec.Index,
		HitNextIndex: s.GetHitNextIndex(),
		OpaqueIndex:  s.GetOpaqueIndex(),
		Advance:      s.GetAdvance(),
		Action:       classifyapi.ClassifyAction(s.GetAction()), //nolint:gosec // enum 0..3
		Metadata:     s.GetMetadata(),
		MatchLen:     uint32(len(match)), //nolint:gosec // ≤ 80
		Match:        match,
	}
	if _, err := classifyapi.NewServiceClient(d.client).ClassifyAddDelSession(ctx, req); err != nil {
		return fmt.Errorf("classify_add_del_session: %w", err)
	}
	return nil
}

// Create implements scheduler.Descriptor.
func (d *SessionDescriptor) Create(ctx context.Context, obj proto.Message) (any, error) {
	s := obj.(*Session)
	rec, ok := d.store.Get(s.GetTable())
	if !ok {
		return nil, fmt.Errorf("%w: %q", ErrNoSuchTable, s.GetTable())
	}
	if err := d.addDel(ctx, s, rec, true); err != nil {
		return nil, err
	}
	if rec.Sessions == nil {
		rec.Sessions = map[string]SessionRecord{}
	}
	rec.Sessions[MatchID(s.GetMatch())] = SessionRecord{Action: int32(s.GetAction()), Metadata: s.GetMetadata()}
	if err := d.store.Put(rec); err != nil {
		return nil, err
	}
	return SessionMeta{TableIndex: rec.Index}, nil
}

// Update implements scheduler.Descriptor: sessions are replaced, never edited.
func (*SessionDescriptor) Update(context.Context, proto.Message, proto.Message, any) (any, error) {
	return nil, scheduler.ErrRecreate
}

// Delete implements scheduler.Descriptor.
func (d *SessionDescriptor) Delete(ctx context.Context, obj proto.Message, meta any) error {
	s := obj.(*Session)
	rec, ok := d.store.Get(s.GetTable())
	if !ok {
		return fmt.Errorf("%w: %q", ErrNoSuchTable, s.GetTable())
	}
	if m, ok := meta.(SessionMeta); ok && m.TableIndex != rec.Index {
		return fmt.Errorf("%s: meta table index %d does not match the store's %d", SessionName, m.TableIndex, rec.Index)
	}
	if err := d.addDel(ctx, s, rec, false); err != nil {
		return err
	}
	delete(rec.Sessions, MatchID(s.GetMatch()))
	return d.store.Put(rec)
}

// Retrieve dumps the sessions of every owned table (classify_session_dump); action and
// metadata come from the store because VPP does not report them.
func (d *SessionDescriptor) Retrieve(ctx context.Context) ([]scheduler.KV, error) {
	recs, err := LiveTables(ctx, d.client, d.store)
	if err != nil {
		return nil, err
	}
	svc := classifyapi.NewServiceClient(d.client)
	var out []scheduler.KV
	for _, rec := range recs {
		stream, err := svc.ClassifySessionDump(ctx, &classifyapi.ClassifySessionDump{TableID: rec.Index})
		if err != nil {
			return nil, fmt.Errorf("classify_session_dump %d: %w", rec.Index, err)
		}
		details, err := df2.Collect(stream.Recv)
		if err != nil {
			return nil, fmt.Errorf("classify_session_dump %d: %w", rec.Index, err)
		}
		for _, det := range details {
			// VPP dumps the key after the skipped vectors; restore the full match layout.
			match := TrimMatch(append(make([]byte, int(rec.SkipNVectors)*VectorSize), det.Match...))
			v := &Session{Table: rec.Name, Match: match, HitNextIndex: det.HitNextIndex, OpaqueIndex: det.OpaqueIndex, Advance: det.Advance}
			if sr, ok := rec.Sessions[MatchID(match)]; ok {
				v.Action, v.Metadata = Session_Action(sr.Action), sr.Metadata
			}
			out = append(out, scheduler.KV{Key: d.KeyOf(v), Value: v, Meta: SessionMeta{TableIndex: rec.Index}})
		}
	}
	return out, nil
}
