package ipfix

import (
	"context"
	"errors"
	"fmt"
	"sort"

	"go.fd.io/govpp/api"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/structpb"

	"ngfw/agent/binapi/ip_types"
	"ngfw/agent/binapi/ipfix_export"
	"ngfw/agent/internal/descriptors/classify"
	"ngfw/agent/internal/descriptors/dfkit"
	"ngfw/agent/internal/scheduler"
	"ngfw/agent/internal/vpp"
)

// ClassifyStream is the ipfix.classify-stream singleton (set_ipfix_classify_stream): the
// observation domain and UDP source port of the classify-table reports. VPP's default is
// "unset" (src_port 0); the tables can only be added once it is set.
type ClassifyStream struct {
	DomainID uint32 `json:"domain_id"`
	SrcPort  uint16 `json:"src_port"`
}

// Proto returns the canonical structpb document.
func (s ClassifyStream) Proto() *structpb.Struct { return dfkit.Encode(s) }

// ClassifyTable is one classify table reported over IPFIX (ipfix_classify_table_add_del): the
// DF-2 classify table by NAME (key classify.table/<name>; its VPP index is resolved from DF-2's
// store right before every call, never kept — review H2), the IP version of its records and the
// transport protocol whose ports are reported (0 = none).
type ClassifyTable struct {
	Table     string `json:"table"`
	IPVersion string `json:"ip_version"` // "ip4" | "ip6"
	Protocol  uint8  `json:"protocol"`
}

// Proto returns the canonical structpb document.
func (s ClassifyTable) Proto() *structpb.Struct { return dfkit.Encode(s) }

// ---- ipfix.classify-stream ------------------------------------------------------------------

// ClassifyStreamID is the object id of the singleton (key ipfix.classify-stream/global).
const ClassifyStreamID = "global"

// KeyClassifyStream is the key of the singleton.
var KeyClassifyStream = scheduler.Join(NameClassifyStream, ClassifyStreamID)

// ClassifyStreamDescriptor manages the ipfix.classify-stream singleton (globals owner only,
// D-071). Retrieve is write-only (ErrClassifyDumpBroken); Delete resets it to domain 0 / port 0
// ("unset").
type ClassifyStreamDescriptor struct {
	client vpp.Client
	o      options
}

var _ scheduler.Descriptor = (*ClassifyStreamDescriptor)(nil)

// NewClassifyStream returns the ipfix.classify-stream descriptor.
func NewClassifyStream(client vpp.Client, opts ...Option) *ClassifyStreamDescriptor {
	return &ClassifyStreamDescriptor{client: client, o: buildOptions(opts)}
}

// Name implements scheduler.Descriptor.
func (*ClassifyStreamDescriptor) Name() string { return NameClassifyStream }

// KeyOf implements scheduler.Descriptor.
func (*ClassifyStreamDescriptor) KeyOf(proto.Message) scheduler.Key { return KeyClassifyStream }

// Dependencies implements scheduler.Descriptor: the default exporter (the reports go to exporter
// 0; optional, the stream can be set before the collector).
func (*ClassifyStreamDescriptor) Dependencies(proto.Message) []scheduler.Dependency {
	return []scheduler.Dependency{{Key: KeyDefaultExporter, Optional: true}}
}

func (d *ClassifyStreamDescriptor) set(ctx context.Context, s ClassifyStream) error {
	_, err := ipfix_export.NewServiceClient(d.client).SetIpfixClassifyStream(ctx,
		&ipfix_export.SetIpfixClassifyStream{DomainID: s.DomainID, SrcPort: s.SrcPort})
	if err != nil {
		return fmt.Errorf("set_ipfix_classify_stream(%d, %d): %w", s.DomainID, s.SrcPort, err)
	}
	return nil
}

func (d *ClassifyStreamDescriptor) apply(ctx context.Context, obj proto.Message) error {
	var s ClassifyStream
	if err := dfkit.Decode(obj, &s); err != nil {
		return err
	}
	if s.SrcPort == 0 {
		return dfkit.Specf("ipfix classify stream: src_port must be set (0 means unset)")
	}
	if !d.o.globals.Owner() {
		return d.o.globals.Require(ctx, NameClassifyStream, obj, nil) // no working getter
	}
	return d.set(ctx, s)
}

// Create implements scheduler.Descriptor.
func (d *ClassifyStreamDescriptor) Create(ctx context.Context, obj proto.Message) (any, error) {
	return nil, d.apply(ctx, obj)
}

// Update implements scheduler.Descriptor (in place; VPP moves the existing reports).
func (d *ClassifyStreamDescriptor) Update(ctx context.Context, _, newObj proto.Message, _ any) (any, error) {
	return nil, d.apply(ctx, newObj)
}

// Delete implements scheduler.Descriptor: back to the unset state (globals owner only).
func (d *ClassifyStreamDescriptor) Delete(ctx context.Context, _ proto.Message, _ any) error {
	if !d.o.globals.Owner() {
		return nil
	}
	return d.set(ctx, ClassifyStream{})
}

// ErrClassifyDumpBroken explains why the classify descriptors are write-only: VPP 26.06 sends
// ipfix_classify_stream_details and ipfix_classify_table_details with the bare message id
// (flow_api.c: ntohs (VL_API_IPFIX_CLASSIFY_*_DETAILS) without REPLY_MSG_ID_BASE), so the client
// receives them as an unrelated message (host run 2026-09-24: "No subscription found for the
// notification message … msgId=12") and the dump returns nothing. Recorded for docs/vpp-code-track.md
// in DF-8-questions.md. The dumps are not sent at all, so no stray message reaches govpp.
var ErrClassifyDumpBroken = fmt.Errorf("%w: ipfix_classify_*_details carry a wrong message id in VPP 26.06", dfkit.ErrRetrieveUnsupported)

// Retrieve implements scheduler.Descriptor: write-only (D-063), see ErrClassifyDumpBroken.
func (d *ClassifyStreamDescriptor) Retrieve(context.Context) ([]scheduler.KV, error) {
	return nil, fmt.Errorf("%s: %w", NameClassifyStream, ErrClassifyDumpBroken)
}

// ---- ipfix.classify-table -------------------------------------------------------------------

// ClassifyTableDescriptor manages ipfix.classify-table objects: key ipfix.classify-table/<name>.
// Only tables in this owner's DF-2 classify store can be named (ownership comes from DF-2).
type ClassifyTableDescriptor struct {
	client vpp.Client
	store  classify.Store
	o      options
}

var _ scheduler.Descriptor = (*ClassifyTableDescriptor)(nil)

// NewClassifyTable returns the ipfix.classify-table descriptor over the owner's DF-2 classify store.
func NewClassifyTable(client vpp.Client, store classify.Store, opts ...Option) *ClassifyTableDescriptor {
	return &ClassifyTableDescriptor{client: client, store: store, o: buildOptions(opts)}
}

// Name implements scheduler.Descriptor.
func (*ClassifyTableDescriptor) Name() string { return NameClassifyTable }

// KeyOf implements scheduler.Descriptor.
func (*ClassifyTableDescriptor) KeyOf(obj proto.Message) scheduler.Key {
	var s ClassifyTable
	if err := dfkit.Decode(obj, &s); err != nil {
		return scheduler.Join(NameClassifyTable, "invalid")
	}
	return scheduler.Join(NameClassifyTable, s.Table)
}

// Dependencies implements scheduler.Descriptor: DF-2's classify table (mandatory) and the
// classify stream (optional: it is registered by the globals owner only, L2; VPP refuses the add
// while it is unset).
func (d *ClassifyTableDescriptor) Dependencies(obj proto.Message) []scheduler.Dependency {
	var s ClassifyTable
	if err := dfkit.Decode(obj, &s); err != nil {
		return nil
	}
	return []scheduler.Dependency{
		{Key: classify.TableKey(s.Table)},
		{Key: KeyClassifyStream, Optional: true},
	}
}

// ErrNoClassifyTable means the named table is not a live table of this owner's DF-2 store.
var ErrNoClassifyTable = errors.New("ipfix: no such classify table of this owner")

// tableIndex resolves a DF-2 table name to its live VPP index (classify.LiveTables verifies the VPP
// instance and the table geometry, so a reused index is never taken for ours).
func (d *ClassifyTableDescriptor) tableIndex(ctx context.Context, name string) (uint32, error) {
	recs, err := classify.LiveTables(ctx, d.client, d.store)
	if err != nil {
		return 0, err
	}
	for _, r := range recs {
		if r.Name == name {
			return r.Index, nil
		}
	}
	return 0, fmt.Errorf("%w: %q", ErrNoClassifyTable, name)
}

func (d *ClassifyTableDescriptor) addDel(ctx context.Context, obj proto.Message, add bool) error {
	var s ClassifyTable
	if err := dfkit.Decode(obj, &s); err != nil {
		return err
	}
	af := ip_types.ADDRESS_IP4
	switch s.IPVersion {
	case "ip4":
	case "ip6":
		af = ip_types.ADDRESS_IP6
	default:
		return dfkit.Specf("ipfix classify table: ip_version %q must be ip4 or ip6", s.IPVersion)
	}
	if s.Table == "" {
		return dfkit.Specf("ipfix classify table: table name is empty")
	}
	idx, err := d.tableIndex(ctx, s.Table) // right before the call (D-071, H2)
	if err != nil {
		return err
	}
	_, err = ipfix_export.NewServiceClient(d.client).IpfixClassifyTableAddDel(ctx, &ipfix_export.IpfixClassifyTableAddDel{
		TableID: idx, IPVersion: af, TransportProtocol: ip_types.IPProto(s.Protocol), IsAdd: add,
	})
	if err != nil {
		return fmt.Errorf("ipfix_classify_table_add_del(%s=%d, add=%t): %w", s.Table, idx, add, err)
	}
	return nil
}

// Create implements scheduler.Descriptor; VALUE_EXIST (already reported) is success.
func (d *ClassifyTableDescriptor) Create(ctx context.Context, obj proto.Message) (any, error) {
	err := d.addDel(ctx, obj, true)
	if dfkit.IsVPPError(err, api.VALUE_EXIST) {
		return nil, nil
	}
	return nil, err
}

// Update implements scheduler.Descriptor: version/protocol are fixed per report — recreate.
func (d *ClassifyTableDescriptor) Update(context.Context, proto.Message, proto.Message, any) (any, error) {
	return nil, scheduler.ErrRecreate
}

// Delete implements scheduler.Descriptor; NO_SUCH_ENTRY, and a table that is no longer a live
// table of ours (DF-2 deleted it, or VPP restarted), count as deleted — an index that may now be
// someone else's is never used.
func (d *ClassifyTableDescriptor) Delete(ctx context.Context, obj proto.Message, _ any) error {
	err := d.addDel(ctx, obj, false)
	if dfkit.IsVPPError(err, api.NO_SUCH_ENTRY) || errors.Is(err, ErrNoClassifyTable) {
		return nil
	}
	return err
}

// Retrieve implements scheduler.Descriptor: write-only (D-063), see ErrClassifyDumpBroken.
// DecodeClassifyTables is the decoder to switch to once VPP sends the details correctly.
func (d *ClassifyTableDescriptor) Retrieve(context.Context) ([]scheduler.KV, error) {
	return nil, fmt.Errorf("%s: %w", NameClassifyTable, ErrClassifyDumpBroken)
}

// DecodeClassifyTables turns ipfix_classify_table_details into KVs for the live tables of this
// owner's DF-2 store (index → name), sorted: the Retrieve body once the VPP message-id bug is
// fixed; unit-tested now.
func (d *ClassifyTableDescriptor) DecodeClassifyTables(details []*ipfix_export.IpfixClassifyTableDetails, live []classify.TableRecord) []scheduler.KV {
	names := map[uint32]string{}
	for _, r := range live {
		names[r.Index] = r.Name
	}
	var out []scheduler.KV
	for _, t := range details {
		name, ok := names[t.TableID]
		if !ok {
			continue
		}
		s := ClassifyTable{Table: name, IPVersion: "ip4", Protocol: uint8(t.TransportProtocol)}
		if t.IPVersion == ip_types.ADDRESS_IP6 {
			s.IPVersion = "ip6"
		}
		out = append(out, scheduler.KV{Key: scheduler.Join(NameClassifyTable, name), Value: s.Proto()})
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Key < out[j].Key })
	return dfkit.Dedupe(out)
}
